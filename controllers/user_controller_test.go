package controllers

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"CompeManage_backend/database"
	"CompeManage_backend/middleware"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupUserManageDBMock(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	assert.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	assert.NoError(t, err)
	database.DB = gormDB
	return mock
}

func buildAssignRoleContext(t *testing.T, targetID string, operatorID *uint, body AssignUserRoleReq) (int, string) {
	t.Helper()
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = gin.Params{{Key: "id", Value: targetID}}
	if operatorID != nil {
		c.Set("user_id", *operatorID)
	}
	AssignUserRole(c)
	return w.Code, w.Body.String()
}

func TestAssignUserRole_InvalidTargetID(t *testing.T) {
	status, body := buildAssignRoleContext(t, "abc", nil, AssignUserRoleReq{RoleID: 1})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, "用户ID无效")
}

func TestAssignUserRole_RequiresOperator(t *testing.T) {
	status, body := buildAssignRoleContext(t, "2", nil, AssignUserRoleReq{RoleID: 1})
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Contains(t, body, "未登录")
}

func TestAssignUserRole_RejectsEmptyRole(t *testing.T) {
	operatorID := uint(1)
	status, body := buildAssignRoleContext(t, "2", &operatorID, AssignUserRoleReq{})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, "参数错误")
}

func TestAssignUserRole_TargetNotFound(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `users`.*FOR UPDATE").
		WithArgs(uint(99), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	operatorID := uint(1)
	status, body := buildAssignRoleContext(t, "99", &operatorID, AssignUserRoleReq{RoleID: 3})
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, body, "用户不存在")
}

func TestAssignUserRole_RoleNotFound(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `users`.*FOR UPDATE").
		WithArgs(uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(2, "target"))
	mock.ExpectQuery("SELECT .* FROM `roles`.*LIMIT .*").
		WithArgs(uint(99), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	operatorID := uint(1)
	status, body := buildAssignRoleContext(t, "2", &operatorID, AssignUserRoleReq{RoleID: 99})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, "角色不存在")
}

func expectRoleAssignmentPrelude(mock sqlmock.Sqlmock, targetID uint, targetRoleID uint, targetRoleCode string, currentRoleID uint) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `users`.*FOR UPDATE").
		WithArgs(targetID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "managed_college_id"}).AddRow(targetID, "target", nil))
	mock.ExpectQuery("SELECT .* FROM `roles`.*LIMIT .*").
		WithArgs(targetRoleID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_name", "role_code"}).AddRow(targetRoleID, "目标角色", targetRoleCode))
	mock.ExpectQuery("SELECT .* FROM `roles`.*role_code = .*FOR UPDATE").
		WithArgs("school_admin", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_name", "role_code"}).AddRow(1, "校级管理员", "school_admin"))
	mock.ExpectQuery("SELECT .* FROM `user_roles` WHERE user_id = .*").
		WithArgs(targetID).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(targetID, currentRoleID))
}

func TestAssignUserRole_PreventsSelfDemotion(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	expectRoleAssignmentPrelude(mock, 1, 3, "teacher", 1)
	mock.ExpectRollback()

	operatorID := uint(1)
	status, body := buildAssignRoleContext(t, "1", &operatorID, AssignUserRoleReq{RoleID: 3})

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, "不能修改自己的校级管理员角色")
}

func TestAssignUserRole_CollegeAdminRequiresManagedCollege(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	expectRoleAssignmentPrelude(mock, 2, 2, "college_admin", 3)
	mock.ExpectRollback()

	operatorID := uint(1)
	status, body := buildAssignRoleContext(t, "2", &operatorID, AssignUserRoleReq{RoleID: 2})

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, "必须指定管理学院")
}

func TestAssignUserRole_NonCollegeAdminRejectsManagedCollege(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	expectRoleAssignmentPrelude(mock, 2, 3, "teacher", 3)
	mock.ExpectRollback()

	operatorID := uint(1)
	managedCollegeID := uint(2)
	status, body := buildAssignRoleContext(t, "2", &operatorID, AssignUserRoleReq{
		RoleID:           3,
		ManagedCollegeID: &managedCollegeID,
	})

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, "非院级管理员不能设置管理学院")
}

func TestAssignUserRole_CollegeMustExist(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	expectRoleAssignmentPrelude(mock, 2, 2, "college_admin", 3)
	mock.ExpectQuery("SELECT .* FROM `colleges`.*LIMIT .*").
		WithArgs(uint(99), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	operatorID := uint(1)
	managedCollegeID := uint(99)
	status, body := buildAssignRoleContext(t, "2", &operatorID, AssignUserRoleReq{
		RoleID:           2,
		ManagedCollegeID: &managedCollegeID,
	})

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, "管理学院不存在")
}

func TestAssignUserRole_PreventsRemovingLastActiveSchoolAdmin(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	expectRoleAssignmentPrelude(mock, 2, 3, "teacher", 1)
	mock.ExpectQuery("SELECT .* FROM `user_roles` JOIN users.*FOR UPDATE").
		WithArgs(uint(1)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(2, 1))
	mock.ExpectRollback()

	operatorID := uint(1)
	status, body := buildAssignRoleContext(t, "2", &operatorID, AssignUserRoleReq{RoleID: 3})

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, "至少保留一个校级管理员")
}

