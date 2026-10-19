package database

import (
	"CompeManage_backend/models"
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BackfillAwardAdvisors only migrates explicit, verifiable work IDs. Names alone
// never grant access. Existing structured relations remain authoritative.
func BackfillAwardAdvisors(db *gorm.DB) error {
	var cursor uint
	for {
		var batch []models.Award
		if err := db.Select("id").Where("id > ? AND advisor_info IS NOT NULL AND advisor_info NOT IN ('', '[]', '{}')", cursor).
			Where("NOT EXISTS (SELECT 1 FROM award_advisors aa WHERE aa.award_id = awards.id)").Order("id ASC").Limit(200).Find(&batch).Error; err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for _, item := range batch {
			cursor = item.ID
			if err := db.Transaction(func(tx *gorm.DB) error {
				var a models.Award
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, item.ID).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return nil
					}
					return err
				}
				var count int64
				if err := tx.Unscoped().Model(&models.AwardAdvisor{}).Where("award_id = ?", a.ID).Count(&count).Error; err != nil {
					return err
				}
				if count > 0 {
					return nil
				}
				var advisors []models.AdvisorInfo
				if json.Unmarshal([]byte(a.AdvisorInfo), &advisors) != nil {
					return nil
				}
				seen := map[string]bool{}
				for i, advisor := range advisors {
					username := strings.TrimSpace(advisor.Username)
					if username == "" || seen[username] {
						continue
					}
					seen[username] = true
					var user models.User
					err := tx.Where("username = ? AND identity_type = 'staff'", username).
						Where(`NOT EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id AND r.delete_time IS NULL WHERE ur.user_id = users.id AND r.role_code IN ('school_admin', 'college_admin', 'expert', 'guest'))`).First(&user).Error
					if errors.Is(err, gorm.ErrRecordNotFound) {
						continue
					}
					if err != nil {
						return err
					}
					// A conflicting name is ambiguous historical data; leave it for manual correction.
					if advisor.Name != "" && strings.TrimSpace(advisor.Name) != user.Realname {
						continue
					}
					if err := tx.Create(&models.AwardAdvisor{AwardID: a.ID, Username: user.Username, Name: user.Realname, College: user.College, Sort: i}).Error; err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
		}
	}
}
