package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupReviewDBMock(t *testing.T) sqlmock.Sqlmock {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	assert.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	assert.NoError(t, err)
	database.DB = gormDB
	return mock
}

func buildReviewPOSTJSON(body interface{}) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
	jsonBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/", bytes.NewBuffer(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return req, w, c
}

// ===================== GetReviewCompList =====================

func TestGetReviewCompList_CountError(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/review/comp/list")
	GetReviewCompList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestGetReviewCompList_SuccessEmpty(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Count query
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Select query (empty result)
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id", "comp_name", "comp_level", "need_review", "review_start_time", "review_end_time"}))

	_, w, c := buildGET("/api/review/comp/list?page=1&size=10")
	GetReviewCompList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":0")
}

func TestGetReviewCompList_SuccessWithData(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()

	// Count query
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// Select query
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id", "comp_name", "comp_level", "need_review", "review_start_time", "review_end_time"}).
			AddRow(1, "数学建模大赛", "校级", 1, now, now))

	// Per-item queries for compID=1
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(15))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// calcReviewStatus queries
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 1, 2).
			AddRow(2, 1, 2, 2).
			AddRow(3, 1, 3, 3))

	_, w, c := buildGET("/api/review/comp/list?page=1&size=10")
	GetReviewCompList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "数学建模大赛")
	assert.Contains(t, w.Body.String(), "\"total\":1")
}

// ===================== GetExpertList =====================

func TestGetExpertList_CountError(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/review/expert/list")
	GetExpertList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestGetExpertList_SuccessEmpty(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "name", "college"}))

	_, w, c := buildGET("/api/review/expert/list?page=1&size=10")
	GetExpertList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":0")
}

func TestGetExpertList_WithKeywordAndCollege(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "name", "college"}).
			AddRow(1, "expert01", "张教授", "计算机学院"))

	_, w, c := buildGET("/api/review/expert/list?keyword=张&college_id=1")
	GetExpertList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "张教授")
}

// ===================== GetReviewTaskList =====================

func TestGetReviewTaskList_MissingCompID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/task/list")
	GetReviewTaskList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "缺少 comp_id 参数")
}

func TestGetReviewTaskList_InvalidCompID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/task/list?comp_id=abc")
	GetReviewTaskList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "comp_id 参数格式错误")
}

func TestGetReviewTaskList_SuccessEmpty(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Count tasks
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Find tasks (empty)
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}))

	_, w, c := buildGET("/api/review/task/list?comp_id=1&page=1&size=10")
	GetReviewTaskList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":0")
}

func TestGetReviewTaskList_SuccessWithData(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()

	// Count tasks
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// Find tasks
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status", "assigned_at", "completed_at"}).
			AddRow(1, 1, 10, 2, now, nil))

	// Per-task queries
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username"}).
			AddRow(10, "张教授", "expert01"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(8))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

	_, w, c := buildGET("/api/review/task/list?comp_id=1&page=1&size=10")
	GetReviewTaskList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "张教授")
	assert.Contains(t, w.Body.String(), "\"total\":1")
}

// ===================== AssignReviewTask =====================

func TestAssignReviewTask_BindingError(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{} // missing required fields
	_, w, c := buildReviewPOSTJSON(body)
	AssignReviewTask(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestAssignReviewTask_Unauthorized(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"comp_id": 1, "expert_ids": []uint{1, 2}}
	_, w, c := buildReviewPOSTJSON(body)
	// no user_id set
	AssignReviewTask(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestAssignReviewTask_CompNotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnError(fmt.Errorf("record not found"))

	body := map[string]interface{}{"comp_id": 999, "expert_ids": []uint{1}}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("user_id", uint(1))
	AssignReviewTask(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在或未开启专家评审")
}

func TestAssignReviewTask_Success(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Check comp detail exists
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).
			AddRow(1, 1, 1))

	// syncReviewTasks: Find existing tasks
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}))

	// Create new task (expert_id=1) - transaction
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `review_tasks`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Create new task (expert_id=2) - transaction
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `review_tasks`").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"comp_id": 1, "expert_ids": []uint{1, 2}}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("user_id", uint(1))
	AssignReviewTask(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "专家分配已更新")
}

// ===================== InitReviewTasks =====================

