package controllers

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"CompeManage_backend/database"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func exportCell(t *testing.T, f *excelize.File, sheet, cell string) string {
	t.Helper()
	value, err := f.GetCellValue(sheet, cell)
	require.NoError(t, err)
	return value
}

func TestAwardExportTemplatesExpandAndPreserveNotes(t *testing.T) {
	for _, kind := range []string{"projects", "students"} {
		t.Run(kind, func(t *testing.T) {
			layout := awardExportTemplateLayout(kind)
			count := layout.LastRow - layout.FirstRow + 4 // 比模板预留区域多3行
			rows := make([][]interface{}, count)
			for i := range rows {
				if kind == "projects" {
					rows[i] = []interface{}{i + 1, "测试赛事", strings.Repeat("长项目名称", 12), "主办单位", "国家级一等奖", "张三 李四", "信息工程学院", "", "", "2026-10", "团体"}
				} else {
					rows[i] = []interface{}{i + 1, "承办学院", "00123", "张三", "信息工程学院", "软件工程", "测试赛事", "测试项目", "2026-10", "国家级", "金奖", ""}
				}
			}
			f, err := buildAwardExportWorkbook(kind, "2025", map[int][][]interface{}{2025: rows})
			require.NoError(t, err)
			defer f.Close()
			require.Contains(t, exportCell(t, f, layout.Sheet, "A1"), "2025年大学生参加省级以上")
			require.NotContains(t, exportCell(t, f, layout.Sheet, "A1"), "2026")
			signatureRow, notesRow, noteCount := 13, 16, 6
			if kind == "students" {
				signatureRow, notesRow, noteCount = 23, 25, 4
				require.Equal(t, "00123", exportCell(t, f, layout.Sheet, "C3"))
				require.Equal(t, "金奖", exportCell(t, f, layout.Sheet, "K3"))
			} else {
				require.Contains(t, exportCell(t, f, layout.Sheet, "A2"), "单位名称（公章）：")
				require.Empty(t, exportCell(t, f, layout.Sheet, "H4"))
				require.Empty(t, exportCell(t, f, layout.Sheet, "I4"))
				height, err := f.GetRowHeight(layout.Sheet, 4)
				require.NoError(t, err)
				require.Greater(t, height, layout.MinHeight)
			}
			signature, _ := excelize.CoordinatesToCellName(1, signatureRow)
			require.Contains(t, exportCell(t, f, layout.Sheet, signature), "填表日期：    年    月    日")
			for i := 0; i < noteCount; i++ {
				cell, _ := excelize.CoordinatesToCellName(1, notesRow+i)
				require.NotEmpty(t, exportCell(t, f, layout.Sheet, cell))
			}
			firstCell, _ := excelize.CoordinatesToCellName(1, layout.FirstRow)
			styleID, err := f.GetCellStyle(layout.Sheet, firstCell)
			require.NoError(t, err)
			style, err := f.GetStyle(styleID)
			require.NoError(t, err)
			require.Equal(t, "宋体", style.Font.Family)
			require.Equal(t, float64(9), style.Font.Size)
			require.Len(t, f.GetDefinedName(), 2)
			buffer, err := f.WriteToBuffer()
			require.NoError(t, err)
			roundtrip, err := excelize.OpenReader(bytes.NewReader(buffer.Bytes()))
			require.NoError(t, err)
			defer roundtrip.Close()
			require.Equal(t, exportCell(t, f, layout.Sheet, signature), exportCell(t, roundtrip, layout.Sheet, signature))
		})
	}
}

func TestAwardExportTemplatesKeepOnlyActualDataRows(t *testing.T) {
	for _, kind := range []string{"projects", "students"} {
		for _, count := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("%s/%d", kind, count), func(t *testing.T) {
				layout := awardExportTemplateLayout(kind)
				rows := make([][]interface{}, count)
				for i := range rows {
					rows[i] = make([]interface{}, layout.Columns)
					for c := range rows[i] {
						rows[i][c] = ""
					}
					rows[i][0] = i + 1
				}
				f, err := buildAwardExportWorkbook(kind, "2025", map[int][][]interface{}{2025: rows})
				require.NoError(t, err)
				defer f.Close()
				// 数据行紧跟表头，数据后第一行已是模板原有的无边框间隔行。
				nextCell := fmt.Sprintf("A%d", layout.FirstRow+count)
				require.Empty(t, exportCell(t, f, layout.Sheet, nextCell))
				styleID, err := f.GetCellStyle(layout.Sheet, nextCell)
				require.NoError(t, err)
				style, err := f.GetStyle(styleID)
				require.NoError(t, err)
				for _, border := range style.Border {
					require.Zero(t, border.Style, "数据后不应保留空白表格行")
				}
				if count > 0 {
					require.Equal(t, fmt.Sprint(count), exportCell(t, f, layout.Sheet, fmt.Sprintf("A%d", layout.FirstRow+count-1)))
				}
				delta := count - (layout.LastRow - layout.FirstRow + 1)
				signatureRow, notesRow := 10+delta, 13+delta
				if kind == "students" {
					signatureRow, notesRow = 20+delta, 22+delta
				}
				require.Contains(t, exportCell(t, f, layout.Sheet, fmt.Sprintf("A%d", signatureRow)), "填表人：")
				require.Contains(t, exportCell(t, f, layout.Sheet, fmt.Sprintf("A%d", notesRow)), "说明：")
				lastCol, _ := excelize.ColumnNumberToName(layout.Columns)
				for _, name := range f.GetDefinedName() {
					if name.Name == "_xlnm.Print_Area" {
						require.Equal(t, fmt.Sprintf("'%s'!$A$1:$%s$%d", layout.Sheet, lastCol, layout.LastNoteRow+delta), name.RefersTo)
					}
				}
				buffer, err := f.WriteToBuffer()
				require.NoError(t, err)
				roundtrip, err := excelize.OpenReader(bytes.NewReader(buffer.Bytes()))
				require.NoError(t, err)
				defer roundtrip.Close()
				require.Contains(t, exportCell(t, roundtrip, layout.Sheet, fmt.Sprintf("A%d", signatureRow)), "填表人：")
			})
		}
	}
}

