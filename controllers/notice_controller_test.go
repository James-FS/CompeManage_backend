package controllers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupNoticeDBMock(t *testing.T) sqlmock.Sqlmock {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	assert.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	assert.NoError(t, err)
	database.DB = gormDB

	// 启动 miniredis，避免缓存逻辑 panic（cache miss 后走 DB）
	mr := miniredis.RunT(t)
	previousRedisClient := middleware.RedisClient
	middleware.RedisClient = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = middleware.RedisClient.Close()
		middleware.RedisClient = previousRedisClient
	})

	return mock
}

func buildNoticeGET(urlStr string) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
	req := httptest.NewRequest("GET", urlStr, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return req, w, c
}

func buildNoticePOSTForm(body url.Values) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return req, w, c
}

// ===================== GetNoticeList =====================

func TestGetNoticeList_PageAndPageSize(t *testing.T) {
	// 每个子测试用独立的 mock，避免共享状态导致后续子测试失败
	tests := []struct {
		name        string
		queryString string
	}{
		{name: "正常分页", queryString: "?page=2&page_size=20"},
		{name: "page为0时修正为1", queryString: "?page=0&page_size=10"},
		{name: "page为负时修正为1", queryString: "?page=-1&page_size=10"},
		{name: "pageSize超过50时修正为10", queryString: "?page=1&page_size=100"},
		{name: "pageSize为0时修正为10", queryString: "?page=1&page_size=0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := setupNoticeDBMock(t)
			defer mock.ExpectationsWereMet()

			mock.ExpectQuery("SELECT count\\(\\*\\) FROM `notices`").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery("SELECT \\* FROM `notices`").
				WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "test"))

			req, w, c := buildNoticeGET("/api/notice/list" + tt.queryString)
			c.Request = req
			GetNoticeList(c)
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestGetNoticeList_IsLatest(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `notices`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "test"))

	req, w, c := buildNoticeGET("/api/notice/list?is_latest=false")
	c.Request = req
	GetNoticeList(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetNoticeList_StatusFilter(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	// status=0 或 status=1 均合法
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `notices`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "test"))

	req, w, c := buildNoticeGET("/api/notice/list?status=1")
	c.Request = req
	GetNoticeList(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetNoticeList_StatusInvalid(t *testing.T) {
	setupNoticeDBMock(t)
	// status 既不是 0 也不是 1 → 400
	req, w, c := buildNoticeGET("/api/notice/list?status=2")
	c.Request = req
	GetNoticeList(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "status只能是0")
}

func TestGetNoticeList_CompIDFormatError(t *testing.T) {
	setupNoticeDBMock(t)
	// compID 不是数字 → 400
	req, w, c := buildNoticeGET("/api/notice/list?compID=abc")
	c.Request = req
	GetNoticeList(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "compID格式错误")
}

func TestGetNoticeList_TimeRange(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	// startTime + endTime 筛选，合法时间格式
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `notices`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "test"))

	req, w, c := buildNoticeGET("/api/notice/list?start_time=2026.1.1&end_time=2026.2.28")
	c.Request = req
	GetNoticeList(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetNoticeList_CountError(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `notices`").
		WillReturnError(gorm.ErrInvalidTransaction)

	req, w, c := buildNoticeGET("/api/notice/list")
	c.Request = req
	GetNoticeList(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== GetNoticeDetail =====================

func TestGetNoticeDetail_IDFormatError(t *testing.T) {
	setupNoticeDBMock(t)
	// 非数字ID → 400
	req := httptest.NewRequest("GET", "/api/notice/abc", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	GetNoticeDetail(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "通知ID格式错误")
}

func TestGetNoticeDetail_NotFound(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnError(gorm.ErrRecordNotFound)

	req := httptest.NewRequest("GET", "/api/notice/999", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	GetNoticeDetail(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "该通知不存在")
}

func TestGetNoticeDetail_DBError(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnError(gorm.ErrInvalidTransaction)

	req := httptest.NewRequest("GET", "/api/notice/1", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	GetNoticeDetail(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== CreateNotice =====================

func TestCreateNotice_TitleEmpty(t *testing.T) {
	setupNoticeDBMock(t)
	body := url.Values{"content": {"测试内容"}}
	_, w, c := buildNoticePOSTForm(body)
	CreateNotice(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "通知标题不能为空")
}

func TestCreateNotice_CompIDFormatError(t *testing.T) {
	setupNoticeDBMock(t)
	body := url.Values{"title": {"测试标题"}, "compID": {"abc"}}
	_, w, c := buildNoticePOSTForm(body)
	CreateNotice(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "compID格式错误")
}

func TestCreateNotice_Success(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `notices`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := url.Values{"title": {"测试标题"}, "content": {"测试内容"}, "attachment": {"http://example.com/file.pdf"}}
	_, w, c := buildNoticePOSTForm(body)
	CreateNotice(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCreateNotice_DBError(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `notices`").
		WillReturnError(gorm.ErrInvalidTransaction)
	mock.ExpectRollback()

	body := url.Values{"title": {"测试标题"}}
	_, w, c := buildNoticePOSTForm(body)
	CreateNotice(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== CreateCompNotice =====================

func TestCreateCompNotice_TitleEmpty(t *testing.T) {
	setupNoticeDBMock(t)
	body := url.Values{"compID": {"1"}}
	_, w, c := buildNoticePOSTForm(body)
	CreateCompNotice(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "通知标题不能为空")
}

func TestCreateCompNotice_CompIDEmpty(t *testing.T) {
	setupNoticeDBMock(t)
	body := url.Values{"title": {"测试标题"}}
	_, w, c := buildNoticePOSTForm(body)
	CreateCompNotice(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须关联具体赛事")
}

func TestCreateCompNotice_CompIDFormatError(t *testing.T) {
	setupNoticeDBMock(t)
	body := url.Values{"title": {"测试标题"}, "compID": {"xyz"}}
	_, w, c := buildNoticePOSTForm(body)
	CreateCompNotice(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "compID格式错误")
}

func TestCreateCompNotice_Success(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `notices`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := url.Values{"title": {"赛事通知"}, "content": {"内容"}, "compID": {"5"}}
	_, w, c := buildNoticePOSTForm(body)
	CreateCompNotice(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== PublishNotice =====================

func TestPublishNotice_IDFormatError(t *testing.T) {
	setupNoticeDBMock(t)
	req := httptest.NewRequest("POST", "/api/notice/publish/abc", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	PublishNotice(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "通知ID格式错误")
}

func TestPublishNotice_NotFound(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnError(gorm.ErrRecordNotFound)

	req := httptest.NewRequest("POST", "/api/notice/publish/999", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	PublishNotice(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPublishNotice_AlreadyPublished(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	rows := sqlmock.NewRows([]string{"id", "status"}).AddRow(1, 1) // status=1 已发布
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(rows)

	req := httptest.NewRequest("POST", "/api/notice/publish/1", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	PublishNotice(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "已发布")
}

func TestPublishNotice_Success(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	rows := sqlmock.NewRows([]string{"id", "status"}).AddRow(1, 0) // status=0 未发布
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(rows)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `notices`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	req := httptest.NewRequest("POST", "/api/notice/publish/1", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	PublishNotice(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPublishNotice_UpdateError(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	rows := sqlmock.NewRows([]string{"id", "status"}).AddRow(1, 0)
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(rows)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `notices`").
		WillReturnError(gorm.ErrInvalidTransaction)
	mock.ExpectRollback()

	req := httptest.NewRequest("POST", "/api/notice/publish/1", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	PublishNotice(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== UpdateNotice =====================

func TestUpdateNotice_IDFormatError(t *testing.T) {
	setupNoticeDBMock(t)
	body := url.Values{"title": {"新标题"}}
	_, w, c := buildNoticePOSTForm(body)
	c.Params = []gin.Param{{Key: "id", Value: "abc"}}
	UpdateNotice(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateNotice_NotFound(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnError(gorm.ErrRecordNotFound)

	body := url.Values{"title": {"新标题"}}
	_, w, c := buildNoticePOSTForm(body)
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	UpdateNotice(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateNotice_NoFields(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	rows := sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "旧标题")
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(rows)

	body := url.Values{}
	_, w, c := buildNoticePOSTForm(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateNotice(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "没有要修改的字段")
}

func TestUpdateNotice_CompIDFormatError(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	rows := sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "标题")
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(rows)

	body := url.Values{"title": {"新标题"}, "compID": {"bad"}}
	_, w, c := buildNoticePOSTForm(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateNotice(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "compID格式错误")
}

func TestUpdateNotice_Success(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	rows := sqlmock.NewRows([]string{"id", "title", "content", "status"}).AddRow(1, "旧标题", "旧内容", 0)
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(rows)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `notices`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := url.Values{"title": {"新标题"}}
	_, w, c := buildNoticePOSTForm(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateNotice(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateNotice_UpdateError(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	rows := sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "标题")
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(rows)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `notices`").
		WillReturnError(gorm.ErrInvalidTransaction)
	mock.ExpectRollback()

	body := url.Values{"title": {"新标题"}}
	_, w, c := buildNoticePOSTForm(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateNotice(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== DeleteNotice =====================

func TestDeleteNotice_IDFormatError(t *testing.T) {
	setupNoticeDBMock(t)
	req := httptest.NewRequest("DELETE", "/api/notice/abc", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	DeleteNotice(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "通知ID格式错误")
}

func TestDeleteNotice_NotFound(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnError(gorm.ErrRecordNotFound)

	req := httptest.NewRequest("DELETE", "/api/notice/999", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	// 手动设置路由参数，因为 test context 不会自动解析 URL
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	DeleteNotice(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "该通知不存在")
}

func TestDeleteNotice_Success(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	rows := sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "标题")
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(rows)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `notices`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	req := httptest.NewRequest("DELETE", "/api/notice/1", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	DeleteNotice(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteNotice_DeleteError(t *testing.T) {
	mock := setupNoticeDBMock(t)
	defer mock.ExpectationsWereMet()

	rows := sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "标题")
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(rows)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `notices`").
		WillReturnError(gorm.ErrInvalidTransaction)
	mock.ExpectRollback()

	req := httptest.NewRequest("DELETE", "/api/notice/1", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	DeleteNotice(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
