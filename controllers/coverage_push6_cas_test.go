package controllers

import (
	"context"
	"testing"

	"CompeManage_backend/database"
	"CompeManage_backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// ===================== ensureCASUserSingleRole =====================

func TestEnsureCASUserSingleRole_NilUser(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	role, err := ensureCASUserSingleRole(context.Background(), nil)
	assert.Error(t, err)
	assert.Empty(t, role)
	assert.Contains(t, err.Error(), "用户信息无效")
}

func TestEnsureCASUserSingleRole_ZeroID(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	role, err := ensureCASUserSingleRole(context.Background(), &models.User{})
	assert.Error(t, err)
	assert.Empty(t, role)
}

func TestEnsureCASUserSingleRole_AlreadySingleRole(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	user := &models.User{BaseModel: models.BaseModel{ID: 1}, Roles: []*models.Role{{RoleCode: "student"}}}

	role, err := ensureCASUserSingleRole(context.Background(), user)
	assert.NoError(t, err)
	assert.Equal(t, "student", role)
}

func TestEnsureCASUserSingleRole_MultipleRoles(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	user := &models.User{BaseModel: models.BaseModel{ID: 1}, Roles: []*models.Role{{RoleCode: "student"}, {RoleCode: "teacher"}}}

	role, err := ensureCASUserSingleRole(context.Background(), user)
	assert.Error(t, err)
	assert.Empty(t, role)
	assert.ErrorIs(t, err, database.ErrUserHasMultipleRoles)
}

func TestEnsureCASUserSingleRole_DefaultRoleMissing(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	// 无角色 + staff 身份 → 默认 teacher；角色表查询失败
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnError(gorm.ErrRecordNotFound)

	user := &models.User{BaseModel: models.BaseModel{ID: 1}, IdentityType: "staff"}
	role, err := ensureCASUserSingleRole(context.Background(), user)
	assert.Error(t, err)
	assert.Empty(t, role)
	assert.Contains(t, err.Error(), "默认角色不存在")
}

func TestEnsureCASUserSingleRole_BackfillRoleForStaff(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(3, "teacher", "老师"))
	// 事务：更新 identity_type + SetUserRole
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET `identity_type`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `user_roles`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_roles`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	user := &models.User{BaseModel: models.BaseModel{ID: 1}, IdentityType: "staff"}
	role, err := ensureCASUserSingleRole(context.Background(), user)
	assert.NoError(t, err)
	assert.Equal(t, "teacher", role)
	assert.Equal(t, "staff", user.IdentityType)
	assert.Len(t, user.Roles, 1)
}

func TestEnsureCASUserSingleRole_LegacyIdentityInferred(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	// IdentityType 为空 → 走 inferLegacyIdentityType：username 前缀 T → staff → teacher
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(3, "teacher", "老师"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET `identity_type`").
		WithArgs("staff", sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `user_roles`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_roles`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	user := &models.User{BaseModel: models.BaseModel{ID: 1}, Username: "T2023001", Grade: ""}
	role, err := ensureCASUserSingleRole(context.Background(), user)
	assert.NoError(t, err)
	assert.Equal(t, "teacher", role)
	assert.Equal(t, "staff", user.IdentityType)
}

func TestEnsureCASUserSingleRole_BackfillFails(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(4, "student", "学生"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET `identity_type`").
		WillReturnError(assertErr("db error"))
	mock.ExpectRollback()

	user := &models.User{BaseModel: models.BaseModel{ID: 1}, IdentityType: "student"}
	role, err := ensureCASUserSingleRole(context.Background(), user)
	assert.Error(t, err)
	assert.Empty(t, role)
	assert.Contains(t, err.Error(), "补齐用户默认角色失败")
}

// ===================== inferLegacyIdentityType =====================

func TestInferLegacyIdentityType(t *testing.T) {
	cases := []struct {
		name string
		user models.User
		want string
	}{
		{name: "grade staff", user: models.User{Username: "X1", Grade: "教职员工"}, want: "staff"},
		{name: "username T prefix", user: models.User{Username: "t0001", Grade: ""}, want: "staff"},
		{name: "grade with level", user: models.User{Username: "X1", Grade: "2024级"}, want: "student"},
		{name: "username S prefix", user: models.User{Username: "s0001", Grade: ""}, want: "student"},
		{name: "fallback external", user: models.User{Username: "Z9", Grade: "其他"}, want: "external"},
		{name: "lowercase t prefix", user: models.User{Username: "T123"}, want: "staff"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, inferLegacyIdentityType(tc.user))
		})
	}
}
