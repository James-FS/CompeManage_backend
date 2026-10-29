package controllers

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// ===================== GetNoticeList =====================

func noticeCacheKey(compID, isLatest, status, start, end string, page, size int) string {
	raw := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d", compID, isLatest, status, start, end, page, size)
	return fmt.Sprintf("cache:notice_list:%x", md5.Sum([]byte(raw)))
}

func TestGetNoticeList_CacheHit(t *testing.T) {
	rdb := ensureRedisForUserList(t)
	rdb.FlushDB(t.Context())

	key := noticeCacheKey("", "true", "", "", "", 1, 10)
	rdb.Set(t.Context(), key, `{"list":[],"total":3}`, 0)

	_, w, c := buildGET("/api/notice/list?page=1&page_size=10")
	GetNoticeList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":3")
}

func TestGetNoticeList_AllFiltersAsc(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `notices`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/notice/list?compID=1&is_latest=false&status=1&start_time=2026-01-01&end_time=2026-12-31&page=0&page_size=999")
	GetNoticeList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetNoticeList_FindError(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `notices`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `notices`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/notice/list?compID=1")
	GetNoticeList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== GetUserList 缓存命中 =====================

func TestGetUserList_CacheHit(t *testing.T) {
	rdb := ensureRedisForUserList(t)
	rdb.FlushDB(t.Context())

	raw := fmt.Sprintf("%s|%s|%d|%d|college:%s", "teacher:v2", "", 1, 10, "")
	key := fmt.Sprintf("cache:reg_user_list:%x", md5.Sum([]byte(raw)))
	rdb.Set(t.Context(), key, `{"list":[],"total":2}`, 0)

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=teacher")
	GetUserList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"total\":2")
}

// ===================== GetManagerList =====================

func TestGetManagerList_AllFilters(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT users.id,users.realname,users.username,users.college FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college"}).
			AddRow(1, "张伟", "T001", "计算机学院"))
	mock.ExpectQuery("SELECT user_roles.user_id AS user_id, roles.role_code AS role_code FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_code"}).AddRow(1, "teacher"))

	_, w, c := buildGET("/api/comp/manager/list?page=1&page_size=10&name=张&work_id=T001&college=计算机学院&manager_only=true")
	GetManagerList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "张伟")
}

func TestGetManagerList_FindError(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT users.id,users.realname,users.username,users.college FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/api/comp/manager/list?page=1&page_size=10")
	GetManagerList(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
