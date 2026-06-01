package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxFileSize = 30 << 20

var uploadDirs = map[string]string{
	"notice":         "static/notices",
	"reg_attachment": "static/reg_attachments",
	"award_proof":    "static/award_proofs",
	"temp":           "static/temp",
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
				"url":       existingRecord.StoragePath,
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
	utils.Success(c, gin.H{
		"url":       storagePath,
		"name":      originalName,
		"size":      file.Size,
		"mime_type": ext,
	})
}
