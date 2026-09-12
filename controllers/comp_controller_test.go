package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupCompDBMock(t *testing.T) sqlmock.Sqlmock {
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

func buildCompGET(urlStr string) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
	req := httptest.NewRequest("GET", urlStr, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return req, w, c
}

func buildCompPOSTJSON(body interface{}) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
	jsonBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/", bytes.NewBuffer(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return req, w, c
}

// ===================== levelPrefix =====================

func TestLevelPrefix(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected string
		ok       bool
	}{
		{name: "国家级", level: "国家级", expected: "G", ok: true},
		{name: "省级", level: "省级", expected: "S", ok: true},
		{name: "校级", level: "校级", expected: "X", ok: true},
		{name: "国际级", level: "国际级", expected: "I", ok: true},
		{name: "带空格", level: " 校级 ", expected: "X", ok: true},
		{name: "不支持的级别", level: "市级", expected: "", ok: false},
		{name: "空字符串", level: "", expected: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, ok := levelPrefix(tt.level)
			assert.Equal(t, tt.expected, prefix)
			assert.Equal(t, tt.ok, ok)
		})
	}
}

// ===================== resolveCompetitionYear =====================

func TestResolveCompetitionYear(t *testing.T) {
	currentYear := time.Now().Year()

	tests := []struct {
		name     string
		yearText string
		expected int
	}{
		{name: "正常年份", yearText: "2025", expected: 2025},
		{name: "带空格", yearText: " 2024 ", expected: 2024},
		{name: "空字符串", yearText: "", expected: currentYear},
		{name: "非数字", yearText: "abc", expected: currentYear},
		{name: "零", yearText: "0", expected: currentYear},
		{name: "负数", yearText: "-1", expected: currentYear},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			year := resolveCompetitionYear(tt.yearText)
			assert.Equal(t, tt.expected, year)
		})
	}
}

// ===================== nextCompetitionCode =====================

func TestNextCompetitionCode_FirstCode(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	// 没有已存在的赛事编号
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_code"}))

	code, err := nextCompetitionCode(database.DB, "校级", 2026)
	assert.NoError(t, err)
	assert.Equal(t, "X2026001", code)
}

func TestNextCompetitionCode_Increment(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	// 已存在 X2026001
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_code"}).AddRow("X2026001"))

	code, err := nextCompetitionCode(database.DB, "校级", 2026)
	assert.NoError(t, err)
	assert.Equal(t, "X2026002", code)
}

func TestNextCompetitionCode_InvalidLevel(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	_, err := nextCompetitionCode(database.DB, "市级", 2026)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不支持的竞赛级别")
}

func TestNextCompetitionCode_AllLevels(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		prefix   string
	}{
		{name: "国家级", level: "国家级", prefix: "G"},
		{name: "省级", level: "省级", prefix: "S"},
		{name: "校级", level: "校级", prefix: "X"},
		{name: "国际级", level: "国际级", prefix: "I"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := setupCompDBMock(t)
			defer mock.ExpectationsWereMet()

			mock.ExpectQuery("SELECT .* FROM `comp_directories`").
				WillReturnRows(sqlmock.NewRows([]string{"comp_code"}))

			code, err := nextCompetitionCode(database.DB, tt.level, 2026)
			assert.NoError(t, err)
			assert.Equal(t, tt.prefix+"2026001", code)
		})
	}
}

// ===================== hasCompetitionStarted =====================

func TestHasCompetitionStarted_NotFound(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id", "comp_start_time"}))

	started, err := hasCompetitionStarted(nil, 1, time.Now())
	assert.NoError(t, err)
	assert.False(t, started)
}

func TestHasCompetitionStarted_ZeroTime(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id", "comp_start_time"}).AddRow(1, nil))

	started, err := hasCompetitionStarted(nil, 1, time.Now())
	assert.NoError(t, err)
	assert.False(t, started)
}

func TestHasCompetitionStarted_BeforeStart(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	startTime := time.Now().Add(24 * time.Hour) // 明天开始
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id", "comp_start_time"}).AddRow(1, startTime))

	started, err := hasCompetitionStarted(nil, 1, time.Now())
	assert.NoError(t, err)
	assert.False(t, started)
}

