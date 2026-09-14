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
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupDeclareDBMock(t *testing.T) sqlmock.Sqlmock {
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

// ===================== CreateDeclare =====================

func TestCreateDeclare_BadRequest(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺少必填字段
	body := map[string]interface{}{"comp_name": "测试赛事"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	CreateDeclare(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "请求参数错误")
}

func TestCreateDeclare_Unauthorized(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	body := DeclareCreateReq{
		CompName:   "测试赛事",
		CompLevel:  "校级",
		CompType:   "学科竞赛",
		Organizer:  "教务处",
		Undertaker: "计算机学院",
		CollegeID:  1,
		ManagerID:  1,
		Year:       2026,
	}
	_, w, c := buildAuthPOSTJSON(body)
	// 不设置 user_id
	CreateDeclare(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未授权")
}

func TestCreateDeclare_Success(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `comp_declarations`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := DeclareCreateReq{
		CompName:   "测试赛事",
		CompLevel:  "校级",
		CompType:   "学科竞赛",
		Organizer:  "教务处",
		Undertaker: "计算机学院",
		CollegeID:  1,
		ManagerID:  1,
		Year:       2026,
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	CreateDeclare(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "创建成功")
}

func TestCreateDeclare_DBError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `comp_declarations`").
		WillReturnError(fmt.Errorf("duplicate key"))
	mock.ExpectRollback()

	body := DeclareCreateReq{
		CompName:   "测试赛事",
		CompLevel:  "校级",
		CompType:   "学科竞赛",
		Organizer:  "教务处",
		Undertaker: "计算机学院",
		CollegeID:  1,
		ManagerID:  1,
		Year:       2026,
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	CreateDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestCreateDeclare_CollegeAdminCannotUseAnotherCollege(t *testing.T) {
	setupDeclareDBMock(t)
	managedCollegeID := uint(2)
	body := DeclareCreateReq{
		CompName:   "测试赛事",
		CompLevel:  "校级",
		CompType:   "学科竞赛",
		Organizer:  "教务处",
		Undertaker: "计算机学院",
		CollegeID:  3,
		ManagerID:  1,
		Year:       2026,
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(10))
	c.Set("role_code", "college_admin")
	c.Set("managed_college_id", &managedCollegeID)

	CreateDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "只能为所管理学院创建申报")
}

// ===================== GetDeclareDetail =====================

func TestGetDeclareDetail_NotFound(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/declare/detail/999")
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	GetDeclareDetail(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "申报记录不存在")
}

func TestGetDeclareDetail_DBError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/declare/detail/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	GetDeclareDetail(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetDeclareDetail_Success(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()
	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name", "declare_status", "college_id", "manager_id", "created_by", "create_time", "update_time"}).
			AddRow(1, "测试赛事", 0, 1, 1, 1, now, now))
	// Preload: CollegeInfo, Manager, Declarer, Auditor
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "计算机学院"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(1, "管理员"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(1, "申报人"))
	// Auditor 可能为空
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}))

	_, w, c := buildGET("/api/declare/detail/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	GetDeclareDetail(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "查询成功")
}

// ===================== UpdateDeclare =====================

func TestUpdateDeclare_BadRequest(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	// 无效的 JSON
	_, w, c := buildGET("/api/declare/update/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateDeclare(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestUpdateDeclare_NotFound(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := map[string]interface{}{"comp_name": "更新后的赛事"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	UpdateDeclare(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "申报记录不存在")
}

func TestUpdateDeclare_Forbidden_AlreadySubmitted(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 1))

	body := map[string]interface{}{"comp_name": "更新后的赛事"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "仅草稿和已驳回状态可编辑")
}

func TestUpdateDeclare_Forbidden_Approved(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 2))

	body := map[string]interface{}{"comp_name": "更新后的赛事"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "仅草稿和已驳回状态可编辑")
}

func TestUpdateDeclare_Success_Draft(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 0))
	// Updates 使用事务
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"comp_name": "更新后的赛事"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateDeclare(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "更新成功")
}

func TestUpdateDeclare_Success_Rejected(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 3))
	// Updates 使用事务
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"comp_name": "更新后的赛事"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateDeclare(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "更新成功")
}

// ===================== SubmitDeclare =====================

func TestSubmitDeclare_NotFound(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/declare/submit/999")
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	SubmitDeclare(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSubmitDeclare_Forbidden_AlreadySubmitted(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 1))

	_, w, c := buildGET("/api/declare/submit/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	SubmitDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "只有草稿和已驳回状态可提交")
}

func TestSubmitDeclare_Forbidden_Approved(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 2))

	_, w, c := buildGET("/api/declare/submit/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	SubmitDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "只有草稿和已驳回状态可提交")
}

func TestSubmitDeclare_Success(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 0))
	// Update 使用事务
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	_, w, c := buildGET("/api/declare/submit/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	SubmitDeclare(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "申报已提交")
}

func TestSubmitDeclare_Success_Rejected(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 3))
	// Update 使用事务
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	_, w, c := buildGET("/api/declare/submit/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	SubmitDeclare(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "申报已提交")
}

// ===================== RevokeDeclare =====================

