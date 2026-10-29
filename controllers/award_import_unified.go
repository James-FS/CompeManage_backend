package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Only these directory fields are exposed in matching results and templates.
type AwardImportCompetition struct {
	ID       uint   `json:"id"`
	CompName string `json:"comp_name"`
	CompCode string `json:"comp_code"`
	Year     int    `json:"year"`
}

type UnifiedAwardSummary struct {
	CompID        uint   `json:"comp_id"`
	CompName      string `json:"comp_name"`
	CompCode      string `json:"comp_code"`
	CompYear      string `json:"comp_year"`
	MatchStatus   string `json:"match_status"`
	StudentCount  int    `json:"student_count"`
	ReadyCount    int    `json:"ready_count"`
	SuccessCount  int    `json:"success_count"`
	ExistingCount int    `json:"existing_count"`
	FailCount     int    `json:"fail_count"`
}

func unifiedAwardHeaders() []string {
	headers := []string{"序号", "赛事名称"}
	return append(headers, awardTemplateHeaders[1:]...)
}

func currentUnifiedAwardYear() int {
	return unifiedAwardYearAt(time.Now())
}

func unifiedAwardYearAt(now time.Time) int {
	return now.In(time.FixedZone("Asia/Hong_Kong", 8*60*60)).Year()
}

func requireUnifiedAwardScope(c *gin.Context) (*UserAccessScope, bool) {
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return nil, false
	}
	if scope.RoleCode != "school_admin" && scope.RoleCode != "college_admin" && scope.RoleCode != "competition_manager" {
		utils.Forbidden(c, "没有获奖填报权限")
		return nil, false
	}
	return scope, true
}

func ExportUnifiedAwardTemplate(c *gin.Context) {
	scope, ok := requireUnifiedAwardScope(c)
	if !ok {
		return
	}
	var competitions []AwardImportCompetition
	if err := awardCompetitionScope(database.DB.WithContext(c.Request.Context()).Model(&models.CompDirectory{}), scope).
		Where("year = ?", currentUnifiedAwardYear()).Select("id, comp_name, comp_code, year").Order("id ASC").Find(&competitions).Error; err != nil {
		utils.InternalServerError(c, "读取可填报赛事失败", err)
		return
	}
	f, err := newUnifiedAwardTemplate(competitions)
	if err != nil {
		utils.InternalServerError(c, "生成统一导入模板失败", err)
		return
	}
	defer f.Close()
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", `attachment; filename="Unified_Award_Template.xlsx"`)
	if err := f.Write(c.Writer); err != nil {
		utils.InternalServerError(c, "生成统一导入模板失败", err)
	}
}

func newUnifiedAwardTemplate(competitions []AwardImportCompetition) (*excelize.File, error) {
	year := currentUnifiedAwardYear()
	f := newAwardTemplateLayout(unifiedAwardHeaders(), nil)
	fail := func(err error) (*excelize.File, error) { _ = f.Close(); return nil, err }
	if err := f.SetSheetName("Sheet1", "获奖名单"); err != nil {
		return fail(err)
	}
	for _, sheet := range []string{"可填报赛事目录", "填写说明"} {
		if _, err := f.NewSheet(sheet); err != nil {
			return fail(err)
		}
	}
	textStyle, err := f.NewStyle(&excelize.Style{NumFmt: 49, Font: &excelize.Font{Family: "宋体", Size: 11}, Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}})
	if err != nil {
		return fail(err)
	}
	headerStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Family: "宋体", Size: 12, Bold: true}, Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}})
	if err != nil {
		return fail(err)
	}
	for i, header := range []string{"赛事名称"} {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue("可填报赛事目录", cell, header); err != nil {
			return fail(err)
		}
	}
	_ = f.SetCellStyle("可填报赛事目录", "A1", "A1", headerStyle)
	_ = f.SetColWidth("可填报赛事目录", "A", "A", 65)
	row := 2
	for _, comp := range competitions {
		if comp.Year != year {
			continue
		}
		values := []interface{}{comp.CompName}
		if err := f.SetSheetRow("可填报赛事目录", fmt.Sprintf("A%d", row), &values); err != nil {
			return fail(err)
		}
		_ = f.SetCellStyle("可填报赛事目录", fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), textStyle)
		row++
	}
	for _, sheet := range []string{"获奖名单", "可填报赛事目录"} {
		_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	}
	notes := append([]string{
		"统一导入获奖名单填写说明",
		"1. 只读取第一个工作表“获奖名单”。一行填写一名学生，最多5000行，文件不超过20MB。不同赛事可混排，不要合并数据单元格。",
		fmt.Sprintf("2. 只需填写系统目录中的完整赛事名称，不用填写赛事编号或年份。系统按当前年份（%d年）匹配赛事，与获奖年月分开处理。", year),
		"3. 当前年份存在多个同名赛事时，请在预览中明确选择目标赛事。不会自动创建赛事或模糊分配。",
		"4. 可填报赛事目录只列当前年份、当前账号有权填报的赛事，提交时重新校验目录及权限。未匹配、无权限和信息冲突的行不会写入。",
		"5. 同一学生可导入不同赛事；同一赛事内每名学生只能有一条获奖记录。不同赛事的同名项目互不合并。重复记录跳过，保留首次填报归属。",
		"6. 有效行可导入，失败行可导出修正后重交；数据库写入异常时本次全部写入回滚。若报名要求已开启，每位学生须有目标赛事审核通过的报名。",
	}, awardTemplateNotes...)
	_ = f.SetColWidth("填写说明", "A", "H", 15)
	for i, note := range notes {
		row := i + 1
		_ = f.MergeCell("填写说明", fmt.Sprintf("A%d", row), fmt.Sprintf("H%d", row))
		_ = f.SetCellValue("填写说明", fmt.Sprintf("A%d", row), note)
		_ = f.SetCellStyle("填写说明", fmt.Sprintf("A%d", row), fmt.Sprintf("H%d", row), textStyle)
		_ = f.SetRowHeight("填写说明", row, 60)
	}
	_ = f.SetCellStyle("填写说明", "A1", "H1", headerStyle)
	return f, nil
}

