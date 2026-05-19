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

	// 计算文件 MD5
	fileHash, err := calcFileMD5(file)
	if err != nil {
		utils.InternalServerError(c, "文件校验失败", err)
		return
	}

	// 查重（同一哈希 + 同一业务类型）
	var existingRecord models.FileRecord
	result := database.DB.Where("file_hash = ? AND biz_type = ?", fileHash, bizType).First(&existingRecord)
	if result.Error == nil {
		// 已存在，直接返回（去重成功），前端无感知
		utils.Success(c, gin.H{
			"url":       existingRecord.StoragePath,
			"name":      existingRecord.OriginalName,
			"size":      existingRecord.FileSize,
			"mime_type": existingRecord.FileExt,
		})
		return
	}

	// 不存在，保存文件
	subDir := time.Now().Format("200601")
	finalDir := filepath.Join(savePathRoot, subDir)

	if err := os.MkdirAll(finalDir, os.ModePerm); err != nil {
		utils.InternalServerError(c, "服务器存储初始化失败", err)
		return
	}

	originalName := sanitizeFilename(file.Filename)
	newFileName := fmt.Sprintf("%s_%s", fileHash, originalName)
	dst := filepath.Join(finalDir, newFileName)

	if err := c.SaveUploadedFile(file, dst); err != nil {
		utils.InternalServerError(c, "文件保存失败", err)
		return
	}

	// 获取上传用户ID
	var uploaderID uint
	if id, exists := c.Get("user_id"); exists {
		uploaderID = id.(uint)
	}

	// 写入 FileRecord
	record := models.FileRecord{
		FileHash:     fileHash,
		OriginalName: originalName,
		StoragePath:  "/" + filepath.ToSlash(dst),
		FileSize:     file.Size,
		FileExt:      ext,
		BizType:      bizType,
		UploaderID:   uploaderID,
	}
	if err := database.DB.Create(&record).Error; err != nil {
		// 插入失败，可能是并发冲突，删掉已保存的物理文件
		os.Remove(dst)
		utils.InternalServerError(c, "文件记录保存失败", err)
		return
	}

	// 返回（与原有响应结构完全一致）
	utils.Success(c, gin.H{
		"url":       record.StoragePath,
		"name":      record.OriginalName,
		"size":      record.FileSize,
		"mime_type": record.FileExt,
	})
}
