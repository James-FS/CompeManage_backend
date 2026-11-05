package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// ===================== SearchCompetition =====================

func TestSearchCompetition_CollegeAdminScopeInvalid(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/award/comp/list?keyword=建模")
	c.Set("role_code", "college_admin") // 未配置管理学院
	c.Set("user_id", uint(1))
	SearchCompetition(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSearchCompetition_PaginatedPageDefault(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT `id`,`comp_name`,`comp_code`,`comp_level`,`year` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name", "comp_code", "comp_level", "year"}).
			AddRow(1, "建模大赛", "X2026001", "校级", 2026))

	_, w, c := buildGET("/api/award/comp/list?keyword=建模&paginated=true&page=0&year=2026")
	SearchCompetition(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "建模大赛")
}

func TestSearchCompetition_PaginatedCountError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/award/comp/list?keyword=建模&paginated=true")
	SearchCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSearchCompetition_PaginatedFindError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT `id`,`comp_name`,`comp_code`,`comp_level`,`year` FROM `comp_directories`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/award/comp/list?keyword=建模&paginated=true")
	SearchCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSearchCompetition_NonPaginatedFindError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`comp_name`,`year` FROM `comp_directories`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/award/comp/list?keyword=建模")
	SearchCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== LookupAwardAdvisors =====================

func TestLookupAwardAdvisors_BlankUsernames(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// 全部为空白 → 归一后为空列表
	body := map[string]interface{}{"usernames": []string{"   "}}
	_, w, c := buildAuthPOSTJSON(body)
	LookupAwardAdvisors(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"list\":[]")
}

func TestLookupAwardAdvisors_FindError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT users.id, users.realname, users.username, users.college, users.grade FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	body := map[string]interface{}{"usernames": []string{"T001"}}
	_, w, c := buildAuthPOSTJSON(body)
	LookupAwardAdvisors(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestLookupAwardAdvisors_RoleQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT users.id, users.realname, users.username, users.college, users.grade FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college", "grade"}).
			AddRow(3, "王指导", "T003", "计算机学院", "教职员工"))
	mock.ExpectQuery("SELECT user_roles.user_id AS user_id, roles.role_code AS role_code FROM `user_roles`").
		WillReturnError(fmt.Errorf("db error"))

	body := map[string]interface{}{"usernames": []string{"T003"}}
	_, w, c := buildAuthPOSTJSON(body)
	LookupAwardAdvisors(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestLookupAwardAdvisors_DefaultTeacherRole(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT users.id, users.realname, users.username, users.college, users.grade FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college", "grade"}).
			AddRow(3, "王指导", "T003", "计算机学院", "教职员工"))
	mock.ExpectQuery("SELECT user_roles.user_id AS user_id, roles.role_code AS role_code FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_code"}))

	body := map[string]interface{}{"usernames": []string{"T003"}}
	_, w, c := buildAuthPOSTJSON(body)
	LookupAwardAdvisors(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"role_code\":\"teacher\"")
}

// ===================== GetCompAwards =====================

func TestGetCompAwards_FindError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/award/comp-awards?comp_id=1")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetCompAwards(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetCompAwards_CollegesError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	submitted := uint(5)
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "reg_id", "project_name", "status"}).
			AddRow(9, 1, 0, "项目A", "approved"))
	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "name", "student_number", "college", "major", "remark", "submitted_college_id", "submitted_by", "student_id"}).
			AddRow(1, 9, "张三", "S001", "计算机学院", "软件", "", &submitted, 1, 11))
	mock.ExpectQuery("SELECT .* FROM `award_advisors`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/award/comp-awards?comp_id=1")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetCompAwards(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetCompAwards_ProjectFallbackToRegister(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "reg_id", "project_name", "status", "award_time"}).
			AddRow(9, 1, 5, "", "approved", nil))
	// Preload：Members/Advisors 为空；Register（含 TeamName）+ Register.Members
	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `award_advisors`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "team_name", "leader_id"}).AddRow(5, 1, "原报名项目", 0))
	mock.ExpectQuery("SELECT .* FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/award/comp-awards?comp_id=1")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetCompAwards(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "原报名项目")
}
