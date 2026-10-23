package controllers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 覆盖率补充 push38：上报导出与权限/基础数据接口的错误分支。

func setupReportScopeDBMock(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	previous := database.DB
	database.DB = gormDB
	db, _ := gormDB.DB()
	t.Cleanup(func() {
		db.Close()
		database.DB = previous
	})
	return mock
}

func callWithRole(role string, fn func(c *gin.Context), urlStr string) *httptest.ResponseRecorder {
	_, w, c := buildGET(urlStr)
	c.Set("user_id", uint(1))
	if role != "" {
		c.Set("role_code", role)
	}
	fn(c)
	return w
}

func TestAwardExportCategory_Fallbacks(t *testing.T) {
	require.Equal(t, "国家级", awardExportCategory("国际级", ""))
	require.Equal(t, "国家级", awardExportCategory("", "国际级金奖"))
	require.Equal(t, "省级", awardExportCategory("省部级", ""))
	require.Equal(t, "校赛", awardExportCategory("校级", ""))
	require.Equal(t, "", awardExportCategory("", ""))
	require.Equal(t, "省级", awardExportCategory("", "省级二等奖"))
}

func TestAwardExportScope_RejectsStudentAndCollegelessManager(t *testing.T) {
	// 学生：默认分支 403
	w := callWithRole("student", ExportAwardProjects, "/api/award/export/projects")
	require.Equal(t, http.StatusForbidden, w.Code)

	// 赛事负责人未配置学院：403（users 查询返回空学院）
	mock := setupReportScopeDBMock(t)
	mock.ExpectQuery("SELECT `college` FROM `users`").WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"college"}).AddRow(""))
	_, w2, c := buildGET("/api/award/export/projects")
	c.Set("user_id", uint(1))
	c.Set("role_code", "competition_manager")
	ExportAwardProjects(c)
	require.Equal(t, http.StatusForbidden, w2.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestExportAwardReport_BadYear(t *testing.T) {
	w := callWithRole("school_admin", ExportAwardReport, "/api/award/export/report?year=20ab")
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExportAwardReport_ProjectsQueryError(t *testing.T) {
	mock := setupReportScopeDBMock(t)
	mock.ExpectQuery("SELECT `college` FROM `users`").WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"college"}).AddRow("教务处"))
	mock.ExpectQuery("SELECT .*FROM `awards`").WillReturnError(errors.New("db down"))
	w := callWithRole("school_admin", ExportAwardReport, "/api/award/export/report")
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAllRoles_DBError(t *testing.T) {
	mock := setupReportScopeDBMock(t)
	mock.ExpectQuery("SELECT .*FROM `roles`").WillReturnError(errors.New("db down"))
	w := callWithRole("", GetAllRoles, "/api/perm/role/list")
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDepartmentList_DBError(t *testing.T) {
	mock := setupReportScopeDBMock(t)
	mock.ExpectQuery("SELECT .*FROM `departments`").WillReturnError(errors.New("db down"))
	w := callWithRole("", GetDepartmentList, "/api/department/list")
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetCollegeList_QueryError(t *testing.T) {
	mock := setupReportScopeDBMock(t)
	mock.ExpectQuery("SELECT .*FROM `colleges`").WillReturnError(errors.New("db down"))
	w := callWithRole("", GetCollegeList, "/api/college/list")
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
