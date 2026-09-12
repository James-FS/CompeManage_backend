package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"CompeManage_backend/config"
	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupCasDBMock(t *testing.T) sqlmock.Sqlmock {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	assert.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	assert.NoError(t, err)
	database.DB = gormDB
	return mock
}

func buildCasGET(urlStr string) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
	req := httptest.NewRequest("GET", urlStr, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return req, w, c
}

// ===================== casProfileResponse.getAttr =====================

func TestCasProfileResponse_GetAttr(t *testing.T) {
	tests := []struct {
		name       string
		attributes []map[string]interface{}
		key        string
		expected   string
	}{
		{
			name: "存在字段",
			attributes: []map[string]interface{}{
				{"ID_NUMBER": "2021001"},
				{"USER_NAME": "张三"},
			},
			key:      "ID_NUMBER",
			expected: "2021001",
		},
		{
			name: "不存在字段",
			attributes: []map[string]interface{}{
				{"ID_NUMBER": "2021001"},
			},
			key:      "EMAIL",
			expected: "",
		},
		{
			name:       "空属性列表",
			attributes: []map[string]interface{}{},
			key:        "ID_NUMBER",
			expected:   "",
		},
		{
			name:       "nil属性列表",
			attributes: nil,
			key:        "ID_NUMBER",
			expected:   "",
		},
		{
			name: "多个属性数组",
			attributes: []map[string]interface{}{
				{"ID_NUMBER": "2021001"},
				{"USER_NAME": "张三"},
				{"UNIT_NAME": "计算机学院"},
			},
			key:      "UNIT_NAME",
			expected: "计算机学院",
		},
		{
			name: "数值类型字段",
			attributes: []map[string]interface{}{
				{"AGE": 20},
			},
			key:      "AGE",
			expected: "20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &casProfileResponse{
				ID:         "test",
				Attributes: tt.attributes,
			}
			result := profile.getAttr(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ===================== redirectWithError =====================

func TestRedirectWithError(t *testing.T) {
	tests := []struct {
		name        string
		frontendURL string
		userMsg     string
		expectedURL string
	}{
		{
			name:        "正常重定向",
			frontendURL: "http://localhost:5219",
			userMsg:     "授权失败",
			expectedURL: "http://localhost:5219/#/login?cas_error=%E6%8E%88%E6%9D%83%E5%A4%B1%E8%B4%A5",
		},
		{
			name:        "带特殊字符的消息",
			frontendURL: "http://localhost:5219",
			userMsg:     "Token生成失败",
			expectedURL: "http://localhost:5219/#/login?cas_error=Token%E7%94%9F%E6%88%90%E5%A4%B1%E8%B4%A5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, w, c := buildCasGET("/")
			redirectWithError(c, tt.frontendURL, fmt.Errorf("原始错误"), tt.userMsg)

			assert.Equal(t, http.StatusFound, w.Code)
			assert.Equal(t, tt.expectedURL, w.Header().Get("Location"))
		})
	}
}

// ===================== CasLogin =====================

func TestCasLogin(t *testing.T) {
	// 保存原始配置
	origConfig := config.AppConfig
	defer func() { config.AppConfig = origConfig }()

	config.AppConfig.Cas = config.CasConfig{
		ServerURL:   "http://cas.example.com",
		ClientID:    "test_client",
		RedirectURI: "http://localhost:8080/api/auth/cas/callback",
	}

	_, w, c := buildCasGET("/api/auth/cas/login")
	CasLogin(c)

	assert.Equal(t, http.StatusFound, w.Code)

	location := w.Header().Get("Location")
	assert.Contains(t, location, "http://cas.example.com/oauth2.0/authorize")
	assert.Contains(t, location, "client_id=test_client")
	assert.Contains(t, location, "response_type=code")
}

// ===================== exchangeCodeForToken =====================

func TestExchangeCodeForToken_Success(t *testing.T) {
	// 创建 mock HTTP 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/oauth2.0/accessToken", r.URL.Path)

		r.ParseForm()
		assert.Equal(t, "test_client", r.Form.Get("client_id"))
		assert.Equal(t, "test_secret", r.Form.Get("client_secret"))
		assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
		assert.Equal(t, "test_code", r.Form.Get("code"))

		w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		fmt.Fprint(w, "access_token=test_token_123&expires=3600")
	}))
	defer server.Close()

	cfg := config.CasConfig{
		ServerURL:    server.URL,
		ClientID:     "test_client",
		ClientSecret: "test_secret",
	}

	token, err := exchangeCodeForToken(cfg, "test_code")
	assert.NoError(t, err)
	assert.Equal(t, "test_token_123", token)
}

