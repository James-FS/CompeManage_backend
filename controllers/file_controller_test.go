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

	"CompeManage_backend/database"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// setupDBMock 创建 mock 数据库并注入到 database.DB
func setupDBMock(t *testing.T) sqlmock.Sqlmock {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	assert.NoError(t, err)

	// 注入 mock DB
	database.DB = gormDB
	return mock
}

// mockFileNotFound 设置 mock：file_hash 查询返回 NotFound（文件未重复）
func mockFileNotFound(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT \\* FROM `file_records`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(gorm.ErrRecordNotFound)
}

// mockFileCreate 设置 mock：INSERT INTO file_records 成功
func mockFileCreate(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `file_records`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

// mockFileFound 设置 mock：file_hash 查询返回已存在（重复文件）
func mockFileFound(mock sqlmock.Sqlmock) {
	rows := sqlmock.NewRows([]string{"id", "file_hash", "biz_type", "storage_path", "original_name", "file_size", "file_ext", "uploader_id"}).
		AddRow(1, "abc123", "notice", "/static/notices/202601/test.pdf", "test.pdf", 100, ".pdf", 1)
	mock.ExpectQuery("SELECT \\* FROM `file_records`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(rows)
}

// 辅助函数：构建 multipart 请求体
func createMultipartRequest(fileContent []byte, fileName string, fileFieldName string, formFields map[string]string) (*bytes.Buffer, string) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	for key, value := range formFields {
		writer.WriteField(key, value)
	}
	part, _ := writer.CreateFormFile(fileFieldName, fileName)
	part.Write(fileContent)
	writer.Close()
	return body, writer.FormDataContentType()
}

func TestUploadFile_Success(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mockFileNotFound(mock)
	mockFileCreate(mock)

	tempDir := t.TempDir()
	originalNoticeDir := uploadDirs["notice"]
	uploadDirs["notice"] = filepath.Join(tempDir, "notices")
	defer func() {
		uploadDirs["notice"] = originalNoticeDir
	}()

	fileContent := []byte("这是一个PDF文件的内容")
	formFields := map[string]string{"type": "notice"}
	body, contentType := createMultipartRequest(fileContent, "test.pdf", "file", formFields)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadFile(c)

	assert.Equal(t, http.StatusOK, w.Code, "状态码应该是 200")
	assert.Contains(t, w.Body.String(), `"code":200`, "响应应该包含成功的 code")
	assert.Contains(t, w.Body.String(), `"url"`, "响应应该包含文件 URL")
}

func TestUploadFile_DuplicateFile(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mockFileFound(mock)

	tempDir := t.TempDir()
	originalNoticeDir := uploadDirs["notice"]
	uploadDirs["notice"] = filepath.Join(tempDir, "notices")
	defer func() {
		uploadDirs["notice"] = originalNoticeDir
	}()

	fileContent := []byte("重复文件内容")
	formFields := map[string]string{"type": "notice"}
	body, contentType := createMultipartRequest(fileContent, "test.pdf", "file", formFields)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadFile(c)

	assert.Equal(t, http.StatusOK, w.Code, "重复文件应返回 200")
	assert.Contains(t, w.Body.String(), `"url"`, "响应应包含已存在文件的 URL")
}

func TestUploadFile_FileTooLarge(t *testing.T) {
	setupDBMock(t)

	tempDir := t.TempDir()
	originalTempDir := uploadDirs["temp"]
	uploadDirs["temp"] = filepath.Join(tempDir, "temp")
	defer func() {
		uploadDirs["temp"] = originalTempDir
	}()

	largeContent := make([]byte, 100<<20) // 51MB
	formFields := map[string]string{"type": "temp"}
	body, contentType := createMultipartRequest(largeContent, "large.pdf", "file", formFields)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadFile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code, "文件过大应该返回 400")
	assert.Contains(t, w.Body.String(), "文件大小", "应该提示文件大小过大")
}

