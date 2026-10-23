package controllers

import (
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

// setupUnorderedDBMock：允许乱序匹配，适合带多个 Preload 的查询（避免顺序耦合）
func setupUnorderedDBMock(t *testing.T) sqlmock.Sqlmock {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	assert.NoError(t, err)
	mock.MatchExpectationsInOrder(false)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	assert.NoError(t, err)
	database.DB = gormDB
	return mock
}

// ===================== SaveRegConfig =====================

func TestSaveRegConfig_SubmitTimesHalfFilled(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{
		"comp_id": 1,
		"reg_start_time": time.Now(),
		"reg_end_time":   time.Now(),
		"submit_start_time": time.Now(),
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SaveRegConfig(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "作品提交时间必须同时填写")
}

func TestSaveRegConfig_ScopeInvalid(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{
		"comp_id": 1,
		"reg_start_time": time.Now(),
		"reg_end_time":   time.Now(),
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin") // 未配置管理学院
	SaveRegConfig(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSaveRegConfig_CompNotFound(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := map[string]interface{}{
		"comp_id": 999,
		"reg_start_time": time.Now(),
		"reg_end_time":   time.Now(),
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	SaveRegConfig(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "您无权操作此赛事或赛事不存在")
}

func TestSaveRegConfig_InvalidNeedRegAudit(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))

	audit := 2
	body := ConfigReq{
		CompID:        1,
		RegStartTime:  timePtr(time.Now().Add(-time.Hour)),
		RegEndTime:    timePtr(time.Now().Add(time.Hour)),
		NeedRegAudit:  &audit,
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"reg:config:edit"})
	SaveRegConfig(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "need_reg_audit 仅支持 0 或 1")
}

// 新建配置：NeedReview=0（覆盖评审时间置零分支）+ 免审核联动 + 赛事状态更新
func TestSaveRegConfig_CreateWithReviewDisabled(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `comp_details`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_details` SET `need_reg_audit`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers` SET `status`=CASE WHEN status = 3 THEN 4 ELSE 1 END").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_directories` SET `status`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	audit := 0
	body := ConfigReq{
		CompID:        1,
		RegStartTime:  timePtr(time.Now().Add(time.Hour)),
		RegEndTime:    timePtr(time.Now().Add(2 * time.Hour)),
		NeedRegAudit:  &audit,
		GradeRequirement: []int{2024},
		AwardHierarchy:   []string{"一等奖"},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"reg:config:edit"})
	SaveRegConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "报名设置保存成功")
}

// 更新配置：NeedReview=1 + 评审起止时间（覆盖评审配置赋值分支）
func TestSaveRegConfig_UpdateWithReviewEnabled(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).AddRow(1, 1, 1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_details` SET `comp_id`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_details` SET `need_reg_audit`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// syncReviewTasks：无已有任务且无专家 → 无写入
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_directories` SET `status`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	review := 1
	body := ConfigReq{
		CompID:           1,
		RegStartTime:     timePtr(time.Now().Add(-time.Hour)),
		RegEndTime:       timePtr(time.Now().Add(time.Hour)),
		NeedReview:       &review,
		ReviewStartTime:  timePtr(time.Now().Add(-time.Hour)),
		ReviewEndTime:    timePtr(time.Now().Add(time.Hour)),
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"reg:config:edit"})
	SaveRegConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "报名设置保存成功")
}

func timePtr(t time.Time) *time.Time { return &t }

// ===================== GetRegList =====================

func TestGetRegList_AllFiltersAndPagingDefaults(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/reg/list?page=abc&size=999&comp_name=建模&status=1&participant_type=2&keyword=k&email=e&phone=p")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetRegList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetRegList_PageSizeTooSmall(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/reg/list?page=0&size=0")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetRegList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetRegList_CountError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/list")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetRegList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetRegList_FindError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/list")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetRegList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== GetMyRegList =====================

func TestGetMyRegList_FullBranches(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	// 两条：status=0 与 status=3（免审核映射两个分支）
	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "attachment_url", "work_attachment_url", "advisor_info", "advisor_id", "track"}).
			AddRow(1, 1, 0, "/a.pdf", "/w.pdf", "[]", 0, "").
			AddRow(2, 1, 3, "/b.pdf", "/w2.pdf", "[]", 0, ""))
	// Preload Competition / Detail（乱序匹配）
	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "建模大赛"))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit", "submit_start_time", "submit_end_time"}).
			AddRow(1, 1, 0, time.Now().Add(-time.Hour), time.Now().Add(time.Hour)))

	_, w, c := buildGET("/api/reg/my-reg?page=0&size=0&id=5&registration_only=true")
	c.Set("user_id", uint(1))
	GetMyRegList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "建模大赛")
}

func TestGetMyRegList_FindError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/my-reg")
	c.Set("user_id", uint(1))
	GetMyRegList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== GetMyRegStatus =====================

func TestGetMyRegStatus_UserIDTypeError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/status?comp_id=1")
	c.Set("user_id", "not-a-uint")
	GetMyRegStatus(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetMyRegStatus_RegQueryError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/status?comp_id=1")
	c.Set("user_id", uint(1))
	GetMyRegStatus(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetMyRegStatus_SuccessWithAutoApprove(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status", "team_name", "reject_reason", "attachment_url", "work_attachment_url", "advisor_info", "advisor_id", "track"}).
			AddRow(1, 1, 1, 0, "团队A", "", "/a.pdf", "/w.pdf", "[]", 0, ""))
	// Preload（乱序）：Members / Competition / Detail / Leader
	mock.ExpectQuery("SELECT \\* FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id", "name", "username", "phone", "email", "college", "is_leader"}).
			AddRow(1, 1, "张三", "S001", "138", "a@b.c", "计算机学院", true))
	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "建模大赛"))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).AddRow(1, 1, 0))
	mock.ExpectQuery("SELECT \\* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname"}).AddRow(1, "S001", "张三"))
	// 免审核自动通过
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers` SET `status`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	_, w, c := buildGET("/api/reg/status?comp_id=1")
	c.Set("user_id", uint(1))
	GetMyRegStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "团队A")
	assert.Contains(t, w.Body.String(), "张三")
}

