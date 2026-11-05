package controllers

import (
	"CompeManage_backend/models"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

func collegeNameForAwardScope(db *gorm.DB, scope *UserAccessScope) (string, error) {
	if scope == nil || !scope.IsCollegeAdmin() {
		return "", nil
	}
	if scope.ManagedCollegeID == nil {
		return "", errors.New("院级管理员尚未配置管理学院")
	}
	var college models.College
	if err := db.Select("id", "name").First(&college, *scope.ManagedCollegeID).Error; err != nil {
		return "", err
	}
	return college.Name, nil
}

// applyAwardStudentCollegeScope keeps college-admin award queries limited to
// awards with at least one participant currently belonging to their college.
// Legacy awards without member snapshots are resolved through their registration.
func applyAwardStudentCollegeScope(q *gorm.DB, scope *UserAccessScope, collegeName string, aliases ...string) *gorm.DB {
	if scope == nil || !scope.IsCollegeAdmin() {
		return q
	}
	if collegeName == "" {
		return q.Where("1 = 0")
	}
	awardAlias := "awards"
	if len(aliases) > 0 && aliases[0] != "" {
		awardAlias = aliases[0]
	}
	condition := fmt.Sprintf(`(
		EXISTS (
			SELECT 1
			FROM award_members am
			JOIN users award_students ON award_students.id = am.student_id AND award_students.delete_time IS NULL
			WHERE am.award_id = %s.id
				AND am.delete_time IS NULL
				AND award_students.college = ?
		)
		OR (
			NOT EXISTS (
				SELECT 1 FROM award_members am_any
				WHERE am_any.award_id = %s.id AND am_any.delete_time IS NULL
			)
			AND (
				EXISTS (
					SELECT 1
					FROM registers award_regs
					JOIN users award_leaders ON award_leaders.id = award_regs.leader_id AND award_leaders.delete_time IS NULL
					WHERE award_regs.id = %s.reg_id
						AND award_regs.delete_time IS NULL
						AND award_leaders.college = ?
				)
				OR EXISTS (
					SELECT 1
					FROM registers award_regs
					JOIN reg_members award_reg_members ON award_reg_members.reg_id = award_regs.id
					JOIN users award_reg_students ON award_reg_students.username = award_reg_members.username AND award_reg_students.delete_time IS NULL
					WHERE award_regs.id = %s.reg_id
						AND award_regs.delete_time IS NULL
						AND award_reg_students.college = ?
				)
			)
		)
	)`, awardAlias, awardAlias, awardAlias, awardAlias)
	return q.Where(condition, collegeName, collegeName, collegeName)
}
func awardBelongsToCollege(db *gorm.DB, awardID uint, scope *UserAccessScope, collegeName string) (bool, error) {
	var count int64
	err := applyAwardStudentCollegeScope(db.Model(&models.Award{}), scope, collegeName).
		Where("awards.id = ?", awardID).Count(&count).Error
	return count > 0, err
}

func filterAwardMembersForCollege(members []models.AwardMember, collegeName string) []models.AwardMember {
	filtered := make([]models.AwardMember, 0, len(members))
	for _, member := range members {
		name := member.Student.College
		if name == "" {
			name = member.College
		}
		if name == collegeName {
			filtered = append(filtered, member)
		}
	}
	return filtered
}

func filterRegMembersForCollege(members []models.RegMember, collegeName string) []models.RegMember {
	filtered := make([]models.RegMember, 0, len(members))
	for _, member := range members {
		if member.College == collegeName {
			filtered = append(filtered, member)
		}
	}
	return filtered
}

func filterAdvisorInfoForCollege(raw, collegeName string) string {
	var advisors []models.AdvisorInfo
	if err := json.Unmarshal([]byte(raw), &advisors); err != nil {
		var advisor models.AdvisorInfo
		if singleErr := json.Unmarshal([]byte(raw), &advisor); singleErr != nil || advisor.Name == "" {
			return "[]"
		}
		advisors = []models.AdvisorInfo{advisor}
	}
	filtered := make([]models.AdvisorInfo, 0, len(advisors))
	for _, advisor := range advisors {
		if advisor.College == collegeName {
			filtered = append(filtered, advisor)
		}
	}
	data, err := json.Marshal(filtered)
	if err != nil {
		return "[]"
	}
	return string(data)
}
