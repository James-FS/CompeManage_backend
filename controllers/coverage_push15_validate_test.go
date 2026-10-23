package controllers

import (
	"strings"
	"testing"
	"time"

	"CompeManage_backend/database"
	"CompeManage_backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// 构造一行合法导入数据
func importRow(student, project string) AwardImportRow {
	return AwardImportRow{
		StudentNumber: student,
		Name:          "张三",
		College:       "计算机学院",
		Major:         "软件工程",
		ProjectName:   project,
		AwardMonth:    "2026-05",
		Category:      "省级",
		AwardName:     "一等奖",
	}
}

// 空 lookup：无用户、无奖项、无报名、无成员关系
func emptyLookup() *awardImportLookup {
	return &awardImportLookup{
		users:         map[string]models.User{},
		teachers:      map[string]models.User{},
		awards:        map[string][]models.Award{},
		legacyAwards:  map[string][]models.Award{},
		registrations: map[uint]map[uint]bool{},
		memberships:   map[uint]map[uint]bool{},
	}
}

// ===================== validateAwardImportTeams（纯函数） =====================

func TestValidateAwardImportTeams_EmptyProjectSkipped(t *testing.T) {
	errorsByTeam := validateAwardImportTeams(emptyLookup(), []AwardImportRow{{ProjectName: "  "}})
	assert.Empty(t, errorsByTeam)
}

func TestValidateAwardImportTeams_ProjectTooLong(t *testing.T) {
	row := importRow("S001", strings.Repeat("项", 256))
	errorsByTeam := validateAwardImportTeams(emptyLookup(), []AwardImportRow{row})
	assert.Contains(t, errorsByTeam[database.AwardProjectKey(row.ProjectName)], "项目或奖项超出长度限制")
}

func TestValidateAwardImportTeams_InvalidCategory(t *testing.T) {
	row := importRow("S001", "项目A")
	row.Category = "国际"
	errorsByTeam := validateAwardImportTeams(emptyLookup(), []AwardImportRow{row})
	assert.Contains(t, errorsByTeam[database.AwardProjectKey("项目A")], "该团队未导入")
}

func TestValidateAwardImportTeams_InvalidAwardName(t *testing.T) {
	row := importRow("S001", "项目A")
	row.AwardName = "特级奖"
	errorsByTeam := validateAwardImportTeams(emptyLookup(), []AwardImportRow{row})
	assert.Contains(t, errorsByTeam[database.AwardProjectKey("项目A")], "该团队未导入")
}

func TestValidateAwardImportTeams_InvalidMonth(t *testing.T) {
	row := importRow("S001", "项目A")
	row.AwardMonth = "2026-13"
	errorsByTeam := validateAwardImportTeams(emptyLookup(), []AwardImportRow{row})
	assert.Contains(t, errorsByTeam[database.AwardProjectKey("项目A")], "该团队未导入")
}

func TestValidateAwardImportTeams_TeamInconsistent(t *testing.T) {
	row1 := importRow("S001", "项目A")
	row2 := importRow("S002", "项目A")
	row2.AwardName = "二等奖"
	errorsByTeam := validateAwardImportTeams(emptyLookup(), []AwardImportRow{row1, row2})
	assert.Contains(t, errorsByTeam[database.AwardProjectKey("项目A")], "同一团队的获奖类别、等级、时间或指导教师不一致")
}

func TestValidateAwardImportTeams_HistoryConflict(t *testing.T) {
	key := database.AwardProjectKey("项目A")
	lookup := emptyLookup()
	lookup.awards[key] = []models.Award{{BaseModel: models.BaseModel{ID: 1}}, {BaseModel: models.BaseModel{ID: 2}}}
	errorsByTeam := validateAwardImportTeams(lookup, []AwardImportRow{importRow("S001", "项目A")})
	assert.Contains(t, errorsByTeam[key], "历史同名奖项冲突")
}

func TestValidateAwardImportTeams_ExistingNotApproved(t *testing.T) {
	key := database.AwardProjectKey("项目A")
	lookup := emptyLookup()
	lookup.awards[key] = []models.Award{{BaseModel: models.BaseModel{ID: 1}, Status: "draft"}}
	errorsByTeam := validateAwardImportTeams(lookup, []AwardImportRow{importRow("S001", "项目A")})
	assert.Contains(t, errorsByTeam[key], "该团队已有待审核或驳回奖项")
}

func TestValidateAwardImportTeams_CategoryConflict(t *testing.T) {
	key := database.AwardProjectKey("项目A")
	lookup := emptyLookup()
	lookup.awards[key] = []models.Award{{
		BaseModel:      models.BaseModel{ID: 1},
		Status:         "approved",
		AwardCategory:  "国家级",
		AwardName:      "一等奖",
	}}
	errorsByTeam := validateAwardImportTeams(lookup, []AwardImportRow{importRow("S001", "项目A")})
	assert.Contains(t, errorsByTeam[key], "与已有团队的类别、等级或时间冲突")
}

func TestValidateAwardImportTeams_AdvisorMismatch(t *testing.T) {
	key := database.AwardProjectKey("项目A")
	month := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	lookup := emptyLookup()
	lookup.awards[key] = []models.Award{{
		BaseModel:     models.BaseModel{ID: 1},
		Status:        "approved",
		AwardCategory: "省级",
		AwardName:     "一等奖",
		AwardTime:     &month,
		AdvisorInfo:   `[{"username":"T001","name":"甲"}]`,
	}}
	row := importRow("S001", "项目A")
	row.AdvisorUsernames = "T002"
	lookup.teachers["T002"] = models.User{BaseModel: models.BaseModel{ID: 2}, Username: "T002", Realname: "乙", College: "计算机学院"}
	errorsByTeam := validateAwardImportTeams(lookup, []AwardImportRow{row})
	assert.Contains(t, errorsByTeam[key], "与已有团队的指导教师不一致")
}

func TestValidateAwardImportTeams_ApprovedMatchNoError(t *testing.T) {
	key := database.AwardProjectKey("项目A")
	month := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	lookup := emptyLookup()
	lookup.awards[key] = []models.Award{{
		BaseModel:     models.BaseModel{ID: 1},
		Status:        "approved",
		AwardCategory: "省级",
		AwardName:     "一等奖",
		AwardTime:     &month,
	}}
	errorsByTeam := validateAwardImportTeams(lookup, []AwardImportRow{importRow("S001", "项目A")})
	assert.Empty(t, errorsByTeam[key])
}

// ===================== validateAwardRows（直调，mock lookup） =====================

// 布置 loadAwardImportLookup 的 5 条查询（无指导教师、单学生单项目）
func expectImportLookup(mock sqlmock.Sqlmock, userRows *sqlmock.Rows, awardRows *sqlmock.Rows) {
	mock.ExpectQuery("SELECT users.id, users.username, users.realname, users.college, users.major, requested.lookup_username").
		WillReturnRows(userRows)
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(awardRows)
	mock.ExpectQuery("SELECT id, leader_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "leader_id"}))
	mock.ExpectQuery("SELECT DISTINCT users.id AS student_id").
		WillReturnRows(sqlmock.NewRows([]string{"student_id", "reg_id"}))
	mock.ExpectQuery("SELECT award_members.student_id, award_members.award_id").
		WillReturnRows(sqlmock.NewRows([]string{"student_id", "award_id"}))
}

func userRow(id uint, username, name, college, major string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "username", "realname", "college", "major", "lookup_username"}).
		AddRow(id, username, name, college, major, username)
}

