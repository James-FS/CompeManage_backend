package models

type AwardMember struct {
	BaseModel
	AwardID            uint   `gorm:"not null;uniqueIndex:idx_award_student" json:"award_id"`
	StudentID          uint   `gorm:"not null;index;uniqueIndex:idx_award_student" json:"student_user_id"`
	RegID              *uint  `gorm:"index" json:"reg_id"`
	Name               string `gorm:"type:varchar(100)" json:"name"`
	StudentNumber      string `gorm:"type:varchar(50)" json:"student_id"`
	College            string `gorm:"type:varchar(255)" json:"college"`
	Major              string `gorm:"type:varchar(100)" json:"major"`
	Remark             string `gorm:"type:text" json:"remark"`
	SubmittedCollegeID *uint  `gorm:"index" json:"submitted_college_id"`
	SubmittedBy        *uint  `gorm:"index" json:"submitted_by"`
	Student            User   `gorm:"foreignKey:StudentID" json:"-"`
}

type AwardImportSetting struct {
	ID                  uint `gorm:"primaryKey" json:"-"`
	RequireRegistration bool `gorm:"not null;default:false" json:"require_registration"`
}
