package models

type User struct {
	BaseModel
	Username string `gorm:"type:varchar(32);uniqueIndex;not null;comment:用户名" json:"username"`
	Realname string `gorm:"type:varchar(32);comment:真实姓名" json:"realname"`
	Password string `gorm:"type:varchar(255);not null;comment:加密密码" json:"-"`
	Grade    string `gorm:"type:varchar(16);comment:年级" json:"grade"`
	College  string `gorm:"type:varchar(255);comment:学院" json:"college"`
	Major    string `gorm:"type:varchar(64);comment:专业" json:"major"`

	// IdentityType 表示人员自然身份，独立于授权角色。
	// 可选值：staff / student / postgraduate / external。
	IdentityType string `gorm:"column:identity_type;type:varchar(32);index;comment:人员身份类型" json:"identity_type"`

	// ManagedCollegeID 仅用于院级管理员的数据授权范围，不能由数据大厅同步覆盖。
	ManagedCollegeID *uint    `gorm:"column:managed_college_id;index;comment:院管理员管理学院ID" json:"managed_college_id"`
	ManagedCollege   *College `gorm:"foreignKey:ManagedCollegeID;references:ID" json:"managed_college,omitempty"`

	// 以下字段来自学校数据中台同步，仅学生有值（指针类型允许 NULL）
	Sex       *string `gorm:"type:varchar(10);comment:性别" json:"sex"`
	MajorCode *string `gorm:"type:varchar(50);comment:专业代码" json:"major_code"`
	ClassCode *string `gorm:"type:varchar(50);comment:班级代码" json:"class_code"`
	ClassName *string `gorm:"type:varchar(100);comment:班级名称" json:"class_name"`
	Title     *string `gorm:"type:varchar(100);comment:职称" json:"title"`

	// 关联仍沿用 many2many 表结构，但业务和数据库唯一索引保证每个用户只有一个角色。
	Roles []*Role `gorm:"many2many:user_roles;" json:"roles"`
}
