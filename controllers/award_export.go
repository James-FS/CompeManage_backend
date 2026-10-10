package controllers

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"CompeManage_backend/database"
	"CompeManage_backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// 附件导出（学校负责人汇总上报用）：
// 附件1 项目一览：按项目一行，学生/学院拼接；指导教师列暂留空，待与老师确认数据来源后补充。
// 附件2 学生一览：按学生一行平铺。
// 主办单位/承办单位/团体或个人均直接读赛事创建时已有的字段。

const awardExportApprovedStatus = "approved"
const awardExportLegacyStatus = "1"

type awardExportMember struct {
	AwardID uint
	Name    string
	College string
}

// awardMergedLevel 附件1的合并等级写法：优先用导入时拼好的 award_level（如"国家级一等奖"），旧数据回退拼接类别+等级。
func awardMergedLevel(level, category, name string) string {
	if level != "" {
		return level
	}
	return category + name
}

// awardParticipantLabel 参赛形式：1 个人，2 团体，未知留空。
func awardParticipantLabel(pt *int8) string {
	if pt == nil {
		return ""
	}
	if *pt == 2 {
		return "团体"
	}
	return "个人"
}

// awardJoinMembers 学生姓名按导入顺序拼接；学院按出现顺序去重拼接。
func awardJoinMembers(members []awardExportMember) (names, collegesJoined string) {
	seenColleges := map[string]bool{}
	for _, m := range members {
		if names != "" {
			names += " "
		}
		names += m.Name
		if m.College != "" && !seenColleges[m.College] {
			seenColleges[m.College] = true
			if collegesJoined != "" {
				collegesJoined += " "
			}
			collegesJoined += m.College
		}
	}
	return names, collegesJoined
}

func awardExportScope(c *gin.Context) (*UserAccessScope, bool) {
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return nil, false
	}
	if !scope.IsSchoolAdmin() && !scope.IsCollegeAdmin() {
		utils.Forbidden(c, "仅支持校级或院级管理员导出")
		return nil, false
	}
	return scope, true
}

// awardExportYear 读取并校验 year 参数，空串表示不过滤；非法时已写响应并返回 ok=false。
func awardExportYear(c *gin.Context) (string, bool) {
	year := c.Query("year")
	if year == "" {
		return "", true
	}
	if _, err := strconv.Atoi(year); err != nil || len(year) != 4 {
		utils.BadRequest(c, "year 参数须为4位年份，留空导出全部")
		return "", false
	}
	return year, true
}

// awardExportFilterStatus 获奖数据可见状态：审核通过，或历史数据的"1"。
func awardExportFilterStatus() []string {
	return []string{awardExportApprovedStatus, awardExportLegacyStatus}
}

