package database

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestBuildMigrationRoleDecisions_QueryErrors(t *testing.T) {
	// users 查询失败
	mock := setupUserDBMock(t)
	mock.ExpectQuery("SELECT .* FROM `users`").WillReturnError(fmt.Errorf("e1"))
	_, err := buildMigrationRoleDecisions([]UserRoleAnomaly{{UserID: 1}})
	assert.ErrorContains(t, err, "e1")
	mock.ExpectationsWereMet()

	// roles 查询失败
	mock = setupUserDBMock(t)
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).AddRow(1, "U1", "staff"))
	mock.ExpectQuery("SELECT .* FROM `roles`").WillReturnError(fmt.Errorf("e2"))
	_, err = buildMigrationRoleDecisions([]UserRoleAnomaly{{UserID: 1}})
	assert.ErrorContains(t, err, "e2")
	mock.ExpectationsWereMet()

	// user_roles 查询失败
	mock = setupUserDBMock(t)
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).AddRow(1, "U1", "staff"))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "teacher"))
	mock.ExpectQuery("SELECT .* FROM `user_roles`").WillReturnError(fmt.Errorf("e3"))
	_, err = buildMigrationRoleDecisions([]UserRoleAnomaly{{UserID: 1}})
	assert.ErrorContains(t, err, "e3")
	mock.ExpectationsWereMet()

	// 赛事负责人查询失败
	mock = setupUserDBMock(t)
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).AddRow(1, "U1", "staff"))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "teacher"))
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 2))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").WillReturnError(fmt.Errorf("e4"))
	_, err = buildMigrationRoleDecisions([]UserRoleAnomaly{{UserID: 1}})
	assert.ErrorContains(t, err, "e4")
	mock.ExpectationsWereMet()

	// 申报负责人查询失败
	mock = setupUserDBMock(t)
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).AddRow(1, "U1", "staff"))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "teacher"))
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 2))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"manager_id"}))
	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").WillReturnError(fmt.Errorf("e5"))
	_, err = buildMigrationRoleDecisions([]UserRoleAnomaly{{UserID: 1}})
	assert.ErrorContains(t, err, "e5")
	mock.ExpectationsWereMet()
}

// 无可保留角色 → 明确报错
func TestBuildMigrationRoleDecisions_NoValidRole(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).AddRow(1, "U1", ""))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}))
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"manager_id"}))
	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"manager_id"}))

	_, err := buildMigrationRoleDecisions([]UserRoleAnomaly{{UserID: 1}})
	assert.ErrorContains(t, err, "没有可保留的有效角色")
}
