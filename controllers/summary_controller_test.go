package controllers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupSummaryDBMock(t *testing.T) sqlmock.Sqlmock {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	assert.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	assert.NoError(t, err)
	database.DB = gormDB
	return mock
}

// ===================== GetSummaryList =====================

func TestGetSummaryList_Unauthorized(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/summary/list")
	GetSummaryList(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestGetSummaryList_InvalidSummaryStatus(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/summary/list?summary_status=2")
	c.Set("user_id", uint(1))
	GetSummaryList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "summary_status 只能是 0 或 1")
}

func TestGetSummaryList_InvalidEndTime(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/summary/list?end_time=not-a-date")
	c.Set("user_id", uint(1))
	GetSummaryList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "end_time 格式错误")
}

func TestGetSummaryList_InvalidYear(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/summary/list?year=abc")
	c.Set("user_id", uint(1))
	GetSummaryList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "year 只能是数字")
}

func TestGetSummaryList_CountError(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	// checkUserIsAdmin → count=1 (admin)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Count total → error
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/summary/list?page=1&page_size=10")
	c.Set("user_id", uint(1))
	GetSummaryList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// ===================== GetSummaryDetail =====================

func TestGetSummaryDetail_InvalidID(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/summary/detail/abc")
	c.Params = []gin.Param{{Key: "id", Value: "abc"}}
	c.Set("user_id", uint(1))
	GetSummaryDetail(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛事ID格式错误")
}

func TestGetSummaryDetail_Unauthorized(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/summary/detail/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	GetSummaryDetail(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestGetSummaryDetail_NotFound(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	// First() → ErrRecordNotFound, GORM 不执行 Preload 查询
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/summary/detail/999")
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	c.Set("user_id", uint(1))
	GetSummaryDetail(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在")
}

func TestGetSummaryDetail_Forbidden(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	// college_id=0 时 GORM 跳过 colleges 查询，顺序：comp_directories → comp_details → users
	mock.ExpectQuery("SELECT \\* FROM `comp_directories` .+ `comp_directories`.`delete_time`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "comp_name"}).
			AddRow(1, 10, "测试赛事"))
	mock.ExpectQuery("SELECT \\* FROM `comp_details` .+ `comp_details`.`comp_id`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).
			AddRow(1, 1))
	mock.ExpectQuery("SELECT \\* FROM `users` .+ `users`.`id`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow(10))

	// checkUserIsAdmin → count=0 (非管理员)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	_, w, c := buildGET("/api/summary/detail/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(5)) // user_id=5, not manager
	GetSummaryDetail(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无权查看")
}

func TestGetSummaryDetail_Success_NoSummary(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	// college_id=1 时 GORM 查询 colleges，顺序：comp_directories → colleges → comp_details → users
	mock.ExpectQuery("SELECT \\* FROM `comp_directories` .+ `comp_directories`.`delete_time`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id", "comp_name", "organizer", "undertaker"}).
			AddRow(1, 1, 1, "测试赛事", "主办方", "承办方"))
	mock.ExpectQuery("SELECT \\* FROM `colleges` .+ `colleges`.`id`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
			AddRow(1, "计算机学院"))
	mock.ExpectQuery("SELECT \\* FROM `comp_details` .+ `comp_details`.`comp_id`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "comp_start_time", "comp_end_time"}).
			AddRow(1, 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local), time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)))
	mock.ExpectQuery("SELECT \\* FROM `users` .+ `users`.`id`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).
			AddRow(1, "管理员"))

	// checkUserIsAdmin → count=1 (admin)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Where("comp_id = ?").First(&summary) → not found
	mock.ExpectQuery("SELECT \\* FROM `summaries` .+ comp_id").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	// 参赛人数统计
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// 获奖统计
	mock.ExpectQuery("SELECT .+ FROM `awards` .+ `awards`.`comp_id`").
		WillReturnRows(sqlmock.NewRows([]string{"level", "count", "level_rank"}))

	_, w, c := buildGET("/api/summary/detail/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	GetSummaryDetail(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "测试赛事")
	assert.Contains(t, w.Body.String(), "管理员")
}

// ===================== SaveSummary =====================

func TestSaveSummary_InvalidID(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildAuthPOSTJSON(map[string]interface{}{})
	c.Params = []gin.Param{{Key: "id", Value: "abc"}}
	c.Set("user_id", uint(1))
	SaveSummary(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛事ID格式错误")
}

func TestSaveSummary_BadRequest(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	// ShouldBindJSON fails (invalid body type)
	_, w, c := buildAuthPOSTJSON("not-json")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	SaveSummary(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestSaveSummary_InvalidStatus(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	body := SaveSummaryReq{SummaryContent: "test", Status: 2}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	SaveSummary(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "status 只能是 0 或 1")
}

func TestSaveSummary_Unauthorized(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	body := SaveSummaryReq{SummaryContent: "test", Status: 0}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	SaveSummary(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestSaveSummary_CompNotFound(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&comp, compID) → not found
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := SaveSummaryReq{SummaryContent: "test", Status: 0}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	c.Set("user_id", uint(1))
	SaveSummary(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在")
}

func TestSaveSummary_CompDBError(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnError(fmt.Errorf("db error"))

	body := SaveSummaryReq{SummaryContent: "test", Status: 0}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	SaveSummary(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestSaveSummary_Forbidden_NonManager(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&comp) → manager_id=10, status=2
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "status"}).
			AddRow(1, 10, 2))

	// checkUserIsAdmin → count=0
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	body := SaveSummaryReq{SummaryContent: "test", Status: 0}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(5)) // not manager
	SaveSummary(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无权操作")
}

func TestSaveSummary_CompNotEnded(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&comp) → status=1 (未结束)
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "status"}).
			AddRow(1, 1, 1))

	// checkUserIsAdmin → count=1
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	body := SaveSummaryReq{SummaryContent: "test", Status: 0}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	SaveSummary(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛事未结束")
}

func TestSaveSummary_AlreadyArchived(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&comp) → status=2
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "status"}).
			AddRow(1, 1, 2))

	// checkUserIsAdmin → count=1
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Where("comp_id = ?").First(&summary) → found, status=1
	mock.ExpectQuery("SELECT .* FROM `summaries`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).
			AddRow(1, 1, 1))

	body := SaveSummaryReq{SummaryContent: "test", Status: 0}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	SaveSummary(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "已归档")
}