func TestInitReviewTasks_BindingError(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{} // missing comp_id
	_, w, c := buildReviewPOSTJSON(body)
	InitReviewTasks(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestInitReviewTasks_CompNotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnError(fmt.Errorf("record not found"))

	body := map[string]interface{}{"comp_id": 999}
	_, w, c := buildReviewPOSTJSON(body)
	InitReviewTasks(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在或未开启专家评审")
}

func TestInitReviewTasks_NoTasks(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Comp detail found
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).
			AddRow(1, 1, 1))
	// No review tasks
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}))

	body := map[string]interface{}{"comp_id": 1}
	_, w, c := buildReviewPOSTJSON(body)
	InitReviewTasks(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "请先在报名设置中分配评审专家")
}

func TestInitReviewTasks_NoSubmittedWorks(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Comp detail found
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).
			AddRow(1, 1, 1))
	// Review tasks exist
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 0))
	// No submitted works
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}))

	body := map[string]interface{}{"comp_id": 1}
	_, w, c := buildReviewPOSTJSON(body)
	InitReviewTasks(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "该赛事尚无已提交作品")
}

// ===================== DeleteReviewTask =====================

func TestDeleteReviewTask_InvalidID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/task/abc")
	c.Params = []gin.Param{{Key: "id", Value: "abc"}}
	DeleteReviewTask(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数格式错误")
}

func TestDeleteReviewTask_NotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/review/task/999")
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	DeleteReviewTask(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "任务不存在")
}

func TestDeleteReviewTask_ConflictWithoutForce(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 2)) // status=2 (reviewing)

	_, w, c := buildGET("/api/review/task/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	DeleteReviewTask(c)

	assert.Equal(t, 409, w.Code)
	assert.Contains(t, w.Body.String(), "确认强制删除")
}

func TestDeleteReviewTask_ForceDelete(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 2))

	// Transaction: soft delete records + soft delete task
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_records` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectExec("UPDATE `review_tasks` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	_, w, c := buildGET("/api/review/task/1?force=true")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	DeleteReviewTask(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "删除成功")
}

func TestDeleteReviewTask_DeleteWithoutForce_StatusZero(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 0)) // status=0, no records

	// Transaction: soft delete records + soft delete task
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_records` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE `review_tasks` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	_, w, c := buildGET("/api/review/task/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	DeleteReviewTask(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "删除成功")
}

// ===================== GetReviewProgress =====================

func TestGetReviewProgress_MissingCompID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/progress")
	GetReviewProgress(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "缺少 comp_id 参数")
}

func TestGetReviewProgress_InvalidCompID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/progress?comp_id=abc")
	GetReviewProgress(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "comp_id 参数格式错误")
}

func TestGetReviewProgress_CompNotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/review/progress?comp_id=999")
	GetReviewProgress(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在")
}

func TestGetReviewProgress_SuccessEmpty(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Comp found
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "测试赛事"))
	// Count queries
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Find tasks (empty)
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}))

	_, w, c := buildGET("/api/review/progress?comp_id=1")
	GetReviewProgress(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "测试赛事")
	assert.Contains(t, w.Body.String(), "\"total_works\":0")
}

// ===================== GetReviewResultList =====================

func TestGetReviewResultList_MissingCompID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/result/list")
	GetReviewResultList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "缺少 comp_id 参数")
}

func TestGetReviewResultList_InvalidCompID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/result/list?comp_id=abc")
	GetReviewResultList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "comp_id 参数格式错误")
}

func TestGetReviewResultList_CompNotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/review/result/list?comp_id=999")
	GetReviewResultList(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在")
}

func TestGetReviewResultList_SuccessEmpty(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Comp found
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "测试赛事"))
	// CompDetail
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_end_time"}).
			AddRow(1, 1, time.Time{}))
	// Count queries
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Award count
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Find records (empty)
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "comment", "status"}))

	_, w, c := buildGET("/api/review/result/list?comp_id=1")
	GetReviewResultList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "测试赛事")
	assert.Contains(t, w.Body.String(), "\"total\":0")
}

func TestGetReviewResultList_SuccessWithAnomalyDetection(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	score1 := 90.0
	score2 := 55.0 // spread = 35 >= 30, should be flagged

	// Comp found
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "测试赛事"))
	// CompDetail
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_end_time"}).
			AddRow(1, 1, time.Time{}))
	// Count queries
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	// Award count
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Find records (2 records for same reg, different scores)
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "comment", "status"}).
			AddRow(1, 1, 100, 10, 1, score1, "优秀", 1).
			AddRow(2, 2, 100, 11, 1, score2, "一般", 1))

	// Expert name lookups
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(10, "张教授"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(11, "李教授"))

	// Register Preload
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_name", "leader_id"}).
			AddRow(100, "梦之队", 1))
	// Preload Members
	mock.ExpectQuery("SELECT .* FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id", "name", "is_leader"}).
			AddRow(1, 100, "王同学", true))

	_, w, c := buildGET("/api/review/result/list?comp_id=1")
	GetReviewResultList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"is_abnormal\":true")
	assert.Contains(t, w.Body.String(), "梦之队")
}

