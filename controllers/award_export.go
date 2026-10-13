package controllers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"github.com/gin-gonic/gin"
)

const awardExportApprovedStatus = "approved"
const awardExportLegacyStatus = "1"

type awardExportMember struct {
	AwardID       uint
	Name, College string
}

func awardMergedLevel(level, category, name string) string {
	if level != "" {
		return level
	}
	return category + name
}

func awardParticipantLabel(pt *int8) string {
	if pt == nil {
		return ""
	}
	if *pt == 2 {
		return "团体"
	}
	return "个人"
}

func awardJoinMembers(members []awardExportMember) (names, collegesJoined string) {
	namesList, collegesList := []string{}, []string{}
	seen := map[string]bool{}
	for _, m := range members {
		if name := strings.TrimSpace(m.Name); name != "" {
			namesList = append(namesList, name)
		}
		college := strings.TrimSpace(m.College)
		if college != "" && !seen[college] {
			seen[college] = true
			collegesList = append(collegesList, college)
		}
	}
	return strings.Join(namesList, " "), strings.Join(collegesList, " ")
}

// decodeAwardAdvisors 解析指导教师 JSON：兼容数组（奖项表）与单对象（报名表），空值返回 nil。
func decodeAwardAdvisors(raw string) []models.AdvisorInfo {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "[]" {
		return nil
	}
	var list []models.AdvisorInfo
	if err := json.Unmarshal([]byte(raw), &list); err == nil && len(list) > 0 {
		return list
	}
	var single models.AdvisorInfo
	if err := json.Unmarshal([]byte(raw), &single); err == nil && strings.TrimSpace(single.Name) != "" {
		return []models.AdvisorInfo{single}
	}
	return nil
}

// awardExportAdvisors 附件1的指导教师两列：优先奖项自身（导入/编辑写入），
// 为空时回退关联报名的指导教师；姓名按序拼接，学院按出现顺序去重拼接。
func awardExportAdvisors(awardJSON, regJSON string) (names, collegesJoined string) {
	advisors := decodeAwardAdvisors(awardJSON)
	if len(advisors) == 0 {
		advisors = decodeAwardAdvisors(regJSON)
	}
	nameList := []string{}
	seen := map[string]bool{}
	for _, adv := range advisors {
		if name := strings.TrimSpace(adv.Name); name != "" {
			nameList = append(nameList, name)
		}
		college := strings.TrimSpace(adv.College)
		if college != "" && !seen[college] {
			seen[college] = true
			collegesJoined = strings.TrimSpace(strings.Join([]string{collegesJoined, college}, " "))
		}
	}
	return strings.Join(nameList, " "), collegesJoined
}

// awardExportAdvisorsFromRows 从多行指导教师数据拼接附件1两列：姓名按序、学院按出现顺序去重。
func awardExportAdvisorsFromRows(rows []models.AwardAdvisor) (names, collegesJoined string) {
	nameList := []string{}
	seen := map[string]bool{}
	for _, row := range rows {
		if name := strings.TrimSpace(row.Name); name != "" {
			nameList = append(nameList, name)
		}
		college := strings.TrimSpace(row.College)
		if college != "" && !seen[college] {
			seen[college] = true
			collegesJoined = strings.TrimSpace(strings.Join([]string{collegesJoined, college}, " "))
		}
	}
	return strings.Join(nameList, " "), collegesJoined
}

// 历史学生补录仅保存合并等级；只补全其明确类别，不修改原数据或换算奖项等级。
func awardExportCategory(category, level string) string {
	for _, value := range []string{strings.TrimSpace(category), strings.TrimSpace(level)} {
		for _, prefix := range []string{"国家级", "国际级", "省部级", "省级", "校赛", "校级"} {
			if strings.HasPrefix(value, prefix) {
				switch prefix {
				case "国际级":
					return "国家级"
				case "省部级":
					return "省级"
				case "校级":
					return "校赛"
				default:
					return prefix
				}
			}
		}
	}
	return ""
}

func awardExportGrade(name, level string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	for _, prefix := range []string{"国家级", "国际级", "省部级", "省级", "校赛", "校级"} {
		if strings.HasPrefix(level, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(level, prefix))
		}
	}
	return level
}

func awardExportScope(c *gin.Context) (*UserAccessScope, bool) {
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return nil, false
	}
	switch {
	case scope.IsSchoolAdmin(), scope.IsCollegeAdmin():
		// 校级导全校，院级按提交学院过滤（见导出查询）
	case scope.RoleCode == "competition_manager":
		// 赛事负责人：用于线下盖章上报，仅导出本人所属学院的数据
		var college string
		if err := database.DB.WithContext(c.Request.Context()).Table("users").
			Where("id = ? AND delete_time IS NULL", scope.UserID).
			Pluck("college", &college).Error; err != nil || college == "" {
			utils.Forbidden(c, "赛事负责人未配置所属学院，无法导出")
			return nil, false
		}
		scope.College = college
	default:
		utils.Forbidden(c, "仅校级/院级管理员或赛事负责人可导出")
		return nil, false
	}
	return scope, true
}

