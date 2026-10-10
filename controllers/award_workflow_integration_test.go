package controllers

import (
	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Explicit opt-in: creates and drops its own isolated MySQL database, never touches the application database.
func TestAwardWorkflowMySQL(t *testing.T) {
	if os.Getenv("COMPE_AWARD_MYSQL_TEST") != "1" {
		t.Skip("set COMPE_AWARD_MYSQL_TEST=1 for isolated MySQL integration")
	}
	originalDB, originalConfig := database.DB, *config.AppConfig
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := filepath.Dir(wd)
	require.NoError(t, os.Chdir(root))
	_ = godotenv.Load()
	config.Init()
	require.NoError(t, os.Chdir(wd))
	t.Cleanup(func() { database.DB = originalDB; *config.AppConfig = originalConfig })
	cfg := config.AppConfig.Database
	dsn := driver.NewConfig()
	dsn.User = cfg.User
	dsn.Passwd = cfg.Password
	dsn.Net = "tcp"
	dsn.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	dsn.ParseTime = true
	dsn.Loc = time.Local
	admin, err := gorm.Open(mysql.Open(dsn.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	name := fmt.Sprintf("compe_award_test_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error)
	t.Cleanup(func() {
		require.NoError(t, admin.Exec("DROP DATABASE `"+name+"`").Error)
		sqlDB, _ := admin.DB()
		_ = sqlDB.Close()
	})
	dsn.DBName = name
	db, err := gorm.Open(mysql.Open(dsn.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	database.DB = db
	sqlConn, err := db.DB()
	require.NoError(t, err)
	sqlConn.SetMaxOpenConns(1)
	require.NoError(t, db.Exec("SET FOREIGN_KEY_CHECKS=0").Error)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.College{}, &models.CompDirectory{}, &models.CompDetail{}, &models.Register{}, &models.RegMember{}, &models.Award{}, &models.AwardMember{}, &models.AwardImportSetting{}))
	require.NoError(t, db.Exec("SET FOREIGN_KEY_CHECKS=1").Error)
	// Verify upgrading the old required-registration column, not only creating fresh tables.
	require.NoError(t, db.Exec("ALTER TABLE awards MODIFY COLUMN reg_id BIGINT UNSIGNED NOT NULL").Error)
	require.NoError(t, db.AutoMigrate(&models.Award{}))
	require.NoError(t, database.BackfillAwardMembers(db))
	ca, cb := models.College{Name: "学院甲", IsValid: true}, models.College{Name: "学院乙", IsValid: true}
	require.NoError(t, db.Create(&ca).Error)
	require.NoError(t, db.Create(&cb).Error)
	studentA := models.User{Username: "00123", Realname: "学生甲", College: ca.Name, Major: "专业甲", IdentityType: "student"}
	studentB := models.User{Username: "00456", Realname: "学生乙", College: cb.Name, Major: "专业乙", IdentityType: "student"}
	require.NoError(t, db.Create(&studentA).Error)
	require.NoError(t, db.Create(&studentB).Error)
	school := models.CompDirectory{CompCode: "XTEST", CompName: "校赛", CompLevel: "校级", ManagerID: studentA.ID, CollegeID: &ca.ID}
	own := models.CompDirectory{CompCode: "ATEST", CompName: "甲院省赛", CompLevel: "省级", ManagerID: studentA.ID, CollegeID: &ca.ID}
	other := models.CompDirectory{CompCode: "BTEST", CompName: "乙院省赛", CompLevel: "省级", ManagerID: studentB.ID, CollegeID: &cb.ID}
	for _, c := range []*models.CompDirectory{&school, &own, &other} {
		require.NoError(t, db.Create(c).Error)
	}
	rowA := AwardImportRow{StudentNumber: studentA.Username, Name: studentA.Realname, College: studentA.College, Major: studentA.Major, ProjectName: "跨院项目", AwardMonth: "2026-10", Category: "国际级", AwardName: "金奖"}
	rowB := AwardImportRow{StudentNumber: studentB.Username, Name: studentB.Realname, College: studentB.College, Major: studentB.Major, ProjectName: rowA.ProjectName, AwardMonth: rowA.AwardMonth, Category: rowA.Category, AwardName: rowA.AwardName}
	invoke := func(method, path, role string, collegeID *uint, userID uint, body interface{}, handler gin.HandlerFunc) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, path, bytes.NewReader(data))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("role_code", role)
		c.Set("user_id", userID)
		c.Set("managed_college_id", collegeID)
		handler(c)
		return w
	}
	importRows := func(rows []AwardImportRow) *httptest.ResponseRecorder {
		return invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", school.ID), "college_admin", &ca.ID, studentA.ID, map[string]interface{}{"rows": rows}, ImportAward)
	}
	t.Run("downloaded template has correct headers and notes", func(t *testing.T) {
		w := invoke("GET", fmt.Sprintf("/api/award/export-template?comp_id=%d", school.ID), "college_admin", &ca.ID, studentA.ID, nil, ExportAwardTemplate)
		require.Equal(t, 200, w.Code, w.Body.String())
		f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
		require.NoError(t, err)
		defer f.Close()
		rows, err := f.GetRows("Sheet1")
		require.NoError(t, err)
		require.Equal(t, awardImportHeaders, rows[0])
		require.NotContains(t, rows[0], "承办单位")
		for i, note := range awardTemplateNotes {
			value, err := f.GetCellValue("Sheet1", fmt.Sprintf("A%d", 16+i))
			require.NoError(t, err)
			require.Equal(t, note, value)
		}
		_, err = parseAwardSheet(rows)
		require.ErrorContains(t, err, "暂无可导入数据")
	})
	t.Run("registration-free and cross-college grouping", func(t *testing.T) {
		w := importRows([]AwardImportRow{rowA, rowB})
		require.Equal(t, 200, w.Code, w.Body.String())
		var awards []models.Award
		require.NoError(t, db.Find(&awards).Error)
		require.Len(t, awards, 1)
		require.Equal(t, "国家级", awards[0].AwardCategory)
		require.Equal(t, "金奖", awards[0].AwardName)
		var regNull int64
		require.NoError(t, db.Model(&models.Award{}).Where("reg_id IS NULL").Count(&regNull).Error)
		require.EqualValues(t, 1, regNull)
		var members []models.AwardMember
		require.NoError(t, db.Find(&members).Error)
		require.Len(t, members, 2)
		require.Equal(t, ca.ID, *members[1].SubmittedCollegeID)
	})
	var a models.Award
	require.NoError(t, db.First(&a).Error)
	t.Run("duplicate preserves first college", func(t *testing.T) {
		w := invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", school.ID), "college_admin", &cb.ID, studentB.ID, map[string]interface{}{"rows": []AwardImportRow{rowB}}, ImportAward)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"existing_count":1`)
		var m models.AwardMember
		require.NoError(t, db.Where("student_id = ?", studentB.ID).First(&m).Error)
		require.Equal(t, ca.ID, *m.SubmittedCollegeID)
	})
	t.Run("conflict is atomic", func(t *testing.T) {
		fresh := rowA
		fresh.ProjectName = "应回滚的新项目"
		conflict := rowB
		conflict.AwardName = "二等奖"
		w := importRows([]AwardImportRow{fresh, conflict})
		require.Equal(t, 400, w.Code, w.Body.String())
		var n int64
		require.NoError(t, db.Model(&models.Award{}).Count(&n).Error)
		require.EqualValues(t, 1, n)
	})
	t.Run("college competition visibility", func(t *testing.T) {
		w := invoke("GET", "/api/award/list", "college_admin", &ca.ID, studentA.ID, nil, GetAwardCompList)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), own.CompName)
		require.NotContains(t, w.Body.String(), other.CompName)
		w = invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", other.ID), "college_admin", &ca.ID, studentA.ID, map[string]interface{}{"rows": []AwardImportRow{rowA}}, ImportAward)
		require.Equal(t, 403, w.Code)
	})
	t.Run("switch covers every competition", func(t *testing.T) {
		w := invoke("PUT", "/settings", "college_admin", &ca.ID, studentA.ID, map[string]interface{}{"require_registration": true}, UpdateAwardImportSetting)
		require.Equal(t, 403, w.Code)
		w = invoke("PUT", "/settings", "school_admin", nil, studentA.ID, map[string]interface{}{"require_registration": true}, UpdateAwardImportSetting)
		require.Equal(t, 200, w.Code)
		w = importRows([]AwardImportRow{rowA})
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "报名")
		w = invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", own.ID), "college_admin", &ca.ID, studentA.ID, map[string]interface{}{"rows": []AwardImportRow{rowA}}, ImportAward)
		require.Equal(t, 400, w.Code)
		reg := models.Register{CompID: school.ID, LeaderID: studentA.ID, TeamName: "原报名名称", Status: 1, AdvisorInfo: "{}"}
		require.NoError(t, db.Create(&reg).Error)
		require.NoError(t, db.Create(&models.RegMember{RegID: reg.ID, StudentID: studentB.Username, Name: studentB.Realname}).Error)
		w = importRows([]AwardImportRow{rowA, rowB})
		require.Equal(t, 200, w.Code, w.Body.String())
		var got models.Register
		require.NoError(t, db.First(&got, reg.ID).Error)
		require.Equal(t, "原报名名称", got.TeamName)
	})
	var m models.AwardMember
	require.NoError(t, db.Where("student_id = ?", studentA.ID).First(&m).Error)
	edit := func(role string, college *uint, memberID uint, body interface{}, handler gin.HandlerFunc) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/edit", bytes.NewReader(b))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("role_code", role)
		c.Set("user_id", studentB.ID)
		c.Set("managed_college_id", college)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(memberID)}}
		handler(c)
		return w
	}
	t.Run("member edits college-owned and team edits member-college-owned", func(t *testing.T) {
		w := edit("college_admin", &ca.ID, m.ID, map[string]interface{}{"remark": "同院另一管理员修改"}, UpdateAwardMember)
		require.Equal(t, 200, w.Code, w.Body.String())
		w = edit("college_admin", &cb.ID, m.ID, map[string]interface{}{"remark": "不应允许"}, UpdateAwardMember)
		require.Equal(t, 403, w.Code)
		w = edit("college_admin", &cb.ID, a.ID, map[string]interface{}{"project_name": a.ProjectName, "award_category": "国家级", "award_name": "一等奖", "award_month": "2026-10"}, UpdateAwardTeam)
		require.Equal(t, 200, w.Code, w.Body.String())
	})
	t.Run("student list works without award registration", func(t *testing.T) {
		w := invoke("GET", "/student/my-awards", "student", nil, studentB.ID, nil, GetStudentMyAwardList)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), school.CompName)
	})
	t.Run("student supplement materializes members and rejects duplicate imported project", func(t *testing.T) {
		body := map[string]interface{}{"comp_id": school.ID, "team_name": a.ProjectName, "award_level": "国家级一等奖", "award_name": "一等奖", "proof_url": "/proof.pdf", "members": []map[string]interface{}{{"name": studentB.Realname, "student_id": studentB.Username, "college": studentB.College, "phone": "13800000000", "is_leader": true}}}
		w := invoke("POST", "/supplement", "student", nil, studentB.ID, body, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code, w.Body.String())
		body["comp_id"] = other.ID
		body["team_name"] = "学生补录项目"
		w = invoke("POST", "/supplement", "student", nil, studentB.ID, body, SubmitStudentAwardSupplement)
		require.Equal(t, 200, w.Code, w.Body.String())
		var supplement models.Award
		require.NoError(t, db.Where("comp_id = ? AND source = ?", other.ID, "supplement").First(&supplement).Error)
		var members []models.AwardMember
		require.NoError(t, db.Where("award_id = ?", supplement.ID).Find(&members).Error)
		require.Len(t, members, 1)
		require.Equal(t, studentB.ID, members[0].StudentID)
	})
	t.Run("legacy migration idempotent and no inferred ownership", func(t *testing.T) {
		var reg models.Register
		require.NoError(t, db.First(&reg).Error)
		legacy := models.Award{CompID: school.ID, RegID: reg.ID, Status: "approved", AwardName: "二等奖"}
		require.NoError(t, db.Create(&legacy).Error)
		require.NoError(t, database.BackfillAwardMembers(db))
		require.NoError(t, database.BackfillAwardMembers(db))
		var members []models.AwardMember
		require.NoError(t, db.Where("award_id = ?", legacy.ID).Find(&members).Error)
		require.Len(t, members, 2)
		require.Nil(t, members[0].SubmittedCollegeID)
	})
}
