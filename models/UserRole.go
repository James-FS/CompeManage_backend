package models

// UserRole 是用户与角色的显式关联模型。
// 正式迁移后 user_id 具有唯一约束，从数据库层保证一个用户最多一个角色。
type UserRole struct {
	UserID uint `gorm:"column:user_id;primaryKey" json:"user_id"`
	RoleID uint `gorm:"column:role_id;not null;index" json:"role_id"`
}

func (UserRole) TableName() string {
	return "user_roles"
}
