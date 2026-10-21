package controllers

import (
	"CompeManage_backend/models"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Handler 级 MySQL 集成：覆盖此前接近 0% 的 Preview/Import/补录/审核列表/详情/导出/指导获奖。
// 运行：COMPE_AWARD_MYSQL_TEST=1 go test ./controllers/ -run 'TestAwardHandlersMySQL' -v
func TestAwardHandlersMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)

	ca := models.College{Name: "集成甲学院", IsValid: true}
	cb := models.College{Name: "集成乙学院", IsValid: true}
	require.NoError(t, db.Create(&ca).Error)
	require.NoError(t, db.Create(&cb).Error)

	student := models.User{Username: "I001", Realname: "补录学生", College: ca.Name, Major: "专业甲", IdentityType: "student"}
	require.NoError(t, db.Create(&student).Error)
	studentB := models.User{Username: "I002", Realname: "乙学生", College: ca.Name, Major: "专业甲", IdentityType: "student"}
	require.NoError(t, db.Create(&studentB).Error)
	teacher := models.User{Username: "IT001", Realname: "集成教师", IdentityType: "staff", College: ca.Name}
	require.NoError(t, db.Create(&teacher).Error)
	studentC := models.User{Username: "I003", Realname: "补录学生C", College: ca.Name, Major: "专业甲", IdentityType: "student"}
	require.NoError(t, db.Create(&studentC).Error)
	schoolAdmin := models.User{Username: "IS001", Realname: "集成校管", IdentityType: "staff", College: "教务处"}
	require.NoError(t, db.Create(&schoolAdmin).Error)

	schoolComp := models.CompDirectory{CompCode: "IX1", CompName: "集成校赛", CompLevel: "校级", ManagerID: teacher.ID, CollegeID: nil, Year: 2026}
	ownComp := models.CompDirectory{CompCode: "IX2", CompName: "集成甲省赛", CompLevel: "省级", ManagerID: teacher.ID, CollegeID: &ca.ID, Year: 2026}
	otherComp := models.CompDirectory{CompCode: "IX3", CompName: "集成乙省赛", CompLevel: "省级", ManagerID: teacher.ID, CollegeID: &cb.ID, Year: 2026}
	for _, c := range []*models.CompDirectory{&schoolComp, &ownComp, &otherComp} {
		require.NoError(t, db.Create(c).Error)
	}
	now := time.Now()
	require.NoError(t, db.Create(&models.CompDetail{
		CompID: schoolComp.ID, ParticipantType: 2,
		CompStartTime: now, CompEndTime: now.Add(time.Hour),
		RegStartTime: now.Add(-time.Hour), RegEndTime: now.Add(time.Hour),
		SubmitStartTime: now.Add(-time.Hour), SubmitEndTime: now.Add(time.Hour),
		ReviewStartTime: now.Add(-time.Hour), ReviewEndTime: now.Add(time.Hour),
		Track: "[]", AwardHierarchy: "[]",
		MinTeamMember: 1, MaxTeamMember: 5,
	}).Error)

	invoke := func(method, path, role string, collegeID *uint, userID uint, body interface{}, handler gin.HandlerFunc, params ...gin.Param) *httptest.ResponseRecorder {
		var reader *bytes.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			reader = bytes.NewReader(data)
		} else {
			reader = bytes.NewReader(nil)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, path, reader)
		c.Request.Header.Set("Content-Type", "application/json")
		if body == nil {
			c.Request = httptest.NewRequest(method, path, nil)
		}
		c.Params = params
		c.Set("role_code", role)
		c.Set("user_id", userID)
		c.Set("managed_college_id", collegeID)
		handler(c)
		return w
	}

	row := func(sid, name, project, level string) AwardImportRow {
		return AwardImportRow{
			StudentNumber: sid, Name: name, College: ca.Name, Major: "专业甲",
			ProjectName: project, AwardMonth: "2026-06", Category: "省级", AwardName: level,
		}
	}

	t.Run("PreviewAwardImport success and validation", func(t *testing.T) {
		w := invoke("POST", fmt.Sprintf("/api/award/import/preview?comp_id=%d", schoolComp.ID), "college_admin", &ca.ID, teacher.ID,
			map[string]interface{}{"rows": []AwardImportRow{row("I001", "补录学生", "预览项目", "金奖")}}, PreviewAwardImport)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"award_name":"一等奖"`)
		require.Contains(t, w.Body.String(), `"error_count":0`)

		w = invoke("POST", fmt.Sprintf("/api/award/import/preview?comp_id=%d", schoolComp.ID), "college_admin", &ca.ID, teacher.ID,
			map[string]interface{}{"rows": []AwardImportRow{row("", "补录学生", "空学号", "一等奖")}}, PreviewAwardImport)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "学号")

		// 跨院：甲院管预览乙院省赛 → 403
		w = invoke("POST", fmt.Sprintf("/api/award/import/preview?comp_id=%d", otherComp.ID), "college_admin", &ca.ID, teacher.ID,
			map[string]interface{}{"rows": []AwardImportRow{row("I001", "补录学生", "越权", "一等奖")}}, PreviewAwardImport)
		require.Equal(t, 403, w.Code)
	})

	t.Run("ImportAward then GetCompAwards", func(t *testing.T) {
		w := invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", schoolComp.ID), "college_admin", &ca.ID, teacher.ID,
			map[string]interface{}{"rows": []AwardImportRow{
				row("I001", "补录学生", "导入团队", "金奖"),
				row("I002", "乙学生", "导入团队", "金奖"),
			}}, ImportAward)
		require.Equal(t, 200, w.Code, w.Body.String())

		var awards []models.Award
		require.NoError(t, db.Where("comp_id = ?", schoolComp.ID).Find(&awards).Error)
		require.Len(t, awards, 1)
		awardID := awards[0].ID

		w = invoke("GET", fmt.Sprintf("/api/award/comp-awards?comp_id=%d", schoolComp.ID), "school_admin", nil, schoolAdmin.ID, nil, GetCompAwards)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "导入团队")
		require.Contains(t, w.Body.String(), `"student_count":2`)

		// 院管看乙院省赛 403
		w = invoke("GET", fmt.Sprintf("/api/award/comp-awards?comp_id=%d", otherComp.ID), "college_admin", &ca.ID, teacher.ID, nil, GetCompAwards)
		require.Equal(t, 403, w.Code)

		// 校管看乙院空列表仍 200
		w = invoke("GET", fmt.Sprintf("/api/award/comp-awards?comp_id=%d", otherComp.ID), "school_admin", nil, schoolAdmin.ID, nil, GetCompAwards)
		require.Equal(t, 200, w.Code)

		// 非法 comp_id
		w = invoke("GET", "/api/award/comp-awards?comp_id=abc", "school_admin", nil, schoolAdmin.ID, nil, GetCompAwards)
		require.Equal(t, 400, w.Code)

		_ = awardID
	})

	t.Run("GetAwardAuditList filters and college scope", func(t *testing.T) {
		// 证明文件记录（补录 proof_url 必须可校验）
		proofHash := "a1b2c3d4e5f6789012345678abcdef01"
		proofName := "proof.png"
		proofURL := buildDownloadURL("award_proof", proofHash, proofName)
		require.NoError(t, db.Create(&models.FileRecord{
			FileHash: proofHash, OriginalName: proofName,
			StoragePath: "award_proof/" + proofHash + "_" + proofName,
			FileSize: 10, FileExt: "png", BizType: "award_proof", UploaderID: studentC.ID,
		}).Error)

		// 学生补录
		supplement := map[string]interface{}{
			"comp_id":           schoolComp.ID,
			"team_name":         "补录项目X",
			"award_level":       "省级一等奖",
			"award_name":        "一等奖",
			"award_date":        "2026-05-10",
			"proof_url":         proofURL,
			"participation_type": "individual",
			"award_category":    "省级",
			"advisors":          []map[string]string{{"username": "IT001", "name": "集成教师", "college": ca.Name}},
			"members": []map[string]interface{}{{
				"name": "补录学生C", "student_id": "I003", "phone": "13800000000",
				"college": ca.Name, "is_leader": true,
			}},
		}
		w := invoke("POST", "/api/award/student/supplement", "student", nil, studentC.ID, supplement, SubmitStudentAwardSupplement)
		require.Equal(t, 200, w.Code, w.Body.String())

		w = invoke("GET", "/api/award/audit/list?source=supplement&page=1&page_size=20", "school_admin", nil, schoolAdmin.ID, nil, GetAwardAuditList)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "补录学生C")
		require.Contains(t, w.Body.String(), "省级一等奖")

		// 院管在学院范围下也可见校级补录
		w = invoke("GET", "/api/award/audit/list?source=supplement", "college_admin", &ca.ID, teacher.ID, nil, GetAwardAuditList)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "补录学生C")

		// 无效 status 字符串走 0 映射，仍 200
		w = invoke("GET", "/api/award/audit/list?status=99&source=all", "school_admin", nil, schoolAdmin.ID, nil, GetAwardAuditList)
		require.Equal(t, 200, w.Code)
	})

	t.Run("SubmitStudentAwardSupplement edge cases", func(t *testing.T) {
		// 未登录
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/award/student/supplement", strings.NewReader("{}"))
		c.Request.Header.Set("Content-Type", "application/json")
		SubmitStudentAwardSupplement(c)
		require.Equal(t, 401, w.Code)

		// 缺参数
		w = invoke("POST", "/api/award/student/supplement", "student", nil, student.ID, map[string]interface{}{}, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code)

		// 无队长
		base := map[string]interface{}{
			"comp_id": schoolComp.ID, "team_name": "无队长项目", "award_level": "省级一等奖",
			"award_name": "一等奖", "proof_url": "/p.png",
			"members": []map[string]interface{}{{"name": "甲", "student_id": "I001", "phone": "1", "college": ca.Name, "is_leader": false}},
			"advisors": []map[string]string{{"username": "IT001"}},
		}
		w = invoke("POST", "/api/award/student/supplement", "student", nil, student.ID, base, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code)

		// 无指导教师
		base["team_name"] = "无教师项目"
		base["members"] = []map[string]interface{}{{"name": "甲", "student_id": "I001", "phone": "1", "college": ca.Name, "is_leader": true}}
		base["advisors"] = []map[string]string{}
		w = invoke("POST", "/api/award/student/supplement", "student", nil, student.ID, base, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code)

		// 赛事不存在
		base["comp_id"] = 999999
		base["advisors"] = []map[string]string{{"username": "IT001"}}
		w = invoke("POST", "/api/award/student/supplement", "student", nil, student.ID, base, SubmitStudentAwardSupplement)
		require.Equal(t, 404, w.Code)
	})

	t.Run("ExportAwardReport three sheets", func(t *testing.T) {
		w := invoke("GET", "/api/award/export/report?year=2026", "school_admin", nil, schoolAdmin.ID, nil, ExportAwardReport)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Header().Get("Content-Type"), "spreadsheet")
		require.Greater(t, w.Body.Len(), 1000)

		// 非法年份
		w = invoke("GET", "/api/award/export/report?year=abc", "school_admin", nil, schoolAdmin.ID, nil, ExportAwardReport)
		require.Equal(t, 400, w.Code)

		// 院管导出（范围过滤路径）
		w = invoke("GET", "/api/award/export/projects?year=2026", "college_admin", &ca.ID, teacher.ID, nil, ExportAwardProjects)
		require.Equal(t, 200, w.Code)
		require.Greater(t, w.Body.Len(), 500)

		// 负责人无所属学院 → 403
		teacherNoCollege := models.User{Username: "IT999", Realname: "无学院教师", IdentityType: "staff", College: ""}
		require.NoError(t, db.Create(&teacherNoCollege).Error)
		w = invoke("GET", "/api/award/export/projects", "competition_manager", nil, teacherNoCollege.ID, nil, ExportAwardProjects)
		require.Equal(t, 403, w.Code)
	})

	t.Run("GetAdvisorAwards and GetAwardChanges", func(t *testing.T) {
		// 把教师挂到导入奖上
		var imported models.Award
		require.NoError(t, db.Where("project_name = ?", "导入团队").First(&imported).Error)
		require.NoError(t, db.Create(&models.AwardAdvisor{AwardID: imported.ID, Username: teacher.Username, Name: teacher.Realname, College: ca.Name, Sort: 0}).Error)
		require.NoError(t, db.Model(&imported).Update("status", "draft").Error)

		// 学生角色无指导权限
		w := invoke("GET", "/api/award/advisor/list", "student", nil, student.ID, nil, GetAdvisorAwards)
		require.Equal(t, 403, w.Code)

		w = invoke("GET", "/api/award/advisor/list?page=1&size=10", "teacher", nil, teacher.ID, nil, GetAdvisorAwards)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "导入团队")

		// 非法 status
		w = invoke("GET", "/api/award/advisor/list?status=bogus", "teacher", nil, teacher.ID, nil, GetAdvisorAwards)
		require.Equal(t, 400, w.Code)

		// keyword 过滤
		w = invoke("GET", "/api/award/advisor/list?keyword=导入团队", "teacher", nil, teacher.ID, nil, GetAdvisorAwards)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "导入团队")

		// 详情
		w = invoke("GET", fmt.Sprintf("/api/award/advisor/%d", imported.ID), "teacher", nil, teacher.ID, nil, GetAdvisorAwardDetail, gin.Param{Key: "id", Value: fmt.Sprint(imported.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "导入团队")

		// 无效 id
		w = invoke("GET", "/api/award/advisor/0", "teacher", nil, teacher.ID, nil, GetAdvisorAwardDetail, gin.Param{Key: "id", Value: "0"})
		require.Equal(t, 400, w.Code)

		// 变更记录
		w = invoke("GET", fmt.Sprintf("/api/award/%d/changes", imported.ID), "school_admin", nil, schoolAdmin.ID, nil, GetAwardChanges, gin.Param{Key: "id", Value: fmt.Sprint(imported.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"list"`)

		// 非法 award id
		w = invoke("GET", "/api/award/abc/changes", "school_admin", nil, schoolAdmin.ID, nil, GetAwardChanges, gin.Param{Key: "id", Value: "abc"})
		require.Equal(t, 400, w.Code)
	})
}

