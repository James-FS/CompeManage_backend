package database

import (
	"CompeManage_backend/config"
	"CompeManage_backend/models"
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

	// 自动迁移数据库表
	autoMigrate()
}

func autoMigrate() {
	// 禁用外键检查，避免迁移时的外键约束问题
	DB.Exec("SET FOREIGN_KEY_CHECKS=0")

	err := DB.AutoMigrate(
		// 在此处添加需要自动迁移的模型
		// 例如：&moder.User{},
		&models.User{},
		&models.Role{},
		&models.Permission{},
		&models.College{},
		&models.CompDirectory{},
		&models.CompDetail{},
		&models.Register{},
		&models.RegMember{},
		&models.Notice{},
		&models.CompDeclaration{},
		&models.Award{},
		&models.Summary{},
	)

	// 重新启用外键检查
	DB.Exec("SET FOREIGN_KEY_CHECKS=1")

	if err != nil {
		log.Fatalf("数据库自动迁移失败：%v", err)
	}
	log.Println("数据库自动迁移成功")
}

// GetDB 获取DB实例（方便其他模块调用）
func GetDB() *gorm.DB {
	return DB
}
