package controllers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// 构造带原始 JSON body 的请求（用于绑定失败分支）
func buildRawPOST(body string) (*httptest.ResponseRecorder, *gin.Context) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return w, c
}

// ===================== PassAwardAudit 分支补齐 =====================

func TestPassAwardAudit_BindError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	w, c := buildRawPOST("{not-json")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	PassAwardAudit(c)

	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestPassAwardAudit_BeginError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id", "reg_id"}).AddRow(1, "draft", 1, 1))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin().WillReturnError(fmt.Errorf("begin failed"))

	_, w, c := buildGET("/api/award/audit/pass/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	PassAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPassAwardAudit_UpdateError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id", "reg_id"}).AddRow(1, "draft", 1, 1))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildGET("/api/award/audit/pass/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	PassAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPassAwardAudit_VersionConflict(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id", "reg_id"}).AddRow(1, "draft", 1, 1))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 行受影响 → 版本冲突
	mock.ExpectRollback()

	_, w, c := buildGET("/api/award/audit/pass/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	PassAwardAudit(c)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestPassAwardAudit_RegisterUpdateError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id", "reg_id"}).AddRow(1, "draft", 1, 10))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `registers` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildGET("/api/award/audit/pass/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	PassAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPassAwardAudit_CommitError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id", "reg_id"}).AddRow(1, "draft", 1, 0))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("commit failed"))

	_, w, c := buildGET("/api/award/audit/pass/1")
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	PassAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== RejectAwardAudit 分支补齐 =====================

func TestRejectAwardAudit_UpdateError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id", "reg_id"}).AddRow(1, "draft", 1, 1))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := map[string]interface{}{"reason": "材料不全"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	RejectAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRejectAwardAudit_VersionConflict(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id", "reg_id"}).AddRow(1, "draft", 1, 1))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	body := map[string]interface{}{"reason": "材料不全"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	RejectAwardAudit(c)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestRejectAwardAudit_RegisterUpdateError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id", "reg_id"}).AddRow(1, "draft", 1, 10))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `registers` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := map[string]interface{}{"reason": "材料不全"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	RejectAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRejectAwardAudit_CommitError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id", "reg_id"}).AddRow(1, "draft", 1, 0))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("commit failed"))

	body := map[string]interface{}{"reason": "材料不全"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	RejectAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== BatchPassAwardAudit 分支补齐 =====================

func TestBatchPassAwardAudit_BeginError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin().WillReturnError(fmt.Errorf("begin failed"))

	body := map[string]interface{}{"ids": []uint{1}}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchPassAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBatchPassAwardAudit_UpdateError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := map[string]interface{}{"ids": []uint{1}}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchPassAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBatchPassAwardAudit_RegisterUpdateError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "reg_id"}).AddRow(1, 1, 10))
	mock.ExpectExec("UPDATE `registers` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := map[string]interface{}{"ids": []uint{1}}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchPassAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBatchPassAwardAudit_CommitError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "reg_id"}).AddRow(1, 1, 10))
	mock.ExpectExec("UPDATE `registers` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("commit failed"))

	body := map[string]interface{}{"ids": []uint{1}}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchPassAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== BatchRejectAwardAudit 分支补齐 =====================

func TestBatchRejectAwardAudit_BeginError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin().WillReturnError(fmt.Errorf("begin failed"))

	body := map[string]interface{}{"ids": []uint{1}, "reason": "不通过"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchRejectAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBatchRejectAwardAudit_UpdateError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := map[string]interface{}{"ids": []uint{1}, "reason": "不通过"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchRejectAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBatchRejectAwardAudit_RegisterUpdateError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "reg_id"}).AddRow(1, 1, 10))
	mock.ExpectExec("UPDATE `registers` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := map[string]interface{}{"ids": []uint{1}, "reason": "不通过"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchRejectAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBatchRejectAwardAudit_CommitError(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "reg_id"}).AddRow(1, 1, 10))
	mock.ExpectExec("UPDATE `registers` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("commit failed"))

	body := map[string]interface{}{"ids": []uint{1}, "reason": "不通过"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchRejectAwardAudit(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// 版本校验失败 → 冲突
func TestBatchPassAwardAudit_VersionConflict(t *testing.T) {
	mock := setupAwardDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "comp_id"}).AddRow(1, "draft", 1))
	mock.ExpectRollback()

	past := time.Now().Add(-time.Hour)
	body := map[string]interface{}{"ids": []uint{1}, "versions": map[string]time.Time{"1": past}}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	BatchPassAwardAudit(c)

	assert.Equal(t, http.StatusConflict, w.Code)
}

var _ = gorm.ErrRecordNotFound
