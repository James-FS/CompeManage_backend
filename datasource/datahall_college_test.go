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

func TestSyncCollegesValidity(t *testing.T) {
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
				_, _ = w.Write([]byte(`{"code":10000,"result":{"max_page":1,"data":[{"D_STATIC_ORG_CODE":"0206","D_STATIC_ORG_NAME":"计算机学院","D_STATIC_ORG_ENAME":"CS","D_STATIC_ORG_IS_VALID":"` + status + `"}]}}`))
			}))
			defer server.Close()
			database.DB, middleware.RedisClient, config.AppConfig.DataHall.BaseURL, httpClient = db, nil, server.URL, server.Client()
			if status == "1" {
				// 已存在但禁用的学院必须重新启用，即使名称等信息未变。
				mock.ExpectQuery("SELECT .* FROM `colleges` WHERE code = ").WithArgs("0206", 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "ename", "is_valid"}).AddRow(8, "计算机学院", "0206", "CS", false))
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta("UPDATE `colleges` SET `category`=?,`code`=?,`ename`=?,`is_valid`=?,`name`=? WHERE `id` = ?")).
					WithArgs("college", "0206", "CS", true, "计算机学院", 8).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			} else {
				// 无效或缺少状态时不新增学院，禁用旧记录且不影响其他同名代码。
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta("UPDATE `colleges` SET `is_valid`=? WHERE (code = ? OR (name = ? AND (code = '' OR code IS NULL))) AND is_valid = ?")).
					WithArgs(false, "0206", "计算机学院", true).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			SyncColleges()
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCollegeOrgType(t *testing.T) {
	// 分类头行（两位代码）不参与
	require.Equal(t, "", collegeOrgType("02", "学院"))
	require.Equal(t, "", collegeOrgType("00", "职能管理部门"))
	require.Equal(t, "", collegeOrgType("04", "教辅单位"))
	// 党政机关 00xx / 教辅部门 04xx（含实验教学中心）
	require.Equal(t, "government", collegeOrgType("0040", "教务处（含非学历教育办学管理办公室）"))
	require.Equal(t, "government", collegeOrgType("0092", "教师教育学部"))
	require.Equal(t, "auxiliary", collegeOrgType("0480", "实验教学中心"))
	require.Equal(t, "auxiliary", collegeOrgType("0470", "图书馆（知识产权信息服务中心）"))
	// 学院段：仅名称含“学院”的参与
	require.Equal(t, "college", collegeOrgType("0206", "计算机科学与网络工程学院"))
	require.Equal(t, "", collegeOrgType("0229", "社科部")) // 0201~0298 内但名称不含学院
	// 其他类别（科研/后勤/直属/二级学院等）不参与
	require.Equal(t, "", collegeOrgType("0301", "广州大学计算科技研究院"))
	require.Equal(t, "", collegeOrgType("1101", "广州大学岭南统计科学研究院"))
	require.Equal(t, "", collegeOrgType("0701", "广州大学附属中学"))
	require.Equal(t, "", collegeOrgType("0802", "广州大学华软软件学院"))
	// 历史无效记录仍按原范围识别（有效性由 is_valid 单独控制）
	require.Equal(t, "college", collegeOrgType("0210", "旅游学院（中法旅游学院）（已撤销）"))
}
