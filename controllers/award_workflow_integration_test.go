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
func newAwardMySQLTestDB(t *testing.T) *gorm.DB {
	t.Helper()
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
	// Allow an isolated local server when the application's MySQL service is stopped.
	if override := os.Getenv("COMPE_AWARD_MYSQL_DSN"); override != "" {
		dsn, err = driver.ParseDSN(override)
		require.NoError(t, err)
	}
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
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.College{}, &models.CompDirectory{}, &models.CompDetail{}, &models.Register{}, &models.RegMember{}, &models.Award{}, &models.AwardMember{}, &models.AwardAdvisor{}, &models.AwardChange{}, &models.AwardImportSetting{}, &models.FileRecord{}))
	require.NoError(t, db.Exec("SET FOREIGN_KEY_CHECKS=1").Error)
	// Verify upgrading the old required-registration column, not only creating fresh tables.
	require.NoError(t, db.Exec("ALTER TABLE awards MODIFY COLUMN reg_id BIGINT UNSIGNED NOT NULL").Error)
	require.NoError(t, db.AutoMigrate(&models.Award{}))
	require.NoError(t, database.BackfillAwardMembers(db))
	return db
}

func TestAwardWorkflowMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	ca, cb := models.College{Name: "学院甲", IsValid: true}, models.College{Name: "学院乙", IsValid: true}
	require.NoError(t, db.Create(&ca).Error)
	require.NoError(t, db.Create(&cb).Error)
	studentA := models.User{Username: "00123", Realname: "学生甲", College: ca.Name, Major: "专业甲", IdentityType: "student"}
	studentB := models.User{Username: "00456", Realname: "学生乙", College: cb.Name, Major: "专业乙", IdentityType: "student"}
	require.NoError(t, db.Create(&studentA).Error)
	require.NoError(t, db.Create(&studentB).Error)
	teacherA := models.User{Username: "T001", Realname: "教师甲", IdentityType: "staff", College: ca.Name}
	teacherB := models.User{Username: "T002", Realname: "教师乙", IdentityType: "staff", College: cb.Name}
	require.NoError(t, db.Create(&teacherA).Error)
	require.NoError(t, db.Create(&teacherB).Error)
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
		require.Equal(t, awardTemplateHeaders, rows[0])
		require.NotContains(t, rows[0], "承办单位")
		for i, note := range awardTemplateNotes {
			value, err := f.GetCellValue("Sheet1", fmt.Sprintf("A%d", 16+i))
			require.NoError(t, err)
			require.Equal(t, note, value)
		}
		_, err = parseAwardSheet(rows)
		require.ErrorContains(t, err, "暂无可导入数据")
	})
	t.Run("registration-free import rejects another college member", func(t *testing.T) {
		w := importRows([]AwardImportRow{rowA, rowB})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"fail_count":1`)
		var awards []models.Award
		require.NoError(t, db.Find(&awards).Error)
		require.Len(t, awards, 1)
		require.Equal(t, "国家级", awards[0].AwardCategory)
		require.Equal(t, "一等奖", awards[0].AwardName)
		var regNull int64
		require.NoError(t, db.Model(&models.Award{}).Where("reg_id IS NULL").Count(&regNull).Error)
		require.EqualValues(t, 1, regNull)
		var members []models.AwardMember
		require.NoError(t, db.Find(&members).Error)
		require.Len(t, members, 1)
		require.Equal(t, studentA.ID, members[0].StudentID)
		require.Equal(t, ca.ID, *members[0].SubmittedCollegeID)
	})
	var a models.Award
	require.NoError(t, db.First(&a).Error)
	t.Run("shared competition lets each college add only its own member", func(t *testing.T) {
		w := invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", school.ID), "college_admin", &cb.ID, studentB.ID, map[string]interface{}{"rows": []AwardImportRow{rowB}}, ImportAward)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"success_count":1`)
		var m models.AwardMember
		require.NoError(t, db.Where("student_id = ?", studentB.ID).First(&m).Error)
		require.Equal(t, cb.ID, *m.SubmittedCollegeID)
	})
	t.Run("conflicting rows are skipped", func(t *testing.T) {
		fresh := rowA
		fresh.ProjectName = "应回滚的新项目"
		conflict := rowB
		conflict.AwardName = "二等奖"
		w := importRows([]AwardImportRow{fresh, conflict})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"fail_count":2`)
		var n int64
		require.NoError(t, db.Model(&models.Award{}).Count(&n).Error)
		require.EqualValues(t, 1, n)
	})
	t.Run("college competition visibility", func(t *testing.T) {
		unassignedProvince := models.CompDirectory{CompCode: "UNASSIGNED_S", CompName: "未设置学院的省赛", CompLevel: "省级", ManagerID: studentA.ID}
		unassignedNational := models.CompDirectory{CompCode: "UNASSIGNED_G", CompName: "未设置学院的国赛", CompLevel: "国家级", ManagerID: studentA.ID}
		require.NoError(t, db.Create(&unassignedProvince).Error)
		require.NoError(t, db.Create(&unassignedNational).Error)

		w := invoke("GET", "/api/award/list", "college_admin", &ca.ID, studentA.ID, nil, GetAwardCompList)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), own.CompName)
		require.NotContains(t, w.Body.String(), other.CompName)
		require.Contains(t, w.Body.String(), unassignedProvince.CompName)
		require.Contains(t, w.Body.String(), unassignedNational.CompName)
		secondCollege := invoke("GET", "/api/award/list", "college_admin", &cb.ID, studentB.ID, nil, GetAwardCompList)
		require.Equal(t, 200, secondCollege.Code, secondCollege.Body.String())
		require.Contains(t, secondCollege.Body.String(), unassignedProvince.CompName)
		require.Contains(t, secondCollege.Body.String(), unassignedNational.CompName)
		require.NotContains(t, secondCollege.Body.String(), own.CompName)
		managerList := invoke("GET", "/api/award/list", "competition_manager", nil, studentB.ID, nil, GetAwardCompList)
		require.Equal(t, 200, managerList.Code, managerList.Body.String())
		require.Contains(t, managerList.Body.String(), other.CompName)
		require.NotContains(t, managerList.Body.String(), unassignedProvince.CompName)
		template := invoke("GET", fmt.Sprintf("/api/award/export-template?comp_id=%d", unassignedProvince.ID), "college_admin", &ca.ID, studentA.ID, nil, ExportAwardTemplate)
		require.Equal(t, 200, template.Code, template.Body.String())
		for _, collegeCase := range []struct {
			collegeID *uint
			userID    uint
			row       AwardImportRow
		}{{&ca.ID, studentA.ID, rowA}, {&cb.ID, studentB.ID, rowB}} {
			imported := invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", unassignedProvince.ID), "college_admin", collegeCase.collegeID, collegeCase.userID, map[string]interface{}{"rows": []AwardImportRow{collegeCase.row}}, ImportAward)
			require.Equal(t, 200, imported.Code, imported.Body.String())
		}
		compAwards := invoke("GET", fmt.Sprintf("/comp-awards?comp_id=%d", unassignedProvince.ID), "college_admin", &cb.ID, studentB.ID, nil, GetCompAwards)
		require.Equal(t, 200, compAwards.Code, compAwards.Body.String())
		require.Contains(t, compAwards.Body.String(), rowA.ProjectName)

		w = invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", other.ID), "college_admin", &ca.ID, studentA.ID, map[string]interface{}{"rows": []AwardImportRow{rowA}}, ImportAward)
		require.Equal(t, 403, w.Code)
	})
	t.Run("switch covers every competition", func(t *testing.T) {
		w := invoke("PUT", "/settings", "college_admin", &ca.ID, studentA.ID, map[string]interface{}{"require_registration": true}, UpdateAwardImportSetting)
		require.Equal(t, 403, w.Code)
		w = invoke("PUT", "/settings", "school_admin", nil, studentA.ID, map[string]interface{}{"require_registration": true}, UpdateAwardImportSetting)
		require.Equal(t, 200, w.Code)
		w = importRows([]AwardImportRow{rowA})
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), `"fail_count":1`)
		require.Contains(t, w.Body.String(), "报名")
		w = invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", own.ID), "college_admin", &ca.ID, studentA.ID, map[string]interface{}{"rows": []AwardImportRow{rowA}}, ImportAward)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), `"fail_count":1`)
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
		studentProof := models.FileRecord{BizType: "award_cert", FileHash: "11234567890123456789012345678901", OriginalName: "student.pdf", UploaderID: studentB.ID, StoragePath: "/static/temp/student.pdf"}
		require.NoError(t, db.Create(&studentProof).Error)
		body := map[string]interface{}{"comp_id": school.ID, "team_name": a.ProjectName, "award_level": "国家级一等奖", "award_name": "一等奖", "proof_url": buildDownloadURL(studentProof.BizType, studentProof.FileHash, studentProof.OriginalName), "members": []map[string]interface{}{{"name": studentB.Realname, "student_id": studentB.Username, "college": studentB.College, "phone": "13800000000", "is_leader": true}}}
		body["advisors"] = []map[string]string{{"username": teacherA.Username, "name": teacherA.Realname}}
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
		body["participation_type"] = "team"
		body["award_category"] = "省级"
		body["award_name"] = "二等奖"
		body["award_date"] = "2026-10-12"
		body["advisors"] = []map[string]string{{"username": teacherA.Username, "name": "教师甲", "college": ca.Name}, {"username": teacherB.Username, "name": "教师乙", "college": cb.Name}}
		body["remark"] = "团队项目说明"
		body["members"] = []map[string]interface{}{
			{"name": studentA.Realname, "student_id": studentA.Username, "college": studentA.College, "phone": "13800000001", "email": "a@example.com", "is_leader": true},
			{"name": studentB.Realname, "student_id": studentB.Username, "college": studentB.College, "phone": "13800000002", "email": "b@example.com", "is_leader": false},
		}
		require.NoError(t, db.Model(&supplement).Update("status", "rejected").Error)
		w = invoke("POST", "/supplement", "student", nil, studentB.ID, body, SubmitStudentAwardSupplement)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.NoError(t, db.Preload("Members").First(&supplement, supplement.ID).Error)
		require.Equal(t, "省级二等奖", supplement.AwardLevel)
		require.Equal(t, "team", supplement.ParticipationType)
		require.Equal(t, "团队项目说明", supplement.Remark)
		require.Contains(t, supplement.AdvisorInfo, "教师乙")
		require.Len(t, supplement.Members, 2)
		var advisors []models.AwardAdvisor
		require.NoError(t, db.Where("award_id = ?", supplement.ID).Find(&advisors).Error)
		require.Len(t, advisors, 2)
		teacherDetail := invoke("GET", "/advisor", "teacher", nil, teacherA.ID, nil, func(c *gin.Context) {
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(supplement.ID)}}
			GetAdvisorAwardDetail(c)
		})
		require.Equal(t, 200, teacherDetail.Code, teacherDetail.Body.String())
		teacherBody := map[string]interface{}{"project_name": supplement.ProjectName, "award_category": "省级", "award_name": "二等奖", "award_date": "2026-10-12", "proof_url": "/api/file/download/award_proof/01234567890123456789012345678901_certificate.pdf", "remark": "教师更正", "update_time": supplement.UpdatedAt}
		require.NoError(t, db.AutoMigrate(&models.FileRecord{}))
		proofRecord := models.FileRecord{BizType: "award_proof", FileHash: "01234567890123456789012345678901", OriginalName: "certificate.pdf", UploaderID: teacherA.ID, StoragePath: "/static/award_proofs/test/certificate.pdf"}
		require.NoError(t, db.Create(&proofRecord).Error)
		teacherUpdate := invoke("PUT", "/advisor", "teacher", nil, teacherA.ID, teacherBody, func(c *gin.Context) {
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(supplement.ID)}}
			UpdateAdvisorAward(c)
		})
		require.Equal(t, 200, teacherUpdate.Code, teacherUpdate.Body.String())
		var changes []models.AwardChange
		require.NoError(t, db.Where("award_id = ?", supplement.ID).Find(&changes).Error)
		require.Len(t, changes, 1)
		require.Equal(t, teacherA.ID, changes[0].OperatorID)
		staleAudit := invoke("PUT", "/pass", "competition_manager", nil, studentB.ID, map[string]interface{}{"update_time": teacherBody["update_time"]}, func(c *gin.Context) {
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(supplement.ID)}}
			PassAwardAudit(c)
		})
		require.Equal(t, 409, staleAudit.Code, staleAudit.Body.String())
		staleBatch := invoke("PUT", "/batch", "competition_manager", nil, studentB.ID, map[string]interface{}{"ids": []uint{supplement.ID}, "versions": map[uint]interface{}{supplement.ID: teacherBody["update_time"]}}, BatchPassAwardAudit)
		require.Equal(t, 409, staleBatch.Code, staleBatch.Body.String())
		// A stale student form cannot overwrite the advisor's new draft.
		body["update_time"] = teacherBody["update_time"]
		staleStudent := invoke("POST", "/supplement", "student", nil, studentB.ID, body, SubmitStudentAwardSupplement)
		require.NotEqual(t, 200, staleStudent.Code, staleStudent.Body.String())
		var latest models.Award
		require.NoError(t, db.First(&latest, supplement.ID).Error)
		require.Equal(t, "教师更正", latest.Remark)
		proof := models.FileRecord{BizType: "award_proof", FileHash: "01234567890123456789012345678901", OriginalName: "certificate.pdf"}
		require.True(t, canAccessAwardProof(t.Context(), teacherB.ID, &UserAccessScope{UserID: teacherB.ID, RoleCode: "teacher"}, proof))
		require.True(t, canAccessAwardProof(t.Context(), studentB.ID, &UserAccessScope{UserID: studentB.ID, RoleCode: "student"}, proof))
		teacherBody["remark"] = "团队项目说明"
		require.NoError(t, db.First(&supplement, supplement.ID).Error)
		teacherBody["update_time"] = supplement.UpdatedAt
		teacherUpdate = invoke("PUT", "/advisor", "competition_manager", nil, teacherB.ID, teacherBody, func(c *gin.Context) {
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(supplement.ID)}}
			UpdateAdvisorAward(c)
		})
		require.Equal(t, 200, teacherUpdate.Code, teacherUpdate.Body.String())
		w = invoke("GET", fmt.Sprintf("/comp-awards?comp_id=%d", other.ID), "school_admin", nil, studentA.ID, nil, GetCompAwards)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), supplement.ProjectName)
		detail := invoke("GET", "/detail", "competition_manager", nil, studentB.ID, nil, func(c *gin.Context) {
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(supplement.ID)}}
			GetAwardAuditDetail(c)
		})
		require.Equal(t, 200, detail.Code, detail.Body.String())
		require.Contains(t, detail.Body.String(), `"student_name":"学生甲"`)
		require.Contains(t, detail.Body.String(), "13800000001")
		require.Contains(t, detail.Body.String(), "教师甲")
		w = invoke("PUT", "/pass", "competition_manager", nil, studentB.ID, nil, func(c *gin.Context) {
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(supplement.ID)}}
			PassAwardAudit(c)
		})
		require.Equal(t, 200, w.Code, w.Body.String())
		w = invoke("GET", fmt.Sprintf("/comp-awards?comp_id=%d", other.ID), "school_admin", nil, studentA.ID, nil, GetCompAwards)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), supplement.ProjectName)
		require.Contains(t, w.Body.String(), "团队项目说明")
		w = invoke("GET", "/student/my-awards", "student", nil, studentB.ID, nil, GetStudentMyAwardList)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "13800000002")
		require.Contains(t, w.Body.String(), "教师乙")

	})
	t.Run("award audits stay within managed competitions including batches", func(t *testing.T) {
		mine := models.Award{CompID: own.ID, ProjectName: "我的申报", Status: "draft", AdvisorInfo: "[]"}
		foreign := models.Award{CompID: other.ID, ProjectName: "他人申报", Status: "draft", AdvisorInfo: "[]"}
		require.NoError(t, db.Create(&mine).Error)
		require.NoError(t, db.Create(&foreign).Error)
		for _, handler := range []gin.HandlerFunc{PassAwardAudit, RejectAwardAudit} {
			w := invoke("PUT", "/audit", "competition_manager", nil, studentA.ID, map[string]interface{}{"reason": "材料不全"}, func(c *gin.Context) {
				c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(foreign.ID)}}
				handler(c)
			})
			require.Equal(t, 403, w.Code, w.Body.String())
		}
		for _, handler := range []gin.HandlerFunc{BatchPassAwardAudit, BatchRejectAwardAudit} {
			w := invoke("PUT", "/batch", "competition_manager", nil, studentA.ID, map[string]interface{}{"ids": []uint{mine.ID, foreign.ID}, "reason": "材料不全"}, handler)
			require.Equal(t, 403, w.Code, w.Body.String())
			require.NoError(t, db.First(&mine, mine.ID).Error)
			require.Equal(t, "draft", mine.Status)
			require.NoError(t, db.First(&foreign, foreign.ID).Error)
			require.Equal(t, "draft", foreign.Status)
		}
		w := invoke("PUT", "/batch", "competition_manager", nil, studentA.ID, map[string]interface{}{"ids": []uint{mine.ID}}, BatchPassAwardAudit)
		require.Equal(t, 200, w.Code, w.Body.String())
		w = invoke("PUT", "/audit", "competition_manager", nil, studentB.ID, map[string]interface{}{"reason": "补交证明"}, func(c *gin.Context) {
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(foreign.ID)}}
			RejectAwardAudit(c)
		})
		require.Equal(t, 200, w.Code, w.Body.String())
	})
	t.Run("legacy migration idempotent and no inferred ownership", func(t *testing.T) {
		var reg models.Register
		require.NoError(t, db.First(&reg).Error)
		legacy := models.Award{CompID: school.ID, RegID: reg.ID, Status: "approved", AwardName: "二等奖", AdvisorInfo: "[]"}
		require.NoError(t, db.Create(&legacy).Error)
		require.NoError(t, database.BackfillAwardMembers(db))
		require.NoError(t, database.BackfillAwardMembers(db))
		var members []models.AwardMember
		require.NoError(t, db.Where("award_id = ?", legacy.ID).Find(&members).Error)
		require.Len(t, members, 2)
		require.Nil(t, members[0].SubmittedCollegeID)
	})

	t.Run("student declarations separated from approved awards and real registrations", func(t *testing.T) {
		pending := models.Award{CompID: other.ID, ProjectName: "待审核申报项目", Status: "draft", Source: "supplement", AdvisorInfo: "[]", MembersMigrated: true}
		require.NoError(t, db.Create(&pending).Error)
		require.NoError(t, db.Create(&models.AwardMember{AwardID: pending.ID, StudentID: studentB.ID, Name: studentB.Realname, StudentNumber: studentB.Username, College: studentB.College}).Error)
		w := invoke("GET", "/student/my-awards?source=supplement", "student", nil, studentB.ID, nil, GetStudentMyAwardList)
		require.Equal(t, 200, w.Code, w.Body.String())
		var result struct {
			Data struct {
				List  []models.Award `json:"list"`
				Total int            `json:"total"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		require.Equal(t, 2, result.Data.Total)
		for _, award := range result.Data.List {
			require.Equal(t, "supplement", award.Source)
		}
		require.Contains(t, w.Body.String(), pending.ProjectName)
		w = invoke("GET", "/student/my-awards?status=approved", "student", nil, studentB.ID, nil, GetStudentMyAwardList)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		for _, award := range result.Data.List {
			require.Equal(t, "approved", award.Status)
		}
		require.NotContains(t, w.Body.String(), pending.ProjectName)
		w = invoke("GET", "/my-reg?registration_only=true", "student", nil, studentB.ID, nil, GetMyRegList)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), school.CompName)
		require.NotContains(t, w.Body.String(), other.CompName)
	})
	t.Run("registration participant modes and member resubmission", func(t *testing.T) {
		comp := models.CompDirectory{CompCode: "REGMODE", CompName: "报名参赛方式测试", ManagerID: studentB.ID}
		require.NoError(t, db.Create(&comp).Error)
		detail := models.CompDetail{CompStartTime: time.Now(), CompEndTime: time.Now(), SubmitStartTime: time.Now(), SubmitEndTime: time.Now(), ReviewStartTime: time.Now(), ReviewEndTime: time.Now(), Track: "[]", AwardHierarchy: "[]", CompID: comp.ID, ParticipantType: 2, MinTeamMember: 1, MaxTeamMember: 5, RegStartTime: time.Now().Add(-time.Hour), RegEndTime: time.Now().Add(time.Hour), NeedRegAudit: 1}
		require.NoError(t, db.Create(&detail).Error)
		leader := map[string]interface{}{"name": studentB.Realname, "stuID": studentB.Username, "phone": "13800000001", "email": "b@example.com", "college": studentB.College}
		member := map[string]interface{}{"name": studentA.Realname, "stuID": studentA.Username, "phone": "13800000002", "email": "a@example.com", "college": studentA.College}
		body := map[string]interface{}{"comp_id": comp.ID, "team_name": "团队参赛项目", "participation_type": "team", "leader": leader, "members": []interface{}{member}}
		w := invoke("POST", "/reg/submit", "student", nil, studentA.ID, body, SubmitRegistration)
		require.Equal(t, 200, w.Code, w.Body.String())
		var reg models.Register
		require.NoError(t, db.Where("comp_id = ?", comp.ID).Preload("Members").First(&reg).Error)
		require.Equal(t, studentB.ID, reg.LeaderID)
		require.Len(t, reg.Members, 2)
		require.NoError(t, db.Model(&models.Register{}).Where("id = ?", reg.ID).Update("status", 2).Error)
		body["reg_id"] = reg.ID
		body["team_name"] = "普通成员重新提交项目"
		body["leader"] = member
		body["members"] = []interface{}{leader}
		w = invoke("PUT", "/reg/resubmit", "student", nil, studentA.ID, body, ResubmitRegistration)
		require.Equal(t, 200, w.Code, w.Body.String())
		var updated models.Register
		require.NoError(t, db.Preload("Members").First(&updated, reg.ID).Error)
		require.Equal(t, studentA.ID, updated.LeaderID)
		require.Equal(t, "普通成员重新提交项目", updated.TeamName)
		require.Len(t, updated.Members, 2)
		body["participation_type"] = "individual"
		body["members"] = []interface{}{}
		require.NoError(t, db.Model(&models.CompDetail{}).Where("id = ?", detail.ID).Update("min_team_member", 2).Error)
		w = invoke("POST", "/reg/submit", "student", nil, studentA.ID, body, SubmitRegistration)
		require.Equal(t, 400, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "不允许个人参赛")
		require.NoError(t, db.Model(&models.CompDetail{}).Where("id = ?", detail.ID).Updates(map[string]interface{}{"participant_type": 1, "min_team_member": 1, "max_team_member": 1}).Error)
		body["leader"] = leader
		w = invoke("POST", "/reg/submit", "student", nil, studentA.ID, body, SubmitRegistration)
		require.Equal(t, 400, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "只能填写本人信息")
	})

}
