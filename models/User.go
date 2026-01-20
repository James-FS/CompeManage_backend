package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID       uint   `gorm:"primaryKey"` // GORM 默认 uint 为主键，对应数据库 BIGINT/INT UNSIGNED
	Username string `gorm:"type:varchar(32);uniqueIndex;not null;comment:用户名"`
	Realname string `gorm:"type:varchar(32);comment:真实姓名"`
	Password string `gorm:"type:varchar(255);not null;comment:加密密码"`
	Grade    string `gorm:"type:varchar(16);comment:年级"`
	College  string `gorm:"type:varchar(255);comment:学院"`
	Major    string `gorm:"type:varchar(64);comment:专业"`

	// 多对多关系：一个用户有多个角色
	Roles []*Role `gorm:"many2many:user_roles;"`

	// 自动处理时间字段
	CreatedAt time.Time      `gorm:"autoCreateTime"` // 对应 create_time
	UpdatedAt time.Time      `gorm:"autoUpdateTime"` // 对应 update_time
	DeletedAt gorm.DeletedAt `gorm:"index"`          // 对应 delete_time (软删除)
}
