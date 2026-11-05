package controllers

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"CompeManage_backend/database"
	"CompeManage_backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// ExportAwardReport 一键导出上报 Excel：附件1 + 附件2 + 汇总表（刘老师最终上报格式）。
// GET /api/award/export/report?year=2025
func ExportAwardReport(c *gin.Context) {
	scope, ok := awardExportScope(c)
	if !ok {
		return
	}
	year, ok := awardExportYear(c)
	if !ok {
		return
	}
	unit := awardExportUnit(c, scope)
	ctx := c.Request.Context()

	projRows, err := buildAwardProjectRows(ctx, scope, year)
	if err != nil {
		utils.InternalServerError(c, "查询获奖项目失败", err)
		return
	}
	stuRows, err := buildAwardStudentRows(ctx, scope, year)
	if err != nil {
		utils.InternalServerError(c, "查询获奖学生失败", err)
		return
	}
	summaryRows, err := buildAwardSummaryRows(ctx, scope, year)
	if err != nil {
		utils.InternalServerError(c, "查询汇总数据失败", err)
		return
	}

	// 合并各年份到单一工作表；year 为空时标题显示“全部赛事年份”
	yearNum := -1
	if year != "" {
		yearNum, _ = strconv.Atoi(year)
	}
	projFlat := flattenAwardRows(projRows)
	stuFlat := flattenAwardRows(stuRows)

	data, err := awardExportTemplates.ReadFile("templates/award_projects.xlsx")
	if err != nil {
		utils.InternalServerError(c, "读取附件模板失败", err)
		return
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		utils.InternalServerError(c, "打开附件模板失败", err)
		return
	}
	defer f.Close()
	projLayout := awardExportTemplateLayout("projects")
	stuLayout := awardExportTemplateLayout("students")
	if err := f.SetSheetName(projLayout.Sheet, "附件1"); err != nil {
		utils.InternalServerError(c, "准备附件1失败", err)
		return
	}
	if err := copyEmbeddedSheetInto(f, "templates/award_students.xlsx", stuLayout.Sheet, "附件2"); err != nil {
		utils.InternalServerError(c, "准备附件2失败", err)
		return
	}
	if err := fillAwardExportSheet(f, "附件1", projLayout, yearNum, unit, projFlat); err != nil {
		utils.InternalServerError(c, "填充附件1失败", err)
		return
	}
	if err := fillAwardExportSheet(f, "附件2", stuLayout, yearNum, unit, stuFlat); err != nil {
		utils.InternalServerError(c, "填充附件2失败", err)
		return
	}
	if err := addAwardSummarySheet(f, yearNum, summaryRows); err != nil {
		utils.InternalServerError(c, "生成汇总表失败", err)
		return
	}
	f.SetActiveSheet(0)

	buffer, err := f.WriteToBuffer()
	if err != nil {
		utils.InternalServerError(c, "生成Excel失败", err)
		return
	}
	label := "学科竞赛获奖上报汇总"
	suffix := "全部赛事年份"
	if year != "" {
		suffix = year + "年"
	}
	filename := url.PathEscape(suffix + label + ".xlsx")
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, filename))
	if _, err := c.Writer.Write(buffer.Bytes()); err != nil {
		utils.InternalServerError(c, "写出Excel失败", err)
	}
}

// flattenAwardRows 把按年份分组的行按年份升序合并为单表数据。
func flattenAwardRows(grouped map[int][][]interface{}) [][]interface{} {
	years := make([]int, 0, len(grouped))
	for y := range grouped {
		years = append(years, y)
	}
	sort.Ints(years)
	flat := [][]interface{}{}
	for _, y := range years {
		flat = append(flat, grouped[y]...)
	}
	return flat
}

// copyEmbeddedSheetInto 把内嵌模板中的工作表整表复制到目标 workbook（值+样式+合并+列宽+行高）。
func copyEmbeddedSheetInto(dst *excelize.File, embeddedPath, srcSheet, dstSheet string) error {
	data, err := awardExportTemplates.ReadFile(embeddedPath)
	if err != nil {
		return err
	}
	src, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer src.Close()
	if _, err := dst.NewSheet(dstSheet); err != nil {
		return err
	}
	// 外部模板可能省略 sheetViews
	if err := dst.SetSheetView(dstSheet, 0, nil); err != nil {
		return err
	}
	rows, err := src.GetRows(srcSheet)
	if err != nil {
		return err
	}
	maxCol := 0
	for _, row := range rows {
		if len(row) > maxCol {
			maxCol = len(row)
		}
	}
	for r, row := range rows {
		for c := 0; c < maxCol; c++ {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			styleID, err := src.GetCellStyle(srcSheet, cell)
			if err == nil && styleID > 0 {
				if err := dst.SetCellStyle(dstSheet, cell, cell, styleID); err != nil {
					return err
				}
			}
			if c < len(row) && row[c] != "" {
				if err := dst.SetCellStr(dstSheet, cell, row[c]); err != nil {
					return err
				}
			}
		}
		if h, err := src.GetRowHeight(srcSheet, r+1); err == nil && h > 0 {
			if err := dst.SetRowHeight(dstSheet, r+1, h); err != nil {
				return err
			}
		}
	}
	for c := 1; c <= maxCol; c++ {
		col, _ := excelize.ColumnNumberToName(c)
		if w, err := src.GetColWidth(col, col); err == nil && w > 0 {
			if err := dst.SetColWidth(dstSheet, col, col, w); err != nil {
				return err
			}
		}
	}
	merges, err := src.GetMergeCells(srcSheet)
	if err != nil {
		return err
	}
	for _, m := range merges {
		if err := dst.MergeCell(dstSheet, m.GetStartAxis(), m.GetEndAxis()); err != nil {
			return err
		}
	}
	return nil
}