func awardExportWorkbook(c *gin.Context, sheetName, title string, headers []string, widths []float64, rows [][]interface{}) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	if sheetName != "" {
		f.SetSheetName(sheet, sheetName)
		sheet = sheetName
	}

	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 14, Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 12, Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"DCE6F1"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "999999"}, {Type: "right", Style: 1, Color: "999999"},
			{Type: "top", Style: 1, Color: "999999"}, {Type: "bottom", Style: 1, Color: "999999"},
		},
	})
	bodyStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 12},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "999999"}, {Type: "right", Style: 1, Color: "999999"},
			{Type: "top", Style: 1, Color: "999999"}, {Type: "bottom", Style: 1, Color: "999999"},
		},
	})

	lastCol, _ := excelize.ColumnNumberToName(len(headers))
	end := fmt.Sprintf("%s1", lastCol)
	f.SetCellStyle(sheet, "A1", end, titleStyle)
	f.MergeCell(sheet, "A1", end)
	f.SetRowHeight(sheet, 1, 28)
	f.SetCellValue(sheet, "A1", title)

	for i, h := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		cell := fmt.Sprintf("%s2", col)
		f.SetCellValue(sheet, cell, h)
		f.SetColWidth(sheet, col, col, widths[i])
	}
	f.SetCellStyle(sheet, "A2", end, headerStyle)
	f.SetRowHeight(sheet, 2, 24)

	for r, row := range rows {
		excelRow := r + 3
		for i, v := range row {
			col, _ := excelize.ColumnNumberToName(i + 1)
			f.SetCellValue(sheet, fmt.Sprintf("%s%d", col, excelRow), v)
		}
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", excelRow), fmt.Sprintf("%s%d", lastCol, excelRow), bodyStyle)
	}

	filename := url.PathEscape(title + ".xlsx")
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="award_export_%d.xlsx"; filename*=UTF-8''%s`, time.Now().UnixNano(), filename))
	if err := f.Write(c.Writer); err != nil {
		utils.InternalServerError(c, "写出Excel失败", err)
	}
}

// ExportAwardProjects 导出附件1：按项目一行。
// GET /api/award/export/projects?year=2025
func ExportAwardProjects(c *gin.Context) {
	scope, ok := awardExportScope(c)
	if !ok {
		return
	}
	year, ok := awardExportYear(c)
	if !ok {
		return
	}

	q := database.DB.WithContext(c.Request.Context()).Table("awards").
		Select("awards.id, awards.project_name, awards.award_level, awards.award_category, awards.award_name, awards.award_time, cd.comp_name, cd.organizer, cdp.participant_type").
		Joins("JOIN comp_directories cd ON cd.id = awards.comp_id AND cd.delete_time IS NULL").
		Joins("LEFT JOIN comp_details cdp ON cdp.comp_id = awards.comp_id").
		Where("awards.delete_time IS NULL").
		Where("awards.status IN ?", awardExportFilterStatus()).
		Order("awards.award_time ASC, cd.comp_name ASC, awards.project_name ASC")
	if year != "" {
		q = q.Where("YEAR(awards.award_time) = ?", year)
	}
	if !scope.IsSchoolAdmin() {
		q = q.Where("EXISTS (SELECT 1 FROM award_members am WHERE am.award_id = awards.id AND am.submitted_college_id = ? AND am.delete_time IS NULL)", *scope.ManagedCollegeID)
	}

	var projects []struct {
		ID              uint
		ProjectName     string
		AwardLevel      string
		AwardCategory   string
		AwardName       string
		AwardTime       *time.Time
		CompName        string
		Organizer       string
		ParticipantType *int8
	}
	if err := q.Find(&projects).Error; err != nil {
		utils.InternalServerError(c, "查询获奖项目失败", err)
		return
	}

	// 一次性取全部成员，按导入顺序（主键序）拼接姓名；学院按出现顺序去重。
	var members []awardExportMember
	if len(projects) > 0 {
		ids := make([]uint, 0, len(projects))
		for _, p := range projects {
			ids = append(ids, p.ID)
		}
		if err := database.DB.WithContext(c.Request.Context()).Table("award_members").
			Select("award_id, name, college").
			Where("award_id IN ? AND delete_time IS NULL", ids).
			Order("id ASC").Find(&members).Error; err != nil {
			utils.InternalServerError(c, "查询获奖学生失败", err)
			return
		}
	}
	membersByAward := map[uint][]awardExportMember{}
	for _, m := range members {
		membersByAward[m.AwardID] = append(membersByAward[m.AwardID], m)
	}

	rows := make([][]interface{}, 0, len(projects))
	for i, p := range projects {
		names, collegesJoined := awardJoinMembers(membersByAward[p.ID])
		level := awardMergedLevel(p.AwardLevel, p.AwardCategory, p.AwardName)
		participant := awardParticipantLabel(p.ParticipantType)
		month := ""
		if p.AwardTime != nil {
			month = p.AwardTime.Format("2006-01")
		}
		rows = append(rows, []interface{}{
			i + 1, p.CompName, p.ProjectName, p.Organizer, level, names, collegesJoined,
			"", "", month, participant,
		})
	}

	title := "大学生参加省级以上各类竞赛获奖项目一览表"
	if year != "" {
		title = fmt.Sprintf("%s年%s", year, title)
	}
	awardExportWorkbook(c, "获奖项目", title,
		[]string{"序号", "赛事名称", "参赛项目名称", "主办单位", "获奖等级", "获奖学生", "学生所在学院", "指导教师", "指导教师所在学院", "获奖时间（年/月）", "团体或个人赛"},
		[]float64{6, 30, 35, 18, 14, 30, 30, 18, 22, 14, 12}, rows)
}

// ExportAwardStudents 导出附件2：按学生一行平铺。
// GET /api/award/export/students?year=2025
func ExportAwardStudents(c *gin.Context) {
	scope, ok := awardExportScope(c)
	if !ok {
		return
	}
	year, ok := awardExportYear(c)
	if !ok {
		return
	}

	q := database.DB.WithContext(c.Request.Context()).Table("award_members am").
		Select("am.student_number, am.name, am.college, am.major, am.remark, a.project_name, a.award_category, a.award_name, a.award_time, cd.comp_name, cd.undertaker").
		Joins("JOIN awards a ON a.id = am.award_id AND a.delete_time IS NULL").
		Joins("JOIN comp_directories cd ON cd.id = a.comp_id AND cd.delete_time IS NULL").
		Where("am.delete_time IS NULL").
		Where("a.status IN ?", awardExportFilterStatus()).
		Order("a.award_time ASC, cd.comp_name ASC, a.project_name ASC, am.id ASC")
	if year != "" {
		q = q.Where("YEAR(a.award_time) = ?", year)
	}
	if !scope.IsSchoolAdmin() {
		q = q.Where("am.submitted_college_id = ?", *scope.ManagedCollegeID)
	}

	var list []struct {
		StudentNumber string
		Name          string
		College       string
		Major         string
		Remark        string
		ProjectName   string
		AwardCategory string
		AwardName     string
		AwardTime     *time.Time
		CompName      string
		Undertaker    string
	}
	if err := q.Find(&list).Error; err != nil {
		utils.InternalServerError(c, "查询获奖学生失败", err)
		return
	}

	rows := make([][]interface{}, 0, len(list))
	for i, s := range list {
		month := ""
		if s.AwardTime != nil {
			month = s.AwardTime.Format("2006-01")
		}
		rows = append(rows, []interface{}{
			i + 1, s.Undertaker, s.StudentNumber, s.Name, s.College, s.Major,
			s.CompName, s.ProjectName, month, s.AwardCategory, s.AwardName, s.Remark,
		})
	}

	title := "大学生参加省级以上各类竞赛获奖学生一览表"
	if year != "" {
		title = fmt.Sprintf("%s年%s", year, title)
	}
	awardExportWorkbook(c, "获奖学生", title,
		[]string{"序号", "承办单位", "学号", "学生姓名", "所属学院", "所在专业", "赛事名称", "参赛项目名称", "获奖时间（年/月）", "获奖类别", "获奖等级", "备注"},
		[]float64{6, 22, 14, 12, 24, 20, 30, 35, 14, 12, 12, 18}, rows)
}