// ===================== ConfirmReviewResult =====================

func TestConfirmReviewResult_BindingError(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{} // missing comp_id
	_, w, c := buildReviewPOSTJSON(body)
	ConfirmReviewResult(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestConfirmReviewResult_DetailNotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnError(fmt.Errorf("record not found"))

	body := map[string]interface{}{
		"comp_id": 999,
		"award_counts": []map[string]interface{}{
			{"level": "一等奖", "count": 1},
		},
	}
	_, w, c := buildReviewPOSTJSON(body)
	ConfirmReviewResult(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "赛事配置不存在")
}

func TestConfirmReviewResult_ReviewNotEnded(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	futureTime := time.Now().Add(24 * time.Hour)

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_end_time"}).
			AddRow(1, 1, futureTime))

	body := map[string]interface{}{
		"comp_id": 1,
		"award_counts": []map[string]interface{}{
			{"level": "一等奖", "count": 1},
		},
	}
	_, w, c := buildReviewPOSTJSON(body)
	ConfirmReviewResult(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "评审尚未结束")
}

func TestConfirmReviewResult_UnreviewedRecords(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	pastTime := time.Now().Add(-24 * time.Hour)

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_end_time"}).
			AddRow(1, 1, pastTime))
	// Unreviewed count > 0
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	body := map[string]interface{}{
		"comp_id": 1,
		"award_counts": []map[string]interface{}{
			{"level": "一等奖", "count": 1},
		},
	}
	_, w, c := buildReviewPOSTJSON(body)
	ConfirmReviewResult(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "未完成评审")
}

func TestConfirmReviewResult_AlreadyHasAward(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	pastTime := time.Now().Add(-24 * time.Hour)

	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_end_time"}).
			AddRow(1, 1, pastTime))
	// Unreviewed count = 0
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Existing award > 0
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	body := map[string]interface{}{
		"comp_id": 1,
		"award_counts": []map[string]interface{}{
			{"level": "一等奖", "count": 1},
		},
	}
	_, w, c := buildReviewPOSTJSON(body)
	ConfirmReviewResult(c)

	assert.Equal(t, 409, w.Code)
	assert.Contains(t, w.Body.String(), "已生成获奖名单")
}

// ===================== GetMyReviewTasks =====================

func TestGetMyReviewTasks_Unauthorized(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/my/tasks")
	GetMyReviewTasks(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestGetMyReviewTasks_SuccessEmpty(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Count
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Find tasks (empty)
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}))

	_, w, c := buildGET("/api/review/my/tasks?page=1&size=10")
	c.Set("user_id", uint(10))
	GetMyReviewTasks(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":0")
}

func TestGetMyReviewTasks_SuccessWithData(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()

	// Count
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// Find tasks
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status", "assigned_at", "completed_at"}).
			AddRow(1, 1, 10, 2, now, nil))

	// Per-task: CompDirectory
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name", "comp_level"}).
			AddRow(1, "数学建模大赛", "校级"))
	// Per-task: CompDetail
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_start_time", "review_end_time"}).
			AddRow(1, 1, now, now))
	// Per-task: total works count
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))
	// Per-task: reviewed count
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))

	_, w, c := buildGET("/api/review/my/tasks?page=1&size=10")
	c.Set("user_id", uint(10))
	GetMyReviewTasks(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "数学建模大赛")
	assert.Contains(t, w.Body.String(), "\"total\":1")
}

// ===================== GetMyReviewWorks =====================

func TestGetMyReviewWorks_Unauthorized(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/my/works?task_id=1")
	GetMyReviewWorks(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestGetMyReviewWorks_MissingTaskID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/my/works")
	c.Set("user_id", uint(10))
	GetMyReviewWorks(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "缺少 task_id 参数")
}

func TestGetMyReviewWorks_InvalidTaskID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/my/works?task_id=abc")
	c.Set("user_id", uint(10))
	GetMyReviewWorks(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "task_id 参数格式错误")
}

