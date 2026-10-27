package controllers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// ===================== SaveRegConfig 剩余分支 =====================

// NeedReview=0 且未传评审时间 → 置零；提交截止晚于报名截止 → CompEndTime 取提交截止
func TestSaveRegConfig_ReviewZeroEpochAndSubmitEnd(t *testing.T) {
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
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_directories` SET `status`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	review := 0
	body := ConfigReq{
		CompID:          1,
		RegStartTime:    timePtr(time.Now().Add(time.Hour)),
		RegEndTime:      timePtr(time.Now().Add(2 * time.Hour)),
		SubmitStartTime: timePtr(time.Now().Add(time.Hour)),
		SubmitEndTime:   timePtr(time.Now().Add(3 * time.Hour)), // 晚于报名截止
		NeedReview:      &review,
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"reg:config:edit"})
	SaveRegConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSaveRegConfig_CreateError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `comp_details`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := ConfigReq{CompID: 1, RegStartTime: timePtr(time.Now()), RegEndTime: timePtr(time.Now())}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"reg:config:edit"})
	SaveRegConfig(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSaveRegConfig_SaveError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).AddRow(1, 1, 1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_details` SET `comp_id`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := ConfigReq{CompID: 1, RegStartTime: timePtr(time.Now()), RegEndTime: timePtr(time.Now())}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"reg:config:edit"})
	SaveRegConfig(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSaveRegConfig_NeedRegAuditUpdateError(t *testing.T) {
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
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := ConfigReq{CompID: 1, RegStartTime: timePtr(time.Now()), RegEndTime: timePtr(time.Now())}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"reg:config:edit"})
	SaveRegConfig(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSaveRegConfig_RegistersUpdateError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit"}).AddRow(1, 1, 0))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_details` SET `comp_id`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_details` SET `need_reg_audit`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers` SET `status`=CASE WHEN status = 3 THEN 4 ELSE 1 END").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := ConfigReq{CompID: 1, RegStartTime: timePtr(time.Now()), RegEndTime: timePtr(time.Now())}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"reg:config:edit"})
	SaveRegConfig(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// 专家移除被跳过 → 返回带警告的保存成功
func TestSaveRegConfig_ReviewWarning(t *testing.T) {
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
	// syncReviewTasks：已有任务（status=1）不在新名单 → 警告
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).AddRow(1, 1, 10, 1))
	mock.ExpectQuery("SELECT `realname` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(10, "何专家"))

	review := 1
	body := ConfigReq{
		CompID:       1,
		RegStartTime: timePtr(time.Now().Add(-time.Hour)),
		RegEndTime:   timePtr(time.Now().Add(time.Hour)),
		NeedReview:   &review,
		ExpertIDs:    []uint{},
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"reg:config:edit"})
	SaveRegConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "但有警告")
}

// 报名期已过 → 赛事状态置 2
func TestSaveRegConfig_CompEndedStatus(t *testing.T) {
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
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_directories` SET `status`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := ConfigReq{
		CompID:       1,
		RegStartTime: timePtr(time.Now().Add(-2 * time.Hour)),
		RegEndTime:   timePtr(time.Now().Add(-time.Hour)),
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"reg:config:edit"})
	SaveRegConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== GetSummaryList 分支补齐 =====================

func summaryQueryURL() string {
	return "/api/summary/list?page=0&size=999&comp_name=建模&organizer=教务处&manager=张&summary_status=0&end_time=2026-05-01&year=2026"
}

func TestGetSummaryList_AllFiltersStatus0(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("(?s)SELECT .*comp_directories\\.id as id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id", "comp_name", "organizer", "undertaker", "college_name", "manager_name", "end_time", "summary_status"}).
			AddRow(1, 1, 1, "建模大赛", "教务处", "学院", "计算机学院", "张老师", time.Now(), 0))

	_, w, c := buildGET(summaryQueryURL())
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"summary:edit", "comp:update"})
	GetSummaryList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "建模大赛")
}

func TestGetSummaryList_Status1DisablesEdit(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("(?s)SELECT .*comp_directories\\.id as id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id", "comp_name", "organizer", "undertaker", "college_name", "manager_name", "end_time", "summary_status"}).
			AddRow(1, 1, 1, "建模大赛", "教务处", "学院", "计算机学院", "张老师", nil, 1))

	_, w, c := buildGET("/api/summary/list?page=1&size=10&summary_status=1")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"summary:edit", "comp:update"})
	GetSummaryList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"can_edit_summary\":false")
}

func TestGetSummaryList_InvalidSummaryStatus2(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/summary/list?page=1&size=10&summary_status=9")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetSummaryList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "summary_status 只能是 0 或 1")
}

func TestGetSummaryList_InvalidEndTime2(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/summary/list?page=1&size=10&end_time=2026/05/01")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetSummaryList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "end_time 格式错误")
}

func TestGetSummaryList_InvalidYear2(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/summary/list?page=1&size=10&year=abc")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetSummaryList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "year 只能是数字")
}

func TestGetSummaryList_FindError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("(?s)SELECT .*comp_directories\\.id as id").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/summary/list?page=1&size=10")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetSummaryList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetSummaryList_CapsQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("(?s)SELECT .*comp_directories\\.id as id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id", "comp_name", "organizer", "undertaker", "college_name", "manager_name", "end_time", "summary_status"}).
			AddRow(1, 1, 1, "建模大赛", "教务处", "学院", "计算机学院", "张老师", nil, 0))
	// 未预置 permission_codes → 能力查询走 DB 报错
	mock.ExpectQuery("SELECT .*FROM `permissions`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/summary/list?page=1&size=10")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("role_id", uint(1))
	c.Set("permission_version", 1)
	GetSummaryList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
