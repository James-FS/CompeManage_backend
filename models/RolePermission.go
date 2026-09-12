package models

type RolePermission struct {
	RoleID       uint `gorm:"column:role_id;primaryKey"`
	PermissionID uint `gorm:"column:permission_id;primaryKey"`
}

func (RolePermission) TableName() string {
	return "role_permissions"
}
