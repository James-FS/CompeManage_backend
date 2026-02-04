package models

import "time"

// Summary 赛事总结表
// 说明：与 comp_directories 一对一，记录赛事总结内容与附件
type Summary struct {
	BaseModel
	CompID         uint       `gorm:"column:comp_id;not null;uniqueIndex;comment:赛事ID" json:"comp_id"`
	SummaryContent string     `gorm:"column:summary_content;type:longtext;comment:总结内容" json:"summary_content"`
	Expenses       string     `gorm:"column:expenses;type:longtext;comment:经费明细(JSON)" json:"expenses"`
	Attachments    string     `gorm:"column:attachments;type:longtext;comment:附件(JSON)" json:"attachments"`
	Status         int8       `gorm:"column:status;type:tinyint;default:0;comment:状态(0:草稿 1:已归档)" json:"status"`
	ArchivedAt     *time.Time `gorm:"column:archived_at;comment:归档时间" json:"archived_at"`
	CreatedBy      uint       `gorm:"column:created_by;comment:创建人ID" json:"created_by"`
	UpdatedBy      uint       `gorm:"column:updated_by;comment:更新人ID" json:"updated_by"`
}