func TestSaveSummary_SummaryDBError(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&comp) → status=2
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "status"}).
			AddRow(1, 1, 2))

	// checkUserIsAdmin → count=1
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Where("comp_id = ?").First(&summary) → db error
	mock.ExpectQuery("SELECT .* FROM `summaries`").
		WillReturnError(fmt.Errorf("db error"))

	body := SaveSummaryReq{SummaryContent: "test", Status: 0}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	SaveSummary(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestSaveSummary_CreateDraft(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&comp) → status=2
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "status"}).
			AddRow(1, 1, 2))

	// checkUserIsAdmin → count=1
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Where("comp_id = ?").First(&summary) → not found
	mock.ExpectQuery("SELECT .* FROM `summaries`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	// Create(&summary)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `summaries`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := SaveSummaryReq{SummaryContent: "总结内容", Status: 0}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	SaveSummary(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "保存成功")
}

func TestSaveSummary_CreateArchived(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "status"}).
			AddRow(1, 1, 2))

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery("SELECT .* FROM `summaries`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `summaries`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := SaveSummaryReq{SummaryContent: "总结内容", Status: 1}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	SaveSummary(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "保存成功")
}

func TestSaveSummary_UpdateExisting(t *testing.T) {
	mock := setupSummaryDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "status"}).
			AddRow(1, 1, 2))

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Where("comp_id = ?").First(&summary) → found, status=0 (草稿)
	mock.ExpectQuery("SELECT .* FROM `summaries`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).
			AddRow(1, 1, 0))

	// Save(&summary) → UPDATE
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `summaries`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := SaveSummaryReq{SummaryContent: "更新内容", Status: 0}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	SaveSummary(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "保存成功")
}
