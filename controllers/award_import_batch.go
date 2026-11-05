package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const awardImportBatchSize = 500

// Validate shared team fields before looking at individual students, so a
// conflicting row cannot silently leave the first member's team imported.
func validateAwardImportTeams(data *awardImportLookup, rows []AwardImportRow) map[string]string {
	errorsByTeam := map[string]string{}
	firstRows := map[string]AwardImportRow{}
	advisorCache := map[string][]models.AdvisorInfo{}
	for _, row := range rows {
		row.ProjectName = strings.TrimSpace(row.ProjectName)
		if row.ProjectName == "" {
			continue
		}
		key := database.AwardProjectKey(row.ProjectName)
		if errorsByTeam[key] != "" {
			continue
		}
		fail := func(message string) { errorsByTeam[key] = "该团队未导入：" + message }
		if utf8.RuneCountInString(row.ProjectName) > 255 || utf8.RuneCountInString(row.AwardName) > 100 {
			fail("项目或奖项超出长度限制")
			continue
		}
		var err error
		row.Category, err = normalizeAwardCategory(row.Category)
		if err != nil {
			fail(err.Error())
			continue
		}
		row.AwardName, err = normalizeAwardName(row.AwardName)
		if err != nil {
			fail(err.Error())
			continue
		}
		month, err := parseAwardImportDate(row.AwardMonth)
		if err != nil {
			fail(err.Error())
			continue
		}
		row.AwardMonth = month.Format("2006-01")
		row.AdvisorUsernames = normalizeAdvisorList(row.AdvisorUsernames)
		row.AdvisorColleges = normalizeAdvisorList(row.AdvisorColleges)
		if row.AdvisorUsernames != "" {
			cacheKey := row.AdvisorUsernames + "\x00" + row.AdvisorColleges
			if _, ok := advisorCache[cacheKey]; !ok {
				pairs, err := resolveAwardAdvisorsWithLookup(row.AdvisorUsernames, row.AdvisorColleges, data.teacher)
				if err != nil {
					fail(err.Error())
					continue
				}
				advisorCache[cacheKey] = pairs
			}
		}
		if first, ok := firstRows[key]; ok && (first.Category != row.Category || first.AwardName != row.AwardName || first.AwardMonth != row.AwardMonth || first.AdvisorUsernames != row.AdvisorUsernames) {
			fail("同一团队的获奖类别、等级、时间或指导教师不一致")
			continue
		}
		firstRows[key] = row
		awards := append([]models.Award(nil), data.awards[key]...)
		awards = append(awards, data.legacyAwards[row.ProjectName]...)
		if len(awards) > 1 {
			fail("历史同名奖项冲突，请校级管理员先更正项目名称")
			continue
		}
		if len(awards) == 0 {
			continue
		}
		award := awards[0]
		if award.Status != "approved" {
			fail("该团队已有待审核或驳回奖项，不能通过导入覆盖")
			continue
		}
		name, err := normalizeAwardName(award.AwardName)
		if err != nil {
			name = award.AwardName
		}
		storedMonth := ""
		if award.AwardTime != nil {
			storedMonth = award.AwardTime.Format("2006-01")
		}
		if award.AwardCategory != row.Category || name != row.AwardName || storedMonth != row.AwardMonth {
			fail("与已有团队的类别、等级或时间冲突，请先更正团队奖项")
			continue
		}
		if row.AdvisorUsernames != "" {
			var stored []models.AdvisorInfo
			_ = json.Unmarshal([]byte(award.AdvisorInfo), &stored)
			storedKeys := []string{}
			allKeyed := len(stored) > 0
			for _, advisor := range stored {
				username := strings.TrimSpace(advisor.Username)
				if username == "" {
					allKeyed = false
				}
				storedKeys = append(storedKeys, username)
			}
			if allKeyed && strings.Join(storedKeys, " ") != row.AdvisorUsernames {
				fail("与已有团队的指导教师不一致，请先更正团队奖项")
			}
		}
	}
	return errorsByTeam
}

type awardImportLookup struct {
	managedCollege string
	users          map[string]models.User
	teachers       map[string]models.User
	awards         map[string][]models.Award
	legacyAwards   map[string][]models.Award
	registrations  map[uint]map[uint]bool
	memberships    map[uint]map[uint]bool
}

// Join the requested identifiers instead of matching them in Go: username
// matching must keep the database's collation (including case sensitivity).
func loadAwardImportUsers(tx *gorm.DB, names []string, teachers bool) (map[string]models.User, error) {
	result := map[string]models.User{}
	for start := 0; start < len(names); start += awardImportBatchSize {
		end := min(start+awardImportBatchSize, len(names))
		selects, args := make([]string, 0, end-start), make([]interface{}, 0, end-start)
		for _, name := range names[start:end] {
			selects = append(selects, "SELECT ? AS lookup_username")
			args = append(args, name)
		}
		var rows []struct {
			models.User    `gorm:"embedded"`
			LookupUsername string
		}
		query := tx.Model(&models.User{}).
			Select("users.id, users.username, users.realname, users.college, users.major, requested.lookup_username").
			Joins("JOIN ("+strings.Join(selects, " UNION ALL ")+") AS requested ON users.username = requested.lookup_username", args...)
		if teachers {
			// 2026-10-29：全校教职工口径（与弹窗一致），不再用 awardAdvisorCandidates
			query = query.Where("users.identity_type = 'staff'").Where("users.delete_time IS NULL")
		}
		if err := query.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			result[row.LookupUsername] = row.User
		}
	}
	return result, nil
}

