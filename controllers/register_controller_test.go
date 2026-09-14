package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupRegDBMock(t *testing.T) sqlmock.Sqlmock {
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

// ===================== isValidTime =====================

func TestIsValidTime(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected bool
	}{
		{name: "zero time", input: time.Time{}, expected: false},
		{name: "year 1970", input: time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), expected: false},
		{name: "year 1969", input: time.Date(1969, 1, 1, 0, 0, 0, 0, time.UTC), expected: false},
		{name: "year 1971", input: time.Date(1971, 1, 1, 0, 0, 0, 0, time.UTC), expected: true},
		{name: "year 2026", input: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), expected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isValidTime(tt.input))
		})
	}
}

// ===================== GetRegConfig =====================

func TestGetRegConfig_MissingCompID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/config")
	GetRegConfig(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "缺少 comp_id")
}

func TestGetRegConfig_InvalidCompID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/config?comp_id=abc")
	GetRegConfig(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "格式错误")
}

func TestGetRegConfig_NotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// Preload("Detail").First(&comp, id) → 先查主表
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}))
	// Preload Detail
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}))

	_, w, c := buildGET("/api/reg/config?comp_id=999")
	GetRegConfig(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在")
}

func TestGetRegConfig_NoDetail(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// Preload("Detail").First → 主表返回记录
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))
	// Preload Detail → Detail.ID == 0 (无配置)
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}))

	_, w, c := buildGET("/api/reg/config?comp_id=1")
	GetRegConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "测试赛事")
}

func TestGetRegConfig_Success(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	regStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	regEnd := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	submitStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	submitEnd := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	gradeJSON, _ := json.Marshal([]int{2022, 2023})
	hierarchyJSON, _ := json.Marshal([]string{"一等奖", "二等奖"})
	trackJSON, _ := json.Marshal([]TrackReq{{TrackName: "赛道A"}})

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "participant_type", "min_team_member", "max_team_member",
			"need_advisor", "need_attachment", "need_reg_audit",
			"grade_requirement", "award_hierarchy", "track",
			"reg_start_time", "reg_end_time", "submit_start_time", "submit_end_time",
		}).AddRow(1, 1, 2, 2, 5, 1, 1, 1,
			string(gradeJSON), string(hierarchyJSON), string(trackJSON),
			regStart, regEnd, submitStart, submitEnd))

	_, w, c := buildGET("/api/reg/config?comp_id=1")
	GetRegConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "测试赛事")
	assert.Contains(t, w.Body.String(), "赛道A")
}

// ===================== GetRegDetail =====================

func TestGetRegDetail_MissingID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/detail")
	c.Set("user_id", uint(1))
	GetRegDetail(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "缺少 id")
}

func TestGetRegDetail_InvalidID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/detail?id=abc")
	c.Set("user_id", uint(1))
	GetRegDetail(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "格式错误")
}

func TestGetRegDetail_NotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/reg/detail?id=999")
	c.Set("user_id", uint(1))
	GetRegDetail(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "报名记录不存在")
}

// ===================== AuditRegister =====================

func TestAuditRegister_BadRequest(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	AuditRegister(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestAuditRegister_InvalidStatus(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"id": 1, "status": 3}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	AuditRegister(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "非法的审核状态")
}

func TestAuditRegister_RejectWithoutReason(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"id": 1, "status": 2, "reason": ""}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	AuditRegister(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须填写原因")
}

func TestAuditRegister_NotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&reg, req.ID) with Preload
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := map[string]interface{}{"id": 999, "status": 1}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	AuditRegister(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "记录不存在")
}

func TestAuditRegister_NoAuditNeeded(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&reg, req.ID) with Preload
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).
			AddRow(1, 1, 0))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id"}).
			AddRow(1, 10))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).
			AddRow(1, 1, 0))

	// NeedRegAudit==0 && status==0 → Updates status to 1
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"id": 1, "status": 1}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	AuditRegister(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "免审核")
}

