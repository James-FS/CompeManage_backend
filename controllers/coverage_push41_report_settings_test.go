package controllers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 覆盖率补充 push41：上报导出错误/成功链路、导入设置写入、Preview 错误分支。

var errPush41DB = errors.New("push41 db error")

func uPtr(v uint) *uint { return &v }

func TestExportAwardReport_StudentsQueryError(t *testing.T) {
	mock := setupReportScopeDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectQuery("SELECT `college` FROM `users`").WillReturnRows(sqlmock.NewRows([]string{"college"}).AddRow("计算机学院"))
	mock.ExpectQuery("SELECT .* FROM `awards` JOIN comp_directories").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM award_members am JOIN").WillReturnError(errPush41DB)

	w := callWithRole("school_admin", ExportAwardReport, "/api/award/export/report")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestExportAwardReport_SummaryQueryError(t *testing.T) {
	mock := setupReportScopeDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectQuery("SELECT `college` FROM `users`").WillReturnRows(sqlmock.NewRows([]string{"college"}).AddRow("计算机学院"))
	mock.ExpectQuery("SELECT .* FROM `awards` JOIN comp_directories").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM award_members am JOIN").WillReturnRows(sqlmock.NewRows([]string{"student_number"}))
	mock.ExpectQuery("SELECT .* awards a .*JOIN").WillReturnError(errPush41DB)

	w := callWithRole("school_admin", ExportAwardReport, "/api/award/export/report")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestExportAwardReport_EmptyDataXlsx(t *testing.T) {
	mock := setupReportScopeDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectQuery("SELECT `college` FROM `users`").WillReturnRows(sqlmock.NewRows([]string{"college"}).AddRow("计算机学院"))
	mock.ExpectQuery("SELECT .* FROM `awards` JOIN comp_directories").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM award_members am JOIN").WillReturnRows(sqlmock.NewRows([]string{"student_number"}))
	mock.ExpectQuery("SELECT .* awards a .*JOIN").WillReturnRows(sqlmock.NewRows([]string{"comp_name"}))

	w := callWithRole("school_admin", ExportAwardReport, "/api/award/export/report?year=2026")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "spreadsheetml")
	assert.Contains(t, w.Header().Get("Content-Disposition"), "2026")
	assert.NotEmpty(t, w.Body.Bytes())
}

// ---- 读取校验失败名单（Preview）错误分支 ----

func TestPreviewAwardImport_ReadErrorBranches(t *testing.T) {
	// comp_id 无效
	w := callWithRole("school_admin", PreviewAwardImport, "/api/award/import/preview")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// JSON 空行
	mock := setupReportScopeDBMock(t)
	_, w2, c2 := buildAuthPOSTJSON(map[string]any{"rows": []any{}})
	c2.Set("user_id", uint(1))
	c2.Set("role_code", "school_admin")
	c2.Request.URL.RawQuery = "comp_id=1"
	PreviewAwardImport(c2)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
	assert.Contains(t, w2.Body.String(), "每批请提交1至5000行")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPreviewAwardImport_CompMissingAndSettingError(t *testing.T) {
	// 赛事不存在 → Forbidden
	mock := setupReportScopeDBMock(t)
	_, w, c := buildAuthPOSTJSON(map[string]any{"rows": []any{map[string]any{"student_number": "S1"}}})
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Request.URL.RawQuery = "comp_id=7"
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	PreviewAwardImport(c)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在或不在可访问范围")
	require.NoError(t, mock.ExpectationsWereMet())

	// 读取设置失败 → 500
	mock2 := setupReportScopeDBMock(t)
	_, w2, c2 := buildAuthPOSTJSON(map[string]any{"rows": []any{map[string]any{"student_number": "S1"}}})
	c2.Set("user_id", uint(1))
	c2.Set("role_code", "school_admin")
	c2.Request.URL.RawQuery = "comp_id=7"
	mock2.ExpectQuery("SELECT .* FROM `comp_directories`").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock2.ExpectQuery("SELECT .* FROM `award_import_settings`").WillReturnError(errPush41DB)
	PreviewAwardImport(c2)
	assert.Equal(t, http.StatusInternalServerError, w2.Code)
	require.NoError(t, mock2.ExpectationsWereMet())
}

// ---- 导入设置写入分支 ----

func TestUpdateAwardImportSetting_Guards(t *testing.T) {
	// 非校管理员 → Forbidden
	_, w, c := buildAuthPOSTJSON(map[string]any{"require_registration": true})
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin")
	UpdateAwardImportSetting(c)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 缺参 → BadRequest
	_, w2, c2 := buildAuthPOSTJSON(map[string]any{})
	c2.Set("user_id", uint(1))
	c2.Set("role_code", "school_admin")
	UpdateAwardImportSetting(c2)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestUpdateAwardImportSetting_LockErrorAndSave(t *testing.T) {
	// 锁查询非 NotFound 错误 → 500
	mock := setupReportScopeDBMock(t)
	_, w, c := buildAuthPOSTJSON(map[string]any{"require_registration": true})
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `award_import_settings`").WillReturnError(errPush41DB)
	mock.ExpectRollback()
	UpdateAwardImportSetting(c)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	// NotFound → Save 成功
	mock2 := setupReportScopeDBMock(t)
	_, w2, c2 := buildAuthPOSTJSON(map[string]any{"require_registration": false})
	c2.Set("user_id", uint(1))
	c2.Set("role_code", "school_admin")
	mock2.ExpectBegin()
	mock2.ExpectQuery("SELECT .* FROM `award_import_settings`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock2.ExpectExec("UPDATE `award_import_settings`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock2.ExpectCommit()
	UpdateAwardImportSetting(c2)
	assert.Equal(t, http.StatusOK, w2.Code)
	require.NoError(t, mock2.ExpectationsWereMet())

	// Save 出错 → 500
	mock3 := setupReportScopeDBMock(t)
	_, w3, c3 := buildAuthPOSTJSON(map[string]any{"require_registration": true})
	c3.Set("user_id", uint(1))
	c3.Set("role_code", "school_admin")
	mock3.ExpectBegin()
	mock3.ExpectQuery("SELECT .* FROM `award_import_settings`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock3.ExpectExec("UPDATE `award_import_settings`").WillReturnError(errPush41DB)
	mock3.ExpectRollback()
	UpdateAwardImportSetting(c3)
	assert.Equal(t, http.StatusInternalServerError, w3.Code)
	require.NoError(t, mock3.ExpectationsWereMet())
}
