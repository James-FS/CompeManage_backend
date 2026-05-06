package models

import "time"

type Register struct {
	BaseModel
	ID                uint          `gorm:"primaryKey" json:"id"`
	CompID            uint          `gorm:"index;not null;uniqueIndex:idx_comp_leader" json:"comp_id"` // 关联赛事ID
	TeamName          string        `gorm:"type:varchar(255);column:team_name" json:"team_name"`
	LeaderID          uint          `gorm:"index;not null;uniqueIndex:idx_comp_leader" json:"leader_id"` // 队长/负责人ID (关联User)
	AdvisorID         *uint         `gorm:"index" json:"advisor_id"`                                     // 指导老师ID (指针类型允许为空/NULL)
	AdvisorInfo       string        `gorm:"type:json;column:advisor_info" json:"advisor_info"`
	Status            int8          `gorm:"default:0;comment:'0:待审核 1:已通过 2:已驳回 3:补录待审核 4:补录已通过 5:补录已驳回'" json:"status"`
	AttachmentUrl     string        `gorm:"type:varchar(512)" json:"attachment_url"`
	WorkAttachmentUrl string        `gorm:"type:text" json:"work_attachment_url"`
	RejectReason      string        `gorm:"type:text" json:"reject_reason"`
	Competition       CompDirectory `gorm:"foreignKey:CompID" json:"competition,omitempty"`
	Leader            User          `gorm:"foreignKey:LeaderID" json:"leader,omitempty"`
	Advisor           *User         `gorm:"foreignKey:AdvisorID" json:"omitempty"`
	Track             string        `gorm:"type:varchar(255)" json:"track"`
	SupplementTime    *time.Time    `gorm:"comment:补录提交时间" json:"supplement_time"`
	// 一个报名包含多个成员
	Members []RegMember `gorm:"foreignKey:RegID" json:"members,omitempty"`
}

type AdvisorInfo struct {
	ID       uint   `json:"id"`       // 教师ID
	Username string `json:"username"` // 工号
	Name     string `json:"name"`     // 教师名称
	Phone    string `json:"phone"`    // 电话
	Email    string `json:"email"`    // 邮箱（可选）
	College  string `json:"college"`  // 学院（可选）
}
