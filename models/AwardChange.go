package models

// AwardChange records the fields changed by an advisor without copying student contact details.
type AwardChange struct {
	BaseModel
	AwardID      uint   `gorm:"not null;index" json:"award_id"`
	OperatorID   uint   `gorm:"not null" json:"operator_id"`
	OperatorName string `gorm:"type:varchar(100)" json:"operator_name"`
	Before       string `gorm:"type:json" json:"before"`
	After        string `gorm:"type:json" json:"after"`
}