func loadAwardImportLookup(tx *gorm.DB, compID uint, rows []AwardImportRow) (*awardImportLookup, error) {
	data := &awardImportLookup{
		awards: map[string][]models.Award{}, legacyAwards: map[string][]models.Award{},
		registrations: map[uint]map[uint]bool{}, memberships: map[uint]map[uint]bool{},
	}
	students, teachers, projects := []string{}, []string{}, []string{}
	studentSet, teacherSet, projectSet := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		student := strings.TrimSpace(row.StudentNumber)
		if student != "" && !studentSet[student] {
			studentSet[student] = true
			students = append(students, student)
		}
		for _, teacher := range strings.Fields(normalizeAdvisorList(row.AdvisorUsernames)) {
			if !teacherSet[teacher] {
				teacherSet[teacher] = true
				teachers = append(teachers, teacher)
			}
		}
		project := strings.TrimSpace(row.ProjectName)
		if project != "" && !projectSet[project] {
			projectSet[project] = true
			projects = append(projects, project)
		}
	}
	var err error
	if data.users, err = loadAwardImportUsers(tx, students, false); err != nil {
		return nil, err
	}
	if data.teachers, err = loadAwardImportUsers(tx, teachers, true); err != nil {
		return nil, err
	}
	seenAwards := map[uint]bool{}
	for start := 0; start < len(projects); start += awardImportBatchSize {
		end := min(start+awardImportBatchSize, len(projects))
		keys := make([]string, 0, end-start)
		for _, project := range projects[start:end] {
			keys = append(keys, database.AwardProjectKey(project))
		}
		var awards []models.Award
		if err := tx.Where("comp_id = ? AND (project_key IN ? OR (project_key IS NULL AND project_name IN ?))", compID, keys, projects[start:end]).Find(&awards).Error; err != nil {
			return nil, err
		}
		for _, award := range awards {
			if seenAwards[award.ID] {
				continue
			}
			seenAwards[award.ID] = true
			if award.ProjectKey == nil {
				// Legacy names were compared with BINARY, so retain exact matching.
				data.legacyAwards[award.ProjectName] = append(data.legacyAwards[award.ProjectName], award)
			} else {
				data.awards[*award.ProjectKey] = append(data.awards[*award.ProjectKey], award)
			}
		}
	}
	ids := []uint{}
	seenUsers := map[uint]bool{}
	for _, name := range students {
		if user, ok := data.users[name]; ok && !seenUsers[user.ID] {
			seenUsers[user.ID] = true
			ids = append(ids, user.ID)
		}
	}
	for start := 0; start < len(ids); start += awardImportBatchSize {
		chunk := ids[start:min(start+awardImportBatchSize, len(ids))]
		var leaders []struct {
			ID       uint
			LeaderID uint
		}
		if err := tx.Model(&models.Register{}).Select("id, leader_id").Where("comp_id = ? AND status IN ? AND leader_id IN ?", compID, []int{1, 4}, chunk).Find(&leaders).Error; err != nil {
			return nil, err
		}
		addReg := func(studentID, regID uint) {
			if data.registrations[studentID] == nil {
				data.registrations[studentID] = map[uint]bool{}
			}
			data.registrations[studentID][regID] = true
		}
		for _, reg := range leaders {
			addReg(reg.LeaderID, reg.ID)
		}
		var members []struct {
			StudentID uint
			RegID     uint
		}
		if err := tx.Model(&models.Register{}).
			Select("DISTINCT users.id AS student_id, registers.id AS reg_id").
			Joins("JOIN reg_members ON reg_members.reg_id = registers.id").
			Joins("JOIN users ON users.username = reg_members.username AND users.delete_time IS NULL").
			Where("registers.comp_id = ? AND registers.status IN ? AND users.id IN ?", compID, []int{1, 4}, chunk).
			Find(&members).Error; err != nil {
			return nil, err
		}
		for _, member := range members {
			addReg(member.StudentID, member.RegID)
		}
		var memberships []models.AwardMember
		if err := tx.Model(&models.AwardMember{}).Select("award_members.student_id, award_members.award_id").
			Joins("JOIN awards ON awards.id = award_members.award_id").
			Where("awards.comp_id = ? AND awards.delete_time IS NULL AND award_members.student_id IN ?", compID, chunk).
			Find(&memberships).Error; err != nil {
			return nil, err
		}
		for _, member := range memberships {
			if data.memberships[member.StudentID] == nil {
				data.memberships[member.StudentID] = map[uint]bool{}
			}
			data.memberships[member.StudentID][member.AwardID] = true
		}
	}
	return data, nil
}

