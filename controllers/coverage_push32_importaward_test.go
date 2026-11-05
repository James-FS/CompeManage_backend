package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func importPOST(t *testing.T, url string, body interface{}) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		jsonBytes, _ := json.Marshal(body)
		buf.Write(jsonBytes)
	}
	req := httptest.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return w, c
}

func importRowBody() map[string]interface{} {
	return map[string]interface{}{"rows": []AwardImportRow{importRow("S001", "项目A")}}
}

func TestImportAward_ReadError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺 comp_id
	w, c := importPOST(t, "/api/award/import", importRowBody())
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	ImportAward(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportAward_SettingError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `award_import_settings`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	w, c := importPOST(t, "/api/award/import?comp_id=1", importRowBody())
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	ImportAward(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestImportAward_CompLockError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `award_import_settings`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "require_registration"}).AddRow(1, false))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnError(fmt.Errorf("lock fail"))
	mock.ExpectRollback()

	w, c := importPOST(t, "/api/award/import?comp_id=1", importRowBody())
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	ImportAward(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// 提交时权限已变化（非本人负责赛事的 competition_manager）
func TestImportAward_ScopeChanged(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 9, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `award_import_settings`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "require_registration"}).AddRow(1, false))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id", "comp_level"}).AddRow(1, 9, 1, "省级"))
	mock.ExpectRollback()

	w, c := importPOST(t, "/api/award/import?comp_id=1", importRowBody())
	c.Set("user_id", uint(1))
	c.Set("role_code", "competition_manager")
	ImportAward(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// 校验查询失败 → 整体回滚
func TestImportAward_ValidateError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `award_import_settings`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "require_registration"}).AddRow(1, false))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id", "comp_level"}).AddRow(1, 1, 1, "校级"))
	mock.ExpectQuery("SELECT users.id, users.username, users.realname, users.college, users.major, requested.lookup_username").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	w, c := importPOST(t, "/api/award/import?comp_id=1", importRowBody())
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	ImportAward(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
