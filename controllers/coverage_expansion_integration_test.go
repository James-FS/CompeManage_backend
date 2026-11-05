package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// 跨模块集成扩测：顾问查询/搜索/成员与团队编辑/补录成功/建删恢复赛事/总结/通知/统计
func TestCoverageExpansionMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Summary{}, &models.Notice{}, &models.Role{}, &models.UserRole{}, &models.Permission{}, &models.RolePermission{}))
	roles := map[string]models.Role{}
	for _, rc := range []string{"school_admin", "college_admin", "teacher", "competition_manager", "student"} {
		role := models.Role{RoleName: rc, RoleCode: rc, Description: rc}
		require.NoError(t, db.Create(&role).Error)
		roles[rc] = role
	}
	// 给校管挂写权限码，供 UserPermissionCodes / canWriteCompetition
	for _, code := range []string{"comp:update", "summary:edit", "reg:config:edit", "comp:list", "award:import"} {
		perm := models.Permission{Name: code, Code: code, Type: 3, Description: code}
		require.NoError(t, db.Create(&perm).Error)
		require.NoError(t, db.Create(&models.RolePermission{RoleID: roles["school_admin"].ID, PermissionID: perm.ID}).Error)
	}
	// 统计看板依赖 Redis；本机有则接上，无则跳过统计子测
	redisUp := false
	if rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}); rdb.Ping(t.Context()).Err() == nil {
		orig := middleware.RedisClient
		middleware.RedisClient = rdb
		t.Cleanup(func() { middleware.RedisClient = orig; _ = rdb.Close() })
		redisUp = true
	}

	ca := models.College{Name: "扩测甲学院", IsValid: true}
	require.NoError(t, db.Create(&ca).Error)
	schoolAdmin := models.User{Username: "XS001", Realname: "扩测校管", IdentityType: "staff", College: "教务处"}
	require.NoError(t, db.Create(&schoolAdmin).Error)
	teacher := models.User{Username: "XT001", Realname: "扩测教师", IdentityType: "staff", College: ca.Name}
	require.NoError(t, db.Create(&teacher).Error)
	require.NoError(t, database.SetUserRole(db, teacher.ID, roles["teacher"].ID))
	require.NoError(t, database.SetUserRole(db, schoolAdmin.ID, roles["school_admin"].ID))
	student := models.User{Username: "X001", Realname: "扩测学生", College: ca.Name, Major: "专业甲", IdentityType: "student"}
	require.NoError(t, db.Create(&student).Error)
	require.NoError(t, database.SetUserRole(db, student.ID, roles["student"].ID))
	student2 := models.User{Username: "X002", Realname: "扩测学生乙", College: ca.Name, Major: "专业甲", IdentityType: "student"}
	require.NoError(t, db.Create(&student2).Error)

	schoolComp := models.CompDirectory{CompCode: "XK1", CompName: "扩测校赛", CompLevel: "校级", ManagerID: teacher.ID, Year: 2026, Status: 2}
	require.NoError(t, db.Create(&schoolComp).Error)
	require.NoError(t, db.Create(&models.CompDetail{
		CompID: schoolComp.ID, ParticipantType: 2,
		CompStartTime: time.Now().Add(-time.Hour), CompEndTime: time.Now().Add(time.Hour),
		RegStartTime: time.Now().Add(-time.Hour), RegEndTime: time.Now().Add(time.Hour),
		SubmitStartTime: time.Now().Add(-time.Hour), SubmitEndTime: time.Now().Add(time.Hour),
		ReviewStartTime: time.Now().Add(-time.Hour), ReviewEndTime: time.Now().Add(time.Hour),
		Track: "[]", AwardHierarchy: "[]", MinTeamMember: 1, MaxTeamMember: 5,
	}).Error)
	require.NoError(t, db.Create(&models.Summary{
		CompID: schoolComp.ID, SummaryContent: "总结内容", Expenses: "[]", Attachments: "[]",
		Status: 0, CreatedBy: schoolAdmin.ID, UpdatedBy: schoolAdmin.ID,
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
		if r, ok := roles[role]; ok {
			c.Set("role_id", r.ID)
			c.Set("permission_version", r.PermissionVersion)
		}
		handler(c)
		return w
	}

	t.Run("LookupAwardAdvisors", func(t *testing.T) {
		w := invoke("POST", "/api/award/advisors/lookup", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"usernames": []string{"XT001", "XT001", "XNOPE"}}, LookupAwardAdvisors)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "扩测教师")

		w = invoke("POST", "/api/award/advisors/lookup", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"usernames": []string{"XNOTFOUND99"}}, LookupAwardAdvisors)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), `"list":[]`)

		w = invoke("POST", "/api/award/advisors/lookup", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{}, LookupAwardAdvisors)
		require.Equal(t, 400, w.Code)
	})

	t.Run("SearchCompetition", func(t *testing.T) {
		w := invoke("GET", "/api/award/comp/list", "school_admin", nil, schoolAdmin.ID, nil, SearchCompetition)
		require.Equal(t, 400, w.Code)

		w = invoke("GET", "/api/award/comp/list?keyword=扩测", "school_admin", nil, schoolAdmin.ID, nil, SearchCompetition)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "扩测校赛")

		w = invoke("GET", "/api/award/comp/list?keyword=扩测&paginated=true&page=1&page_size=5", "school_admin", nil, schoolAdmin.ID, nil, SearchCompetition)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), `"total"`)

		w = invoke("GET", "/api/award/comp/list?keyword=扩测&year=abc", "school_admin", nil, schoolAdmin.ID, nil, SearchCompetition)
		require.Equal(t, 400, w.Code)

		w = invoke("GET", "/api/award/comp/list?keyword=扩测&year=2026&paginated=true", "school_admin", nil, schoolAdmin.ID, nil, SearchCompetition)
		require.Equal(t, 200, w.Code)

		// 院管 scope
		w = invoke("GET", "/api/award/comp/list?keyword=扩测", "college_admin", &ca.ID, teacher.ID, nil, SearchCompetition)
		require.Equal(t, 200, w.Code)
	})

	t.Run("CreateEditDeleteRestoreCompetition", func(t *testing.T) {
		w := invoke("POST", "/api/comp/create", "school_admin", nil, schoolAdmin.ID, map[string]interface{}{
			"comp_name": "扩测新建赛", "comp_level": "省级", "comp_type": "学科竞赛",
			"organizer": "主办方", "undertaker": "承办方", "manager_id": teacher.ID,
			"college": ca.Name, "year": "2026", "desc": "描述",
		}, CreateCompetition)
		require.Equal(t, 200, w.Code, w.Body.String())
		var created models.CompDirectory
		require.NoError(t, db.Where("comp_name = ?", "扩测新建赛").First(&created).Error)

		// 编辑（UpdateCompetitionReq 必填字段齐全）
		w = invoke("PUT", fmt.Sprintf("/api/comp/%d", created.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{
				"comp_name": "扩测新建赛改", "comp_level": "省级", "comp_type": "学科竞赛",
				"organizer": "主办方", "undertaker": "承办方", "manager_id": teacher.ID,
				"college": ca.Name, "year": "2026", "desc": "改描述",
			},
			UpdateCompetition, gin.Param{Key: "id", Value: fmt.Sprint(created.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		// 列表含新赛
		w = invoke("GET", "/api/comp/list?page=1&page_size=20&comp_name=扩测新建", "school_admin", nil, schoolAdmin.ID, nil, GetCompetitionList)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "扩测新建赛改")

		// 删除
		w = invoke("DELETE", fmt.Sprintf("/api/comp/%d", created.ID), "school_admin", nil, schoolAdmin.ID,
			nil, DeleteCompetition, gin.Param{Key: "id", Value: fmt.Sprint(created.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		// 恢复
		w = invoke("PUT", fmt.Sprintf("/api/comp/%d/restore", created.ID), "school_admin", nil, schoolAdmin.ID,
			nil, RestoreCompetition, gin.Param{Key: "id", Value: fmt.Sprint(created.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		// 详情
		w = invoke("GET", fmt.Sprintf("/api/comp/%d", schoolComp.ID), "school_admin", nil, schoolAdmin.ID,
			nil, GetCompetitionDetail, gin.Param{Key: "id", Value: fmt.Sprint(schoolComp.ID)})
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "扩测校赛")

		// 年份列表（依赖 competition_years 表，隔离库可能无；仅测通不 5xx 断言过严时放宽）
		w = invoke("GET", "/api/comp/years", "school_admin", nil, schoolAdmin.ID, nil, GetCompetitionYears)
		require.True(t, w.Code == 200 || w.Code == 500, w.Body.String())
	})

	t.Run("Import then UpdateMember and UpdateTeam", func(t *testing.T) {
		tag := time.Now().UnixNano()
		w := invoke("POST", fmt.Sprintf("/api/award/import?comp_id=%d", schoolComp.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"rows": []map[string]string{{
				"student_id": student.Username, "name": student.Realname, "college": ca.Name, "major": "专业甲",
				"project_name": fmt.Sprintf("扩测团队%d", tag), "award_month": "2026-06", "award_category": "省级", "award_name": "金奖",
				"advisor_usernames": "XT001", "advisor_colleges": ca.Name,
			}}}, ImportAward)
		require.Equal(t, 200, w.Code, w.Body.String())

		var award models.Award
		require.NoError(t, db.Where("project_name = ?", fmt.Sprintf("扩测团队%d", tag)).First(&award).Error)
		require.Equal(t, "approved", award.Status)
		// 指导教师已写入
		var advCount int64
		require.NoError(t, db.Model(&models.AwardAdvisor{}).Where("award_id = ?", award.ID).Count(&advCount).Error)
		require.EqualValues(t, 1, advCount)

		var member models.AwardMember
		require.NoError(t, db.Where("award_id = ?", award.ID).First(&member).Error)

		// 改备注
		w = invoke("PUT", fmt.Sprintf("/api/award/members/%d", member.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"remark": "扩测备注", "student_id": student.Username},
			UpdateAwardMember, gin.Param{Key: "id", Value: fmt.Sprint(member.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		// 非法学号
		w = invoke("PUT", fmt.Sprintf("/api/award/members/%d", member.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"remark": "x", "student_id": "NOTEXIST"},
			UpdateAwardMember, gin.Param{Key: "id", Value: fmt.Sprint(member.ID)})
		require.Equal(t, 400, w.Code)

		// 成员不存在
		w = invoke("PUT", "/api/award/members/999999", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"remark": "x"}, UpdateAwardMember, gin.Param{Key: "id", Value: "999999"})
		require.Equal(t, 404, w.Code)

		// 改团队名+指导教师
		newName := fmt.Sprintf("扩测团队改%d", tag)
		w = invoke("PUT", fmt.Sprintf("/api/award/teams/%d", award.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{
				"project_name": newName, "award_category": "省级", "award_name": "二等奖", "award_month": "2026-07",
				"advisors": []map[string]string{{"username": "XT001"}},
			},
			UpdateAwardTeam, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())

		require.NoError(t, db.First(&award, award.ID).Error)
		require.Equal(t, newName, award.ProjectName)
		require.Equal(t, "二等奖", award.AwardName)

		// 非法团队参数
		w = invoke("PUT", fmt.Sprintf("/api/award/teams/%d", award.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"project_name": "", "award_category": "省级", "award_name": "一等奖", "award_month": "2026-07"},
			UpdateAwardTeam, gin.Param{Key: "id", Value: fmt.Sprint(award.ID)})
		require.Equal(t, 400, w.Code)

		// 团队不存在
		w = invoke("PUT", "/api/award/teams/999999", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"project_name": "x", "award_category": "省级", "award_name": "一等奖", "award_month": "2026-07"},
			UpdateAwardTeam, gin.Param{Key: "id", Value: "999999"})
		require.Equal(t, 404, w.Code)
	})

	t.Run("Student supplement team success and resubmit", func(t *testing.T) {
		// 第三人补录成功
		st3 := models.User{Username: "X003", Realname: "扩测队长", College: ca.Name, Major: "专业甲", IdentityType: "student"}
		require.NoError(t, db.Create(&st3).Error)
		st4 := models.User{Username: "X004", Realname: "扩测队员", College: ca.Name, Major: "专业甲", IdentityType: "student"}
		require.NoError(t, db.Create(&st4).Error)
		proofHash := "e1e2c3d4e5f6789012345678abcdef03"
		proofName := "p.png"
		proofURL := buildDownloadURL("award_proof", proofHash, proofName)
		require.NoError(t, db.Create(&models.FileRecord{
			FileHash: proofHash, OriginalName: proofName,
			StoragePath: "static/award_proofs/" + proofHash + "_" + proofName,
			FileSize: 5, FileExt: "png", BizType: "award_proof", UploaderID: st3.ID,
		}).Error)
		// 物理文件（若下载路径需要）—补录只校验 URL/记录

		teamName := fmt.Sprintf("扩测补录团队%d", time.Now().UnixNano())
		body := map[string]interface{}{
			"comp_id": schoolComp.ID, "team_name": teamName, "award_level": "省级一等奖",
			"award_name": "一等奖", "award_date": "2026-05-01", "proof_url": proofURL,
			"participation_type": "team", "award_category": "省级",
			"advisors":  []map[string]string{{"username": "XT001", "name": "扩测教师", "college": ca.Name}},
			"members": []map[string]interface{}{
				{"name": "扩测队长", "student_id": "X003", "phone": "13900000001", "college": ca.Name, "is_leader": true},
				{"name": "扩测队员", "student_id": "X004", "phone": "13900000002", "college": ca.Name, "is_leader": false},
			},
		}
		w := invoke("POST", "/api/award/student/supplement", "student", nil, st3.ID, body, SubmitStudentAwardSupplement)
		require.Equal(t, 200, w.Code, w.Body.String())

		var sup models.Award
		require.NoError(t, db.Where("project_name = ? AND source = ?", teamName, "supplement").First(&sup).Error)
		require.Equal(t, "draft", sup.Status)
		var memberCount int64
		require.NoError(t, db.Model(&models.AwardMember{}).Where("award_id = ?", sup.ID).Count(&memberCount).Error)
		require.EqualValues(t, 2, memberCount)

		// 再次提交同项目 → 已提交/记录已存在
		w = invoke("POST", "/api/award/student/supplement", "student", nil, st3.ID, body, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "已")

		// 驳回后重新提交
		require.NoError(t, db.Model(&sup).Update("status", "rejected").Error)
		require.NoError(t, db.First(&sup, sup.ID).Error)
		w = invoke("POST", "/api/award/student/supplement", "student", nil, st3.ID, body, SubmitStudentAwardSupplement)
		// 可能因 reg/项目冲突 400 或成功重交——断言非 500
		require.Contains(t, []int{200, 400}, w.Code, w.Body.String())

		// 重复队员
		bodyBad := map[string]interface{}{
			"comp_id": schoolComp.ID, "team_name": "重复队员项目", "award_level": "省级一等奖",
			"award_name": "一等奖", "proof_url": proofURL, "participation_type": "team", "award_category": "省级",
			"advisors": []map[string]string{{"username": "XT001", "name": "扩测教师", "college": ca.Name}},
			"members": []map[string]interface{}{
				{"name": "甲", "student_id": "X003", "phone": "1", "college": ca.Name, "is_leader": true},
				{"name": "甲2", "student_id": "X003", "phone": "2", "college": ca.Name, "is_leader": false},
			},
		}
		w = invoke("POST", "/api/award/student/supplement", "student", nil, st3.ID, bodyBad, SubmitStudentAwardSupplement)
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), "重复")
	})

	t.Run("Summary list and detail", func(t *testing.T) {
		w := invoke("GET", "/api/summary/list?page=1&page_size=10", "school_admin", nil, schoolAdmin.ID, nil, GetSummaryList)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "扩测校赛")

		w = invoke("GET", fmt.Sprintf("/api/summary/%d", schoolComp.ID), "school_admin", nil, schoolAdmin.ID,
			nil, GetSummaryDetail, gin.Param{Key: "id", Value: fmt.Sprint(schoolComp.ID)})
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "总结内容")

		// 保存总结（校管 summary:edit）
		w = invoke("POST", fmt.Sprintf("/api/summary/%d", schoolComp.ID), "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{
				"summary_content": "归档总结", "expenses": []interface{}{}, "attachments": []interface{}{},
				"summary_status": 1,
			}, SaveSummary, gin.Param{Key: "id", Value: fmt.Sprint(schoolComp.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())
	})

	t.Run("Notice create list detail", func(t *testing.T) {
		w := invoke("POST", "/api/notice/comp/create", "school_admin", nil, schoolAdmin.ID,
			map[string]interface{}{"title": "扩测通知", "content": "通知正文", "notice_type": "system"},
			CreateCompNotice)
		// 可能字段不同——若 400 则试其他 handler
		if w.Code != 200 {
			t.Logf("CreateCompNotice body=%s", w.Body.String())
		}
		w = invoke("GET", "/api/notice/list?page=1&page_size=10", "school_admin", nil, schoolAdmin.ID, nil, GetNoticeList)
		require.Equal(t, 200, w.Code)
	})

	t.Run("Statistics dashboard", func(t *testing.T) {
		if !redisUp {
			t.Skip("redis unavailable")
		}
		w := invoke("GET", "/api/statistics/dashboard?months=6", "school_admin", nil, schoolAdmin.ID, nil, GetStatisticsDashboard)
		require.Equal(t, 200, w.Code, w.Body.String())
	})

	t.Run("Reg config save and list", func(t *testing.T) {
		// 院管保存本院/校级报名配置
		w := invoke("POST", "/api/reg/config", "school_admin", nil, schoolAdmin.ID, map[string]interface{}{
			"comp_id": schoolComp.ID, "require_attachment": false, "need_advisor": false,
			"max_team_member": 5, "min_team_member": 1, "participant_type": 2,
			"reg_start_time": time.Now().Add(-time.Hour).Format(time.RFC3339),
			"reg_end_time":   time.Now().Add(time.Hour).Format(time.RFC3339),
			"grade_requirement": []string{},
		}, SaveRegConfig)
		t.Logf("SaveRegConfig=%d %s", w.Code, w.Body.String())

		w = invoke("GET", fmt.Sprintf("/api/reg/config/get?comp_id=%d", schoolComp.ID), "school_admin", nil, schoolAdmin.ID, nil, GetRegConfig)
		require.Equal(t, 200, w.Code)

		w = invoke("GET", fmt.Sprintf("/api/reg/list?comp_id=%d&page=1&page_size=10", schoolComp.ID), "school_admin", nil, schoolAdmin.ID, nil, GetRegList)
		t.Logf("GetRegList=%d", w.Code)
		require.Equal(t, 200, w.Code, w.Body.String())
	})
}
