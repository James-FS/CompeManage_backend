package models

type User struct {
	BaseModel
	Username string `gorm:"type:varchar(32);uniqueIndex;not null;comment:用户名" json:"username"`
	Realname string `gorm:"type:varchar(32);comment:真实姓名" json:"realname"`
	Password string `gorm:"type:varchar(255);not null;comment:加密密码" json:"-"`
	Grade    string `gorm:"type:varchar(16);comment:年级" json:"grade"`
	College  string `gorm:"type:varchar(255);comment:学院" json:"college"`
	Major    string `gorm:"type:varchar(64);comment:专业" json:"major"`

	// 以下字段来自学校数据中台同步，仅学生有值（指针类型允许 NULL）
	Sex       *string `gorm:"type:varchar(10);comment:性别" json:"sex"`
	MajorCode *string `gorm:"type:varchar(50);comment:专业代码" json:"major_code"`
	ClassCode *string `gorm:"type:varchar(50);comment:班级代码" json:"class_code"`
	ClassName *string `gorm:"type:varchar(100);comment:班级名称" json:"class_name"`

	// 多对多关系：一个用户有多个角色
	Roles []*Role `gorm:"many2many:user_roles;" json:"roles"`
}
