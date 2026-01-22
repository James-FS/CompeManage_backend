package models

import (
	"time"
)

type CompDetail struct {
	ID                 uint      `gorm:"primaryKey;autoIncrement;comment:主键ID"`
	CompID             uint      `gorm:"column:comp_id;not null;index;comment:关联竞赛目录ID"`
	CompStartTime      time.Time `gorm:"column:comp_start_time;comment:比赛开始时间"`
	CompEndTime        time.Time `gorm:"column:comp_end_time;comment:比赛结束时间"`
	RegStartTime       time.Time `gorm:"column:reg_start_time;comment:报名开始时间"`
	RegEndTime         time.Time `gorm:"column:reg_end_time;comment:报名结束时间"`
	ParticipantType    int8      `gorm:"column:participant_type;type:tinyint;default:1;comment:参赛形式(1:个人 2:团队)"`
	MaxTeamMember      int       `gorm:"column:max_team_member;default:1;comment:团队最高人数"`
	MinTeamMember      int       `gorm:"column:min_team_member;default:1;comment:团队最低人数"`
	GradeRequirement   string    `gorm:"column:grade_requirement;type:varchar(255);comment:年级要求(如:2022,2023)"`
	RegistrationMethod string    `gorm:"column:registration_method;type:longtext;comment:报名方式/参赛流程说明"`
	NeedAttachment     int       `gorm:"column:need_attachment;default:0;comment:是否需要上传附件,0:无需 1:可选 2:必须"`
	NeedAdvisor        int       `gorm:"column:need_advisor;default:0;comment:是否需要指导老师,0:无需 1:可选 2:必须"`
	AttachmentTemplate string    `gorm:"column:attachment_template;type:varchar(255);comment:附件模板地址"`
	BaseModel
}

// TableName 指定表名
//func (CompetitionDetail) TableName() string {
//return "competition_detail"
//}
