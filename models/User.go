package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID       uint   `gorm:"primaryKey" json:"id"` // GORM 默认 uint 为主键，对应数据库 BIGINT/INT UNSIGNED
	Username string `gorm:"type:varchar(32);uniqueIndex;not null;comment:用户名" json:"username"`
	Realname string `gorm:"type:varchar(32);comment:真实姓名" json:"realname"`
	Password string `gorm:"type:varchar(255);not null;comment:加密密码" json:"-"`
	Grade    string `gorm:"type:varchar(16);comment:年级" json:"grade"`
	College  string `gorm:"type:varchar(255);comment:学院" json:"college"`
	Major    string `gorm:"type:varchar(64);comment:专业" json:"major"`

	// 多对多关系：一个用户有多个角色
	Roles []*Role `gorm:"many2many:user_roles;" json:"roles"`

	// 自动处理时间字段
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"` // 对应 create_time
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"` // 对应 update_time
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`                   // 对应 delete_time (软删除)
}
