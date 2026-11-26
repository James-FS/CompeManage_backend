package controllers

import (
	"CompeManage_backend/models"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPaginatedAwardStatisticsMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Summary{}, &models.CompDeclaration{}))
	colleges := []models.College{{Name: "甲学院", IsValid: true}, {Name: "乙学院", IsValid: true}}
	require.NoError(t, db.Create(&colleges).Error)
	students := []models.User{
		{Username: "PAGE001", Realname: "学生甲", College: colleges[0].Name},
		{Username: "PAGE002", Realname: "学生乙", College: colleges[1].Name},
		{Username: "PAGE003", Realname: "不同拼写学生", College: "甲学院 "},
	}
	require.NoError(t, db.Create(&students).Error)
	comps := []models.CompDirectory{
		{CompCode: "PAGE1", CompName: "项目赛事甲", CompLevel: "省级", CollegeID: &colleges[0].ID, ManagerID: students[0].ID, ProjectOrigin: models.ProjectOrigin{ProjectSource: colleges[0].Name, ProjectSourceCollegeID: &colleges[0].ID}},
		{CompCode: "PAGE2", CompName: "项目赛事乙", CompLevel: "校级", CollegeID: &colleges[1].ID, ManagerID: students[1].ID, ProjectOrigin: models.ProjectOrigin{ProjectSource: colleges[1].Name, ProjectSourceCollegeID: &colleges[1].ID}},
		{CompCode: "PAGE3", CompName: "无奖赛事", CollegeID: &colleges[0].ID, ManagerID: students[0].ID, ProjectOrigin: models.ProjectOrigin{ProjectSource: colleges[0].Name, ProjectSourceCollegeID: &colleges[0].ID}},
	}
	require.NoError(t, db.Create(&comps).Error)
	reg := models.Register{CompID: comps[0].ID, LeaderID: students[0].ID, TeamName: "旧项目", AdvisorInfo: "[]"}
	require.NoError(t, db.Create(&reg).Error)
	require.NoError(t, db.Create(&models.RegMember{
		RegID: reg.ID, Name: "登记队长", StudentID: students[0].Username, College: colleges[0].Name, IsLeader: true,
	}).Error)
	awards := []models.Award{
		{CompID: comps[0].ID, ProjectName: "智能项目", AwardCategory: "国际级", AwardName: "（金 奖）", Status: "approved", Source: "supplement",
			AdvisorInfo: `[{"name":"旧教师甲","college":"甲学院"},{"name":"旧教师乙","college":"乙学院"},{"name":"配对教师","college":"甲学院"},{"name":"不可见教师","college":"甲学院 "}]`},
		{CompID: comps[0].ID, ProjectName: "另一项目", AwardCategory: "校赛", AwardName: "铜奖", Status: "approved", Source: "supplement", AdvisorInfo: "[]"},
		{CompID: comps[0].ID, RegID: reg.ID, AwardName: "旧奖项", Status: "draft", Source: "supplement", AdvisorInfo: "[]"},
		{CompID: comps[1].ID, ProjectName: "乙院项目", Status: "approved", Source: "import", AdvisorInfo: "[]"},
	}
	require.NoError(t, db.Create(&awards).Error)
	require.NoError(t, db.Create(&[]models.AwardMember{
		{AwardID: awards[0].ID, StudentID: students[0].ID, Name: "本院队员", StudentNumber: students[0].Username, College: colleges[0].Name},
		{AwardID: awards[0].ID, StudentID: students[1].ID, Name: "跨院队长", StudentNumber: students[1].Username, College: colleges[1].Name, IsLeader: true},
		{AwardID: awards[0].ID, StudentID: students[2].ID, Name: "不同拼写学生", StudentNumber: students[2].Username, College: students[2].College, IsLeader: true},
		{AwardID: awards[1].ID, StudentID: students[0].ID, Name: "本院队长", StudentNumber: students[0].Username, College: colleges[0].Name, IsLeader: true},
		{AwardID: awards[3].ID, StudentID: students[1].ID, Name: "乙院队员", StudentNumber: students[1].Username, College: colleges[1].Name},
	}).Error)
	require.NoError(t, db.Create(&models.AwardAdvisor{
		AwardID: awards[0].ID, Username: "TEACH_A", Name: "新教师", College: colleges[0].Name,
	}).Error)

	invoke := func(handler gin.HandlerFunc, path, role string) json.RawMessage {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", path, nil)
		c.Set("user_id", students[0].ID)
		c.Set("role_code", role)
		if role == "college_admin" {
			c.Set("managed_college_id", &colleges[0].ID)
		}
		handler(c)
		require.Equal(t, 200, w.Code, w.Body.String())
		var payload struct{ Data json.RawMessage }
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
		return payload.Data
	}
	t.Run("audit uses visible leader and paginates matches", func(t *testing.T) {
		base := "/api/award/audit/list?page_size=1&keyword="
		for _, tc := range []struct {
			keyword string
			role    string
			total   int
		}{
			{"跨院队长", "college_admin", 0},
			{"不同拼写学生", "college_admin", 1},
			{"跨院队长", "school_admin", 0},
			{"不同拼写学生", "school_admin", 1},
			{"本院", "college_admin", 1},
			{"登记队长", "college_admin", 1},
			{"page001", "college_admin", 0},
		} {
			var result struct {
				Total int
				List  []AwardAuditListItem
			}
			require.NoError(t, json.Unmarshal(invoke(GetAwardAuditList, base+url.QueryEscape(tc.keyword), tc.role), &result))
			require.Equal(t, tc.total, result.Total, tc.keyword)
			if tc.total > 0 {
				require.Len(t, result.List, 1)
				require.Contains(t, result.List[0].StudentName, tc.keyword)
			}
		}
	})

	t.Run("award detail counts and searches all members of a managed source", func(t *testing.T) {
		base := fmt.Sprintf("/api/award/comp-awards?comp_id=%d", comps[0].ID)
		type result struct {
			TeamCount    int `json:"team_count"`
			StudentCount int `json:"student_count"`
			List         []struct {
				ID      uint
				CanEdit bool `json:"can_edit_team"`
				Members []struct{ Name string }
			}
		}
		var first, second, filtered result
		require.NoError(t, json.Unmarshal(invoke(GetCompAwards, base+"&page=1&page_size=1", "school_admin"), &first))
		require.NoError(t, json.Unmarshal(invoke(GetCompAwards, base+"&page=2&page_size=1", "school_admin"), &second))
		require.Equal(t, 2, first.TeamCount)
		require.Equal(t, 4, first.StudentCount)
		require.Len(t, first.List, 1)
		require.Len(t, second.List, 1)
		require.NotEqual(t, first.List[0].ID, second.List[0].ID)

		filter := base + "&page=1&category=" + url.QueryEscape("国家级") + "&level=" + url.QueryEscape("一等奖")
		for _, keyword := range []string{"本院队员", "teach_a", "旧教师甲、旧教师乙", "跨院队长", "不同拼写学生", "不可见教师 新教师"} {
			require.NoError(t, json.Unmarshal(invoke(GetCompAwards, filter+"&keyword="+url.QueryEscape(keyword), "college_admin"), &filtered))
			require.Equal(t, 1, filtered.TeamCount, keyword)
			require.Equal(t, 3, filtered.StudentCount)
			require.Len(t, filtered.List, 1)
			require.Len(t, filtered.List[0].Members, 3)
			require.Equal(t, "本院队员", filtered.List[0].Members[0].Name)
			require.True(t, filtered.List[0].CanEdit)
		}
	})

	t.Run("dashboard returns overview and paged scope independently", func(t *testing.T) {
		require.NoError(t, db.Model(&students[0]).Update("college", "工程训练中心").Error)
		base := "/api/statistics/dashboard/competitions"
		var result struct {
			Total int
			List  []competitionStatItem
		}
		require.NoError(t, json.Unmarshal(invoke(GetDashboardCompetitionStats, base+"?page=2&page_size=1", "college_admin"), &result))
		require.Equal(t, 2, result.Total)
		require.Len(t, result.List, 1)
		require.Equal(t, comps[2].ID, result.List[0].ID)
		require.Equal(t, "工程训练中心", result.List[0].CollegeName)
		require.NoError(t, json.Unmarshal(invoke(GetDashboardCompetitionStats, base+"?college="+url.QueryEscape("工程训练中心"), "college_admin"), &result))
		require.Equal(t, 2, result.Total)
		require.NoError(t, json.Unmarshal(invoke(GetDashboardCompetitionStats, base+"?ranking=true", "college_admin"), &result))
		require.Equal(t, 1, result.Total)
		require.Len(t, result.List, 1)
		require.Equal(t, int64(4), result.List[0].AwardCount)
		overview := invoke(GetStatisticsDashboard, "/api/statistics/dashboard?include_competition_stats=false", "college_admin")
		require.NotContains(t, string(overview), "competition_stats")
		require.Contains(t, string(overview), "competition_colleges")
		require.Contains(t, string(overview), "工程训练中心")
		require.Contains(t, string(overview), "\"total_competitions\":2")
	})
}
