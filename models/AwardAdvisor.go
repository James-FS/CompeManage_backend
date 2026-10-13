package models

// AwardAdvisor 获奖项目指导教师。
// 唯一标识采用「工号+姓名+所属学院」三元组中的工号作主键成分（unique award_id+username），
// 姓名与学院为系统资料快照：同名不同工号、同一人多工号均能区分，避免按姓名关联的混淆。
type AwardAdvisor struct {
	BaseModel
	AwardID  uint   `gorm:"not null;uniqueIndex:idx_award_advisor;comment:获奖项目ID" json:"award_id"`
	Username string `gorm:"type:varchar(50);not null;uniqueIndex:idx_award_advisor;comment:教师工号" json:"username"`
	Name     string `gorm:"type:varchar(100);comment:教师姓名(系统资料快照)" json:"name"`
	College  string `gorm:"type:varchar(255);comment:教师归属学院(系统资料快照)" json:"college"`
	Sort     int    `gorm:"default:0;comment:排序(附件1按序拼接)" json:"sort"`
}
