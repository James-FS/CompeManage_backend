package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxFileSize = 30 << 20

var uploadDirs = map[string]string{
	"notice":           "static/notices",
	"reg_attachment":   "static/reg_attachments",
	"competition_work": "static/reg_attachments",
	"award_proof":      "static/award_proofs",
	"temp":             "static/temp",
}

var allowedExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".pdf": true, ".doc": true, ".docx": true,
	".xls": true, ".xlsx": true, ".csv": true,
	".zip": true, ".rar": true, ".ppt": true, ".pptx": true,
}

// calcFileMD5 计算上传文件的 MD5 哈希
func calcFileMD5(file *multipart.FileHeader) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	hash := md5.New()
	if _, err := io.Copy(hash, src); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// sanitizeFilename 清理文件名，去除危险字符
func sanitizeFilename(filename string) string {
	filename = filepath.Base(filename)
	filename = strings.ReplaceAll(filename, " ", "_")
	filename = strings.ReplaceAll(filename, "..", "")
	return filename
}

func UploadFile(c *gin.Context) {
	bizType := c.PostForm("type")
	savePathRoot, ok := uploadDirs[bizType]
	if !ok {
		savePathRoot = uploadDirs["temp"]
	}

	file, err := c.FormFile("file")
	if err != nil {
		utils.BadRequest(c, "未检测到文件")
		return
	}

	if file.Size > maxFileSize {
		utils.BadRequest(c, "文件大小不能超过30MB")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedExts[ext] {
		utils.BadRequest(c, "不支持的文件格式(仅支持图片、PDF、Word、Excel、压缩包)")
		return
	}

	db := database.DB.WithContext(c.Request.Context())

	// 计算文件 MD5
	fileHash, err := calcFileMD5(file)
	if err != nil {
		utils.InternalServerError(c, "文件校验失败", err)
		return
	}

	// 获取上传用户ID
	var uploaderID uint
	if id, exists := c.Get("user_id"); exists {
		uploaderID = id.(uint)
	}

	// ext 已在文件格式校验时声明过（line 76），这里复用
	originalName := sanitizeFilename(file.Filename)

	// 用 INSERT 作为分布式锁：先插入记录，抢到锁的才写文件
	// INSERT 失败（1062 duplicate key）→ 抢不到锁 → 查询已有记录返回
	record := models.FileRecord{
		FileHash:     fileHash,
		OriginalName: originalName,
		StoragePath:  "", // 先占位，文件保存成功后再填入
		FileSize:     file.Size,
		FileExt:      ext,
		BizType:      bizType,
		UploaderID:   uploaderID,
	}
	if err := db.Create(&record).Error; err != nil {
		// INSERT 失败，说明另一个请求先插成功了，直接返回已有记录
		var existingRecord models.FileRecord
		if err2 := db.Where("file_hash = ? AND biz_type = ?", fileHash, bizType).First(&existingRecord).Error; err2 == nil {
			utils.Success(c, gin.H{
				"url":       buildDownloadURL(existingRecord.BizType, existingRecord.FileHash, existingRecord.OriginalName),
				"name":      existingRecord.OriginalName,
				"size":      existingRecord.FileSize,
				"mime_type": existingRecord.FileExt,
			})
			return
		}
		utils.InternalServerError(c, "文件记录保存失败", err)
		return
	}

	// INSERT 成功，获得了分布式锁，开始保存物理文件
	subDir := time.Now().Format("200601")
	finalDir := filepath.Join(savePathRoot, subDir)
	if err := os.MkdirAll(finalDir, os.ModePerm); err != nil {
		db.Delete(&record) // 目录创建失败，删记录释放锁
		utils.InternalServerError(c, "服务器存储初始化失败", err)
		return
	}

	newFileName := fmt.Sprintf("%s_%s", fileHash, originalName)
	dst := filepath.Join(finalDir, newFileName)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		db.Delete(&record) // 文件保存失败，删记录释放锁
		utils.InternalServerError(c, "文件保存失败", err)
		return
	}

	// 文件保存成功，更新 storage_path 并返回
	storagePath := "/" + filepath.ToSlash(dst)
	db.Model(&record).Updates(map[string]interface{}{
		"storage_path": storagePath,
	})
	// 返回受控下载 URL，前端访问时走 /api/file/download 受权限校验
	utils.Success(c, gin.H{
		"url":       buildDownloadURL(bizType, fileHash, originalName),
		"name":      originalName,
		"size":      file.Size,
		"mime_type": ext,
	})
}

