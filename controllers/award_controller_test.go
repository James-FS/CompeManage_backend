package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"CompeManage_backend/database"
	"CompeManage_backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func buildGET(urlStr string) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
	req := httptest.NewRequest("GET", urlStr, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return req, w, c
}

func setupAwardDBMock(t *testing.T) sqlmock.Sqlmock {
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

// ===================== mapAwardStatusToInt =====================

func TestMapAwardStatusToInt(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected int
	}{
		{name: "approved", status: "approved", expected: 1},
		{name: "rejected", status: "rejected", expected: 2},
		{name: "draft", status: "draft", expected: 0},
		{name: "空字符串", status: "", expected: 0},
		{name: "未知状态", status: "unknown", expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapAwardStatusToInt(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ===================== mapAwardStatusFromInt =====================

func TestMapAwardStatusFromInt(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		expected string
	}{
		{name: "1=approved", status: 1, expected: "approved"},
		{name: "2=rejected", status: 2, expected: "rejected"},
		{name: "0=draft", status: 0, expected: "draft"},
		{name: "负数", status: -1, expected: "draft"},
		{name: "大数字", status: 999, expected: "draft"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapAwardStatusFromInt(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ===================== getLeaderMember =====================

func TestGetLeaderMember_WithLeader(t *testing.T) {
	members := []models.RegMember{
		{Name: "张三", StudentID: "2021001", Phone: "13800138000", Email: "zhangsan@test.com", IsLeader: false},
		{Name: "李四", StudentID: "2021002", Phone: "13900139000", Email: "lisi@test.com", IsLeader: true},
	}
	fallback := models.User{Realname: "王五", Username: "2021003"}

	name, studentID, phone, email := getLeaderMember(members, fallback)
	assert.Equal(t, "李四", name)
	assert.Equal(t, "2021002", studentID)
	assert.Equal(t, "13900139000", phone)
	assert.Equal(t, "lisi@test.com", email)
}

func TestGetLeaderMember_NoLeader(t *testing.T) {
	members := []models.RegMember{
		{Name: "张三", StudentID: "2021001", Phone: "13800138000", Email: "zhangsan@test.com", IsLeader: false},
		{Name: "李四", StudentID: "2021002", Phone: "13900139000", Email: "lisi@test.com", IsLeader: false},
	}
	fallback := models.User{Realname: "王五", Username: "2021003"}

	name, studentID, phone, email := getLeaderMember(members, fallback)
	assert.Equal(t, "王五", name)
	assert.Equal(t, "2021003", studentID)
	assert.Equal(t, "", phone)
	assert.Equal(t, "", email)
}

func TestGetLeaderMember_EmptyMembers(t *testing.T) {
	members := []models.RegMember{}
	fallback := models.User{Realname: "王五", Username: "2021003"}

	name, studentID, phone, email := getLeaderMember(members, fallback)
	assert.Equal(t, "王五", name)
	assert.Equal(t, "2021003", studentID)
	assert.Equal(t, "", phone)
	assert.Equal(t, "", email)
}

func TestGetLeaderMember_NilMembers(t *testing.T) {
	fallback := models.User{Realname: "王五", Username: "2021003"}

	name, studentID, phone, email := getLeaderMember(nil, fallback)
	assert.Equal(t, "王五", name)
	assert.Equal(t, "2021003", studentID)
	assert.Equal(t, "", phone)
	assert.Equal(t, "", email)
}

// ===================== DateOnly.UnmarshalJSON =====================

func TestDateOnly_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
		expectNil bool
	}{
		{
			name:      "日期格式",
			input:     `"2026-01-15"`,
			expectErr: false,
		},
		{
			name:      "RFC3339格式",
			input:     `"2026-01-15T10:30:00Z"`,
			expectErr: false,
		},
		{
			name:      "空字符串",
			input:     `""`,
			expectErr: false,
			expectNil: true,
		},
		{
			name:      "null值",
			input:     `"null"`,
			expectErr: false,
			expectNil: true,
		},
		{
			name:      "无效格式",
			input:     `"invalid-date"`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d DateOnly
			err := json.Unmarshal([]byte(tt.input), &d)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ===================== SearchCompetition =====================

func TestSearchCompetition_EmptyKeyword(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/award/search")
	SearchCompetition(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "关键词不能为空")
}

func TestSearchCompetition_Success(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name", "year", "create_time"}).
			AddRow(1, "数学建模大赛", 2026, now).
			AddRow(2, "数学竞赛", 2025, now))

	_, w, c := buildGET("/api/award/search?keyword=数学")
	SearchCompetition(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "数学建模大赛")
	assert.Contains(t, w.Body.String(), "数学竞赛")
}

func TestSearchCompetition_NoResult(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name", "year"}))

	_, w, c := buildGET("/api/award/search?keyword=不存在的赛事")
	SearchCompetition(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSearchCompetition_CustomPageSize(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name", "year"}))

	_, w, c := buildGET("/api/award/search?keyword=数学&page_size=50")
	SearchCompetition(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== GetAwardAuditDetail =====================

func TestGetAwardAuditDetail_NotFound(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/award/audit/999")
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	GetAwardAuditDetail(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "记录不存在")
}

// ===================== PassAwardAudit =====================

func TestPassAwardAudit_NotFound(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/award/audit/pass/999")
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	c.Set("user_id", uint(1))
	PassAwardAudit(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPassAwardAudit_AlreadyAudited(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id"}).AddRow(1, "approved", 1))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	_, w, c := buildGET("/api/award/audit/pass/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	PassAwardAudit(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "该记录已审核")
}

// ===================== RejectAwardAudit =====================

func TestRejectAwardAudit_NotFound(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := map[string]interface{}{"reason": "材料不全"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	c.Set("user_id", uint(1))
	RejectAwardAudit(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRejectAudit_MissingReason(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	RejectAwardAudit(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "驳回原因必填")
}

func TestRejectAudit_AlreadyAudited(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id"}).AddRow(1, "approved", 1))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	body := map[string]interface{}{"reason": "材料不全"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	RejectAwardAudit(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "该记录已审核")
}

// ===================== BatchPassAwardAudit =====================

func TestBatchPassAudit_EmptyIDs(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"ids": []int{}}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchPassAwardAudit(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "ids 必填")
}

func TestBatchPassAudit_NoEligibleRecords(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards`").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	body := map[string]interface{}{"ids": []uint{999}}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchPassAwardAudit(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "没有可审核的记录")
}

// ===================== BatchRejectAwardAudit =====================

func TestBatchRejectAudit_EmptyIDs(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"ids": []int{}, "reason": "材料不全"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchRejectAwardAudit(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "ids 和 reason 必填")
}

func TestBatchRejectAudit_MissingReason(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"ids": []uint{1}}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchRejectAwardAudit(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBatchRejectAudit_NoEligibleRecords(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards`").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	body := map[string]interface{}{"ids": []uint{999}, "reason": "材料不全"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchRejectAwardAudit(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "没有可审核的记录")
}

// ===================== GetStudentMyAwardList =====================

func TestGetStudentMyAwardList_Unauthorized(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/award/my")
	GetStudentMyAwardList(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "请先登录")
}

func TestGetStudentMyAwardList_InvalidStatus(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/award/my?status=invalid")
	c.Set("user_id", uint(1))
	GetStudentMyAwardList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "状态参数错误")
}

func TestGetStudentMyAwardList_InvalidCompID(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/award/my?comp_id=abc")
	c.Set("user_id", uint(1))
	GetStudentMyAwardList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛事ID格式错误")
}

func TestGetStudentMyAwardList_Success(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "reg_id", "award_level"}).AddRow(1, 1, 1, "一等奖"))
	// GORM Preload queries: registers -> users (Leader) -> reg_members (Members) -> comp_directories (Competition)
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "leader_id"}).AddRow(1, 1))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(1, "张三"))
	mock.ExpectQuery("SELECT .* FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id"}).AddRow(1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "测试赛事"))

	_, w, c := buildGET("/api/award/my?page=1&size=10")
	c.Set("user_id", uint(1))
	GetStudentMyAwardList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== GetAwardCompList =====================

func TestGetAwardCompList_Success(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	// checkUserIsAdmin 查询 user_roles JOIN roles
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// Count comp_directories
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// Find comp_directories
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "测试赛事"))
	// Preload Detail queries comp_details
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(1, 1))

	_, w, c := buildGET("/api/award/comps?page=1&size=10")
	c.Set("user_id", uint(1))
	GetAwardCompList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}
