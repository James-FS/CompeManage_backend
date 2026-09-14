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
	CreatedAt           time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"`
}
