package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestGetAwardImportSetting2_ForbiddenRole(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/award/import/setting")
	c.Set("user_id", uint(1))
	c.Set("role_code", "teacher")
	GetAwardImportSetting(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "没有填报权限")
}

func TestGetAwardImportSetting2_Success(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_import_settings`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "require_registration"}).AddRow(1, true))

	_, w, c := buildGET("/api/award/import/setting")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetAwardImportSetting(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetAwardImportSetting2_ReadError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_import_settings`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/award/import/setting")
	c.Set("user_id", uint(1))
	c.Set("role_code", "competition_manager")
	GetAwardImportSetting(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