func TestAuditRegister_AlreadyAudited(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&reg) with Preload → status=1 (已通过)
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).
			AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id"}).
			AddRow(1, 10))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).
			AddRow(1, 1, 1))

	body := map[string]interface{}{"id": 1, "status": 1}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	AuditRegister(c)

	assert.Equal(t, 409, w.Code)
	assert.Contains(t, w.Body.String(), "已被审核")
}

func TestAuditRegister_Forbidden_NonManager(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&reg) with Preload → status=0
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).
			AddRow(1, 1, 0))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id"}).
			AddRow(1, 10)) // manager_id=10
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).
			AddRow(1, 1, 1))

	// checkUserIsAdmin → count=0 (非管理员)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	body := map[string]interface{}{"id": 1, "status": 1}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(5)) // user_id=5, not manager
	AuditRegister(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无权审核")
}

// ===================== GetMyRegStatus =====================

func TestGetMyRegStatus_MissingCompID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/my/status")
	c.Set("user_id", uint(1))
	GetMyRegStatus(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "缺少 comp_id")
}

func TestGetMyRegStatus_InvalidCompID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/my/status?comp_id=abc")
	c.Set("user_id", uint(1))
	GetMyRegStatus(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "格式错误")
}

func TestGetMyRegStatus_Unauthorized(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/my/status?comp_id=1")
	GetMyRegStatus(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestGetMyRegStatus_UserDBError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// Select("username").First(&user, userID) → DB error
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/my/status?comp_id=1")
	c.Set("user_id", uint(1))
	GetMyRegStatus(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestGetMyRegStatus_NotRegistered(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// Select("username").First(&user, userID)
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).
			AddRow(1, "2022001"))

	// Preload("Members").Preload("Competition").Preload("Competition.Detail").Preload("Leader").
	// Joins("INNER JOIN reg_members ...").
	// Where("registers.comp_id = ? AND reg_members.username = ?", ...).First(&reg)
	// → record not found
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/reg/my/status?comp_id=1")
	c.Set("user_id", uint(1))
	GetMyRegStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "success")
}

// ===================== SubmitWork =====================

func TestSubmitWork_BadRequest(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitWork(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestSubmitWork_Unauthorized(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	body := SubmitWorkReq{RegID: 1, WorkUrl: "/files/work.pdf"}
	_, w, c := buildAuthPOSTJSON(body)
	SubmitWork(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestSubmitWork_NotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// Select("username").First(&user, userID)
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).
			AddRow(1, "2022001"))

	// Preload("Competition.Detail").First(&reg, req.RegID) → not found
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := SubmitWorkReq{RegID: 999, WorkUrl: "/files/work.pdf"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitWork(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "报名记录不存在")
}

func TestSubmitWork_NotMember(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// Select("username").First(&user)
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).
			AddRow(1, "2022001"))

	// Preload("Competition.Detail").First(&reg)
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).
			AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit", "submit_start_time", "submit_end_time"}).
			AddRow(1, 1, 1, time.Now().Add(-1*time.Hour), time.Now().Add(1*time.Hour)))

	// Count reg_members → 0 (不是团队成员)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	body := SubmitWorkReq{RegID: 1, WorkUrl: "/files/work.pdf"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitWork(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "不是该团队的成员")
}

// ===================== ResubmitRegistration =====================

func TestResubmitRegistration_BadRequest(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺少 comp_id (binding:"required")
	body := map[string]interface{}{"team_name": "test"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestResubmitRegistration_Unauthorized(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestResubmitRegistration_NotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// Where("comp_id = ? AND leader_id = ?").First(&reg) → not found
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "未找到原报名记录")
}

func TestResubmitRegistration_AlreadyApproved(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&reg) → status=1 (已通过)
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).
			AddRow(1, 1, 1, 1))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "审核已通过")
}

func TestResubmitRegistration_ConfigNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&reg)
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).
			AddRow(1, 1, 1, 0))
	// First(&detail) → not found
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛事配置不存在")
}

