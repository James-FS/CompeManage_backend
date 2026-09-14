package database

import (
	"testing"

	"CompeManage_backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupUserRoleMigrationDBMock(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	assert.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	assert.NoError(t, err)
	DB = gormDB
	return mock
}

func TestInferMigrationIdentityType(t *testing.T) {
	tests := []struct {
		name string
		user models.User
		want string
	}{
		{name: "staff by grade", user: models.User{Grade: "教职员工"}, want: "staff"},
		{name: "staff by username", user: models.User{Username: "T10001"}, want: "staff"},
		{name: "student by grade", user: models.User{Grade: "2025级"}, want: "student"},
		{name: "student by username", user: models.User{Username: "S2025001"}, want: "student"},
		{name: "external fallback", user: models.User{Username: "guest"}, want: "external"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, inferMigrationIdentityType(tt.user))
		})
	}
}

func TestChooseMigrationRole_DoesNotPromoteAllStaffToCompetitionManager(t *testing.T) {
	roles := map[uint]models.Role{
		1: {ID: 1, RoleCode: "competition_manager"},
		2: {ID: 2, RoleCode: "teacher"},
	}
	links := []models.UserRole{{UserID: 10, RoleID: 1}, {UserID: 10, RoleID: 2}}
	user := models.User{IdentityType: "staff"}

	withoutManagedCompetition := chooseMigrationRole(links, roles, user, false)
	assert.Equal(t, "teacher", withoutManagedCompetition.RoleCode)
	assert.Contains(t, withoutManagedCompetition.Reason, "未发现负责赛事")

	withManagedCompetition := chooseMigrationRole(links, roles, user, true)
	assert.Equal(t, "competition_manager", withManagedCompetition.RoleCode)
}

func TestChooseMigrationRole_PreservesAdministrativeRoles(t *testing.T) {
	roles := map[uint]models.Role{
		1: {ID: 1, RoleCode: "school_admin"},
		2: {ID: 2, RoleCode: "competition_manager"},
		3: {ID: 3, RoleCode: "teacher"},
	}
	links := []models.UserRole{{UserID: 1, RoleID: 2}, {UserID: 1, RoleID: 1}, {UserID: 1, RoleID: 3}}

	decision := chooseMigrationRole(links, roles, models.User{IdentityType: "staff"}, false)
	assert.Equal(t, "school_admin", decision.RoleCode)
}

func expectRollbackDataRestore(mock sqlmock.Sqlmock, backupTable string) {
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM user_roles").WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("INSERT INTO user_roles SELECT \\* FROM " + backupTable).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("UPDATE users.*JOIN " + backupTable + "_users").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
}

func expectUniqueIndexLookup(mock sqlmock.Sqlmock, exists bool) {
	count := 0
	if exists {
		count = 1
	}
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM information_schema.statistics").
		WithArgs(userRoleUniqueIndex).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func TestRollbackUserRoleMigration_PreservesPreexistingUniqueIndex(t *testing.T) {
	mock := setupUserRoleMigrationDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()

	mock.ExpectQuery("SELECT had_unique_index FROM backup_20260712_meta LIMIT 1").
		WillReturnRows(sqlmock.NewRows([]string{"had_unique_index"}).AddRow(true))
	expectUniqueIndexLookup(mock, true)
	expectRollbackDataRestore(mock, "backup_20260712")

	assert.NoError(t, RollbackUserRoleMigration("backup_20260712"))
}

func TestRollbackUserRoleMigration_DropsMigrationCreatedUniqueIndex(t *testing.T) {
	mock := setupUserRoleMigrationDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()

	mock.ExpectQuery("SELECT had_unique_index FROM backup_20260712_meta LIMIT 1").
		WillReturnRows(sqlmock.NewRows([]string{"had_unique_index"}).AddRow(false))
	expectUniqueIndexLookup(mock, true)
	mock.ExpectExec("ALTER TABLE user_roles DROP INDEX " + userRoleUniqueIndex).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectRollbackDataRestore(mock, "backup_20260712")

	assert.NoError(t, RollbackUserRoleMigration("backup_20260712"))
}

func TestRollbackUserRoleMigration_RejectsUnsafeBackupName(t *testing.T) {
	assert.Error(t, RollbackUserRoleMigration("backup;DROP_TABLE"))
}
