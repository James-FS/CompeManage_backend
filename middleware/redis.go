package middleware

import (
	"CompeManage_backend/config"
	"context"
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
		poolSize = 50  // 增大连接池以支持高并发
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
			"password", password,
			"db", db,
			"poolSize", poolSize,
			"error", err)
	}
	logger.Info("Redis启动成功", "host", host, "port", port, "password", password, "db", db)
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
