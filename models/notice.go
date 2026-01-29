package models

import "gorm.io/gorm"

type Notice struct {
	gorm.Model
	Title               string `gorm:"size:255;not null" json:"title"` // 通知标题
	Status              int    `gorm:"default:0" json:"status"`        // 0-未发布 1-已发布
	Content             string `gorm:"type:text" json:"content"`       // 通知详情内容
	CompetitionDetailID uint   `json:"compID"`                         // 关联赛事详情的ID
	Attachment          string `gorm:"size:512" json:"attachment"`     // 附件URL
}