// Called inside the import transaction, after every row has passed validation.
func writeAwardImportRows(tx *gorm.DB, scope *UserAccessScope, compID uint, rows []AwardImportRow) (created, existing, teams int, err error) {
	groupIDs, firstRows := map[string]uint{}, map[string]AwardImportRow{}
	keys := []string{}
	newAwards := []models.Award{}
	for _, row := range rows {
		key := database.AwardProjectKey(row.ProjectName)
		if _, ok := firstRows[key]; ok {
			continue
		}
		keys = append(keys, key)
		firstRows[key] = row
		groupIDs[key] = row.AwardID
		if row.AwardID != 0 {
			// Only legacy awards need a key backfill.
			if row.NeedsProjectKey {
				if err = tx.Model(&models.Award{}).Where("id = ? AND project_key IS NULL", row.AwardID).Update("project_key", key).Error; err != nil {
					return
				}
			}
			continue
		}
		month, _ := parseAwardImportDate(row.AwardMonth)
		now := time.Now()
		advisorInfo := advisorsMirrorJSON(row.Resolved)
		if advisorInfo == "" {
			advisorInfo = advisorInfoForImport(row.AdvisorUsernames, row.AdvisorColleges)
		}
		newAwards = append(newAwards, models.Award{MembersMigrated: true, CompID: compID, ProjectName: row.ProjectName, ProjectKey: &key, AwardCategory: row.Category, AwardName: row.AwardName, AwardLevel: row.Category + row.AwardName, AwardTime: &month, Status: "approved", Source: "import", AuditTime: &now, LevelRank: 99, AdvisorInfo: advisorInfo})
	}
	if len(newAwards) > 0 {
		if err = tx.CreateInBatches(&newAwards, awardImportBatchSize).Error; err != nil {
			return
		}
		for _, award := range newAwards {
			groupIDs[*award.ProjectKey] = award.ID
		}
	}
	// Fetch advisor presence once per chunk; fill each team only once.
	advisorIDs := []uint{}
	for _, key := range keys {
		if len(firstRows[key].Resolved) > 0 {
			advisorIDs = append(advisorIDs, groupIDs[key])
		}
	}
	hasAdvisors := map[uint]bool{}
	for start := 0; start < len(advisorIDs); start += awardImportBatchSize {
		var found []uint
		if err = tx.Model(&models.AwardAdvisor{}).Distinct("award_id").Where("award_id IN ?", advisorIDs[start:min(start+awardImportBatchSize, len(advisorIDs))]).Pluck("award_id", &found).Error; err != nil {
			return
		}
		for _, id := range found {
			hasAdvisors[id] = true
		}
	}
	advisors := []models.AwardAdvisor{}
	for _, key := range keys {
		row, id := firstRows[key], groupIDs[key]
		if len(row.Resolved) == 0 || hasAdvisors[id] {
			continue
		}
		for i, pair := range row.Resolved {
			advisors = append(advisors, models.AwardAdvisor{AwardID: id, Username: pair.Username, Name: pair.Name, College: pair.College, Sort: i})
		}
		if row.AwardID != 0 {
			if err = tx.Model(&models.Award{}).Where("id = ? AND (advisor_info IS NULL OR advisor_info IN ('', '[]'))", id).Update("advisor_info", advisorsMirrorJSON(row.Resolved)).Error; err != nil {
				return
			}
		}
	}
	if len(advisors) > 0 {
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&advisors, awardImportBatchSize).Error; err != nil {
			return
		}
	}
	members := []models.AwardMember{}
	regIDs, seenRegs := []uint{}, map[uint]bool{}
	for _, row := range rows {
		if row.Existing || row.Duplicate {
			existing++
			continue
		}
		uid := scope.UserID
		members = append(members, models.AwardMember{AwardID: groupIDs[database.AwardProjectKey(row.ProjectName)], StudentID: row.UserID, RegID: row.RegID, Name: row.Name, StudentNumber: row.StudentNumber, College: row.College, Major: row.Major, Remark: row.Remark, SubmittedCollegeID: scope.ManagedCollegeID, SubmittedBy: &uid})
		if row.RegID != nil && !seenRegs[*row.RegID] {
			seenRegs[*row.RegID] = true
			regIDs = append(regIDs, *row.RegID)
		}
	}
	if len(members) > 0 {
		if err = tx.CreateInBatches(&members, awardImportBatchSize).Error; err != nil {
			return
		}
	}
	for start := 0; start < len(regIDs); start += awardImportBatchSize {
		if err = tx.Model(&models.Register{}).Where("id IN ? AND comp_id = ? AND status IN ?", regIDs[start:min(start+awardImportBatchSize, len(regIDs))], compID, []int{1, 4}).Update("status", 4).Error; err != nil {
			return
		}
	}
	created, teams = len(members), len(keys)
	return
}

func (data *awardImportLookup) teacher(username string) (models.User, error) {
	if user, ok := data.teachers[username]; ok {
		if data.managedCollege != "" && user.College != data.managedCollege {
			return models.User{}, fmt.Errorf("指导教师工号 %s 不属于本院", username)
		}
		return user, nil
	}
	return models.User{}, fmt.Errorf("指导教师工号 %s 不存在或不是教职工", username)
}
