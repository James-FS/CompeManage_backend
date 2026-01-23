package models

type Permission struct {
	BaseModel
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"type:varchar(255);not null;comment:权限名称"`
	Code string `gorm:"type:varchar(255);uniqueIndex;not null;comment:权限标识(如 user:list)"`
	Type int    `gorm:"type:tinyint;default:1;comment:类型 1:菜单 2:按钮"`
	//Path        string         `gorm:"type:varchar(255);comment:前端路由地址"`
	ParentID    uint   `gorm:"default:0;comment:父级ID"` // 用于构建树形结构
	Description string `gorm:"type:varchar(255);comment:描述"`
}
