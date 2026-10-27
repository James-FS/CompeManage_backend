package controllers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ===================== GetSummaryDetail 剩余分支 =====================

func TestGetSummaryDetail2_CompQueryError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/summary/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetSummaryDetail(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetSummaryDetail2_ScopeInvalid(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/summary/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin") // 未配置管理学院
	GetSummaryDetail(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetSummaryDetail2_SummaryQueryError(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `summaries`").WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/summary/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetSummaryDetail(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// 成功路径：经费/附件/时间范围/负责人/学院
func TestGetSummaryDetail2_WithExpenses(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	// Preload：Detail / Manager / CollegeInfo
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "comp_start_time", "comp_end_time"}).
			AddRow(1, 1, nil, time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(1, "张负责"))
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "计算机学院"))
	mock.ExpectQuery("SELECT .* FROM `summaries`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "summary_content", "expenses", "attachments"}).
			AddRow(1, 1, 1, "圆满举办",
				`[{"usage":"打印","amount":10.5},{"usage":"交通","amount":20}]`,
				`[{"name":"a.pdf","url":"/api/file/download/a"}]`))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(8))
	mock.ExpectQuery("SELECT award_level as level.*FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"level", "count", "level_rank"}).
			AddRow("省级一等奖", 2, 1).AddRow("省级二等奖", 1, 2))

	_, w, c := buildGET("/api/summary/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"summary:edit", "comp:update"})
	GetSummaryDetail(c)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "圆满举办")
	assert.Contains(t, body, "张负责")
	assert.Contains(t, body, "计算机学院")
	assert.Contains(t, body, "2026-05-01")
	assert.Contains(t, body, "30.5")
}

// ===================== SaveSummary 剩余分支 =====================

func saveSummaryBody() map[string]interface{} {
	return map[string]interface{}{
		"summary_content": "总结",
		"expenses":        []map[string]interface{}{{"usage": "打印", "amount": 3.5, "remark": ""}},
		"attachments":     []map[string]interface{}{{"name": "a.pdf", "url": "/api/file/download/a"}},
		"status":          1,
	}
}

func TestSaveSummary2_ScopeInvalid(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))

	_, w, c := buildAuthPOSTJSON(saveSummaryBody())
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin") // 未配置管理学院
	SaveSummary(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSaveSummary2_CreateError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id", "status"}).AddRow(1, 1, 1, 2))
	mock.ExpectQuery("SELECT .* FROM `summaries`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `summaries`").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(saveSummaryBody())
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"summary:edit"})
	SaveSummary(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSaveSummary2_SaveError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id", "status"}).AddRow(1, 1, 1, 2))
	mock.ExpectQuery("SELECT .* FROM `summaries`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 0))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `summaries`").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(saveSummaryBody())
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"summary:edit"})
	SaveSummary(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
