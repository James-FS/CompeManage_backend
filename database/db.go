package database

import (
	"CompeManage_backend/config"
	"fmt"
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB // 全局DB实例

// Init 初始化数据库连接
func Init() {
	// 拼接DSN（MySQL连接字符串）
	dsn := "%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local"
	dsn = fmt.Sprintf(
		dsn,
		config.GetString("database.user"),
		config.GetString("database.password"),
		config.GetString("database.host"),
		config.GetString("database.port"),
		config.GetString("database.name"),
	)

	// 连接数据库
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info), // 开发环境打印SQL
	})
	if err != nil {
		log.Fatalf("数据库连接失败：%v", err)
	}

	// 可选：获取底层sql.DB，设置连接池
	sqlDB, _ := DB.DB()
	sqlDB.SetMaxIdleConns(10)  // 最大空闲连接
	sqlDB.SetMaxOpenConns(100) // 最大打开连接
	log.Println("数据库连接成功")
}

// GetDB 获取DB实例（方便其他模块调用）
func GetDB() *gorm.DB {
	return DB
}
