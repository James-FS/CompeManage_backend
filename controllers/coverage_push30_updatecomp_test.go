package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func compUpdateBody() map[string]interface{} {
	return map[string]interface{}{
		"comp_name": "新名字", "comp_level": "校级", "comp_type": "个人赛",
		"organizer": "教务处", "undertaker": "学院", "manager_id": 5,
		"college": "", "year": "2026", "desc": "描述",
	}
}

func TestUpdateCompetition2_ScopeInvalid(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))

	_, w, c := buildAuthPOSTJSON(compUpdateBody())
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin") // 未配置管理学院
	UpdateCompetition(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestUpdateCompetition2_WriteDenied(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))

	_, w, c := buildAuthPOSTJSON(compUpdateBody())
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "teacher")
	c.Set("permission_codes", []string{}) // 无 comp:update
	UpdateCompetition(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestUpdateCompetition2_ManagerValidateError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(compUpdateBody())
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"comp:update"})
	UpdateCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateCompetition2_UpdatesError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectBegin()
	// validateCompetitionManager：users → user_roles → roles
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college"}).AddRow(5, "T005", "王老师", "计算机学院"))
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(5, 3))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(3, "teacher", "老师"))
	mock.ExpectExec("UPDATE `comp_directories` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(compUpdateBody())
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"comp:update"})
	UpdateCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
