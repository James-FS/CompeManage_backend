package controllers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// 辅助函数：构建 multipart 请求体
// 用来模拟前端上传文件的 form-data
func createMultipartRequest(fileContent []byte, fileName string, fileFieldName string, formFields map[string]string) (*bytes.Buffer, string) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	// 1. 添加所有表单字段（比如 type=notice）
	for key, value := range formFields {
		writer.WriteField(key, value)
	}

	// 2. 添加文件
	part, _ := writer.CreateFormFile(fileFieldName, fileName)
	part.Write(fileContent)

	writer.Close()

	// 返回请求体 和 Content-Type
	return body, writer.FormDataContentType()
}

// 测试场景：正常上传一个 PDF 文件
func TestUploadFile_Success(t *testing.T) {
	// ========== 准备阶段 ==========

	// 1.1 创建临时目录（用来保存上传的文件）
	tempDir := t.TempDir()

	// 1.2 修改 uploadDirs 指向临时目录（这样不会污染真实系统）
	//     保存原值，待会儿恢复
	originalNoticeDir := uploadDirs["notice"]
	uploadDirs["notice"] = filepath.Join(tempDir, "notices")
	defer func() {
		uploadDirs["notice"] = originalNoticeDir // 测试完恢复
	}()

	// 1.3 构建上传请求
	fileContent := []byte("这是一个PDF文件的内容")
	formFields := map[string]string{
		"type": "notice", // 业务类型
	}
	body, contentType := createMultipartRequest(
		fileContent,
		"test.pdf", // 文件名
		"file",     // 表单字段名
		formFields,
	)

	// 1.4 创建 HTTP 请求
	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)

	// 1.5 创建响应记录器（用来捕获函数的返回）
	w := httptest.NewRecorder()

	// 1.6 创建 Gin 上下文
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// ========== 执行阶段 ==========
	UploadFile(c)

	// ========== 验证阶段 ==========

	// 2.1 验证 HTTP 状态码
	assert.Equal(t, http.StatusOK, w.Code, "状态码应该是 200")

	// 2.2 验证响应体中包含 "code": 200
	assert.Contains(t, w.Body.String(), `"code":200`, "响应应该包含成功的 code")
	assert.Contains(t, w.Body.String(), `"url"`, "响应应该包含文件 URL")

	// 2.3 验证文件确实被保存到了临时目录
	// 列出保存目录中的文件
	entries, err := os.ReadDir(filepath.Join(tempDir, "notices"))
	if err == nil {
		// 应该存在以年月为子目录的文件
		assert.Greater(t, len(entries), 0, "应该创建了年月子目录")
	}
}

// 测试场景：上传一个超过 50MB 的文件
func TestUploadFile_FileTooLarge(t *testing.T) {
	// ========== 准备阶段 ==========

	tempDir := t.TempDir()
	originalTempDir := uploadDirs["temp"]
	uploadDirs["temp"] = filepath.Join(tempDir, "temp")
	defer func() {
		uploadDirs["temp"] = originalTempDir
	}()

	// 创建超过大小限制的文件
	largeContent := make([]byte, 100<<20) // 51MB

	formFields := map[string]string{
		"type": "temp",
	}
	body, contentType := createMultipartRequest(largeContent, "large.pdf", "file", formFields)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// ========== 执行阶段 ==========
	UploadFile(c)

	// ========== 验证阶段 ==========

	// 应该返回 400 错误
	assert.Equal(t, http.StatusBadRequest, w.Code, "文件过大应该返回 400")

	// 响应中应该包含文件大小的错误信息
	assert.Contains(t, w.Body.String(), "文件大小", "应该提示文件大小过大")
}

func TestUploadFile_File_UnsupportedExt(t *testing.T) {
	tempDir := t.TempDir()
	originalTempDir := uploadDirs["temp"]
	uploadDirs["temp"] = filepath.Join(tempDir, "temp")
	defer func() {
		uploadDirs["temp"] = originalTempDir
	}()
	fileContent := []byte("非法后缀")
	formFields := map[string]string{
		"type": "temp",
	}
	body, contentType := createMultipartRequest(fileContent, "virus.exe", "file", formFields)
	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	UploadFile(c)
	assert.Equal(t, http.StatusBadRequest, w.Code, "不支持的后缀应该返回 400")
	assert.Contains(t, w.Body.String(), "不支持的文件格式", "应该提示文件格式不支持")
}

func TestUploadFile_NoFile(t *testing.T) {
	// ========== 准备阶段 ==========

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	// 只添加 type 字段，不添加文件
	writer.WriteField("type", "temp")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// ========== 执行阶段 ==========
	UploadFile(c)

	// ========== 验证阶段 ==========

	assert.Equal(t, http.StatusBadRequest, w.Code, "缺少文件应该返回 400")
	assert.Contains(t, w.Body.String(), "未检测到文件", "应该提示缺少文件")
}

