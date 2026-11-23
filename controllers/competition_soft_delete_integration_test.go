package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"CompeManage_backend/middleware"
	"CompeManage_backend/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSoftDeletedCompetitionListsAndStatisticsMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.CompDeclaration{}, &models.Summary{}, &models.ReviewTask{}, &models.ReviewRecord{}))
	originalRedis := middleware.RedisClient
	middleware.RedisClient = nil
	t.Cleanup(func() { middleware.RedisClient = originalRedis })

	admin := models.User{Username: "cleanup-admin", Realname: "测试管理者"}
	reviewer := models.User{Username: "cleanup-reviewer", Realname: "测试评审员"}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&reviewer).Error)

	now := time.Now()
	createCompetition := func(code, name string) models.CompDirectory {
		comp := models.CompDirectory{CompCode: code, CompName: name, Status: 2, ManagerID: admin.ID}
		require.NoError(t, db.Create(&comp).Error)
		detail := models.CompDetail{
			CompID: comp.ID, NeedReview: 1, Track: "[]", AwardHierarchy: "[]",
			CompStartTime: now.Add(-48 * time.Hour), CompEndTime: now.Add(-time.Hour),
			RegStartTime: now.Add(-72 * time.Hour), RegEndTime: now.Add(-48 * time.Hour),
			SubmitStartTime: now.Add(-48 * time.Hour), SubmitEndTime: now.Add(-2 * time.Hour),
			ReviewStartTime: now.Add(-2 * time.Hour), ReviewEndTime: now.Add(-time.Hour),
		}
		require.NoError(t, db.Create(&detail).Error)
		reg := models.Register{CompID: comp.ID, LeaderID: admin.ID, Status: 1, AdvisorInfo: "[]", WorkAttachmentUrl: "/static/work.zip"}
		require.NoError(t, db.Create(&reg).Error)
		award := models.Award{CompID: comp.ID, RegID: reg.ID, Status: "approved", Source: "review", AdvisorInfo: "[]"}
		require.NoError(t, db.Create(&award).Error)
		task := models.ReviewTask{CompID: comp.ID, ExpertID: reviewer.ID, AssignedBy: admin.ID}
		require.NoError(t, db.Create(&task).Error)
		require.NoError(t, db.Create(&models.Summary{CompID: comp.ID, Status: 1, Expenses: "[]", Attachments: "[]"}).Error)
		return comp
	}
	active := createCompetition("CLEANUP_ACTIVE", "保留赛事")
	deleted := createCompetition("CLEANUP_DELETED", "已删除赛事")
	require.NoError(t, db.Delete(&deleted).Error)
	removedReg := models.Register{
		CompID: active.ID, LeaderID: reviewer.ID, Status: 1, AdvisorInfo: "[]", WorkAttachmentUrl: "/static/removed.zip",
	}
	require.NoError(t, db.Create(&removedReg).Error)
	require.NoError(t, db.Delete(&removedReg).Error)
	require.NoError(t, db.Where("comp_id = ?", active.ID).Delete(&models.Summary{}).Error)

	type response struct {
		Data struct {
			Total   int              `json:"total"`
			List    []map[string]any `json:"list"`
			Summary map[string]any   `json:"summary"`
			Rows    []map[string]any `json:"competition_stats"`
		} `json:"data"`
	}
	invoke := func(path, role string, userID uint, handler gin.HandlerFunc) response {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, path, nil)
		c.Set("user_id", userID)
		c.Set("role_code", role)
		c.Set("permission_codes", []string{"summary:edit"})
		handler(c)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var result response
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		require.NotContains(t, w.Body.String(), deleted.CompName)
		return result
	}

	work := invoke("/api/reg/work/audit/comp/list", "school_admin", admin.ID, GetWorkAuditCompList)
	require.Equal(t, 1, work.Data.Total)
	require.Len(t, work.Data.List, 1)
	require.EqualValues(t, 1, work.Data.List[0]["submit_count"])
	workDetail := invoke(fmt.Sprintf("/api/reg/work/audit/student/list?comp_id=%d", active.ID), "school_admin", admin.ID, GetWorkAuditStudentList)
	require.Equal(t, 1, workDetail.Data.Total)

	review := invoke("/api/review/comp/list", "school_admin", admin.ID, GetReviewCompList)
	require.Equal(t, 1, review.Data.Total)
	require.Len(t, review.Data.List, 1)
	require.EqualValues(t, active.ID, review.Data.List[0]["comp_id"])
	tasks := invoke("/api/review/my/tasks", "expert", reviewer.ID, GetMyReviewTasks)
	require.Equal(t, 1, tasks.Data.Total)
	require.Len(t, tasks.Data.List, 1)
	require.EqualValues(t, active.ID, tasks.Data.List[0]["comp_id"])

	summary := invoke("/api/summary/list?summary_status=0", "school_admin", admin.ID, GetSummaryList)
	require.Equal(t, 1, summary.Data.Total)
	require.Len(t, summary.Data.List, 1)
	require.EqualValues(t, 0, summary.Data.List[0]["summary_status"])

	stats := invoke("/api/statistics/dashboard?months=6", "school_admin", admin.ID, GetStatisticsDashboard)
	require.EqualValues(t, 1, stats.Data.Summary["total_competitions"])
	require.EqualValues(t, 1, stats.Data.Summary["total_registrations"])
	require.EqualValues(t, 1, stats.Data.Summary["total_awards"])
	require.EqualValues(t, 0, stats.Data.Summary["summary_archive_rate"])
	require.Len(t, stats.Data.Rows, 1)
	require.EqualValues(t, 1, stats.Data.Rows[0]["reg_count"])
}
