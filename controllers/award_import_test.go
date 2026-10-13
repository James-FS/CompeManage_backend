package controllers

import (
	"CompeManage_backend/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"regexp"
	"testing"
)

func TestParseAwardTemplateNotesAndReorderedColumns(t *testing.T) {
	headers := append([]string(nil), awardImportHeaders...)
	headers[1], headers[2] = headers[2], headers[1]
	rows := [][]string{headers, {"示例", "张三", "0001"}, {}, {"1", "李四", "0002", "学院", "专业", "项目", "2026-10", "国际级", "金奖", "刘涛 曾岑", "新闻学院 创新创业学院", "备注"}, {"说明：1. 获奖类别"}, {"2. 获奖等级"}, {"3. 一行一个学生"}}
	parsed, err := parseAwardSheet(rows)
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, "0002", parsed[0].StudentNumber)
	require.Equal(t, "李四", parsed[0].Name)
	require.Equal(t, 4, parsed[0].RowNumber)
	require.Equal(t, "刘涛 曾岑", parsed[0].AdvisorUsernames)
	require.Equal(t, "新闻学院 创新创业学院", parsed[0].AdvisorColleges)
	_, err = parseAwardSheet([][]string{{"奖项等级", "获奖项目名称", "负责人", "学号"}})
	require.ErrorContains(t, err, "新版模板")
	// 指导教师两列为可选：旧模板（无这两列）仍可导入，教师为空
	oldHeaders := []string{"序号", "学号", "学生姓名", "所属学院", "所在专业", "参赛项目名称", "获奖时间（年/月）", "获奖类别", "获奖等级", "备注"}
	oldParsed, err := parseAwardSheet([][]string{oldHeaders, {"1", "0002", "李四", "学院", "专业", "项目", "2026-10", "国家级", "一等奖", "无教师"}})
	require.NoError(t, err)
	require.Len(t, oldParsed, 1)
	require.Empty(t, oldParsed[0].AdvisorUsernames)
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

func TestAwardImportAdvisorInfoDefaultsToEmptyArray(t *testing.T) {
	require.JSONEq(t, `[]`, advisorInfoForImport("", ""))
	require.JSONEq(t, `[{"id":0,"username":"","name":"指导教师","phone":"","email":"","college":"学院"}]`, advisorInfoForImport("指导教师", "学院"))
}

// 模板说明新增/修改时必须同步 noteHeights（行高数组在函数内，长度不一致会越界panic），
// 此用例兜底：模板能生成且最新一条说明落在预期单元格。
func TestAwardTemplateGeneration(t *testing.T) {
	f := newAwardTemplate()
	defer f.Close()
	v, err := f.GetCellValue("Sheet1", "A22")
	require.NoError(t, err)
	require.Contains(t, v, "参赛项目名称一定要一样")
	// 表头12列含指导教师
	h, err := f.GetCellValue("Sheet1", "J1")
	require.NoError(t, err)
	require.Equal(t, "指导教师工号", h)
}

// 三元组校验（任务6/8）：工号必须为系统可选教师，学院须与归属一致
func TestResolveAwardAdvisors(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)

	teacherRow := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "username", "realname", "college", "identity_type"}).
			AddRow(5, "T2023010", "陈老师", "计算机科学与网络工程学院", "staff")
	}

	// 工号存在且学院一致 → 快照取系统资料
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users`")).WithArgs("T2023010", 1).WillReturnRows(teacherRow())
	pairs, err := resolveAwardAdvisors(db, "T2023010", "计算机科学与网络工程学院")
	require.NoError(t, err)
	require.Len(t, pairs, 1)
	require.Equal(t, "T2023010", pairs[0].Username)
	require.Equal(t, "陈老师", pairs[0].Name)
	require.Equal(t, "计算机科学与网络工程学院", pairs[0].College)

	// 学院列与系统归属不符 → 报错
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users`")).WithArgs("T2023010", 1).WillReturnRows(teacherRow())
	_, err = resolveAwardAdvisors(db, "T2023010", "新闻学院")
	require.ErrorContains(t, err, "归属学院不符")

	// 工号不存在/非教师 → 报错
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users`")).WithArgs("X99999", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, err = resolveAwardAdvisors(db, "X99999", "")
	require.ErrorContains(t, err, "不存在")

	// 工号重复填写 → 报错
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users`")).WithArgs("T2023010", 1).WillReturnRows(teacherRow())
	_, err = resolveAwardAdvisors(db, "T2023010 T2023010", "")
	require.ErrorContains(t, err, "重复")

	require.NoError(t, mock.ExpectationsWereMet())
}
