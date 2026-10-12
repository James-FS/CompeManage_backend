package models

import "time"

type Award struct {
	BaseModel
	CompID            uint    `gorm:"index;not null;uniqueIndex:idx_award_project;comment:冗余赛事ID方便查询" json:"comp_id"`
	RegID             uint    `gorm:"index;default:null;unique;comment:可选关联报名ID" json:"reg_id"`
	ParticipationType string  `gorm:"type:varchar(20)" json:"participation_type"`
	Remark            string  `gorm:"type:text" json:"remark"`
	ProjectName       string  `gorm:"type:varchar(255)" json:"project_name"`
	ProjectKey        *string `gorm:"type:char(64);uniqueIndex:idx_award_project" json:"-"`
	MembersMigrated   bool    `gorm:"not null;default:false" json:"-"`
	AwardCategory     string  `gorm:"type:varchar(30)" json:"award_category"`
	LevelRank         int     `gorm:"column:level_rank;default:99;comment:排序权重" json:"level_rank"`
	AwardLevel        string  `gorm:"type:varchar(150);comment:获奖等级" json:"award_level"`
	AwardName         string  `gorm:"type:varchar(100);comment:具体奖项名" json:"award_name"`

	Status   string `gorm:"type:varchar(20);default:'draft';comment:申报状态(draft/待审核 approved/通过 rejected/驳回)" json:"status"`
	ProofUrl string `gorm:"type:varchar(255);comment:获奖证明文件URL" json:"proof_url"`

	Source       string     `gorm:"type:varchar(20);default:'import';comment:奖项来源 import-系统导入 supplement-学生补录" json:"source"`
	AuditorID    *uint      `gorm:"comment:审核人ID" json:"auditor_id"`
	AuditTime    *time.Time `gorm:"comment:审核时间" json:"audit_time"`
	RejectReason string     `gorm:"type:varchar(200);comment:驳回理由" json:"reject_reason"`
	AwardTime    *time.Time `gorm:"comment:获奖时间" json:"award_time"`
	// AdvisorInfo 指导教师，JSON 数组 [{"name":"","college":""}]；附件1导出使用，报名来源的奖项也可回退读 Register.AdvisorInfo
	AdvisorInfo string `gorm:"type:json;comment:指导教师信息" json:"advisor_info"`
	// 关联关系
	Auditor     User          `gorm:"foreignKey:AuditorID" json:"auditor,omitempty"` // 审核人信息
	Register    Register      `gorm:"foreignKey:RegID" json:"register"`
	Members     []AwardMember `gorm:"foreignKey:AwardID" json:"award_members"`
	Competition CompDirectory `gorm:"foreignKey:CompID" json:"competition"`
}