func TestUploadFile_DirectoryStructure(t *testing.T) {
	// ========== 准备阶段 ==========

	// 使用临时目录
	tempDir := t.TempDir()
	originalNoticeDir := uploadDirs["notice"]
	uploadDirs["notice"] = filepath.Join(tempDir, "notices")
	defer func() {
		uploadDirs["notice"] = originalNoticeDir
	}()

	// 构建请求
	fileContent := []byte("test content")
	formFields := map[string]string{
		"type": "notice",
	}
	body, contentType := createMultipartRequest(fileContent, "test.pdf", "file", formFields)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// ========== 执行阶段 ==========
	UploadFile(c)

	// ========== 验证阶段 ==========

	//  验证上传成功
	assert.Equal(t, http.StatusOK, w.Code)

	//  验证年月子目录是否创建
	expectedSubDir := time.Now().Format("200601") // 当前年月，比如 "202602"
	expectedDir := filepath.Join(tempDir, "notices", expectedSubDir)

	// 检查目录是否存在
	_, err := os.Stat(expectedDir)
	assert.NoError(t, err, "年月子目录应该被创建: %s", expectedDir)

	//  验证文件是否在该目录中
	files, err := os.ReadDir(expectedDir)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(files), "目录中应该有1个文件")

	//  验证文件名格式（应该是：时间戳_test.pdf）
	fileName := files[0].Name()
	assert.Contains(t, fileName, "_test.pdf", "文件名应该包含原始文件名")
	assert.Regexp(t, `^\d+_test\.pdf$`, fileName, "文件名格式应该是：时间戳_原名")

	// 验证文件内容是否正确
	savedFilePath := filepath.Join(expectedDir, fileName)
	savedContent, err := os.ReadFile(savedFilePath)
	assert.NoError(t, err)
	assert.Equal(t, fileContent, savedContent, "保存的文件内容应该和上传的一致")
}
func TestUploadFile_MissingType(t *testing.T) {
	// ========== 准备阶段 ==========
	tempDir := t.TempDir()
	originalTempDir := uploadDirs["temp"]
	uploadDirs["temp"] = filepath.Join(tempDir, "temp")
	defer func() {
		uploadDirs["temp"] = originalTempDir
	}()

	// 构建请求，不传 type 字段
	fileContent := []byte("test without type")
	formFields := map[string]string{
		// 注意：这里故意不添加 type 字段
	}
	body, contentType := createMultipartRequest(fileContent, "test.pdf", "file", formFields)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// ========== 执行阶段 ==========
	UploadFile(c)

	// ========== 验证阶段 ==========

	// 1. 应该上传成功
	assert.Equal(t, http.StatusOK, w.Code, "缺少type字段应该默认存到temp并成功")

	// 2. 验证文件确实存到了 temp 目录
	expectedSubDir := time.Now().Format("200601")
	expectedDir := filepath.Join(tempDir, "temp", expectedSubDir)

	_, err := os.Stat(expectedDir)
	assert.NoError(t, err, "应该在temp目录下创建子目录")

	// 3. 验证文件存在
	files, err := os.ReadDir(expectedDir)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(files), "temp目录中应该有1个文件")
}

func TestUploadFile_MkdirAllFails(t *testing.T) {
	// ========== 准备阶段 ==========
	tempDir := t.TempDir()

	// 创建一个普通文件，占用 "notices" 这个名字
	blockingFile := filepath.Join(tempDir, "notices")
	err := os.WriteFile(blockingFile, []byte("blocker"), 0644)
	assert.NoError(t, err, "准备阶段：创建阻塞文件")

	originalNoticeDir := uploadDirs["notice"]
	uploadDirs["notice"] = blockingFile //
	defer func() {
		uploadDirs["notice"] = originalNoticeDir
	}()

	// 构建上传请求
	fileContent := []byte("test content")
	formFields := map[string]string{
		"type": "notice",
	}
	body, contentType := createMultipartRequest(fileContent, "test.pdf", "file", formFields)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// ========== 执行阶段 ==========
	UploadFile(c)

	// 1. 应该返回 500 错误
	assert.Equal(t, http.StatusInternalServerError, w.Code, "目录创建失败应该返回 500")

	// 2. 错误信息应该包含 "服务器存储初始化失败"
	assert.Contains(t, w.Body.String(), "服务器存储初始化失败", "应该提示存储初始化失败")
}

// 测试场景：权限不足，无法创建目录
func TestUploadFile_PermissionDenied(t *testing.T) {
	// 注意：这个测试在 Windows 上可能不生效，主要适用于 Linux/Mac
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Windows 权限测试不稳定，跳过")
	}

	// ========== 准备阶段 ==========
	tempDir := t.TempDir()

	// 创建一个只读的父目录
	readOnlyParent := filepath.Join(tempDir, "readonly_parent")
	err := os.MkdirAll(readOnlyParent, 0755)
	assert.NoError(t, err)

	// 改为只读权限（无法在其中创建子目录）
	err = os.Chmod(readOnlyParent, 0444)
	assert.NoError(t, err)

	originalNoticeDir := uploadDirs["notice"]
	uploadDirs["notice"] = filepath.Join(readOnlyParent, "notices") // 试图在只读目录中创建
	defer func() {
		uploadDirs["notice"] = originalNoticeDir
		os.Chmod(readOnlyParent, 0755) // 恢复权限，方便清理
	}()

	// 构建请求
	fileContent := []byte("test")
	formFields := map[string]string{
		"type": "notice",
	}
	body, contentType := createMultipartRequest(fileContent, "test.pdf", "file", formFields)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// ========== 执行阶段 ==========
	UploadFile(c)

	// ========== 验证阶段 ==========
	assert.Equal(t, http.StatusInternalServerError, w.Code, "权限不足应该返回 500")
	assert.Contains(t, w.Body.String(), "服务器存储初始化失败", "应该提示存储失败")
}