// 汇总表表头（刘老师「汇总」页样式；2026-10-29 扩至四级：新增 校级一等/二等/三等）
var awardSummaryHeaders = []string{
	"序号", "承办学院", "竞赛名称", "国际一等奖", "国际二等奖", "国家特等奖", "国家一等", "国家二等",
	"国家三等", "省特等奖", "省级一等", "省级二等", "省级三等", "校级一等", "校级二等", "校级三等", "合计", "备注",
}

// awardSummaryBucket 把一场比赛的获奖归入汇总表等级列；返回列下标（0=国际一等奖 ... 9=省级三等，
// 10=校级一等 11=校级二等 12=校级三等），无法归类返回 -1（仍计入合计）。
// 2026-10-29：口径扩至四级，校级赛事也进入汇总；"校赛"为历史值同校级。
func awardSummaryBucket(compLevel, awardText string) int {
	family := 0 // 0国际 1国家 2省 3校
	switch compLevel {
	case "国际级":
		family = 0
	case "国家级":
		family = 1
	case "省级":
		family = 2
	case "校级", "校赛":
		family = 3
	default:
		return -1
	}
	grade := -1
	switch {
	case strings.Contains(awardText, "特等奖"):
		grade = 0
	case strings.Contains(awardText, "一等"), strings.Contains(awardText, "金"), strings.Contains(awardText, "冠军"), strings.Contains(awardText, "第一名"):
		grade = 1
	case strings.Contains(awardText, "二等"), strings.Contains(awardText, "银"), strings.Contains(awardText, "亚军"), strings.Contains(awardText, "第二名"):
		grade = 2
	case strings.Contains(awardText, "三等"), strings.Contains(awardText, "铜"), strings.Contains(awardText, "季军"), strings.Contains(awardText, "第三名"):
		grade = 3
	default:
		return -1
	}
	switch family {
	case 0: // 国际列只有 一等/二等
		if grade == 1 {
			return 0
		}
		if grade == 2 {
			return 1
		}
		return -1
	case 1: // 国家特等/一等/二等/三等 -> 2..5
		if grade >= 0 && grade <= 3 {
			return 2 + grade
		}
	case 2: // 省特等/一等/二等/三等 -> 6..9
		if grade >= 0 && grade <= 3 {
			return 6 + grade
		}
	default: // 校级一等/二等/三等 -> 10..12（无特等列）
		if grade >= 1 && grade <= 3 {
			return 9 + grade
		}
	}
	return -1
}

