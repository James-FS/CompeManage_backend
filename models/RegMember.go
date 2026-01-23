package models

type RegMember struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	RegID     uint   `gorm:"index;not null;column:reg_id" json:"reg_id"` // 外键关联 Register.ID
	Name      string `gorm:"type:varchar(50);not null" json:"name"`
	StudentID string `gorm:"type:varchar(50);column:username" json:"stu_id"`
	Phone     string `gorm:"type:varchar(20)" json:"phone"`
	IsLeader  bool   `gorm:"default:false" json:"is_leader"`
	Year      string `gorm:"type:varchar(20)" json:"year"`
}
