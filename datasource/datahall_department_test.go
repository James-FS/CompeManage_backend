package datasource

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestSyncDepartmentsValidity(t *testing.T) {
	for _, status := range []string{"1", "0", ""} {
		t.Run("status="+status, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer sqlDB.Close()
			db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
			require.NoError(t, err)
			oldDB, oldRedis, oldBaseURL, oldClient := database.DB, middleware.RedisClient, config.AppConfig.DataHall.BaseURL, httpClient
			t.Cleanup(func() {
				database.DB, middleware.RedisClient, config.AppConfig.DataHall.BaseURL, httpClient = oldDB, oldRedis, oldBaseURL, oldClient
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/open_api/authentication/get_access_token" {
					_, _ = w.Write([]byte(`{"code":10000,"result":{"access_token":"test-token","expires_in":7200}}`))
					return
				}
				_, _ = w.Write([]byte(`{"code":10000,"result":{"max_page":1,"data":[{"D_STATIC_ORG_CODE":"0002","D_STATIC_ORG_NAME":"计算机部门","D_STATIC_ORG_ENAME":"CS","D_STATIC_ORG_IS_VALID":"` + status + `"}]}}`))
			}))
			defer server.Close()
			database.DB, middleware.RedisClient, config.AppConfig.DataHall.BaseURL, httpClient = db, nil, server.URL, server.Client()
			if status == "1" {
				// 已存在但禁用的部门必须重新启用，即使名称等信息未变。
				mock.ExpectQuery("SELECT .* FROM `departments` WHERE code = ").WithArgs("0002", 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename", "is_valid"}).AddRow(8, "计算机部门", "0002", "CS", false))
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta("UPDATE `departments` SET `code`=?,`ename`=?,`is_valid`=?,`name`=? WHERE `id` = ?")).
					WithArgs("0002", "CS", true, "计算机部门", 8).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			} else {
				// 无效或缺少状态时不新增部门，禁用旧记录且不影响其他同名代码。
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta("UPDATE `departments` SET `is_valid`=? WHERE ((code = ? AND code <> '') OR (name = ? AND (code = '' OR code IS NULL))) AND is_valid = ?")).
					WithArgs(false, "0002", "计算机部门", true).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			SyncDepartments()
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
