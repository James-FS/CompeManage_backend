package middleware

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

// 保存/还原全局 RedisClient
func withRedisClient(t *testing.T, c *redis.Client) {
	t.Helper()
	prev := RedisClient
	RedisClient = c
	t.Cleanup(func() { RedisClient = prev })
}

func localRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if rdb.Ping(t.Context()).Err() != nil {
		t.Skip("本地 Redis 不可用")
	}
	return rdb
}

func unreachableRedis() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond})
}

// ===================== ClearUserPermissionCache =====================

func TestClearUserPermissionCache_InvalidArgs(t *testing.T) {
	withRedisClient(t, nil)
	assert.EqualError(t, ClearUserPermissionCache(1), "Redis客户端未初始化")

	withRedisClient(t, unreachableRedis())
	assert.EqualError(t, ClearUserPermissionCache(0), "用户ID无效")
}

// 扫描失败 → 重试 3 次后返回错误
func TestClearUserPermissionCache_RetriesThenFails(t *testing.T) {
	withRedisClient(t, unreachableRedis())
	start := time.Now()
	err := ClearUserPermissionCache(5)
	assert.Error(t, err)
	assert.GreaterOrEqual(t, time.Since(start), 300*time.Millisecond) // 100ms + 200ms 两次退避
}

func TestClearUserPermissionCache_ClearsKeys(t *testing.T) {
	rdb := localRedis(t)
	withRedisClient(t, rdb)
	ctx := t.Context()
	rdb.Set(ctx, "perm:77:comp:list", "1", time.Minute)
	rdb.Set(ctx, "perm:77:comp:delete", "1", time.Minute)
	rdb.Set(ctx, "perm:78:comp:list", "1", time.Minute)

	assert.NoError(t, ClearUserPermissionCache(77))

	assert.Equal(t, int64(0), rdb.Exists(ctx, "perm:77:comp:list").Val())
	assert.Equal(t, int64(0), rdb.Exists(ctx, "perm:77:comp:delete").Val())
	assert.Equal(t, int64(1), rdb.Exists(ctx, "perm:78:comp:list").Val())
}

// ===================== ClearAwardImportPermissionCache =====================

func TestClearAwardImportPermissionCache_NilClient(t *testing.T) {
	withRedisClient(t, nil)
	assert.NoError(t, ClearAwardImportPermissionCache())
}

func TestClearAwardImportPermissionCache_ScanError(t *testing.T) {
	withRedisClient(t, unreachableRedis())
	assert.Error(t, ClearAwardImportPermissionCache())
}

func TestClearAwardImportPermissionCache_Clears(t *testing.T) {
	rdb := localRedis(t)
	withRedisClient(t, rdb)
	ctx := t.Context()
	rdb.Set(ctx, "perm:v2:role:1:version:2:award:import", "1", time.Minute)
	rdb.Set(ctx, "perm:v2:role:9:version:2:award:import", "1", time.Minute)

	assert.NoError(t, ClearAwardImportPermissionCache())

	assert.Equal(t, int64(0), rdb.Exists(ctx, "perm:v2:role:1:version:2:award:import").Val())
	assert.Equal(t, int64(0), rdb.Exists(ctx, "perm:v2:role:9:version:2:award:import").Val())
}

// ===================== InitRedis / GetRedisClient / CloseRedis =====================

func TestInitRedis_Success(t *testing.T) {
	localRedis(t) // 不可达则跳过（失败分支会 os.Exit，不可单测）
	prev := RedisClient
	t.Cleanup(func() {
		RedisClient = prev
	})

	InitRedis()

	assert.NotNil(t, GetRedisClient())
	assert.NoError(t, CloseRedis())
	assert.Nil(t, GetRedisClient())
}

func TestCloseRedis_NilClient(t *testing.T) {
	withRedisClient(t, nil)
	assert.NoError(t, CloseRedis())
}
