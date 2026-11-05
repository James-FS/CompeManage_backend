package controllers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"CompeManage_backend/middleware"

	"github.com/redis/go-redis/v9"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)


// ensureRedisForUserList 为 GetUserList 注入可用的 Redis 客户端（Get 会命中 miss，异步写入被丢弃）
func ensureRedisForUserList(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	if err := rdb.Ping(t.Context()).Err(); err != nil {
		_ = rdb.Close()
		t.Skip("redis 不可用，跳过 GetUserList 缓存相关断言")
	}
	previous := middleware.RedisClient
	middleware.RedisClient = rdb
	t.Cleanup(func() { middleware.RedisClient = previous; _ = rdb.Close() })
	return rdb
}

// ===================== GetUserList =====================

// 正常列表：按角色取人（默认分支）
func TestGetUserList_SuccessByRole(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	ensureRedisForUserList(t).FlushDB(t.Context())

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college", "grade"}).
			AddRow(1, "张三", "S001", "计算机学院", "2024"))

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=student")
	GetUserList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "张三")
	assert.Contains(t, w.Body.String(), "\"total\":1")
}

// teacher 走身份口径（identity_type='staff' 且排除管理员）
func TestGetUserList_TeacherIdentityBranch(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	ensureRedisForUserList(t).FlushDB(t.Context())

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college", "grade"}).
			AddRow(2, "李老师", "T002", "计算机学院", "教职员工"))

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=teacher")
	GetUserList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "李老师")
}

// award_advisor 分支：额外查询角色并回填 role_code
func TestGetUserList_AwardAdvisorBranch(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	ensureRedisForUserList(t).FlushDB(t.Context())

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college", "grade"}).
			AddRow(3, "王指导", "T003", "数学学院", "教职员工"))
	// 角色回填查询
	mock.ExpectQuery("SELECT user_roles.user_id AS user_id, roles.role_code AS role_code FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_code"}).
			AddRow(3, "teacher"))

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=award_advisor")
	GetUserList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "王指导")
	assert.Contains(t, w.Body.String(), "teacher")
}

// award_advisor 且用户无角色：回退默认 teacher
func TestGetUserList_AwardAdvisorDefaultRoleFallback(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	ensureRedisForUserList(t).FlushDB(t.Context())

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college", "grade"}).
			AddRow(4, "赵指导", "T004", "外语学院", "教职员工"))
	// 无角色行
	mock.ExpectQuery("SELECT user_roles.user_id AS user_id, roles.role_code AS role_code FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_code"}))

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=award_advisor")
	GetUserList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"role_code\":\"teacher\"")
}

