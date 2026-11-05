package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// 补录 body：去掉 advisors（覆盖 req.Advisors == nil 分支）
func supplementBodyNoAdvisors() map[string]interface{} {
	body := supplementBody()
	delete(body, "advisors")
	return body
}

// 通用前缀：Begin + 赛事锁定 + 指导教师解析 + 登录学生 + 无既有报名
func suppPrefixOK(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	expectCompLockOK(mock)
	expectTeacherFound(mock)
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(11, "S001"))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

// advisors 为 nil + 赛事查询失败
func TestSubmitSupplement_AdvisorsNilCompQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBodyNoAdvisors())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// 同队被退回记录：成员关联查询失败
func TestSubmitSupplement_TeammateMemberCountError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()
	suppPrefixOK(mock)
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "reg_id"}).
			AddRow(9, 1, "rejected", 5))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_members`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// 不是该获奖成员 → 跳过；随后成员学号不存在 → 400
func TestSubmitSupplement_TeammateNotMemberThenBadMember(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()
	suppPrefixOK(mock)
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "reg_id"}).
			AddRow(9, 1, "rejected", 5))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// 成员学号查询：未命中
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参赛成员学号不存在")
}

// 原报名查询失败
func TestSubmitSupplement_TeammateRegQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()
	suppPrefixOK(mock)
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "reg_id"}).
			AddRow(9, 1, "rejected", 5))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// 原申报赛事与当前不一致 → 400
func TestSubmitSupplement_TeammateRegCompMismatch(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()
	suppPrefixOK(mock)
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "reg_id"}).
			AddRow(9, 1, "rejected", 5))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(5, 2))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "原申报赛事与当前赛事不一致")
}

// 成员查询异常 → 500
func TestSubmitSupplement_MemberQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()
	suppPrefixOK(mock)
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("conn lost"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// 申报人不在成员列表 → 400
func TestSubmitSupplement_ApplicantNotInMembers(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()
	suppPrefixOK(mock)
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college", "major"}).
			AddRow(99, "S001", "张三", "计算机学院", "软件"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(supplementBody())
	c.Set("user_id", uint(11))
	SubmitStudentAwardSupplement(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "申报人必须是参赛成员")
}
