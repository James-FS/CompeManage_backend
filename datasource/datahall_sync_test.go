package datasource

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func readJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// token + 数据接口共用的 mock 服务器
func dataHallServer(t *testing.T, dataJSON string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/open_api/authentication/get_access_token":
			tokenOK(w)
		default:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, dataJSON)
		}
	}))
}

// ===================== SyncStudents =====================

func TestSyncStudents_TokenError(t *testing.T) {
	withDataHall(t, "http://127.0.0.1:1")
	SyncStudents() // 仅覆盖失败即返回路径
}

func TestSyncStudents_FetchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open_api/authentication/get_access_token":
			tokenOK(w)
		default:
			fmt.Fprint(w, `{"code":50001,"message":"busy"}`)
		}
	}))
	defer server.Close()
	withDataHall(t, server.URL)

	SyncStudents()
}

// token 与数据都正常，但缺 student 角色 → 终止
func TestSyncStudents_NoStudentRole(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, studentResp(1, 1, 1, twoStudents))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnError(fmt.Errorf("record not found"))

	SyncStudents()
}

// 新建用户：INSERT users + SetUserRole（Delete/Insert user_roles）在同一事务
func TestSyncStudents_CreateUser(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, studentResp(1, 1, 1, `[{"O_STUDENT_BASIC_STUDENTID":"2022001","O_STUDENT_BASIC_NAME":"张三","O_STUDENT_BASIC_COLLEGENAME":"计算机学院"}]`))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "student"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // 空结果 → gorm.ErrRecordNotFound
	// TransactionWithDeadlockRetry → 事务
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `users`").WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectExec("DELETE FROM `user_roles`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `user_roles`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	SyncStudents()
}

// 空学号 → 计入失败；identity_type 非 student → 跳过
func TestSyncStudents_SkipCases(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, studentResp(1, 1, 2, `[{"O_STUDENT_BASIC_NAME":"无学号"},{"O_STUDENT_BASIC_STUDENTID":"T001","O_STUDENT_BASIC_NAME":"老师"}]`))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "student"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type", "realname"}).AddRow(1, "T001", "teacher", "老师"))

	SyncStudents()
}

// 字段无变化 → 跳过；有变化 → UPDATE；更新失败 → 计入失败
func TestSyncStudents_UpdatePaths(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, studentResp(1, 1, 3, `[
		{"O_STUDENT_BASIC_STUDENTID":"2022001","O_STUDENT_BASIC_NAME":"同名"},
		{"O_STUDENT_BASIC_STUDENTID":"2022002","O_STUDENT_BASIC_NAME":"新名"},
		{"O_STUDENT_BASIC_STUDENTID":"2022003","O_STUDENT_BASIC_NAME":"改不动"}
	]`))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "student"))
	// 2022001：全部字段一致 → 跳过
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type", "realname", "college", "major", "grade"}).
			AddRow(1, "2022001", "student", "同名", "", "", ""))
	// 2022002：realname 不同 → UPDATE
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type", "realname", "college", "major", "grade"}).
			AddRow(2, "2022002", "student", "旧名", "", "", ""))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// 2022003：UPDATE 失败 → failed
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type", "realname", "college", "major", "grade"}).
			AddRow(3, "2022003", "student", "旧名", "", "", ""))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	SyncStudents()
}

// 用户查询异常 → 计入失败
func TestSyncStudents_UserQueryError(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, studentResp(1, 1, 1, `[{"O_STUDENT_BASIC_STUDENTID":"2022009","O_STUDENT_BASIC_NAME":"张"}]`))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "student"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("conn lost"))

	SyncStudents()
}

// 创建失败 → 计入失败
func TestSyncStudents_CreateError(t *testing.T) {
	mock := setupSyncDBMock(t)
	defer mock.ExpectationsWereMet()
	server := dataHallServer(t, studentResp(1, 1, 1, `[{"O_STUDENT_BASIC_STUDENTID":"2022010","O_STUDENT_BASIC_NAME":"李"}]`))
	defer server.Close()
	withDataHall(t, server.URL)

	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "student"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `users`").WillReturnError(fmt.Errorf("dup key"))
	mock.ExpectRollback()

	SyncStudents()
}
