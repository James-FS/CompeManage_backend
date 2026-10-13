package database

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCollegeManagerPermissionSeedPreservesRevocations(t *testing.T) {
	previous := DB
	mock := setupUserRoleMigrationDBMock(t)
	t.Cleanup(func() { DB = previous })
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `roles`").WithArgs("college_admin", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	for _, code := range []string{"comp:update:own", "reg:config:edit:own", "summary:edit:own"} {
		mock.ExpectQuery("SELECT .* FROM `permissions`").WithArgs(code, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(20, code))
	}
	// No grant or permission-version write when permission rows already exist.
	mock.ExpectCommit()
	ensureCollegeManagerPermissions()
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollegeManagerPermissionSeedCreatesGrantsAtomically(t *testing.T) {
	previous := DB
	mock := setupUserRoleMigrationDBMock(t)
	t.Cleanup(func() { DB = previous })
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `roles`").WithArgs("college_admin", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	for i, item := range []struct{ code, parent string }{
		{"comp:update:own", "comp"}, {"reg:config:edit:own", "reg:config"}, {"summary:edit:own", "summary"},
	} {
		mock.ExpectQuery("SELECT .* FROM `permissions`").WithArgs(item.code, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectQuery("SELECT .* FROM `permissions`").WithArgs(item.parent, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(i + 10))
		mock.ExpectExec("INSERT INTO `permissions`").WillReturnResult(sqlmock.NewResult(int64(i+20), 1))
		mock.ExpectExec("INSERT INTO `role_permissions`").WithArgs(2, i+20).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("UPDATE `roles` SET `permission_version`=permission_version \\+ 1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	ensureCollegeManagerPermissions()
	require.NoError(t, mock.ExpectationsWereMet())
}