func TestResubmitRegistration_NeedAttachment(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// First(&reg)
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).
			AddRow(1, 1, 1, 0))
	// First(&detail) → need_attachment=2
	now := time.Now()
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "need_attachment", "need_advisor",
			"reg_start_time", "reg_end_time",
		}).AddRow(1, 1, 2, 0, now.Add(-1*time.Hour), now.Add(1*time.Hour)))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须上传报名附件")
}

func TestResubmitRegistration_NeedAdvisor(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).
			AddRow(1, 1, 1, 0))

	now := time.Now()
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "need_attachment", "need_advisor",
			"reg_start_time", "reg_end_time",
		}).AddRow(1, 1, 0, 2, now.Add(-1*time.Hour), now.Add(1*time.Hour)))

	body := ApplicationReq{
		CompID:        1,
		TeamName:      "test",
		AttachmentUrl: "/files/a.pdf",
		Leader:        MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须填写指导老师")
}

func TestResubmitRegistration_NeedTrack(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).
			AddRow(1, 1, 1, 0))

	now := time.Now()
	trackJSON := `[{"trackName":"赛道A","subTrack":[]}]`
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "need_attachment", "need_advisor", "track",
			"reg_start_time", "reg_end_time",
		}).AddRow(1, 1, 0, 0, trackJSON, now.Add(-1*time.Hour), now.Add(1*time.Hour)))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Track:    "",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须选择赛道")
}

func TestResubmitRegistration_InvalidTrack(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).
			AddRow(1, 1, 1, 0))

	now := time.Now()
	trackJSON := `[{"trackName":"赛道A","subTrack":[]}]`
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "need_attachment", "need_advisor", "track",
			"reg_start_time", "reg_end_time",
		}).AddRow(1, 1, 0, 0, trackJSON, now.Add(-1*time.Hour), now.Add(1*time.Hour)))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Track:    "不存在的赛道",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛道无效")
}

func TestResubmitRegistration_SubmitTooEarly(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).
			AddRow(1, 1, 1, 0))

	// 报名尚未开始
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "need_attachment", "need_advisor",
			"reg_start_time", "reg_end_time",
		}).AddRow(1, 1, 0, 0, time.Now().Add(1*time.Hour), time.Now().Add(2*time.Hour)))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "报名尚未开始")
}

func TestResubmitRegistration_SubmitTooLate(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).
			AddRow(1, 1, 1, 0))

	// 报名已截止
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "need_attachment", "need_advisor",
			"reg_start_time", "reg_end_time",
		}).AddRow(1, 1, 0, 0, time.Now().Add(-2*time.Hour), time.Now().Add(-1*time.Hour)))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "报名已截止")
}

// ===================== GetMyRegList =====================

