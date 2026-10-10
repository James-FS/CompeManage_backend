package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var awardImportHeaders = []string{"序号", "学号", "学生姓名", "所属学院", "所在专业", "参赛项目名称", "获奖时间（年/月）", "获奖类别", "获奖等级", "备注"}
var awardTemplateNotes = []string{
	"说明：1. 获奖类别：校赛、国家级、省级、国际级；国际级统一保存为国家级。",
	"2. 获奖等级：指一等奖、二等奖、三等奖，冠军（金奖）、特等奖等同于一等奖，亚军（银奖）等同于二等奖，季军（铜奖）等同于三等奖。如冠亚季军、金银铜奖从一等奖获奖者中决出，则按一等奖计算。",
	"3. 一行只能填写1个学生。学生须为我校全日制本科生（不含继续教育、二级学院），学号务必准确（否则影响奖励发放）。",
	"4. 获奖时间填写 YYYY-MM；同一赛事内项目名称相同视为同一团队，类别、等级、时间必须一致。",
	"5. 所属学院和专业需填写完整名称。",
}

type AwardImportRow struct {
	StudentNumber string `json:"student_id"`
	Name          string `json:"name"`
	College       string `json:"college"`
	Major         string `json:"major"`
	ProjectName   string `json:"project_name"`
	AwardMonth    string `json:"award_month"`
	Category      string `json:"award_category"`
	AwardName     string `json:"award_name"`
	Remark        string `json:"remark"`
	RowNumber     int    `json:"row_number"`
	Error         string `json:"error,omitempty"`
	Existing      bool   `json:"existing"`
	UserID        uint   `json:"-"`
	RegID         *uint  `json:"-"`
	AwardID       uint   `json:"-"`
}

func normalizeAwardCategory(category string) (string, error) {
	switch strings.TrimSpace(category) {
	case "校赛":
		return "校赛", nil
	case "国家级", "国际级":
		return "国家级", nil
	case "省级":
		return "省级", nil
	default:
		return "", errors.New("获奖类别必须为校赛、国家级、省级或国际级")
	}
}

func awardCompetitionScope(q *gorm.DB, scope *UserAccessScope) *gorm.DB {
	if scope.IsCollegeAdmin() {
		if scope.ManagedCollegeID == nil {
			return q.Where("1=0")
		}
		return q.Where("(comp_directories.college_id = ? OR comp_directories.comp_level IN ?)", *scope.ManagedCollegeID, []string{"校级", "校赛"})
	}
	return applyCompetitionScope(q, scope, "comp_directories")
}

func requireAwardCompetition(c *gin.Context, compID uint) (*UserAccessScope, *models.CompDirectory, bool) {
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return nil, nil, false
	}
	if scope.RoleCode != "school_admin" && scope.RoleCode != "college_admin" && scope.RoleCode != "competition_manager" {
		utils.Forbidden(c, "没有获奖填报权限")
		return nil, nil, false
	}
	var comp models.CompDirectory
	if err := awardCompetitionScope(database.DB.WithContext(c.Request.Context()).Model(&models.CompDirectory{}), scope).First(&comp, compID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.Forbidden(c, "赛事不存在或不在可访问范围")
		} else {
			utils.InternalServerError(c, "查询赛事失败", err)
		}
		return nil, nil, false
	}
	return scope, &comp, true
}

func ExportAwardTemplate(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Query("comp_id"), 10, 32)
	if id == 0 {
		utils.BadRequest(c, "缺少有效赛事ID")
		return
	}
	if _, _, ok := requireAwardCompetition(c, uint(id)); !ok {
		return
	}
	f := newAwardTemplate()
	defer f.Close()
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", `attachment; filename="Award_Template.xlsx"`)
	if err := f.Write(c.Writer); err != nil {
		utils.InternalServerError(c, "生成模板失败", err)
	}
}

