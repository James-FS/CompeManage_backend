package models

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel 公共模型
type BaseModel struct {
	ID uint `gorm:"primaryKey" json:"id"` // 主键
	// autoCreateTime 用于在创建时自动写入当前时间
	CreatedAt time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"`

	// autoUpdateTime 用于在更新时自动写入当前时间
	UpdatedAt time.Time `gorm:"column:update_time;autoUpdateTime" json:"update_time"`

	// 软删除，查询时会自动过滤掉该字段不为空的记录
	DeletedAt gorm.DeletedAt `gorm:"column:delete_time;index" json:"-"`
}
