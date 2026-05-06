package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Logger   LoggerConfig   `mapstructure:"logger"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Dbname   string `mapstructure:"dbname"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
}
type LoggerConfig struct {
	Level      string `mapstructure:"level"`
	Filename   string `mapstructure:"filename"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
	Compress   bool   `mapstructure:"compress"`
	ShowLine   bool   `mapstructure:"show_line"`
}

var AppConfig = &Config{}

// GetEnvironment 获取当前环境，默认为 dev
func GetEnvironment() string {
	if env := os.Getenv("ENV"); env != "" {
		return env
	}
	return "dev"
}

// GetConfigPath 获取配置文件路径
func GetConfigPath(filename string) string {
	// 获取当前工作目录
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("获取工作目录失败: %v", err)
	}
	// 返回 config 目录下的文件路径
	return filepath.Join(wd, "config", filename)
}

// Init 初始化配置（从环境变量读取配置）
func Init() {
	//设置yaml文件
	env := GetEnvironment()
	configFileName := "config-" + env + ".yaml"
	configPath := GetConfigPath(configFileName)

	log.Printf("加载配置环境: %s，配置文件: %s\n", env, configPath)

	// 尝试加载环境特定的配置文件
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")
	// 从环境变量读取配置
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("读取 %s 失败: %v，尝试加载默认配置\n", configPath, err)
		// 回退到默认配置文件
		defaultPath := GetConfigPath("config.yaml")
		viper.SetConfigFile(defaultPath)
		if err := viper.ReadInConfig(); err != nil {
			log.Fatalf("读取 %s 失败: %v", defaultPath, err)
		}
	}

	if err := viper.Unmarshal(AppConfig); err != nil {
		log.Fatalf("解析配置到结构体失败: %v", err)
	}

	// 数据库配置
	viper.Set("database.host", getEnv("DB_HOST", "localhost"))
	viper.Set("database.port", getEnv("DB_PORT", "3306"))
	viper.Set("database.user", getEnv("DB_USER", "root"))
	viper.Set("database.password", getEnv("DB_PASSWORD", "123456"))
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
