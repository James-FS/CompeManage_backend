package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxFileSize = 30 << 20

const maxBusinessAttachments = 5
const maxWorkAttachments = 10

func attachmentURLCount(value string) int {
	return len(attachmentURLs(value))
}

func attachmentURLsToLegacyString(value string) string {
	return strings.Join(attachmentURLs(value), ",")
}

func attachmentURLs(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "[") {
		var refs []struct {
			URL string `json:"url"`
		}
		if json.Unmarshal([]byte(value), &refs) == nil {
			urls := make([]string, 0, len(refs))
			for _, ref := range refs {
				if url := strings.TrimSpace(ref.URL); url != "" {
					urls = append(urls, url)
				}
			}
			return urls
		}
	}
	urls := make([]string, 0)
	for _, url := range strings.Split(value, ",") {
		if url = strings.TrimSpace(url); url != "" {
			urls = append(urls, url)
		}
	}
	return urls
}

var uploadDirs = map[string]string{
	"notice":             "static/notices",
	"notice_attachment":  "static/notices", // Legacy alias used by older notice upload forms.
	"reg_attachment":     "static/reg_attachments",
	"competition_work":   "static/reg_attachments",
	"award_proof":        "static/award_proofs",
	"award_cert":         "static/temp", // Legacy student uploads retained this business type.
	"declare_attachment": "static/declare_attachments",
	"summary_attachment": "static/summary_attachments",
	"temp":               "static/temp", // Legacy temporary uploads; new forms should use a business type.
}

func extensionSet(exts ...string) map[string]bool {
	allowed := make(map[string]bool, len(exts))
	for _, ext := range exts {
		allowed[ext] = true
	}
	return allowed
}

var knownMimeTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".pdf":  "application/pdf",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".csv":  "text/csv; charset=utf-8",
	".zip":  "application/zip",
	".rar":  "application/vnd.rar",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
}

func mimeTypeForExtension(ext string) string {
	if contentType, ok := knownMimeTypes[strings.ToLower(ext)]; ok {
		return contentType
	}
	if contentType := mime.TypeByExtension(ext); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

var allUploadExts = extensionSet(
	".jpg", ".jpeg", ".png", ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".csv", ".zip", ".rar", ".ppt", ".pptx",
)

var officeDocumentExts = extensionSet(".pdf", ".doc", ".docx", ".xls", ".xlsx", ".csv")
var registrationAttachmentExts = extensionSet(".pdf", ".doc", ".docx", ".xls", ".xlsx", ".csv", ".zip")
var awardProofExts = extensionSet(".jpg", ".jpeg", ".png", ".pdf", ".doc", ".docx")

// uploadAllowedExts mirrors the file picker policies while keeping the server authoritative.
var uploadAllowedExts = map[string]map[string]bool{
	"notice":             allUploadExts,
	"notice_attachment":  allUploadExts,
	"reg_attachment":     registrationAttachmentExts,
	"competition_work":   allUploadExts,
	"award_proof":        awardProofExts,
	"award_cert":         awardProofExts,
	"declare_attachment": officeDocumentExts,
	"summary_attachment": extensionSet(".pdf", ".doc", ".docx", ".xls", ".xlsx", ".csv", ".zip", ".rar"),
	"temp":               allUploadExts,
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
	filename = strings.ReplaceAll(filename, ",", "_")
	filename = strings.ReplaceAll(filename, "..", "")
	return filename
}

func UploadFile(c *gin.Context) {
	bizType := strings.TrimSpace(c.PostForm("type"))
	savePathRoot, ok := uploadDirs[bizType]
	if !ok {
		utils.BadRequest(c, "上传业务类型无效")
		return
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
	if !uploadAllowedExts[bizType][ext] {
		utils.BadRequest(c, "该类附件不支持此文件格式")
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
				"file_ext":  existingRecord.FileExt,
				"mime_type": mimeTypeForExtension(existingRecord.FileExt),
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
		"file_ext":  ext,
		"mime_type": mimeTypeForExtension(ext),
	})
}

// buildDownloadURL 构造受控下载 URL
func buildDownloadURL(bizType, fileHash, originalName string) string {
	return fmt.Sprintf("/api/file/download/%s/%s_%s", bizType, fileHash, url.PathEscape(originalName))
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

	// 设置原始文件名，前端按返回的 MIME 类型决定预览或下载。
	// 中文 filename 用 URL 编码防止乱码。
	encodedName := url.QueryEscape(record.OriginalName)
	disposition := fmt.Sprintf("attachment; filename*=UTF-8''%s", encodedName)
	c.Header("Content-Disposition", disposition)
	contentType := mimeTypeForExtension(record.FileExt)
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")

	data, err := os.ReadFile(filePath)
	if err != nil {
		utils.NotFound(c, "文件读取失败")
		return
	}
	c.Data(200, contentType, data)
}

// DeleteUnlinkedUpload removes an uploaded file after a form discards it, but only while no business record references it.
func DeleteUnlinkedUpload(c *gin.Context) {
	fileType := c.Param("type")
	filename := c.Param("filename")
	if _, ok := uploadDirs[fileType]; !ok {
		utils.BadRequest(c, "不支持的文件类型")
		return
	}
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) != 2 || len(parts[0]) != 32 {
		utils.BadRequest(c, "无效的文件名格式")
		return
	}

	var record models.FileRecord
	if err := database.DB.WithContext(c.Request.Context()).
		Where("file_hash = ? AND biz_type = ?", parts[0], fileType).
		First(&record).Error; err != nil {
		utils.Success(c, gin.H{"removed": false})
		return
	}
	uidValue, exists := c.Get("user_id")
	uid, valid := uidValue.(uint)
	if !exists || !valid || record.UploaderID != uid || fileRecordIsReferenced(record) {
		utils.Success(c, gin.H{"removed": false})
		return
	}

	if record.StoragePath != "" {
		filePath := filepath.FromSlash(strings.TrimPrefix(record.StoragePath, "/"))
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			utils.InternalServerError(c, "清理未使用附件失败", err)
			return
		}
	}
	if err := database.DB.WithContext(c.Request.Context()).Delete(&record).Error; err != nil {
		utils.InternalServerError(c, "清理附件记录失败", err)
		return
	}
	utils.Success(c, gin.H{"removed": true})
}

