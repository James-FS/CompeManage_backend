package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"CompeManage_backend/database"
	"CompeManage_backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// ===================== fillAwardAdvisorsIfEmpty =====================

func TestFillAwardAdvisorsIfEmpty_EmptyPairs(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	assert.NoError(t, fillAwardAdvisorsIfEmpty(database.DB, 1, nil))
}

func TestFillAwardAdvisorsIfEmpty_CountError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_advisors`").
		WillReturnError(fmt.Errorf("db error"))

	assert.Error(t, fillAwardAdvisorsIfEmpty(database.DB, 1,
		[]models.AdvisorInfo{{Username: "T1", Name: "张老师"}}))
}

func TestFillAwardAdvisorsIfEmpty_AlreadyHasAdvisors(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_advisors`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	assert.NoError(t, fillAwardAdvisorsIfEmpty(database.DB, 1,
		[]models.AdvisorInfo{{Username: "T1", Name: "张老师"}}))
}

func TestFillAwardAdvisorsIfEmpty_CreateError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_advisors`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `award_advisors`").
		WillReturnError(fmt.Errorf("insert failed"))
	mock.ExpectRollback()

	assert.Error(t, fillAwardAdvisorsIfEmpty(database.DB, 1,
		[]models.AdvisorInfo{{Username: "T1", Name: "张老师"}}))
}

func TestFillAwardAdvisorsIfEmpty_Success(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_advisors`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `award_advisors`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `award_advisors`").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET `advisor_info`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	assert.NoError(t, fillAwardAdvisorsIfEmpty(database.DB, 1,
		[]models.AdvisorInfo{
			{Username: "T1", Name: "张老师", College: "计算机学院"},
			{Username: "T2", Name: "李老师", College: "数学学院"},
		}))
}

// ===================== UpdateAwardMember =====================

func TestUpdateAwardMember_InvalidID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildAuthPOSTJSON(map[string]interface{}{"remark": "x"})
	c.Params = []gin.Param{{Key: "id", Value: "abc"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardMember(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "成员ID无效")
}

func TestUpdateAwardMember_MemberNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildAuthPOSTJSON(map[string]interface{}{"remark": "x"})
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardMember(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "成员不存在")
}

func TestUpdateAwardMember_AwardNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id"}).AddRow(1, 9))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildAuthPOSTJSON(map[string]interface{}{"remark": "x"})
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardMember(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "奖项不存在")
}

func TestUpdateAwardMember_NoAwardPermission(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id"}).AddRow(1, 9))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))

	_, w, c := buildAuthPOSTJSON(map[string]interface{}{"remark": "x"})
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "expert") // 无获奖填报权限
	UpdateAwardMember(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "没有获奖填报权限")
}

func TestUpdateAwardMember_RemarkTooLong(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id"}).AddRow(1, 9))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	_, w, c := buildAuthPOSTJSON(map[string]interface{}{"remark": strings.Repeat("长", 2001)})
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardMember(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "备注无效")
}

func TestUpdateAwardMember_TransferDenied(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number"}).AddRow(1, 9, "S001"))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	belong := uint(3)
	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number", "submitted_college_id"}).AddRow(1, 9, "S001", &belong))
	mock.ExpectRollback()

	managed := uint(3)
	collegeID := uint(5)
	body := map[string]interface{}{"remark": "x", "submitted_college_id": collegeID}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin") // 院管可改成员，但不得确认归属
	c.Set("managed_college_id", &managed)
	UpdateAwardMember(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "只能修改本院填报成员，历史归属须由校管理员确认")
}

func TestUpdateAwardMember_CollegeNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number"}).AddRow(1, 9, "S001"))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number", "submitted_college_id"}).AddRow(1, 9, "S001", nil))
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	collegeID := uint(5)
	body := map[string]interface{}{"remark": "x", "submitted_college_id": collegeID}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardMember(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "学院不存在")
}

func TestUpdateAwardMember_AlreadyTransferred(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number"}).AddRow(1, 9, "S001"))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	existing := uint(3)
	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number", "submitted_college_id"}).AddRow(1, 9, "S001", &existing))
	mock.ExpectRollback()

	collegeID := uint(5)
	body := map[string]interface{}{"remark": "x", "submitted_college_id": collegeID}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardMember(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "已有填报归属不能转移")
}

func TestUpdateAwardMember_StudentNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number"}).AddRow(1, 9, "S001"))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number"}).AddRow(1, 9, "S001"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	body := map[string]interface{}{"remark": "x", "student_id": "S404"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardMember(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "学号不存在")
}

func TestUpdateAwardMember_DuplicateStudent(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number"}).AddRow(1, 9, "S001"))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number"}).AddRow(1, 9, "S001"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college", "major"}).AddRow(2, "S002", "李四", "数学学院", "统计学"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	body := map[string]interface{}{"remark": "x", "student_id": "S002"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardMember(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "已有其他获奖记录")
}

func TestUpdateAwardMember_StudentChangeSuccess(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number"}).AddRow(1, 9, "S001"))
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT .* FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "award_id", "student_number"}).AddRow(1, 9, "S001"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college", "major"}).AddRow(2, "S002", "李四", "数学学院", "统计学"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_members`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("UPDATE `award_members` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := map[string]interface{}{"remark": "更换成员", "student_id": "S002"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardMember(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===================== UpdateAwardTeam =====================

func TestUpdateAwardTeam_BindError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	_, w, c := buildAuthPOSTJSON(map[string]interface{}{"project_name": 123})
	c.Params = []gin.Param{{Key: "id", Value: "9"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardTeam(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "参数错误")
}

func TestUpdateAwardTeam_InvalidCategory(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id"}).AddRow(9, 1))
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	body := map[string]interface{}{
		"project_name": "新项目", "award_category": "省级", "award_name": "一等奖", "award_month": "2026-13",
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "9"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardTeam(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "项目、类别、等级或年月无效")
}

func TestUpdateAwardTeam_NameConflict(t *testing.T) {
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
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	body := map[string]interface{}{
		"project_name": "同名项目", "award_category": "省级", "award_name": "一等奖", "award_month": "2026-05",
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "9"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardTeam(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "该赛事已存在同名项目")
}

func TestUpdateAwardTeam_SuccessWithAdvisors(t *testing.T) {
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
		WillReturnResult(sqlmock.NewResult(0, 1))
	// replaceAwardAdvisors：工号回查 + 物理删 + 重建 + 镜像
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college", "identity_type"}).
			AddRow(3, "T003", "王指导", "计算机学院", "staff"))
	mock.ExpectExec("DELETE FROM `award_advisors`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `award_advisors`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE `awards` SET `advisor_info`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	advisors := []AwardAdvisorEditReq{{Username: "T003"}}
	body := map[string]interface{}{
		"project_name": "新项目", "award_category": "省级", "award_name": "一等奖", "award_month": "2026-05",
		"advisors": advisors,
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Params = []gin.Param{{Key: "id", Value: "9"}}
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	UpdateAwardTeam(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

var _ = gorm.ErrRecordNotFound
