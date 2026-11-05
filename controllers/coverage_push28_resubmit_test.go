package controllers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// Resubmit：Save 之后的成员清理/入库/提交错误分支
func resubmitPrefix(mock sqlmock.Sqlmock, audit int) {
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).AddRow(5, 1, 1, 0))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "participant_type", "min_team_member", "max_team_member", "need_reg_audit", "reg_start_time", "reg_end_time"}).
			AddRow(1, 1, 1, 1, 5, audit, time.Now().Add(-time.Hour), time.Now().Add(time.Hour)))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `registers` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestResubmitRegistration_DeleteMembersError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()
	resubmitPrefix(mock, 1)
	mock.ExpectExec("UPDATE `reg_members` SET `delete_time`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(resubmitBody())
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

func TestResubmitRegistration_CreateMembersError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()
	resubmitPrefix(mock, 1)
	mock.ExpectExec("UPDATE `reg_members` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `reg_members`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	_, w, c := buildAuthPOSTJSON(resubmitBody())
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestResubmitRegistration_CommitError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()
	resubmitPrefix(mock, 1)
	mock.ExpectExec("UPDATE `reg_members` SET `delete_time`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `reg_members`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("commit fail"))

	_, w, c := buildAuthPOSTJSON(resubmitBody())
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
