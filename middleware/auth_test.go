package middleware

import (
	"CompeManage_backend/database"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupAuthMiddlewareDBMock(t *testing.T) sqlmock.Sqlmock {
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

func TestRequirePermission_DeniedPermissionCacheReturnsForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	defer mr.Close()

	previousRedisClient := RedisClient
	RedisClient = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		_ = RedisClient.Close()
		RedisClient = previousRedisClient
	}()

	ctx := context.Background()
	cacheKey := "perm:1:summary:edit"
	assert.NoError(t, RedisClient.Set(ctx, cacheKey, "0", 5*time.Minute).Err())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/summary/1", nil)
	c.Set("user_id", uint(1))

	RequirePermission("summary:edit")(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "权限不足")
}

func TestRequireRole_AllowsMatchingSingleRole(t *testing.T) {
	mock := setupAuthMiddlewareDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles` JOIN roles").
		WithArgs(uint(7), "school_admin").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/perm/user/list", nil)
	c.Set("user_id", uint(7))

	RequireRole("school_admin")(c)

	assert.False(t, c.IsAborted())
	assert.Equal(t, "school_admin", c.GetString("role_code"))
}

func TestRequireRole_DeniesNonMatchingOrDuplicateRole(t *testing.T) {
	mock := setupAuthMiddlewareDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles` JOIN roles").
		WithArgs(uint(8), "school_admin").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/perm/user/list", nil)
	c.Set("user_id", uint(8))

	RequireRole("school_admin")(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireRole_DatabaseError(t *testing.T) {
	mock := setupAuthMiddlewareDBMock(t)
	defer func() { assert.NoError(t, mock.ExpectationsWereMet()) }()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_roles` JOIN roles").
		WithArgs(uint(9), "school_admin").
		WillReturnError(fmt.Errorf("db error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/perm/user/list", nil)
	c.Set("user_id", uint(9))

	RequireRole("school_admin")(c)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
