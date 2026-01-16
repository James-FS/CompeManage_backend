package config

import (
	"os"

	"github.com/spf13/viper"
)

// Init 初始化配置（从环境变量读取配置）
func Init() {
	// 从环境变量读取配置
	viper.AutomaticEnv()

	// 数据库配置
	viper.Set("database.host", getEnv("DB_HOST", "localhost"))
	viper.Set("database.port", getEnv("DB_PORT", "3306"))
	viper.Set("database.user", getEnv("DB_USER", "root"))
	viper.Set("database.password", getEnv("DB_PASSWORD", ""))
	viper.Set("database.name", getEnv("DB_NAME", "CompeManage"))

	// 服务器配置
	viper.Set("server.port", getEnv("SERVER_PORT", "8080"))
	viper.Set("server.host", getEnv("SERVER_HOST", "0.0.0.0"))

	// JWT配置
	viper.Set("jwt.secret", getEnv("JWT_SECRET", "your-jwt-secret"))
}

// getEnv 读取环境变量，无则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetString 获取字符串配置
func GetString(key string) string {
	return viper.GetString(key)
}

// GetInt 获取整数配置
func GetInt(key string) int {
	return viper.GetInt(key)
}
