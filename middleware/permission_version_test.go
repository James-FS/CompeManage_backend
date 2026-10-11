package middleware

import (
	"CompeManage_backend/utils"
	"context"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func permissionContext(roleID uint, version uint64) (*httptest.ResponseRecorder, *gin.Context) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/protected", nil)
	c.Set("user_id", uint(7))
	c.Set("role_id", roleID)
	c.Set("permission_version", version)
	return w, c
}

func usePermissionRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr := miniredis.RunT(t)
	previous := RedisClient
	RedisClient = redis.NewClient(&redis.Options{Addr: mr.Addr(), ContextTimeoutEnabled: true})
	client := RedisClient
	t.Cleanup(func() { _ = client.Close(); RedisClient = previous })
	return mr
}

func TestPermissionVersion_RevocationAndGrantIgnoreOldCaches(t *testing.T) {
	for _, granted := range []bool{false, true} {
		t.Run(fmt.Sprint(granted), func(t *testing.T) {
			mock := setupAuthMiddlewareDBMock(t)
			defer func() { require.NoError(t, mock.ExpectationsWereMet()) }()
			mr := usePermissionRedis(t)
			mr.Set("perm:7:summary:edit", "1")
			mr.Set("perm:v2:role:3:version:1", `["summary:edit"]`)
			rows := sqlmock.NewRows([]string{"code"})
			if granted {
				rows.AddRow("summary:edit")
			}
			mock.ExpectQuery("SELECT .* FROM `permissions` JOIN role_permissions.*roles.permission_version").
				WithArgs(uint(3), uint64(2)).WillReturnRows(rows)
			w, c := permissionContext(3, 2)
			RequirePermission("summary:edit")(c)
			if granted {
				require.Equal(t, 200, w.Code)
			} else {
				require.Equal(t, 403, w.Code)
			}
			require.True(t, mr.Exists("perm:v2:role:3:version:2"))
			require.Equal(t, PermissionCacheTTL, mr.TTL("perm:v2:role:3:version:2"))
			// 同一角色的另一个用户可复用缓存，不再查询数据库。
			w, c = permissionContext(3, 2)
			c.Set("user_id", uint(8))
			RequirePermission("summary:edit")(c)
			if granted {
				require.Equal(t, 200, w.Code)
			} else {
				require.Equal(t, 403, w.Code)
			}
		})
	}
}

func TestPermissionVersion_RoleSwitchDoesNotReusePreviousRole(t *testing.T) {
	usePermissionRedis(t).Set("perm:v2:role:3:version:1", `["summary:edit"]`)
	mock := setupAuthMiddlewareDBMock(t)
	defer func() { require.NoError(t, mock.ExpectationsWereMet()) }()
	mock.ExpectQuery("SELECT .* FROM `permissions`").WithArgs(uint(4), uint64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"code"}))
	w, c := permissionContext(4, 1)
	RequirePermission("summary:edit")(c)
	require.Equal(t, 403, w.Code)
}

func TestPermissionVersion_DatabaseFallback(t *testing.T) {
	for _, mode := range []string{"nil", "unavailable", "corrupt", "database_error"} {
		t.Run(mode, func(t *testing.T) {
			mock := setupAuthMiddlewareDBMock(t)
			defer func() { require.NoError(t, mock.ExpectationsWereMet()) }()
			mr := usePermissionRedis(t)
			switch mode {
			case "nil":
				_ = RedisClient.Close()
				RedisClient = nil
			case "unavailable":
				mr.Close()
			case "corrupt":
				mr.Set("perm:v2:role:3:version:2", "broken-json")
			}
			query := mock.ExpectQuery("SELECT .* FROM `permissions`").WithArgs(uint(3), uint64(2))
			if mode == "database_error" {
				query.WillReturnError(fmt.Errorf("db unavailable"))
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("summary:edit"))
			}
			w, c := permissionContext(3, 2)
			start := time.Now()
			RequirePermission("summary:edit")(c)
			require.Less(t, time.Since(start), 2*time.Second)
			if mode == "database_error" {
				require.Equal(t, 500, w.Code)
			} else {
				require.Equal(t, 200, w.Code)
			}
		})
	}
}

func TestAuthRequired_LoadsCurrentRoleVersionInsteadOfJWT(t *testing.T) {
	mock := setupAuthMiddlewareDBMock(t)
	defer func() { require.NoError(t, mock.ExpectationsWereMet()) }()
	token, err := utils.GenerateToken(7, "student", "old_role")
	require.NoError(t, err)
	mock.ExpectQuery("SELECT roles.id AS role_id, roles.permission_version, roles.role_code, users.managed_college_id").
		WithArgs(uint(7), 2).WillReturnRows(sqlmock.NewRows([]string{"role_id", "permission_version", "role_code", "managed_college_id"}).
		AddRow(3, 9, "student", nil))
	w, c := permissionContext(0, 0)
	c.Request.Header.Set("Authorization", "Bearer "+token)
	AuthRequired()(c)
	require.Equal(t, 200, w.Code)
	require.Equal(t, uint(3), c.MustGet("role_id"))
	require.Equal(t, uint64(9), c.MustGet("permission_version"))
	require.Equal(t, "student", c.GetString("role_code"))
}

func TestPermissionVersion_LateOldRequestCannotOverwriteNewCache(t *testing.T) {
	mr := usePermissionRedis(t)
	mr.Set("perm:v2:role:3:version:2", `["summary:edit"]`)
	mock := setupAuthMiddlewareDBMock(t)
	defer func() { require.NoError(t, mock.ExpectationsWereMet()) }()
	// 旧请求加载时角色已经更新，版本约束返回空，最多写入旧版本 key。
	mock.ExpectQuery("SELECT .* FROM `permissions`").WithArgs(uint(3), uint64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"code"}))
	_, c := permissionContext(3, 1)
	RequirePermission("summary:edit")(c)
	payload, err := RedisClient.Get(context.Background(), "perm:v2:role:3:version:2").Result()
	require.NoError(t, err)
	require.Equal(t, `["summary:edit"]`, payload)
	w, c := permissionContext(3, 2)
	RequirePermission("summary:edit")(c)
	require.Equal(t, 200, w.Code)
}
