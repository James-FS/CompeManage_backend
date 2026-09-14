package models

import "time"

const (
	CacheInvalidationPending   = "pending"
	CacheInvalidationCompleted = "completed"
	CacheInvalidationFailed    = "failed"
)

// PermissionCacheInvalidationTask 保存权限缓存删除失败后的持久化重试任务。
// 一个用户始终复用同一条任务记录，避免并发产生重复任务。
type PermissionCacheInvalidationTask struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	UserID      uint       `gorm:"not null;uniqueIndex" json:"user_id"`
	Status      string     `gorm:"type:varchar(16);not null;index" json:"status"`
	Attempts    int        `gorm:"not null;default:0" json:"attempts"`
	NextRetryAt time.Time  `gorm:"index" json:"next_retry_at"`
	LastError   string     `gorm:"type:text" json:"last_error"`
	CreatedAt   time.Time  `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdatedAt   time.Time  `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
	CompletedAt *time.Time `json:"completed_at"`
}
