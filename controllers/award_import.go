package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"encoding/json"
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

var awardImportHeaders = []string{"序号", "学号", "学生姓名", "所属学院", "所在专业", "参赛项目名称", "获奖时间（年/月）", "获奖类别", "获奖等级", "指导教师工号", "指导教师所在学院", "备注"}
var awardTemplateHeaders = []string{"序号", "学号", "学生姓名", "所属学院", "所在专业", "参赛项目名称", "获奖时间（年/月）", "获奖类别", "获奖等级", "指导教师工号", "备注"}
var awardTemplateNotes = []string{
	"说明：1. 获奖类别：校赛、国家级、省级、国际级；国际级统一保存为国家级。",
	"2. 获奖等级：指一等奖、二等奖、三等奖，冠军（金奖）、特等奖等同于一等奖，亚军（银奖）等同于二等奖，季军（铜奖）等同于三等奖。如冠亚季军、金银铜奖从一等奖获奖者中决出，则按一等奖计算。",
	"3. 一行只能填写1个学生。学生须为我校全日制本科生（不含继续教育、二级学院），学号务必准确（否则影响奖励发放）。",
	"4. 获奖时间填写 YYYY-MM 或 YYYY.MM；日期版模板中的 YYYY-MM-DD 也可导入并按月份保存。同一赛事内项目名称相同视为同一团队，类别、等级、时间必须一致。",
	"5. 所属学院和专业需填写完整名称。",
	"6. 指导教师只需填写教师工号；多人可用逗号、空格、顿号或分号分隔，系统按工号自动获取姓名和学院，无需填写教师所在学院；工号须为系统在职教师，不填表示无指导教师。",
	"7. 同个团队的参赛项目名称一定要一样：同一赛事内项目名称相同才会归为同一团队，名称不同会被拆成不同团队。",
}

type AwardImportRow struct {
	StudentNumber    string               `json:"student_id"`
	Name             string               `json:"name"`
	College          string               `json:"college"`
	Major            string               `json:"major"`
	ProjectName      string               `json:"project_name"`
	AwardMonth       string               `json:"award_month"`
	Category         string               `json:"award_category"`
	AwardName        string               `json:"award_name"`
	AdvisorUsernames string               `json:"advisor_usernames"` // 空格分隔的指导教师工号
	AdvisorColleges  string               `json:"advisor_colleges"`  // 空格分隔的指导教师所在学院（归属校验用）
	Resolved         []models.AdvisorInfo `json:"-"`                 // 校验通过后的系统三元组快照
	Remark           string               `json:"remark"`
	RowNumber        int                  `json:"row_number"`
	Error            string               `json:"error,omitempty"`
	Existing         bool                 `json:"existing"`
	UserID           uint                 `json:"-"`
	RegID            *uint                `json:"-"`
	AwardID          uint                 `json:"-"`
	NeedsProjectKey  bool                 `json:"-"`
}

// normalizeAdvisorList 把常见中英文标点和空白分隔的教师工号规范化为单空格连接。
func normalizeAdvisorList(s string) string {
	for _, separator := range []string{",", "，", "、", ";", "；", "|"} {
		s = strings.ReplaceAll(s, separator, " ")
	}
	return strings.Join(strings.Fields(s), " ")
}

// findAwardTeacher 按工号查系统教师，口径与指导教师候选接口（role=teacher）一致：
// identity_type=staff 且非校管/院管/专家/访客角色。查不到即视为非法工号。
func findAwardTeacher(tx *gorm.DB, username string) (models.User, error) {
	var user models.User
	err := awardTeacherCandidates(tx.Model(&models.User{})).Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.User{}, fmt.Errorf("指导教师工号 %s 不存在或不是可选教师", username)
	}
	return user, err
}

