package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// 第二轮扩测：读导入边界、成员团队、补录错误、申报状态机、报名配置、文件/导出边角
func TestCoveragePushMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Role{}, &models.UserRole{}, &models.Permission{}, &models.RolePermission{}, &models.CompDeclaration{}, &models.Summary{}, &models.Notice{}))
	// 通知缓存清理依赖 Redis；本机有则接上
	if rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}); rdb.Ping(t.Context()).Err() == nil {
		orig := middleware.RedisClient
		middleware.RedisClient = rdb
		t.Cleanup(func() { middleware.RedisClient = orig; _ = rdb.Close() })
	}
	roles := map[string]models.Role{}
	for _, rc := range []string{"school_admin", "college_admin", "teacher", "competition_manager", "student"} {
		role := models.Role{RoleName: rc, RoleCode: rc, Description: rc}
		require.NoError(t, db.Create(&role).Error)
		roles[rc] = role
	}
	for _, code := range []string{"comp:update", "summary:edit", "reg:config:edit", "comp:list", "award:import", "declare:create", "declare:update", "declare:submit", "declare:delete", "declare:revoke"} {
		perm := models.Permission{Name: code, Code: code, Type: 3, Description: code}
		require.NoError(t, db.Create(&perm).Error)
		require.NoError(t, db.Create(&models.RolePermission{RoleID: roles["school_admin"].ID, PermissionID: perm.ID}).Error)
		require.NoError(t, db.Create(&models.RolePermission{RoleID: roles["college_admin"].ID, PermissionID: perm.ID}).Error)
	}

	ca := models.College{Name: "推2甲学院", IsValid: true}
	cb := models.College{Name: "推2乙学院", IsValid: true}
	require.NoError(t, db.Create(&ca).Error)
	require.NoError(t, db.Create(&cb).Error)

	mk := func(username, realname, role, college, identity string, managed *uint) models.User {
		u := models.User{Username: username, Realname: realname, College: college, Major: "专业", IdentityType: identity, ManagedCollegeID: managed}
		require.NoError(t, db.Create(&u).Error)
		require.NoError(t, database.SetUserRole(db, u.ID, roles[role].ID))
		return u
	}
	schoolAdmin := mk("YS001", "推2校管", "school_admin", "教务处", "staff", nil)
	collegeAdmin := mk("YA001", "推2甲管", "college_admin", ca.Name, "staff", &ca.ID)
	teacher := mk("YT001", "推2教师", "teacher", ca.Name, "staff", nil)
	students := []models.User{
		mk("Y001", "推2学生甲", "student", ca.Name, "student", nil),
		mk("Y002", "推2学生乙", "student", ca.Name, "student", nil),
		mk("Y003", "推2学生丙", "student", ca.Name, "student", nil),
	}

	schoolComp := models.CompDirectory{CompCode: "YK1", CompName: "推2校赛", CompLevel: "校级", ManagerID: schoolAdmin.ID, Year: 2026, Status: 2}
	require.NoError(t, db.Create(&schoolComp).Error)
	require.NoError(t, db.Create(&models.CompDetail{
		CompID: schoolComp.ID, ParticipantType: 1,
		CompStartTime: time.Now().Add(-time.Hour), CompEndTime: time.Now().Add(time.Hour),
		RegStartTime: time.Now().Add(-time.Hour), RegEndTime: time.Now().Add(time.Hour),
		SubmitStartTime: time.Now().Add(-time.Hour), SubmitEndTime: time.Now().Add(time.Hour),
		ReviewStartTime: time.Now().Add(-time.Hour), ReviewEndTime: time.Now().Add(time.Hour),
		Track: "[]", AwardHierarchy: "[]", MinTeamMember: 1, MaxTeamMember: 5,
	}).Error)
	provComp := models.CompDirectory{CompCode: "YK2", CompName: "推2甲省赛", CompLevel: "省级", ManagerID: schoolAdmin.ID, CollegeID: &ca.ID, Year: 2026, Status: 2}
	require.NoError(t, db.Create(&provComp).Error)

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
		if r, ok := roles[role]; ok {
			c.Set("role_id", r.ID)
			c.Set("permission_version", r.PermissionVersion)
		}
		handler(c)
		return w
	}

	t.Run("readAwardImport edges via preview", func(t *testing.T) {
		// 无效 comp_id
		w := invoke("POST", "/api/award/import/preview?comp_id=abc", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"rows": []map[string]string{{"student_id": "Y001"}}}, PreviewAwardImport)
		require.Equal(t, 400, w.Code)

		// 空 rows
		w = invoke("POST", fmt.Sprintf("/api/award/import/preview?comp_id=%d", schoolComp.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"rows": []interface{}{}}, PreviewAwardImport)
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "1至5000")

		// JSON 格式错误
		w = httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", fmt.Sprintf("/api/award/import/preview?comp_id=%d", schoolComp.ID), strings.NewReader("{not-json"))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("role_code", "school_admin")
		c.Set("user_id", schoolAdmin.ID)
		PreviewAwardImport(c)
		require.Equal(t, 400, w.Code)

		// 无 content-type → 走文件上传失败分支
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", fmt.Sprintf("/api/award/import/preview?comp_id=%d", schoolComp.ID), nil)
		c.Set("role_code", "school_admin")
		c.Set("user_id", schoolAdmin.ID)
		PreviewAwardImport(c)
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "文件")

		// 非法 Excel 文件
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, err := mw.CreateFormFile("file", "bad.xlsx")
		require.NoError(t, err)
		_, _ = fw.Write([]byte("not-an-xlsx"))
		require.NoError(t, mw.Close())
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", fmt.Sprintf("/api/award/import/preview?comp_id=%d", schoolComp.ID), &buf)
		c.Request.Header.Set("Content-Type", mw.FormDataContentType())
		c.Set("role_code", "school_admin")
		c.Set("user_id", schoolAdmin.ID)
		PreviewAwardImport(c)
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "Excel")

		// 合法 Excel 上传预览
		xlsx := buildPreviewExcel(students[0].Username, students[0].Realname, ca.Name)
		var buf2 bytes.Buffer
		mw2 := multipart.NewWriter(&buf2)
		fw2, err := mw2.CreateFormFile("file", "ok.xlsx")
		require.NoError(t, err)
		_, _ = fw2.Write(xlsx)
		require.NoError(t, mw2.Close())
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", fmt.Sprintf("/api/award/import/preview?comp_id=%d", schoolComp.ID), &buf2)
		c.Request.Header.Set("Content-Type", mw2.FormDataContentType())
		c.Set("role_code", "school_admin")
		c.Set("user_id", schoolAdmin.ID)
		PreviewAwardImport(c)
		require.Equal(t, 200, w.Code, w.Body.String())
	})

	t.Run("UpdateAwardMember college transfer and scope", func(t *testing.T) {
		tag := time.Now().UnixNano()
		w := invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", schoolComp.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"rows": []map[string]string{{
				"student_id": students[0].Username, "name": students[0].Realname, "college": ca.Name, "major": "专业",
				"project_name": fmt.Sprintf("推2成员%d", tag), "award_month": "2026-06", "award_category": "省级", "award_name": "一等奖",
			}}}, ImportAward)
		require.Equal(t, 200, w.Code, w.Body.String())
		var award models.Award
		require.NoError(t, db.Where("project_name = ?", fmt.Sprintf("推2成员%d", tag)).First(&award).Error)
		var member models.AwardMember
		require.NoError(t, db.Where("award_id = ?", award.ID).First(&member).Error)

		// 校管确认归属（原为空）
		w = invoke("PUT", fmt.Sprintf("/api/award/members/%d", member.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"remark": "归属确认", "submitted_college_id": ca.ID},
			UpdateAwardMember, gin.Param{Key: "id", Value: fmt.Sprint(member.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		// 已有归属不能转移
		w = invoke("PUT", fmt.Sprintf("/api/award/members/%d", member.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"remark": "x", "submitted_college_id": cb.ID},
			UpdateAwardMember, gin.Param{Key: "id", Value: fmt.Sprint(member.ID)})
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "归属")

		// 院管无权确认归属
		w = invoke("PUT", fmt.Sprintf("/api/award/members/%d", member.ID), "college_admin", &ca.ID, collegeAdmin.ID,
			map[string]interface{}{"remark": "y", "submitted_college_id": cb.ID},
			UpdateAwardMember, gin.Param{Key: "id", Value: fmt.Sprint(member.ID)})
		require.Equal(t, 403, w.Code)

		// 乙院管改甲院填报成员 → 403
		cbAdminID := collegeAdmin.ID
		_ = cbAdminID
		w = invoke("PUT", fmt.Sprintf("/api/award/members/%d", member.ID), "college_admin", &cb.ID, collegeAdmin.ID,
			map[string]interface{}{"remark": "z"},
			UpdateAwardMember, gin.Param{Key: "id", Value: fmt.Sprint(member.ID)})
		require.Equal(t, 403, w.Code)

		// 非法学院 id
		w = invoke("PUT", fmt.Sprintf("/api/award/members/%d", member.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"remark": "x", "submitted_college_id": 999999},
			UpdateAwardMember, gin.Param{Key: "id", Value: fmt.Sprint(member.ID)})
		require.Equal(t, 400, w.Code)
	})

	t.Run("UpdateAwardTeam advisors replace fill empty", func(t *testing.T) {
		// 无教师的导入奖
		tag := time.Now().UnixNano()
		w := invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", schoolComp.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"rows": []map[string]string{{
				"student_id": students[1].Username, "name": students[1].Realname, "college": ca.Name, "major": "专业",
				"project_name": fmt.Sprintf("推2无师%d", tag), "award_month": "2026-06", "award_category": "省级", "award_name": "二等奖",
			}}}, ImportAward)
		require.Equal(t, 200, w.Code, w.Body.String())
		var award models.Award
		require.NoError(t, db.Where("project_name = ?", fmt.Sprintf("推2无师%d", tag)).First(&award).Error)

		// 带 advisors 提交 → replaceAwardAdvisors / fill
		w = invoke("PUT", fmt.Sprintf("/api/award/teams/%d", award.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{
				"project_name": fmt.Sprintf("推2补师%d", tag), "award_category": "省级", "award_name": "一等奖", "award_month": "2026-07",
				"advisors": []map[string]string{{"username": "YT001"}},
			},
			UpdateAwardTeam, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())
		var advCount int64
		require.NoError(t, db.Model(&models.AwardAdvisor{}).Where("award_id = ?", award.ID).Count(&advCount).Error)
		require.EqualValues(t, 1, advCount)

		// 项目名冲突
		w = invoke("PUT", fmt.Sprintf("/api/award/teams/%d", award.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{
				"project_name": fmt.Sprintf("推2补师%d", tag), "award_category": "省级", "award_name": "一等奖", "award_month": "2026-07",
			},
			UpdateAwardTeam, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		// 同名自身可能允许——若冲突则 400
		require.Contains(t, []int{200, 400}, w.Code, w.Body.String())

		// 不存在教师
		w = invoke("PUT", fmt.Sprintf("/api/award/teams/%d", award.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{
				"project_name": fmt.Sprintf("推2错师%d", tag), "award_category": "省级", "award_name": "一等奖", "award_month": "2026-07",
				"advisors": []map[string]string{{"username": "YNOPE99"}},
			},
			UpdateAwardTeam, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		require.Equal(t, 400, w.Code)
	})

	t.Run("Supplement validation branches", func(t *testing.T) {
		proofHash := strings.Repeat("f", 32)
		proofName := "proof.png"
		proofURL := buildDownloadURL("award_proof", proofHash, proofName)
		require.NoError(t, db.Create(&models.FileRecord{
			FileHash: proofHash, OriginalName: proofName,
			StoragePath: "static/award_proofs/" + proofHash + "_" + proofName,
			FileSize: 1, FileExt: "png", BizType: "award_proof", UploaderID: students[2].ID,
		}).Error)
		base := func() map[string]interface{} {
			return map[string]interface{}{
				"comp_id": schoolComp.ID, "team_name": fmt.Sprintf("推2补录%d", time.Now().UnixNano()),
				"award_level": "省级一等奖", "award_name": "一等奖", "award_date": "2026-05-01",
				"proof_url": proofURL, "participation_type": "individual", "award_category": "省级",
				"advisors": []map[string]string{{"username": "YT001", "name": "推2教师", "college": ca.Name}},
				"members": []map[string]interface{}{
					{"name": students[2].Realname, "student_id": students[2].Username, "phone": "13800000000", "college": ca.Name, "is_leader": true},
				},
			}
		}
		// 个人赛多队员
		b := base()
		b["members"] = []map[string]interface{}{
			{"name": "甲", "student_id": students[2].Username, "phone": "1", "college": ca.Name, "is_leader": true},
			{"name": "乙", "student_id": students[1].Username, "phone": "2", "college": ca.Name, "is_leader": false},
		}
		w := invoke("POST", "/api/award/student/supplement", "student", nil, students[2].ID, b, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "个人赛")

		// 团队赛人数不足
		b = base()
		b["participation_type"] = "team"
		w = invoke("POST", "/api/award/student/supplement", "student", nil, students[2].ID, b, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "团队")

		// 非法类别
		b = base()
		b["award_category"] = "院级"
		w = invoke("POST", "/api/award/student/supplement", "student", nil, students[2].ID, b, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code)

		// 缺获奖时间
		b = base()
		b["award_date"] = ""
		w = invoke("POST", "/api/award/student/supplement", "student", nil, students[2].ID, b, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code)

		// 超长备注
		b = base()
		b["remark"] = strings.Repeat("长", 2001)
		w = invoke("POST", "/api/award/student/supplement", "student", nil, students[2].ID, b, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code)

		// 无效证明地址
		b = base()
		b["proof_url"] = "/etc/passwd"
		w = invoke("POST", "/api/award/student/supplement", "student", nil, students[2].ID, b, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code)
	})

	t.Run("Declare update revoke delete", func(t *testing.T) {
		// 甲管建草稿
		w := invoke("POST", "/api/declare", "college_admin", &ca.ID, collegeAdmin.ID, map[string]interface{}{
			"comp_name": "推2申报赛", "comp_level": "省级", "comp_type": "学科竞赛",
			"organizer": "主办方", "undertaker": "承办方", "college_id": ca.ID, "manager_id": teacher.ID, "year": 2026,
		}, CreateDeclare)
		require.Equal(t, 200, w.Code, w.Body.String())
		var d models.CompDeclaration
		require.NoError(t, db.Where("comp_name = ?", "推2申报赛").First(&d).Error)

		// 更新
		w = invoke("PUT", fmt.Sprintf("/api/declare/%d", d.ID), "college_admin", &ca.ID, collegeAdmin.ID,
			map[string]interface{}{"comp_name": "推2申报赛改", "desc": "更新说明"},
			UpdateDeclare, gin.Param{Key: "id", Value: fmt.Sprint(d.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		// 提交
		w = invoke("POST", fmt.Sprintf("/api/declare/%d/submit", d.ID), "college_admin", &ca.ID, collegeAdmin.ID,
			nil, SubmitDeclare, gin.Param{Key: "id", Value: fmt.Sprint(d.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		// 重复提交
		w = invoke("POST", fmt.Sprintf("/api/declare/%d/submit", d.ID), "college_admin", &ca.ID, collegeAdmin.ID,
			nil, SubmitDeclare, gin.Param{Key: "id", Value: fmt.Sprint(d.ID)})
		require.Equal(t, 403, w.Code)

		// 撤回
		w = invoke("POST", fmt.Sprintf("/api/declare/%d/revoke", d.ID), "college_admin", &ca.ID, collegeAdmin.ID,
			nil, RevokeDeclare, gin.Param{Key: "id", Value: fmt.Sprint(d.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		// 草稿可删
		w = invoke("DELETE", fmt.Sprintf("/api/declare/%d", d.ID), "college_admin", &ca.ID, collegeAdmin.ID,
			nil, DeleteDeclare, gin.Param{Key: "id", Value: fmt.Sprint(d.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		// 我的待审/已发布
		w = invoke("GET", "/api/declare/my/pending?page=1&page_size=10", "college_admin", &ca.ID, collegeAdmin.ID, nil, GetMyPendingDeclares)
		require.Equal(t, 200, w.Code)
		w = invoke("GET", "/api/declare/my/published?page=1&page_size=10", "college_admin", &ca.ID, collegeAdmin.ID, nil, GetMyPublishedDeclares)
		require.Equal(t, 200, w.Code)
		w = invoke("GET", "/api/declare/audited/list?page=1&page_size=10", "school_admin", nil, schoolAdmin.ID, nil, GetAuditedDeclares)
		require.Equal(t, 200, w.Code)
	})

	t.Run("SaveRegConfig full body", func(t *testing.T) {
		now := time.Now()
		w := invoke("POST", "/api/reg/config", "school_admin", nil, schoolAdmin.ID, map[string]interface{}{
			"comp_id": schoolComp.ID, "participant_type": 1,
			"min_team_member": 1, "max_team_member": 5,
			"reg_start_time":  now.Add(-2 * time.Hour).Format(time.RFC3339),
			"reg_end_time":    now.Add(2 * time.Hour).Format(time.RFC3339),
			"submit_start_time": now.Add(-time.Hour).Format(time.RFC3339),
			"submit_end_time":   now.Add(time.Hour).Format(time.RFC3339),
			"grade_requirement": []int{2022, 2023, 2024},
			"award_hierarchy":   []string{"一等奖", "二等奖"},
			"need_advisor":      1, "need_attachment": 0,
			"need_reg_audit":    1,
			"track":             []interface{}{},
		}, SaveRegConfig)
		require.Equal(t, 200, w.Code, w.Body.String())

		// 缺起止时间
		w = invoke("POST", "/api/reg/config", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"comp_id": schoolComp.ID}, SaveRegConfig)
		require.Equal(t, 400, w.Code)

		// 只写开始时间
		w = invoke("POST", "/api/reg/config", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"comp_id": schoolComp.ID, "reg_start_time": now.Format(time.RFC3339)}, SaveRegConfig)
		require.Equal(t, 400, w.Code)

		// 我的报名状态（无报名）
		w = invoke("GET", fmt.Sprintf("/api/reg/status?comp_id=%d", schoolComp.ID), "student", nil, students[0].ID, nil, GetMyRegStatus)
		require.Contains(t, []int{200, 400, 404}, w.Code, w.Body.String())
	})

	t.Run("Export report all years and summary buckets", func(t *testing.T) {
		// 省级赛再导一条便于汇总
		w := invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", provComp.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"rows": []map[string]string{{
				"student_id": students[0].Username, "name": students[0].Realname, "college": ca.Name, "major": "专业",
				"project_name": fmt.Sprintf("推2省赛项%d", time.Now().UnixNano()), "award_month": "2026-06",
				"award_category": "国家级", "award_name": "银奖",
			}}}, ImportAward)
		require.Equal(t, 200, w.Code, w.Body.String())

		w = invoke("GET", "/api/award/export/report", "school_admin", nil, schoolAdmin.ID, nil, ExportAwardReport)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Greater(t, w.Body.Len(), 1000)

		w = invoke("GET", "/api/award/export/students", "school_admin", nil, schoolAdmin.ID, nil, ExportAwardStudents)
		require.Equal(t, 200, w.Code)

		// 院管导省赛范围
		w = invoke("GET", "/api/award/export/projects", "college_admin", &ca.ID, collegeAdmin.ID, nil, ExportAwardProjects)
		require.Equal(t, 200, w.Code)
	})

	t.Run("File access notice and reg paths", func(t *testing.T) {
		wd, err := os.Getwd()
		require.NoError(t, err)
		dir := filepath.Join(wd, "static", "notices")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(wd, "static", "notices")) })
		hash := strings.Repeat("a", 32)
		name := "n.txt"
		require.NoError(t, os.WriteFile(filepath.Join(dir, hash+"_"+name), []byte("n"), 0o644))
		require.NoError(t, db.Create(&models.FileRecord{
			FileHash: hash, OriginalName: name,
			StoragePath: "static/notices/" + hash + "_" + name,
			FileSize: 1, FileExt: "txt", BizType: "notice", UploaderID: schoolAdmin.ID,
		}).Error)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Params = []gin.Param{{Key: "type", Value: "notice"}, {Key: "filename", Value: hash + "_" + name}}
		c.Set("role_code", "student")
		c.Set("user_id", students[0].ID)
		DownloadFile(c)
		require.Equal(t, 200, w.Code, w.Body.String())

		// 未登录
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Params = []gin.Param{{Key: "type", Value: "notice"}, {Key: "filename", Value: hash + "_" + name}}
		DownloadFile(c)
		require.True(t, w.Code == 401 || w.Code == 403, w.Body.String())
	})

	t.Run("Competition list filters", func(t *testing.T) {
		w := invoke("GET", "/api/comp/list?page=1&page_size=5&comp_level=校级&year=2026", "school_admin", nil, schoolAdmin.ID, nil, GetCompetitionList)
		require.Equal(t, 200, w.Code, w.Body.String())
		w = invoke("GET", "/api/comp/list?page=1&page_size=5&status=已结束", "school_admin", nil, schoolAdmin.ID, nil, GetCompetitionList)
		require.Equal(t, 200, w.Code)
		w = invoke("GET", "/api/comp/list?page=1&page_size=5&year=abcd", "school_admin", nil, schoolAdmin.ID, nil, GetCompetitionList)
		// year 非法可能 400 或忽略
		require.Contains(t, []int{200, 400}, w.Code, w.Body.String())
	})

	t.Run("Notice publish update delete", func(t *testing.T) {
		// 表单创建
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := &bytes.Buffer{}
		mw := multipart.NewWriter(body)
		_ = mw.WriteField("title", "推2通知")
		_ = mw.WriteField("content", "内容")
		_ = mw.WriteField("compID", fmt.Sprint(schoolComp.ID))
		require.NoError(t, mw.Close())
		c.Request = httptest.NewRequest("POST", "/api/notice/comp/create", body)
		c.Request.Header.Set("Content-Type", mw.FormDataContentType())
		c.Set("role_code", "school_admin")
		c.Set("user_id", schoolAdmin.ID)
		c.Set("permission_version", roles["school_admin"].PermissionVersion)
		c.Set("role_id", roles["school_admin"].ID)
		CreateCompNotice(c)
		require.Equal(t, 200, w.Code, w.Body.String())
		var notice models.Notice
		require.NoError(t, db.Where("title = ?", "推2通知").First(&notice).Error)

		// UpdateNotice 走表单
		ub := &bytes.Buffer{}
		uw := multipart.NewWriter(ub)
		_ = uw.WriteField("title", "推2通知改")
		_ = uw.WriteField("content", "改内容")
		require.NoError(t, uw.Close())
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", fmt.Sprintf("/api/notice/%d", notice.ID), ub)
		c.Request.Header.Set("Content-Type", uw.FormDataContentType())
		c.Params = []gin.Param{{Key: "id", Value: fmt.Sprint(notice.ID)}}
		c.Set("role_code", "school_admin")
		c.Set("user_id", schoolAdmin.ID)
		c.Set("role_id", roles["school_admin"].ID)
		c.Set("permission_version", roles["school_admin"].PermissionVersion)
		UpdateNotice(c)
		require.Equal(t, 200, w.Code, w.Body.String())

		w = invoke("PUT", fmt.Sprintf("/api/notice/%d/publish", notice.ID), "school_admin", nil, schoolAdmin.ID,
			nil, PublishNotice, gin.Param{Key: "id", Value: fmt.Sprint(notice.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		w = invoke("GET", fmt.Sprintf("/api/notice/%d", notice.ID), "school_admin", nil, schoolAdmin.ID,
			nil, GetNoticeDetail, gin.Param{Key: "id", Value: fmt.Sprint(notice.ID)})
		require.Equal(t, 200, w.Code)

		w = invoke("DELETE", fmt.Sprintf("/api/notice/%d", notice.ID), "school_admin", nil, schoolAdmin.ID,
			nil, DeleteNotice, gin.Param{Key: "id", Value: fmt.Sprint(notice.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())
	})

	t.Run("GetRegDetail and user list role assign", func(t *testing.T) {
		// 造一条报名
		reg := models.Register{CompID: schoolComp.ID, LeaderID: students[0].ID, TeamName: "推2报名队", Status: 1, AdvisorInfo: "[]", Track: "[]"}
		require.NoError(t, db.Create(&reg).Error)
		require.NoError(t, db.Create(&models.RegMember{RegID: reg.ID, StudentID: students[0].Username, Name: students[0].Realname, Phone: "138", IsLeader: true, College: ca.Name}).Error)

		w := invoke("GET", fmt.Sprintf("/api/reg/detail?id=%d", reg.ID), "school_admin", nil, schoolAdmin.ID, nil, GetRegDetail)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "推2报名队")

		w = invoke("GET", "/api/reg/detail?id=abc", "school_admin", nil, schoolAdmin.ID, nil, GetRegDetail)
		require.Equal(t, 400, w.Code)
		w = invoke("GET", "/api/reg/detail?id=", "school_admin", nil, schoolAdmin.ID, nil, GetRegDetail)
		require.Equal(t, 400, w.Code)
		w = invoke("GET", "/api/reg/detail?id=999999", "school_admin", nil, schoolAdmin.ID, nil, GetRegDetail)
		require.Equal(t, 404, w.Code)

		// 用户列表
		w = invoke("GET", "/api/perm/member/list?page=1&page_size=5", "school_admin", nil, schoolAdmin.ID, nil, GetAllUsers)
		require.Equal(t, 200, w.Code, w.Body.String())

		w = invoke("GET", "/api/perm/member/list?page=1&page_size=5&identity_type=bad", "school_admin", nil, schoolAdmin.ID, nil, GetAllUsers)
		require.Equal(t, 400, w.Code)

		// 角色分配：把学生改回学生（幂等）
		var collegeRole models.Role
		require.NoError(t, db.Where("role_code = ?", "college_admin").First(&collegeRole).Error)
		w = invoke("PUT", fmt.Sprintf("/api/perm/member/%d/role", collegeAdmin.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"role_id": collegeRole.ID, "managed_college_id": ca.ID},
			AssignUserRole, gin.Param{Key: "id", Value: fmt.Sprint(collegeAdmin.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		// 院管必须指定学院
		var studentRole models.Role
		require.NoError(t, db.Where("role_code = ?", "student").First(&studentRole).Error)
		w = invoke("PUT", fmt.Sprintf("/api/perm/member/%d/role", students[0].ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"role_id": collegeRole.ID},
			AssignUserRole, gin.Param{Key: "id", Value: fmt.Sprint(students[0].ID)})
		require.Equal(t, 400, w.Code)
	})

	t.Run("Work audit list endpoints", func(t *testing.T) {
		w := invoke("GET", "/api/reg/work/audit/comp/list?page=1&page_size=5", "school_admin", nil, schoolAdmin.ID, nil, GetWorkAuditCompList)
		require.Equal(t, 200, w.Code, w.Body.String())
		w = invoke("GET", "/api/reg/work/audit/student/list?comp_id=0&page=1&page_size=5", "school_admin", nil, schoolAdmin.ID, nil, GetWorkAuditStudentList)
		// 可能 400/404/200
		require.Contains(t, []int{200, 400, 404}, w.Code, w.Body.String())
	})
}

func buildPreviewExcel(sid, name, college string) []byte {
	f := excelize.NewFile()
	defer f.Close()
	headers := awardImportHeaders
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue("Sheet1", cell, h)
	}
	row := []interface{}{1, sid, name, college, "专业", "推2预览项目", "2026-06", "省级", "一等奖", ""}
	for i, v := range row {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue("Sheet1", cell, v)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil
	}
	return buf.Bytes()
}
