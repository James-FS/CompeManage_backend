package models

type College struct {
	ID   uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Name string `gorm:"type:varchar(100);not null;unique;comment:学院名称" json:"name"`
	Code string `gorm:"type:varchar(50);comment:学院代码(可选)" json:"code"`
}
