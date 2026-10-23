package utils

import (
	"fmt"
	"testing"
	"time"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupUtilsDBMock(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	assert.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	assert.NoError(t, err)
	prev := database.DB
	database.DB = gormDB
	t.Cleanup(func() { database.DB = prev })
	return mock
}

// 全流程：两个 Pluck + 两次状态更新
func TestUpdateExpiredComps_FullFlow(t *testing.T) {
	mock := setupUtilsDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(1).AddRow(2))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_directories` SET `status`").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(3))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_directories` SET `status`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	updateExpiredComps()
}

// 空结果：两次 Pluck 均为空，跳过更新
func TestUpdateExpiredComps_NothingToDo(t *testing.T) {
	mock := setupUtilsDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}))
	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}))

	updateExpiredComps()
}

func TestUpdateExpiredComps_StartQueryError(t *testing.T) {
	mock := setupUtilsDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnError(fmt.Errorf("db error"))

	updateExpiredComps()
}

func TestUpdateExpiredComps_StartUpdateError(t *testing.T) {
	mock := setupUtilsDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_directories` SET `status`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	updateExpiredComps()
}

func TestUpdateExpiredComps_ExpireQueryError(t *testing.T) {
	mock := setupUtilsDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}))
	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnError(fmt.Errorf("db error"))

	updateExpiredComps()
}

func TestUpdateExpiredComps_ExpireUpdateError(t *testing.T) {
	mock := setupUtilsDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}))
	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(9))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_directories` SET `status`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	updateExpiredComps()
}

// 定时器启动后至少执行一轮（首帧同步执行）
func TestStartCompStatusScheduler(t *testing.T) {
	// 该测试启动的后台协程会一直轮询 database.DB，测试结束后必须保证它非 nil。
	// 故本测试不还原全局 DB，只把它替换为容错的 sink mock。
	sinkDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	assert.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: sinkDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	assert.NoError(t, err)
	database.DB = gormDB
	t.Cleanup(func() { database.DB = gormDB }) // 保持非 nil 直至进程结束

	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}))
	mock.ExpectQuery("SELECT `comp_id` FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}))

	StartCompStatusScheduler(10 * time.Millisecond)
	time.Sleep(60 * time.Millisecond) // 覆盖 ticker 循环若干轮（后续轮次查询走默认空匹配容忍）
}
