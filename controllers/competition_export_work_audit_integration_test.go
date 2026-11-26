package controllers

import (
	"CompeManage_backend/models"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestCompetitionExportDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	var output bytes.Buffer
	writer := competitionExportWriter{ctx: ctx, writer: &output}
	_, err := writer.Write([]byte("export data"))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, output.Bytes())

	_, w, c := buildGET("/api/comp/export?year=2026")
	respondCompetitionExportError(c, ctx, err)
	require.Equal(t, 504, w.Code)
	require.Contains(t, w.Body.String(), "赛事目录导出超时")
}

func TestCompetitionExportAndWorkAuditMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	colleges := []models.College{{Name: "赛事学院", IsValid: true}, {Name: "教师学院", IsValid: true}, {Name: "来源学院", IsValid: true}}
	require.NoError(t, db.Create(&colleges).Error)
	users := []models.User{
		{Username: "EXP001", Realname: "赛事负责人", College: colleges[1].Name},
		{Username: "EXP002", Realname: "参赛学生", College: colleges[0].Name},
		{Username: "EXP003", Realname: "另一学生", College: colleges[0].Name},
	}
	require.NoError(t, db.Create(&users).Error)
	comps := make([]models.CompDirectory, 2003)
	for i := range comps {
		comps[i] = models.CompDirectory{
			CompCode: fmt.Sprintf("EX2026%04d", i+1), CompName: fmt.Sprintf("导出赛事 %04d", i+1),
			CompLevel: "省级", CollegeID: &colleges[0].ID, ManagerID: users[0].ID, Year: 2026, Status: 1, ProjectOrigin: models.ProjectOrigin{ProjectSource: colleges[2].Name, ProjectSourceCollegeID: &colleges[2].ID},
		}
	}
	require.NoError(t, db.CreateInBatches(&comps, 200).Error)
	require.NoError(t, db.Delete(&comps[0]).Error)
	draft := models.CompDirectory{CompCode: "EXP-DRAFT", CompName: "导出赛事 待补充", CollegeID: &colleges[0].ID, Year: 2026, Status: 1}
	require.NoError(t, db.Omit("ManagerID").Create(&draft).Error)
	other := models.CompDirectory{CompCode: "EXP-OTHER", CompName: "导出赛事 其他学院", CollegeID: &colleges[1].ID, ManagerID: users[1].ID, Year: 2026, Status: 1, ProjectOrigin: models.ProjectOrigin{ProjectSource: colleges[0].Name, ProjectSourceCollegeID: &colleges[0].ID}}
	require.NoError(t, db.Create(&other).Error)
	otherYear := models.CompDirectory{CompCode: "EXP-2025", CompName: "导出赛事 其他年份", CollegeID: &colleges[0].ID, ManagerID: users[0].ID, Year: 2025, Status: 1}
	require.NoError(t, db.Create(&otherYear).Error)

	invoke := func(handler gin.HandlerFunc, path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", path, nil)
		c.Set("user_id", users[0].ID)
		c.Set("role_code", "college_admin")
		c.Set("managed_college_id", &colleges[2].ID)
		handler(c)
		require.Equal(t, 200, w.Code)
		return w
	}

	t.Run("export exceeds the old limit without changing columns or scope", func(t *testing.T) {
		w := invoke(ExportCompetitionDirectory, "/api/comp/export?year=2026&status=ongoing&comp_name="+url.QueryEscape("导出赛事"))
		require.Contains(t, w.Header().Get("Content-Disposition"), url.PathEscape("赛事列表_2026年度.xlsx"))
		book, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
		require.NoError(t, err)
		defer book.Close()
		rows, err := book.GetRows("赛事列表")
		require.NoError(t, err)
		require.Len(t, rows, 2003)
		require.Equal(t, []string{"赛事编号", "赛事名称", "赛事级别", "主办单位", "承办单位", "赛事负责人", "所属单位", "项目来源", "状态"}, rows[0])
		require.Equal(t, comps[len(comps)-1].CompCode, rows[1][0])
		require.Equal(t, "教师学院", rows[1][6])
		require.Equal(t, "来源学院", rows[1][7])
		require.Equal(t, comps[1].CompCode, rows[len(rows)-1][0])

		filtered := invoke(ExportCompetitionDirectory, "/api/comp/export?year=2026&college="+url.QueryEscape(colleges[0].Name))
		require.Contains(t, filtered.Header().Get("Content-Type"), "application/json")
		require.Contains(t, filtered.Body.String(), "暂无数据可导出")
	})

	t.Run("work audit counts only submissions for the visible page", func(t *testing.T) {
		past := time.Now().Add(-time.Hour)
		eligible := []uint{comps[1].ID, comps[2].ID, comps[2].ID, other.ID}
		for _, id := range eligible {
			detail := models.CompDetail{
				CompID: id, CompStartTime: past, CompEndTime: past,
				RegStartTime: past, RegEndTime: past, SubmitStartTime: past, SubmitEndTime: past,
				ReviewStartTime: past, ReviewEndTime: past, Track: "[]", AwardHierarchy: "[]",
			}
			require.NoError(t, db.Create(&detail).Error)
		}
		deleted := models.Register{CompID: comps[2].ID, LeaderID: users[2].ID, WorkAttachmentUrl: "removed", AdvisorInfo: "[]"}
		require.NoError(t, db.Create(&deleted).Error)
		require.NoError(t, db.Delete(&deleted).Error)
		require.NoError(t, db.Create(&[]models.Register{
			{CompID: comps[2].ID, LeaderID: users[0].ID, WorkAttachmentUrl: "one", AdvisorInfo: "[]"},
			{CompID: comps[2].ID, LeaderID: users[1].ID, AdvisorInfo: "[]"},
			{CompID: comps[1].ID, LeaderID: users[1].ID, WorkAttachmentUrl: "two", AdvisorInfo: "[]"},
		}).Error)
		type item struct {
			CompID      uint   `json:"comp_id"`
			SubmitCount int    `json:"submit_count"`
			CollegeName string `json:"college_name"`
		}
		for page, id := range []uint{comps[2].ID, comps[1].ID} {
			w := invoke(GetWorkAuditCompList, fmt.Sprintf("/api/reg/work/audit/comp/list?page=%d&page_size=1&college=%s", page+1, url.QueryEscape(colleges[1].Name)))
			var response struct {
				Data struct {
					Total int
					List  []item
				}
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.Equal(t, 2, response.Data.Total)
			require.Equal(t, []item{{CompID: id, SubmitCount: 1, CollegeName: colleges[1].Name}}, response.Data.List)
		}
	})

	t.Run("user management keeps substring matches and page totals", func(t *testing.T) {
		_, w, c := buildGET("/api/perm/member/list?page=2&page_size=1&search=" + url.QueryEscape(colleges[0].Name))
		c.Set("role_code", "school_admin")
		c.Set("user_id", users[0].ID)
		GetAllUsers(c)
		require.Equal(t, 200, w.Code, w.Body.String())
		var response struct {
			Data struct {
				Total int
				List  []UserWithRoleResp
			}
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Equal(t, 2, response.Data.Total)
		require.Len(t, response.Data.List, 1)
		require.Equal(t, users[1].ID, response.Data.List[0].ID)
	})
}
