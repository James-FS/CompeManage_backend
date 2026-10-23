package controllers

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ===================== GetStudentMyAwardList =====================

func TestGetStudentMyAwardList_UserIDTypeError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/award/student/my-awards")
	c.Set("user_id", "x")
	GetStudentMyAwardList(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "用户ID格式错误")
}

func TestGetStudentMyAwardList_AllFilters(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/award/student/my-awards?page=0&size=999&comp_id=1&source=supplement&status=approved")
	c.Set("user_id", uint(11))
	GetStudentMyAwardList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetStudentMyAwardList_CountError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/award/student/my-awards")
	c.Set("user_id", uint(11))
	GetStudentMyAwardList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetStudentMyAwardList_FindError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/award/student/my-awards?source=import")
	c.Set("user_id", uint(11))
	GetStudentMyAwardList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== GetAwardAuditList =====================

func TestGetAwardAuditList_FiltersAndPaging(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	// 两行：一命中关键词、一被过滤
	mock.ExpectQuery("(?s)SELECT .*FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "reg_id", "project_name", "award_level", "award_time", "status", "updated_at", "created_at"}).
			AddRow(9, 1, 0, "项目A", "省级一等奖", nil, "draft", nil, nil).
			AddRow(10, 1, 0, "项目B", "省级一等奖", nil, "draft", nil, nil))
	mock.ExpectQuery("SELECT \\* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "name", "student_number", "phone", "is_leader"}).
			AddRow(1, 9, "张三", "S001", "138", true))
	mock.ExpectQuery("(?s)SELECT (\\*|`comp_directories`)").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "建模大赛"))

	_, w, c := buildGET("/api/award/audit/list?page=9&pageSize=999&keyword=张&award_level=省级一等奖&comp_name=建模&source=all&status=1")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetAwardAuditList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":1")
}

func TestGetAwardAuditList_ScopeInvalid(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/award/audit/list")
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin") // 未配置管理学院
	GetAwardAuditList(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetAwardAuditList_FindError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("(?s)SELECT .*FROM `awards`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/award/audit/list?page_size=0")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	GetAwardAuditList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== BatchDeleteCompetition =====================

func TestBatchDeleteCompetition_CompsQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`comp_name` FROM `comp_directories`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildAuthPOSTJSON(map[string]interface{}{"ids": []uint{1}})
	BatchDeleteCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBatchDeleteCompetition_DetailsQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`comp_name` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "赛事A"))
	mock.ExpectQuery("SELECT `comp_id`,`comp_start_time` FROM `comp_details`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildAuthPOSTJSON(map[string]interface{}{"ids": []uint{1}})
	BatchDeleteCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBatchDeleteCompetition_DeleteError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`comp_name` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "赛事A"))
	mock.ExpectQuery("SELECT `comp_id`,`comp_start_time` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id", "comp_start_time"}).AddRow(1, nil))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_directories` SET `delete_time`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(map[string]interface{}{"ids": []uint{1}})
	BatchDeleteCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBatchDeleteCompetition_Success(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `id`,`comp_name` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "赛事A"))
	mock.ExpectQuery("SELECT `comp_id`,`comp_start_time` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id", "comp_start_time"}))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_directories` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	_, w, c := buildAuthPOSTJSON(map[string]interface{}{"ids": []uint{1}})
	BatchDeleteCompetition(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"deleted_count\":1")
}

// ===================== GetCompetitionList 缓存与筛选 =====================

// 非管理角色走缓存：命中后直接返回
func TestGetCompetitionList_CacheHit(t *testing.T) {
	rdb := ensureRedisForUserList(t)
	rdb.FlushDB(t.Context())

	req := CompListReq{Page: 1, PageSize: 10}
	reqBytes, _ := json.Marshal(req)
	identity := fmt.Sprintf("%v:%v:%v:%d", interface{}(uint(5)), interface{}("student"), interface{}(nil), uint(0))
	cacheKey := fmt.Sprintf("cache:comp_list:duty_v3:%s:%x", identity, md5.Sum(reqBytes))
	rdb.Set(t.Context(), cacheKey, `{"list":[],"total":0,"page":1,"size":10}`, 0)

	_, w, c := buildGET("/api/comp/list?page=1&page_size=10")
	c.Set("user_id", uint(5))
	c.Set("role_code", "student")
	c.Set("permission_codes", []string{})
	GetCompetitionList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// 管理角色不走缓存：全部筛选命中查询分支
func TestGetCompetitionList_AllFilters(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("(?s)SELECT (\\*|`comp_directories`)").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/comp/list?page=1&page_size=10&comp_name=建模&manager=张&status=ended&comp_level=校级&college=计算机学院&year=2026&is_my=true&is_reg=true")
	c.Set("user_id", uint(5))
	c.Set("role_code", "school_admin")
	GetCompetitionList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// status 三分支
func TestGetCompetitionList_StatusBranches(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	for i := 0; i < 2; i++ {
		mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("(?s)SELECT (\\*|`comp_directories`)").
			WillReturnRows(sqlmock.NewRows([]string{"id"}))
	}

	_, w, c := buildGET("/api/comp/list?page=1&page_size=10&status=upcoming")
	c.Set("user_id", uint(5))
	c.Set("role_code", "school_admin")
	GetCompetitionList(c)
	assert.Equal(t, http.StatusOK, w.Code)

	_, w2, c2 := buildGET("/api/comp/list?page=1&page_size=10&status=ongoing")
	c2.Set("user_id", uint(5))
	c2.Set("role_code", "school_admin")
	GetCompetitionList(c2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// ===================== GetStatisticsDashboard 缓存命中 =====================

func TestGetStatisticsDashboard_CacheHit(t *testing.T) {
	rdb := ensureRedisForUserList(t)
	rdb.FlushDB(t.Context())

	managed := uint(3)
	cacheKey := fmt.Sprintf("cache:stats:dashboard:v3:%s:%d:%s:%d", "", uint(5), "college_admin", uint(3))
	rdb.Set(t.Context(), cacheKey, `{"total_competitions":7}`, 0)

	_, w, c := buildGET("/api/statistics/dashboard")
	c.Set("user_id", uint(5))
	c.Set("role_code", "college_admin")
	c.Set("managed_college_id", &managed)
	GetStatisticsDashboard(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "total_competitions")
}

var _ = gin.H{}