func TestHasCompetitionStarted_AfterStart(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	startTime := time.Now().Add(-24 * time.Hour) // 昨天开始
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id", "comp_start_time"}).AddRow(1, startTime))

	started, err := hasCompetitionStarted(nil, 1, time.Now())
	assert.NoError(t, err)
	assert.True(t, started)
}

// ===================== resolveCollegeIDByName =====================

func TestResolveCollegeIDByName_EmptyName(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	id, err := resolveCollegeIDByName(database.DB, "")
	assert.NoError(t, err)
	assert.Nil(t, id)
}

func TestResolveCollegeIDByName_Found(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "计算机学院"))

	id, err := resolveCollegeIDByName(database.DB, "计算机学院")
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.Equal(t, uint(1), *id)
}

func TestResolveCollegeIDByName_NotFound(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))

	_, err := resolveCollegeIDByName(database.DB, "不存在的学院")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "学院不存在")
}

// ===================== GetCompetitionList =====================
// 注意：GetCompetitionList 依赖 Redis 缓存，需要集成测试环境

func TestGetCompetitionList_BadRequest(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺少必填参数 page 和 page_size
	_, w, c := buildCompGET("/api/comp/list")
	GetCompetitionList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ===================== CreateCompetition =====================
// 注意：CreateCompetition 成功后会调用 clearCompListCache，需要 Redis

func TestCreateCompetition_BadRequest(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺少必填字段
	req := map[string]interface{}{
		"comp_name": "测试赛事",
	}
	_, w, c := buildCompPOSTJSON(req)
	CreateCompetition(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateCompetition_CollegeNotFound(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	mock.ExpectRollback()

	req := CreateCompetitionReq{
		CompName:  "测试赛事",
		CompLevel: "校级",
		ManagerID: 1,
		College:   "不存在的学院",
	}
	_, w, c := buildCompPOSTJSON(req)
	CreateCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestCreateCompetition_InvalidLevel(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "计算机学院"))
	mock.ExpectRollback()

	req := CreateCompetitionReq{
		CompName:  "测试赛事",
		CompLevel: "市级",
		ManagerID: 1,
		College:   "计算机学院",
	}
	_, w, c := buildCompPOSTJSON(req)
	CreateCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== DeleteCompetition =====================
// 注意：删除成功后会调用 clearCompListCache，需要 Redis

func TestDeleteCompetition_NotFound(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_code"}))

	_, w, c := buildCompGET("/api/comp/delete/999")
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	DeleteCompetition(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteCompetition_Started(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	startTime := time.Now().Add(-24 * time.Hour) // 已开始
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_code"}).AddRow(1, "X2026001"))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id", "comp_start_time"}).AddRow(1, startTime))

	_, w, c := buildCompGET("/api/comp/delete/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	DeleteCompetition(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ===================== RestoreCompetition =====================
// 注意：恢复成功后会调用 clearCompListCache，需要 Redis

func TestRestoreCompetition_NotFound(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_code", "delete_time"}))

	_, w, c := buildCompGET("/api/comp/restore/999")
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	RestoreCompetition(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRestoreCompetition_NotDeleted(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_code", "delete_time"}).AddRow(1, "X2026001", nil))

	_, w, c := buildCompGET("/api/comp/restore/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	RestoreCompetition(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ===================== GetCompetitionDetail =====================

func TestGetCompetitionDetail_NotFound(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_code"}))

	_, w, c := buildCompGET("/api/comp/detail/999")
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	GetCompetitionDetail(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetCompetitionDetail_Success(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_code", "comp_name", "manager_id", "college_id"}).AddRow(1, "X2026001", "测试赛事", 1, 1))
	// GORM Preload 会先查 colleges 再查 users
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "计算机学院"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(1, "管理员"))

	_, w, c := buildCompGET("/api/comp/detail/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	GetCompetitionDetail(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== UpdateCompetition =====================
// 注意：更新成功后会调用 clearCompListCache，需要 Redis

func TestUpdateCompetition_BadRequest(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	req := map[string]interface{}{
		"comp_name": "测试赛事",
		// 缺少必填字段
	}
	_, w, c := buildCompPOSTJSON(req)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateCompetition(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateCompetition_NotFound(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := UpdateCompetitionReq{
		CompName:  "测试赛事",
		CompLevel: "校级",
		ManagerID: 1,
	}
	_, w, c := buildCompPOSTJSON(req)
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	UpdateCompetition(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateCompetition_CollegeNotFound(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_code"}).AddRow(1, "X2026001"))
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))

	req := UpdateCompetitionReq{
		CompName:  "更新后的赛事",
		CompLevel: "省级",
		ManagerID: 2,
		College:   "不存在的学院",
	}
	_, w, c := buildCompPOSTJSON(req)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	// P0-1 后 UpdateCompetition 读取 role_code/user_id 做范围与改派校验；
	// 设置 school_admin 使 scope 短路（真实请求由 AuthRequired 写入）。
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	UpdateCompetition(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ===================== BatchDeleteCompetition =====================
// 注意：删除成功后会调用 clearCompListCache，需要 Redis

func TestBatchDeleteCompetition_BadRequest(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	req := map[string]interface{}{
		"ids": []int{},
	}
	_, w, c := buildCompPOSTJSON(req)
	BatchDeleteCompetition(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBatchDeleteCompetition_StartedComp(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	startTime := time.Now().Add(-24 * time.Hour)
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "已开始赛事"))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id", "comp_start_time"}).AddRow(1, startTime))

	req := BatchDeleteReq{IDs: []uint{1}}
	_, w, c := buildCompPOSTJSON(req)
	BatchDeleteCompetition(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ===================== GetCompetitionYears =====================

func TestGetCompetitionYears_Success(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT DISTINCT `year` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"year"}).AddRow(2026).AddRow(2025).AddRow(2024))

	_, w, c := buildCompGET("/api/comp/years")
	GetCompetitionYears(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetCompetitionYears_DBError(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT DISTINCT `year` FROM `comp_directories`").
		WillReturnError(assert.AnError)

	_, w, c := buildCompGET("/api/comp/years")
	GetCompetitionYears(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== GetManagerList =====================

func TestGetManagerList_BadRequest(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺少必填参数
	_, w, c := buildCompGET("/api/comp/managers")
	GetManagerList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetManagerList_RoleNotFound 已随 P2 改造删除：
// 新实现直接按「教职工池」查 users，不存在「角色不存在」这一失败路径。

func TestGetManagerList_Success(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	// P2 改造后不再查 roles 取角色，直接按「教职工池」查 users；再批量取本页用户角色。
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college"}).AddRow(1, "张三", "T001", "计算机学院"))
	mock.ExpectQuery("SELECT user_roles.user_id AS user_id, roles.role_code AS role_code FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_code"}).AddRow(1, "teacher"))

	_, w, c := buildCompGET("/api/comp/managers?page=1&page_size=10")
	GetManagerList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	// 断言角色映射生效（防止 Scan 失败被忽略后 RoleCode 恒为空）
	assert.Contains(t, w.Body.String(), `"role_code":"teacher"`)
}

func TestGetManagerList_WithFilters(t *testing.T) {
	tests := []struct {
		name        string
		queryString string
	}{
		{name: "按姓名筛选", queryString: "?page=1&page_size=10&name=张"},
		{name: "按工号筛选", queryString: "?page=1&page_size=10&work_id=T001"},
		{name: "按学院筛选", queryString: "?page=1&page_size=10&college=计算机学院"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := setupCompDBMock(t)
			defer mock.ExpectationsWereMet()

			// P2 改造后：count + users；本页 0 行时不触发角色批量查询。
			mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			mock.ExpectQuery("SELECT .* FROM `users`").
				WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college"}))

			_, w, c := buildCompGET("/api/comp/managers" + tt.queryString)
			GetManagerList(c)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// ===================== BatchImportCompetition =====================

func TestBatchImportCompetition_BadRequest(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺少 items 字段
	req := map[string]interface{}{}
	_, w, c := buildCompPOSTJSON(req)
	BatchImportCompetition(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBatchImportCompetition_CollegeNotFound(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	mock.ExpectRollback()

	req := struct {
		Items []CreateCompetitionReq `json:"items"`
	}{
		Items: []CreateCompetitionReq{
			{CompName: "赛事1", CompLevel: "校级", ManagerID: 1, College: "不存在的学院"},
		},
	}
	_, w, c := buildCompPOSTJSON(req)
	BatchImportCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
