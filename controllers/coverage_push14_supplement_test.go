package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// 补录请求最小合法载荷
func supplementBody() map[string]interface{} {
	return map[string]interface{}{
		"comp_id":     1,
		"team_name":   "补录项目",
		"award_level": "省级一等奖",
		"award_name":  "一等奖",
		"award_date":  "2026-05-01",
		"proof_url":   "/api/file/download/award_proof/0123456789abcdef0123456789abcdef_proof.pdf",
		"advisors":    []map[string]string{{"username": "T003", "name": "王指导", "college": "计算机学院"}},
		"members": []map[string]interface{}{{
			"name": "张三", "student_id": "S001", "phone": "13800000000",
			"college": "计算机学院", "is_leader": true,
		}},
	}
}

// 常用期望：事务内赛事锁定查询
func expectCompLockOK(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
}

// 常用期望：指导教师工号回查命中
func expectTeacherFound(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college", "identity_type"}).
			AddRow(3, "T003", "王指导", "计算机学院", "staff"))
}

// ===================== SubmitStudentAwardSupplement =====================

func TestSubmitSupplement_UserIDTypeError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", "not-a-uint")
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "用户ID格式错误")
}

func TestSubmitSupplement_MemberStudentIDBlank(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	body := supplementBody()
	body["members"] = []map[string]interface{}{{
		"name": "张三", "student_id": "   ", "phone": "13800000000",
		"college": "计算机学院", "is_leader": true,
	}}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "不能为空")
}

func TestSubmitSupplement_TeamNameBlank(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	body := supplementBody()
	body["team_name"] = "   "
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "项目名称不能为空且不能超过255字")
}

func TestSubmitSupplement_BeginError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin().WillReturnError(fmt.Errorf("begin failed"))

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestSubmitSupplement_CompNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在")
}

func TestSubmitSupplement_TeacherNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectCompLockOK(mock)
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "不存在或不是教职工")
}

func TestSubmitSupplement_LoginStudentQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectCompLockOK(mock)
	expectTeacherFound(mock)
	// 登录学生回查失败
	mock.ExpectQuery("SELECT `id`,`username` FROM `users`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestSubmitSupplement_RegQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectCompLockOK(mock)
	expectTeacherFound(mock)
	mock.ExpectQuery("SELECT `id`,`username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(11, "S001"))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestSubmitSupplement_AlreadyApproved(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectCompLockOK(mock)
	expectTeacherFound(mock)
	mock.ExpectQuery("SELECT `id`,`username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(11, "S001"))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id"}).AddRow(5, 1, 11))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id", "status"}).AddRow(9, 5, "approved"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "记录已存在")
}

func TestSubmitSupplement_AwardQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectCompLockOK(mock)
	expectTeacherFound(mock)
	mock.ExpectQuery("SELECT `id`,`username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(11, "S001"))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id"}).AddRow(5, 1, 11))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestSubmitSupplement_ProjectAwardsQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectCompLockOK(mock)
	expectTeacherFound(mock)
	mock.ExpectQuery("SELECT `id`,`username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(11, "S001"))
	// 无本人报名记录
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// 队员复用被驳回奖项的申报路径（覆盖 586-613 分支）后成功重新申报
func TestSubmitSupplement_TeammateResubmitRejectedAward(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectCompLockOK(mock)
	expectTeacherFound(mock)
	mock.ExpectQuery("SELECT `id`,`username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(11, "S001"))
	// 本人无报名记录 → 走项目奖项复用
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnError(gorm.ErrRecordNotFound)
	// 同名项目存在被驳回奖项（含 reg_id）
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "reg_id", "status", "project_name"}).
			AddRow(9, 1, 5, "rejected", "补录项目"))
	// 本人是该奖项成员 → 允许复用
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id"}).AddRow(5, 1, 12))
	// 成员逐个回查
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college", "major"}).
			AddRow(11, "S001", "张三", "计算机学院", "软件工程"))
	// 报名队员用户名
	mock.ExpectQuery("SELECT `username` FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("S001"))
	// 参与人回查
	mock.ExpectQuery("SELECT `id`,`username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(11, "S001"))
	// 一生一赛一奖校验
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// 证明文件归属校验
	mock.ExpectQuery("SELECT .* FROM `file_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "file_hash", "biz_type", "original_name", "storage_path", "uploader_id"}).
			AddRow(1, "0123456789abcdef0123456789abcdef", "award_proof", "proof.pdf",
				"/static/award_proofs/202606/0123456789abcdef0123456789abcdef_proof.pdf", 11))
	// 重新申报：更新奖项
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 指导教师快照
	mock.ExpectExec("DELETE FROM `award_advisors`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `award_advisors`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// 成员快照
	mock.ExpectExec("DELETE FROM `award_members`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `award_members`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE `awards` SET `members_migrated`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 报名状态回到待审核
	mock.ExpectExec("UPDATE `registers` SET `status`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "重新申报成功，待审核")
}

// 同名项目已存在其他奖项 → 拒绝合并
func TestSubmitSupplement_SameProjectConflict(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	expectCompLockOK(mock)
	expectTeacherFound(mock)
	mock.ExpectQuery("SELECT `id`,`username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(11, "S001"))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnError(gorm.ErrRecordNotFound)
	// 同名项目存在其他奖项（非 rejected 复用候选）
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "reg_id", "status", "project_name"}).
			AddRow(9, 1, 0, "draft", "补录项目"))
	// 成员逐个回查
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college", "major"}).
			AddRow(11, "S001", "张三", "计算机学院", "软件工程"))
	mock.ExpectQuery("SELECT `id`,`username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(11, "S001"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "该赛事已存在同名项目奖项")
}
