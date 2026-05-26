package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type DataHallConfig struct {
	Key      string `mapstructure:"key"`
	Secret   string `mapstructure:"secret"`
	BaseURL  string `mapstructure:"base_url"`
	MinGrade string `mapstructure:"min_grade"` // 只同步此年级及之后的学生，如 "2022"，为空则全量
}

type Config struct {
	Logger   LoggerConfig   `mapstructure:"logger"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	DataHall DataHallConfig `mapstructure:"datahall"`
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
	if env := os.Getenv("APP_ENV"); env != "" {
		switch env {
		case "development":
			return "dev"
		case "production":
			return "prod"
		default:
			return env
		}
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

// Init 初始化配置
func Init() {
	env := GetEnvironment()
	configFileName := "config-" + env + ".yaml"
	configPath := GetConfigPath(configFileName)
	log.Printf("加载配置环境: %s，配置文件: %s\n", env, configPath)

	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("读取 %s 失败: %v，尝试加载默认配置\n", configPath, err)
		defaultPath := GetConfigPath("config.yaml")
		viper.SetConfigFile(defaultPath)
		if err := viper.ReadInConfig(); err != nil {
			log.Fatalf("读取 %s 失败: %v", defaultPath, err)
		}
	}

	// 绑定环境变量（env var 优先级高于 YAML）
	viper.BindEnv("database.host", "DB_HOST")
	viper.BindEnv("database.port", "DB_PORT")
	viper.BindEnv("database.user", "DB_USER")
	viper.BindEnv("database.password", "DB_PASSWORD")
	viper.BindEnv("database.dbname", "DB_NAME")
	viper.BindEnv("redis.host", "REDIS_HOST")
	viper.BindEnv("redis.port", "REDIS_PORT")
	viper.BindEnv("redis.password", "REDIS_PASSWORD")
	viper.BindEnv("redis.db", "REDIS_DB")
	viper.BindEnv("server.port", "SERVER_PORT")
	viper.BindEnv("server.host", "SERVER_HOST")
	viper.BindEnv("jwt.secret", "JWT_SECRET")

	// 默认值（YAML 没配、env var 也没设时使用）
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 3306)
	viper.SetDefault("database.user", "root")
	viper.SetDefault("database.password", "123456")
	viper.SetDefault("database.dbname", "CompeManage")
	viper.SetDefault("server.port", "8080")
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("jwt.secret", "your-jwt-secret")

	// Unmarshal 放最后 —— BindEnv 绑定的 env var 会自动覆盖 YAML 值
	if err := viper.Unmarshal(AppConfig); err != nil {
		log.Fatalf("解析配置到结构体失败: %v", err)
	}
}

// GetString 获取字符串配置
func GetString(key string) string {
	return viper.GetString(key)
}

// GetInt 获取整数配置
func GetInt(key string) int {
	return viper.GetInt(key)
}
