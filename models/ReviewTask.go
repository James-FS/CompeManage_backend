package models

import "time"

type ReviewTask struct {
	BaseModel
	CompID      uint       `gorm:"uniqueIndex:idx_comp_expert;not null;comment:关联赛事ID" json:"comp_id"`
	ExpertID    uint       `gorm:"uniqueIndex:idx_comp_expert;not null;index:idx_expert_status;comment:专家用户ID" json:"expert_id"`
	Status      int8       `gorm:"default:0;index:idx_expert_status;comment:状态(0:已分配 1:待评审 2:评审中 3:已完成)" json:"status"`
	AssignedBy  uint       `gorm:"not null;comment:分配人ID" json:"assigned_by"`
	AssignedAt  *time.Time `gorm:"comment:分配时间" json:"assigned_at"`
	CompletedAt *time.Time `gorm:"comment:完成时间" json:"completed_at"`
}
