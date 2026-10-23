package controllers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// ===================== SubmitRegistration 分支补齐 =====================

func submitRegBody() map[string]interface{} {
	return map[string]interface{}{
		"comp_id":   1,
		"team_name": "报名项目",
		"leader": map[string]interface{}{
			"name": "张三", "stuID": "S001", "phone": "138", "college": "计算机学院",
		},
	}
}

// 期待赛事预加载查询（主表 + Detail）
func expectCompWithDetail(mock sqlmock.Sqlmock, detailRows *sqlmock.Rows) {
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(detailRows)
}

func TestSubmitRegistration_UserIDTypeError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(1, 1))

	body := submitRegBody()
	body["participation_type"] = "individual"
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", "not-a-uint")
	SubmitRegistration(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "用户信息异常")
}

func TestSubmitRegistration_TrackNameMatchThenAdvisorRequired(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectCompWithDetail(mock,
		sqlmock.NewRows([]string{"id", "comp_id", "track", "need_advisor", "reg_start_time", "reg_end_time"}).
			AddRow(1, 1, `[{"trackName":"软件赛道","subTrack":[{"title":"Web开发"}]}]`, 2,
				time.Now().Add(-time.Hour), time.Now().Add(time.Hour)))

	body := submitRegBody()
	body["track"] = "软件赛道"
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须填写指导老师")
}

func TestSubmitRegistration_SubTrackMatchThenAttachmentRequired(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectCompWithDetail(mock,
		sqlmock.NewRows([]string{"id", "comp_id", "track", "need_attachment", "reg_start_time", "reg_end_time"}).
			AddRow(1, 1, `[{"trackName":"软件赛道","subTrack":[{"title":"Web开发"}]}]`, 2,
				time.Now().Add(-time.Hour), time.Now().Add(time.Hour)))

	body := submitRegBody()
	body["track"] = "软件赛道 / Web开发"
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须上传报名附件")
}

func TestSubmitRegistration_LeaderQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectCompWithDetail(mock,
		sqlmock.NewRows([]string{"id", "comp_id", "reg_start_time", "reg_end_time"}).
			AddRow(1, 1, time.Now().Add(-time.Hour), time.Now().Add(time.Hour)))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildAuthPOSTJSON(submitRegBody())
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestSubmitRegistration_AdvisorMarshalThenCreateError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectCompWithDetail(mock,
		sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit", "reg_start_time", "reg_end_time"}).
			AddRow(1, 1, 1, time.Now().Add(-time.Hour), time.Now().Add(time.Hour)))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `registers`").
		WillReturnError(fmt.Errorf("insert failed"))
	mock.ExpectRollback()

	body := submitRegBody()
	body["advisor_info"] = map[string]interface{}{"username": "T003"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSubmitRegistration_DuplicateCreate(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectCompWithDetail(mock,
		sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit", "reg_start_time", "reg_end_time"}).
			AddRow(1, 1, 0, time.Now().Add(-time.Hour), time.Now().Add(time.Hour)))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `registers`").
		WillReturnError(fmt.Errorf("Error 1062: Duplicate entry 'x' for key 'uni'"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(submitRegBody())
	c.Set("user_id", uint(1))
	SubmitRegistration(c)

	assert.Equal(t, 409, w.Code)
	assert.Contains(t, w.Body.String(), "请勿重复提交")
}

// ===================== GetAllUsers 分支补齐 =====================

func TestGetAllUsers_BindError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/perm/member/list?page=abc")
	GetAllUsers(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetAllUsers_AllFilters(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(5, "计算机学院"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/perm/member/list?page=0&size=999&search=张&role_id=3&identity_type=student&college_id=5")
	c.Set("user_id", uint(1))
	GetAllUsers(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetAllUsers_CollegeQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/perm/member/list?page=1&size=10&college_id=5")
	GetAllUsers(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetAllUsers_CountError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/perm/member/list?page=1&size=10&search=张")
	GetAllUsers(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetAllUsers_FindError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/perm/member/list?page=1&size=10&identity_type=student")
	GetAllUsers(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== AssignUserRole 分支补齐 =====================

func assignBody() map[string]interface{} {
	return map[string]interface{}{"role_id": 2}
}

func expectUserAndRoleOK(mock sqlmock.Sqlmock, targetRoleCode string, roleID int64) {
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "managed_college_id"}).AddRow(10, "U10", nil))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(roleID, targetRoleCode, targetRoleCode))
	// school_admin 角色查询
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(1, "school_admin", "校管"))
}

func TestAssignUserRole_UserQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(assignBody())
	c.Params = []gin.Param{{Key: "id", Value: "10"}}
	c.Set("user_id", uint(1))
	AssignUserRole(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAssignUserRole_RoleQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "managed_college_id"}).AddRow(10, "U10", nil))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(assignBody())
	c.Params = []gin.Param{{Key: "id", Value: "10"}}
	c.Set("user_id", uint(1))
	AssignUserRole(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAssignUserRole_SchoolAdminRoleQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "managed_college_id"}).AddRow(10, "U10", nil))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(2, "teacher", "老师"))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(assignBody())
	c.Params = []gin.Param{{Key: "id", Value: "10"}}
	c.Set("user_id", uint(1))
	AssignUserRole(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAssignUserRole_CurrentLinksError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectUserAndRoleOK(mock, "teacher", 2)
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(assignBody())
	c.Params = []gin.Param{{Key: "id", Value: "10"}}
	c.Set("user_id", uint(1))
	AssignUserRole(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAssignUserRole_MultiRoleConflict(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectUserAndRoleOK(mock, "teacher", 2)
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).
			AddRow(10, 2).AddRow(10, 3))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(assignBody())
	c.Params = []gin.Param{{Key: "id", Value: "10"}}
	c.Set("user_id", uint(1))
	AssignUserRole(c)

	assert.Equal(t, 409, w.Code)
	assert.Contains(t, w.Body.String(), "用户存在多个角色")
}

func TestAssignUserRole_CollegeQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "managed_college_id"}).AddRow(10, "U10", nil))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(2, "college_admin", "院管"))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(1, "school_admin", "校管"))
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := map[string]interface{}{"role_id": 2, "managed_college_id": 5}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "10"}}
	c.Set("user_id", uint(1))
	AssignUserRole(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAssignUserRole_SetUserRoleError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectUserAndRoleOK(mock, "teacher", 2)
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))
	mock.ExpectExec("DELETE FROM `user_roles`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(assignBody())
	c.Params = []gin.Param{{Key: "id", Value: "10"}}
	c.Set("user_id", uint(1))
	AssignUserRole(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAssignUserRole_ManagedCollegeUpdateError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectUserAndRoleOK(mock, "teacher", 2)
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))
	mock.ExpectExec("DELETE FROM `user_roles`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_roles`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE `users` SET `managed_college_id`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(assignBody())
	c.Params = []gin.Param{{Key: "id", Value: "10"}}
	c.Set("user_id", uint(1))
	AssignUserRole(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAssignUserRole_AuditCreateError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectUserAndRoleOK(mock, "teacher", 2)
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))
	mock.ExpectExec("DELETE FROM `user_roles`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_roles`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE `users` SET `managed_college_id`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_role_audits`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(assignBody())
	c.Params = []gin.Param{{Key: "id", Value: "10"}}
	c.Set("user_id", uint(1))
	AssignUserRole(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

var _ = gorm.ErrRecordNotFound