func awardTeacherCandidates(tx *gorm.DB) *gorm.DB {
	return tx.Where("users.delete_time IS NULL AND identity_type = 'staff'").
		Where(`NOT EXISTS (
			SELECT 1 FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id AND r.delete_time IS NULL
		WHERE ur.user_id = users.id AND r.role_code IN ('school_admin', 'college_admin', 'expert', 'guest'))`)
}

// resolveAwardAdvisors 按工号解析指导教师（任务6/8）：
// 工号为唯一标识（同名/多工号均可区分），姓名与学院一律取系统资料；
// 所在学院列若填写，须与教师系统归属学院一致，否则报错。
func resolveAwardAdvisors(tx *gorm.DB, usernames, colleges string) ([]models.AdvisorInfo, error) {
	return resolveAwardAdvisorsWithLookup(usernames, colleges, func(username string) (models.User, error) {
		return findAwardTeacher(tx, username)
	})
}

func resolveAwardAdvisorsWithLookup(usernames, colleges string, lookup func(string) (models.User, error)) ([]models.AdvisorInfo, error) {
	uList := strings.Fields(usernames)
	if len(uList) == 0 {
		return nil, nil
	}
	cList := strings.Fields(colleges)
	pairs := make([]models.AdvisorInfo, 0, len(uList))
	seen := map[string]bool{}
	for i, username := range uList {
		if seen[username] {
			return nil, fmt.Errorf("指导教师工号 %s 重复填写", username)
		}
		seen[username] = true
		user, err := lookup(username)
		if err != nil {
			return nil, err
		}
		if i < len(cList) && cList[i] != "" && cList[i] != user.College {
			return nil, fmt.Errorf("指导教师 %s 归属学院不符（系统：%s）", username, user.College)
		}
		pairs = append(pairs, models.AdvisorInfo{Username: user.Username, Name: user.Realname, College: user.College})
	}
	return pairs, nil
}

// advisorsMirrorJSON 三元组镜像到 awards.advisor_info，保持历史消费方兼容。
func advisorsMirrorJSON(pairs []models.AdvisorInfo) string {
	if len(pairs) == 0 {
		return ""
	}
	data, err := json.Marshal(pairs)
	if err != nil {
		return ""
	}
	return string(data)
}

