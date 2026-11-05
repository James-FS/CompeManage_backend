package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// ===================== UpdateDeclare 分支补齐 =====================

func TestUpdateDeclare_QueryError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))

	body := map[string]interface{}{"comp_name": "x"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateDeclare_ScopeInvalid(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 0))

	body := map[string]interface{}{"comp_name": "x"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin") // 未配置 managed_college_id
	UpdateDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "尚未配置管理学院")
}

func TestUpdateDeclare_AccessDenied(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status", "created_by", "manager_id"}).
			AddRow(1, 0, 99, 99))

	body := map[string]interface{}{"comp_name": "x"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "competition_manager")
	UpdateDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无权编辑该申报")
}

func TestUpdateDeclare_CollegeAdminCannotTransfer(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	managed := uint(3)
	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status", "college_id"}).AddRow(1, 0, 3))

	body := map[string]interface{}{"college_id": 5}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin")
	c.Set("managed_college_id", &managed)
	UpdateDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "不能将申报转移到其他学院")
}

// 全字段更新：覆盖 updates 各字段分支
func TestUpdateDeclare_AllFieldsUpdate(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	attachment := "/static/x.pdf"
	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status", "manager_id", "college_id"}).
			AddRow(1, 0, 5, 3))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := DeclareUpdateReq{
		CompName: "新名", CompLevel: "省级", CompType: "学科竞赛",
		Organizer: "教务处", Undertaker: "学院",
		CollegeID: 3, ManagerID: 5, Year: 2026,
		Desc: "说明", AttachmentPath: &attachment,
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateDeclare(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "更新成功")
}

func TestUpdateDeclare_UpdateError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status", "manager_id", "college_id"}).
			AddRow(1, 0, 5, 3))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := map[string]interface{}{"comp_name": "x"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	UpdateDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== SubmitDeclare 分支补齐 =====================

func TestSubmitDeclare_QueryError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildAuthPOSTJSON(nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	SubmitDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSubmitDeclare_AccessDenied(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status", "created_by", "manager_id"}).
			AddRow(1, 0, 99, 99))

	_, w, c := buildAuthPOSTJSON(nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "competition_manager")
	SubmitDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无权提交该申报")
}

func TestSubmitDeclare_UpdateError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 0))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations` SET `declare_status`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	SubmitDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== RevokeDeclare 分支补齐 =====================

func TestRevokeDeclare_QueryError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildAuthPOSTJSON(nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	RevokeDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRevokeDeclare_AccessDenied(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status", "created_by", "manager_id"}).
			AddRow(1, 1, 99, 99))

	_, w, c := buildAuthPOSTJSON(nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "competition_manager")
	RevokeDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无权撤回该申报")
}

func TestRevokeDeclare_UpdateError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations` SET `declare_status`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	RevokeDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== DeleteDeclare 分支补齐 =====================

func TestDeleteDeclare_QueryError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildAuthPOSTJSON(nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	DeleteDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestDeleteDeclare_AccessDenied(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status", "created_by", "manager_id"}).
			AddRow(1, 0, 99, 99))

	_, w, c := buildAuthPOSTJSON(nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "competition_manager")
	DeleteDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无权删除该申报")
}

func TestDeleteDeclare_DeleteError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 0))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	DeleteDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== AuditDeclare 分支补齐 =====================

func TestAuditDeclare_ScopeInvalid(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	body := DeclareAuditReq{DeclareID: 1, AuditStatus: 3}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin") // 未配置管理学院
	AuditDeclare(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAuditDeclare_QueryError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))

	body := DeclareAuditReq{DeclareID: 1, AuditStatus: 3}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	AuditDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAuditDeclare_ApproveCodegenError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status", "comp_level", "year"}).
			AddRow(1, 1, "校级", 2026))
	// nextCompetitionCode：非 ErrRecordNotFound 的错误
	mock.ExpectQuery("SELECT `comp_code` FROM `comp_directories`").
		WillReturnError(fmt.Errorf("db error"))

	body := DeclareAuditReq{DeclareID: 1, AuditStatus: 2}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	AuditDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAuditDeclare_ApproveCreateCompError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status", "comp_level", "year"}).
			AddRow(1, 1, "校级", 2026))
	mock.ExpectQuery("SELECT `comp_code` FROM `comp_directories`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `comp_directories`").
		WillReturnError(fmt.Errorf("insert failed"))
	mock.ExpectRollback()

	body := DeclareAuditReq{DeclareID: 1, AuditStatus: 2}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	AuditDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAuditDeclare_RejectSaveError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := DeclareAuditReq{DeclareID: 1, AuditStatus: 3, AuditRemark: "不通过"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	AuditDeclare(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== 列表查询分支补齐 =====================

func TestGetPendingDeclares_ScopeInvalid(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/declare/pending/list?page=1&page_size=10")
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin")
	GetPendingDeclares(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetPendingDeclares_WithFilters(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/declare/pending/list?page=1&page_size=10&comp_name=建模&comp_level=省级&college_id=3")
	GetPendingDeclares(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":0")
}

func TestGetPendingDeclares_FindError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/declare/pending/list?page=1&page_size=10")
	GetPendingDeclares(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetAllDeclares_ScopeInvalid(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/declare/all?page=1&page_size=10")
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin")
	GetAllDeclares(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetAllDeclares_WithFilters(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/declare/all?page=1&page_size=10&comp_name=建模&comp_level=省级&college_id=3")
	GetAllDeclares(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetAllDeclares_FindError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/declare/all?page=1&page_size=10")
	GetAllDeclares(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetAuditedDeclares_ScopeInvalid(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/declare/audited/list?page=1&page_size=10")
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin")
	GetAuditedDeclares(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetAuditedDeclares_WithFilters(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/declare/audited/list?page=1&page_size=10&declare_status=2&comp_name=建模&comp_level=省级&college_id=3")
	GetAuditedDeclares(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetAuditedDeclares_FindError(t *testing.T) {
	mock := setupDeclareDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `comp_declarations`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/declare/audited/list?page=1&page_size=10")
	GetAuditedDeclares(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
