package controllers

import (
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

func TestUnifiedAwardMatching(t *testing.T) {
	competitions := []AwardImportCompetition{
		{ID: 1, CompName: "程序设计竞赛", CompCode: "0001", Year: 2025},
		{ID: 2, CompName: "程序设计竞赛", CompCode: "0002", Year: 2026},
		{ID: 3, CompName: "同名赛事", CompCode: "0003", Year: 2026},
		{ID: 4, CompName: "同名赛事", CompCode: "0004", Year: 2026},
		{ID: 5, CompName: "仅往年赛事", CompCode: "0005", Year: 2025},
	}
	for _, tc := range []struct {
		name                  string
		row                   AwardImportRow
		wantID                uint
		wantStatus, wantError string
	}{
		{"name only", AwardImportRow{CompName: " 程序设计竞赛 "}, 2, "matched", ""},
		{"legacy year cannot redirect", AwardImportRow{CompName: "程序设计竞赛", CompYear: "2025"}, 2, "matched", ""},
		{"code alone cannot disambiguate", AwardImportRow{CompName: "同名赛事", CompCode: "0004"}, 0, "ambiguous", "存在多个"},
		{"explicit selection", AwardImportRow{CompID: 4, CompName: "同名赛事"}, 4, "matched", ""},
		{"ambiguous", AwardImportRow{CompName: "同名赛事"}, 0, "ambiguous", "存在多个"},
		{"legacy code cannot redirect", AwardImportRow{CompName: "程序设计竞赛", CompCode: "0001"}, 2, "matched", ""},
		{"selected ID name mismatch", AwardImportRow{CompID: 3, CompName: "其他名字"}, 3, "unmatched", "不一致"},
		{"legacy invalid code ignored", AwardImportRow{CompName: "程序设计竞赛", CompCode: "9999"}, 2, "matched", ""},
		{"previous year only", AwardImportRow{CompName: "仅往年赛事"}, 0, "unmatched", "不存在或不在"},
		{"partial name", AwardImportRow{CompName: "程序设计", CompYear: "2026"}, 0, "unmatched", "不存在或不在"},
		{"year not inferred from award month", AwardImportRow{CompName: "程序设计竞赛", AwardMonth: "2025-10"}, 2, "matched", ""},
		{"legacy invalid year ignored", AwardImportRow{CompName: "程序设计竞赛", CompYear: "26"}, 2, "matched", ""},
		{"empty name", AwardImportRow{CompYear: "2026", CompCode: "0002"}, 0, "unmatched", "不能为空"},
		{"old bound ID cannot retarget", AwardImportRow{CompID: 1, CompName: "程序设计竞赛"}, 1, "unmatched", "不存在或不在"},
		{"removed bound ID cannot fall back", AwardImportRow{CompID: 99, CompName: "程序设计竞赛", CompYear: "2026"}, 99, "unmatched", "不存在或不在"},
		{"client result is not trusted", AwardImportRow{CompName: "不存在", CompYear: "2026", MatchStatus: "matched", Competition: &competitions[0], Existing: true}, 0, "unmatched", "不存在或不在"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []AwardImportRow{tc.row}
			got := matchUnifiedAwardRows(input, competitions, 2026)[0]
			require.Equal(t, "2026", got.CompYear)
			require.Equal(t, tc.wantID, got.CompID)
			require.Equal(t, tc.wantStatus, got.MatchStatus)
			if tc.wantError == "" {
				require.Empty(t, got.Error)
				require.Equal(t, tc.wantID, got.Competition.ID)
			} else {
				require.Contains(t, got.Error, tc.wantError)
				require.Nil(t, got.Competition)
			}
			require.Equal(t, tc.row, input[0], "matching must not mutate the request")
			if got.MatchStatus == "ambiguous" {
				require.Len(t, got.Candidates, 2)
			}
		})
	}
}

func TestUnifiedAwardCurrentYearBoundary(t *testing.T) {
	require.Equal(t, 2026, unifiedAwardYearAt(time.Date(2026, 12, 31, 15, 59, 59, 0, time.UTC)))
	require.Equal(t, 2027, unifiedAwardYearAt(time.Date(2026, 12, 31, 16, 0, 0, 0, time.UTC)))
	rows := matchUnifiedAwardRows([]AwardImportRow{{CompName: "跨年赛事"}}, []AwardImportCompetition{
		{ID: 1, CompName: "跨年赛事", Year: 2026}, {ID: 2, CompName: "跨年赛事", Year: 2027},
	}, 2027)
	require.Equal(t, uint(2), rows[0].CompID)
	require.Equal(t, "2027", rows[0].CompYear)
}

