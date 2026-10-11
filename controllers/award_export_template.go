package controllers

import (
	"bytes"
	"embed"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// 模板由学校附件1/附件2转换为 xlsx，已去除样例，保留表头、签章区和全部说明。
//
//go:embed templates/award_projects.xlsx templates/award_students.xlsx
var awardExportTemplates embed.FS

type awardExportLayout struct {
	Sheet                                              string
	FirstRow, LastRow, LastNoteRow, HeaderRow, Columns int
	MinHeight                                          float64
}

func awardExportTemplateLayout(kind string) awardExportLayout {
	if kind == "projects" {
		return awardExportLayout{"获奖项目", 4, 8, 18, 3, 11, 56.25}
	}
	return awardExportLayout{"获奖学生", 3, 18, 25, 2, 12, 33}
}

// 一份附件按赛事年份分工作表；获奖时间仅显示原获奖月份，不用于选择赛事年份。
func buildAwardExportWorkbook(kind, selectedYear string, rowsByYear map[int][][]interface{}) (*excelize.File, error) {
	data, err := awardExportTemplates.ReadFile("templates/award_" + kind + ".xlsx")
	if err != nil {
		return nil, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*excelize.File, error) { f.Close(); return nil, err }
	layout := awardExportTemplateLayout(kind)
	// 外部模板可能省略 sheetViews，先初始化，避免 excelize.CopySheet 访问空视图。
	if err := f.SetSheetView(layout.Sheet, 0, nil); err != nil {
		return fail(err)
	}
	years := []int{}
	if selectedYear != "" {
		year, err := strconv.Atoi(selectedYear)
		if err != nil {
			return fail(err)
		}
		years = append(years, year)
	} else {
		for year := range rowsByYear {
			years = append(years, year)
		}
		sort.Ints(years)
		if len(years) == 0 {
			years = append(years, 0)
		}
	}
	// 先复制空模板，再填数据，避免将前一年数据带入后面的工作表。
	sheets := make([]string, len(years))
	sheets[0] = layout.Sheet
	for i := 1; i < len(years); i++ {
		sheets[i] = fmt.Sprintf("年度附件%d", i+1)
		idx, err := f.NewSheet(sheets[i])
		if err != nil {
			return fail(err)
		}
		if err := f.CopySheet(0, idx); err != nil {
			return fail(err)
		}
	}
	for i, year := range years {
		sheet := sheets[i]
		if len(years) > 1 {
			name := fmt.Sprintf("%d年%s", year, layout.Sheet)
			if year <= 0 {
				name = "未设置赛事年份" + layout.Sheet
			}
			if err := f.SetSheetName(sheet, name); err != nil {
				return fail(err)
			}
			sheet = name
		}
		if err := fillAwardExportSheet(f, sheet, layout, year, rowsByYear[year]); err != nil {
			return fail(err)
		}
	}
	f.SetActiveSheet(0)
	return f, nil
}

func fillAwardExportSheet(f *excelize.File, sheet string, layout awardExportLayout, year int, rows [][]interface{}) error {
	title, err := f.GetCellValue(sheet, "A1")
	if err != nil {
		return err
	}
	yearLabel := "全部赛事年份"
	if year > 0 {
		yearLabel = fmt.Sprintf("%d年", year)
	} else if len(rows) > 0 {
		yearLabel = "未设置赛事年份"
	}
	if err := f.SetCellStr(sheet, "A1", strings.ReplaceAll(title, "{{year}}年", yearLabel)); err != nil {
		return err
	}
	// 先读取数据行样式；没有数据时，模板中的所有数据行都会被删除。
	styles := make([]int, layout.Columns)
	widths := make([]float64, layout.Columns)
	for c := 0; c < layout.Columns; c++ {
		col, _ := excelize.ColumnNumberToName(c + 1)
		styles[c], err = f.GetCellStyle(sheet, fmt.Sprintf("%s%d", col, layout.FirstRow))
		if err != nil {
			return err
		}
		widths[c], err = f.GetColWidth(sheet, col)
		if err != nil {
			return err
		}
	}
	// 表格只保留实际数据行，签章区和说明随行数一起移动。
	extra := len(rows) - (layout.LastRow - layout.FirstRow + 1)
	if extra > 0 {
		if err := f.InsertRows(sheet, layout.LastRow+1, extra); err != nil {
			return err
		}
	} else {
		for i := 0; i < -extra; i++ {
			if err := f.RemoveRow(sheet, layout.FirstRow+len(rows)); err != nil {
				return err
			}
		}
	}
	for i, row := range rows {
		r := layout.FirstRow + i
		if len(row) != layout.Columns {
			return fmt.Errorf("导出行字段数量不正确")
		}
		for c, value := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r)
			// 留空字段不写共享字符串，保留模板中供人工填写的空白单元格。
			if value != "" {
				if err := f.SetCellValue(sheet, cell, value); err != nil {
					return err
				}
			}
			if err := f.SetCellStyle(sheet, cell, cell, styles[c]); err != nil {
				return err
			}
		}
		if err := f.SetRowHeight(sheet, r, awardExportRowHeight(row, widths, layout.MinHeight)); err != nil {
			return err
		}
	}
	lastCol, _ := excelize.ColumnNumberToName(layout.Columns)
	endRow := layout.LastNoteRow + extra
	if err := f.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Area", Scope: sheet, RefersTo: fmt.Sprintf("'%s'!$A$1:$%s$%d", sheet, lastCol, endRow)}); err != nil {
		return err
	}
	if err := f.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Titles", Scope: sheet, RefersTo: fmt.Sprintf("'%s'!$1:$%d", sheet, layout.HeaderRow)}); err != nil {
		return err
	}
	orientation, paper, width, height, fit := "landscape", 9, 1, 0, true
	if err := f.SetSheetProps(sheet, &excelize.SheetPropsOptions{FitToPage: &fit}); err != nil {
		return err
	}
	return f.SetPageLayout(sheet, &excelize.PageLayoutOptions{Orientation: &orientation, Size: &paper, FitToWidth: &width, FitToHeight: &height})
}

// Excel 不会在服务器写出时自动调整行高；按中英文显示宽度预留多行空间，避免长项目名和团队名单被截断。
func awardExportRowHeight(row []interface{}, widths []float64, minimum float64) float64 {
	lines := 1
	for c, v := range row {
		capacity := math.Max(1, widths[c]*11/9-2)
		n := 0
		for _, paragraph := range strings.Split(fmt.Sprint(v), "\n") {
			units := 0.0
			for _, r := range paragraph {
				if r >= 0x2e80 {
					units += 2
				} else {
					units++
				}
			}
			n += int(math.Max(1, math.Ceil(units/capacity)))
		}
		if n > lines {
			lines = n
		}
	}
	return math.Min(409.5, math.Max(minimum, float64(lines)*13+8))
}
