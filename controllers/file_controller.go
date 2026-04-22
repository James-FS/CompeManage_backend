package controllers

import (
	"CompeManage_backend/utils"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxFileSize = 20 << 20

var uploadDirs = map[string]string{
	"notice":         "static/notices",
	"reg_attachment": "static/reg_attachments",
	"award_proof":    "static/award_proofs",
	"temp":           "static/temp",
}

var allowedExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".pdf": true, ".doc": true, ".docx": true,
	".xls": true, ".xlsx": true,
	".zip": true, ".rar": true,
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
		utils.BadRequest(c, "文件大小不能超过50MB")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedExts[ext] {
		utils.BadRequest(c, "不支持的文件格式(仅支持图片、PDF、Word、Excel、压缩包)")
		return
	}

	subDir := time.Now().Format("200601")
	finalDir := filepath.Join(savePathRoot, subDir)

	if err := os.MkdirAll(finalDir, os.ModePerm); err != nil {
		utils.InternalServerError(c, "服务器存储初始化失败", err)
		return
	}

	originalName := filepath.Base(file.Filename)
	originalName = strings.ReplaceAll(originalName, " ", "_")
	newFileName := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), originalName)
	dst := filepath.Join(finalDir, newFileName)

	if err := c.SaveUploadedFile(file, dst); err != nil {
		utils.InternalServerError(c, "文件保存失败", err)
		return
	}

	urlPath := "/" + filepath.ToSlash(dst)

	utils.SuccessWithMessage(c, "上传成功", gin.H{
		"url":       urlPath,
		"name":      file.Filename,
		"size":      file.Size,
		"mime_type": ext,
	})
}
