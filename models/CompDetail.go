package models

import (
	"time"
)

type CompDetail struct {
	ID                 uint      `gorm:"primaryKey;autoIncrement;comment:主键ID" json:"id"`
	CompID             uint      `gorm:"column:comp_id;not null;index;comment:关联竞赛目录ID" json:"comp_id"`
	CompStartTime      time.Time `gorm:"column:comp_start_time;comment:比赛开始时间" json:"comp_start_time"`
	CompEndTime        time.Time `gorm:"column:comp_end_time;comment:比赛结束时间" json:"comp_end_time"`
	RegStartTime       time.Time `gorm:"column:reg_start_time;comment:报名开始时间" json:"reg_start_time"`
	RegEndTime         time.Time `gorm:"column:reg_end_time;comment:报名结束时间" json:"reg_end_time"`
	SubmitStartTime    time.Time `gorm:"column:submit_start_time;comment:提交作品开始时间" json:"submit_start_time"`
	SubmitEndTime      time.Time `gorm:"column:submit_end_time;comment:提交作品截止时间" json:"submit_end_time"`
	ParticipantType    int8      `gorm:"column:participant_type;type:tinyint;default:1;comment:参赛形式(1:个人 2:团队)" json:"participant_type"`
	MaxTeamMember      int       `gorm:"column:max_team_member;default:1;comment:团队最高人数" json:"max_team_member"`
	MinTeamMember      int       `gorm:"column:min_team_member;default:1;comment:团队最低人数" json:"min_team_member"`
	GradeRequirement   string    `gorm:"column:grade_requirement;type:varchar(255);comment:年级要求(如:2022,2023)" json:"grade_requirement"`
	RegistrationMethod string    `gorm:"column:registration_method;type:longtext;comment:报名方式/参赛流程说明" json:"registration_method"`
	NeedAttachment     int       `gorm:"column:need_attachment;default:0;comment:是否需要上传附件,0:无需 1:可选 2:必须" json:"need_attachment"`
	NeedAdvisor        int       `gorm:"column:need_advisor;default:0;comment:是否需要指导老师,0:无需 1:可选 2:必须" json:"need_advisor"`
	AttachmentTemplate string    `gorm:"column:attachment_template;type:varchar(255);comment:附件模板地址" json:"attachment_template"`
	BaseModel
}

// TableName 指定表名
//func (CompetitionDetail) TableName() string {
//return "competition_detail"
//}
