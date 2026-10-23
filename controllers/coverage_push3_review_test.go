package controllers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ===================== updateTaskStatus =====================

// updateTaskStatus 鏃犱换浣曡褰曟椂璧?else 鍒嗘敮锛坰tatus=2 璇勫涓級
func TestUpdateTaskStatus_NoRecords(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_tasks` SET `status`").
		WithArgs(2, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx := database.DB.Begin()
	updateTaskStatus(tx, 1)
	_ = tx.Commit()
}

// updateTaskStatus 鍏ㄩ儴璇勫瀹屾垚鏃剁疆 status=3 骞跺啓 completed_at
func TestUpdateTaskStatus_AllReviewed(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_tasks`").
		WithArgs(3, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx := database.DB.Begin()
	updateTaskStatus(tx, 1)
	_ = tx.Commit()
}

// updateTaskStatus 閮ㄥ垎璇勫瀹屾垚鏃朵粛涓?status=2
func TestUpdateTaskStatus_PartiallyReviewed(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_tasks` SET `status`").
		WithArgs(2, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx := database.DB.Begin()
	updateTaskStatus(tx, 1)
	_ = tx.Commit()
}

// ===================== AutoCreateReviewRecordsForWork =====================

// 璧涗簨鏈紑鍚瘎瀹★細comp_details 鏌ヨ澶辫触鐩存帴杩斿洖
func TestAutoCreateReviewRecords_ReviewNotEnabled(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnError(fmt.Errorf("record not found"))

	AutoCreateReviewRecordsForWork(database.DB, 1, 1)
}

// 赛事开启评审但未分配任务：无任务直接返回
func TestAutoCreateReviewRecords_NoTasks(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}))

	AutoCreateReviewRecordsForWork(database.DB, 1, 1)
}

// 报名记录缺少作品附件：在任务查询后返回
func TestAutoCreateReviewRecords_NoWorkAttachment(t *testing.T) {
	mock := setupReviewDBMock(t)

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).AddRow(1, 1, 10, 0))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnError(fmt.Errorf("record not found"))

	AutoCreateReviewRecordsForWork(database.DB, 1, 100)
}

// 璁板綍宸插瓨鍦細璺宠繃鍒涘缓锛屼笉浜х敓鍐欏叆
func TestAutoCreateReviewRecords_SkipsExistingRecord(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).AddRow(1, 1, 10, 0))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "work_attachment_url"}).
			AddRow(100, 1, "/uploads/work.pdf"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "status"}).
			AddRow(1, 1, 100, 0))
	mock.ExpectCommit()

	AutoCreateReviewRecordsForWork(database.DB, 1, 100)
}

// 璁板綍涓嶅瓨鍦細鍒涘缓骞舵妸 task.Status 0 鈫?1锛堟柊璁板綍瑙﹀彂鐘舵€佹祦杞級
func TestAutoCreateReviewRecords_CreatesRecordAndActivatesTask(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).AddRow(1, 1, 10, 0))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "work_attachment_url"}).
			AddRow(100, 1, "/uploads/work.pdf"))
	mock.ExpectBegin()
	// 璁板綍涓嶅瓨鍦?鈫?鍒涘缓
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnError(fmt.Errorf("record not found"))
	mock.ExpectExec("INSERT INTO `review_records`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// task.Status == 0 鈫?缃负 1
	mock.ExpectExec("UPDATE `review_tasks` SET `status`").
		WithArgs(1, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	AutoCreateReviewRecordsForWork(database.DB, 1, 100)
}

// 宸插畬鎴?status=3)鐨勪换鍔″湪鏂板璁板綍鍚庡洖閫€鍒?status=2
func TestAutoCreateReviewRecords_CompletedTaskReopensToReviewing(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).AddRow(1, 1, 10, 3))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "work_attachment_url"}).
			AddRow(100, 1, "/uploads/work.pdf"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnError(fmt.Errorf("record not found"))
	mock.ExpectExec("INSERT INTO `review_records`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// task.Status == 3 鈫?鍥為€€涓?2
	mock.ExpectExec("UPDATE `review_tasks` SET `status`").
		WithArgs(2, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	AutoCreateReviewRecordsForWork(database.DB, 1, 100)
}

// 鍒涘缓璁板綍澶辫触锛氬洖婊氬苟杩斿洖
func TestAutoCreateReviewRecords_CreateRecordFailsRollsBack(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).AddRow(1, 1, 10, 0))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "work_attachment_url"}).
			AddRow(100, 1, "/uploads/work.pdf"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnError(fmt.Errorf("record not found"))
	mock.ExpectExec("INSERT INTO `review_records`").
		WillReturnError(fmt.Errorf("insert failed"))
	mock.ExpectRollback()

	AutoCreateReviewRecordsForWork(database.DB, 1, 100)
}

// 任务已在评审中(status=1)：创建记录但不改任务状态
func TestAutoCreateReviewRecords_TaskAlreadyReviewingKeepsStatus(t *testing.T) {
	mock := setupReviewDBMock(t)

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).AddRow(1, 1, 10, 1))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "work_attachment_url"}).
			AddRow(100, 1, "/uploads/work.pdf"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnError(fmt.Errorf("record not found"))
	mock.ExpectExec("INSERT INTO `review_records`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// status=1 鏃舵棤闇€ UPDATE
	mock.ExpectCommit()

	AutoCreateReviewRecordsForWork(database.DB, 1, 100)
}

// ===================== InitReviewTasks 鏉冮檺涓庢垚鍔熻矾寰?=====================

// 鏃?role_code锛堢粫杩?AuthRequired 鐨勬祴璇曪級鏃惰烦杩囦綔鐢ㄥ煙鏍￠獙锛岃繘鍏ュ悗缁€昏緫
func TestInitReviewTasks_SkipsScopeCheckWithoutRoleCode(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}))

	body := map[string]interface{}{"comp_id": 1}
	_, w, c := buildReviewPOSTJSON(body)
	InitReviewTasks(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "请先在报名设置中分配评审专家")
}

// 院级管理员访问非本学院赛事被拒绝
func TestInitReviewTasks_CollegeAdminForbiddenForOtherCollege(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	// requireUserAccessScope 从 role_code 读；comp 查询后判定无权
	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 99, 7))

	body := map[string]interface{}{"comp_id": 1}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "college_admin")
	managedCollegeID := uint(3)
	c.Set("managed_college_id", &managedCollegeID)
	c.Set("user_id", uint(5))
	InitReviewTasks(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无权访问该赛事")
}

// 闄㈢骇绠＄悊鍛樻湭閰嶇疆绠＄悊瀛﹂櫌锛氱洿鎺?403
func TestInitReviewTasks_CollegeAdminWithoutManagedCollege(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"comp_id": 1}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "college_admin")
	c.Set("user_id", uint(5))
	InitReviewTasks(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "尚未配置管理学院")
}

// 全流程成功：新建记录、任务 0->1、返回新增条数
func TestInitReviewTasks_Success(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).AddRow(1, 1, 10, 0))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "work_attachment_url"}).
			AddRow(100, 1, 1, "/uploads/a.pdf"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnError(fmt.Errorf("record not found"))
	mock.ExpectExec("INSERT INTO `review_records`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// task.Status == 0 鈫?1
	mock.ExpectExec("UPDATE `review_tasks` SET `status`").
		WithArgs(1, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"comp_id": 1}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	InitReviewTasks(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"new_records\":1")
	assert.Contains(t, w.Body.String(), "评审任务初始化完成")
}

// 记录已存在全部跳过：new_records=0 且不改任务状态
func TestInitReviewTasks_AllRecordsAlreadyExist(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).AddRow(1, 1, 10, 0))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "work_attachment_url"}).
			AddRow(100, 1, 1, "/uploads/a.pdf"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "status"}).
			AddRow(1, 1, 100, 0))
	// 鏃犳柊澧?鈫?涓?UPDATE task
	mock.ExpectCommit()

	body := map[string]interface{}{"comp_id": 1}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	InitReviewTasks(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"new_records\":0")
}

// 宸插畬鎴愪换鍔?status=3)鍦ㄦ湁鏂板璁板綍鏃跺洖閫€涓?2
func TestInitReviewTasks_CompletedTaskReopensWhenNewRecordAdded(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).AddRow(1, 1, 10, 3))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "work_attachment_url"}).
			AddRow(100, 1, 1, "/uploads/a.pdf"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnError(fmt.Errorf("record not found"))
	mock.ExpectExec("INSERT INTO `review_records`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE `review_tasks` SET `status`").
		WithArgs(2, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"comp_id": 1}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	InitReviewTasks(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"new_records\":1")
}

// 鍒涘缓璁板綍澶辫触 鈫?500
func TestInitReviewTasks_CreateRecordFails(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_review"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "expert_id", "status"}).AddRow(1, 1, 10, 0))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "work_attachment_url"}).
			AddRow(100, 1, 1, "/uploads/a.pdf"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnError(fmt.Errorf("record not found"))
	mock.ExpectExec("INSERT INTO `review_records`").
		WillReturnError(fmt.Errorf("insert failed"))
	mock.ExpectRollback()

	body := map[string]interface{}{"comp_id": 1}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	InitReviewTasks(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// 赛事不存在：comp_details 查询失败（有 role_code 时作用域查询先失败）
func TestInitReviewTasks_ScopedCompNotFound(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnError(fmt.Errorf("record not found"))

	body := map[string]interface{}{"comp_id": 999}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	InitReviewTasks(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在或未开启专家评审")
}

// ===================== ConfirmReviewResult 成功路径 =====================

// 完整成功路径：解析排名、按名额生成获奖记录
func TestConfirmReviewResult_Success(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	pastEnd := time.Now().Add(-24 * time.Hour)

	// 赛事配置
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_end_time", "award_hierarchy"}).
			AddRow(1, 1, pastEnd, "[\"一等奖\",\"二等奖\"]"))
	// 未评审记录数 = 0
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// 已存在获奖数 = 0
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// 已完成评审记录（两条不同 reg，用于排名）
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "status"}).
			AddRow(1, 1, 100, 10, 1, 95.0, 1).
			AddRow(2, 1, 200, 10, 1, 88.5, 1))
	// 作品信息
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "update_time"}).
			AddRow(100, 1, time.Now()).
			AddRow(200, 1, time.Now()))
	// comp_level
	mock.ExpectQuery("SELECT `comp_level` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_level"}).AddRow("校级"))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `awards`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO `awards`").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{
		"comp_id": 1,
		"award_counts": []map[string]interface{}{
			{"level": "一等奖", "count": 1},
			{"level": "二等奖", "count": 1},
		},
	}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	ConfirmReviewResult(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "获奖名单已生成")
	assert.Contains(t, w.Body.String(), "\"total_awards\":2")
}

// 名额多于作品数：自动缩减，只生成实际作品数
func TestConfirmReviewResult_AwardCountsExceedWorks(t *testing.T) {
	mock := setupReviewDBMock(t)

	pastEnd := time.Now().Add(-24 * time.Hour)

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_end_time", "award_hierarchy"}).
			AddRow(1, 1, pastEnd, "[\"一等奖\",\"二等奖\"]"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// 只有一件作品
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "status"}).
			AddRow(1, 1, 100, 10, 1, 95.0, 1))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "update_time"}).
			AddRow(100, 1, time.Now()))
	mock.ExpectQuery("SELECT `comp_level` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_level"}).AddRow("校级"))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `awards`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{
		"comp_id": 1,
		"award_counts": []map[string]interface{}{
			{"level": "一等奖", "count": 1},
			{"level": "二等奖", "count": 3},
		},
	}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	ConfirmReviewResult(c)

	assert.Equal(t, http.StatusOK, w.Code)
	// 名额从 3 缩减到 0，只生成 1 条
	assert.Contains(t, w.Body.String(), "\"total_awards\":1")

}
// 无已完成评审记录：不生成任何获奖
func TestConfirmReviewResult_NoReviewedRecords(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	pastEnd := time.Now().Add(-24 * time.Hour)

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_end_time", "award_hierarchy"}).
			AddRow(1, 1, pastEnd, "[\"一等奖\"]"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// 没有任何已完成记录
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "status"}))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "update_time"}))
	mock.ExpectQuery("SELECT `comp_level` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_level"}).AddRow("校级"))
	mock.ExpectBegin()
	mock.ExpectCommit()

	body := map[string]interface{}{
		"comp_id": 1,
		"award_counts": []map[string]interface{}{
			{"level": "一等奖", "count": 1},
		},
	}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	ConfirmReviewResult(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total_awards\":0")
}

// 鍒涘缓鑾峰璁板綍澶辫触 鈫?500
func TestConfirmReviewResult_CreateAwardFails(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	pastEnd := time.Now().Add(-24 * time.Hour)

	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_end_time", "award_hierarchy"}).
			AddRow(1, 1, pastEnd, "[\"涓€绛夊\"]"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "status"}).
			AddRow(1, 1, 100, 10, 1, 95.0, 1))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "update_time"}).
			AddRow(100, 1, time.Now()))
	mock.ExpectQuery("SELECT `comp_level` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_level"}).AddRow("鏍＄骇"))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `awards`").
		WillReturnError(fmt.Errorf("insert failed"))
	mock.ExpectRollback()

	body := map[string]interface{}{
		"comp_id": 1,
		"award_counts": []map[string]interface{}{
			{"level": "涓€绛夊", "count": 1},
		},
	}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	ConfirmReviewResult(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// 院级管理员越权访问他院赛事
func TestConfirmReviewResult_CollegeAdminForbidden(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 99, 7))

	body := map[string]interface{}{
		"comp_id": 1,
		"award_counts": []map[string]interface{}{
			{"level": "一等奖", "count": 1},
		},
	}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("role_code", "college_admin")
	managedCollegeID := uint(3)
	c.Set("managed_college_id", &managedCollegeID)
	c.Set("user_id", uint(5))
	ConfirmReviewResult(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无权访问该赛事")
}

// ===================== SubmitReview / UpdateReview 成功路径 =====================

// SubmitReview 成功：写入分数并更新任务状态
func TestSubmitReview_Success(t *testing.T) {
	mock := setupReviewDBMock(t)

	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "status"}).
			AddRow(1, 1, 100, 10, 1, 0))
	// 鏃堕棿绐楀彛鏍￠獙閫氳繃
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_start_time", "review_end_time"}).
			AddRow(1, 1, now.Add(-time.Hour), now.Add(time.Hour)))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_records`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// updateTaskStatus
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectExec("UPDATE `review_tasks`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"record_id": 1, "score": 92.5, "comment": "寰堝ソ"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("user_id", uint(10))
	SubmitReview(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "评审提交成功")
}

// UpdateReview 成功：保留原始分数字段
func TestUpdateReview_Success(t *testing.T) {
	mock := setupReviewDBMock(t)

	now := time.Now()
	originalScore := 88.0
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "comment", "status", "original_score", "original_comment"}).
			AddRow(1, 1, 100, 10, 1, originalScore, "原评语", 1, nil, nil))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_start_time", "review_end_time"}).
			AddRow(1, 1, now.Add(-time.Hour), now.Add(time.Hour)))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_records`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"score": 95.0, "comment": "修改后评语"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(10))
	UpdateReview(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "评审修改成功")
}

// UpdateReview 已有原始分数字段时不覆盖
func TestUpdateReview_KeepsExistingOriginalValues(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()
	originalScore := 88.0
	originalComment := "首次评语"
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "comment", "status", "original_score", "original_comment"}).
			AddRow(1, 1, 100, 10, 1, originalScore, "第二次评语", 1, originalScore, originalComment))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_start_time", "review_end_time"}).
			AddRow(1, 1, now.Add(-time.Hour), now.Add(time.Hour)))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_records`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"score": 95.0, "comment": "第三次评语"}
	_, w, c := buildReviewPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(10))
	UpdateReview(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "评审修改成功")
}

// SubmitReview 时 comp_details 查询失败：不阻断提交（容错分支）
func TestSubmitReview_CompDetailLookupErrorDoesNotBlock(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "status"}).
			AddRow(1, 1, 100, 10, 1, 0))
	// 时间窗口查询失败 -> 跳过校验
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `review_records`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("UPDATE `review_tasks`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"record_id": 1, "score": 80.0, "comment": ""}
	_, w, c := buildReviewPOSTJSON(body)
	c.Set("user_id", uint(10))
	SubmitReview(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// 缂撳瓨娓呯悊鍑芥暟锛氭棤 Redis 鏃朵笉 panic锛堣鐩?4 涓崰浣嶅嚱鏁帮級
func TestReviewCacheClearHelpers_NoRedis(t *testing.T) {
	// clearReview*Cache 鍧囦负鍗犱綅瀹炵幇锛屼粎楠岃瘉鍙畨鍏ㄨ皟鐢?	clearReviewCompListCache(t.Context())
	clearReviewProgressCache(t.Context(), 1)
	clearReviewResultCache(t.Context(), 1)
	clearMyTasksCache(t.Context(), 1)
}




