package datasource

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// 教职工：字段变化→更新；身份非 staff→跳过；更新失败→计失败
func TestSyncStaff_UpdateAndSkipPaths(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, staffResp(1, 1, 3, `[
		{"O_STAFF_BASIC_STAFFID":"T001","O_STAFF_BASIC_NAME":"同名"},
		{"O_STAFF_BASIC_STAFFID":"T002","O_STAFF_BASIC_NAME":"新名"},
		{"O_STAFF_BASIC_STAFFID":"T003","O_STAFF_BASIC_NAME":"改不动"}
	]`))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(3, "teacher"))
	// T001 无变化 → 跳过
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type", "realname", "college", "sex", "title"}).
			AddRow(1, "T001", "staff", "同名", "", nil, nil))
	// T002 有变化 → UPDATE 成功
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type", "realname", "college", "sex", "title"}).
			AddRow(2, "T002", "staff", "旧名", "", nil, nil))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// T003 更新失败 → failed
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type", "realname", "college", "sex", "title"}).
			AddRow(3, "T003", "staff", "旧名", "", nil, nil))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	SyncStaff()
}

// 教职工：identity 非 staff → 跳过；空工号 → 计失败
func TestSyncStaff_IdentitySkipAndEmptyID(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, staffResp(1, 1, 2, `[
		{"O_STAFF_BASIC_NAME":"无工号"},
		{"O_STAFF_BASIC_STAFFID":"S001","O_STAFF_BASIC_NAME":"学生兼职"}
	]`))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(3, "teacher"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).
			AddRow(1, "S001", "student"))

	SyncStaff()
}

// 研究生：字段变化→更新；identity 非 postgraduate→跳过
func TestSyncPostgrad_UpdateAndIdentitySkip(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, postgradResp(1, 1, 3, `[
		{"O_STUDENT_BASIC_STUDENTID":"2023001","O_STUDENT_BASIC_NAME":"同名"},
		{"O_STUDENT_BASIC_STUDENTID":"2023002","O_STUDENT_BASIC_NAME":"新名"},
		{"O_STUDENT_BASIC_STUDENTID":"2023003","O_STUDENT_BASIC_NAME":"别的身份"}
	]`))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "student"))
	// 2023001 无变化 → 跳过
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type", "realname", "college", "major", "grade"}).
			AddRow(1, "2023001", "postgraduate", "同名", "", "", ""))
	// 2023002 有变化 → UPDATE
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type", "realname", "college", "major", "grade"}).
			AddRow(2, "2023002", "postgraduate", "旧名", "", "", ""))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// 2023003 身份非 postgraduate → 跳过
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).
			AddRow(3, "2023003", "student"))

	SyncPostgrad()
}

// token 失败即返回
func TestSyncStaffAndPostgrad_TokenError(t *testing.T) {
	withDataHall(t, "http://127.0.0.1:1")
	SyncStaff()
	SyncPostgrad()
}
