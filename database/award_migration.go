package database

import (
	"CompeManage_backend/models"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"log"
	"strings"
)

func AwardProjectKey(name string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(name)))
	return hex.EncodeToString(h[:])
}

// EnsureAwardMembers 将原报名成员转成获奖成员一次，绝不猜测历史填报学院。
func EnsureAwardMembers(tx *gorm.DB, awardID uint) error {
	var a models.Award
	if err := tx.Preload("Register.Members").Preload("Register.Leader").First(&a, awardID).Error; err != nil {
		return err
	}
	if a.MembersMigrated {
		return nil
	}
	if a.ProjectName == "" {
		name := strings.TrimSpace(a.Register.TeamName)
		if name == "" {
			name = fmt.Sprintf("历史项目-%d", a.ID)
		}
		if err := tx.Model(&a).Update("project_name", name).Error; err != nil {
			return err
		}
	}
	members := a.Register.Members
	if a.Register.Leader.ID != 0 {
		found := false
		for _, m := range members {
			if m.StudentID == a.Register.Leader.Username {
				found = true
			}
		}
		if !found {
			members = append(members, models.RegMember{StudentID: a.Register.Leader.Username})
		}
	}
	unmatched := 0
	for _, m := range members {
		var user models.User
		err := tx.Where("username = ?", m.StudentID).First(&user).Error
		if err == gorm.ErrRecordNotFound {
			unmatched++
			continue
		}
		if err != nil {
			return err
		}
		regID := a.RegID
		member := models.AwardMember{AwardID: a.ID, StudentID: user.ID, RegID: &regID, Name: user.Realname, StudentNumber: user.Username, College: user.College, Major: user.Major}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&member).Error; err != nil {
			return err
		}
	}
	if unmatched > 0 {
		log.Printf("[AwardMigration] 奖项 %d 有 %d 个成员未匹配系统账号，原报名数据保留，需校级管理员核对", a.ID, unmatched)
	}
	return tx.Model(&a).Update("members_migrated", true).Error
}

func BackfillAwardMembers(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.AwardImportSetting{ID: 1}).Error; err != nil {
			return err
		}
		var ids []uint
		if err := tx.Model(&models.Award{}).Where("reg_id IS NOT NULL AND members_migrated = ?", false).Pluck("id", &ids).Error; err != nil {
			return err
		}
		for _, id := range ids {
			if err := EnsureAwardMembers(tx, id); err != nil {
				return err
			}
		}
		return nil
	})
}