// 审核状态机 + 批量通过/驳回 + 详情（冲 BatchReject / Pass / Reject 覆盖）
func TestAwardAuditStateMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	ca := models.College{Name: "审核甲学院", IsValid: true}
	require.NoError(t, db.Create(&ca).Error)

	schoolAdmin := models.User{Username: "AS001", Realname: "审核校管", IdentityType: "staff", College: "教务处"}
	require.NoError(t, db.Create(&schoolAdmin).Error)
	auditor2 := models.User{Username: "AS002", Realname: "审核校管B", IdentityType: "staff", College: "教务处"}
	require.NoError(t, db.Create(&auditor2).Error)
	teacher := models.User{Username: "AT001", Realname: "审核教师", IdentityType: "staff", College: ca.Name}
	require.NoError(t, db.Create(&teacher).Error)

	students := make([]models.User, 4)
	for i := range students {
		students[i] = models.User{
			Username: fmt.Sprintf("A00%d", i), Realname: fmt.Sprintf("审核学生%d", i),
			College: ca.Name, Major: "专业甲", IdentityType: "student",
		}
		require.NoError(t, db.Create(&students[i]).Error)
	}
	comp := models.CompDirectory{CompCode: "AX1", CompName: "审核校赛", CompLevel: "校级", ManagerID: teacher.ID, Year: 2026}
	require.NoError(t, db.Create(&comp).Error)

	// 4 条 draft 补录奖
	awards := make([]models.Award, 4)
	for i := range awards {
		awards[i] = models.Award{
			CompID: comp.ID, ProjectName: fmt.Sprintf("审核项目%d", i),
			AwardLevel: "省级一等奖", AwardCategory: "省级", AwardName: "一等奖",
			Status: "draft", Source: "supplement", AdvisorInfo: "[]", LevelRank: 99,
		}
		require.NoError(t, db.Create(&awards[i]).Error)
		require.NoError(t, db.Create(&models.AwardMember{
			AwardID: awards[i].ID, StudentID: students[i].ID, StudentNumber: students[i].Username,
			Name: students[i].Realname, College: ca.Name, Major: "专业甲", IsLeader: true,
		}).Error)
	}

	invoke := func(method, path, role string, collegeID *uint, userID uint, body interface{}, handler gin.HandlerFunc, params ...gin.Param) *httptest.ResponseRecorder {
		var reader *bytes.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			reader = bytes.NewReader(data)
		} else {
			reader = bytes.NewReader(nil)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, path, reader)
		c.Request.Header.Set("Content-Type", "application/json")
		if body == nil {
			c.Request = httptest.NewRequest(method, path, nil)
		}
		c.Params = params
		c.Set("role_code", role)
		c.Set("user_id", userID)
		c.Set("managed_college_id", collegeID)
		handler(c)
		return w
	}

	t.Run("GetAwardAuditDetail", func(t *testing.T) {
		w := invoke("GET", fmt.Sprintf("/api/award/audit/detail/%d", awards[0].ID), "school_admin", nil, schoolAdmin.ID, nil,
			GetAwardAuditDetail, gin.Param{Key: "id", Value: fmt.Sprint(awards[0].ID)})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "审核项目0")

		w = invoke("GET", "/api/award/audit/detail/999999", "school_admin", nil, schoolAdmin.ID, nil,
			GetAwardAuditDetail, gin.Param{Key: "id", Value: "999999"})
		require.Equal(t, 404, w.Code)
	})

	t.Run("PassAwardAudit happy + duplicate + version conflict", func(t *testing.T) {
		// 版本冲突：传错误 update_time
		badTime := awards[0].UpdatedAt.Add(-time.Hour)
		w := invoke("PUT", fmt.Sprintf("/api/award/audit/%d/pass", awards[0].ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"update_time": badTime},
			PassAwardAudit, gin.Param{Key: "id", Value: fmt.Sprint(awards[0].ID)})
		require.Equal(t, 409, w.Code, w.Body.String())

		// 正确版本通过
		w = invoke("PUT", fmt.Sprintf("/api/award/audit/%d/pass", awards[0].ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"update_time": awards[0].UpdatedAt},
			PassAwardAudit, gin.Param{Key: "id", Value: fmt.Sprint(awards[0].ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		// 重复通过
		w = invoke("PUT", fmt.Sprintf("/api/award/audit/%d/pass", awards[0].ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{},
			PassAwardAudit, gin.Param{Key: "id", Value: fmt.Sprint(awards[0].ID)})
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "已审核")

		// 不存在
		w = invoke("PUT", "/api/award/audit/999999/pass", "school_admin", nil, schoolAdmin.ID, map[string]interface{}{},
			PassAwardAudit, gin.Param{Key: "id", Value: "999999"})
		require.Equal(t, 404, w.Code)
	})

	t.Run("RejectAwardAudit", func(t *testing.T) {
		// 缺原因
		w := invoke("PUT", fmt.Sprintf("/api/award/audit/%d/reject", awards[1].ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{},
			RejectAwardAudit, gin.Param{Key: "id", Value: fmt.Sprint(awards[1].ID)})
		require.Equal(t, 400, w.Code)

		// 正确驳回
		w = invoke("PUT", fmt.Sprintf("/api/award/audit/%d/reject", awards[1].ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"reason": "材料不全", "update_time": awards[1].UpdatedAt},
			RejectAwardAudit, gin.Param{Key: "id", Value: fmt.Sprint(awards[1].ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		var rejected models.Award
		require.NoError(t, db.First(&rejected, awards[1].ID).Error)
		require.Equal(t, "rejected", rejected.Status)
		require.Equal(t, "材料不全", rejected.RejectReason)

		// 重复驳回
		w = invoke("PUT", fmt.Sprintf("/api/award/audit/%d/reject", awards[1].ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"reason": "再驳回"},
			RejectAwardAudit, gin.Param{Key: "id", Value: fmt.Sprint(awards[1].ID)})
		require.Equal(t, 400, w.Code)
	})

	t.Run("BatchPassAwardAudit", func(t *testing.T) {
		// 空 ids
		w := invoke("PUT", "/api/award/audit/batch/pass", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"ids": []uint{}}, BatchPassAwardAudit)
		require.Equal(t, 400, w.Code)

		// 版本错误 → 冲突；awards[2] 仍为 draft
		var cur models.Award
		require.NoError(t, db.First(&cur, awards[2].ID).Error)
		w = invoke("PUT", "/api/award/audit/batch/pass", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"ids": []uint{cur.ID}, "versions": map[string]string{fmt.Sprint(cur.ID): cur.UpdatedAt.Add(-time.Hour).Format(time.RFC3339Nano)}},
			BatchPassAwardAudit)
		require.Equal(t, 409, w.Code, w.Body.String())

		// 正确批量通过 awards[2]
		w = invoke("PUT", "/api/award/audit/batch/pass", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"ids": []uint{cur.ID}, "versions": map[string]string{fmt.Sprint(cur.ID): cur.UpdatedAt.Format(time.RFC3339Nano)}},
			BatchPassAwardAudit)
		require.Equal(t, 200, w.Code, w.Body.String())

		// 已审核再批量 → 没有可审核
		w = invoke("PUT", "/api/award/audit/batch/pass", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"ids": []uint{cur.ID}}, BatchPassAwardAudit)
		require.Equal(t, 400, w.Code)
	})

	t.Run("BatchRejectAwardAudit", func(t *testing.T) {
		// 缺 reason
		w := invoke("PUT", "/api/award/audit/batch/reject", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"ids": []uint{awards[3].ID}}, BatchRejectAwardAudit)
		require.Equal(t, 400, w.Code)

		// 正确批量驳回 awards[3]
		var cur models.Award
		require.NoError(t, db.First(&cur, awards[3].ID).Error)
		w = invoke("PUT", "/api/award/audit/batch/reject", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{
				"ids":      []uint{cur.ID},
				"reason":   "批量驳回原因",
				"versions": map[string]string{fmt.Sprint(cur.ID): cur.UpdatedAt.Format(time.RFC3339Nano)},
			}, BatchRejectAwardAudit)
		require.Equal(t, 200, w.Code, w.Body.String())

		var rejected models.Award
		require.NoError(t, db.First(&rejected, cur.ID).Error)
		require.Equal(t, "rejected", rejected.Status)
		require.Equal(t, "批量驳回原因", rejected.RejectReason)

		// 已驳回再批量驳回
		w = invoke("PUT", "/api/award/audit/batch/reject", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"ids": []uint{cur.ID}, "reason": "再来"},
			BatchRejectAwardAudit)
		require.Equal(t, 400, w.Code)
	})

	t.Run("Pass empty body path and multi auditor", func(t *testing.T) {
		// 新建 draft，用第二个校管通过（ContentLength=0 空 body）
		a := models.Award{CompID: comp.ID, ProjectName: "空body通过", AwardLevel: "省级一等奖", AwardCategory: "省级", AwardName: "一等奖", Status: "draft", Source: "supplement", AdvisorInfo: "[]"}
		require.NoError(t, db.Create(&a).Error)
		w := invoke("PUT", fmt.Sprintf("/api/award/audit/%d/pass", a.ID), "school_admin", nil, auditor2.ID, nil,
			PassAwardAudit, gin.Param{Key: "id", Value: fmt.Sprint(a.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())
		var approved models.Award
		require.NoError(t, db.First(&approved, a.ID).Error)
		require.Equal(t, "approved", approved.Status)
		require.NotNil(t, approved.AuditorID)
		require.Equal(t, auditor2.ID, *approved.AuditorID)
	})
}

// 导入设置 / 学生变更记录 / 指导教师更新（补 Preview 以外的 0% 与低覆盖路径）
func TestAwardSettingsAdvisorExtraMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	ca := models.College{Name: "附加甲学院", IsValid: true}
	require.NoError(t, db.Create(&ca).Error)
	schoolAdmin := models.User{Username: "ES001", Realname: "附加校管", IdentityType: "staff", College: "教务处"}
	require.NoError(t, db.Create(&schoolAdmin).Error)
	teacher := models.User{Username: "ET001", Realname: "附加教师", IdentityType: "staff", College: ca.Name}
	require.NoError(t, db.Create(&teacher).Error)
	student := models.User{Username: "E001", Realname: "附加学生", College: ca.Name, Major: "专业甲", IdentityType: "student"}
	require.NoError(t, db.Create(&student).Error)
	comp := models.CompDirectory{CompCode: "EX1", CompName: "附加校赛", CompLevel: "校级", ManagerID: teacher.ID, Year: 2026}
	require.NoError(t, db.Create(&comp).Error)

	proofHash := "b1b2c3d4e5f6789012345678abcdef02"
	proofName := "p.png"
	proofURL := buildDownloadURL("award_proof", proofHash, proofName)
	require.NoError(t, db.Create(&models.FileRecord{
		FileHash: proofHash, OriginalName: proofName,
		StoragePath: "award_proof/" + proofHash + "_" + proofName,
		FileSize: 5, FileExt: "png", BizType: "award_proof", UploaderID: student.ID,
	}).Error)

	award := models.Award{
		CompID: comp.ID, ProjectName: "附加指导项目", AwardLevel: "省级一等奖",
		AwardCategory: "省级", AwardName: "一等奖", Status: "draft", Source: "supplement",
		AdvisorInfo: "[]", ProofUrl: proofURL, LevelRank: 99,
	}
	require.NoError(t, db.Create(&award).Error)
	require.NoError(t, db.Create(&models.AwardMember{
		AwardID: award.ID, StudentID: student.ID, StudentNumber: student.Username,
		Name: student.Realname, College: ca.Name, Major: "专业甲", IsLeader: true,
	}).Error)
	require.NoError(t, db.Create(&models.AwardAdvisor{
		AwardID: award.ID, Username: teacher.Username, Name: teacher.Realname, College: ca.Name, Sort: 0,
	}).Error)

	invoke := func(method, path, role string, collegeID *uint, userID uint, body interface{}, handler gin.HandlerFunc, params ...gin.Param) *httptest.ResponseRecorder {
		var reader *bytes.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			reader = bytes.NewReader(data)
		} else {
			reader = bytes.NewReader(nil)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, path, reader)
		c.Request.Header.Set("Content-Type", "application/json")
		if body == nil {
			c.Request = httptest.NewRequest(method, path, nil)
		}
		c.Params = params
		c.Set("role_code", role)
		c.Set("user_id", userID)
		c.Set("managed_college_id", collegeID)
		handler(c)
		return w
	}

	t.Run("import settings get/update permissions", func(t *testing.T) {
		w := invoke("GET", "/api/award/import/settings", "school_admin", nil, schoolAdmin.ID, nil, GetAwardImportSetting)
		require.Equal(t, 200, w.Code, w.Body.String())

		// 学生无填报权限
		w = invoke("GET", "/api/award/import/settings", "student", nil, student.ID, nil, GetAwardImportSetting)
		require.Equal(t, 403, w.Code)

		// 院管可读不可写
		w = invoke("GET", "/api/award/import/settings", "college_admin", &ca.ID, teacher.ID, nil, GetAwardImportSetting)
		require.Equal(t, 200, w.Code)
		w = invoke("PUT", "/api/award/import/settings", "college_admin", &ca.ID, teacher.ID,
			map[string]interface{}{"require_registration": true}, UpdateAwardImportSetting)
		require.Equal(t, 403, w.Code)

		// 校管写
		w = invoke("PUT", "/api/award/import/settings", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"require_registration": true}, UpdateAwardImportSetting)
		require.Equal(t, 200, w.Code, w.Body.String())

		// 缺字段
		w = invoke("PUT", "/api/award/import/settings", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{}, UpdateAwardImportSetting)
		require.Equal(t, 400, w.Code)
	})

	t.Run("GetAwardChanges for student member", func(t *testing.T) {
		require.NoError(t, db.Create(&models.AwardChange{
			AwardID: award.ID, OperatorID: teacher.ID, OperatorName: teacher.Realname,
			Before: "{}", After: `{"remark":"导师修改"}`,
		}).Error)

		w := invoke("GET", fmt.Sprintf("/api/award/%d/changes", award.ID), "student", nil, student.ID, nil,
			GetAwardChanges, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "导师修改")

		// 非成员学生 → 403/404
		other := models.User{Username: "E999", Realname: "外人", College: ca.Name, Major: "专业甲", IdentityType: "student"}
		require.NoError(t, db.Create(&other).Error)
		w = invoke("GET", fmt.Sprintf("/api/award/%d/changes", award.ID), "student", nil, other.ID, nil,
			GetAwardChanges, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		require.Equal(t, 403, w.Code)
	})

	t.Run("UpdateAdvisorAward validation and version", func(t *testing.T) {
		// 参数错误
		w := invoke("PUT", fmt.Sprintf("/api/award/advisor/%d", award.ID), "teacher", nil, teacher.ID,
			map[string]interface{}{},
			UpdateAdvisorAward, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		require.Equal(t, 400, w.Code)

		// 证明地址非法
		w = invoke("PUT", fmt.Sprintf("/api/award/advisor/%d", award.ID), "teacher", nil, teacher.ID,
			map[string]interface{}{
				"project_name": "改名", "award_category": "省级", "award_name": "一等奖",
				"award_date": "2026-05-01", "proof_url": "evil://x", "update_time": award.UpdatedAt,
			},
			UpdateAdvisorAward, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		require.Equal(t, 400, w.Code)

		// 版本冲突
		bad := award.UpdatedAt.Add(-time.Hour)
		w = invoke("PUT", fmt.Sprintf("/api/award/advisor/%d", award.ID), "teacher", nil, teacher.ID,
			map[string]interface{}{
				"project_name": "改名", "award_category": "省级", "award_name": "一等奖",
				"award_date": "2026-05-01", "proof_url": proofURL, "update_time": bad,
			},
			UpdateAdvisorAward, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		require.Equal(t, 409, w.Code, w.Body.String())

		// 成功修改
		w = invoke("PUT", fmt.Sprintf("/api/award/advisor/%d", award.ID), "teacher", nil, teacher.ID,
			map[string]interface{}{
				"project_name": "附加指导项目改", "award_category": "省级", "award_name": "二等奖",
				"award_date": "2026-05-01", "proof_url": proofURL, "remark": "导师修改",
				"update_time": award.UpdatedAt,
			},
			UpdateAdvisorAward, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		var updated models.Award
		require.NoError(t, db.First(&updated, award.ID).Error)
		require.Equal(t, "附加指导项目改", updated.ProjectName)
		require.Equal(t, "二等奖", updated.AwardName)

		// 已批准不可改
		require.NoError(t, db.Model(&updated).Update("status", "approved").Error)
		require.NoError(t, db.First(&updated, award.ID).Error)
		w = invoke("PUT", fmt.Sprintf("/api/award/advisor/%d", award.ID), "teacher", nil, teacher.ID,
			map[string]interface{}{
				"project_name": "再改", "award_category": "省级", "award_name": "一等奖",
				"award_date": "2026-05-01", "proof_url": proofURL, "update_time": updated.UpdatedAt,
			},
			UpdateAdvisorAward, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		require.Equal(t, 403, w.Code)
	})
}