// year 是赛事举办年份（comp_directories.year），而非获奖时间的年份。
func awardExportYear(c *gin.Context) (string, bool) {
	year := c.Query("year")
	if year == "" {
		return "", true
	}
	valid := len(year) == 4
	for _, r := range year {
		if r < '0' || r > '9' {
			valid = false
		}
	}
	n, err := strconv.Atoi(year)
	if !valid || err != nil || n < 1000 || n > 9999 {
		utils.BadRequest(c, "year 参数须为4位赛事年份，留空按赛事年份分表导出")
		return "", false
	}
	return year, true
}

// awardExportUnit 导出人所属单位（附件"单位名称（公章）"自动填写）：
// 赛事负责人复用 scope.College；其余角色查本人 users.college，
// 院级管理员为空时回落其管理学院名称。
func awardExportUnit(c *gin.Context, scope *UserAccessScope) string {
	unit := strings.TrimSpace(scope.College)
	if unit == "" {
		_ = database.DB.WithContext(c.Request.Context()).Table("users").
			Where("id = ? AND delete_time IS NULL", scope.UserID).
			Pluck("college", &unit).Error
		unit = strings.TrimSpace(unit)
	}
	if unit == "" && scope.ManagedCollegeID != nil {
		var college models.College
		if err := database.DB.WithContext(c.Request.Context()).
			First(&college, *scope.ManagedCollegeID).Error; err == nil {
			unit = college.Name
		}
	}
	return unit
}

func awardExportFilterStatus() []string {
	return []string{awardExportApprovedStatus, awardExportLegacyStatus}
}