func TestValidateAwardRows_EmptyRequiredFields(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// 全空行：lookup 无查询
	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{{Name: "张三"}}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "不能为空")
}

func TestValidateAwardRows_TooLong(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	row := importRow("S001", strings.Repeat("项", 256))
	expectImportLookup(mock, userRow(11, "S001", "张三", "计算机学院", "软件工程"),
		sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{row}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "超出长度限制")
}

func TestValidateAwardRows_InvalidCategory(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	row := importRow("S001", "项目A")
	row.Category = "国际"
	expectImportLookup(mock, userRow(11, "S001", "张三", "计算机学院", "软件工程"),
		sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{row}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.NotEmpty(t, result[0].Error)
}

func TestValidateAwardRows_InvalidAwardName(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	row := importRow("S001", "项目A")
	row.AwardName = "特级奖"
	expectImportLookup(mock, userRow(11, "S001", "张三", "计算机学院", "软件工程"),
		sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{row}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "获奖等级必须为")
}

func TestValidateAwardRows_InvalidMonth(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	row := importRow("S001", "项目A")
	row.AwardMonth = "2026-13"
	expectImportLookup(mock, userRow(11, "S001", "张三", "计算机学院", "软件工程"),
		sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{row}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.NotEmpty(t, result[0].Error)
}

func TestValidateAwardRows_StudentNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectImportLookup(mock,
		sqlmock.NewRows([]string{"id", "username", "realname", "college", "major", "lookup_username"}),
		sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{importRow("S404", "项目A")}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "学号不存在")
}

func TestValidateAwardRows_NameMismatch(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectImportLookup(mock, userRow(11, "S001", "李四", "计算机学院", "软件工程"),
		sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{importRow("S001", "项目A")}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "学生姓名与系统学号不匹配")
}

func TestValidateAwardRows_CollegeAndMajorMismatch(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectImportLookup(mock, userRow(11, "S001", "张三", "数学学院", "统计学"),
		sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{importRow("S001", "项目A")}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "学生学院与系统学号不匹配")
	assert.Contains(t, result[0].Error, "学生专业与系统学号不匹配")
}

func TestValidateAwardRows_RegistrationMissing(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectImportLookup(mock, userRow(11, "S001", "张三", "计算机学院", "软件工程"),
		sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{importRow("S001", "项目A")}, true)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "未找到目标赛事审核通过的报名")
}

