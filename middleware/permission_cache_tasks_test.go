package middleware

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

func setupPermTaskDBMock(t *testing.T) sqlmock.Sqlmock {
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

// ===================== QueuePermissionCacheInvalidation =====================

func TestQueuePermissionCacheInvalidation_Success(t *testing.T) {
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `permission_cache_invalidation_tasks`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := QueuePermissionCacheInvalidation(5, fmt.Errorf("重载失败"))
	assert.NoError(t, err)
}

func TestQueuePermissionCacheInvalidation_NilCause(t *testing.T) {
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `permission_cache_invalidation_tasks`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	assert.NoError(t, QueuePermissionCacheInvalidation(6, nil))
}

func TestQueuePermissionCacheInvalidation_DBError(t *testing.T) {
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `permission_cache_invalidation_tasks`").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	assert.ErrorContains(t, QueuePermissionCacheInvalidation(7, nil), "db error")
}

// ===================== ProcessPermissionCacheInvalidationTasks =====================

func TestProcessPermTasks_QueryError(t *testing.T) {
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()
	withRedisClient(t, nil)

	mock.ExpectQuery("SELECT \\* FROM `permission_cache_invalidation_tasks`").
		WillReturnError(fmt.Errorf("db error"))

	ProcessPermissionCacheInvalidationTasks(0) // limit<=0 → 默认 50
}

// 执行失败（Redis 不可达）→ 更新重试计划
func TestProcessPermTasks_FailureUpdatesAttempts(t *testing.T) {
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()
	withRedisClient(t, unreachableRedis())

	mock.ExpectQuery("SELECT \\* FROM `permission_cache_invalidation_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "attempts"}).
			AddRow(1, 5, "pending", 0).
			AddRow(2, 6, "failed", 2))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `permission_cache_invalidation_tasks` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `permission_cache_invalidation_tasks` SET").
		WillReturnError(fmt.Errorf("update error"))
	mock.ExpectRollback()

	ProcessPermissionCacheInvalidationTasks(10)
}

// 成功路径：真实 Redis 清缓存 → 状态置 completed（不可用则跳过）
func TestProcessPermTasks_SuccessCompletesTask(t *testing.T) {
	rdb := localRedis(t)
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()
	withRedisClient(t, rdb)
	rdb.Set(t.Context(), "perm:99:*:x", "1", time.Minute)

	mock.ExpectQuery("SELECT \\* FROM `permission_cache_invalidation_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "attempts"}).
			AddRow(1, 99, "pending", 0))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `permission_cache_invalidation_tasks` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ProcessPermissionCacheInvalidationTasks(10)
}

// ===================== StartPermissionCacheInvalidationScheduler =====================

func TestStartPermTaskScheduler_DefaultInterval(t *testing.T) {
	sinkDB, sinkMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	assert.NoError(t, err)
	sinkGorm, err := gorm.Open(mysql.New(mysql.Config{Conn: sinkDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	assert.NoError(t, err)
	t.Cleanup(func() { database.DB = sinkGorm })

	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()
	withRedisClient(t, unreachableRedis())

	// 立即执行一轮（interval<=0 → 默认 1 分钟，ticker 不再触发）
	mock.ExpectQuery("SELECT \\* FROM `permission_cache_invalidation_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "attempts"}))

	stop := StartPermissionCacheInvalidationScheduler(0)
	defer stop() // 等协程退出后再进入 cleanup，避免与全局 DB 写竞争
	time.Sleep(50 * time.Millisecond)
	_ = sinkMock
}
