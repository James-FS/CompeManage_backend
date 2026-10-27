package controllers

import (
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ===================== UpdateCompetitionYear =====================

// 年份参数非法 → 400
func TestUpdateCompetitionYear_InvalidYearParam(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildCompPOSTJSON(map[string]string{"year": "2028"})
	c.Params = gin.Params{{Key: "year", Value: "abc"}}
	UpdateCompetitionYear(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "年份须为")
}

// 请求体缺失/非法 → 400
func TestUpdateCompetitionYear_BindError(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildCompPOSTJSON(map[string]int{"year": 2028})
	c.Params = gin.Params{{Key: "year", Value: "2027"}}
	UpdateCompetitionYear(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateCompetitionYear_InvalidNewYear(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildCompPOSTJSON(map[string]string{"year": "0000"})
	c.Params = gin.Params{{Key: "year", Value: "2027"}}
	UpdateCompetitionYear(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "年份须为")
}

// 源年度包含赛事 → 400 occupied
func TestUpdateCompetitionYear_SourceOccupied(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `competition_years` WHERE year = \\?.*FOR UPDATE").
		WithArgs(2027, 1).
		WillReturnRows(sqlmock.NewRows([]string{"year"}).AddRow(2027))
	expectYearCompetitions(mock, 2027, true)
	mock.ExpectRollback()

	_, w, c := buildCompPOSTJSON(map[string]string{"year": "2028"})
	c.Params = gin.Params{{Key: "year", Value: "2027"}}
	UpdateCompetitionYear(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "不能修改或删除")
}

// 目录不存在 → 404
func TestUpdateCompetitionYear_DirectoryMissing(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `competition_years` WHERE year = \\?.*FOR UPDATE").
		WithArgs(2027, 1).
		WillReturnRows(sqlmock.NewRows([]string{"year"}))
	expectYearCompetitions(mock, 2027, false)
	mock.ExpectRollback()

	_, w, c := buildCompPOSTJSON(map[string]string{"year": "2028"})
	c.Params = gin.Params{{Key: "year", Value: "2027"}}
	UpdateCompetitionYear(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "年度目录不存在")
}

// 新旧年份相同 → 直接成功，无 UPDATE
func TestUpdateCompetitionYear_SameYearNoOp(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `competition_years` WHERE year = \\?.*FOR UPDATE").
		WithArgs(2027, 1).
		WillReturnRows(sqlmock.NewRows([]string{"year"}).AddRow(2027))
	expectYearCompetitions(mock, 2027, false)
	mock.ExpectCommit()

	_, w, c := buildCompPOSTJSON(map[string]string{"year": "2027"})
	c.Params = gin.Params{{Key: "year", Value: "2027"}}
	UpdateCompetitionYear(c)

	require.Equal(t, http.StatusOK, w.Code)
}

// 目标年份已有赛事 → 400
func TestUpdateCompetitionYear_TargetOccupied(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `competition_years` WHERE year = \\?.*FOR UPDATE").
		WithArgs(2027, 1).
		WillReturnRows(sqlmock.NewRows([]string{"year"}).AddRow(2027))
	expectYearCompetitions(mock, 2027, false)
	expectYearCompetitions(mock, 2028, true)
	mock.ExpectRollback()

	_, w, c := buildCompPOSTJSON(map[string]string{"year": "2028"})
	c.Params = gin.Params{{Key: "year", Value: "2027"}}
	UpdateCompetitionYear(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "该年度目录已存在")
}

// 重命名成功
func TestUpdateCompetitionYear_Success(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `competition_years` WHERE year = \\?.*FOR UPDATE").
		WithArgs(2027, 1).
		WillReturnRows(sqlmock.NewRows([]string{"year"}).AddRow(2027))
	expectYearCompetitions(mock, 2027, false)
	expectYearCompetitions(mock, 2028, false)
	mock.ExpectExec("UPDATE `competition_years` SET `year`=\\?").
		WithArgs(2028, 2027).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	_, w, c := buildCompPOSTJSON(map[string]string{"year": "2028"})
	c.Params = gin.Params{{Key: "year", Value: "2027"}}
	UpdateCompetitionYear(c)

	require.Equal(t, http.StatusOK, w.Code)
}

// ===================== LookupManagersByWorkIDs =====================

func TestLookupManagersByWorkIDs_BindingError(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildCompPOSTJSON(map[string]string{})
	LookupManagersByWorkIDs(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "工号列表参数错误")
}

func TestLookupManagersByWorkIDs_BlankWorkIDRejected(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	// dive,required：空字符串工号在绑定阶段被拒
	_, w, c := buildCompPOSTJSON(map[string]interface{}{"work_ids": []string{""}})
	LookupManagersByWorkIDs(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "工号列表参数错误")
}

func TestLookupManagersByWorkIDs_EmptyResult(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	// 工号合法但查无此人 → 空列表
	mock.ExpectQuery("SELECT users.id,users.realname,users.username,users.college FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college"}))

	_, w, c := buildCompPOSTJSON(map[string]interface{}{"work_ids": []string{"T404"}})
	LookupManagersByWorkIDs(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "\"list\":[]")
}

func TestLookupManagersByWorkIDs_Success(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT users.id,users.realname,users.username,users.college FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college"}).
			AddRow(1, "张伟", "T001", "计算机学院"))
	mock.ExpectQuery("SELECT user_roles.user_id AS user_id, roles.role_code AS role_code FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_code"}).AddRow(1, "teacher"))

	_, w, c := buildCompPOSTJSON(map[string][]string{"work_ids": {"T001", "T001"}})
	LookupManagersByWorkIDs(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "张伟")
	require.Contains(t, w.Body.String(), "teacher")
}

func TestLookupManagersByWorkIDs_QueryError(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT users.id,users.realname,users.username,users.college FROM `users`").
		WillReturnError(assertErr("db error"))

	_, w, c := buildCompPOSTJSON(map[string][]string{"work_ids": {"T001"}})
	LookupManagersByWorkIDs(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestLookupManagersByWorkIDs_RoleQueryError(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT users.id,users.realname,users.username,users.college FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college"}).
			AddRow(1, "张伟", "T001", "计算机学院"))
	mock.ExpectQuery("SELECT user_roles.user_id AS user_id, roles.role_code AS role_code FROM `user_roles`").
		WillReturnError(assertErr("db error"))

	_, w, c := buildCompPOSTJSON(map[string][]string{"work_ids": {"T001"}})
	LookupManagersByWorkIDs(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}
