package datasource

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// ===================== 教职工（staff） =====================

const twoStaff = `[{"O_STAFF_BASIC_STAFFID":"T001","O_STAFF_BASIC_NAME":"王老师","O_STAFF_BASIC_ORG":"计算机学院"},{"O_STAFF_BASIC_STAFFID":"T002","O_STAFF_BASIC_NAME":"李老师"}]`

func staffResp(page, maxPage, total int, items string) string {
	return fmt.Sprintf(`{"code":10000,"message":"ok","result":{"data":%s,"max_page":%d,"page":%d,"per_page":%d,"total":%d}}`, items, maxPage, page, perPage, total)
}

func TestGetStaffAccessToken_SuccessAndError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "" {
			tokenOK(w)
			return
		}
		fmt.Fprint(w, `{"code":40001,"message":"bad"}`)
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	tok, err := getStaffAccessToken()
	assert.NoError(t, err)
	assert.Equal(t, "tok123", tok)

	_, err = getStaffAccessToken()
	assert.NoError(t, err) // 命中缓存或再次成功均不报错
}

func TestGetStaffAccessToken_BadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not-json")
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	_, err := getStaffAccessToken()
	assert.ErrorContains(t, err, "token")
}

func TestFetchAllStaff_SinglePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "adsstaffbasic")
		fmt.Fprint(w, staffResp(1, 1, 2, twoStaff))
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	items, err := fetchAllStaff("tok")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(items))
}

func TestFetchAllStaff_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":50001,"message":"busy","result":{}}`)
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	_, err := fetchAllStaff("tok")
	assert.Error(t, err)
}

func TestSyncStaff_CreateAndSkip(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, staffResp(1, 1, 2, twoStaff))
	defer server.Close()
	withDataHall(t, server.URL)

	// teacher 角色
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(3, "teacher"))
	// T001 不存在 → 创建
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `users`").WillReturnResult(sqlmock.NewResult(20, 1))
	mock.ExpectExec("DELETE FROM `user_roles`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `user_roles`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	// T002 已存在且字段一致 → 跳过
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type", "realname", "college", "sex", "title"}).
			AddRow(2, "T002", "staff", "李老师", "", nil, nil))

	SyncStaff()
}

func TestSyncStaff_NoTeacherRole(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, staffResp(1, 1, 1, twoStaff))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnError(fmt.Errorf("record not found"))

	SyncStaff()
}

// ===================== 研究生（postgrad） =====================

const twoPostgrad = `[{"O_STUDENT_BASIC_STUDENTID":"2023001","O_STUDENT_BASIC_NAME":"研一","O_STUDENT_BASIC_COLLEGENAME":"数学院","O_STUDENT_BASIC_ENROLLYEAR":"2023"},{"O_STUDENT_BASIC_STUDENTID":"2023002","O_STUDENT_BASIC_NAME":"研二"}]`

func postgradResp(page, maxPage, total int, items string) string {
	return fmt.Sprintf(`{"code":10000,"message":"ok","result":{"data":%s,"max_page":%d,"page":%d,"per_page":%d,"total":%d}}`, items, maxPage, page, perPage, total)
}

func TestGetPostgradAccessToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenOK(w)
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	tok, err := getPostgradAccessToken()
	assert.NoError(t, err)
	assert.Equal(t, "tok123", tok)
}

func TestGetPostgradAccessToken_WrongCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":40001,"message":"bad"}`)
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	_, err := getPostgradAccessToken()
	assert.ErrorContains(t, err, "code=40001")
}

func TestFetchAllPostgrad_SinglePageAndError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "adsstudentpgbasic")
		fmt.Fprint(w, postgradResp(1, 1, 2, twoPostgrad))
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	items, err := fetchAllPostgrad("tok")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(items))

	// 第二次请求返回错误
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":50001,"message":"busy","result":{}}`)
	})
	_, err = fetchAllPostgrad("tok")
	assert.Error(t, err)
}

func TestSyncPostgrad_CreateAndQueryError(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, postgradResp(1, 1, 2, twoPostgrad))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "student"))
	// 2023001 → 创建
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `users`").WillReturnResult(sqlmock.NewResult(30, 1))
	mock.ExpectExec("DELETE FROM `user_roles`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `user_roles`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	// 2023002 → 查询失败计入 failed
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("conn lost"))

	SyncPostgrad()
}

// ===================== 学院/部门同步 =====================

func TestFetchAllColleges_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":50001,"message":"busy","result":{}}`)
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	_, err := fetchAllColleges("tok")
	assert.Error(t, err)
}

func TestFetchAllDepartments_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "broken")
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	_, err := fetchAllDepartments("tok")
	assert.Error(t, err)
}