func fileRecordIsReferenced(record models.FileRecord) bool {
	pattern := "%" + record.FileHash + "%"
	checks := []struct {
		model interface{}
		query string
	}{
		{&models.Notice{}, "attachment LIKE ?"},
		{&models.CompDeclaration{}, "attachment_path LIKE ?"},
		{&models.Summary{}, "attachments LIKE ?"},
		{&models.Register{}, "attachment_url LIKE ? OR work_attachment_url LIKE ?"},
		{&models.Award{}, "proof_url LIKE ?"},
	}
	for _, check := range checks {
		var count int64
		args := []interface{}{pattern}
		if strings.Count(check.query, "?") == 2 {
			args = append(args, pattern)
		}
		if err := database.DB.Model(check.model).Where(check.query, args...).Count(&count).Error; err != nil {
			return true
		}
		if count > 0 {
			return true
		}
	}
	return false
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
				if allowed, err := competitionScopeContains(database.DB.WithContext(ctx), scope, compID); err == nil && allowed {
					return true
				}
			}
		}
	}

	switch fileType {
	case "notice", "notice_attachment":
		return true
	case "declare_attachment":
		return canAccessDeclarationFile(ctx, scope, record)
	case "summary_attachment":
		return canAccessSummaryFile(ctx, scope, record)
	case "reg_attachment":
		if isTeamMemberForFile(ctx, uid, record.StoragePath) {
			return true
		}
		return !scope.IsCollegeAdmin() && isCompManagerForFile(ctx, uid, record.StoragePath)
	case "competition_work":
		if isTeamMemberForFile(ctx, uid, record.StoragePath) {
			return true
		}
		if !scope.IsCollegeAdmin() && isCompManagerForFile(ctx, uid, record.StoragePath) {
			return true
		}
		return isExpertForFile(ctx, uid, record.StoragePath)
	case "award_proof", "award_cert":
		return canAccessAwardProof(ctx, uid, scope, record)
	case "temp":
		// Legacy temp files may still be linked to summaries or declarations.
		return canAccessAwardProof(ctx, uid, scope, record) ||
			canAccessDeclarationFile(ctx, scope, record) ||
			canAccessSummaryFile(ctx, scope, record)
	}

	return false
}

func canAccessDeclarationFile(ctx context.Context, scope *UserAccessScope, record models.FileRecord) bool {
	if scope == nil || record.FileHash == "" {
		return false
	}

	var declarations []models.CompDeclaration
	if err := database.DB.WithContext(ctx).
		Where("attachment_path LIKE ?", "%"+record.FileHash+"%").
		Find(&declarations).Error; err != nil {
		return false
	}
	for _, declaration := range declarations {
		if strings.Contains(declaration.AttachmentPath, record.FileHash) && canAccessDeclaration(scope, declaration) {
			return true
		}
	}
	return false
}

func canAccessSummaryFile(ctx context.Context, scope *UserAccessScope, record models.FileRecord) bool {
	if scope == nil || record.FileHash == "" {
		return false
	}

	var summaries []models.Summary
	if err := database.DB.WithContext(ctx).
		Select("comp_id", "attachments").
		Where("attachments LIKE ?", "%"+record.FileHash+"%").
		Find(&summaries).Error; err != nil {
		return false
	}
	for _, summary := range summaries {
		if !strings.Contains(summary.Attachments, record.FileHash) {
			continue
		}
		var comp models.CompDirectory
		if err := database.DB.WithContext(ctx).
			Select("id", "manager_id", "college_id").
			First(&comp, summary.CompID).Error; err == nil && canAccessLoadedCompetition(database.DB.WithContext(ctx), scope, comp) {
			return true
		}
	}
	return false
}

func canAccessAwardProof(ctx context.Context, uid uint, scope *UserAccessScope, record models.FileRecord) bool {
	q := database.DB.WithContext(ctx).Model(&models.Award{}).
		Joins("JOIN comp_directories cd ON cd.id = awards.comp_id AND cd.delete_time IS NULL").
		Where("awards.proof_url IN ?", []string{buildDownloadURL(record.BizType, record.FileHash, record.OriginalName), record.StoragePath})
	if scope.IsCollegeAdmin() {
		q = applyCompetitionScope(q, scope, "cd")
	} else {
		advisorRole := scope != nil && (scope.RoleCode == "teacher" || scope.RoleCode == "competition_manager")
		q = q.Where(`cd.manager_id = ? OR EXISTS (SELECT 1 FROM award_members am WHERE am.award_id = awards.id AND am.student_id = ? AND am.delete_time IS NULL) OR EXISTS (SELECT 1 FROM registers r WHERE r.id = awards.reg_id AND r.leader_id = ? AND r.delete_time IS NULL) OR (? AND EXISTS (SELECT 1 FROM award_advisors aa JOIN users u ON u.username = aa.username AND u.delete_time IS NULL AND u.identity_type = 'staff' WHERE aa.award_id = awards.id AND aa.delete_time IS NULL AND u.id = ?))`, uid, uid, uid, advisorRole, uid)
	}
	var count int64
	return q.Count(&count).Error == nil && count > 0
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