// Opt-in browser test uses real JWT/permission middleware and real controllers
// against the same isolated database helper as the other MySQL workflow tests.
func TestUnifiedAwardBrowserMySQL(t *testing.T) {
	if os.Getenv("COMPE_AWARD_BROWSER_TEST") != "1" {
		t.Skip("set COMPE_AWARD_BROWSER_TEST=1 for the real browser/API/MySQL workflow")
	}
	db := newAwardMySQLTestDB(t)
	t.Setenv("JWT_SECRET", "isolated-unified-award-browser-test-secret")
	require.NoError(t, db.AutoMigrate(&models.Permission{}, &models.Role{}, &models.UserRole{}, &models.RolePermission{}))
	college := models.College{Name: "统一导入实测学院", IsValid: true}
	require.NoError(t, db.Create(&college).Error)
	admin := models.User{Username: "LIVEADMIN", Realname: "实测管理员", IdentityType: "staff", College: college.Name}
	require.NoError(t, db.Create(&admin).Error)
	role := models.Role{RoleCode: "school_admin", RoleName: "实测校管理员"}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&models.UserRole{UserID: admin.ID, RoleID: role.ID}).Error)
	for _, code := range []string{"award:import", "award:list", "award:comp:list", "comp:list", "reg:user:list"} {
		permission := models.Permission{Name: code, Code: code}
		require.NoError(t, db.Create(&permission).Error)
		require.NoError(t, db.Create(&models.RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error)
	}
	students := []models.User{
		{Username: "000101", Realname: "实测学生甲", College: college.Name, Major: "实测专业", IdentityType: "student"},
		{Username: "000102", Realname: "实测学生乙", College: college.Name, Major: "实测专业", IdentityType: "student"},
	}
	require.NoError(t, db.Create(&students).Error)
	teacher := models.User{Username: "T0001", Realname: "实测教师", College: college.Name, IdentityType: "staff"}
	require.NoError(t, db.Create(&teacher).Error)
	year := currentUnifiedAwardYear()
	competitions := []models.CompDirectory{
		{CompCode: "000025", CompName: "真实联调赛事甲", Year: year, CompLevel: "省级", ManagerID: admin.ID, CollegeID: &college.ID},
		{CompCode: "000026", CompName: "真实联调赛事乙", Year: year, CompLevel: "省级", ManagerID: admin.ID, CollegeID: &college.ID},
		{CompCode: "000024", CompName: "真实联调赛事甲", Year: year - 1, CompLevel: "省级", ManagerID: admin.ID, CollegeID: &college.ID},
	}
	require.NoError(t, db.Create(&competitions).Error)
	token, err := utils.GenerateToken(admin.ID, admin.Username, "school_admin")
	require.NoError(t, err)
	router := gin.New()
	api := router.Group("/api", middleware.AuthRequired())
	api.GET("/college/list", GetCollegeList)
	api.GET("/comp/:id", middleware.RequirePermission("comp:list"), GetCompetitionDetail)
	api.GET("/award/list", middleware.RequirePermission("award:list"), GetAwardCompList)
	api.GET("/award/comp-awards", middleware.RequirePermission("award:comp:list"), GetCompAwards)
	api.GET("/award/import/unified/template", middleware.RequirePermission("award:import"), ExportUnifiedAwardTemplate)
	api.POST("/award/import/unified/preview", middleware.RequirePermission("award:import"), PreviewUnifiedAwardImport)
	api.POST("/award/import/unified", middleware.RequirePermission("award:import"), ImportUnifiedAward)
	api.POST("/award/advisors/lookup", middleware.RequirePermission("reg:user:list"), LookupAwardAdvisors)
	api.PUT("/award/members/:id", middleware.RequirePermission("award:import"), UpdateAwardMember)
	server := httptest.NewServer(router)
	defer server.Close()
	fixture, err := json.Marshal(gin.H{"url": server.URL, "token": token, "competitions": competitions, "college": college.Name})
	require.NoError(t, err)
	frontend, err := filepath.Abs(filepath.Join("..", "..", "CompeManage_frontend", "CompeManage"))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", filepath.Join(frontend, "node_modules", "@playwright", "test", "cli.js"), "test", "--config", "playwright.award.config.js", "unified-import-live.spec.js")
	cmd.Dir = frontend
	cmd.Env = append(os.Environ(), "COMPE_UNIFIED_LIVE_FIXTURE="+string(fixture))
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	t.Log(string(output))
	var awards []models.Award
	require.NoError(t, db.Preload("Members").Preload("Advisors").Order("comp_id").Find(&awards).Error)
	require.Len(t, awards, 2)
	require.Len(t, awards[0].Members, 2)
	require.Len(t, awards[1].Members, 1)
	for _, award := range awards {
		require.Equal(t, "approved", award.Status)
		require.Len(t, award.Advisors, 1)
		require.Equal(t, "T0001", award.Advisors[0].Username)
		for _, member := range award.Members {
			require.Equal(t, admin.ID, *member.SubmittedBy)
		}
	}
	var member models.AwardMember
	require.NoError(t, db.Where("student_id = ?", students[1].ID).First(&member).Error)
	require.Equal(t, "真实数据库中修改备注", member.Remark)
}

