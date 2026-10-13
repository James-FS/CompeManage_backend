package database

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestEnsureAwardStudentPermissionsGrantsManagerAuditAndInvalidatesCache(t *testing.T) {
	previousDB := DB
	mock := setupUserRoleMigrationDBMock(t)
	t.Cleanup(func() { DB = previousDB })
	mock.ExpectQuery("SELECT .* FROM `permissions`").WithArgs("award", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(1, "award"))
	for _, code := range []string{"award:student:my-list", "award:student:supplement", "award:audit", "award:import"} {
		mock.ExpectQuery("SELECT .* FROM `permissions`").WithArgs(code, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(2, code))
	}
	for _, role := range []string{"student", "school_admin", "college_admin"} {
		mock.ExpectQuery("SELECT .* FROM `roles`").WithArgs(role, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}))
	}
	mock.ExpectQuery("SELECT .* FROM `roles`").WithArgs("competition_manager", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "permission_version"}).AddRow(3, "competition_manager", 1))
	mock.ExpectQuery("SELECT .* FROM `permissions`").WithArgs("award:audit").
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(2, "award:audit"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `roles` SET `permission_version`=permission_version \\+ 1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `roles` SET `update_time`=").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `permissions`").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("INSERT INTO `role_permissions`").WithArgs(3, 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT .* FROM `roles`").WithArgs("college_admin", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}))
	ensureAwardStudentPermissions()
	require.NoError(t, mock.ExpectationsWereMet())
}
