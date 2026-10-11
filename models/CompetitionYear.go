package models

// CompetitionYear 保存年度目录，包括尚未录入赛事的空目录。
// 使用年份作为主键；空目录删除为物理删除，不影响赛事的软删除。
type CompetitionYear struct {
	Year int `gorm:"primaryKey;autoIncrement:false" json:"year"`
}
