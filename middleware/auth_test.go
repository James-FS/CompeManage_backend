package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

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