func TestUploadFile_UnsupportedExt(t *testing.T) {
	setupDBMock(t)

	tempDir := t.TempDir()
	originalTempDir := uploadDirs["temp"]
	uploadDirs["temp"] = filepath.Join(tempDir, "temp")
	defer func() {
		uploadDirs["temp"] = originalTempDir
	}()

	fileContent := []byte("非法后缀")
	formFields := map[string]string{"type": "temp"}
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
	setupDBMock(t)

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	writer.WriteField("type", "temp")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadFile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code, "缺少文件应该返回 400")
	assert.Contains(t, w.Body.String(), "未检测到文件", "应该提示缺少文件")
}

func TestUploadFile_DirectoryStructure(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mockFileNotFound(mock)
	mockFileCreate(mock)

	tempDir := t.TempDir()
	originalNoticeDir := uploadDirs["notice"]
	uploadDirs["notice"] = filepath.Join(tempDir, "notices")
	defer func() {
		uploadDirs["notice"] = originalNoticeDir
	}()

	fileContent := []byte("test content")
	formFields := map[string]string{"type": "notice"}
	body, contentType := createMultipartRequest(fileContent, "test.pdf", "file", formFields)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadFile(c)

	assert.Equal(t, http.StatusOK, w.Code)

	expectedSubDir := time.Now().Format("200601")
	expectedDir := filepath.Join(tempDir, "notices", expectedSubDir)
	_, err := os.Stat(expectedDir)
	assert.NoError(t, err, "年月子目录应该被创建: %s", expectedDir)

	files, err := os.ReadDir(expectedDir)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(files), "目录中应该有1个文件")

	savedFilePath := filepath.Join(expectedDir, files[0].Name())
	savedContent, err := os.ReadFile(savedFilePath)
	assert.NoError(t, err)
	assert.Equal(t, fileContent, savedContent, "保存的文件内容应该和上传的一致")
}

func TestUploadFile_MissingType(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mockFileNotFound(mock)
	mockFileCreate(mock)

	tempDir := t.TempDir()
	originalTempDir := uploadDirs["temp"]
	uploadDirs["temp"] = filepath.Join(tempDir, "temp")
	defer func() {
		uploadDirs["temp"] = originalTempDir
	}()

	fileContent := []byte("test without type")
	formFields := map[string]string{}
	body, contentType := createMultipartRequest(fileContent, "test.pdf", "file", formFields)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadFile(c)

	assert.Equal(t, http.StatusOK, w.Code, "缺少type字段应该默认存到temp并成功")

	expectedSubDir := time.Now().Format("200601")
	expectedDir := filepath.Join(tempDir, "temp", expectedSubDir)
	_, err := os.Stat(expectedDir)
	assert.NoError(t, err, "应该在temp目录下创建子目录")

	files, err := os.ReadDir(expectedDir)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(files), "temp目录中应该有1个文件")
}

func TestUploadFile_MkdirAllFails(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mockFileNotFound(mock)

	tempDir := t.TempDir()
	blockingFile := filepath.Join(tempDir, "notices")
	err := os.WriteFile(blockingFile, []byte("blocker"), 0644)
	assert.NoError(t, err, "准备阶段：创建阻塞文件")

	originalNoticeDir := uploadDirs["notice"]
	uploadDirs["notice"] = blockingFile
	defer func() {
		uploadDirs["notice"] = originalNoticeDir
	}()

	fileContent := []byte("test content")
	formFields := map[string]string{"type": "notice"}
	body, contentType := createMultipartRequest(fileContent, "test.pdf", "file", formFields)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadFile(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "目录创建失败应该返回 500")
	// 生产环境返回通用错误信息
	assert.Contains(t, w.Body.String(), "服务器内部错误", "应该提示服务器错误")
}

func TestUploadFile_PermissionDenied(t *testing.T) {
	// chmod 在 Windows 上无效，直接跳过
	t.Skip("Windows 不支持有效的权限测试，跳过")
}