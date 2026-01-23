package models

type Role struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	RoleName    string `gorm:"type:varchar(255);not null;comment:角色名称" json:"role_name"`
	RoleCode    string `gorm:"type:varchar(255);uniqueIndex;not null;comment:角色标识(如 admin)" json:"role_code"`
	Description string `gorm:"type:varchar(255);comment:描述" json:"description"`

	// 多对多关系：一个角色有多个权限
	Permissions []*Permission `gorm:"many2many:role_permissions;" json:"permissions"`
	BaseModel
}