func TestUnifiedAwardTemplateAndParser(t *testing.T) {
	f, err := newUnifiedAwardTemplate([]AwardImportCompetition{
		{ID: 7, CompName: "测试竞赛", CompCode: "000007", Year: currentUnifiedAwardYear()},
		{ID: 8, CompName: "往年竞赛", Year: currentUnifiedAwardYear() - 1},
	})
	require.NoError(t, err)
	defer f.Close()
	require.Equal(t, []string{"获奖名单", "可填报赛事目录", "填写说明"}, f.GetSheetList())
	headers, err := f.GetRows("获奖名单")
	require.NoError(t, err)
	require.Equal(t, unifiedAwardHeaders(), headers[0])
	require.NotContains(t, headers[0], "赛事编号")
	require.NotContains(t, headers[0], "赛事年份")
	directory, err := f.GetRows("可填报赛事目录")
	require.NoError(t, err)
	require.Equal(t, [][]string{{"赛事名称"}, {"测试竞赛"}}, directory)
	values := []interface{}{1, "测试竞赛", "000123", "学生甲", "学院甲", "专业甲", "同名项目", "2026-10", "省级", "一等奖", "00101,00102", "备注"}
	require.NoError(t, f.SetSheetRow("获奖名单", "A2", &values))
	// Spreadsheet serialization must preserve leading zeros and original row numbers.
	buffer, err := f.WriteToBuffer()
	require.NoError(t, err)
	loaded, err := excelize.OpenReader(buffer)
	require.NoError(t, err)
	defer loaded.Close()
	// A text number format must not replace the header or table appearance.
	headerID, err := loaded.GetCellStyle("获奖名单", "A1")
	require.NoError(t, err)
	headerStyle, err := loaded.GetStyle(headerID)
	require.NoError(t, err)
	for column, label := range unifiedAwardHeaders() {
		col, err := excelize.ColumnNumberToName(column + 1)
		require.NoError(t, err)
		styleID, err := loaded.GetCellStyle("获奖名单", col+"1")
		require.NoError(t, err)
		style, err := loaded.GetStyle(styleID)
		require.NoError(t, err)
		require.Equal(t, headerStyle, style, "%s header must keep the same font, alignment and borders", label)
		for _, row := range []string{"2", "13"} {
			styleID, err = loaded.GetCellStyle("获奖名单", col+row)
			require.NoError(t, err)
			style, err = loaded.GetStyle(styleID)
			require.NoError(t, err)
			require.Equal(t, headerStyle.Border, style.Border, "%s data borders", label)
			require.Equal(t, "宋体", style.Font.Family, "%s data font", label)
			require.Equal(t, float64(11), style.Font.Size, "%s data font size", label)
			require.False(t, style.Font.Bold, "%s data font weight", label)
			require.Equal(t, "center", style.Alignment.Vertical, "%s vertical alignment", label)
			horizontal := "center"
			if label == "赛事名称" || label == "参赛项目名称" {
				horizontal = "left"
			}
			require.Equal(t, horizontal, style.Alignment.Horizontal, "%s horizontal alignment", label)
		}
	}
	for _, col := range []string{"C", "H", "K"} {
		styleID, err := loaded.GetCellStyle("获奖名单", col+"5001")
		require.NoError(t, err)
		style, err := loaded.GetStyle(styleID)
		require.NoError(t, err)
		require.Equal(t, 49, style.NumFmt, "%s must remain text when adding rows", col)
	}
	sheet, err := loaded.GetRows("获奖名单")
	require.NoError(t, err)
	rows, err := parseUnifiedAwardSheet(sheet)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "000123", rows[0].StudentNumber)
	require.Empty(t, rows[0].CompCode)
	require.Empty(t, rows[0].CompYear)
	require.Equal(t, "00101 00102", rows[0].AdvisorUsernames)
	require.Equal(t, 2, rows[0].RowNumber)
	// Reordered columns work without any year/code fields.
	sheet[0][1], sheet[0][2] = sheet[0][2], sheet[0][1]
	sheet[1][1], sheet[1][2] = sheet[1][2], sheet[1][1]
	rows, err = parseUnifiedAwardSheet(sheet)
	require.NoError(t, err)
	require.Equal(t, "测试竞赛", rows[0].CompName)
	require.Equal(t, "000123", rows[0].StudentNumber)
	require.Empty(t, rows[0].CompCode)
	_, err = parseUnifiedAwardSheet([][]string{awardTemplateHeaders})
	require.ErrorContains(t, err, "统一导入模板")
	_, err = parseUnifiedAwardSheet(nil)
	require.Error(t, err)
	// A row with only a competition must be reported as invalid, not silently lost.
	metadataOnly := make([]string, len(unifiedAwardHeaders()))
	metadataOnly[1] = "测试竞赛"
	rows, err = parseUnifiedAwardSheet([][]string{unifiedAwardHeaders(), metadataOnly})
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestUnifiedAwardRequestAndSummary(t *testing.T) {
	validJSON := []byte(`{"rows":[{"comp_name":"赛事甲","comp_year":"2026","student_id":"0001"}]}`)
	newContext := func(body []byte, contentType string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/api/award/import/unified", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", contentType)
		return c
	}
	parsed, err := readUnifiedAwardImport(newContext(validJSON, "application/json"))
	require.NoError(t, err)
	require.Equal(t, "0001", parsed[0].StudentNumber)
	_, err = readUnifiedAwardImport(newContext([]byte(`{"rows":`), "application/json"))
	require.ErrorContains(t, err, "数据格式错误")
	_, err = readUnifiedAwardImport(newContext(nil, "multipart/form-data"))
	require.ErrorContains(t, err, "文件上传失败")
	upload := func(data []byte) *gin.Context {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		file, err := writer.CreateFormFile("file", "统一获奖名单.xlsx")
		require.NoError(t, err)
		_, err = file.Write(data)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		return newContext(body.Bytes(), writer.FormDataContentType())
	}
	_, err = readUnifiedAwardImport(upload([]byte("not an excel file")))
	require.ErrorContains(t, err, "Excel读取失败")
	f, err := newUnifiedAwardTemplate(nil)
	require.NoError(t, err)
	defer f.Close()
	values := []interface{}{1, "赛事甲", "0001", "学生甲", "学院甲", "专业甲", "项目甲", "2026-10", "省级", "一等奖", "", ""}
	require.NoError(t, f.SetSheetRow("获奖名单", "A2", &values))
	buffer, err := f.WriteToBuffer()
	require.NoError(t, err)
	parsed, err = readUnifiedAwardImport(upload(buffer.Bytes()))
	require.NoError(t, err)
	require.Equal(t, "0001", parsed[0].StudentNumber)
	for _, size := range []int{0, 5001} {
		body, err := json.Marshal(gin.H{"rows": make([]AwardImportRow, size)})
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/api/award/import/unified", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		_, err = readUnifiedAwardImport(c)
		require.ErrorContains(t, err, "1至5000")
	}
	for _, role := range []string{"student", "teacher", "expert"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/award/import/unified", nil)
		c.Set("user_id", uint(1))
		c.Set("role_code", role)
		ImportUnifiedAward(c)
		require.Equal(t, 403, w.Code)
	}
	rows := []AwardImportRow{
		{CompID: 1, CompName: "甲", MatchStatus: "matched"},
		{CompID: 1, CompName: "甲", MatchStatus: "matched", Duplicate: true},
		{CompID: 2, CompName: "乙", MatchStatus: "matched", ImportStatus: "imported"},
		{CompName: "丙", MatchStatus: "unmatched", Error: "不存在"},
	}
	result := unifiedAwardResult(rows, 2)
	require.Equal(t, 1, result["ready_count"])
	require.Equal(t, 1, result["existing_count"])
	require.Equal(t, 1, result["success_count"])
	require.Equal(t, 1, result["fail_count"])
	require.Len(t, result["competitions"], 3)
}

func TestUnifiedAwardImportMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	year := currentUnifiedAwardYear()
	ca, cb := models.College{Name: "统一导入甲院", IsValid: true}, models.College{Name: "统一导入乙院", IsValid: true}
	require.NoError(t, db.Create(&ca).Error)
	require.NoError(t, db.Create(&cb).Error)
	a := models.User{Username: "000101", Realname: "学生甲", College: ca.Name, Major: "专业甲", IdentityType: "student"}
	b := models.User{Username: "000102", Realname: "学生乙", College: cb.Name, Major: "专业乙", IdentityType: "student"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	sequence := 0
	newComp := func(name string, year int, collegeID *uint, level string, manager uint) models.CompDirectory {
		sequence++
		comp := models.CompDirectory{CompCode: fmt.Sprintf("U%04d", sequence), CompName: name, Year: year, CollegeID: collegeID, CompLevel: level, ManagerID: manager}
		require.NoError(t, db.Create(&comp).Error)
		return comp
	}
	row := func(comp models.CompDirectory, user models.User) AwardImportRow {
		return AwardImportRow{CompName: comp.CompName, StudentNumber: user.Username, Name: user.Realname, College: user.College, Major: user.Major, ProjectName: "相同项目名称", Category: "省级", AwardName: "金奖", AwardMonth: "2026-10"}
	}
	type response struct {
		Rows     []AwardImportRow `json:"rows"`
		Success  int              `json:"success_count"`
		Existing int              `json:"existing_count"`
		Failed   int              `json:"fail_count"`
		Ready    int              `json:"ready_count"`
	}
	invoke := func(input []AwardImportRow, commit bool, role string, actor uint, collegeID *uint) (int, response) {
		body, err := json.Marshal(gin.H{"rows": input})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/award/import/unified", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("role_code", role)
		c.Set("user_id", actor)
		c.Set("managed_college_id", collegeID)
		if commit {
			ImportUnifiedAward(c)
		} else {
			PreviewUnifiedAwardImport(c)
		}
		var envelope struct {
			Data response `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope), w.Body.String())
		return w.Code, envelope.Data
	}
	count := func(model interface{}, condition string, args ...interface{}) int64 {
		var n int64
		require.NoError(t, db.Model(model).Where(condition, args...).Count(&n).Error)
		return n
	}
	old := newComp("跨年同名赛事", year-1, &ca.ID, "省级", a.ID)
	current := newComp("跨年同名赛事", year, &ca.ID, "省级", a.ID)
	otherCurrent := newComp("另一个当年赛事", year, &ca.ID, "省级", a.ID)
	t.Run("name-only import targets current year and keeps events independent", func(t *testing.T) {
		input := []AwardImportRow{row(current, a), row(otherCurrent, a)}
		status, preview := invoke(input, false, "school_admin", a.ID, nil)
		require.Equal(t, 200, status)
		require.Equal(t, 2, preview.Ready)
		require.Zero(t, count(&models.Award{}, "1=1"))
		status, result := invoke(preview.Rows, true, "school_admin", a.ID, nil)
		require.Equal(t, 200, status)
		require.Equal(t, 2, result.Success)
		require.EqualValues(t, 2, count(&models.Award{}, "1=1"))
		require.Zero(t, count(&models.Award{}, "comp_id = ?", old.ID))
		status, repeated := invoke(input, true, "school_admin", b.ID, nil)
		require.Equal(t, 200, status)
		require.Zero(t, repeated.Success)
		require.Equal(t, 2, repeated.Existing)
		var members []models.AwardMember
		require.NoError(t, db.Find(&members).Error)
		for _, member := range members {
			require.Equal(t, a.ID, *member.SubmittedBy)
		}
	})
	t.Run("ambiguity and inconsistent code do not affect a valid event", func(t *testing.T) {
		first := newComp("同年同名", year, &ca.ID, "省级", a.ID)
		second := newComp("同年同名", year, &ca.ID, "省级", a.ID)
		valid := newComp("独立赛事", year, &ca.ID, "省级", a.ID)
		badCode := row(current, b)
		badCode.CompID = old.ID
		status, result := invoke([]AwardImportRow{row(first, a), badCode, row(valid, a)}, true, "school_admin", a.ID, nil)
		require.Equal(t, 200, status)
		require.Equal(t, 1, result.Success)
		require.Equal(t, 2, result.Failed)
		require.Len(t, result.Rows[0].Candidates, 2)
		selected := row(second, a)
		selected.CompID = second.ID
		selected.CompCode = second.CompCode
		_, result = invoke([]AwardImportRow{selected}, true, "school_admin", a.ID, nil)
		require.Equal(t, 1, result.Success)
		require.Zero(t, count(&models.Award{}, "comp_id = ?", first.ID))
	})
	t.Run("scope filters candidates and the downloaded directory", func(t *testing.T) {
		own := newComp("权限同名", year, &ca.ID, "省级", a.ID)
		hidden := newComp("权限同名", year, &cb.ID, "省级", b.ID)
		school := newComp("校级可填报", year, &cb.ID, "校级", b.ID)
		unassigned := newComp("未绑定学院", year, nil, "省级", b.ID)
		blocked := row(hidden, b)
		blocked.CompCode = hidden.CompCode
		blocked.CompID = hidden.ID
		_, result := invoke([]AwardImportRow{row(own, b), blocked, row(school, b), row(unassigned, a)}, true, "college_admin", a.ID, &ca.ID)
		require.Equal(t, 1, result.Success)
		require.Equal(t, 3, result.Failed)
		require.Contains(t, result.Rows[0].Error, "只能填报本院学生")
		require.NotEmpty(t, result.Rows[1].Error)
		require.Contains(t, result.Rows[2].Error, "只能填报本院学生")
		require.Empty(t, result.Rows[3].Error)
		require.Equal(t, own.ID, result.Rows[0].CompID)
		require.Empty(t, result.Rows[1].Candidates)
		_, manager := invoke([]AwardImportRow{row(own, a), row(school, a)}, false, "competition_manager", a.ID, nil)
		require.Equal(t, 1, manager.Ready)
		require.Equal(t, 1, manager.Failed)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/award/import/unified/template", nil)
		c.Set("role_code", "college_admin")
		c.Set("user_id", a.ID)
		c.Set("managed_college_id", &ca.ID)
		ExportUnifiedAwardTemplate(c)
		require.Equal(t, 200, w.Code)
		f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
		require.NoError(t, err)
		defer f.Close()
		directory, err := f.GetRows("可填报赛事目录")
		require.NoError(t, err)
		var sameNameCount int
		for _, line := range directory[1:] {
			if line[0] == hidden.CompName {
				sameNameCount++
			}
		}
		require.Equal(t, 1, sameNameCount, "only the accessible same-name competition belongs in the directory")
	})
	t.Run("partial members, repair and per-event student uniqueness", func(t *testing.T) {
		comp := newComp("部分成员", year, &ca.ID, "省级", a.ID)
		bad := row(comp, b)
		bad.Name = "错误姓名"
		_, result := invoke([]AwardImportRow{row(comp, a), bad}, true, "school_admin", a.ID, nil)
		require.Equal(t, 1, result.Success)
		require.Equal(t, 1, result.Failed)
		_, result = invoke([]AwardImportRow{row(comp, a), row(comp, b)}, true, "school_admin", a.ID, nil)
		require.Equal(t, 1, result.Success)
		require.Equal(t, 1, result.Existing)
		other := row(comp, a)
		other.ProjectName = "另一个团队"
		_, result = invoke([]AwardImportRow{other}, true, "school_admin", a.ID, nil)
		require.Equal(t, 1, result.Failed)
		conflict := newComp("共享信息冲突", year, &ca.ID, "省级", a.ID)
		bad = row(conflict, b)
		bad.AwardName = "二等奖"
		_, result = invoke([]AwardImportRow{row(conflict, a), bad}, true, "school_admin", a.ID, nil)
		require.Equal(t, 2, result.Failed)
		require.Zero(t, count(&models.Award{}, "comp_id = ?", conflict.ID))
	})
	t.Run("a bound preview cannot move to a replacement directory entry", func(t *testing.T) {
		comp := newComp("已删除赛事", year, &ca.ID, "省级", a.ID)
		_, preview := invoke([]AwardImportRow{row(comp, a)}, false, "school_admin", a.ID, nil)
		require.NoError(t, db.Delete(&comp).Error)
		replacement := newComp(comp.CompName, year, &ca.ID, "省级", a.ID)
		_, result := invoke(preview.Rows, true, "school_admin", a.ID, nil)
		require.Equal(t, 1, result.Failed)
		require.Zero(t, count(&models.Award{}, "comp_id = ?", replacement.ID))
	})
	t.Run("a write failure rolls back every competition", func(t *testing.T) {
		one := newComp("回滚甲", year, &ca.ID, "省级", a.ID)
		two := newComp("回滚乙", year, &ca.ID, "省级", a.ID)
		writes := 0
		require.NoError(t, db.Callback().Create().Before("gorm:create").Register("unified_test_failure", func(tx *gorm.DB) {
			if tx.Statement.Table == "award_members" {
				writes++
				if writes == 2 {
					_ = tx.AddError(errors.New("injected member write failure"))
				}
			}
		}))
		defer db.Callback().Create().Remove("unified_test_failure")
		status, _ := invoke([]AwardImportRow{row(one, a), row(two, a)}, true, "school_admin", a.ID, nil)
		require.Equal(t, 500, status)
		require.Equal(t, 2, writes)
		require.Zero(t, count(&models.Award{}, "comp_id IN ?", []uint{one.ID, two.ID}))
	})
	t.Run("waiting for a concurrent import must not validate an old snapshot", func(t *testing.T) {
		comp := newComp("并发获奖唯一性", year, &ca.ID, "省级", a.ID)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(2)
		defer sqlDB.SetMaxOpenConns(1)
		writer := db.Begin()
		require.NoError(t, writer.Error)
		defer writer.Rollback()
		// Simulate the same mutex used by single-event imports and member edits.
		require.NoError(t, writer.Exec("SELECT id FROM comp_directories WHERE id = ? FOR UPDATE", comp.ID).Error)
		directoryRead := make(chan struct{})
		var once sync.Once
		require.NoError(t, db.Callback().Query().After("gorm:query").Register("unified_directory_barrier", func(query *gorm.DB) {
			if query.Statement.Table == "comp_directories" {
				if _, locking := query.Statement.Clauses["FOR"]; !locking {
					once.Do(func() { close(directoryRead) })
				}
			}
		}))
		defer db.Callback().Query().Remove("unified_directory_barrier")
		type concurrentResult struct {
			status int
			result response
		}
		done := make(chan concurrentResult, 1)
		later := row(comp, a)
		later.ProjectName = "后提交项目"
		go func() {
			status, result := invoke([]AwardImportRow{later}, true, "school_admin", a.ID, nil)
			done <- concurrentResult{status, result}
		}()
		select {
		case <-directoryRead:
		case <-time.After(10 * time.Second):
			t.Fatal("directory lookup did not reach the concurrency barrier")
		}
		first := row(comp, a)
		first.ProjectName = "先提交项目"
		validated, bad, err := validateAwardRows(writer, comp.ID, []AwardImportRow{first}, false)
		require.NoError(t, err)
		require.Zero(t, bad)
		_, _, _, err = writeAwardImportRows(writer, &UserAccessScope{RoleCode: "school_admin", UserID: a.ID}, comp.ID, validated)
		require.NoError(t, err)
		require.NoError(t, writer.Commit().Error)
		select {
		case got := <-done:
			require.Equal(t, 200, got.status)
			require.Zero(t, got.result.Success, "a stale snapshot must not allow a second award for this student")
			require.Equal(t, 1, got.result.Failed)
			require.Contains(t, got.result.Rows[0].Error, "已有其他获奖记录")
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent import did not finish after the competition lock was released")
		}
		require.EqualValues(t, 1, count(&models.Award{}, "comp_id = ?", comp.ID))
	})
	t.Run("global registration requirement uses the target event", func(t *testing.T) {
		one := newComp("报名甲", year, &ca.ID, "省级", a.ID)
		two := newComp("报名乙", year, &ca.ID, "省级", a.ID)
		require.NoError(t, db.Save(&models.AwardImportSetting{ID: 1, RequireRegistration: true}).Error)
		require.NoError(t, db.Create(&models.Register{CompID: one.ID, LeaderID: a.ID, Status: 1, AdvisorInfo: "[]"}).Error)
		_, result := invoke([]AwardImportRow{row(one, a), row(two, a)}, true, "school_admin", a.ID, nil)
		require.Equal(t, 1, result.Success)
		require.Equal(t, 1, result.Failed)
		require.Contains(t, result.Rows[1].Error, "报名")
	})
}