func TestGetMyReviewWorks_TaskNotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/review/my/works?task_id=999")
	c.Set("user_id", uint(10))
	GetMyReviewWorks(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "任务不存在")
}

func TestGetMyReviewWorks_SuccessEmpty(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Task found
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 1))
	// Count records
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Find records (empty)
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "status"}))

	_, w, c := buildGET("/api/review/my/works?task_id=1&page=1&size=10")
	c.Set("user_id", uint(10))
	GetMyReviewWorks(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":0")
}

// ===================== GetReviewWorkDetail =====================

func TestGetReviewWorkDetail_Unauthorized(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/my/works/1?task_id=1")
	GetReviewWorkDetail(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestGetReviewWorkDetail_InvalidRegID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/my/works/abc?task_id=1")
	c.Params = []gin.Param{{Key: "regId", Value: "abc"}}
	c.Set("user_id", uint(10))
	GetReviewWorkDetail(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数格式错误")
}

func TestGetReviewWorkDetail_MissingTaskID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/my/works/1")
	c.Params = []gin.Param{{Key: "regId", Value: "1"}}
	c.Set("user_id", uint(10))
	GetReviewWorkDetail(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "缺少 task_id 参数")
}

func TestGetReviewWorkDetail_InvalidTaskID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/review/my/works/1?task_id=abc")
	c.Params = []gin.Param{{Key: "regId", Value: "1"}}
	c.Set("user_id", uint(10))
	GetReviewWorkDetail(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "task_id 参数格式错误")
}

func TestGetReviewWorkDetail_TaskNotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/review/my/works/1?task_id=999")
	c.Params = []gin.Param{{Key: "regId", Value: "1"}}
	c.Set("user_id", uint(10))
	GetReviewWorkDetail(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "任务不存在")
}

func TestGetReviewWorkDetail_RecordNotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Task found
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 1))
	// Record not found
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/review/my/works/1?task_id=1")
	c.Params = []gin.Param{{Key: "regId", Value: "1"}}
	c.Set("user_id", uint(10))
	GetReviewWorkDetail(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "评审记录不存在")
}

func TestGetReviewWorkDetail_Success(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()

	// Task found
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 1))
	// Record found
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "comment", "status", "reviewed_at"}).
			AddRow(1, 1, 100, 10, 1, nil, "", 0, nil))
	// Register with Preload Members
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_name", "leader_id", "work_attachment_url", "create_time", "update_time"}).
			AddRow(100, "梦之队", 1, "http://example.com/file.pdf", now, now))
	mock.ExpectQuery("SELECT .* FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id", "name", "student_id", "is_leader"}).
			AddRow(1, 100, "王同学", "2022001", true))
	// CompDirectory
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name", "comp_level"}).
			AddRow(1, "数学建模大赛", "校级"))

	_, w, c := buildGET("/api/review/my/works/100?task_id=1")
	c.Params = []gin.Param{{Key: "regId", Value: "100"}}
	c.Set("user_id", uint(10))
	GetReviewWorkDetail(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "梦之队")
	assert.Contains(t, w.Body.String(), "数学建模大赛")
	assert.Contains(t, w.Body.String(), "王同学")
}

// ===================== SubmitReview =====================

func TestSubmitReview_BindingError(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{} // missing required fields
	_, w, c := buildReviewPOSTJSON(body)
	SubmitReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestSubmitReview_Unauthorized(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"record_id": 1, "score": 85.0, "comment": "不错"}
	_, w, c := buildReviewPOSTJSON(body)
	SubmitReview(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestSubmitReview_ScoreOutOfRange_Negative(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"record_id": 1, "score": -1.0}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("user_id", uint(10))
	SubmitReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "分数必须在 0-100 之间")
}

func TestSubmitReview_ScoreOutOfRange_Over100(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"record_id": 1, "score": 101.0}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("user_id", uint(10))
	SubmitReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "分数必须在 0-100 之间")
}

func TestSubmitReview_RecordNotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := map[string]interface{}{"record_id": 999, "score": 85.0, "comment": "不错"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("user_id", uint(10))
	SubmitReview(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "评审记录不存在")
}

func TestSubmitReview_AlreadyReviewed(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "status"}).
			AddRow(1, 1, 100, 10, 1, 1)) // status=1, already reviewed

	body := map[string]interface{}{"record_id": 1, "score": 85.0, "comment": "不错"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("user_id", uint(10))
	SubmitReview(c)

	assert.Equal(t, 409, w.Code)
	assert.Contains(t, w.Body.String(), "该作品已评审")
}

