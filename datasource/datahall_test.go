package datasource

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupSyncDBMock(t *testing.T) sqlmock.Sqlmock {
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

// 配置 DataHall 指向 mock 服务器；隔离全局配置与 Redis
func withDataHall(t *testing.T, baseURL string) {
	t.Helper()
	origCfg := config.AppConfig
	origRedis := middleware.RedisClient
	middleware.RedisClient = nil
	t.Cleanup(func() {
		config.AppConfig = origCfg
		middleware.RedisClient = origRedis
	})
	config.AppConfig = &config.Config{DataHall: config.DataHallConfig{
		BaseURL: baseURL, Key: "k1", Secret: "s1",
	}}
}

func tokenOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"code":10000,"message":"ok","result":{"access_token":"tok123","expires_in":7000}}`)
}

// ===================== 纯函数 =====================

func TestDerefStr(t *testing.T) {
	assert.Equal(t, "", derefStr(nil))
	s := "abc"
	assert.Equal(t, "abc", derefStr(&s))
}

func TestLastNChars(t *testing.T) {
	assert.Equal(t, "abc", lastNChars("abc", 6))
	assert.Equal(t, "cdef", lastNChars("abcdef", 4))
}

func TestHashPassword(t *testing.T) {
	h, err := hashPassword("pwd123")
	assert.NoError(t, err)
	assert.NotEmpty(t, h)
	assert.NotEqual(t, "pwd123", h)
}

// ===================== getAccessToken =====================

func TestGetAccessToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/open_api/authentication/get_access_token", r.URL.Path)
		assert.Equal(t, "k1", r.URL.Query().Get("key"))
		tokenOK(w)
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	tok, err := getAccessToken()
	assert.NoError(t, err)
	assert.Equal(t, "tok123", tok)
}

func TestGetAccessToken_RequestError(t *testing.T) {
	withDataHall(t, "http://127.0.0.1:1") // 必然连接失败
	_, err := getAccessToken()
	assert.ErrorContains(t, err, "获取 token 请求失败")
}

func TestGetAccessToken_BadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html>not json</html>")
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	_, err := getAccessToken()
	assert.ErrorContains(t, err, "解析 token 响应失败")
}

func TestGetAccessToken_WrongCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":40001,"message":"bad secret"}`)
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	_, err := getAccessToken()
	assert.ErrorContains(t, err, "code=40001")
}

// Redis 命中缓存直接返回（本地 Redis 不可用则跳过）
func TestGetAccessToken_RedisCacheHit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("缓存命中时不应请求 token 接口")
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if rdb.Ping(t.Context()).Err() != nil {
		t.Skip("本地 Redis 不可用")
	}
	middleware.RedisClient = rdb
	rdb.Set(t.Context(), tokenCacheKey, "cached-token", time.Minute)

	tok, err := getAccessToken()
	assert.NoError(t, err)
	assert.Equal(t, "cached-token", tok)
}

// ===================== fetchAllStudents =====================

func studentResp(page, maxPage, total int, items string) string {
	return fmt.Sprintf(`{"code":10000,"message":"ok","result":{"data":%s,"max_page":%d,"page":%d,"per_page":%d,"total":%d}}`, items, maxPage, page, perPage, total)
}

const twoStudents = `[{"O_STUDENT_BASIC_STUDENTID":"2022001","O_STUDENT_BASIC_NAME":"张三"},{"O_STUDENT_BASIC_STUDENTID":"2022002","O_STUDENT_BASIC_NAME":"李四"}]`

func TestFetchAllStudents_SinglePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/open_api/customization/adsstudentugbasicnew_alpha/full", r.URL.Path)
		fmt.Fprint(w, studentResp(1, 1, 2, twoStudents))
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	items, err := fetchAllStudents("tok")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(items))
	assert.Equal(t, "2022001", *items[0].StudentID)
}

// 多页 + MinGrade 过滤条件注入
func TestFetchAllStudents_MultiPageWithMinGrade(t *testing.T) {
	pages := map[int]string{1: studentResp(1, 2, 3, twoStudents), 2: studentResp(2, 2, 3, `[{"O_STUDENT_BASIC_STUDENTID":"2022003"}]`)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		assert.NoError(t, readJSON(r, &body))
		assert.NotNil(t, body["O_STUDENT_BASIC_GRADE"], "MinGrade 非空时应带上过滤条件")
		page := int(body["page"].(float64))
		fmt.Fprint(w, pages[page])
	}))
	defer server.Close()
	withDataHall(t, server.URL)
	config.AppConfig.DataHall.MinGrade = "2022"

	items, err := fetchAllStudents("tok")
	assert.NoError(t, err)
	assert.Equal(t, 3, len(items))
}

// max_page=0 时按 total 推算页数
func TestFetchAllStudents_MaxPageZeroFallback(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, studentResp(1, 0, 2, twoStudents)) // total=2 → maxPage=1
		} else {
			fmt.Fprint(w, studentResp(2, 0, 0, `[]`))
		}
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	items, err := fetchAllStudents("tok")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(items))
	assert.Equal(t, 1, calls) // maxPage 推算为 1，不发第二页
}

func TestFetchAllStudents_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":50001,"message":"server busy","result":{}}`)
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	_, err := fetchAllStudents("tok")
	assert.ErrorContains(t, err, "API 返回错误")
}

func TestFetchAllStudents_BadJSONWithPreview(t *testing.T) {
	long := "<html>" + strings.Repeat("x", 600) + "</html>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, long)
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	_, err := fetchAllStudents("tok")
	assert.ErrorContains(t, err, "解析学生数据失败")
}

func TestFetchAllStudents_RequestError(t *testing.T) {
	withDataHall(t, "http://127.0.0.1:1")
	_, err := fetchAllStudents("tok")
	assert.ErrorContains(t, err, "请求学生数据失败")
}