func TestExchangeCodeForToken_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer server.Close()

	cfg := config.CasConfig{
		ServerURL: server.URL,
	}

	_, err := exchangeCodeForToken(cfg, "test_code")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status=500")
}

func TestExchangeCodeForToken_EmptyToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		fmt.Fprint(w, "expires=3600")
	}))
	defer server.Close()

	cfg := config.CasConfig{
		ServerURL: server.URL,
	}

	_, err := exchangeCodeForToken(cfg, "test_code")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access_token为空")
}

func TestExchangeCodeForToken_InvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		fmt.Fprint(w, "%%invalid%%")
	}))
	defer server.Close()

	cfg := config.CasConfig{
		ServerURL: server.URL,
	}

	_, err := exchangeCodeForToken(cfg, "test_code")
	assert.Error(t, err)
}

func TestExchangeCodeForToken_ServerDown(t *testing.T) {
	cfg := config.CasConfig{
		ServerURL: "http://localhost:1", // 不存在的服务器
	}

	_, err := exchangeCodeForToken(cfg, "test_code")
	assert.Error(t, err)
}

// ===================== fetchCasUserProfile =====================

func TestFetchCasUserProfile_Success(t *testing.T) {
	profile := casProfileResponse{
		ID: "user123",
		Attributes: []map[string]interface{}{
			{"ID_NUMBER": "2021001"},
			{"USER_NAME": "张三"},
			{"UNIT_NAME": "计算机学院"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "/oauth2.0/profile")
		assert.Contains(t, r.URL.RawQuery, "access_token=test_token")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profile)
	}))
	defer server.Close()

	cfg := config.CasConfig{
		ServerURL: server.URL,
	}

	result, err := fetchCasUserProfile(cfg, "test_token")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "user123", result.ID)
	assert.Equal(t, "2021001", result.getAttr("ID_NUMBER"))
	assert.Equal(t, "张三", result.getAttr("USER_NAME"))
}

func TestFetchCasUserProfile_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "unauthorized")
	}))
	defer server.Close()

	cfg := config.CasConfig{
		ServerURL: server.URL,
	}

	_, err := fetchCasUserProfile(cfg, "invalid_token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status=401")
}

func TestFetchCasUserProfile_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	cfg := config.CasConfig{
		ServerURL: server.URL,
	}

	_, err := fetchCasUserProfile(cfg, "test_token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "解析用户信息JSON失败")
}

func TestFetchCasUserProfile_EmptyIDNumber(t *testing.T) {
	profile := casProfileResponse{
		ID: "user123",
		Attributes: []map[string]interface{}{
			{"USER_NAME": "张三"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profile)
	}))
	defer server.Close()

	cfg := config.CasConfig{
		ServerURL: server.URL,
	}

	_, err := fetchCasUserProfile(cfg, "test_token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ID_NUMBER为空")
}

func TestFetchCasUserProfile_ServerDown(t *testing.T) {
	cfg := config.CasConfig{
		ServerURL: "http://localhost:1",
	}

	_, err := fetchCasUserProfile(cfg, "test_token")
	assert.Error(t, err)
}

// ===================== findOrCreateUser =====================

func TestFindOrCreateUser_ExistingUser(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "password", "college", "create_time", "update_time"}).
			AddRow(1, "2021001", "张三", "hashed_password", "计算机学院", now, now))
	// GORM Preload 查询关联表
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 1))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).
			AddRow(1, "student", "学生"))

	casUser := &casProfileResponse{
		ID: "user123",
		Attributes: []map[string]interface{}{
			{"ID_NUMBER": "2021001"},
			{"USER_NAME": "张三"},
			{"UNIT_NAME": "计算机学院"},
		},
	}

	user, err := findOrCreateUser(nil, casUser)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, uint(1), user.ID)
	assert.Equal(t, "2021001", user.Username)
}

func TestFindOrCreateUser_ExistingUser_UpdateRealname(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()
	// 返回的用户没有 realname
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "password", "college", "create_time", "update_time"}).
			AddRow(1, "2021001", "", "hashed_password", "计算机学院", now, now))
	// GORM Preload 查询关联表
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))
	// 期望更新 realname
	mock.ExpectExec("UPDATE `users` SET `realname`").
		WillReturnResult(sqlmock.NewResult(1, 1))

	casUser := &casProfileResponse{
		ID: "user123",
		Attributes: []map[string]interface{}{
			{"ID_NUMBER": "2021001"},
			{"USER_NAME": "张三"},
		},
	}

	user, err := findOrCreateUser(nil, casUser)
	assert.NoError(t, err)
	assert.NotNil(t, user)
}

