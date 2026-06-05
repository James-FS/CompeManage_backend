package models

type Department struct {
	ID    uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Name  string `gorm:"type:varchar(100);not null;unique;comment:部门名称" json:"name"`
	Code  string `gorm:"type:varchar(50);index;comment:机构代码" json:"code"`
	Ename string `gorm:"type:varchar(200);comment:机构英文名称" json:"ename"`
}
