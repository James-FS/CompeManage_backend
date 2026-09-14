package models

import "time"

// UserRoleAudit 记录用户角色和管理学院的变更，供安全审计使用。
type UserRoleAudit struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	OperatorID          uint      `gorm:"not null;index" json:"operator_id"`
	TargetUserID        uint      `gorm:"not null;index" json:"target_user_id"`
	OldRoleID           *uint     `json:"old_role_id"`
	NewRoleID           uint      `gorm:"not null" json:"new_role_id"`
	OldManagedCollegeID *uint     `json:"old_managed_college_id"`
	NewManagedCollegeID *uint     `json:"new_managed_college_id"`
	RequestIP           string    `gorm:"type:varchar(64)" json:"request_ip"`
	// Reason 记录变更来源：manual=用户管理页手工分配；competition=赛事写入路径自动提升；
	// import=批量导入自动提升；declare=申报写入路径自动提升。
	// 自动回收（如将来启用降级）只允许作用于非 manual 来源的用户。
	Reason string `gorm:"type:varchar(32);index;comment:manual|competition|import|declare" json:"reason"`
	// SourceID 记录触发变更的业务对象ID（赛事/申报ID），自动变更时写入。
	SourceID *uint `json:"source_id"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"`
}
