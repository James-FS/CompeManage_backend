package controllers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxFileSize = 20 << 20

// UploadConfig 定义允许的业务类型和对应的存储路径
// 前端上传时，form-data 中必须带上 type 参数，例如 type=reg_attachment
var uploadDirs = map[string]string{
	// 1. 赛事通知 (管理员发布比赛时上传的附件)
	"notice": "static/notices",
	// 2. 报名附件 (学生报名时上传的计划书等)
	"reg_attachment": "static/reg_attachments",
	// 3. 获奖证实 (学生上传的获奖证书扫描件)
	"award_proof": "static/award_proofs",
	// 4. 临时文件
	"temp": "static/temp",
}

// AllowedExts 定义允许的后缀名 (白名单)
var allowedExts = map[string]bool{
	// 图片
	".jpg": true, ".jpeg": true, ".png": true,
	// 文档 (Word/PDF)
	".pdf": true, ".doc": true, ".docx": true,
	// 表格 (Excel) - 新增需求
	".xls": true, ".xlsx": true,
	// 压缩包
	".zip": true, ".rar": true,
}

// UploadFile 通用文件上传接口
// 参数: file (文件流), type (业务类型)
func UploadFile(c *gin.Context) {
	// 1. 获取业务类型 (决定存哪里)
	bizType := c.PostForm("type")
	savePathRoot, ok := uploadDirs[bizType]
	if !ok {
		// 如果没传type或者type不对，默认存到 temp
		savePathRoot = uploadDirs["temp"]
	}

	// 2. 获取文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "未检测到文件"})
		return
	}

	// 3. 校验文件大小
	if file.Size > maxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "文件大小不能超过50MB"})
		return
	}

	// 4. 校验文件后缀 (安全性)
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedExts[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "不支持的文件格式(仅支持图片、PDF、Word、Excel、压缩包)"})
		return
	}

	// 5. 生成最终保存目录：根目录/年月日 (防止单目录文件过多)
	// 结果类似: ./uploads/reg_attachments/20260123
	subDir := time.Now().Format("200601")
	finalDir := filepath.Join(savePathRoot, subDir)

	// 6. 创建目录
	if err := os.MkdirAll(finalDir, os.ModePerm); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "服务器存储初始化失败"})
		return
	}

	// 只取文件名
	originalName := filepath.Base(file.Filename)
	// 兼容处理：把空格替换为下划线 (防止 URL 访问时出现编码问题)
	originalName = strings.ReplaceAll(originalName, " ", "_")
	// 唯一化处理：前缀加毫秒时间戳，防止同名覆盖
	// 格式：1708923456123_我的实验报告.docx
	newFileName := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), originalName)
	dst := filepath.Join(finalDir, newFileName)

	// 8. 保存文件
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "文件保存失败"})
		return
	}

	// 9. 返回相对 URL
	// 返回格式: /uploads/reg_attachments/20260123/uuid.xlsx
	urlPath := "/" + filepath.ToSlash(dst)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "上传成功",
		"data": gin.H{
			"url":       urlPath,
			"name":      file.Filename, // 原始文件名
			"size":      file.Size,
			"mime_type": ext,
		},
	})
}
