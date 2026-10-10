package controllers

import (
	"CompeManage_backend/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestParseAwardTemplateNotesAndReorderedColumns(t *testing.T) {
	headers := append([]string(nil), awardImportHeaders...)
	headers[1], headers[2] = headers[2], headers[1]
	rows := [][]string{headers, {"示例", "张三", "0001"}, {}, {"1", "李四", "0002", "学院", "专业", "项目", "2026-10", "国际级", "金奖", "备注"}, {"说明：1. 获奖类别"}, {"2. 获奖等级"}, {"3. 一行一个学生"}}
	parsed, err := parseAwardSheet(rows)
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, "0002", parsed[0].StudentNumber)
	require.Equal(t, "李四", parsed[0].Name)
	require.Equal(t, 4, parsed[0].RowNumber)
	_, err = parseAwardSheet([][]string{{"奖项等级", "获奖项目名称", "负责人", "学号"}})
	require.ErrorContains(t, err, "新版模板")
}

func TestAwardMemberPermissionIsSubmittingCollege(t *testing.T) {
	a, b := uint(1), uint(2)
	m := models.AwardMember{College: "另一学院学生", SubmittedCollegeID: &a}
	require.True(t, canEditAwardMember(&UserAccessScope{RoleCode: "college_admin", ManagedCollegeID: &a, UserID: 7}, m))
	require.True(t, canEditAwardMember(&UserAccessScope{RoleCode: "college_admin", ManagedCollegeID: &a, UserID: 8}, m))
	require.False(t, canEditAwardMember(&UserAccessScope{RoleCode: "college_admin", ManagedCollegeID: &b}, m))
	require.False(t, canEditAwardMember(&UserAccessScope{RoleCode: "competition_manager", UserID: 7}, m))
	m.SubmittedCollegeID = nil
	require.False(t, canEditAwardMember(&UserAccessScope{RoleCode: "college_admin", ManagedCollegeID: &a}, m))
	require.True(t, canEditAwardMember(&UserAccessScope{RoleCode: "school_admin"}, m))
}

func TestAwardCategoryNormalization(t *testing.T) {
	for _, category := range []string{"校赛", "省级", "国家级"} {
		got, err := normalizeAwardCategory(category)
		require.NoError(t, err)
		require.Equal(t, category, got)
	}
	got, err := normalizeAwardCategory("国际级")
	require.NoError(t, err)
	require.Equal(t, "国家级", got)
	_, err = normalizeAwardCategory("其他")
	require.Error(t, err)
}