func awardExportWorkbook(c *gin.Context, kind, year, unit string, rowsByYear map[int][][]interface{}) {
	f, err := buildAwardExportWorkbook(kind, year, unit, rowsByYear)
	if err != nil {
		utils.InternalServerError(c, "生成附件失败", err)
		return
	}
	defer f.Close()
	buffer, err := f.WriteToBuffer()
	if err != nil {
		utils.InternalServerError(c, "生成Excel失败", err)
		return
	}
	label := "获奖项目"
	if kind == "students" {
		label = "获奖学生"
	}
	suffix := "全部赛事年份"
	if year != "" {
		suffix = year + "年"
	}
	filename := url.PathEscape(suffix + "大学生参加省级以上各类竞赛" + label + "一览表（本科生获奖统计）.xlsx")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="award_export_%d.xlsx"; filename*=UTF-8''%s`, time.Now().UnixNano(), filename))
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
}

// ExportAwardProjects 导出附件1：按奖项一行，教师及签章区留空供人工填写。
func ExportAwardProjects(c *gin.Context) {
	scope, ok := awardExportScope(c)
	if !ok {
		return
	}
	year, ok := awardExportYear(c)
	if !ok {
		return
	}
	unit := awardExportUnit(c, scope)
	q := database.DB.WithContext(c.Request.Context()).Table("awards").
		Select("awards.id, awards.project_name, awards.award_level, awards.award_category, awards.award_name, awards.award_time, awards.advisor_info, r.advisor_info AS reg_advisor_info, cd.comp_name, cd.organizer, cd.year AS competition_year, cdp.participant_type").
		Joins("JOIN comp_directories cd ON cd.id = awards.comp_id AND cd.delete_time IS NULL").
		Joins("LEFT JOIN comp_details cdp ON cdp.comp_id = awards.comp_id AND cdp.delete_time IS NULL").
		Joins("LEFT JOIN registers r ON r.id = awards.reg_id AND r.delete_time IS NULL").
		Where("awards.delete_time IS NULL").Where("awards.status IN ?", awardExportFilterStatus()).
		Order("cd.year ASC, awards.award_time ASC, cd.comp_name ASC, awards.project_name ASC, awards.id ASC")
	if year != "" {
		q = q.Where("cd.year = ?", year)
	}
	if scope.RoleCode == "competition_manager" {
		q = q.Where("EXISTS (SELECT 1 FROM award_members am WHERE am.award_id = awards.id AND am.delete_time IS NULL AND am.college = ?)", scope.College)
	} else if !scope.IsSchoolAdmin() {
		q = q.Where("EXISTS (SELECT 1 FROM award_members am WHERE am.award_id = awards.id AND am.submitted_college_id = ? AND am.delete_time IS NULL)", *scope.ManagedCollegeID)
	}
	var projects []struct {
		ID                                                uint
		ProjectName, AwardLevel, AwardCategory, AwardName string
		AwardTime                                         *time.Time
		CompName, Organizer                               string
		AdvisorInfo, RegAdvisorInfo                       string
		CompetitionYear                                   int
		ParticipantType                                   *int8
	}
	if err := q.Find(&projects).Error; err != nil {
		utils.InternalServerError(c, "查询获奖项目失败", err)
		return
	}
	ids := []uint{}
	for _, p := range projects {
		category := awardExportCategory(p.AwardCategory, p.AwardLevel)
		if category == "国家级" || category == "省级" {
			ids = append(ids, p.ID)
		}
	}
	var advisorRows []models.AwardAdvisor
	if len(ids) > 0 {
		if err := database.DB.WithContext(c.Request.Context()).
			Where("award_id IN ?", ids).
			Order("award_id ASC, sort ASC, id ASC").
			Find(&advisorRows).Error; err != nil {
			utils.InternalServerError(c, "查询指导教师失败", err)
			return
		}
	}
	advisorsByAward := map[uint][]models.AwardAdvisor{}
	for _, adv := range advisorRows {
		advisorsByAward[adv.AwardID] = append(advisorsByAward[adv.AwardID], adv)
	}

	var members []awardExportMember
	if len(ids) > 0 {
		if err := database.DB.WithContext(c.Request.Context()).Table("award_members").Select("award_id, name, college").Where("award_id IN ? AND delete_time IS NULL", ids).Order("id ASC").Find(&members).Error; err != nil {
			utils.InternalServerError(c, "查询获奖学生失败", err)
			return
		}
	}
	membersByAward := map[uint][]awardExportMember{}
	for _, m := range members {
		membersByAward[m.AwardID] = append(membersByAward[m.AwardID], m)
	}
	rows := map[int][][]interface{}{}
	for _, p := range projects {
		category := awardExportCategory(p.AwardCategory, p.AwardLevel)
		if category != "国家级" && category != "省级" {
			continue
		}
		names, colleges := awardJoinMembers(membersByAward[p.ID])
		month := ""
		if p.AwardTime != nil {
			month = p.AwardTime.Format("2006-01")
		}
		advNames, advColleges := awardExportAdvisorsFromRows(advisorsByAward[p.ID])
		if advNames == "" {
			advNames, advColleges = awardExportAdvisors(p.AdvisorInfo, p.RegAdvisorInfo)
		}
		rows[p.CompetitionYear] = append(rows[p.CompetitionYear], []interface{}{len(rows[p.CompetitionYear]) + 1, p.CompName, p.ProjectName, p.Organizer, category + awardExportGrade(p.AwardName, p.AwardLevel), names, colleges, advNames, advColleges, month, awardParticipantLabel(p.ParticipantType)})
	}
	awardExportWorkbook(c, "projects", year, unit, rows)
}

// ExportAwardStudents 导出附件2：按学生一行，列顺序与学校模板保持一致。
func ExportAwardStudents(c *gin.Context) {
	scope, ok := awardExportScope(c)
	if !ok {
		return
	}
	year, ok := awardExportYear(c)
	if !ok {
		return
	}
	unit := awardExportUnit(c, scope)
	q := database.DB.WithContext(c.Request.Context()).Table("award_members am").
		Select("am.student_number, am.name, am.college, am.major, am.remark, a.project_name, a.award_level, a.award_category, a.award_name, a.award_time, cd.comp_name, cd.undertaker, cd.year AS competition_year").
		Joins("JOIN awards a ON a.id = am.award_id AND a.delete_time IS NULL").
		Joins("JOIN comp_directories cd ON cd.id = a.comp_id AND cd.delete_time IS NULL").
		Where("am.delete_time IS NULL").Where("a.status IN ?", awardExportFilterStatus()).
		Order("cd.year ASC, a.award_time ASC, cd.comp_name ASC, a.project_name ASC, am.id ASC")
	if year != "" {
		q = q.Where("cd.year = ?", year)
	}
	if scope.RoleCode == "competition_manager" {
		q = q.Where("am.college = ?", scope.College)
	} else if !scope.IsSchoolAdmin() {
		q = q.Where("am.submitted_college_id = ?", *scope.ManagedCollegeID)
	}
	var list []struct {
		StudentNumber, Name, College, Major, Remark, ProjectName, AwardLevel, AwardCategory, AwardName string
		AwardTime                                                                                      *time.Time
		CompName, Undertaker                                                                           string
		CompetitionYear                                                                                int
	}
	if err := q.Find(&list).Error; err != nil {
		utils.InternalServerError(c, "查询获奖学生失败", err)
		return
	}
	rows := map[int][][]interface{}{}
	for _, s := range list {
		category := awardExportCategory(s.AwardCategory, s.AwardLevel)
		if category != "国家级" && category != "省级" {
			continue
		}
		month := ""
		if s.AwardTime != nil {
			month = s.AwardTime.Format("2006-01")
		}
		rows[s.CompetitionYear] = append(rows[s.CompetitionYear], []interface{}{len(rows[s.CompetitionYear]) + 1, s.Undertaker, s.StudentNumber, s.Name, s.College, s.Major, s.CompName, s.ProjectName, month, category, awardExportGrade(s.AwardName, s.AwardLevel), s.Remark})
	}
	awardExportWorkbook(c, "students", year, unit, rows)
}