func TestSubmitReview_ReviewNotStarted(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	futureStart := time.Now().Add(24 * time.Hour)

	// Record found, status=0
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "status"}).
			AddRow(1, 1, 100, 10, 1, 0))
	// CompDetail with future start time
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_start_time", "review_end_time"}).
			AddRow(1, 1, futureStart, time.Time{}))

	body := map[string]interface{}{"record_id": 1, "score": 85.0, "comment": "不错"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("user_id", uint(10))
	SubmitReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "评审尚未开始")
}

func TestSubmitReview_ReviewEnded(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	pastEnd := time.Now().Add(-24 * time.Hour)

	// Record found, status=0
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "status"}).
			AddRow(1, 1, 100, 10, 1, 0))
	// CompDetail with past end time
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_start_time", "review_end_time"}).
			AddRow(1, 1, time.Time{}, pastEnd))

	body := map[string]interface{}{"record_id": 1, "score": 85.0, "comment": "不错"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("user_id", uint(10))
	SubmitReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "评审已结束")
}

// ===================== UpdateReview =====================

func TestUpdateReview_InvalidID(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"score": 90.0, "comment": "修改"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "abc"}}
	c.Set("user_id", uint(10))
	UpdateReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数格式错误")
}

func TestUpdateReview_BindingError(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{} // missing score
	_, w, c := buildReviewPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(10))
	UpdateReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestUpdateReview_Unauthorized(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"score": 90.0, "comment": "修改"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateReview(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

func TestUpdateReview_ScoreOutOfRange(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"score": -5.0, "comment": "修改"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(10))
	UpdateReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "分数必须在 0-100 之间")
}

func TestUpdateReview_RecordNotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := map[string]interface{}{"score": 90.0, "comment": "修改"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	c.Set("user_id", uint(10))
	UpdateReview(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "评审记录不存在")
}

func TestUpdateReview_NotYetReviewed(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "comment", "status"}).
			AddRow(1, 1, 100, 10, 1, nil, "", 0)) // status=0, not reviewed

	body := map[string]interface{}{"score": 90.0, "comment": "修改"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(10))
	UpdateReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "该作品尚未评审")
}

func TestUpdateReview_ReviewNotStarted(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	futureStart := time.Now().Add(24 * time.Hour)
	score := 85.0

	// Record found, status=1 (already reviewed)
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "comment", "status"}).
			AddRow(1, 1, 100, 10, 1, score, "原始评语", 1))
	// CompDetail with future start
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_start_time", "review_end_time"}).
			AddRow(1, 1, futureStart, time.Time{}))

	body := map[string]interface{}{"score": 90.0, "comment": "修改"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(10))
	UpdateReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "评审尚未开始")
}

func TestUpdateReview_ReviewEnded(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	pastEnd := time.Now().Add(-24 * time.Hour)
	score := 85.0

	// Record found, status=1
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "comment", "status"}).
			AddRow(1, 1, 100, 10, 1, score, "原始评语", 1))
	// CompDetail with past end
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_start_time", "review_end_time"}).
			AddRow(1, 1, time.Time{}, pastEnd))

	body := map[string]interface{}{"score": 90.0, "comment": "修改"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(10))
	UpdateReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "评审已结束")
}

// ===================== calcReviewStatus =====================

func TestCalcReviewStatus_NoTasks(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}))

	_, _, c := buildGET("/test")
	result := calcReviewStatus(c, 1, 0, 0)
	assert.Equal(t, "uninit", result)
}

func TestCalcReviewStatus_AllZero(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 0).
			AddRow(2, 1, 11, 0))

	_, _, c := buildGET("/test")
	result := calcReviewStatus(c, 1, 0, 0)
	assert.Equal(t, "uninit", result)
}

func TestCalcReviewStatus_AllOne(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 1).
			AddRow(2, 1, 11, 1))

	_, _, c := buildGET("/test")
	result := calcReviewStatus(c, 1, 10, 0)
	assert.Equal(t, "pending", result)
}

func TestCalcReviewStatus_AllThree(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 3).
			AddRow(2, 1, 11, 3))

	_, _, c := buildGET("/test")
	result := calcReviewStatus(c, 1, 10, 10)
	assert.Equal(t, "completed", result)
}

func TestCalcReviewStatus_Mixed(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 2).
			AddRow(2, 1, 11, 3))

	_, _, c := buildGET("/test")
	result := calcReviewStatus(c, 1, 10, 7)
	assert.Equal(t, "reviewing", result)
}