// fillAwardAdvisorsIfEmpty 为尚无指导教师的奖项写入多行数据并镜像 JSON（幂等，事务内调用）。
func fillAwardAdvisorsIfEmpty(tx *gorm.DB, awardID uint, pairs []models.AdvisorInfo) error {
	if len(pairs) == 0 {
		return nil
	}
	var count int64
	if err := tx.Model(&models.AwardAdvisor{}).Where("award_id = ?", awardID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for i, pair := range pairs {
		row := models.AwardAdvisor{AwardID: awardID, Username: pair.Username, Name: pair.Name, College: pair.College, Sort: i}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return tx.Model(&models.Award{}).
		Where("id = ? AND (advisor_info IS NULL OR advisor_info IN ('', '[]'))", awardID).
		Update("advisor_info", advisorsMirrorJSON(pairs)).Error
}

// replaceAwardAdvisors 整体替换项目指导教师（弹窗选择提交，任务7）：
// 忽略客户端提交的姓名/学院，以工号回查系统资料重建三元组。
func replaceAwardAdvisors(tx *gorm.DB, awardID uint, edits []AwardAdvisorEditReq) error {
	pairs := make([]models.AdvisorInfo, 0, len(edits))
	seen := map[string]bool{}
	for _, e := range edits {
		username := strings.TrimSpace(e.Username)
		if username == "" || seen[username] {
			continue
		}
		seen[username] = true
		user, err := findAwardTeacher(tx, username)
		if err != nil {
			return err
		}
		pairs = append(pairs, models.AdvisorInfo{Username: user.Username, Name: user.Realname, College: user.College})
	}
	// 指导教师为快照数据：物理删除，否则软删记录仍占唯一索引(award_id+username)导致重建冲突
	if err := tx.Unscoped().Where("award_id = ?", awardID).Delete(&models.AwardAdvisor{}).Error; err != nil {
		return err
	}
	for i, pair := range pairs {
		row := models.AwardAdvisor{AwardID: awardID, Username: pair.Username, Name: pair.Name, College: pair.College, Sort: i}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return tx.Model(&models.Award{}).Where("id = ?", awardID).
		Update("advisor_info", advisorsMirrorJSON(pairs)).Error
}

// AwardAdvisorEditReq 弹窗提交的指导教师条目（仅 username 参与校验）。
type AwardAdvisorEditReq struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	College  string `json:"college"`
}

// advisorPairsJSON serializes legacy name/college columns for consumers that
// still read the advisor_info JSON snapshot.
func advisorPairsJSON(names, colleges string) string {
	nameList := strings.Fields(names)
	if len(nameList) == 0 {
		return ""
	}
	collegeList := strings.Fields(colleges)
	pairs := make([]models.AdvisorInfo, 0, len(nameList))
	for i, name := range nameList {
		college := ""
		if i < len(collegeList) {
			college = collegeList[i]
		}
		pairs = append(pairs, models.AdvisorInfo{Name: name, College: college})
	}
	data, err := json.Marshal(pairs)
	if err != nil {
		return ""
	}
	return string(data)
}

// advisorInfoForImport keeps the advisor_info JSON column valid when the
// optional teacher columns are empty.
func advisorInfoForImport(names, colleges string) string {
	if data := advisorPairsJSON(names, colleges); data != "" {
		return data
	}
	return "[]"
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

func normalizeAwardName(name string) (string, error) {
	name = strings.NewReplacer(" ", "", "\t", "", "\u3000", "", "（", "(", "）", ")").Replace(strings.TrimSpace(name))
	if strings.Contains(name, "一等奖") || strings.Contains(name, "一等") || strings.Contains(name, "冠军") || strings.Contains(name, "金奖") || strings.Contains(name, "特等奖") {
		return "一等奖", nil
	}
	if strings.Contains(name, "二等奖") || strings.Contains(name, "二等") || strings.Contains(name, "亚军") || strings.Contains(name, "银奖") {
		return "二等奖", nil
	}
	if strings.Contains(name, "三等奖") || strings.Contains(name, "三等") || strings.Contains(name, "季军") || strings.Contains(name, "铜奖") {
		return "三等奖", nil
	}
	return "", errors.New("获奖等级必须为一等奖、二等奖或三等奖；冠军（金奖）、特等奖按一等奖，亚军（银奖）按二等奖，季军（铜奖）按三等奖")
}

func parseAwardImportDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if month, err := time.Parse("2006-01", value); err == nil {
		return month, nil
	}
	if month, err := time.Parse("2006.01", value); err == nil {
		return month, nil
	}
	if date, err := time.Parse("2006-01-02", value); err == nil {
		return time.Parse("2006-01", date.Format("2006-01"))
	}
	return time.Time{}, errors.New("获奖时间须为 YYYY-MM 或 YYYY.MM")
}

func awardCompetitionScope(q *gorm.DB, scope *UserAccessScope) *gorm.DB {
	if scope.IsCollegeAdmin() {
		if scope.ManagedCollegeID == nil {
			return q.Where("1=0")
		}
		return q.Where("(comp_directories.college_id = ? OR comp_directories.college_id IS NULL OR comp_directories.college_id = 0 OR comp_directories.comp_level IN ?)", *scope.ManagedCollegeID, []string{"校级", "校赛"})
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
	widths := []float64{7, 15, 12, 23, 21, 38, 15, 13, 13, 24, 14}
	for i, h := range awardTemplateHeaders {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetCellValue(sheet, col+"1", h)
		f.SetColWidth(sheet, col, col, widths[i])
	}
	f.SetCellStyle(sheet, "A1", "K13", bodyID)
	f.SetCellStyle(sheet, "A1", "K1", headerID)
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
	noteHeights := []float64{32, 52, 44, 36, 32, 52, 48}
	for i, note := range awardTemplateNotes {
		row := 16 + i
		f.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("K%d", row))
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), note)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("K%d", row), noteID)
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
		if h == "获奖时间（年/月）" {
			if _, ok := columns[h]; !ok {
				if _, dateTemplateOK := columns["获奖时间（年/月/日）"]; !dateTemplateOK {
					return nil, fmt.Errorf("缺少表头 %s，请下载新版模板", h)
				}
			}
			continue
		}
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
		awardDate := value("获奖时间（年/月）")
		if awardDate == "" {
			awardDate = value("获奖时间（年/月/日）")
		}
		r := AwardImportRow{RowNumber: index + 2, StudentNumber: value("学号"), Name: value("学生姓名"), College: value("所属学院"), Major: value("所在专业"), ProjectName: value("参赛项目名称"), AwardMonth: awardDate, Category: value("获奖类别"), AwardName: value("获奖等级"), AdvisorUsernames: normalizeAdvisorList(value("指导教师工号")), AdvisorColleges: normalizeAdvisorList(value("指导教师所在学院")), Remark: value("备注")}
		if r.StudentNumber == "" && r.Name == "" && r.ProjectName == "" && r.Category == "" && r.AwardName == "" && r.College == "" && r.Major == "" && r.AwardMonth == "" && r.AdvisorUsernames == "" && r.AdvisorColleges == "" && r.Remark == "" {
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
	lookup, err := loadAwardImportLookup(tx, compID, input)
	if err != nil {
		return nil, 0, err
	}
	result := append([]AwardImportRow(nil), input...)
	bad := 0
	groups := map[string]AwardImportRow{}
	seen := map[string]bool{}
	advisorResolveCache := map[string][]models.AdvisorInfo{}
	seenStudents := map[uint]bool{}
	for i := range result {
		r := &result[i]
		r.Error = ""
		r.Existing = false
		r.UserID = 0
		r.RegID = nil
		r.AwardID = 0
		r.NeedsProjectKey = false
		r.Resolved = nil
		r.StudentNumber = strings.TrimSpace(r.StudentNumber)
		r.Name = strings.TrimSpace(r.Name)
		r.College = strings.TrimSpace(r.College)
		r.Major = strings.TrimSpace(r.Major)
		r.ProjectName = strings.TrimSpace(r.ProjectName)
		r.AwardMonth = strings.TrimSpace(r.AwardMonth)
		r.AwardName = strings.TrimSpace(r.AwardName)
		r.AdvisorUsernames = normalizeAdvisorList(r.AdvisorUsernames)
		r.AdvisorColleges = normalizeAdvisorList(r.AdvisorColleges)
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
		awardName, err := normalizeAwardName(r.AwardName)
		if err != nil {
			fail(err.Error())
			continue
		}
		r.AwardName = awardName
		awardDate, err := parseAwardImportDate(r.AwardMonth)
		if err != nil {
			fail(err.Error())
			continue
		}
		r.AwardMonth = awardDate.Format("2006-01")
		if r.AdvisorUsernames != "" {
			cacheKey := r.AdvisorUsernames + "\x00" + r.AdvisorColleges
			if pairs, ok := advisorResolveCache[cacheKey]; ok {
				r.Resolved = pairs
			} else {
				pairs, err := resolveAwardAdvisorsWithLookup(r.AdvisorUsernames, r.AdvisorColleges, lookup.teacher)
				if err != nil {
					fail(err.Error())
					continue
				}
				advisorResolveCache[cacheKey] = pairs
				r.Resolved = pairs
			}
		}
		key := database.AwardProjectKey(r.ProjectName)
		if first, ok := groups[key]; ok && (first.Category != r.Category || first.AwardName != r.AwardName || first.AwardMonth != r.AwardMonth || first.AdvisorUsernames != r.AdvisorUsernames) {
			fail("同一团队的获奖类别、等级、时间或指导教师不一致")
			continue
		}
		groups[key] = *r
		duplicateKey := key + "/" + r.StudentNumber
		if seen[duplicateKey] {
			fail("文件内同一团队的学生重复")
			continue
		}
		seen[duplicateKey] = true
		user, found := lookup.users[r.StudentNumber]
		if !found {
			fail("学号不存在")
			continue
		}
		mismatches := []string{}
		if user.Realname != r.Name {
			mismatches = append(mismatches, "学生姓名与系统学号不匹配")
		}
		if user.College != r.College {
			mismatches = append(mismatches, "学生学院与系统学号不匹配")
		}
		if r.Major != "" && user.Major != r.Major {
			mismatches = append(mismatches, "学生专业与系统学号不匹配")
		}
		if len(mismatches) > 0 {
			fail(strings.Join(mismatches, "；"))
			continue
		}
		r.UserID = user.ID
		r.Major = user.Major
		if seenStudents[user.ID] {
			fail("同一赛事中，每名学生只能申报一条获奖记录")
			continue
		}
		seenStudents[user.ID] = true
		ids := lookup.registrations[user.ID]
		if requireReg && len(ids) == 0 {
			fail("未找到目标赛事审核通过的报名")
			continue
		}
		if len(ids) == 1 {
			for id := range ids {
				r.RegID = &id
			}
		}

		awards := append([]models.Award(nil), lookup.awards[key]...)
		awards = append(awards, lookup.legacyAwards[r.ProjectName]...)
		if len(awards) > 1 {
			fail("历史同名奖项冲突，请校级管理员先更正项目名称")
			continue
		}
		if len(awards) == 1 {
			a := awards[0]
			r.AwardID = a.ID
			r.NeedsProjectKey = a.ProjectKey == nil
			month := ""
			existingAwardName, nameErr := normalizeAwardName(a.AwardName)
			if nameErr != nil {
				existingAwardName = a.AwardName
			}
			if a.AwardTime != nil {
				month = a.AwardTime.Format("2006-01")
			}
			if a.AwardCategory != r.Category || existingAwardName != r.AwardName || month != r.AwardMonth {
				fail("与已有团队的类别、等级或时间冲突，请先更正团队奖项")
				continue
			}
			if r.AdvisorUsernames != "" {
				var stored []models.AdvisorInfo
				_ = json.Unmarshal([]byte(a.AdvisorInfo), &stored)
				storedKeys := make([]string, 0, len(stored))
				allKeyed := len(stored) > 0
				for _, adv := range stored {
					if strings.TrimSpace(adv.Username) == "" {
						allKeyed = false
					}
					key := strings.TrimSpace(adv.Username)
					if key == "" {
						key = strings.TrimSpace(adv.Name)
					}
					if key != "" {
						storedKeys = append(storedKeys, key)
					}
				}
				// 仅当历史数据均为工号键时才比对；旧自由文本数据不拦截，交由编辑维护
				if allKeyed && strings.Join(storedKeys, " ") != r.AdvisorUsernames {
					fail("与已有团队的指导教师不一致，请先更正团队奖项")
					continue
				}
			}
			if a.Status != "approved" {
				fail("该团队已有待审核或驳回奖项，不能通过导入覆盖")
				continue
			}
			r.Existing = lookup.memberships[user.ID][a.ID]
		}
		excludedAwardID := uint(0)
		if r.Existing {
			excludedAwardID = r.AwardID
		}
		otherMembership := false
		for awardID := range lookup.memberships[user.ID] {
			if awardID != excludedAwardID {
				otherMembership = true
				break
			}
		}
		if otherMembership {
			fail("该学生在本赛事已有其他获奖记录")
			continue
		}
	}
	return result, bad, nil
}

// countOtherAwardMemberships checks the event-wide student uniqueness rule.
// excludeMemberID is used when correcting a member; excludeAwardID is used
// when validating an idempotent import or resubmitting a rejected award.
func countOtherAwardMemberships(tx *gorm.DB, compID, studentID, excludeMemberID, excludeAwardID uint) (int64, error) {
	query := tx.Model(&models.AwardMember{}).
		Joins("JOIN awards ON awards.id = award_members.award_id").
		Where("awards.comp_id = ? AND awards.delete_time IS NULL AND award_members.student_id = ?", compID, studentID)
	if excludeMemberID != 0 {
		query = query.Where("award_members.id <> ?", excludeMemberID)
	}
	if excludeAwardID != 0 {
		query = query.Where("award_members.award_id <> ?", excludeAwardID)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
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
		if !scope.IsSchoolAdmin() && !(scope.IsCollegeAdmin() && scope.ManagedCollegeID != nil && (comp.CollegeID == nil || *comp.CollegeID == 0 || *comp.CollegeID == *scope.ManagedCollegeID || comp.CompLevel == "校级" || comp.CompLevel == "校赛")) && !(scope.RoleCode == "competition_manager" && comp.ManagerID == scope.UserID) {
			return errors.New("赛事权限已变化")
		}
		rows, bad, err = validateAwardRows(tx, compID, input, setting.RequireRegistration)
		if err != nil {
			return err
		}
		if bad > 0 {
			return errors.New("数据校验未通过")
		}
		created, existing, teams, err = writeAwardImportRows(tx, scope, compID, rows)
		return err
	})
	if err != nil {
		if bad > 0 {
			c.JSON(http.StatusBadRequest, utils.Response{Code: 400, Message: "数据校验未通过", Data: gin.H{"rows": rows, "error_count": bad}, Timestamp: time.Now().Unix()})
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
			count, err := countOtherAwardMemberships(tx, a.CompID, user.ID, m.ID, 0)
			if err != nil {
				return err
			}
			if count > 0 {
				invalid = "该学生在本赛事已有其他获奖记录，不能覆盖其他填报记录"
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
		ProjectName string                 `json:"project_name"`
		Category    string                 `json:"award_category"`
		AwardName   string                 `json:"award_name"`
		Month       string                 `json:"award_month"`
		Advisors    *[]AwardAdvisorEditReq `json:"advisors"` // 指导教师（弹窗提交）；nil=不改动
	}
	if c.ShouldBindJSON(&req) != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	req.ProjectName = strings.TrimSpace(req.ProjectName)
	req.AwardName = strings.TrimSpace(req.AwardName)
	category, err := normalizeAwardCategory(req.Category)
	month, dateErr := parseAwardImportDate(req.Month)
	if err != nil || dateErr != nil || req.ProjectName == "" || req.AwardName == "" || utf8.RuneCountInString(req.ProjectName) > 255 || utf8.RuneCountInString(req.AwardName) > 100 {
		utils.BadRequest(c, "项目、类别、等级或年月无效")
		return
	}
	forbidden := false
	conflict := false
	advisorInvalid := false
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
		if err := tx.Model(&a).Updates(map[string]interface{}{"project_name": req.ProjectName, "project_key": key, "award_category": category, "award_name": req.AwardName, "award_level": category + req.AwardName, "award_time": month}).Error; err != nil {
			return err
		}
		if req.Advisors != nil {
			if err := replaceAwardAdvisors(tx, a.ID, *req.Advisors); err != nil {
				advisorInvalid = true
				return err
			}
		}
		return nil
	})
	if forbidden {
		utils.Forbidden(c, "仅团队成员所在学院的管理员或校级管理员可修改团队奖项")
		return
	}
	if conflict {
		utils.BadRequest(c, "该赛事已存在同名项目，请勿合并覆盖")
		return
	}
	if advisorInvalid {
		utils.BadRequest(c, err.Error())
		return
	}
	if err != nil {
		utils.InternalServerError(c, "修改失败", err)
		return
	}
	clearAwardCache(c.Request.Context(), a.CompID)
	utils.Success(c, nil)
}