func TestAssignUserRole_CacheFailureReturnsAcceptedAndQueuesRetry(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	previousRedisClient := middleware.RedisClient
	middleware.RedisClient = nil
	t.Cleanup(func() { middleware.RedisClient = previousRedisClient })

	expectRoleAssignmentPrelude(mock, 2, 2, "college_admin", 3)
	mock.ExpectQuery("SELECT .* FROM `colleges`.*LIMIT .*").
		WithArgs(uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(2, "计算机学院"))
	mock.ExpectExec("DELETE FROM `user_roles` WHERE user_id = .*").
		WithArgs(uint(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_roles`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE `users` SET .*managed_college_id.*").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_role_audits`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `permission_cache_invalidation_tasks`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	operatorID := uint(1)
	managedCollegeID := uint(2)
	status, body := buildAssignRoleContext(t, "2", &operatorID, AssignUserRoleReq{
		RoleID:           2,
		ManagedCollegeID: &managedCollegeID,
	})

	assert.Equal(t, http.StatusAccepted, status)
	assert.Contains(t, body, "权限缓存正在重试刷新")
	assert.Contains(t, body, `"cache_refreshed":false`)
	assert.Contains(t, body, `"cache_retry_queued":true`)
}

func TestAssignUserRole_CacheAndRetryQueueFailureReturnsTruthfulAcceptedResponse(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	previousRedisClient := middleware.RedisClient
	middleware.RedisClient = nil
	t.Cleanup(func() { middleware.RedisClient = previousRedisClient })

	expectRoleAssignmentPrelude(mock, 2, 2, "college_admin", 3)
	mock.ExpectQuery("SELECT .* FROM `colleges`.*LIMIT .*").
		WithArgs(uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(2, "计算机学院"))
	mock.ExpectExec("DELETE FROM `user_roles` WHERE user_id = .*").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_roles`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE `users` SET .*managed_college_id.*").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_role_audits`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `permission_cache_invalidation_tasks`").
		WillReturnError(fmt.Errorf("queue db unavailable"))
	mock.ExpectRollback()

	operatorID := uint(1)
	managedCollegeID := uint(2)
	status, body := buildAssignRoleContext(t, "2", &operatorID, AssignUserRoleReq{
		RoleID:           2,
		ManagedCollegeID: &managedCollegeID,
	})

	assert.Equal(t, http.StatusAccepted, status)
	assert.Contains(t, body, "未能创建重试任务")
	assert.Contains(t, body, `"cache_refreshed":false`)
	assert.Contains(t, body, `"cache_retry_queued":false`)
	assert.Contains(t, body, `"manual_follow_up_required":true`)
	assert.Contains(t, body, `"cache_ttl_seconds":300`)
}

func TestAssignUserRole_IdempotentRequestDoesNotWriteAudit(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	expectRoleAssignmentPrelude(mock, 2, 3, "teacher", 3)
	mock.ExpectCommit()

	operatorID := uint(1)
	status, body := buildAssignRoleContext(t, "2", &operatorID, AssignUserRoleReq{RoleID: 3})

	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, "角色配置未发生变化")
	assert.Contains(t, body, `"role_updated":false`)
}

func TestAssignUserRole_SuccessClearsPermissionCache(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	mr := miniredis.RunT(t)
	previousRedisClient := middleware.RedisClient
	middleware.RedisClient = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = middleware.RedisClient.Close()
		middleware.RedisClient = previousRedisClient
	})
	assert.NoError(t, middleware.RedisClient.Set(context.Background(), "perm:2:user:list", "1", time.Minute).Err())

	expectRoleAssignmentPrelude(mock, 2, 2, "college_admin", 3)
	mock.ExpectQuery("SELECT .* FROM `colleges`.*LIMIT .*").
		WithArgs(uint(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(2, "计算机学院"))
	mock.ExpectExec("DELETE FROM `user_roles` WHERE user_id = .*").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_roles`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE `users` SET .*managed_college_id.*").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_role_audits`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	operatorID := uint(1)
	managedCollegeID := uint(2)
	status, body := buildAssignRoleContext(t, "2", &operatorID, AssignUserRoleReq{
		RoleID:           2,
		ManagedCollegeID: &managedCollegeID,
	})

	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"cache_refreshed":true`)
	assert.False(t, mr.Exists("perm:2:user:list"))
}

func TestGetAllUsers_RejectsInvalidIdentityType(t *testing.T) {
	setupUserManageDBMock(t)
	_, w, c := buildGET("/api/perm/user/list?identity_type=administrator")

	GetAllUsers(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "人员身份类型无效")
}

func TestGetAllUsers_EmptyPage(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `users`.*ORDER BY users.create_time DESC, users.id DESC").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}))

	_, w, c := buildGET("/api/perm/user/list?page=1&page_size=20")
	GetAllUsers(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"list":[]`)
	assert.Contains(t, w.Body.String(), `"total":0`)
}

func TestGetAllUsers_LegacyUserWithoutRoleReturnsNull(t *testing.T) {
	mock := setupUserManageDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `users`.*ORDER BY users.create_time DESC, users.id DESC").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "identity_type"}).
			AddRow(9, "legacy", "历史用户", "staff"))
	mock.ExpectQuery("SELECT .* FROM `user_roles` WHERE `user_roles`.`user_id` = .*").
		WithArgs(uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))

	_, w, c := buildGET("/api/perm/user/list?page=1&page_size=20")
	GetAllUsers(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"role":null`)
	assert.Contains(t, w.Body.String(), `"role_conflict":false`)
}
