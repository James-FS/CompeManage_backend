package controllers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// ===================== advisorAwardEdit.validate =====================

func validAdvisorEdit() advisorAwardEdit {
	return advisorAwardEdit{
		ProjectName:   "指导项目",
		AwardCategory: "省级",
		AwardName:     "一等奖",
		AwardDate:     DateOnly{Time: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
		ProofURL:      validProofURL,
		UpdatedAt:     advisorTestTime,
	}
}

func TestAdvisorEditValidate_ProjectBlank(t *testing.T) {
	r := validAdvisorEdit()
	r.ProjectName = " "
	assert.ErrorContains(t, r.validate(), "项目名称不能为空且不能超过255字")
}

func TestAdvisorEditValidate_InvalidCategory(t *testing.T) {
	r := validAdvisorEdit()
	r.AwardCategory = "国际"
	assert.ErrorContains(t, r.validate(), "请选择有效的获奖类别")
}

func TestAdvisorEditValidate_InvalidAwardName(t *testing.T) {
	r := validAdvisorEdit()
	r.AwardName = "特等奖"
	assert.ErrorContains(t, r.validate(), "请选择有效的获奖等级")
}

func TestAdvisorEditValidate_ZeroDate(t *testing.T) {
	r := validAdvisorEdit()
	r.AwardDate = DateOnly{}
	assert.ErrorContains(t, r.validate(), "请选择获奖时间")
}

func TestAdvisorEditValidate_RemarkTooLong(t *testing.T) {
	r := validAdvisorEdit()
	r.Remark = makeRune('长', 2001)
	assert.ErrorContains(t, r.validate(), "备注不能超过2000字")
}

func TestAdvisorEditValidate_InvalidProof(t *testing.T) {
	r := validAdvisorEdit()
	r.ProofURL = "C:\\evil\\proof.pdf"
	assert.ErrorContains(t, r.validate(), "请上传有效的获奖证明文件")
}

// 固定时间：binding:required 要求非零值，且需与记录 update_time 精确相等
var advisorTestTime = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

const validProofURL = "/api/file/download/award_proof/0123456789abcdef0123456789abcdef_p.pdf"

func makeRune(ch rune, n int) string {
	out := make([]rune, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, ch)
	}
	return string(out)
}

// ===================== UpdateAdvisorAward 分支补齐 =====================

func advisorEditBody() advisorAwardEdit {
	return validAdvisorEdit()
}

func TestUpdateAdvisorAward_InvalidID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectAdvisorUser(mock)
	c, w := advisorTestContext("POST", "teacher", advisorEditBody())
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	UpdateAdvisorAward(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "无效的获奖ID")
}

func TestUpdateAdvisorAward_AwardNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectAdvisorUser(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	c, w := advisorTestContext("POST", "teacher", advisorEditBody())
	UpdateAdvisorAward(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "获奖记录不存在")
}

func TestUpdateAdvisorAward_LockError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectAdvisorUser(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	c, w := advisorTestContext("POST", "teacher", advisorEditBody())
	UpdateAdvisorAward(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateAdvisorAward_ScopeCountError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectAdvisorUser(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "reg_id", "proof_url"}).AddRow(7, 1, "rejected", 0, ""))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	c, w := advisorTestContext("POST", "teacher", advisorEditBody())
	UpdateAdvisorAward(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateAdvisorAward_ProofInvalid(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectAdvisorUser(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "reg_id", "proof_url", "update_time", "source"}).
			AddRow(7, 1, "rejected", 0, "", advisorTestTime, "supplement"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// 证明文件不同 → 校验失败
	mock.ExpectQuery("SELECT \\* FROM `file_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	c, w := advisorTestContext("POST", "teacher", advisorEditBody())
	UpdateAdvisorAward(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "请使用本人上传或自己指导项目已有的证明文件")
}

func TestUpdateAdvisorAward_VersionConflict(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectAdvisorUser(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "reg_id", "proof_url", "update_time", "source"}).
			AddRow(7, 1, "rejected", 0, validProofURL, advisorTestTime.Add(-time.Hour), "supplement"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	// ProofURL 相同跳过证明校验；update_time 不同 → 409
	body := advisorEditBody()
	c, w := advisorTestContext("POST", "teacher", body)
	UpdateAdvisorAward(c)

	assert.Equal(t, 409, w.Code)
	assert.Contains(t, w.Body.String(), "填报已被他人修改")
}

func TestUpdateAdvisorAward_DuplicateConflict(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectAdvisorUser(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "reg_id", "proof_url", "update_time", "source"}).
			AddRow(7, 1, "rejected", 0, validProofURL, advisorTestTime, "supplement"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// 同名项目已存在
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	body := advisorEditBody()
	body.UpdatedAt = advisorTestTime
	c, w := advisorTestContext("POST", "teacher", body)
	UpdateAdvisorAward(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "该赛事已存在同名获奖项目")
}

func TestUpdateAdvisorAward_UpdateError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectAdvisorUser(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "reg_id", "proof_url", "update_time", "source"}).
			AddRow(7, 1, "rejected", 0, validProofURL, advisorTestTime, "supplement"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := advisorEditBody()
	body.UpdatedAt = advisorTestTime
	c, w := advisorTestContext("POST", "teacher", body)
	UpdateAdvisorAward(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateAdvisorAward_SuccessWithRegUpdate(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectAdvisorUser(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "reg_id", "proof_url", "update_time", "source"}).
			AddRow(7, 1, "rejected", 5, validProofURL, advisorTestTime, "supplement"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `award_changes`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE `registers` SET `status`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := advisorEditBody()
	body.UpdatedAt = advisorTestTime
	c, w := advisorTestContext("POST", "teacher", body)
	UpdateAdvisorAward(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "修改成功")
}

// ===================== UpdateAwardTeam 剩余分支 =====================

func teamBody() map[string]interface{} {
	return map[string]interface{}{
		"project_name": "新项目", "award_category": "省级", "award_name": "一等奖", "award_month": "2026-05",
	}
}

func TestUpdateAwardTeam_AwardNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildAuthPOSTJSON(teamBody())
	c.Params = []gin.Param{{Key: "id", Value: "9"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardTeam(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateAwardTeam_ForbiddenForCollegeAdmin(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	// canEditAwardTeam：学院存在但无本院成员
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(3, "数学学院"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	managed := uint(3)
	_, w, c := buildAuthPOSTJSON(teamBody())
	c.Params = []gin.Param{{Key: "id", Value: "9"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin")
	c.Set("managed_college_id", &managed)
	UpdateAwardTeam(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "仅团队成员所在学院的管理员或校级管理员可修改团队奖项")
}

func TestUpdateAwardTeam_DupCountError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(teamBody())
	c.Params = []gin.Param{{Key: "id", Value: "9"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardTeam(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateAwardTeam_UpdateError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("UPDATE `awards` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(teamBody())
	c.Params = []gin.Param{{Key: "id", Value: "9"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardTeam(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

var _ = gorm.ErrRecordNotFound