// buildDownloadURL 构造受控下载 URL
func buildDownloadURL(bizType, fileHash, originalName string) string {
	return fmt.Sprintf("/api/file/download/%s/%s_%s", bizType, fileHash, originalName)
}

// DownloadFile 处理文件下载/预览，支持权限校验
// URL: GET /api/file/download/:type/:filename
func DownloadFile(c *gin.Context) {
	fileType := c.Param("type")
	filename := c.Param("filename")

	if _, ok := uploadDirs[fileType]; !ok {
		utils.BadRequest(c, "不支持的文件类型")
		return
	}

	// 从 filename 提取 md5（格式: md5_originalname）
	md5Part := strings.SplitN(filename, "_", 2)[0]
	if len(md5Part) != 32 {
		utils.BadRequest(c, "无效的文件名格式")
		return
	}

	ctx := c.Request.Context()
	db := database.DB.WithContext(ctx)

	// 查询 file_records 验证文件存在且属于该 biz_type
	var record models.FileRecord
	if err := db.Where("file_hash = ? AND biz_type = ?", md5Part, fileType).First(&record).Error; err != nil {
		utils.NotFound(c, "文件不存在")
		return
	}

	// 鉴权：上传者本人 / 管理员 / 赛事负责人 / 同团队成员
	if !canAccessFile(ctx, c, fileType, record) {
		utils.Forbidden(c, "无权限访问此文件")
		return
	}

	// 拼接物理文件路径
	relativePath := strings.TrimPrefix(record.StoragePath, "/")
	filePath := filepath.FromSlash(relativePath)

	if _, err := os.Stat(filePath); err != nil {
		utils.NotFound(c, "文件已丢失，请重新上传")
		return
	}

	// 设置 Content-Disposition：attachment 触发下载，inline 内嵌预览
	// 中文 filename 用 URL 编码防止乱码
	encodedName := url.QueryEscape(record.OriginalName)
	disposition := fmt.Sprintf("attachment; filename*=UTF-8''%s", encodedName)
	c.Header("Content-Disposition", disposition)
	c.Header("Content-Type", "application/octet-stream")

	data, err := os.ReadFile(filePath)
	if err != nil {
		utils.NotFound(c, "文件读取失败")
		return
	}
	c.Data(200, "application/octet-stream", data)
}

// canAccessFile 判断当前用户是否有权限访问指定文件
func canAccessFile(ctx context.Context, c *gin.Context, fileType string, record models.FileRecord) bool {
	userID, exists := c.Get("user_id")
	if !exists {
		return false
	}
	uid := userID.(uint)

	// 上传者本人
	if record.UploaderID == uid {
		return true
	}

	// 校管理员可访问全部受控文件；院管理员仅可访问管理学院赛事文件。
	scope, err := GetUserAccessScope(ctx, uid)
	if err == nil {
		if scope.IsSchoolAdmin() {
			return true
		}
		if scope.IsCollegeAdmin() && scope.ManagedCollegeID != nil {
			compID := findCompIDByStoragePath(ctx, record.StoragePath)
			if compID != 0 {
				var comp models.CompDirectory
				if database.DB.WithContext(ctx).Select("id", "manager_id", "college_id").First(&comp, compID).Error == nil &&
					canAccessCompetition(scope, comp) {
					return true
				}
			}
		}
	}

	switch fileType {
	case "notice":
		return true
	case "reg_attachment":
		if isTeamMemberForFile(ctx, uid, record.StoragePath) {
			return true
		}
		return isCompManagerForFile(ctx, uid, record.StoragePath)
	case "competition_work":
		if isTeamMemberForFile(ctx, uid, record.StoragePath) {
			return true
		}
		if isCompManagerForFile(ctx, uid, record.StoragePath) {
			return true
		}
		return isExpertForFile(ctx, uid, record.StoragePath)
	case "award_proof":
		return isCompManagerForFile(ctx, uid, record.StoragePath)
	case "temp":
		return false
	}

	return false
}