func TestRevokeDeclare_NotFound(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/declare/revoke/999")
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	RevokeDeclare(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRevokeDeclare_Forbidden_Draft(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 0))

	_, w, c := buildGET("/api/declare/revoke/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	RevokeDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "只有已提交状态可撤回")
}

func TestRevokeDeclare_Forbidden_Approved(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 2))

	_, w, c := buildGET("/api/declare/revoke/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	RevokeDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "只有已提交状态可撤回")
}

func TestRevokeDeclare_Success(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 1))
	// Update 使用事务
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	_, w, c := buildGET("/api/declare/revoke/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	RevokeDeclare(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "申报已撤回")
}

// ===================== GetMyDeclares =====================

func TestGetMyDeclares_BadRequest(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺少 page 和 page_size
	_, w, c := buildGET("/api/declare/my")
	c.Set("user_id", uint(1))
	GetMyDeclares(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestGetMyDeclares_Unauthorized(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/declare/my?page=1&page_size=10")
	GetMyDeclares(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未授权")
}

func TestGetMyDeclares_Success(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name", "declare_status", "created_by", "college_id", "manager_id"}).AddRow(1, "测试赛事", 0, 1, 1, 1))
	// Preload queries: CollegeInfo -> Manager -> Declarer -> Auditor (BelongsTo 优先)
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "计算机学院"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(1, "管理员"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(1, "申报人"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}))

	_, w, c := buildGET("/api/declare/my?page=1&page_size=10")
	c.Set("user_id", uint(1))
	GetMyDeclares(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== GetMyPendingDeclares =====================

func TestGetMyPendingDeclares_BadRequest(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/declare/my/pending")
	c.Set("user_id", uint(1))
	GetMyPendingDeclares(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetMyPendingDeclares_Unauthorized(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/declare/my/pending?page=1&page_size=10")
	GetMyPendingDeclares(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ===================== GetMyPublishedDeclares =====================

func TestGetMyPublishedDeclares_BadRequest(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/declare/my/published")
	c.Set("user_id", uint(1))
	GetMyPublishedDeclares(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetMyPublishedDeclares_Unauthorized(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/declare/my/published?page=1&page_size=10")
	GetMyPublishedDeclares(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ===================== DeleteDeclare =====================

func TestDeleteDeclare_NotFound(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/declare/delete/999")
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	DeleteDeclare(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteDeclare_Forbidden_AlreadySubmitted(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 1))

	_, w, c := buildGET("/api/declare/delete/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	DeleteDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "仅草稿状态可删除")
}

func TestDeleteDeclare_Success(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 0))
	// Unscoped Delete 使用事务
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `comp_declarations`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	_, w, c := buildGET("/api/declare/delete/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	DeleteDeclare(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "删除成功")
}

// ===================== GetPendingDeclares =====================

func TestGetPendingDeclares_BadRequest(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/declare/pending")
	GetPendingDeclares(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetPendingDeclares_Success(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name", "declare_status"}))

	_, w, c := buildGET("/api/declare/pending?page=1&page_size=10")
	GetPendingDeclares(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== AuditDeclare =====================

func TestAuditDeclare_BadRequest(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	// 缺少必填字段
	body := map[string]interface{}{}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	AuditDeclare(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestAuditDeclare_CollegeAdminForbidden(t *testing.T) {
	setupDeclareDBMock(t)
	managedCollegeID := uint(2)
	body := map[string]interface{}{"declare_id": 1, "audit_status": 2}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(10))
	c.Set("role_code", "college_admin")
	c.Set("managed_college_id", &managedCollegeID)

	AuditDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "仅校级管理员")
}

func TestAuditDeclare_BadRequest_InvalidStatus(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	// audit_status 必须是 2 或 3
	body := map[string]interface{}{"declare_id": 1, "audit_status": 1}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	AuditDeclare(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuditDeclare_Unauthorized(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	body := map[string]interface{}{"declare_id": 1, "audit_status": 2}
	_, w, c := buildAuthPOSTJSON(body)
	AuditDeclare(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuditDeclare_NotFound(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := map[string]interface{}{"declare_id": 999, "audit_status": 2}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	AuditDeclare(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAuditDeclare_Forbidden_NotSubmitted(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 0))

	body := map[string]interface{}{"declare_id": 1, "audit_status": 2}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	AuditDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "只能审核已提交的申报")
}

func TestAuditDeclare_Reject_Success(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status", "college_id", "manager_id", "comp_level", "year"}).
			AddRow(1, 1, 1, 1, "校级", 2026))
	// Save 使用事务
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"declare_id": 1, "audit_status": 3, "audit_remark": "材料不全"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	AuditDeclare(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "申报审核拒绝")
}

// ===================== GetAllDeclares =====================

func TestGetAllDeclares_BadRequest(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/declare/all")
	GetAllDeclares(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetAllDeclares_Success(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name", "declare_status"}))

	_, w, c := buildGET("/api/declare/all?page=1&page_size=10")
	GetAllDeclares(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== GetAuditedDeclares =====================

func TestGetAuditedDeclares_BadRequest(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/declare/audited")
	GetAuditedDeclares(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetAuditedDeclares_Success(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name", "declare_status"}))

	_, w, c := buildGET("/api/declare/audited?page=1&page_size=10")
	GetAuditedDeclares(c)

	assert.Equal(t, http.StatusOK, w.Code)
}
