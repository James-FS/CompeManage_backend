package controllers

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestAwardSummaryBucket(t *testing.T) {
	// 国际级：只有 一等/二等 两列
	require.Equal(t, 0, awardSummaryBucket("国际级", "一等奖"))
	require.Equal(t, 1, awardSummaryBucket("国际级", "银奖"))
	require.Equal(t, -1, awardSummaryBucket("国际级", "三等奖"))
	// 国家级：特等/一等/二等/三等 -> 2..5
	require.Equal(t, 2, awardSummaryBucket("国家级", "特等奖"))
	require.Equal(t, 3, awardSummaryBucket("国家级", "金奖"))
	require.Equal(t, 4, awardSummaryBucket("国家级", "二等奖"))
	require.Equal(t, 5, awardSummaryBucket("国家级", "铜奖"))
	// 省级：特等/一等/二等/三等 -> 6..9
	require.Equal(t, 6, awardSummaryBucket("省级", "特等奖"))
	require.Equal(t, 7, awardSummaryBucket("省级", "一等奖"))
	require.Equal(t, 8, awardSummaryBucket("省级", "二等奖"))
	require.Equal(t, 9, awardSummaryBucket("省级", "三等奖"))
	// 校级/院级不进汇总；未知等级 -1（仍计入合计）
	require.Equal(t, -1, awardSummaryBucket("校级", "一等奖"))
	require.Equal(t, -1, awardSummaryBucket("国家级", "优胜奖"))
}

// 三合一导出的模板装配：附件1(含上报学院列) + 附件2(跨模板复制) + 汇总表
func TestAwardReportWorkbookAssembly(t *testing.T) {
	data, err := awardExportTemplates.ReadFile("templates/award_projects.xlsx")
	require.NoError(t, err)
	f, err := excelize.OpenReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer f.Close()
	projLayout := awardExportTemplateLayout("projects")
	stuLayout := awardExportTemplateLayout("students")
	require.NoError(t, f.SetSheetName(projLayout.Sheet, "附件1"))
	require.NoError(t, copyEmbeddedSheetInto(f, "templates/award_students.xlsx", stuLayout.Sheet, "附件2"))

	projRows := [][]interface{}{{1, "计算机学院", "赛事", "项目", "教育部", "国家级一等奖", "张三", "计算机学院", "刘老师", "计算机学院", "2025-11", "团体"}}
	require.NoError(t, fillAwardExportSheet(f, "附件1", projLayout, 2025, "教务处", projRows))
	stuRows := [][]interface{}{{1, "计算机学院", "00001", "张三", "计算机学院", "计算机", "赛事", "项目", "2025-11", "国家级", "一等奖", ""}}
	require.NoError(t, fillAwardExportSheet(f, "附件2", stuLayout, 2025, "教务处", stuRows))
	summaryRows := [][]interface{}{{1, "计算机学院", "赛事", 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, "团体"}}
	require.NoError(t, addAwardSummarySheet(f, 2025, summaryRows))

	require.Equal(t, []string{"附件1", "附件2", "汇总表"}, f.GetSheetList())
	// 附件1：新列与上报学院数据
	require.Equal(t, "上报学院", exportCell(t, f, "附件1", "B3"))
	require.Equal(t, "计算机学院", exportCell(t, f, "附件1", "B4"))
	require.Contains(t, exportCell(t, f, "附件1", "A1"), "2025年")
	require.Contains(t, exportCell(t, f, "附件1", "A2"), "单位名称（公章）：教务处")
	// 附件2：表头换列 + 数据
	require.Equal(t, "上报学院", exportCell(t, f, "附件2", "B2"))
	require.Equal(t, "计算机学院", exportCell(t, f, "附件2", "B3"))
	// 汇总表：标题与表头
	require.Contains(t, exportCell(t, f, "汇总表", "A1"), "2025年学校组织参加的各类竞赛获奖汇总表")
	require.Equal(t, "国际一等奖", exportCell(t, f, "汇总表", "D2"))
	require.Equal(t, "合计", exportCell(t, f, "汇总表", "N2"))
	require.Equal(t, "1", exportCell(t, f, "汇总表", "G3"))
	require.Equal(t, "1", exportCell(t, f, "汇总表", "N3"))
}