func readUnifiedAwardImport(c *gin.Context) ([]AwardImportRow, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 20*1024*1024)
	var rows []AwardImportRow
	if strings.HasPrefix(c.ContentType(), "application/json") {
		var body struct {
			Rows []AwardImportRow `json:"rows"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			return nil, errors.New("数据格式错误或请求超过20MB")
		}
		rows = body.Rows
	} else {
		file, _, err := c.Request.FormFile("file")
		if err != nil {
			return nil, errors.New("文件上传失败，文件不能超过20MB")
		}
		defer file.Close()
		f, err := excelize.OpenReader(file)
		if err != nil {
			return nil, errors.New("Excel读取失败")
		}
		defer f.Close()
		sheet, err := f.GetRows(f.GetSheetName(0))
		if err != nil {
			return nil, err
		}
		rows, err = parseUnifiedAwardSheet(sheet)
		if err != nil {
			return nil, err
		}
	}
	if len(rows) == 0 || len(rows) > 5000 {
		return nil, errors.New("每批请提交1至5000行")
	}
	return rows, nil
}

func parseUnifiedAwardSheet(sheet [][]string) ([]AwardImportRow, error) {
	if len(sheet) == 0 {
		return nil, errors.New("Excel为空")
	}
	for _, required := range []string{"赛事名称"} {
		found := false
		for _, header := range sheet[0] {
			if strings.TrimSpace(header) == required {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("缺少表头 %s，请下载统一导入模板", required)
		}
	}
	return parseAwardSheet(sheet)
}

// Batch directory lookup stays inside the award-specific scope, including its
// college exceptions. Never reveal candidates outside the caller's scope.
func loadUnifiedAwardCompetitions(tx *gorm.DB, scope *UserAccessScope, rows []AwardImportRow, year int) ([]AwardImportCompetition, error) {
	ids, names := []uint{}, []string{}
	seenID, seenName := map[uint]bool{}, map[string]bool{}
	for _, row := range rows {
		if row.CompID != 0 && !seenID[row.CompID] {
			ids = append(ids, row.CompID)
			seenID[row.CompID] = true
		}
		name := strings.TrimSpace(row.CompName)
		if name != "" && !seenName[name] {
			names = append(names, name)
			seenName[name] = true
		}
	}
	result := []AwardImportCompetition{}
	seen := map[uint]bool{}
	for start := 0; start < max(len(ids), len(names)); start += awardImportBatchSize {
		partIDs := ids[min(start, len(ids)):min(start+awardImportBatchSize, len(ids))]
		partNames := names[min(start, len(names)):min(start+awardImportBatchSize, len(names))]
		var found []AwardImportCompetition
		err := awardCompetitionScope(tx.Model(&models.CompDirectory{}), scope).Select("id, comp_name, comp_code, year").
			Where("year = ?", year).Where("id IN ? OR comp_name IN ?", partIDs, partNames).Order("id ASC").Find(&found).Error
		if err != nil {
			return nil, err
		}
		for _, comp := range found {
			if !seen[comp.ID] {
				result = append(result, comp)
				seen[comp.ID] = true
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func matchUnifiedAwardRows(input []AwardImportRow, competitions []AwardImportCompetition, year int) []AwardImportRow {
	byID := map[uint]AwardImportCompetition{}
	byNameYear := map[string][]AwardImportCompetition{}
	for _, comp := range competitions {
		if comp.Year != year {
			continue
		}
		byID[comp.ID] = comp
		key := comp.CompName + "\x00" + strconv.Itoa(comp.Year)
		byNameYear[key] = append(byNameYear[key], comp)
	}
	rows := append([]AwardImportRow(nil), input...)
	for i := range rows {
		r := &rows[i]
		r.Error, r.ImportStatus, r.MatchStatus = "", "", "unmatched"
		r.Existing, r.Duplicate = false, false
		r.Competition, r.Candidates = nil, nil
		r.CompName = strings.TrimSpace(r.CompName)
		// Year is authoritative server state; legacy spreadsheet/JSON fields
		// cannot redirect a unified import to a historical directory.
		r.CompYear, r.CompCode = strconv.Itoa(year), ""
		if r.RowNumber == 0 {
			r.RowNumber = i + 1
		}
		if r.CompName == "" {
			r.Error = "赛事名称不能为空"
			continue
		}
		var comp AwardImportCompetition
		var found bool
		switch {
		case r.CompID != 0:
			comp, found = byID[r.CompID]
		default:
			candidates := byNameYear[r.CompName+"\x00"+r.CompYear]
			if len(candidates) > 1 {
				r.MatchStatus, r.Candidates, r.Error = "ambiguous", candidates, "当前年份存在多个同名赛事，请在预览中明确选择目标赛事"
				continue
			}
			if len(candidates) == 1 {
				comp, found = candidates[0], true
			}
		}
		if !found {
			r.Error = fmt.Sprintf("%d年赛事不存在或不在可填报范围，请核对当前年份的赛事目录", year)
			continue
		}
		if comp.CompName != r.CompName {
			r.Error = "目标赛事与填写的名称不一致，请重新核对"
			continue
		}
		r.CompID, r.MatchStatus = comp.ID, "matched"
		r.CompCode = comp.CompCode
		r.Competition = &comp
	}
	return rows
}

func unifiedAwardGroupIndices(rows []AwardImportRow) (map[uint][]int, []uint) {
	groups := map[uint][]int{}
	for i, row := range rows {
		if row.MatchStatus == "matched" && row.Error == "" {
			groups[row.CompID] = append(groups[row.CompID], i)
		}
	}
	ids := make([]uint, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return groups, ids
}

func validateUnifiedAwardRows(tx *gorm.DB, rows []AwardImportRow, requireReg bool) error {
	groups, ids := unifiedAwardGroupIndices(rows)
	for _, id := range ids {
		input := make([]AwardImportRow, 0, len(groups[id]))
		for _, index := range groups[id] {
			input = append(input, rows[index])
		}
		validated, _, err := validateAwardRows(tx, id, input, requireReg)
		if err != nil {
			return err
		}
		for i, index := range groups[id] {
			rows[index] = validated[i]
		}
	}
	return nil
}

func unifiedAwardResult(rows []AwardImportRow, teams int) gin.H {
	summaries := []UnifiedAwardSummary{}
	positions := map[string]int{}
	ready, success, existing, failed := 0, 0, 0, 0
	for _, row := range rows {
		key := fmt.Sprintf("%d", row.CompID)
		if row.MatchStatus != "matched" {
			key = row.CompName + "\x00" + row.CompYear + "\x00" + row.CompCode
		}
		position, found := positions[key]
		if !found {
			position, positions[key] = len(summaries), len(summaries)
			summaries = append(summaries, UnifiedAwardSummary{CompID: row.CompID, CompName: row.CompName, CompYear: row.CompYear, CompCode: row.CompCode, MatchStatus: row.MatchStatus})
			if row.Competition != nil {
				summaries[position].CompCode = row.Competition.CompCode
			}
		}
		summary := &summaries[position]
		summary.StudentCount++
		switch {
		case row.Error != "":
			failed++
			summary.FailCount++
		case row.ImportStatus == "imported":
			success++
			summary.SuccessCount++
		case row.Existing || row.Duplicate:
			existing++
			summary.ExistingCount++
		default:
			ready++
			summary.ReadyCount++
		}
	}
	return gin.H{"rows": rows, "competitions": summaries, "student_count": len(rows), "team_count": teams, "ready_count": ready, "success_count": success, "existing_count": existing, "fail_count": failed, "error_count": failed}
}

func PreviewUnifiedAwardImport(c *gin.Context) { handleUnifiedAwardImport(c, false) }
func ImportUnifiedAward(c *gin.Context)        { handleUnifiedAwardImport(c, true) }

func handleUnifiedAwardImport(c *gin.Context, commit bool) {
	scope, ok := requireUnifiedAwardScope(c)
	if !ok {
		return
	}
	input, err := readUnifiedAwardImport(c)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	db := database.DB.WithContext(c.Request.Context())
	year := currentUnifiedAwardYear()
	// Resolve outside the write transaction: a normal directory SELECT inside a
	// MySQL REPEATABLE READ transaction would establish a snapshot before waiting
	// for competition locks. Subsequent validation could then miss an import
	// which committed while we waited, defeating event-wide student uniqueness.
	competitions, err := loadUnifiedAwardCompetitions(db, scope, input, year)
	if err != nil {
		utils.InternalServerError(c, "读取可填报赛事失败", err)
		return
	}
	rows := matchUnifiedAwardRows(input, competitions, year)
	var changed []uint
	teams := 0
	requireRegistration := false
	process := func(tx *gorm.DB) error {
		setting, err := getAwardSetting(tx, commit)
		if err != nil {
			return err
		}
		requireRegistration = setting.RequireRegistration
		if commit {
			_, ids := unifiedAwardGroupIndices(rows)
			var locked []AwardImportCompetition
			if len(ids) > 0 {
				// All imports lock competitions in ascending ID order. Recheck the
				// scope and the bound ID under lock; never remap a stale preview.
				err = awardCompetitionScope(tx.Model(&models.CompDirectory{}), scope).
					Clauses(clause.Locking{Strength: "UPDATE"}).Select("id, comp_name, comp_code, year").
					Where("id IN ?", ids).Order("id ASC").Find(&locked).Error
				if err != nil {
					return err
				}
			}
			bound := []AwardImportRow{}
			indices := []int{}
			for i, row := range rows {
				if row.MatchStatus == "matched" {
					bound = append(bound, row)
					indices = append(indices, i)
				}
			}
			for i, row := range matchUnifiedAwardRows(bound, locked, year) {
				rows[indices[i]] = row
			}
		}
		if err := validateUnifiedAwardRows(tx, rows, requireRegistration); err != nil {
			return err
		}
		if !commit {
			return nil
		}
		groups, ids := unifiedAwardGroupIndices(rows)
		for _, id := range ids {
			valid := make([]AwardImportRow, 0, len(groups[id]))
			for _, index := range groups[id] {
				valid = append(valid, rows[index])
			}
			_, _, teamCount, err := writeAwardImportRows(tx, scope, id, valid)
			if err != nil {
				return err
			}
			teams += teamCount
			changed = append(changed, id)
		}
		return nil
	}
	if commit {
		err = db.Transaction(process)
	} else {
		err = process(db)
	}
	if err != nil {
		utils.InternalServerError(c, "统一导入处理失败，本次未写入获奖记录，请重新校验后提交", err)
		return
	}
	if commit {
		for _, id := range changed {
			clearAwardCache(c.Request.Context(), id)
		}
		for i := range rows {
			switch {
			case rows[i].Error != "":
				rows[i].ImportStatus = "failed"
			case rows[i].Existing || rows[i].Duplicate:
				rows[i].ImportStatus = "existing"
			default:
				rows[i].ImportStatus = "imported"
			}
		}
	}
	result := unifiedAwardResult(rows, teams)
	result["require_registration"] = requireRegistration
	result["import_year"] = year
	message := "已完成赛事匹配与获奖数据校验，尚未写入"
	if commit {
		message = fmt.Sprintf("本次导入%d行，新增%d行，重复跳过%d行，失败%d行", len(rows), result["success_count"], result["existing_count"], result["fail_count"])
	}
	utils.SuccessWithMessage(c, message, result)
}
