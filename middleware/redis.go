package middleware

import (
	"CompeManage_backend/config"
	"context"
	"errors"
	"fmt"
	"time"

	"CompeManage_backend/logger"

	"github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client

func InitRedis() {
	host := config.GetString("redis.host")
	port := config.GetInt("redis.port")
	password := config.GetString("redis.password")
	db := config.GetInt("redis.db")
	maxRetries := config.GetInt("redis.max_retries")
	poolSize := config.GetInt("redis.pool_size")
	if host == "" {
		host = "localhost"
	}
	if port == 0 {
		port = 6379
	}
	if maxRetries == 0 {
		maxRetries = 3
	}
	if poolSize == 0 {
		poolSize = 50 // 增大连接池以支持高并发
	}

	RedisClient = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Password:     password,
		DB:           db,
		MaxRetries:   maxRetries,
		PoolSize:     poolSize,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := RedisClient.Ping(ctx).Err(); err != nil {
		logger.Fatal("Redis连接失败",
			"host", host,
			"port", port,
			"db", db,
			"poolSize", poolSize,
			"error", err)
	}
	logger.Info("Redis启动成功", "host", host, "port", port, "db", db)
}

func GetRedisClient() *redis.Client { return RedisClient }

func CloseRedis() error {
	if RedisClient != nil {
		if err := RedisClient.Close(); err != nil {
			logger.Error("Redis连接关闭失败", "error", err)
			return err
		}
		logger.Info("Redis连接已关闭")
	}
	return nil
}

// ClearUserPermissionCache 删除指定用户的全部权限判断缓存。
// 使用 SCAN 避免 KEYS 阻塞 Redis，并对短暂错误进行有限重试。
func ClearUserPermissionCache(userID uint) error {
	if RedisClient == nil {
		return errors.New("Redis客户端未初始化")
	}
	if userID == 0 {
		return errors.New("用户ID无效")
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := clearUserPermissionCacheOnce(userID); err == nil {
			return nil
		} else {
			lastErr = err
			logger.Error("清理用户权限缓存失败",
				"userID", userID,
				"attempt", attempt,
				"error", err,
			)
		}
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
		}
	}
	return lastErr
}

func clearUserPermissionCacheOnce(userID uint) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pattern := fmt.Sprintf("perm:%d:*", userID)
	var cursor uint64
	for {
		keys, nextCursor, err := RedisClient.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("扫描权限缓存失败: %w", err)
		}
		if len(keys) > 0 {
			if err := RedisClient.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("删除权限缓存失败: %w", err)
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			return nil
		}
	}
}
