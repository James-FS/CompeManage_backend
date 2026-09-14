package models

import "gorm.io/gorm"

type Notice struct {
	gorm.Model
	Title               string `gorm:"size:255;not null" json:"title"` // 通知标题
	PublishTime         string `gorm:"size:20" json:"publish_time"`    // 发布时间字段
	Status              int    `gorm:"default:0" json:"status"`        // 0-未发布 1-已发布
	Content             string `gorm:"type:text" json:"content"`       // 通知详情内容
	CompetitionDetailID uint   `json:"compID"`                         // 关联赛事详情的ID（实际存 comp_directories.id）
	Attachment          string `gorm:"size:512" json:"attachment"`     // 附件URL
	// PublisherID 通知归属人（创建/发布者）。指针允许 NULL 以兼容存量数据；
	// 用于「赛事负责人只能发布/编辑/删除自己发布的通知」的归属校验（P0-3）。
	PublisherID *uint `gorm:"column:publisher_id;index;comment:发布人ID" json:"publisher_id"`
}
