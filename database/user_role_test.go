package database

import (
	"errors"
	"fmt"
	"testing"

	"CompeManage_backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	gosqlmysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupUserDBMock(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	assert.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	assert.NoError(t, err)
	prev := DB
	DB = gormDB
	t.Cleanup(func() { DB = prev })
	return mock
}

// ===================== TransactionWithDeadlockRetry =====================

func TestTransactionWithDeadlockRetry_NilDB(t *testing.T) {
	err := TransactionWithDeadlockRetry(nil, func(tx *gorm.DB) error { return nil })
	assert.EqualError(t, err, "数据库连接为空")
}

func TestTransactionWithDeadlockRetry_Success(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectBegin()
	mock.ExpectCommit()

	calls := 0
	err := TransactionWithDeadlockRetry(DB, func(tx *gorm.DB) error {
		calls++
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
}

// 死锁 1213 → 整体重试直至成功
func TestTransactionWithDeadlockRetry_DeadlockThenSuccess(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectBegin()
	mock.ExpectRollback()
	mock.ExpectBegin()
	mock.ExpectCommit()

	calls := 0
	err := TransactionWithDeadlockRetry(DB, func(tx *gorm.DB) error {
		calls++
		if calls == 1 {
			return &gosqlmysql.MySQLError{Number: 1213, Message: "deadlock"}
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, calls)
}

// 非死锁错误不重试
func TestTransactionWithDeadlockRetry_OtherErrorNoRetry(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectBegin()
	mock.ExpectRollback()

	calls := 0
	err := TransactionWithDeadlockRetry(DB, func(tx *gorm.DB) error {
		calls++
		return fmt.Errorf("boom")
	})
	assert.ErrorContains(t, err, "boom")
	assert.Equal(t, 1, calls)
}

// 连续死锁耗尽重试次数
func TestTransactionWithDeadlockRetry_Exhausted(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	for i := 0; i < 3; i++ {
		mock.ExpectBegin()
		mock.ExpectRollback()
	}

	calls := 0
	err := TransactionWithDeadlockRetry(DB, func(tx *gorm.DB) error {
		calls++
		return &gosqlmysql.MySQLError{Number: 1213}
	})
	assert.Error(t, err)
	assert.Equal(t, 3, calls)
}

// ===================== SetUserRole =====================

func TestSetUserRole_InvalidArgs(t *testing.T) {
	assert.EqualError(t, SetUserRole(nil, 1, 2), "数据库连接为空")
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	assert.EqualError(t, SetUserRole(DB, 0, 2), "用户ID和角色ID必须大于0")
	assert.EqualError(t, SetUserRole(DB, 1, 0), "用户ID和角色ID必须大于0")
}

func TestSetUserRole_Success(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	// Delete 与 Create 各自隐式事务
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `user_roles`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `user_roles`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	assert.NoError(t, SetUserRole(DB, 3, 7))
}

func TestSetUserRole_DeleteError(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `user_roles`").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	err := SetUserRole(DB, 3, 7)
	assert.ErrorContains(t, err, "删除旧用户角色失败")
}

func TestSetUserRole_CreateError(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `user_roles`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `user_roles`").WillReturnError(fmt.Errorf("dup"))
	mock.ExpectRollback()

	err := SetUserRole(DB, 3, 7)
	assert.ErrorContains(t, err, "写入新用户角色失败")
}

// ===================== GetSingleUserRole =====================

func TestGetSingleUserRole_Cases(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()

	// 0 个角色
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}))
	_, err := GetSingleUserRole(DB, 1)
	assert.ErrorIs(t, err, ErrUserHasNoRole)

	// 1 个角色
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(5, "student"))
	role, err := GetSingleUserRole(DB, 1)
	assert.NoError(t, err)
	assert.Equal(t, uint(5), role.ID)

	// 多角色 → 明确报错
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(1, "student").AddRow(2, "teacher"))
	_, err = GetSingleUserRole(DB, 1)
	assert.ErrorIs(t, err, ErrUserHasMultipleRoles)

	// 查询错误
	mock.ExpectQuery("SELECT .* FROM `roles`").WillReturnError(errors.New("db error"))
	_, err = GetSingleUserRole(DB, 1)
	assert.ErrorContains(t, err, "db error")
}

var _ = models.UserRole{}