func TestGetMyRegList_Unauthorized(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/my/list")
	GetMyRegList(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestGetMyRegList_UserDBError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/my/list")
	c.Set("user_id", uint(1))
	GetMyRegList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// ===================== checkUserIsAdmin =====================

func TestCheckUserIsAdmin_True(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	result := checkUserIsAdmin(nil, 1)
	assert.True(t, result)
}

func TestCheckUserIsAdmin_False(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	result := checkUserIsAdmin(nil, 1)
	assert.False(t, result)
}

func TestCheckUserIsAdmin_DBError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles`").
		WillReturnError(fmt.Errorf("db error"))

	result := checkUserIsAdmin(nil, 1)
	assert.False(t, result)
}

// ===================== SubmitRegistration =====================

func TestSubmitRegistration_BadRequest(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺少 comp_id (binding:"required")
	body := map[string]interface{}{"team_name": "test"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数格式错误")
}

func TestSubmitRegistration_Unauthorized(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	SubmitRegistration(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestSubmitRegistration_CompNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// Preload("Detail").First(&comp, req.CompID) → not found
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := ApplicationReq{
		CompID:   999,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在")
}

func TestSubmitRegistration_ConfigNotComplete(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// Preload("Detail").First → Detail.ID == 0
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"})) // ID=0

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛事配置未完成")
}

func TestSubmitRegistration_RegNotStarted(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// Preload("Detail").First → reg_start_time in the future
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "reg_start_time", "reg_end_time",
		}).AddRow(1, 1, time.Now().Add(1*time.Hour), time.Now().Add(2*time.Hour)))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "报名尚未开始")
}

func TestSubmitRegistration_RegEnded(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "reg_start_time", "reg_end_time",
		}).AddRow(1, 1, time.Now().Add(-2*time.Hour), time.Now().Add(-1*time.Hour)))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "报名已截止")
}

func TestSubmitRegistration_NeedTrack(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))

	trackJSON := `[{"trackName":"赛道A","subTrack":[]}]`
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "reg_start_time", "reg_end_time", "track",
		}).AddRow(1, 1, time.Now().Add(-1*time.Hour), time.Now().Add(1*time.Hour), trackJSON))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Track:    "",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须选择赛道")
}

func TestSubmitRegistration_InvalidTrack(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))

	trackJSON := `[{"trackName":"赛道A","subTrack":[]}]`
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "reg_start_time", "reg_end_time", "track",
		}).AddRow(1, 1, time.Now().Add(-1*time.Hour), time.Now().Add(1*time.Hour), trackJSON))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Track:    "不存在的赛道",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛道无效")
}

func TestSubmitRegistration_NeedAttachment(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "reg_start_time", "reg_end_time", "need_attachment",
		}).AddRow(1, 1, time.Now().Add(-1*time.Hour), time.Now().Add(1*time.Hour), 2))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须上传报名附件")
}

func TestSubmitRegistration_NeedAdvisor(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "reg_start_time", "reg_end_time", "need_attachment", "need_advisor",
		}).AddRow(1, 1, time.Now().Add(-1*time.Hour), time.Now().Add(1*time.Hour), 0, 2))

	body := ApplicationReq{
		CompID:        1,
		TeamName:      "test",
		AttachmentUrl: "/files/a.pdf",
		Leader:        MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须填写指导老师")
}

func TestSubmitRegistration_LeaderNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "reg_start_time", "reg_end_time", "need_attachment", "need_advisor",
		}).AddRow(1, 1, time.Now().Add(-1*time.Hour), time.Now().Add(1*time.Hour), 0, 0))

	// Where("username = ?", req.Leader.StuID).First(&leader) → not found
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "99999999"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "负责人学号不存在")
}

func TestSubmitRegistration_Duplicate(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "reg_start_time", "reg_end_time", "need_attachment", "need_advisor",
		}).AddRow(1, 1, time.Now().Add(-1*time.Hour), time.Now().Add(1*time.Hour), 0, 0))

	// Where("username = ?", ...).First(&leader) → found
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).
			AddRow(1, "2022001"))

	// Create(&register) → duplicate key
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `registers`").
		WillReturnError(fmt.Errorf("Duplicate entry '1-1' for key 'idx_comp_leader'"))
	mock.ExpectRollback()

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, 409, w.Code)
	assert.Contains(t, w.Body.String(), "请勿重复提交")
}

func TestSubmitRegistration_CreateDBError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).
			AddRow(1, "测试赛事"))

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "comp_id", "reg_start_time", "reg_end_time", "need_attachment", "need_advisor", "need_reg_audit",
		}).AddRow(1, 1, time.Now().Add(-1*time.Hour), time.Now().Add(1*time.Hour), 0, 0, 1))

	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).
			AddRow(1, "2022001"))

	// Create(&register) → generic error
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `registers`").
		WillReturnError(fmt.Errorf("some db error"))
	mock.ExpectRollback()

	body := ApplicationReq{
		CompID:   1,
		TeamName: "test",
		Leader:   MemberReq{Name: "张三", StuID: "2022001"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// ===================== GetRegList =====================

func TestGetRegList_Unauthorized(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/list")
	GetRegList(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

// ===================== GetUserList =====================

func TestGetUserList_BadRequest(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺少 role (binding:"required")
	_, w, c := buildGET("/api/reg/users?page=1&page_size=10")
	GetUserList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}
