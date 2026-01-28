package models

type Award struct {
	BaseModel
	CompID     uint   `gorm:"index;not null;comment:冗余赛事ID方便查询" json:"comp_id"`
	RegID      uint   `gorm:"index;not null;unique;comment:关联报名ID" json:"reg_id"` // 设置 unique 保证一个报名记录主要对应一个奖项结果
	AwardLevel string `gorm:"type:varchar(50);comment:获奖等级" json:"award_level"`
	AwardName  string `gorm:"type:varchar(100);comment:具体奖项名" json:"award_name"`

	// 关联关系
	Register Register `gorm:"foreignKey:RegID" json:"register"`
}