// findCompIDByStoragePath 根据文件物理路径找到关联赛事的 comp_id
// storagePath 如 /static/reg_attachments/202606/md5_originalname
// register 表中存的是下载 URL 如 /api/file/download/competition_work/md5_originalname
func findCompIDByStoragePath(ctx context.Context, storagePath string) uint {
	// 从物理路径提取文件名部分 (md5_originalname)
	idx := strings.LastIndexByte(storagePath, '/')
	if idx < 1 {
		return 0
	}
	fileName := storagePath[idx+1:]
	parts := strings.SplitN(fileName, "_", 2)
	if len(parts) != 2 || len(parts[0]) != 32 {
		return 0
	}
	// 用 md5 hash 模糊匹配 register 中的下载 URL
	var compID uint
	database.DB.WithContext(ctx).Table("registers").
		Select("comp_id").
		Where("attachment_url LIKE ? OR work_attachment_url LIKE ?", "%"+parts[0]+"%", "%"+parts[0]+"%").
		Scan(&compID)
	return compID
}

// isCompManagerForFile 检查用户是否为该文件所属赛事的负责人
func isCompManagerForFile(ctx context.Context, userID uint, storagePath string) bool {
	compID := findCompIDByStoragePath(ctx, storagePath)
	if compID > 0 {
		return isCompManager(ctx, userID, compID)
	}
	// 非 register 关联的文件（如 award_proof），尝试从 awards 表查找
	var mID uint
	err := database.DB.WithContext(ctx).Table("awards").
		Joins("JOIN competitions ON competitions.id = awards.comp_id").
		Select("comp_directories.manager_id").
		Joins("JOIN comp_directories ON comp_directories.id = competitions.comp_directory_id").
		Where("awards.proof_url LIKE ?", "%"+extractMD5FromPath(storagePath)+"%").
		Scan(&mID).Error
	if err != nil || mID == 0 {
		return false
	}
	return mID == userID
}

// isTeamMemberForFile 检查用户是否为使用该文件的团队成员
func isTeamMemberForFile(ctx context.Context, userID uint, storagePath string) bool {
	var count int64
	database.DB.WithContext(ctx).Table("registers").
		Joins("JOIN reg_members ON reg_members.reg_id = registers.id").
		Where("reg_members.user_id = ? AND (registers.attachment_url LIKE ? OR registers.work_attachment_url LIKE ?)", userID, "%"+extractMD5FromPath(storagePath)+"%", "%"+extractMD5FromPath(storagePath)+"%").
		Count(&count)
	return count > 0
}

// extractMD5FromPath 从存储路径中提取 32 位 MD5 哈希
func extractMD5FromPath(storagePath string) string {
	idx := strings.LastIndexByte(storagePath, '/')
	if idx < 1 {
		return ""
	}
	fileName := storagePath[idx+1:]
	parts := strings.SplitN(fileName, "_", 2)
	if len(parts) != 2 || len(parts[0]) != 32 {
		return ""
	}
	return parts[0]
}

// isExpertForFile 检查用户是否是指定评审该文件所属赛事的专家
func isExpertForFile(ctx context.Context, userID uint, storagePath string) bool {
	compID := findCompIDByStoragePath(ctx, storagePath)
	if compID == 0 {
		return false
	}
	var count int64
	database.DB.WithContext(ctx).Table("review_tasks").
		Where("comp_id = ? AND expert_id = ?", compID, userID).
		Count(&count)
	return count > 0
}

// isCompManager 检查用户是否为该赛事的负责人
func isCompManager(ctx context.Context, userID uint, compID uint) bool {
	var managerID uint
	err := database.DB.WithContext(ctx).Table("comp_directories").
		Select("manager_id").
		Where("id = ?", compID).
		Scan(&managerID).Error
	if err != nil {
		return false
	}
	return managerID == userID
}
