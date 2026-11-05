package middleware

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func permContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)
	c.Set("user_id", uint(1))
	return c, w
}

func TestRequireAnyPermission_NotLoggedIn(t *testing.T) {
	c, w := permContext(t)
	c.Keys = map[any]any{} // 去掉 user_id
	RequireAnyPermission("comp:list")(c)
	assert.Equal(t, 401, w.Code)
}

func TestRequireAnyPermission_InvalidRoleInfo(t *testing.T) {
	// 缺 role_id / version / role_id=0 三种失效形态
	for _, tc := range []func(c *gin.Context){
		func(c *gin.Context) {},
		func(c *gin.Context) { c.Set("role_id", uint(2)) },
		func(c *gin.Context) { c.Set("role_id", uint(0)); c.Set("permission_version", uint64(3)) },
		func(c *gin.Context) { c.Set("role_id", "x"); c.Set("permission_version", uint64(3)) },
	} {
		c, w := permContext(t)
		tc(c)
		RequireAnyPermission("comp:list")(c)
		assert.Equal(t, 403, w.Code)
		assert.Contains(t, w.Body.String(), "角色信息无效")
	}
}

func TestRequireAnyPermission_DBError(t *testing.T) {
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()
	withRedisClient(t, nil)

	mock.ExpectQuery("SELECT .* FROM `permissions`").WillReturnError(fmt.Errorf("db error"))

	c, w := permContext(t)
	c.Set("role_id", uint(2))
	c.Set("permission_version", uint64(3))
	RequireAnyPermission("comp:list")(c)
	assert.Equal(t, 500, w.Code)
}

func TestRequireAnyPermission_NoMatchDenied(t *testing.T) {
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()
	withRedisClient(t, nil)

	mock.ExpectQuery("SELECT .* FROM `permissions`").
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("other:perm"))

	c, w := permContext(t)
	c.Set("role_id", uint(2))
	c.Set("permission_version", uint64(3))
	RequireAnyPermission("comp:list")(c)
	assert.Equal(t, 403, w.Code)
	assert.Contains(t, w.Body.String(), "权限不足")
}

func TestRequireAnyPermission_DBGrant(t *testing.T) {
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()
	withRedisClient(t, nil)

	mock.ExpectQuery("SELECT .* FROM `permissions`").
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("comp:list").AddRow("comp:detail"))

	c, w := permContext(t)
	c.Set("role_id", uint(2))
	c.Set("permission_version", uint64(3))
	RequireAnyPermission("comp:list")(c)
	assert.Equal(t, 200, w.Code) // 未中止
	assert.False(t, c.IsAborted())
	codes, ok := c.Get("permission_codes")
	assert.True(t, ok)
	assert.Equal(t, []string{"comp:list", "comp:detail"}, codes)
}

// Redis 缓存命中：不查库直接放行
func TestRequireAnyPermission_RedisCacheHit(t *testing.T) {
	rdb := localRedis(t)
	withRedisClient(t, rdb)
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()
	// 无 DB 期望——命中缓存则不触发查询

	rdb.Set(t.Context(), "perm:v2:role:2:version:3", `["comp:list"]`, 0)

	c, w := permContext(t)
	c.Set("role_id", uint(2))
	c.Set("permission_version", uint64(3))
	RequireAnyPermission("comp:list")(c)
	assert.Equal(t, 200, w.Code)
	assert.False(t, c.IsAborted())
}

// Redis 缓存损坏：回退数据库
func TestRequireAnyPermission_RedisCacheCorruptFallbackDB(t *testing.T) {
	rdb := localRedis(t)
	withRedisClient(t, rdb)
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()

	rdb.Set(t.Context(), "perm:v2:role:2:version:3", `not-json`, 0)
	mock.ExpectQuery("SELECT .* FROM `permissions`").
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("comp:list"))

	c, w := permContext(t)
	c.Set("role_id", uint(2))
	c.Set("permission_version", uint64(3))
	RequireAnyPermission("comp:list")(c)
	assert.Equal(t, 200, w.Code)
	assert.False(t, c.IsAborted())
}

