package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProjectSourceManagementMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	source := models.College{Name: "项目来源学院", IsValid: true}
	host := models.College{Name: "所属单位学院", IsValid: true}
	require.NoError(t, db.Create(&source).Error)
	require.NoError(t, db.Create(&host).Error)
	manager := models.User{Username: "source-manager", College: host.Name, IdentityType: "staff"}
	student := models.User{Username: "host-student", Realname: "跨院学生", College: host.Name, IdentityType: "student"}
	supplementStudent := models.User{Username: "supplement-student", Realname: "补录学生", College: host.Name, IdentityType: "student"}
	admin := models.User{Username: "source-admin", College: source.Name, IdentityType: "staff"}
	for _, user := range []*models.User{&manager, &student, &supplementStudent, &admin} {
		require.NoError(t, db.Create(user).Error)
	}
	origin := models.ProjectOrigin{ProjectSource: source.Name, ProjectSourceCollegeID: &source.ID}
	comp := models.CompDirectory{CompCode: "PROJECT-SOURCE", CompName: "跨学院赛事", CollegeID: &host.ID, ManagerID: manager.ID, Year: 2026, CompLevel: "国家级", ProjectOrigin: origin}
	require.NoError(t, db.Create(&comp).Error)
	now := time.Now()
	require.NoError(t, db.Create(&models.CompDetail{
		CompID: comp.ID, NeedRegAudit: 1, AwardHierarchy: "[]", Track: "[]",
		CompStartTime: now, CompEndTime: now, RegStartTime: now, RegEndTime: now,
		SubmitStartTime: now, SubmitEndTime: now, ReviewStartTime: now, ReviewEndTime: now,
	}).Error)
	reg := models.Register{CompID: comp.ID, LeaderID: student.ID, TeamName: "普通报名项目", AdvisorInfo: "[]"}
	require.NoError(t, db.Create(&reg).Error)
	supplementReg := models.Register{CompID: comp.ID, LeaderID: supplementStudent.ID, TeamName: "获奖补录关联项目", Status: 3, SupplementTime: &now, AdvisorInfo: "[]"}
	require.NoError(t, db.Create(&supplementReg).Error)
	award := models.Award{CompID: comp.ID, RegID: supplementReg.ID, ProjectName: "跨院获奖项目", Status: "draft", Source: "supplement", AdvisorInfo: "[]"}
	require.NoError(t, db.Create(&award).Error)
	require.NoError(t, db.Create(&models.AwardMember{
		AwardID: award.ID, StudentID: supplementStudent.ID, Name: supplementStudent.Realname, StudentNumber: supplementStudent.Username, College: host.Name, IsLeader: true,
	}).Error)
	declaration := models.CompDeclaration{
		CompName: "跨学院申报", CompLevel: "省级", Year: 2026, ManagerID: manager.ID, CollegeID: host.ID,
		CreatedBy: admin.ID, DeclareStatus: 1, ProjectOrigin: origin,
	}
	require.NoError(t, db.Create(&declaration).Error)

	invoke := func(collegeID uint, method, url string, id uint, body any, handler gin.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, url, bytes.NewReader(data))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user_id", admin.ID)
		c.Set("role_code", "college_admin")
		c.Set("managed_college_id", &collegeID)
		c.Set("permission_codes", []string{"comp:update:source", "reg:config:edit:source"})
		if id != 0 {
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(id)}}
		}
		handler(c)
		return w
	}

	for _, tc := range []struct {
		name, url, visible string
		handler            gin.HandlerFunc
	}{
		{"赛事目录", "/?year=2026&page=1&page_size=10", comp.CompName, GetCompetitionList},
		{"报名列表", "/", comp.CompName, GetRegList},
		{"获奖审核", "/", supplementStudent.Realname, GetAwardAuditList},
		{"待审核申报", "/?page=1&page_size=10", declaration.CompName, GetPendingDeclares},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := invoke(source.ID, "GET", tc.url, 0, nil, tc.handler)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tc.visible)
			w = invoke(host.ID, "GET", tc.url, 0, nil, tc.handler)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.NotContains(t, w.Body.String(), tc.visible)
		})
	}

	t.Run("报名审核排除获奖补录记录", func(t *testing.T) {
		w := invoke(source.ID, "GET", "/", 0, nil, GetRegList)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), reg.TeamName)
		require.NotContains(t, w.Body.String(), supplementReg.TeamName)
		require.Contains(t, w.Body.String(), `"total":1`)
		w = invoke(source.ID, "PUT", "/", 0, gin.H{"id": supplementReg.ID, "status": 1}, AuditRegister)
		require.Equal(t, 400, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "获奖补录请在填报审核中处理")
	})

	w := invoke(source.ID, "GET", "/", comp.ID, nil, GetCompetitionDetail)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"can_edit_competition":true`)
	require.Contains(t, w.Body.String(), `"can_edit_registration":true`)
	update := UpdateCompetitionReq{CompName: "来源学院已编辑", CompLevel: comp.CompLevel, ManagerID: manager.ID, College: host.Name, Year: "2026"}
	w = invoke(host.ID, "PUT", "/", comp.ID, update, UpdateCompetition)
	require.Equal(t, 403, w.Code, w.Body.String())
	w = invoke(source.ID, "PUT", "/", comp.ID, update, UpdateCompetition)
	require.Equal(t, 200, w.Code, w.Body.String())

	w = invoke(host.ID, "GET", fmt.Sprintf("/?id=%d", reg.ID), 0, nil, GetRegDetail)
	require.Equal(t, 403, w.Code, w.Body.String())
	w = invoke(source.ID, "GET", fmt.Sprintf("/?id=%d", reg.ID), 0, nil, GetRegDetail)
	require.Equal(t, 200, w.Code, w.Body.String())
	regAudit := gin.H{"id": reg.ID, "status": 1}
	w = invoke(host.ID, "PUT", "/", 0, regAudit, AuditRegister)
	require.Equal(t, 403, w.Code, w.Body.String())
	w = invoke(source.ID, "PUT", "/", 0, regAudit, AuditRegister)
	require.Equal(t, 200, w.Code, w.Body.String())

	w = invoke(host.ID, "GET", "/", award.ID, nil, GetAwardAuditDetail)
	require.Equal(t, 403, w.Code, w.Body.String())
	w = invoke(source.ID, "GET", "/", award.ID, nil, GetAwardAuditDetail)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), supplementStudent.Realname)
	awardAudit := gin.H{"ids": []uint{award.ID}}
	w = invoke(host.ID, "PUT", "/", 0, awardAudit, BatchPassAwardAudit)
	require.Equal(t, 403, w.Code, w.Body.String())
	w = invoke(source.ID, "PUT", "/", 0, awardAudit, BatchPassAwardAudit)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, db.First(&supplementReg, supplementReg.ID).Error)
	require.Equal(t, int8(4), supplementReg.Status)

	declareAudit := DeclareAuditReq{DeclareID: declaration.ID, AuditStatus: 2}
	w = invoke(host.ID, "POST", "/", 0, declareAudit, AuditDeclare)
	require.Equal(t, 403, w.Code, w.Body.String())
	w = invoke(source.ID, "POST", "/", 0, declareAudit, AuditDeclare)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, db.First(&declaration, declaration.ID).Error)
	require.NotNil(t, declaration.CompDirectoryID)
	var approved models.CompDirectory
	require.NoError(t, db.First(&approved, *declaration.CompDirectoryID).Error)
	require.Equal(t, origin, approved.ProjectOrigin)
	require.Equal(t, &host.ID, approved.CollegeID)

	require.NoError(t, db.Model(&manager).Update("college", "其他部门").Error)
	w = invoke(source.ID, "GET", "/", comp.ID, nil, GetCompetitionDetail)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "其他部门")
	require.Contains(t, w.Body.String(), `"can_edit_competition":true`)
	w = invoke(host.ID, "GET", "/", comp.ID, nil, GetCompetitionDetail)
	require.Equal(t, 403, w.Code, w.Body.String())
}

func TestCompetitionRecordingUnitMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	host := models.College{Name: "计算机学院", IsValid: true}
	source := models.College{Name: "数学学院", IsValid: true}
	require.NoError(t, db.Create(&host).Error)
	require.NoError(t, db.Create(&source).Error)
	school := models.User{Username: "recording-school", College: "科研处", IdentityType: "staff"}
	college := models.User{Username: "recording-college", College: source.Name, IdentityType: "staff"}
	manager := models.User{Username: "recording-manager", College: host.Name, IdentityType: "staff"}
	nextManager := models.User{Username: "next-manager", College: "网络中心", IdentityType: "staff"}
	missingUnit := models.User{Username: "empty-unit", IdentityType: "staff"}
	role := models.Role{RoleCode: "competition_manager", RoleName: "赛事负责人"}
	require.NoError(t, db.Create(&role).Error)
	for _, user := range []*models.User{&school, &college, &manager, &nextManager, &missingUnit} {
		require.NoError(t, db.Create(user).Error)
	}
	require.NoError(t, db.Create(&[]models.UserRole{{UserID: manager.ID, RoleID: role.ID}, {UserID: nextManager.ID, RoleID: role.ID}}).Error)

	invoke := func(actor models.User, role, method string, payload any, handler gin.HandlerFunc, params ...gin.Param) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(payload)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, "/", bytes.NewReader(data))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user_id", actor.ID)
		c.Set("role_code", role)
		c.Set("managed_college_id", &source.ID)
		c.Set("permission_codes", []string{"comp:update"})
		c.Params = params
		handler(c)
		return w
	}
	request := CreateCompetitionReq{CompName: "校级直录赛事", CompLevel: "省级", ManagerID: manager.ID, College: "不采用的旧单位", Year: "2026"}
	w := invoke(school, "school_admin", "POST", request, CreateCompetition)
	require.Equal(t, 200, w.Code, w.Body.String())
	request.CompName = "批量录入赛事"
	w = invoke(college, "college_admin", "POST", gin.H{"items": []CreateCompetitionReq{request}}, BatchImportCompetition)
	require.Equal(t, 200, w.Code, w.Body.String())
	for _, expected := range []struct {
		name   string
		actor  models.User
		origin *uint
	}{{"校级直录赛事", school, nil}, {"批量录入赛事", college, &source.ID}} {
		var comp models.CompDirectory
		require.NoError(t, db.Where("comp_name = ?", expected.name).First(&comp).Error)
		require.Equal(t, expected.actor.ID, comp.CreatedBy)
		require.Equal(t, expected.actor.College, comp.ProjectSource)
		require.Equal(t, expected.origin, comp.ProjectSourceCollegeID)
		require.Equal(t, &host.ID, comp.CollegeID)
	}
	var reassigned models.CompDirectory
	require.NoError(t, db.Where("comp_name = ?", request.CompName).First(&reassigned).Error)
	update := UpdateCompetitionReq{CompName: request.CompName, CompLevel: request.CompLevel, ManagerID: nextManager.ID, College: host.Name, Year: request.Year}
	w = invoke(school, "school_admin", "PUT", update, UpdateCompetition, gin.Param{Key: "id", Value: fmt.Sprint(reassigned.ID)})
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, db.Preload("Manager").First(&reassigned, reassigned.ID).Error)
	require.Equal(t, nextManager.College, reassigned.Manager.College)
	require.Nil(t, reassigned.CollegeID)
	require.Equal(t, college.College, reassigned.ProjectSource)
	require.Equal(t, &source.ID, reassigned.ProjectSourceCollegeID)

	declareRequest := DeclareCreateReq{
		CompName: "跨管理范围申报", CompLevel: "省级", CompType: "学科竞赛",
		Organizer: "主办方", Undertaker: "承办方", CollegeID: source.ID, ManagerID: manager.ID, Year: 2026,
	}
	w = invoke(manager, "college_admin", "POST", declareRequest, CreateDeclare)
	require.Equal(t, 403, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "账号所属单位需与管理学院一致")
	var draftCount int64
	require.NoError(t, db.Model(&models.CompDeclaration{}).Where("comp_name = ?", declareRequest.CompName).Count(&draftCount).Error)
	require.Zero(t, draftCount)

	unknown := models.CompDirectory{CompCode: "OLD-UNKNOWN", CompName: "未知来源", CollegeID: &host.ID, ManagerID: manager.ID}
	known := models.CompDirectory{CompCode: "OLD-KNOWN", CompName: "可追溯来源", CreatedBy: school.ID, ManagerID: manager.ID}
	missingDeclarerUnit := models.CompDeclaration{
		CompName: "申报人单位缺失", CollegeID: host.ID, ManagerID: manager.ID, CreatedBy: missingUnit.ID,
	}
	knownDeclarerUnit := models.CompDeclaration{
		CompName: "申报人单位可追溯", CollegeID: host.ID, ManagerID: manager.ID, CreatedBy: college.ID,
	}
	for _, legacy := range []*models.CompDeclaration{&missingDeclarerUnit, &knownDeclarerUnit} {
		require.NoError(t, db.Create(legacy).Error)
		require.NoError(t, db.Model(legacy).UpdateColumn("project_source", nil).Error)
	}
	unknownApproved := models.CompDirectory{
		CompCode: "OLD-UNKNOWN-APPROVED", CompName: missingDeclarerUnit.CompName, ManagerID: manager.ID, Source: 2, DeclareID: &missingDeclarerUnit.ID, CreatedBy: school.ID,
	}
	knownApproved := models.CompDirectory{
		CompCode: "OLD-KNOWN-APPROVED", CompName: knownDeclarerUnit.CompName, ManagerID: manager.ID, Source: 2, DeclareID: &knownDeclarerUnit.ID, CreatedBy: school.ID,
	}
	for _, legacy := range []*models.CompDirectory{&unknown, &known, &unknownApproved, &knownApproved} {
		require.NoError(t, db.Create(legacy).Error)
		require.NoError(t, db.Model(legacy).UpdateColumn("project_source", nil).Error)
	}
	empty := models.CompDirectory{CompCode: "RECORDED-EMPTY", CompName: "已记录空来源", CreatedBy: school.ID, ManagerID: manager.ID}
	require.NoError(t, db.Create(&empty).Error)
	require.NoError(t, database.BackfillCompetitionProjectSources(db))
	require.NoError(t, db.First(&unknown, unknown.ID).Error)
	require.Empty(t, unknown.ProjectSource)
	require.NoError(t, db.First(&known, known.ID).Error)
	require.Equal(t, school.College, known.ProjectSource)
	require.NoError(t, db.First(&empty, empty.ID).Error)
	require.Empty(t, empty.ProjectSource)
	require.NoError(t, db.First(&unknownApproved, unknownApproved.ID).Error)
	require.Empty(t, unknownApproved.ProjectSource)
	require.Nil(t, unknownApproved.ProjectSourceCollegeID)
	require.NoError(t, db.First(&knownApproved, knownApproved.ID).Error)
	require.Equal(t, source.Name, knownApproved.ProjectSource)
	require.Equal(t, &source.ID, knownApproved.ProjectSourceCollegeID)
}
