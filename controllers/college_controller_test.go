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
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `colleges`").
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
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename"}))

	_, w, c := buildGET("/api/college/list")
	GetCollegeList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetCollegeList_DBError(t *testing.T) {
	mock := setupCollegeDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnError(assert.AnError)

	_, w, c := buildGET("/api/college/list")
	GetCollegeList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}