func newAwardTemplate() *excelize.File {
	f := excelize.NewFile()
	sheet := "Sheet1"
	borders := []excelize.Border{
		{Type: "left", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1},
		{Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1},
	}
	bodyStyle := excelize.Style{Border: borders, Font: &excelize.Font{Family: "宋体", Size: 11, Color: "000000"}, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true}}
	bodyID, _ := f.NewStyle(&bodyStyle)
	header := bodyStyle
	header.Font = &excelize.Font{Family: "宋体", Size: 12, Bold: true, Color: "000000"}
	headerID, _ := f.NewStyle(&header)
	text := bodyStyle
	text.NumFmt = 49
	textID, _ := f.NewStyle(&text)
	project := bodyStyle
	project.Alignment = &excelize.Alignment{Horizontal: "left", Vertical: "center", WrapText: true}
	projectID, _ := f.NewStyle(&project)
	noteID, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Horizontal: "left", WrapText: true, Vertical: "center"}, Font: &excelize.Font{Family: "宋体", Size: 12, Color: "000000"}})
	widths := []float64{7, 15, 12, 23, 21, 38, 15, 13, 13, 10}
	for i, h := range awardImportHeaders {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetCellValue(sheet, col+"1", h)
		f.SetColWidth(sheet, col, col, widths[i])
	}
	f.SetCellStyle(sheet, "A1", "J13", bodyID)
	f.SetCellStyle(sheet, "A1", "J1", headerID)
	f.SetCellStyle(sheet, "B2", "B13", textID)
	f.SetCellStyle(sheet, "G2", "G13", textID)
	f.SetCellStyle(sheet, "F2", "F13", projectID)
	f.SetRowHeight(sheet, 1, 42)
	// 单独划定填写区和说明区，解析以“说明：”为边界。
	for row := 2; row <= 13; row++ {
		f.SetRowHeight(sheet, row, 34)
	}
	f.SetRowHeight(sheet, 14, 12)
	f.SetRowHeight(sheet, 15, 12)
	noteHeights := []float64{32, 52, 44, 36, 32}
	for i, note := range awardTemplateNotes {
		row := 16 + i
		f.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("J%d", row))
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), note)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("J%d", row), noteID)
		// 仅强调说明标题，正文保持正常字重。
		parts := strings.SplitN(note, "：", 2)
		if len(parts) == 2 {
			label := parts[0] + "："
			if i == 0 {
				label = "说明：1. 获奖类别："
				parts[1] = strings.TrimPrefix(note, label)
			}
			f.SetCellRichText(sheet, fmt.Sprintf("A%d", row), []excelize.RichTextRun{{Text: label, Font: &excelize.Font{Family: "宋体", Size: 12, Bold: true}}, {Text: parts[1], Font: &excelize.Font{Family: "宋体", Size: 12}}})
		}
		f.SetRowHeight(sheet, row, noteHeights[i])
	}
	return f
}

func parseAwardSheet(rows [][]string) ([]AwardImportRow, error) {
	if len(rows) == 0 {
		return nil, errors.New("Excel 为空")
	}
	columns := map[string]int{}
	for i, h := range rows[0] {
		columns[strings.TrimSpace(h)] = i
	}
	for _, h := range awardImportHeaders[1:9] {
		if _, ok := columns[h]; !ok {
			return nil, fmt.Errorf("缺少表头 %s，请下载新版模板", h)
		}
	}
	result := []AwardImportRow{}
	for index, row := range rows[1:] {
		if len(row) > 0 && strings.HasPrefix(strings.TrimSpace(row[0]), "说明：") {
			break
		}
		if len(row) > 0 && strings.TrimSpace(row[0]) == "示例" {
			continue
		}
		value := func(h string) string {
			i, ok := columns[h]
			if !ok || i >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[i])
		}
		r := AwardImportRow{RowNumber: index + 2, StudentNumber: value("学号"), Name: value("学生姓名"), College: value("所属学院"), Major: value("所在专业"), ProjectName: value("参赛项目名称"), AwardMonth: value("获奖时间（年/月）"), Category: value("获奖类别"), AwardName: value("获奖等级"), Remark: value("备注")}
		if r.StudentNumber == "" && r.Name == "" && r.ProjectName == "" && r.Category == "" && r.AwardName == "" && r.College == "" && r.Major == "" && r.AwardMonth == "" && r.Remark == "" {
			continue
		}
		result = append(result, r)
	}
	if len(result) == 0 {
		return nil, errors.New("暂无可导入数据")
	}
	return result, nil
}

