package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ===================== GetRegConfig 剩余分支 =====================

func TestGetRegConfig_CapsQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "赛事A"))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(1, 1))

	_, w, c := buildGET("/api/reg/config/get?comp_id=1")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("role_id", uint(1))
	c.Set("permission_version", 1) // 无 permission_codes → 走 DB 并报错
	GetRegConfig(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetRegConfig_EmptyConfigFields(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "赛事A"))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(1, 1))

	_, w, c := buildGET("/api/reg/config/get?comp_id=1")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"comp:update"})
	GetRegConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetRegConfig_InvalidTrackJSON(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "赛事A"))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "track", "grade_requirement", "award_hierarchy"}).
			AddRow(1, 1, "not-json", "[]", "[]"))

	_, w, c := buildGET("/api/reg/config/get?comp_id=1")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"comp:update"})
	GetRegConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetRegConfig_ExpertsBranch(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "赛事A"))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id"}).AddRow(1, 1, 10))
	mock.ExpectQuery("SELECT id, realname, username FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username"}).AddRow(10, "何专家", "E001"))

	_, w, c := buildGET("/api/reg/config/get?comp_id=1")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"comp:update"})
	GetRegConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "何专家")
}

// ===================== GetStatisticsDashboard 剩余分支 =====================

func dashboardCountOK(mock sqlmock.Sqlmock, n int) {
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(n)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(n)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(n)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(n)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(n)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(n)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(n)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(n)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(n)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `summaries`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(n)))
}

func TestGetStatisticsDashboard_MonthsClampAndFallbacks(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	dashboardCountOK(mock, 0)
	// 分布扫描失败 → 回落空数组
	mock.ExpectQuery("SELECT COALESCE\\(comp_level").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectQuery("SELECT COALESCE\\(users.college").
		WillReturnError(fmt.Errorf("db error"))
	// 月度统计（reg/award 月计数）
	for i := 0; i < 6; i++ {
		mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	}
	// 学院排名 / 级别统计等尾部查询
	for i := 0; i < 8; i++ {
		mock.ExpectQuery("(?s)SELECT .*").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	}

	managed := uint(3)
	_, w, c := buildGET("/api/statistics/dashboard?months=99")
	c.Set("user_id", uint(5))
	c.Set("role_code", "college_admin")
	c.Set("managed_college_id", &managed)
	GetStatisticsDashboard(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetStatisticsDashboard_CompetitionsCountError(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/statistics/dashboard")
	c.Set("user_id", uint(5))
	c.Set("role_code", "school_admin")
	GetStatisticsDashboard(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetStatisticsDashboard_RegistrationsCountError(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/statistics/dashboard")
	c.Set("user_id", uint(5))
	c.Set("role_code", "school_admin")
	GetStatisticsDashboard(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== GetCompetitionList 剩余分支 =====================

func TestGetCompetitionList_ScopeError(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/comp/list?page=1&page_size=10&is_my=true")
	c.Set("user_id", uint(5))
	c.Set("role_code", "college_admin") // 未配置管理学院
	GetCompetitionList(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetCompetitionList_CountError(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/comp/list?page=1&page_size=10&is_reg=true")
	c.Set("user_id", uint(5))
	c.Set("role_code", "school_admin")
	GetCompetitionList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== UpdateDeclare 负责人改派（promote） =====================

func TestUpdateDeclare_ManagerChangePromotesTeacher(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()
	ensureRedisForUserList(t).FlushDB(t.Context())

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status", "manager_id", "college_id"}).AddRow(1, 0, 4, 3))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// promote：teacher → competition_manager
	mock.ExpectQuery("SELECT \\* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(5, 3))
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(3, "teacher", "老师"))
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(4, "competition_manager", "赛事负责人"))
	mock.ExpectExec("DELETE FROM `user_roles`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_roles`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	// finalizeRolePromotions：审计（独立事务）+ promotedUsersResp
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `user_role_audits`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT `id`,`username`,`realname` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname"}).AddRow(5, "T005", "王老师"))

	body := map[string]interface{}{"manager_id": 5}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	UpdateDeclare(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "更新成功")
}
