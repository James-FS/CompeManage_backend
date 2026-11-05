package datasource

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// ===================== SyncColleges 全分支 =====================

func collegeResp(page, maxPage, total int, items string) string {
	return fmt.Sprintf(`{"code":10000,"message":"ok","result":{"data":%s,"max_page":%d,"page":%d,"per_page":%d,"total":%d}}`, items, maxPage, page, perPage, total)
}

func TestSyncColleges_AllPaths(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	items := `[
		{"D_STATIC_ORG_NAME":""},
		{"D_STATIC_ORG_NAME":"非学术机构","D_STATIC_ORG_CODE":"9999"},
		{"D_STATIC_ORG_NAME":"旧学院","D_STATIC_ORG_CODE":"0201","D_STATIC_ORG_IS_VALID":"0"},
		{"D_STATIC_ORG_NAME":"坏学院","D_STATIC_ORG_CODE":"0202","D_STATIC_ORG_IS_VALID":"0"},
		{"D_STATIC_ORG_NAME":"计算机学院","D_STATIC_ORG_CODE":"0203","D_STATIC_ORG_IS_VALID":"1","D_STATIC_ORG_ENAME":"cs"},
		{"D_STATIC_ORG_NAME":"新名学院","D_STATIC_ORG_CODE":"0204","D_STATIC_ORG_IS_VALID":"1","D_STATIC_ORG_ENAME":"x"},
		{"D_STATIC_ORG_NAME":"外语学院","D_STATIC_ORG_CODE":"0205","D_STATIC_ORG_IS_VALID":"1"},
		{"D_STATIC_ORG_NAME":"数学学院","D_STATIC_ORG_IS_VALID":"1"},
		{"D_STATIC_ORG_NAME":"新学院","D_STATIC_ORG_CODE":"0206","D_STATIC_ORG_IS_VALID":"1"},
		{"D_STATIC_ORG_NAME":"建院","D_STATIC_ORG_CODE":"0207","D_STATIC_ORG_IS_VALID":"1"}
	]`
	server := dataHallServer(t, collegeResp(1, 1, 10, items))
	defer server.Close()
	withDataHall(t, server.URL)

	// item3 禁用成功
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `colleges` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// item4 禁用失败
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `colleges` SET").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()
	// item5 code 命中且无变化 → 跳过
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename", "is_valid", "category"}).
			AddRow(5, "计算机学院", "0203", "cs", true, "college"))
	// item6 code 命中但改名 → 更新成功
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename", "is_valid", "category"}).
			AddRow(6, "旧名", "0204", "", true, "college"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `colleges` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// item7 更新失败
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename", "is_valid", "category"}).
			AddRow(7, "外语", "0205", "", true, "college"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `colleges` SET").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()
	// item8 code 为空 → 按 name 回退命中 → 更新
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename", "is_valid", "category"}).
			AddRow(8, "数学学院", "", "", true, "college"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `colleges` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// item9 两段查找都未命中 → 创建
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `colleges`").WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectCommit()
	// item10 创建失败
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `colleges`").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	SyncColleges()
}

func TestSyncColleges_TokenFetchEmpty(t *testing.T) {
	// token 失败
	withDataHall(t, "http://127.0.0.1:1")
	SyncColleges()

	// 拉取失败
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open_api/authentication/get_access_token" {
			tokenOK(w)
			return
		}
		fmt.Fprint(w, `{"code":50001,"message":"busy"}`)
	}))
	defer server.Close()
	withDataHall(t, server.URL)
	SyncColleges()

	// 空数据跳过
	empty := dataHallServer(t, collegeResp(1, 1, 0, `[]`))
	defer empty.Close()
	withDataHall(t, empty.URL)
	SyncColleges()
}

// ===================== SyncDepartments =====================

func deptResp(page, maxPage, total int, items string) string {
	return fmt.Sprintf(`{"code":10000,"message":"ok","result":{"data":%s,"max_page":%d,"page":%d,"per_page":%d,"total":%d}}`, items, maxPage, page, perPage, total)
}

func TestSyncDepartments_CreateUpdateSkip(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	items := `[
		{"D_STATIC_ORG_NAME":"教务办公室","D_STATIC_ORG_CODE":"0012","D_STATIC_ORG_IS_VALID":"1"},
		{"D_STATIC_ORG_NAME":"科研办公室","D_STATIC_ORG_CODE":"0412","D_STATIC_ORG_IS_VALID":"1","D_STATIC_ORG_ENAME":"rd"},
		{"D_STATIC_ORG_NAME":"院办","D_STATIC_ORG_CODE":"0013","D_STATIC_ORG_IS_VALID":"0"},
		{"D_STATIC_ORG_NAME":"档案室","D_STATIC_ORG_CODE":"0014","D_STATIC_ORG_IS_VALID":"1"}
	]`
	server := dataHallServer(t, deptResp(1, 1, 4, items))
	defer server.Close()
	withDataHall(t, server.URL)

	// 教务办公室：两段查找都未命中 → 创建
	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `departments`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	// 科研办公室：code 命中但有变化 → 更新
	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename", "is_valid"}).
			AddRow(2, "旧名", "0412", "", true))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `departments` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// 院办：无效 → 禁用
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `departments` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// 档案室：命中且无变化 → 跳过
	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename", "is_valid"}).
			AddRow(4, "档案室", "0014", "", true))

	SyncDepartments()
}

func TestSyncDepartments_TokenAndEmpty(t *testing.T) {
	withDataHall(t, "http://127.0.0.1:1")
	SyncDepartments()

	empty := dataHallServer(t, deptResp(1, 1, 0, `[]`))
	defer empty.Close()
	withDataHall(t, empty.URL)
	SyncDepartments()
}

// ===================== fetch 分页/错误分支 =====================

func TestFetchAllColleges_PaginationAndErrors(t *testing.T) {
	// max_page=0 按 total 推算 2 页
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, collegeResp(1, 0, 2, `[{"D_STATIC_ORG_NAME":"甲","D_STATIC_ORG_CODE":"0201"}]`))
		} else {
			fmt.Fprint(w, collegeResp(2, 0, 0, `[]`))
		}
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	items, err := fetchAllColleges("tok")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(items))

	// 请求失败
	withDataHall(t, "http://127.0.0.1:1")
	_, err = fetchAllColleges("tok")
	assert.ErrorContains(t, err, "请求机构数据失败")

	// 解析失败（长预览分支）
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html>"+string(make([]byte, 600))+"</html>")
	}))
	defer bad.Close()
	withDataHall(t, bad.URL)
	_, err = fetchAllColleges("tok")
	assert.ErrorContains(t, err, "解析机构数据失败")
}

func TestFetchAllStaff_Postgrad_Pagination(t *testing.T) {
	// staff 多页（maxPage=2）
	pages := map[int]string{1: staffResp(1, 2, 2, twoStaff), 2: staffResp(2, 2, 2, `[]`)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = readJSON(r, &body)
		page := int(body["page"].(float64))
		fmt.Fprint(w, pages[page])
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	items, err := fetchAllStaff("tok")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(items))

	// postgrad 请求失败
	withDataHall(t, "http://127.0.0.1:1")
	_, err = fetchAllPostgrad("tok")
	assert.Error(t, err)
}
