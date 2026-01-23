package models

// CompetitionDirectory 竞赛目录表 (主表)
type CompDirectory struct {
	ID         uint   `gorm:"primaryKey;autoIncrement;comment:主键ID"`
	CompCode   string `gorm:"column:comp_code;not null;comment:竞赛代码(数字型)"`
	CompName   string `gorm:"column:comp_name;type:varchar(255);not null;comment:竞赛名称"`
	CompType   string `gorm:"column:comp_type;type:varchar(100);comment:竞赛类别(如:A类,B类)"`
	CompLevel  string `gorm:"column:comp_level;type:varchar(100);comment:竞赛级别(如:国家级,省级)"`
	Organizer  string `gorm:"column:organizer;type:varchar(255);comment:主办方"`
	Undertaker string `gorm:"column:undertaker;type:varchar(255);comment:承办方"`
	CollegeID  uint   `gorm:"column:college_id;index;comment:所属学院ID"`
	Year       int    `gorm:"column:year;comment:举办年份"`
	ManagerID  uint   `gorm:"column:manager_id;index;comment:负责人ID(关联用户表)"`
	Status     int8   `gorm:"column:status;type:tinyint;default:0;comment:状态(0:草稿 1:发布 2:结束)"` // 建议使用常量管理
	CreatedBy  uint   `gorm:"column:created_by;comment:创建人ID"`

	BaseModel

	// 关联关系：一对一 (目录拥有一个详情)
	Detail CompDetail `gorm:"foreignKey:CompID;references:ID"`
}

//// TableName 指定表名
//func (CompDirectory) TableName() string {
//	return "competition_directory"
//}