// buildAwardSummaryRows 汇总表数据：按赛事聚合省级以上获奖等级分布。
// 列：序号, 承办学院, 竞赛名称, 10个等级列, 合计, 备注(团体/个人)。
func buildAwardSummaryRows(ctx context.Context, scope *UserAccessScope, year string) ([][]interface{}, error) {
	managedCollege, err := collegeNameForAwardScope(database.DB.WithContext(ctx), scope)
	if err != nil {
		return nil, err
	}
	q := database.DB.WithContext(ctx).Table("awards a").
		Select("cd.comp_name, cd.comp_level, cn.name AS college_name, cdp.participant_type, a.award_name, a.award_level, a.award_category").
		Joins("JOIN comp_directories cd ON cd.id = a.comp_id AND cd.delete_time IS NULL").
		Joins("LEFT JOIN colleges cn ON cn.id = cd.college_id").
		Joins("LEFT JOIN comp_details cdp ON cdp.comp_id = a.comp_id AND cdp.delete_time IS NULL").
		Where("a.delete_time IS NULL").Where("a.status IN ?", awardExportFilterStatus()).
		Where("cd.comp_level IN ?", []string{"国际级", "国家级", "省级", "校级", "校赛"}).
		Order("cn.name ASC, cd.comp_name ASC, a.id ASC")
	if year != "" {
		q = q.Where("cd.year = ?", year)
	}
	// 2026-10-31 合并说明：本查询赛事表别名是 cd（Join "comp_directories cd"），而
	// awardCompetitionScope 内部固定拼 comp_directories.* 前缀——别名不一致时 WHERE
	// 指向不存在的表名，MySQL 按零长别名处理导致恒过滤为空（汇总 sheet 无数据根因）。
	// 因此此处不走 awardCompetitionScope，按 cd 别名手工套用同一可见性规则。
	if scope.IsCollegeAdmin() {
		if scope.ManagedCollegeID == nil {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("(cd.college_id = ? OR cd.college_id IS NULL OR cd.college_id = 0 OR cd.comp_level IN ?)",
				*scope.ManagedCollegeID, []string{"校级", "校赛"})
		}
	} else if !scope.IsSchoolAdmin() {
		q = q.Where("cd.manager_id = ?", scope.UserID)
	}
	// develop 新增的学生学院过滤（与赛事可见性正交，别名 "a" 正确）一并保留
	q = applyAwardStudentCollegeScope(q, scope, managedCollege, "a")

	var list []struct {
		CompName, CompLevel, CollegeName     string
		ParticipantType                      *int8
		AwardName, AwardLevel, AwardCategory string
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}

	type summaryGroup struct {
		college, name string
		participant   *int8
		buckets       [13]int
		total         int
	}
	order := []string{}
	groups := map[string]*summaryGroup{}
	for _, item := range list {
		key := item.CompName + "\x00" + item.CompLevel
		g, ok := groups[key]
		if !ok {
			g = &summaryGroup{college: item.CollegeName, name: item.CompName, participant: item.ParticipantType}
			groups[key] = g
			order = append(order, key)
		}
		g.total++
		text := item.AwardName
		if text == "" {
			text = item.AwardLevel
		}
		if b := awardSummaryBucket(item.CompLevel, text+item.AwardCategory); b >= 0 {
			g.buckets[b]++
		}
	}

	rows := make([][]interface{}, 0, len(order))
	for i, key := range order {
		g := groups[key]
		row := []interface{}{i + 1, g.college, g.name}
		for _, b := range g.buckets {
			row = append(row, b)
		}
		row = append(row, g.total, awardParticipantLabel(g.participant))
		rows = append(rows, row)
	}
	return rows, nil
}

// addAwardSummarySheet 生成「汇总表」工作表（标题 + 表头 + 数据，细边框样式）。
func addAwardSummarySheet(f *excelize.File, year int, rows [][]interface{}) error {
	const sheet = "汇总表"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		return err
	}
	_ = idx
	title := "学校组织参加的各类竞赛获奖汇总表"
	if year > 0 {
		title = fmt.Sprintf("%d年学校组织参加的各类竞赛获奖汇总表", year)
	}
	lastCol, _ := excelize.ColumnNumberToName(len(awardSummaryHeaders))

	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 14, Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 11, Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"DCE6F1"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "808080"}, {Type: "right", Style: 1, Color: "808080"},
			{Type: "top", Style: 1, Color: "808080"}, {Type: "bottom", Style: 1, Color: "808080"},
		},
	})
	bodyStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 11},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "808080"}, {Type: "right", Style: 1, Color: "808080"},
			{Type: "top", Style: 1, Color: "808080"}, {Type: "bottom", Style: 1, Color: "808080"},
		},
	})

	f.MergeCell(sheet, "A1", lastCol+"1")
	f.SetCellValue(sheet, "A1", title)
	f.SetCellStyle(sheet, "A1", lastCol+"1", titleStyle)
	f.SetRowHeight(sheet, 1, 28)

	for i, h := range awardSummaryHeaders {
		col, _ := excelize.ColumnNumberToName(i + 1)
		cell := col + "2"
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}
	f.SetRowHeight(sheet, 2, 32)

	for r, row := range rows {
		excelRow := r + 3
		for i, v := range row {
			col, _ := excelize.ColumnNumberToName(i + 1)
			cell := col + fmt.Sprint(excelRow)
			if v != "" {
				f.SetCellValue(sheet, cell, v)
			}
			f.SetCellStyle(sheet, cell, cell, bodyStyle)
		}
	}

	widths := []float64{6, 22, 40, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 8, 8}
	for i, w := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, col, col, w)
	}

	endRow := 2 + len(rows)
	f.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Area", Scope: sheet,
		RefersTo: fmt.Sprintf("'%s'!$A$1:$%s$%d", sheet, lastCol, endRow)})
	f.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Titles", Scope: sheet,
		RefersTo: fmt.Sprintf("'%s'!$1:$2", sheet)})
	fit := true
	_ = f.SetSheetProps(sheet, &excelize.SheetPropsOptions{FitToPage: &fit})
	return f.SetPageLayout(sheet, &excelize.PageLayoutOptions{
		Orientation: strPtr("landscape"), Size: intPtr(9), FitToWidth: intPtr(1), FitToHeight: intPtr(0)})
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
