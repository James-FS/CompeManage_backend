package controllers

import (
	"net/http"
	"testing"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupCollegeDBMock(t *testing.T) sqlmock.Sqlmock {
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

// ===================== GetCollegeList =====================

func TestGetCollegeList_Success(t *testing.T) {
	mock := setupCollegeDBMock(t)
	t.Cleanup(func() { assert.NoError(t, mock.ExpectationsWereMet()) })

	mock.ExpectQuery("SELECT .* FROM `colleges` WHERE is_valid = ").WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename"}).
			AddRow(1, "计算机学院", "CS", "School of Computer Science").
			AddRow(2, "数学学院", "MATH", "School of Mathematics"))

	_, w, c := buildGET("/api/college/list")
	GetCollegeList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "计算机学院")
	assert.Contains(t, w.Body.String(), "数学学院")
}

func TestGetCollegeList_EmptyList(t *testing.T) {
	mock := setupCollegeDBMock(t)
	t.Cleanup(func() { assert.NoError(t, mock.ExpectationsWereMet()) })

	mock.ExpectQuery("SELECT .* FROM `colleges` WHERE is_valid = ").WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename"}))

	_, w, c := buildGET("/api/college/list")
	GetCollegeList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetCollegeList_DBError(t *testing.T) {
	mock := setupCollegeDBMock(t)
	t.Cleanup(func() { assert.NoError(t, mock.ExpectationsWereMet()) })

	mock.ExpectQuery("SELECT .* FROM `colleges` WHERE is_valid = ").WithArgs(true).
		WillReturnError(assert.AnError)

	_, w, c := buildGET("/api/college/list")
	GetCollegeList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestGetDepartmentList_OnlyValid(t *testing.T) {
	mock := setupCollegeDBMock(t)
	t.Cleanup(func() { assert.NoError(t, mock.ExpectationsWereMet()) })
	mock.ExpectQuery("SELECT .* FROM `departments` WHERE is_valid = ").WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename", "is_valid"}).
			AddRow(1, "审计处", "0002", "Audit Office", true))
	_, w, c := buildGET("/api/department/list")
	GetDepartmentList(c)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "审计处")
	assert.Contains(t, w.Body.String(), `"is_valid":true`)
}
