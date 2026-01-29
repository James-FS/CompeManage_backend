package models

import "time"

// CompDeclaration 赛事申报记录表 (院级申报)
type CompDeclaration struct {
	ID             uint   `gorm:"primaryKey;autoIncrement;comment:主键ID" json:"id"`
	CompName       string `gorm:"column:comp_name;type:varchar(255);not null;comment:赛事名称" json:"comp_name"`
	CompLevel      string `gorm:"column:comp_level;type:varchar(100);comment:赛事等级(校级/省级/国家级/国际级)" json:"comp_level"`
	CompType       string `gorm:"column:comp_type;type:varchar(100);comment:赛事类型(学科竞赛/创新创业/体育竞赛/其他)" json:"comp_type"`
	Organizer      string `gorm:"column:organizer;type:varchar(255);comment:主办单位" json:"organizer"`
	Undertaker     string `gorm:"column:undertaker;type:varchar(255);comment:承办单位" json:"undertaker"`
	CollegeID      uint   `gorm:"column:college_id;index;not null;comment:申报学院ID" json:"college_id"`
	ManagerID      uint   `gorm:"column:manager_id;index;comment:赛事负责人ID(关联用户表)" json:"manager_id"`
	Year           int    `gorm:"column:year;comment:举办年份" json:"year"`
	Desc           string `gorm:"column:desc;type:text;comment:赛事简介" json:"desc"`
	AttachmentPath string `gorm:"column:attachment_path;type:varchar(255);comment:申报附件路径" json:"attachment_path"`

	// 申报状态：0-草稿 1-已提交 2-已通过 3-已拒绝
	DeclareStatus int8      `gorm:"column:declare_status;type:tinyint;default:0;comment:申报状态(0:草稿 1:已提交 2:已通过 3:已拒绝)" json:"declare_status"`
	AuditRemark   string    `gorm:"column:audit_remark;type:text;comment:审核意见" json:"audit_remark"`
	AuditBy       uint      `gorm:"column:audit_by;comment:审核人ID(校级管理员)" json:"audit_by"`
	AuditAt       time.Time `gorm:"column:audit_at;comment:审核时间" json:"audit_at"`

	// 申报人ID (创建人)
	CreatedBy uint `gorm:"column:created_by;comment:申报人ID" json:"created_by"`

	// 关联的赛事目录ID (审核通过后生成)
	CompDirectoryID uint `gorm:"column:comp_directory_id;index;comment:关联的赛事目录ID(通过后)" json:"comp_directory_id"`

	BaseModel

	// 关联关系：多对一 (申报关联一个学院)
	CollegeInfo *College `gorm:"foreignKey:CollegeID;references:ID" json:"college_info"`

	// 关联关系：多对一 (申报关联一个赛事负责人)
	Manager *User `gorm:"foreignKey:ManagerID;references:ID" json:"manager"`

	// 关联关系：多对一 (申报关联一个申报人/创建人)
	Declarer *User `gorm:"foreignKey:CreatedBy;references:ID" json:"declarer"`

	// 关联关系：多对一 (申报关联一个审核人)
	Auditor *User `gorm:"foreignKey:AuditBy;references:ID" json:"auditor"`

	// 关联关系：一对一 (申报关联一个赛事目录)
	CompDirectory *CompDirectory `gorm:"foreignKey:CompDirectoryID;references:ID" json:"comp_directory"`
}

//// TableName 指定表名
//func (CompDeclaration) TableName() string {
//	return "comp_declaration"
//}
