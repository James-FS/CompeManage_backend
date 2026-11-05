package controllers

import (
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ===================== GetAwardChanges =====================

func TestGetAwardChanges_InvalidID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	c, w := advisorTestContext("GET", "school_admin", nil)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	GetAwardChanges(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "无效的获奖ID")
}

func TestGetAwardChanges_AwardNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnError(assertErr("record not found"))

	c, w := advisorTestContext("GET", "school_admin", nil)
	GetAwardChanges(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "获奖记录不存在")
}

func TestGetAwardChanges_Unauthorized(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(7, 1))

	c, w := advisorTestContext("GET", "school_admin", nil)
	// 覆盖 user_id 以模拟未登录
	c.Set("user_id", nil)
	GetAwardChanges(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Body.String(), "请先登录")
}

func TestGetAwardChanges_StudentOwnAward(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(7, 1))
	// 学生权限校验：命中 1 条
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `award_changes`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id"}).
			AddRow(1, 7))

	c, w := advisorTestContext("GET", "student", nil)
	GetAwardChanges(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "\"list\"")
}

func TestGetAwardChanges_StudentForbidden(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(7, 1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	c, w := advisorTestContext("GET", "student", nil)
	GetAwardChanges(c)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "只能查看自己的获奖修改记录")
}

func TestGetAwardChanges_StudentScopeQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(7, 1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnError(assertErr("db error"))

	c, w := advisorTestContext("GET", "student", nil)
	GetAwardChanges(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestGetAwardChanges_TeacherGuidesAward(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(7, 1))
	// requireAwardAdvisorUser
	expectAdvisorUser(mock)
	// 指导关系命中
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `award_changes`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id"}))

	c, w := advisorTestContext("GET", "teacher", nil)
	GetAwardChanges(c)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestGetAwardChanges_TeacherNotGuidingForbidden(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(7, 1))
	expectAdvisorUser(mock)
	// 指导关系未命中
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	c, w := advisorTestContext("GET", "teacher", nil)
	GetAwardChanges(c)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "只能查看自己指导的项目")
}

func TestGetAwardChanges_AdvisorScopeQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(7, 1))
	expectAdvisorUser(mock)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnError(assertErr("db error"))

	c, w := advisorTestContext("GET", "teacher", nil)
	GetAwardChanges(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetAwardChanges_CompetitionManagerFallbackToCompScope(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(7, 1))
	expectAdvisorUser(mock)
	// 指导关系未命中 → 回落到赛事作用域（school_admin 快路径）
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `comp_directories` WHERE comp_directories.manager_id = \\?").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 11, 1))
	mock.ExpectQuery("SELECT \\* FROM `award_changes`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id"}))

	c, w := advisorTestContext("GET", "competition_manager", nil)
	GetAwardChanges(c)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestGetAwardChanges_ChangesQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(7, 1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `award_changes`").
		WillReturnError(assertErr("db error"))

	c, w := advisorTestContext("GET", "student", nil)
	GetAwardChanges(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestGetAwardChanges_DefaultBranchCompetitionScopeDenied(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(7, 1))
	// 默认分支（如 expert）走 requireAwardCompetition → 查 comp_directories
	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 99, 1))

	c, w := advisorTestContext("GET", "expert", nil)
	GetAwardChanges(c)

	require.Equal(t, http.StatusForbidden, w.Code)
}
