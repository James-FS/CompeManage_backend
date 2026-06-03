package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupPermDBMock(t *testing.T) sqlmock.Sqlmock {
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

// ===================== GetAllPermissions =====================

func TestGetAllPermissions_Success(t *testing.T) {
	mock := setupPermDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `permissions`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "type"}).
			AddRow(1, "用户列表", "user:list", 1).
			AddRow(2, "用户创建", "user:create", 2))

	_, w, c := buildGET("/api/perm/permissions")
	GetAllPermissions(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "获取成功")
	assert.Contains(t, w.Body.String(), "user:list")
	assert.Contains(t, w.Body.String(), "user:create")
}

func TestGetAllPermissions_EmptyList(t *testing.T) {
	mock := setupPermDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `permissions`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "type"}))

	_, w, c := buildGET("/api/perm/permissions")
	GetAllPermissions(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "获取成功")
}

func TestGetAllPermissions_DBError(t *testing.T) {
	mock := setupPermDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `permissions`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/perm/permissions")
	GetAllPermissions(c)

	// Find 不返回 error，所以即使 DB 出错也会返回 200
	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== GetAllRoles =====================

func TestGetAllRoles_Success(t *testing.T) {
	mock := setupPermDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_name", "role_code"}).
			AddRow(1, "管理员", "admin").
			AddRow(2, "学生", "student"))
	// Preload Permissions: 多对多关系先查中间表，再查目标表
	mock.ExpectQuery("SELECT .* FROM `role_permissions`").
		WillReturnRows(sqlmock.NewRows([]string{"role_id", "permission_id"}).
			AddRow(1, 1).
			AddRow(1, 2))
	mock.ExpectQuery("SELECT .* FROM `permissions`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code"}).
			AddRow(1, "用户列表", "user:list").
			AddRow(2, "用户创建", "user:create"))

	_, w, c := buildGET("/api/perm/roles")
	GetAllRoles(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "获取成功")
	assert.Contains(t, w.Body.String(), "admin")
	assert.Contains(t, w.Body.String(), "user:list")
}

func TestGetAllRoles_EmptyList(t *testing.T) {
	mock := setupPermDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_name", "role_code"}))

	_, w, c := buildGET("/api/perm/roles")
	GetAllRoles(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "获取成功")
}

// ===================== AssignPermissions =====================

func TestAssignPermissions_BadRequest(t *testing.T) {
	mock := setupPermDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺少 role_id
	body := map[string]interface{}{"perm_ids": []uint{1, 2}}
	_, w, c := buildAuthPOSTJSON(body)
	AssignPermissions(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestAssignPermissions_BadRequest_EmptyBody(t *testing.T) {
	mock := setupPermDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{}
	_, w, c := buildAuthPOSTJSON(body)
	AssignPermissions(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAssignPermissions_RoleNotFound(t *testing.T) {
	mock := setupPermDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := AssignPermReq{RoleID: 999, PermIDs: []uint{1}}
	_, w, c := buildAuthPOSTJSON(body)
	AssignPermissions(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "角色不存在")
}

func TestAssignPermissions_Success_WithPerms(t *testing.T) {
	mock := setupPermDBMock(t)
	defer mock.ExpectationsWereMet()

	// 查找角色
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_name", "role_code"}).AddRow(1, "管理员", "admin"))
	// 查找权限
	mock.ExpectQuery("SELECT .* FROM `permissions`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code"}).
			AddRow(1, "用户列表", "user:list").
			AddRow(2, "用户创建", "user:create"))
	// Association Replace: 更新角色时间戳 + 删除旧关联 + 插入新关联
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `roles`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM `role_permissions`").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO `role_permissions`").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	body := AssignPermReq{RoleID: 1, PermIDs: []uint{1, 2}}
	_, w, c := buildAuthPOSTJSON(body)
	AssignPermissions(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "权限分配成功")
}

func TestAssignPermissions_Success_EmptyPerms(t *testing.T) {
	mock := setupPermDBMock(t)
	defer mock.ExpectationsWereMet()

	// 查找角色
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_name", "role_code"}).AddRow(1, "管理员", "admin"))
	// perm_ids 为空，不查询权限
	// Association Replace: 更新角色时间戳 + 删除旧关联
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `roles`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM `role_permissions`").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	body := AssignPermReq{RoleID: 1, PermIDs: []uint{}}
	_, w, c := buildAuthPOSTJSON(body)
	AssignPermissions(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "权限分配成功")
}

func TestAssignPermissions_RoleDBError(t *testing.T) {
	mock := setupPermDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnError(fmt.Errorf("db error"))

	body := AssignPermReq{RoleID: 1, PermIDs: []uint{1}}
	_, w, c := buildAuthPOSTJSON(body)
	AssignPermissions(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "角色不存在")
}
