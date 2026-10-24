package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"CompeManage_backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// 覆盖率补充 push39：CAS 回调、用户角色迁移工具、通知管理权限、模板参数的错误/边界分支。

// withCasConfig 临时替换 CAS 配置并注册恢复
func withCasConfig(t *testing.T, serverURL string) {
	t.Helper()
	previous := config.AppConfig.Cas
	config.AppConfig.Cas = config.CasConfig{
		ServerURL:    serverURL,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "http://localhost:8080/api/cas/callback",
		FrontendURL:  "http://fe.local",
	}
	t.Cleanup(func() { config.AppConfig.Cas = previous })
}

// newCasStubServer 返回按路径分发的 CAS 模拟服务：
// /oauth2.0/accessToken 返回 tokenOK；profilePath 由调用方自定义（构造 profile 失败/成功）。
func newCasStubServer(t *testing.T, profileHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2.0/accessToken":
			_, _ = w.Write([]byte("access_token=stub-token"))
		default:
			profileHandler(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestP39CasCallback_NoCode(t *testing.T) {
	withCasConfig(t, "http://cas.invalid")
	_, w, c := buildGET("/api/cas/callback?error=access_denied")
	CasCallback(c)
	require.Equal(t, http.StatusFound, w.Code)
	require.Contains(t, w.Header().Get("Location"), "cas_error=")
}

func TestP39CasCallback_ExchangeFail(t *testing.T) {
	withCasConfig(t, "http://127.0.0.1:1") // 不可达地址
	_, w, c := buildGET("/api/cas/callback?code=x")
	CasCallback(c)
	require.Equal(t, http.StatusFound, w.Code)
	require.Contains(t, w.Header().Get("Location"), "cas_error=%E8%8E%B7%E5%8F%96access_token%E5%A4%B1%E8%B4%A5")
}

func TestP39CasCallback_ProfileFail(t *testing.T) {
	server := newCasStubServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	withCasConfig(t, server.URL)
	_, w, c := buildGET("/api/cas/callback?code=x")
	CasCallback(c)
	require.Equal(t, http.StatusFound, w.Code)
	require.Contains(t, w.Header().Get("Location"), "cas_error=%E8%8E%B7%E5%8F%96%E7%94%A8%E6%88%B7%E4%BF%A1%E6%81%AF%E5%A4%B1%E8%B4%A5")
}

func TestP39CasCallback_NewUserButNoGuestRole(t *testing.T) {
	// profile 返回空属性 → 用户名空 → 本地查无此用户 → 访客角色缺失 → 处理用户信息失败
	server := newCasStubServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"attributes":[{"ID_NUMBER":"900009"}]}`))
	})
	withCasConfig(t, server.URL)
	mock := setupReportScopeDBMock(t)
	mock.ExpectQuery("SELECT .* FROM `users` WHERE username = ").WithArgs("900009", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `roles` WHERE role_code = ").WithArgs("guest", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, w, c := buildGET("/api/cas/callback?code=x")
	CasCallback(c)
	require.Equal(t, http.StatusFound, w.Code)
	require.Contains(t, w.Header().Get("Location"), "cas_error=%E5%A4%84%E7%90%86%E7%94%A8%E6%88%B7%E4%BF%A1%E6%81%AF%E5%A4%B1%E8%B4%A5")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestP39CasCallback_SuccessExistingSingleRole(t *testing.T) {
	server := newCasStubServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"attributes":[{"ID_NUMBER":"900001"},{"USER_NAME":"张三"},{"UNIT_NAME":"教务处"}]}`))
	})
	withCasConfig(t, server.URL)
	mock := setupReportScopeDBMock(t)
	mock.ExpectQuery("SELECT .* FROM `users` WHERE username = ").WithArgs("900001", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "identity_type"}).
			AddRow(7, "900001", "张三", "staff"))
	// Preload("Roles") 两段式：先查关联表，再按角色 id 查角色
	mock.ExpectQuery("SELECT .* FROM `user_roles` WHERE `user_roles`.`user_id` = ").WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(7, 1))
	mock.ExpectQuery("SELECT .* FROM `roles` WHERE `roles`.`id`").WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).
			AddRow(1, "school_admin", "校级管理员"))

	_, w, c := buildGET("/api/cas/callback?code=x")
	CasCallback(c)
	require.Equal(t, http.StatusFound, w.Code)
	loc := w.Header().Get("Location")
	require.Contains(t, loc, "http://fe.local/#/login?token=")
	require.Contains(t, loc, "role=school_admin")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyUserRoleMigration_InvalidBackupName(t *testing.T) {
	_, err := database.ApplyUserRoleMigration("bad-name!")
	require.ErrorContains(t, err, "备份表名")
}

func TestRollbackUserRoleMigration_InvalidBackupName(t *testing.T) {
	require.ErrorContains(t, database.RollbackUserRoleMigration(""), "必须提供有效备份表名")
	require.ErrorContains(t, database.RollbackUserRoleMigration("bad name"), "必须提供有效备份表名")
}

func TestCanManageNotice_Branches(t *testing.T) {
	pid := uint(99)
	notice := models.Notice{PublisherID: &pid}

	// 无 role_code：requestAccessScopeIfAvailable 返回 (nil, true)，按设计放行（真实请求必经 AuthRequired）
	_, _, c := buildGET("/api/notice/1")
	require.True(t, canManageNotice(c, notice))

	// 有 role_code 但缺 user_id：鉴权失败 -> false
	_, _, c0 := buildGET("/api/notice/1")
	c0.Set("role_code", "competition_manager")
	require.False(t, canManageNotice(c0, notice))

	// 校管/院管：true
	_, _, c2 := buildGET("/api/notice/1")
	c2.Set("user_id", uint(1))
	c2.Set("role_code", "school_admin")
	require.True(t, canManageNotice(c2, notice))

	_, _, c3 := buildGET("/api/notice/1")
	c3.Set("user_id", uint(1))
	c3.Set("role_code", "college_admin")
	managed := uint(2)
	c3.Set("managed_college_id", &managed)
	require.True(t, canManageNotice(c3, notice))

	// 负责人 + 通知无归属人：false（存量通知）
	_, _, c4 := buildGET("/api/notice/1")
	c4.Set("user_id", pid)
	c4.Set("role_code", "competition_manager")
	require.False(t, canManageNotice(c4, models.Notice{}))

	// 负责人 + 归属人是自己：true；归属他人：false
	require.True(t, canManageNotice(c4, notice))
	other := uint(100)
	require.False(t, canManageNotice(c4, models.Notice{PublisherID: &other}))
}

func TestExportAwardTemplate_MissingCompID(t *testing.T) {
	_, w, c := buildGET("/api/award/export-template")
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	ExportAwardTemplate(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