func TestAwardExportGroupsCompetitionYearsWithoutCopyingPreviousData(t *testing.T) {
	f, err := buildAwardExportWorkbook("students", "", map[int][][]interface{}{
		2025: {{1, "学院", "00123", "张三", "学院", "专业", "旧赛事", "旧项目", "2026-10", "省级", "二等奖", "备注"}},
		2026: {{1, "学院", "00456", "李四", "学院", "专业", "新赛事", "新项目", "2026-10", "国家级", "一等奖", ""}},
	})
	require.NoError(t, err)
	defer f.Close()
	require.Equal(t, []string{"2025年获奖学生", "2026年获奖学生"}, f.GetSheetList())
	require.Contains(t, exportCell(t, f, "2025年获奖学生", "A1"), "2025年")
	require.Equal(t, "2026-10", exportCell(t, f, "2025年获奖学生", "I3"))
	require.Equal(t, "李四", exportCell(t, f, "2026年获奖学生", "D3"))
	require.Empty(t, exportCell(t, f, "2026年获奖学生", "L3"))
	require.Empty(t, exportCell(t, f, "2026年获奖学生", "D4"))
	require.Len(t, f.GetDefinedName(), 4)
}

func TestAwardExportStudentsUsesCompetitionYearAndFiltersSchoolAwards(t *testing.T) {
	previousDB := database.DB
	mock := setupAwardDBMock(t)
	db, err := database.DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close(); database.DB = previousDB })
	date := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT .*cd.year AS competition_year.*FROM award_members am.*cd.year = \?`).
		WithArgs("approved", "1", "2025").WillReturnRows(sqlmock.NewRows([]string{
		"student_number", "name", "college", "major", "remark", "project_name", "award_level", "award_category", "award_name", "award_time", "comp_name", "undertaker", "competition_year",
	}).AddRow("00123", "张三", "学院", "专业", "备注", "项目", "国际级金奖", "", "", date, "赛事", "承办学院", 2025).
		AddRow("00456", "李四", "学院", "专业", "", "校赛项目", "校赛一等奖", "校赛", "一等奖", date, "赛事", "承办学院", 2025))
	_, w, c := buildGET("/api/award/export/students?year=2025")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	ExportAwardStudents(c)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "spreadsheetml")
	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	require.NoError(t, err)
	defer f.Close()
	require.Contains(t, exportCell(t, f, "获奖学生", "A1"), "2025年")
	require.Equal(t, "00123", exportCell(t, f, "获奖学生", "C3"))
	require.Equal(t, "2026-10", exportCell(t, f, "获奖学生", "I3"))
	require.Equal(t, "国家级", exportCell(t, f, "获奖学生", "J3"))
	require.Equal(t, "金奖", exportCell(t, f, "获奖学生", "K3"))
	require.Empty(t, exportCell(t, f, "获奖学生", "C4"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAwardExportProjectsGroupsMembersAndLeavesTeacherFieldsBlank(t *testing.T) {
	previousDB := database.DB
	mock := setupAwardDBMock(t)
	db, err := database.DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close(); database.DB = previousDB })
	mock.ExpectQuery(`SELECT .*cd.year AS competition_year.*FROM .awards.*cd.year = \?`).
		WithArgs("approved", "1", "2025").WillReturnRows(sqlmock.NewRows([]string{
		"id", "project_name", "award_level", "award_category", "award_name", "award_time", "comp_name", "organizer", "competition_year", "participant_type",
	}).AddRow(1, "项目", "省级二等奖", "省级", "二等奖", nil, "赛事", "主办单位", 2025, 2).
		AddRow(2, "校赛项目", "校赛一等奖", "校赛", "一等奖", nil, "赛事", "主办单位", 2025, 1).
		AddRow(3, "国际项目", "国际级金奖", "", "", nil, "国际赛事", "主办单位", 2025, 1))
	mock.ExpectQuery(`SELECT award_id, name, college FROM .award_members.*award_id IN`).WithArgs(1, 3).
		WillReturnRows(sqlmock.NewRows([]string{"award_id", "name", "college"}).AddRow(1, "张三", "学院甲").AddRow(1, "李四", "学院乙"))
	_, w, c := buildGET("/api/award/export/projects?year=2025")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	ExportAwardProjects(c)
	require.Equal(t, 200, w.Code)
	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	require.NoError(t, err)
	defer f.Close()
	require.Equal(t, "张三 李四", exportCell(t, f, "获奖项目", "F4"))
	require.Equal(t, "学院甲 学院乙", exportCell(t, f, "获奖项目", "G4"))
	require.Empty(t, exportCell(t, f, "获奖项目", "H4"))
	require.Empty(t, exportCell(t, f, "获奖项目", "I4"))
	require.Equal(t, "团体", exportCell(t, f, "获奖项目", "K4"))
	require.Equal(t, "国际项目", exportCell(t, f, "获奖项目", "C5"))
	require.Equal(t, "国家级金奖", exportCell(t, f, "获奖项目", "E5"))
	require.Empty(t, exportCell(t, f, "获奖项目", "C6"))
	require.NoError(t, mock.ExpectationsWereMet())
}
