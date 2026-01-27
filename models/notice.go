package models

import "gorm.io/gorm"

type Notice struct {
	gorm.Model
	Title               string `gorm:"size:255;not null" json:"title"` // 通知标题
	PublishTime         string `gorm:"not null" json:"publish_time"`   // 通知发布时间
	Content             string `gorm:"type:text" json:"content"`       // 通知详情内容
	CompetitionDetailID uint   `json:"compID"`                         // 关联赛事详情的ID
	Attachment          string `gorm:"size:512" json:"attachment"`     // 附件URL
}
