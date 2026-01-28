package models

type Register struct {
	BaseModel
	ID                uint          `gorm:"primaryKey" json:"id"`
	CompID            uint          `gorm:"index;not null" json:"comp_id"` // 关联赛事ID
	TeamName          string        `gorm:"type:varchar(255);column:team_name" json:"team_name"`
	LeaderID          uint          `gorm:"index;not null" json:"leader_id"` // 队长/负责人ID (关联User)
	AdvisorID         *uint         `gorm:"index" json:"advisor_id"`         // 指导老师ID (指针类型允许为空/NULL)
	Status            int8          `gorm:"default:0;comment:'0:待审核 1:已通过 2:已驳回'" json:"status"`
	AttachmentUrl     string        `gorm:"type:varchar(512)" json:"attachment_url"`
	WorkAttachmentUrl string        `gorm:"type:text" json:"work_attachment_url"`
	RejectReason      string        `gorm:"type:text" json:"reject_reason"`
	Competition       CompDirectory `gorm:"foreignKey:CompID" json:"competition,omitempty"`
	Leader            User          `gorm:"foreignKey:LeaderID" json:"leader,omitempty"`
	Advisor           *User         `gorm:"foreignKey:AdvisorID" json:"omitempty"`

	// 一个报名包含多个成员
	Members []RegMember `gorm:"foreignKey:RegID" json:"members,omitempty"`
}
