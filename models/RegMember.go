package models

type RegMember struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	RegID     uint   `gorm:"index;not null;column:reg_id" json:"reg_id"` // 外键关联 Register.ID
	Name      string `gorm:"type:varchar(50);not null" json:"name"`
	StudentID string `gorm:"type:varchar(50);column:username" json:"stu_id"`
	Phone     string `gorm:"type:varchar(20)" json:"phone" binding:"required"`
	Email     string `gorm:"type:varchar(255);column:email" json:"email"`
	College   string `gorm:"type:varchar(255);column:college" json:"college"`
	IsLeader  bool   `gorm:"default:false" json:"is_leader"`
	Year      string `gorm:"type:varchar(20)" json:"year"`
}

// RegMemberInfo 【前端请求参数结构体】- 专门接收学生申报时提交的队员信息
// 仅保留前端需要传递的字段，搭配gin的binding标签做参数校验，json标签严格匹配前端传参名
type RegMemberInfo struct {
	Name      string `json:"name" binding:"required"`       // 队员姓名 - 前端传name，必填
	StudentID string `json:"student_id" binding:"required"` // 学号 - 前端传student_id，必填（核心匹配前端字段）
	Phone     string `json:"phone" binding:"required"`      // 手机号 - 前端传phone，必填
	Email     string `json:"email"`                         // 邮箱 - 前端传email，可选
	College   string `json:"college" binding:"required"`    // 所属学院 - 前端传college，必填
	IsLeader  bool   `json:"is_leader"`                     // 是否为队长 - 前端传is_leader，可选（业务层校验有且仅一个）
	Year      string `json:"year"`                          // 年级 - 前端传year，可选
}
