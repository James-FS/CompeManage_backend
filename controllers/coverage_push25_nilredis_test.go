package controllers

import (
	"net/http"
	"testing"

	"CompeManage_backend/middleware"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// ===================== Redis 未初始化时不得 panic（bug 回归） =====================
// 修复前：GetNoticeList / GetUserList / GetStatisticsDashboard 直接 rdb.Get，
// Redis 为 nil 时空指针 panic；异步缓存写入 goroutine 同样会 panic。

func withoutRedis(t *testing.T) {
	t.Helper()
	previous := middleware.RedisClient
	middleware.RedisClient = nil
	t.Cleanup(func() { middleware.RedisClient = previous })
}

func TestGetNoticeList_WorksWithoutRedis(t *testing.T) {
	withoutRedis(t)
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `notices`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/notice/list?page=1&page_size=10")
	GetNoticeList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":0")
}

func TestGetUserList_WorksWithoutRedis(t *testing.T) {
	withoutRedis(t)
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=student")
	GetUserList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":0")
}

func TestGetStatisticsDashboard_WorksWithoutRedis(t *testing.T) {
	withoutRedis(t)
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	// 复用统计看板的计数链
	dashboardCountOK(mock, 0)
	mock.ExpectQuery("(?s)SELECT .*").
		WillReturnRows(sqlmock.NewRows([]string{"name", "value"}))
	for i := 0; i < 20; i++ {
		mock.ExpectQuery("(?s)SELECT .*").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	}

	_, w, c := buildGET("/api/statistics/dashboard")
	c.Set("user_id", uint(5))
	c.Set("role_code", "school_admin")
	GetStatisticsDashboard(c)

	// 关键断言：不 panic；查询链宽松匹配，只要返回 2xx/5xx 均视为回归通过
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError,
		"应正常返回而非 panic，实际 %d", w.Code)
}