func readAwardImport(c *gin.Context) (uint, []AwardImportRow, error) {
	id, err := strconv.ParseUint(c.Query("comp_id"), 10, 32)
	if err != nil || id == 0 {
		return 0, nil, errors.New("赛事ID无效")
	}
	if strings.HasPrefix(c.ContentType(), "application/json") {
		var body struct {
			Rows []AwardImportRow `json:"rows"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			return 0, nil, errors.New("数据格式错误")
		}
		if len(body.Rows) == 0 || len(body.Rows) > 5000 {
			return 0, nil, errors.New("每批请提交1至5000行")
		}
		return uint(id), body.Rows, nil
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		return 0, nil, errors.New("文件上传失败")
	}
	defer file.Close()
	f, err := excelize.OpenReader(file)
	if err != nil {
		return 0, nil, errors.New("Excel读取失败")
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		return 0, nil, err
	}
	data, err := parseAwardSheet(rows)
	if len(data) > 5000 {
		return 0, nil, errors.New("每批最多5000行")
	}
	return uint(id), data, err
}

func validateAwardRows(tx *gorm.DB, compID uint, input []AwardImportRow, requireReg bool) ([]AwardImportRow, int, error) {
	result := append([]AwardImportRow(nil), input...)
	bad := 0
	groups := map[string]AwardImportRow{}
	seen := map[string]bool{}
	for i := range result {
		r := &result[i]
		r.Error = ""
		r.Existing = false
		r.UserID = 0
		r.RegID = nil
		r.AwardID = 0
		r.StudentNumber = strings.TrimSpace(r.StudentNumber)
		r.Name = strings.TrimSpace(r.Name)
		r.College = strings.TrimSpace(r.College)
		r.Major = strings.TrimSpace(r.Major)
		r.ProjectName = strings.TrimSpace(r.ProjectName)
		r.AwardMonth = strings.TrimSpace(r.AwardMonth)
		r.AwardName = strings.TrimSpace(r.AwardName)
		if r.RowNumber == 0 {
			r.RowNumber = i + 1
		}
		fail := func(msg string) { r.Error = msg; bad++ }
		if r.StudentNumber == "" || r.Name == "" || r.College == "" || r.ProjectName == "" || r.AwardMonth == "" || r.AwardName == "" {
			fail("学号、姓名、学院、项目、获奖时间和等级不能为空")
			continue
		}
		if utf8.RuneCountInString(r.ProjectName) > 255 || utf8.RuneCountInString(r.AwardName) > 100 || len(r.StudentNumber) > 50 || utf8.RuneCountInString(r.Remark) > 2000 {
			fail("项目、奖项、学号或备注超出长度限制")
			continue
		}
		category, err := normalizeAwardCategory(r.Category)
		if err != nil {
			fail(err.Error())
			continue
		}
		r.Category = category
		if _, err := time.Parse("2006-01", r.AwardMonth); err != nil {
			fail("获奖时间须为 YYYY-MM")
			continue
		}
		key := database.AwardProjectKey(r.ProjectName)
		if first, ok := groups[key]; ok && (first.Category != r.Category || first.AwardName != r.AwardName || first.AwardMonth != r.AwardMonth) {
			fail("同一团队的获奖类别、等级、时间不一致")
			continue
		}
		groups[key] = *r
		duplicateKey := key + "/" + r.StudentNumber
		if seen[duplicateKey] {
			fail("文件内同一团队的学生重复")
			continue
		}
		seen[duplicateKey] = true
		var user models.User
		err = tx.Where("username = ?", r.StudentNumber).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail("学号不存在")
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		if user.Realname != r.Name || user.College != r.College || (r.Major != "" && user.Major != r.Major) {
			fail("姓名、学院或专业与系统学生资料不一致")
			continue
		}
		r.UserID = user.ID
		r.Major = user.Major
		var ids []uint
		err = tx.Model(&models.Register{}).Where("comp_id = ? AND status IN ?", compID, []int{1, 4}).Where("leader_id = ? OR EXISTS (SELECT 1 FROM reg_members WHERE reg_members.reg_id = registers.id AND reg_members.username = ?)", user.ID, user.Username).Pluck("id", &ids).Error
		if err != nil {
			return nil, 0, err
		}
		if requireReg && len(ids) == 0 {
			fail("未找到目标赛事审核通过的报名")
			continue
		}
		if len(ids) == 1 {
			r.RegID = &ids[0]
		}

		var awards []models.Award
		err = tx.Where("comp_id = ? AND (project_key = ? OR (project_key IS NULL AND BINARY project_name = ?))", compID, key, r.ProjectName).Find(&awards).Error
		if err != nil {
			return nil, 0, err
		}
		if len(awards) > 1 {
			fail("历史同名奖项冲突，请校级管理员先更正项目名称")
			continue
		}
		if len(awards) == 1 {
			a := awards[0]
			r.AwardID = a.ID
			month := ""
			if a.AwardTime != nil {
				month = a.AwardTime.Format("2006-01")
			}
			if a.AwardCategory != r.Category || a.AwardName != r.AwardName || month != r.AwardMonth {
				fail("与已有团队的类别、等级或时间冲突，请先更正团队奖项")
				continue
			}
			if a.Status != "approved" {
				fail("该团队已有待审核或驳回奖项，不能通过导入覆盖")
				continue
			}
			var count int64
			if err := tx.Model(&models.AwardMember{}).Where("award_id = ? AND student_id = ?", a.ID, user.ID).Count(&count).Error; err != nil {
				return nil, 0, err
			}
			r.Existing = count > 0
		}
	}
	return result, bad, nil
}

func getAwardSetting(tx *gorm.DB, lock bool) (models.AwardImportSetting, error) {
	var setting models.AwardImportSetting
	q := tx
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(&setting, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.AwardImportSetting{ID: 1}, nil
	}
	return setting, err
}

func PreviewAwardImport(c *gin.Context) {
	compID, rows, err := readAwardImport(c)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	if _, _, ok := requireAwardCompetition(c, compID); !ok {
		return
	}
	db := database.DB.WithContext(c.Request.Context())
	setting, err := getAwardSetting(db, false)
	if err != nil {
		utils.InternalServerError(c, "读取报名要求失败", err)
		return
	}
	result, bad, err := validateAwardRows(db, compID, rows, setting.RequireRegistration)
	if err != nil {
		utils.InternalServerError(c, "预校验失败", err)
		return
	}
	utils.Success(c, gin.H{"rows": result, "error_count": bad, "require_registration": setting.RequireRegistration})
}

func ImportAward(c *gin.Context) {
	compID, input, err := readAwardImport(c)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	scope, _, ok := requireAwardCompetition(c, compID)
	if !ok {
		return
	}
	var rows []AwardImportRow
	bad, created, existing, teams := 0, 0, 0, 0
	err = database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		setting, err := getAwardSetting(tx, true)
		if err != nil {
			return err
		}
		var comp models.CompDirectory
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&comp, compID).Error; err != nil {
			return err
		}
		// 提交时再次检查，不能使用预览阶段的权限或报名结果。
		if !scope.IsSchoolAdmin() && !(scope.IsCollegeAdmin() && scope.ManagedCollegeID != nil && ((comp.CollegeID != nil && *comp.CollegeID == *scope.ManagedCollegeID) || comp.CompLevel == "校级" || comp.CompLevel == "校赛")) && !(scope.RoleCode == "competition_manager" && comp.ManagerID == scope.UserID) {
			return errors.New("赛事权限已变化")
		}
		rows, bad, err = validateAwardRows(tx, compID, input, setting.RequireRegistration)
		if err != nil {
			return err
		}
		if bad > 0 {
			return errors.New("数据校验未通过")
		}
		groupIDs := map[string]uint{}
		for _, r := range rows {
			key := database.AwardProjectKey(r.ProjectName)
			awardID := groupIDs[key]
			if awardID == 0 {
				awardID = r.AwardID
				if awardID == 0 {
					month, _ := time.Parse("2006-01", r.AwardMonth)
					now := time.Now()
					a := models.Award{MembersMigrated: true, CompID: compID, ProjectName: r.ProjectName, ProjectKey: &key, AwardCategory: r.Category, AwardName: r.AwardName, AwardLevel: r.Category + r.AwardName, AwardTime: &month, Status: "approved", Source: "import", AuditTime: &now, LevelRank: 99}
					if err := tx.Create(&a).Error; err != nil {
						return err
					}
					awardID = a.ID
				} else {
					if err := tx.Model(&models.Award{}).Where("id = ?", awardID).Update("project_key", key).Error; err != nil {
						return err
					}
				}
				groupIDs[key] = awardID
			}
			if r.Existing {
				existing++
				continue
			}
			uid := scope.UserID
			m := models.AwardMember{AwardID: awardID, StudentID: r.UserID, RegID: r.RegID, Name: r.Name, StudentNumber: r.StudentNumber, College: r.College, Major: r.Major, Remark: r.Remark, SubmittedCollegeID: scope.ManagedCollegeID, SubmittedBy: &uid}
			if err := tx.Create(&m).Error; err != nil {
				return err
			}
			if r.RegID != nil {
				if err := tx.Model(&models.Register{}).Where("id = ? AND comp_id = ? AND status IN ?", *r.RegID, compID, []int{1, 4}).Update("status", 4).Error; err != nil {
					return err
				}
			}
			created++
		}
		teams = len(groupIDs)
		return nil
	})
	if err != nil {
		if bad > 0 {
			c.JSON(http.StatusBadRequest, utils.Response{Code: 400, Message: "数据校验未通过，未写入任何数据", Data: gin.H{"rows": rows, "error_count": bad}, Timestamp: time.Now().Unix()})
		} else {
			utils.InternalServerError(c, "导入失败，整批已回滚", err)
		}
		return
	}
	clearAwardCache(c.Request.Context(), compID)
	utils.SuccessWithMessage(c, fmt.Sprintf("处理%d名学生、%d个团队，新增%d人，已存在%d人", len(rows), teams, created, existing), gin.H{"success_count": created, "existing_count": existing, "student_count": len(rows), "team_count": teams, "fail_count": 0})
}

func GetAwardImportSetting(c *gin.Context) {
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return
	}
	if scope.RoleCode != "school_admin" && scope.RoleCode != "college_admin" && scope.RoleCode != "competition_manager" {
		utils.Forbidden(c, "没有填报权限")
		return
	}
	setting, err := getAwardSetting(database.DB.WithContext(c.Request.Context()), false)
	if err != nil {
		utils.InternalServerError(c, "读取设置失败", err)
		return
	}
	utils.Success(c, setting)
}
func UpdateAwardImportSetting(c *gin.Context) {
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return
	}
	if !scope.IsSchoolAdmin() {
		utils.Forbidden(c, "仅校级管理员可调整报名要求")
		return
	}
	var req struct {
		RequireRegistration *bool `json:"require_registration"`
	}
	if c.ShouldBindJSON(&req) != nil || req.RequireRegistration == nil {
		utils.BadRequest(c, "报名要求必填")
		return
	}
	setting := models.AwardImportSetting{ID: 1, RequireRegistration: *req.RequireRegistration}
	if err := database.DB.WithContext(c.Request.Context()).Save(&setting).Error; err != nil {
		utils.InternalServerError(c, "保存设置失败", err)
		return
	}
	utils.Success(c, setting)
}

func canEditAwardMember(scope *UserAccessScope, m models.AwardMember) bool {
	return scope.IsSchoolAdmin() || (scope.IsCollegeAdmin() && scope.ManagedCollegeID != nil && m.SubmittedCollegeID != nil && *scope.ManagedCollegeID == *m.SubmittedCollegeID)
}
func canEditAwardTeam(tx *gorm.DB, scope *UserAccessScope, awardID uint) (bool, error) {
	if scope.IsSchoolAdmin() {
		return true, nil
	}
	if !scope.IsCollegeAdmin() || scope.ManagedCollegeID == nil {
		return false, nil
	}
	var college models.College
	if err := tx.First(&college, *scope.ManagedCollegeID).Error; err != nil {
		return false, err
	}
	var count int64
	err := tx.Model(&models.AwardMember{}).Joins("JOIN users ON users.id = award_members.student_id AND users.delete_time IS NULL").Where("award_id = ? AND users.college = ?", awardID, college.Name).Count(&count).Error
	return count > 0, err
}

func UpdateAwardMember(c *gin.Context) {
	id, parseErr := strconv.ParseUint(c.Param("id"), 10, 32)
	if parseErr != nil || id == 0 {
		utils.BadRequest(c, "成员ID无效")
		return
	}
	db := database.DB.WithContext(c.Request.Context())
	var m models.AwardMember
	if err := db.First(&m, id).Error; err != nil {
		utils.NotFound(c, "成员不存在")
		return
	}
	var a models.Award
	if err := db.First(&a, m.AwardID).Error; err != nil {
		utils.NotFound(c, "奖项不存在")
		return
	}
	scope, _, ok := requireAwardCompetition(c, a.CompID)
	if !ok {
		return
	}
	var req struct {
		StudentNumber      string `json:"student_id"`
		Remark             string `json:"remark"`
		SubmittedCollegeID *uint  `json:"submitted_college_id"`
	}
	if c.ShouldBindJSON(&req) != nil || utf8.RuneCountInString(req.Remark) > 2000 {
		utils.BadRequest(c, "备注无效")
		return
	}
	denied := false
	invalid := ""
	err := db.Transaction(func(tx *gorm.DB) error {
		var comp models.CompDirectory
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&comp, a.CompID).Error; err != nil {
			return err
		}
		if err := tx.First(&m, id).Error; err != nil {
			return err
		}
		if !canEditAwardMember(scope, m) {
			denied = true
			return errors.New("仅能修改本院填报成员")
		}
		updates := map[string]interface{}{"remark": req.Remark}
		if req.SubmittedCollegeID != nil {
			if !scope.IsSchoolAdmin() {
				denied = true
				return errors.New("仅校管理员可确认归属")
			}
			if m.SubmittedCollegeID != nil && *m.SubmittedCollegeID != *req.SubmittedCollegeID {
				invalid = "已有填报归属不能转移"
				return errors.New(invalid)
			}
			var college models.College
			if tx.First(&college, *req.SubmittedCollegeID).Error != nil {
				invalid = "学院不存在"
				return errors.New(invalid)
			}
			updates["submitted_college_id"] = *req.SubmittedCollegeID
		}
		if req.StudentNumber != "" && strings.TrimSpace(req.StudentNumber) != m.StudentNumber {
			var user models.User
			if err := tx.Where("username = ?", strings.TrimSpace(req.StudentNumber)).First(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					invalid = "学号不存在"
				}
				return err
			}
			var count int64
			if err := tx.Model(&models.AwardMember{}).Where("award_id = ? AND student_id = ? AND id <> ?", a.ID, user.ID, m.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				invalid = "该学生已存在，不能覆盖其他填报记录"
				return errors.New(invalid)
			}
			updates["student_id"] = user.ID
			updates["student_number"] = user.Username
			updates["name"] = user.Realname
			updates["college"] = user.College
			updates["major"] = user.Major
			updates["reg_id"] = nil
		}
		return tx.Model(&m).Updates(updates).Error
	})
	if denied {
		utils.Forbidden(c, "只能修改本院填报成员，历史归属须由校管理员确认")
		return
	}
	if invalid != "" {
		utils.BadRequest(c, invalid)
		return
	}
	if err != nil {
		utils.InternalServerError(c, "保存失败", err)
		return
	}
	clearAwardCache(c.Request.Context(), a.CompID)
	utils.Success(c, nil)
}

func UpdateAwardTeam(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	db := database.DB.WithContext(c.Request.Context())
	var a models.Award
	if db.First(&a, id).Error != nil {
		utils.NotFound(c, "奖项不存在")
		return
	}
	scope, _, ok := requireAwardCompetition(c, a.CompID)
	if !ok {
		return
	}
	var req struct {
		ProjectName string `json:"project_name"`
		Category    string `json:"award_category"`
		AwardName   string `json:"award_name"`
		Month       string `json:"award_month"`
	}
	if c.ShouldBindJSON(&req) != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	req.ProjectName = strings.TrimSpace(req.ProjectName)
	req.AwardName = strings.TrimSpace(req.AwardName)
	category, err := normalizeAwardCategory(req.Category)
	month, dateErr := time.Parse("2006-01", req.Month)
	if err != nil || dateErr != nil || req.ProjectName == "" || req.AwardName == "" || utf8.RuneCountInString(req.ProjectName) > 255 || utf8.RuneCountInString(req.AwardName) > 100 {
		utils.BadRequest(c, "项目、类别、等级或年月无效")
		return
	}
	forbidden := false
	conflict := false
	err = db.Transaction(func(tx *gorm.DB) error {
		var comp models.CompDirectory
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&comp, a.CompID).Error; err != nil {
			return err
		}
		allowed, err := canEditAwardTeam(tx, scope, a.ID)
		if err != nil {
			return err
		}
		if !allowed {
			forbidden = true
			return errors.New("无权限")
		}
		key := database.AwardProjectKey(req.ProjectName)
		var count int64
		if err := tx.Model(&models.Award{}).Where("comp_id = ? AND id <> ? AND (project_key = ? OR BINARY project_name = ?)", a.CompID, a.ID, key, req.ProjectName).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			conflict = true
			return errors.New("项目名称已存在")
		}
		return tx.Model(&a).Updates(map[string]interface{}{"project_name": req.ProjectName, "project_key": key, "award_category": category, "award_name": req.AwardName, "award_level": category + req.AwardName, "award_time": month}).Error
	})
	if forbidden {
		utils.Forbidden(c, "仅团队成员所在学院的管理员或校级管理员可修改团队奖项")
		return
	}
	if conflict {
		utils.BadRequest(c, "该赛事已存在同名项目，请勿合并覆盖")
		return
	}
	if err != nil {
		utils.InternalServerError(c, "修改失败", err)
		return
	}
	clearAwardCache(c.Request.Context(), a.CompID)
	utils.Success(c, nil)
}