// page_size 超上限被裁剪为 50
func TestGetUserList_PageSizeClamped(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	ensureRedisForUserList(t).FlushDB(t.Context())

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college", "grade"}))

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=999&role=student")
	GetUserList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// 关键词搜索
func TestGetUserList_WithSearch(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	ensureRedisForUserList(t).FlushDB(t.Context())

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`.*realname LIKE .*").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college", "grade"}).
			AddRow(1, "张三丰", "S001", "计算机学院", "2024"))

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=student&search=张三")
	GetUserList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "张三丰")
}

// 统计失败 → 500
func TestGetUserList_CountError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	rdb := ensureRedisForUserList(t)
	rdb.FlushDB(t.Context())

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=student")
	GetUserList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// 列表查询失败 → 500
func TestGetUserList_FindError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	rdb := ensureRedisForUserList(t)
	rdb.FlushDB(t.Context())

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=student")
	GetUserList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// award_advisor 角色回填查询失败 → 500
func TestGetUserList_AwardAdvisorRoleQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	rdb := ensureRedisForUserList(t)
	rdb.FlushDB(t.Context())

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college", "grade"}).
			AddRow(3, "王指导", "T003", "数学学院", "教职员工"))
	mock.ExpectQuery("SELECT user_roles.user_id AS user_id, roles.role_code AS role_code FROM `user_roles`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=award_advisor")
	GetUserList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// ===================== GetWorkAuditStudentList =====================

// 成功：有关键词过滤，返回 leader 信息
func TestGetWorkAuditStudentList_SuccessWithKeyword(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	// 赛事存在
	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	// count
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// 列表
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"reg_id", "team_name", "leader_name", "stu_id", "college", "phone", "email", "update_time"}).
			AddRow(100, "团队甲", "张三", "S001", "计算机学院", "13800000000", "a@b.c", "2026-05-01 10:00"))

	_, w, c := buildGET("/api/reg/work/audit/student/list?comp_id=1&page=1&page_size=10&keyword=团队")
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	GetWorkAuditStudentList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "团队甲")
	assert.Contains(t, w.Body.String(), "张三")
}

// 成功：无关键词，空结果
func TestGetWorkAuditStudentList_SuccessEmpty(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"reg_id", "team_name", "leader_name", "stu_id", "college", "phone", "email", "update_time"}))

	_, w, c := buildGET("/api/reg/work/audit/student/list?comp_id=1&page=1&page_size=10")
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	GetWorkAuditStudentList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":0")
}

// 缺少 comp_id → 400
func TestGetWorkAuditStudentList_MissingCompID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/work/audit/student/list?page=1&page_size=10")
	GetWorkAuditStudentList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

// comp_id 非法 → 400
func TestGetWorkAuditStudentList_InvalidCompID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/reg/work/audit/student/list?comp_id=abc")
	GetWorkAuditStudentList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

// 赛事不存在 → 404
func TestGetWorkAuditStudentList_CompNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnError(fmt.Errorf("record not found"))

	_, w, c := buildGET("/api/reg/work/audit/student/list?comp_id=999")
	GetWorkAuditStudentList(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "赛事不存在")
}

// 无 user_id → 401
func TestGetWorkAuditStudentList_Unauthorized(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))

	_, w, c := buildGET("/api/reg/work/audit/student/list?comp_id=1")
	GetWorkAuditStudentList(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}

// 院级管理员访问他院赛事 → 403
func TestGetWorkAuditStudentList_CollegeAdminForbidden(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 99, 7))

	managedCollegeID := uint(3)
	_, w, c := buildGET("/api/reg/work/audit/student/list?comp_id=1")
	c.Set("role_code", "college_admin")
	c.Set("managed_college_id", &managedCollegeID)
	c.Set("user_id", uint(5))
	GetWorkAuditStudentList(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无权查看该赛事提交信息")
}

// count 失败 → 500
func TestGetWorkAuditStudentList_CountError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/reg/work/audit/student/list?comp_id=1")
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	GetWorkAuditStudentList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// 分页参数默认值：page/page_size 缺省时补 1/10，且上限 100
func TestGetWorkAuditStudentList_DefaultPaging(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"reg_id", "team_name", "leader_name", "stu_id", "college", "phone", "email", "update_time"}))

	_, w, c := buildGET("/api/reg/work/audit/student/list?comp_id=1")
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	GetWorkAuditStudentList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"page\":1")
	assert.Contains(t, w.Body.String(), "\"size\":10")
}

// ===================== SubmitWork 成功路径 =====================

// 成功：成员 + 免审核赛事（status=0 & need_reg_audit=0 → 自动置 1）+ 时间窗口内
func TestSubmitWork_SuccessWithoutRegAudit(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()

	// Select("username").First(&user)
	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	// Preload("Competition.Detail").First(&reg)
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 0))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit", "submit_start_time", "submit_end_time"}).
			AddRow(1, 1, 0, now.Add(-time.Hour), now.Add(time.Hour)))
	// Count reg_members → 1
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// status=0 & need_reg_audit=0 → 自动置 1
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers` SET `status`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// 保存作品附件
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers` SET `work_attachment_url`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := SubmitWorkReq{RegID: 1, WorkUrl: "/files/work.pdf"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitWork(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "作品已成功保存")
}

// 成功：报名已通过（status=1），直接保存
func TestSubmitWork_SuccessAlreadyApproved(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit", "submit_start_time", "submit_end_time"}).
			AddRow(1, 1, 1, now.Add(-time.Hour), now.Add(time.Hour)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// status=1 不再 UPDATE status，直接保存附件（同样存在 comp_directories Upsert）
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers` SET `work_attachment_url`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := SubmitWorkReq{RegID: 1, WorkUrl: "/files/work.pdf"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitWork(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "作品已成功保存")
}

// 报名未通过审核（status=0 且 need_reg_audit=1）→ 400
func TestSubmitWork_RegNotApproved(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 0))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit", "submit_start_time", "submit_end_time"}).
			AddRow(1, 1, 1, now.Add(-time.Hour), now.Add(time.Hour)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	body := SubmitWorkReq{RegID: 1, WorkUrl: "/files/work.pdf"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitWork(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "报名未通过审核")
}

// 获取用户信息失败 → 500
func TestSubmitWork_UserLookupError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	body := SubmitWorkReq{RegID: 1, WorkUrl: "/files/work.pdf"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitWork(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// 提交时间未配置 → 403
func TestSubmitWork_SubmitWindowNotConfigured(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit", "submit_start_time", "submit_end_time"}).
			AddRow(1, 1, 1, time.Time{}, time.Time{}))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	body := SubmitWorkReq{RegID: 1, WorkUrl: "/files/work.pdf"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitWork(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "暂未开放作品提交")
}

// 提交尚未开始 → 403
func TestSubmitWork_SubmitNotStarted(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit", "submit_start_time", "submit_end_time"}).
			AddRow(1, 1, 1, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	body := SubmitWorkReq{RegID: 1, WorkUrl: "/files/work.pdf"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitWork(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "作品提交通道尚未开启")
}

// 提交已截止 → 403
func TestSubmitWork_SubmitEnded(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit", "submit_start_time", "submit_end_time"}).
			AddRow(1, 1, 1, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	body := SubmitWorkReq{RegID: 1, WorkUrl: "/files/work.pdf"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitWork(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "作品提交已截止")
}

// 保存作品失败 → 500
func TestSubmitWork_SaveFails(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "need_reg_audit", "submit_start_time", "submit_end_time"}).
			AddRow(1, 1, 1, now.Add(-time.Hour), now.Add(time.Hour)))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers` SET `work_attachment_url`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := SubmitWorkReq{RegID: 1, WorkUrl: "/files/work.pdf"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	SubmitWork(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}