// ===================== GetWorkAuditCompList =====================

func TestGetWorkAuditCompList_AllFiltersAndPaging(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("(?s)SELECT COUNT\\(DISTINCT\\(`comp_directories`\\.`id`\\)\\)").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("(?s)SELECT .*comp_directories\\.id AS comp_id").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}))

	_, w, c := buildGET("/api/reg/work/audit/comp/list?page=0&size=0&comp_name=建模&manager=张&college=计算机学院&status=1")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetWorkAuditCompList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"page\":1")
}

func TestGetWorkAuditCompList_CountError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("(?s)SELECT COUNT\\(DISTINCT\\(`comp_directories`\\.`id`\\)\\)").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/work/audit/comp/list")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetWorkAuditCompList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetWorkAuditCompList_ListError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("(?s)SELECT COUNT\\(DISTINCT\\(`comp_directories`\\.`id`\\)\\)").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("(?s)SELECT .*comp_directories\\.id AS comp_id").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/work/audit/comp/list")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetWorkAuditCompList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== AuditRegister 分支补齐 =====================

func TestAuditRegister_ScopeInvalid(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 0))
	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).AddRow(1, 1, 1))

	body := map[string]interface{}{"id": 1, "status": 1}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin") // 未配置管理学院
	AuditRegister(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAuditRegister_AccessDenied(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 0))
	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 99, 1))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).AddRow(1, 1, 1))

	body := map[string]interface{}{"id": 1, "status": 1}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "competition_manager")
	AuditRegister(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "您无权审核此条记录")
}

func TestAuditRegister_PassSuccess(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 0))
	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).AddRow(1, 1, 1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"id": 1, "status": 1}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	AuditRegister(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "审核完成")
}

func TestAuditRegister_RejectSuccess(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 0))
	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).AddRow(1, 1, 1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"id": 1, "status": 2, "reason": "材料不全"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	AuditRegister(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuditRegister_UpdateError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("(?s)SELECT (\\*|`registers`\\.`id`).*FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 0))
	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).AddRow(1, 1, 1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := map[string]interface{}{"id": 1, "status": 1}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	AuditRegister(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