// ===================== syncReviewTasks =====================

func TestSyncReviewTasks_AddNewTasks(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// No existing tasks
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}))

	// Create task for expert 1
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `review_tasks`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Create task for expert 2
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `review_tasks`").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	_, _, c := buildGET("/test")
	c.Set("user_id", uint(1))
	now := time.Now()
	warnings := syncReviewTasks(c, 1, []uint{1, 2}, 1, now, false, "")
	assert.Empty(t, warnings)
}

func TestSyncReviewTasks_RemoveUninitTask(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Existing task with status=0 (uninit)
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 0))

	// Soft delete the task
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_tasks` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	_, _, c := buildGET("/test")
	c.Set("user_id", uint(1))
	now := time.Now()
	// expert_ids is empty, so expert 10 should be removed
	warnings := syncReviewTasks(c, 1, []uint{}, 1, now, false, "")
	assert.Empty(t, warnings)
}

func TestSyncReviewTasks_SkipInitTask(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Existing task with status=2 (reviewing)
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 2))

	// User lookup for warning message
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(10, "张教授"))

	_, _, c := buildGET("/test")
	c.Set("user_id", uint(1))
	now := time.Now()
	// expert_ids doesn't include 10, but task has status>=1
	warnings := syncReviewTasks(c, 1, []uint{}, 1, now, false, "")
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "张教授")
	assert.Contains(t, warnings[0], "已跳过删除")
}

func TestSyncReviewTasks_ForceCloseReview(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Existing tasks
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 2).
			AddRow(2, 1, 11, 1))

	// Soft delete records for task 1
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_records` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectCommit()

	// Soft delete task 1
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_tasks` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// Soft delete records for task 2
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_records` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	// Soft delete task 2
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_tasks` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	_, _, c := buildGET("/test")
	c.Set("user_id", uint(1))
	now := time.Now()
	warnings := syncReviewTasks(c, 1, []uint{}, 1, now, true, "true")
	assert.Empty(t, warnings)
}

// ===================== GetReviewCompList page/size defaults =====================

func TestGetReviewCompList_DefaultPageSize(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Count query
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Select query with default page=1, size=10
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id", "comp_name", "comp_level", "need_review", "review_start_time", "review_end_time"}))

	// page=0 should default to 1, size=0 should default to 10
	_, w, c := buildGET("/api/review/comp/list?page=0&size=0")
	GetReviewCompList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== GetExpertList page/size defaults =====================

func TestGetExpertList_DefaultPageSize(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "name", "college"}))

	_, w, c := buildGET("/api/review/expert/list?page=0&size=200") // size>100 should default to 10
	GetExpertList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== GetMyReviewTasks page/size defaults =====================

func TestGetMyReviewTasks_DefaultPageSize(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}))

	_, w, c := buildGET("/api/review/my/tasks?page=-1&size=0")
	c.Set("user_id", uint(10))
	GetMyReviewTasks(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== GetMyReviewWorks with status filter =====================

func TestGetMyReviewWorks_WithStatusFilter(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// Task found
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 1))
	// Count with status filter
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	// Find records with status filter
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "status"}))

	_, w, c := buildGET("/api/review/my/works?task_id=1&status=0")
	c.Set("user_id", uint(10))
	GetMyReviewWorks(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":3")
}

// ===================== GetMyReviewWorks success with data =====================

func TestGetMyReviewWorks_SuccessWithData(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()

	// Task found
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).
			AddRow(1, 1, 10, 2))
	// Count records
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// Find records
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "comment", "status", "reviewed_at"}).
			AddRow(1, 1, 100, 10, 1, nil, "", 0, nil))

	// Register with Preload Members
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_name", "leader_id", "work_attachment_url", "create_time", "update_time"}).
			AddRow(100, "梦之队", 1, "http://example.com/work.pdf", now, now))
	mock.ExpectQuery("SELECT .* FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id", "name", "student_id", "is_leader"}).
			AddRow(1, 100, "王同学", "2022001", true).
			AddRow(2, 100, "李同学", "2022002", false))

	_, w, c := buildGET("/api/review/my/works?task_id=1&page=1&size=10")
	c.Set("user_id", uint(10))
	GetMyReviewWorks(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "梦之队")
	assert.Contains(t, w.Body.String(), "王同学")
	assert.Contains(t, w.Body.String(), "\"total\":1")
}
