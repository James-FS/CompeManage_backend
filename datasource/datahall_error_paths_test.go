package datasource

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ===================== SyncDepartments 错误/禁用分支 =====================

func TestSyncDepartments_ErrorPaths(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	items := `[
		{"D_STATIC_ORG_NAME":"","D_STATIC_ORG_CODE":"0011"},
		{"D_STATIC_ORG_NAME":"坏部门","D_STATIC_ORG_CODE":"0012","D_STATIC_ORG_IS_VALID":"0"},
		{"D_STATIC_ORG_NAME":"建院","D_STATIC_ORG_CODE":"0201","D_STATIC_ORG_IS_VALID":"1"},
		{"D_STATIC_ORG_NAME":"改院","D_STATIC_ORG_CODE":"0202","D_STATIC_ORG_IS_VALID":"1"},
		{"D_STATIC_ORG_NAME":"院办","D_STATIC_ORG_CODE":"0203","D_STATIC_ORG_IS_VALID":"1"}
	]`
	server := dataHallServer(t, deptResp(1, 1, 5, items))
	defer server.Close()
	withDataHall(t, server.URL)

	// 坏部门：禁用失败
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `departments` SET").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()
	// 建院：未命中 → 创建失败
	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `departments`").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()
	// 改院：命中 → 更新失败
	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename", "is_valid"}).
			AddRow(4, "旧名", "0202", "", true))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `departments` SET").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()
	// 院办：lookup 异常
	mock.ExpectQuery("SELECT .* FROM `departments`").WillReturnError(fmt.Errorf("conn lost"))
	mock.ExpectRollback()

	SyncDepartments()
}

// ===================== SyncStaff 错误分支 =====================

func TestSyncStaff_ErrorPaths(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	items := `[
		{"O_STAFF_BASIC_STAFFID":"T001","O_STAFF_BASIC_NAME":"建失败"},
		{"O_STAFF_BASIC_STAFFID":"T002","O_STAFF_BASIC_NAME":"老师"}
	]`
	server := dataHallServer(t, staffResp(1, 1, 2, items))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(3, "teacher"))
	// T001：创建失败
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `users`").WillReturnError(fmt.Errorf("dup"))
	mock.ExpectRollback()
	// T002：查询异常
	mock.ExpectQuery("SELECT .* FROM `users`").WillReturnError(fmt.Errorf("conn lost"))

	SyncStaff()
}

// ===================== SyncPostgrad 错误分支 =====================

func TestSyncPostgrad_ErrorPaths(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	items := `[
		{"O_STUDENT_BASIC_STUDENTID":"2023001","O_STUDENT_BASIC_NAME":"建失败"},
		{"O_STUDENT_BASIC_STUDENTID":"2023002","O_STUDENT_BASIC_NAME":"改不动"}
	]`
	server := dataHallServer(t, postgradResp(1, 1, 2, items))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "student"))
	// 2023001：创建失败
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `users`").WillReturnError(fmt.Errorf("dup"))
	mock.ExpectRollback()
	// 2023002：更新失败
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type", "realname", "college", "major", "grade"}).
			AddRow(2, "2023002", "postgraduate", "旧名", "", "", ""))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	SyncPostgrad()
}

// ===================== fetch 请求/解析错误分支 =====================

func TestFetchStaffPostgradDept_Errors(t *testing.T) {
	// staff 请求失败
	withDataHall(t, "http://127.0.0.1:1")
	if _, err := fetchAllStaff("tok"); err == nil {
		t.Fatal("staff 请求应失败")
	}

	// staff 解析失败（长预览）
	server := dataHallServer(t, "<html>bad</html>")
	defer server.Close()
	withDataHall(t, server.URL)
	if _, err := fetchAllStaff("tok"); err == nil {
		t.Fatal("staff 解析应失败")
	}

	// postgrad 解析失败
	if _, err := fetchAllPostgrad("tok"); err == nil {
		t.Fatal("postgrad 解析应失败")
	}

	// departments 请求失败
	withDataHall(t, "http://127.0.0.1:1")
	if _, err := fetchAllDepartments("tok"); err == nil {
		t.Fatal("departments 请求应失败")
	}
}
