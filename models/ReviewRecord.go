package models

import "time"

type ReviewRecord struct {
	BaseModel
	TaskID          uint       `gorm:"uniqueIndex:idx_task_reg;not null;comment:关联评审任务ID" json:"task_id"`
	RegID           uint       `gorm:"uniqueIndex:idx_task_reg;not null;comment:关联报名记录ID" json:"reg_id"`
	ExpertID        uint       `gorm:"not null;index:idx_expert_status;comment:专家用户ID(冗余)" json:"expert_id"`
	CompID          uint       `gorm:"not null;index;comment:赛事ID(冗余)" json:"comp_id"`
	Score           *float64   `gorm:"type:decimal(5,1);comment:总分(0-100，null表示未评审)" json:"score"`
	Comment         string     `gorm:"type:text;comment:评审意见" json:"comment"`
	Status          int8       `gorm:"default:0;index:idx_expert_status;comment:状态(0:未评审 1:已评审)" json:"status"`
	ReviewedAt      *time.Time `gorm:"comment:评审时间" json:"reviewed_at"`
	OriginalScore   *float64   `gorm:"type:decimal(5,1);comment:修改前分数" json:"original_score"`
	OriginalComment *string    `gorm:"type:text;comment:修改前评语" json:"original_comment"`
}