func TestValidateAwardRows_HistoryConflict(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	key := database.AwardProjectKey("项目A")
	awardRows := sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}).
		AddRow(1, 1, key, "项目A").
		AddRow(2, 1, key, "项目A")
	expectImportLookup(mock, userRow(11, "S001", "张三", "计算机学院", "软件工程"), awardRows)

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{importRow("S001", "项目A")}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "历史同名奖项冲突")
}

func TestValidateAwardRows_ExistingCategoryConflict(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	key := database.AwardProjectKey("项目A")
	awardRows := sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name", "award_category", "award_name", "status", "award_time"}).
		AddRow(1, 1, key, "项目A", "国家级", "一等奖", "approved", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	expectImportLookup(mock, userRow(11, "S001", "张三", "计算机学院", "软件工程"), awardRows)

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{importRow("S001", "项目A")}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "与已有团队的类别、等级或时间冲突")
}

func TestValidateAwardRows_DuplicateWithinFile(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// 两个学生各一行同项目 → 合法；同一学生两行同项目 → 后行标记 Duplicate
	rows := []AwardImportRow{importRow("S001", "项目A"), importRow("S001", "项目A")}
	mock.ExpectQuery("SELECT users.id, users.username, users.realname, users.college, users.major, requested.lookup_username").
		WillReturnRows(userRow(11, "S001", "张三", "计算机学院", "软件工程"))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))
	mock.ExpectQuery("SELECT id, leader_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "leader_id"}))
	mock.ExpectQuery("SELECT DISTINCT users.id AS student_id").
		WillReturnRows(sqlmock.NewRows([]string{"student_id", "reg_id"}))
	mock.ExpectQuery("SELECT award_members.student_id, award_members.award_id").
		WillReturnRows(sqlmock.NewRows([]string{"student_id", "award_id"}))

	result, bad, err := validateAwardRows(database.DB, 1, rows, false)
	assert.NoError(t, err)
	assert.Equal(t, 0, bad)
	assert.False(t, result[0].Duplicate)
	assert.True(t, result[1].Duplicate)
}

func TestValidateAwardRows_CrossTeamConflictInFile(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// 同一学生出现在两个项目 → 整组标记冲突
	rows := []AwardImportRow{importRow("S001", "项目A"), importRow("S001", "项目B")}
	mock.ExpectQuery("SELECT users.id, users.username, users.realname, users.college, users.major, requested.lookup_username").
		WillReturnRows(userRow(11, "S001", "张三", "计算机学院", "软件工程"))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))
	mock.ExpectQuery("SELECT id, leader_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "leader_id"}))
	mock.ExpectQuery("SELECT DISTINCT users.id AS student_id").
		WillReturnRows(sqlmock.NewRows([]string{"student_id", "reg_id"}))
	mock.ExpectQuery("SELECT award_members.student_id, award_members.award_id").
		WillReturnRows(sqlmock.NewRows([]string{"student_id", "award_id"}))

	result, bad, err := validateAwardRows(database.DB, 1, rows, false)
	assert.NoError(t, err)
	assert.Equal(t, 2, bad)
	assert.Contains(t, result[0].Error, "只能申报一条获奖记录")
	assert.Contains(t, result[1].Error, "只能申报一条获奖记录")
}

func TestValidateAwardRows_OtherMembershipBlocked(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// 学生已在其他奖项有成员关系 → 拒绝
	mock.ExpectQuery("SELECT users.id, users.username, users.realname, users.college, users.major, requested.lookup_username").
		WillReturnRows(userRow(11, "S001", "张三", "计算机学院", "软件工程"))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))
	mock.ExpectQuery("SELECT id, leader_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "leader_id"}))
	mock.ExpectQuery("SELECT DISTINCT users.id AS student_id").
		WillReturnRows(sqlmock.NewRows([]string{"student_id", "reg_id"}))
	mock.ExpectQuery("SELECT award_members.student_id, award_members.award_id").
		WillReturnRows(sqlmock.NewRows([]string{"student_id", "award_id"}).AddRow(11, 99))

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{importRow("S001", "项目A")}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "已有其他获奖记录")
}
