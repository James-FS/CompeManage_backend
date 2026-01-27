package models

// CompDirectory 竞赛目录表 (主表)
type CompDirectory struct {
	ID         uint   `gorm:"primaryKey;autoIncrement;comment:主键ID" json:"id"`
	CompCode   string `gorm:"column:comp_code;not null;comment:竞赛代码(数字型)" json:"comp_code"`
	CompName   string `gorm:"column:comp_name;type:varchar(255);not null;comment:竞赛名称" json:"comp_name"`
	CompType   string `gorm:"column:comp_type;type:varchar(100);comment:竞赛类别(如:A类,B类)" json:"comp_type"`
	CompLevel  string `gorm:"column:comp_level;type:varchar(100);comment:竞赛级别(如:国家级,省级)" json:"comp_level"`
	Organizer  string `gorm:"column:organizer;type:varchar(255);comment:主办方" json:"organizer"`
	Undertaker string `gorm:"column:undertaker;type:varchar(255);comment:承办方" json:"undertaker"`
	CollegeID  uint   `gorm:"column:college_id;index;comment:所属学院ID" json:"college_id"`
	Year       int    `gorm:"column:year;comment:举办年份" json:"year"`
	ManagerID  uint   `gorm:"column:manager_id;index;comment:负责人ID(关联用户表)" json:"manager_id"`
	Status     int8   `gorm:"column:status;type:tinyint;default:0;comment:状态(0:草稿 1:发布 2:结束)" json:"status"`
	CreatedBy  uint   `gorm:"column:created_by;comment:创建人ID" json:"created_by"`
	Desc       string `gorm:"column:desc;type:text;comment:竞赛描述说明" json:"desc"`

	BaseModel

	// 关联关系：一对一 (目录拥有一个详情)
	Detail CompDetail `gorm:"foreignKey:CompID;references:ID" json:"detail"`

	// 关联关系：多对一 (多个竞赛目录关联一个负责人)
	Manager *User `gorm:"foreignKey:ManagerID;references:ID" json:"manager"`

	// 关联关系：多对一 (多个竞赛目录关联一个学院)
	CollegeInfo *College `gorm:"foreignKey:CollegeID;references:ID" json:"college_info"`
}

//// TableName 指定表名
//func (CompDirectory) TableName() string {
//	return "competition_directory"
//}
