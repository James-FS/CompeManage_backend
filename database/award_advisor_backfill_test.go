package database

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func batchQuery(mock sqlmock.Sqlmock, ids ...uint) {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range ids {
		rows.AddRow(id)
	}
	mock.ExpectQuery("SELECT `id` FROM `awards`").WillReturnRows(rows)
}

func TestBackfillAwardAdvisors_BatchQueryError(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectQuery("SELECT `id` FROM `awards`").WillReturnError(fmt.Errorf("db error"))
	assert.ErrorContains(t, BackfillAwardAdvisors(DB), "db error")
}

// 锁定时记录已被删 → 跳过该条，继续下一批
func TestBackfillAwardAdvisors_LockNotFoundContinues(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	batchQuery(mock, 1)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `awards`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()
	batchQuery(mock)
	assert.NoError(t, BackfillAwardAdvisors(DB))
}

func TestBackfillAwardAdvisors_LockError(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	batchQuery(mock, 1)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `awards`").WillReturnError(fmt.Errorf("lock fail"))
	mock.ExpectRollback()
	assert.ErrorContains(t, BackfillAwardAdvisors(DB), "lock fail")
}

func TestBackfillAwardAdvisors_CountError(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	batchQuery(mock, 1)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "advisor_info"}).AddRow(1, `[{"username":"T001"}]`))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_advisors`").WillReturnError(fmt.Errorf("cnt fail"))
	mock.ExpectRollback()
	assert.ErrorContains(t, BackfillAwardAdvisors(DB), "cnt fail")
}

// 已有结构化关联 → 跳过解析
func TestBackfillAwardAdvisors_HasRelationsSkip(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	batchQuery(mock, 1)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "advisor_info"}).AddRow(1, `[{"username":"T001"}]`))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_advisors`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectCommit()
	batchQuery(mock)
	assert.NoError(t, BackfillAwardAdvisors(DB))
}

// JSON 损坏 → 静默跳过
func TestBackfillAwardAdvisors_BadJSONSkip(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	batchQuery(mock, 1)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "advisor_info"}).AddRow(1, `not-json`))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_advisors`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectCommit()
	batchQuery(mock)
	assert.NoError(t, BackfillAwardAdvisors(DB))
}

func TestBackfillAwardAdvisors_UserQueryError(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	batchQuery(mock, 1)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "advisor_info"}).AddRow(1, `[{"username":"T001","name":"王"}]`))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_advisors`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `users`").WillReturnError(fmt.Errorf("conn lost"))
	mock.ExpectRollback()
	assert.ErrorContains(t, BackfillAwardAdvisors(DB), "conn lost")
}

// 各跳过分支 + 创建成功/失败
func TestBackfillAwardAdvisors_SkipAndCreatePaths(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	batchQuery(mock, 1)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "advisor_info"}).AddRow(1, `[
			{"username":" T001 ","name":"王老师"},
			{"username":"T001","name":"重复"},
			{"username":"","name":"空工号"},
			{"username":"T002","name":"不匹配的名字"},
			{"username":"T003","name":""},
			{"username":"T004","name":"会失败"}
		]`))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_advisors`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// T001 名字匹配 → 创建成功
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college"}).
			AddRow(31, "T001", "王老师", "计算机学院"))
	mock.ExpectExec("INSERT INTO `award_advisors`").WillReturnResult(sqlmock.NewResult(1, 1))
	// T002 名字不匹配 → 跳过
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college"}).
			AddRow(32, "T002", "李老师", "计算机学院"))
	// T003 名字为空 → 创建成功
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college"}).
			AddRow(33, "T003", "赵老师", "计算机学院"))
	mock.ExpectExec("INSERT INTO `award_advisors`").WillReturnResult(sqlmock.NewResult(2, 1))
	// T004 → 创建失败
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college"}).
			AddRow(34, "T004", "钱老师", "计算机学院"))
	mock.ExpectExec("INSERT INTO `award_advisors`").WillReturnError(fmt.Errorf("dup"))
	mock.ExpectRollback()

	assert.ErrorContains(t, BackfillAwardAdvisors(DB), "dup")
}

// 用户未命中 → 跳过
func TestBackfillAwardAdvisors_UserNotFoundSkip(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	batchQuery(mock, 1)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "advisor_info"}).AddRow(1, `[{"username":"T999"}]`))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `award_advisors`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()
	batchQuery(mock)
	assert.NoError(t, BackfillAwardAdvisors(DB))
}
