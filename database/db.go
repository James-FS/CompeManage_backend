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
	c := config.AppConfig.Database

	// 调试打印：确认拿到的数据是否正确
	fmt.Printf("[DEBUG] 数据库配置: 用户=%s, 密码=%s, 地址=%s:%d, 库名=%s\n",
		c.User, "******", c.Host, c.Port, c.Dbname)

	// 拼接DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&allowNativePasswords=true",
		c.User, c.Password, c.Host, c.Port, c.Dbname,
	)

	// 连接数据库
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info), // 开发环境打印SQL
	})
	if err != nil {
		log.Fatalf("数据库连接失败：%v", err)
	}

	// 连接池调优 - 支持更高并发
	sqlDB, _ := DB.DB()
	sqlDB.SetMaxIdleConns(50)  // 增大空闲连接
	sqlDB.SetMaxOpenConns(300) // 增大最大打开连接
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
		&models.FileRecord{},
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