func TestFindOrCreateUser_NewUser(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	// 用户不存在
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "password", "college"}))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WithArgs("guest", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_name", "role_code"}).
			AddRow(7, "访客", "guest"))
	// GORM Create 使用事务
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `users`").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("DELETE FROM `user_roles` WHERE user_id = ?").
		WithArgs(uint(2)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `user_roles`").
		WithArgs(uint(7), uint(2)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	casUser := &casProfileResponse{
		ID: "user456",
		Attributes: []map[string]interface{}{
			{"ID_NUMBER": "2021002"},
			{"USER_NAME": "李四"},
			{"UNIT_NAME": "数学学院"},
		},
	}

	user, err := findOrCreateUser(nil, casUser)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "2021002", user.Username)
	assert.Equal(t, "李四", user.Realname)
	assert.Equal(t, "数学学院", user.College)
}

func TestFindOrCreateUser_CreateError(t *testing.T) {
	mock := setupCasDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WithArgs("guest", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_name", "role_code"}).
			AddRow(7, "访客", "guest"))
	// GORM Create 使用事务
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `users`").
		WillReturnError(fmt.Errorf("duplicate key"))
	mock.ExpectRollback()

	casUser := &casProfileResponse{
		ID: "user789",
		Attributes: []map[string]interface{}{
			{"ID_NUMBER": "2021003"},
			{"USER_NAME": "王五"},
		},
	}

	_, err := findOrCreateUser(nil, casUser)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "创建用户失败")
}

func TestDefaultRoleCodeForIdentity(t *testing.T) {
	tests := []struct {
		identityType string
		wantRole     string
	}{
		// P1-2：教职工默认角色改为 teacher（赛事负责人按需提升）
		{identityType: "staff", wantRole: "teacher"},
		{identityType: "student", wantRole: "student"},
		{identityType: "postgraduate", wantRole: "student"},
		{identityType: "external", wantRole: "guest"},
	}

	for _, tt := range tests {
		t.Run(tt.identityType, func(t *testing.T) {
			assert.Equal(t, tt.wantRole, defaultRoleCodeForIdentity(tt.identityType))
		})
	}
}

// ===================== CasCallback =====================

func TestCasCallback_NoCode(t *testing.T) {
	// 保存原始配置
	origConfig := config.AppConfig
	defer func() { config.AppConfig = origConfig }()

	config.AppConfig.Cas = config.CasConfig{
		FrontendURL: "http://localhost:5219",
	}

	_, w, c := buildCasGET("/api/auth/cas/callback?error=access_denied")
	CasCallback(c)

	assert.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	assert.Contains(t, location, "cas_error=")
}

func TestCasCallback_WithCode_TokenExchangeFails(t *testing.T) {
	// 保存原始配置
	origConfig := config.AppConfig
	defer func() { config.AppConfig = origConfig }()

	// 创建 mock CAS 服务器，返回错误
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "server error")
	}))
	defer server.Close()

	config.AppConfig.Cas = config.CasConfig{
		ServerURL:    server.URL,
		FrontendURL:  "http://localhost:5219",
		ClientID:     "test",
		ClientSecret: "test",
	}

	_, w, c := buildCasGET("/api/auth/cas/callback?code=test_code")
	CasCallback(c)

	assert.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	assert.Contains(t, location, "cas_error=")
}

func TestCasCallback_WithCode_ProfileFetchFails(t *testing.T) {
	// 保存原始配置
	origConfig := config.AppConfig
	defer func() { config.AppConfig = origConfig }()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			// 第一次请求：token 交换成功
			w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
			fmt.Fprint(w, "access_token=test_token&expires=3600")
		} else {
			// 第二次请求：获取用户信息失败
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "unauthorized")
		}
	}))
	defer server.Close()

	config.AppConfig.Cas = config.CasConfig{
		ServerURL:    server.URL,
		FrontendURL:  "http://localhost:5219",
		ClientID:     "test",
		ClientSecret: "test",
	}

	_, w, c := buildCasGET("/api/auth/cas/callback?code=test_code")
	CasCallback(c)

	assert.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	assert.Contains(t, location, "cas_error=")
}
