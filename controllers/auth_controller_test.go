package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupAuthDBMock(t *testing.T) sqlmock.Sqlmock {
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

func buildAuthPOSTJSON(body interface{}) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
	jsonBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/", bytes.NewBuffer(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return req, w, c
}

func setupJWTSecret() {
	os.Setenv("JWT_SECRET", "test_secret_key_for_jwt_token_generation")
}

func hashTestPassword(password string) string {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash)
}

// ===================== checkPassword =====================

func TestCheckPassword(t *testing.T) {
	tests := []struct {
		name          string
		dbPassword    string
		inputPassword string
		expected      bool
	}{
		{
			name:          "密码正确",
			dbPassword:    hashTestPassword("123456"),
			inputPassword: "123456",
			expected:      true,
		},
		{
			name:          "密码错误",
			dbPassword:    hashTestPassword("123456"),
			inputPassword: "wrong_password",
			expected:      false,
		},
		{
			name:          "空密码比较",
			dbPassword:    hashTestPassword("123456"),
			inputPassword: "",
			expected:      false,
		},
		{
			name:          "两个都是空密码",
			dbPassword:    "",
			inputPassword: "",
			expected:      false,
		},
		{
			name:          "db密码为空",
			dbPassword:    "",
			inputPassword: "123456",
			expected:      false,
		},
		{
			name:          "特殊字符密码",
			dbPassword:    hashTestPassword("p@ssw0rd!#$%"),
			inputPassword: "p@ssw0rd!#$%",
			expected:      true,
		},
		{
			name:          "中文密码",
			dbPassword:    hashTestPassword("你好世界"),
			inputPassword: "你好世界",
			expected:      true,
		},
		{
			name:          "长密码",
			dbPassword:    hashTestPassword("this_is_a_very_long_password_that_should_still_work_correctly"),
			inputPassword: "this_is_a_very_long_password_that_should_still_work_correctly",
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkPassword(tt.dbPassword, tt.inputPassword)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ===================== Login =====================

func TestLogin_BadRequest(t *testing.T) {
	mock := setupAuthDBMock(t)
	defer mock.ExpectationsWereMet()

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{
			name: "缺少username",
			body: map[string]interface{}{"password": "123456"},
		},
		{
			name: "缺少password",
			body: map[string]interface{}{"username": "admin"},
		},
		{
			name: "空body",
			body: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, w, c := buildAuthPOSTJSON(tt.body)
			Login(c)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	mock := setupAuthDBMock(t)
	defer mock.ExpectationsWereMet()

	// 用户不存在
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}))

	body := map[string]interface{}{
		"username": "nonexistent",
		"password": "123456",
	}
	_, w, c := buildAuthPOSTJSON(body)
	Login(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "用户不存在")
}

func TestLogin_WrongPassword(t *testing.T) {
	mock := setupAuthDBMock(t)
	defer mock.ExpectationsWereMet()

	now := time.Now()
	// 返回用户，密码是 "correct_password" 的哈希
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "password", "college", "create_time", "update_time"}).
			AddRow(1, "admin", "管理员", hashTestPassword("correct_password"), "计算机学院", now, now))
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))

	body := map[string]interface{}{
		"username": "admin",
		"password": "wrong_password",
	}
	_, w, c := buildAuthPOSTJSON(body)
	Login(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "密码错误")
}

func TestLogin_Success_WithRole(t *testing.T) {
	mock := setupAuthDBMock(t)
	defer mock.ExpectationsWereMet()

	setupJWTSecret()

	now := time.Now()
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "password", "college", "create_time", "update_time"}).
			AddRow(1, "admin", "管理员", hashTestPassword("123456"), "计算机学院", now, now))
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 1))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).
			AddRow(1, "school_admin", "校级管理员"))

	body := map[string]interface{}{
		"username": "admin",
		"password": "123456",
	}
	_, w, c := buildAuthPOSTJSON(body)
	Login(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "token")
	assert.Contains(t, w.Body.String(), "school_admin")
	assert.Contains(t, w.Body.String(), "管理员")
}

func TestLogin_Forbidden_WithoutRole(t *testing.T) {
	mock := setupAuthDBMock(t)
	defer mock.ExpectationsWereMet()

	setupJWTSecret()

	now := time.Now()
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "password", "college", "create_time", "update_time"}).
			AddRow(2, "student", "学生", hashTestPassword("123456"), "数学学院", now, now))
	// 没有角色
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))

	body := map[string]interface{}{
		"username": "student",
		"password": "123456",
	}
	_, w, c := buildAuthPOSTJSON(body)
	Login(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "尚未分配角色")
}

func TestLogin_Error_MultipleRoles(t *testing.T) {
	mock := setupAuthDBMock(t)
	defer mock.ExpectationsWereMet()

	setupJWTSecret()

	now := time.Now()
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "password", "college", "create_time", "update_time"}).
			AddRow(3, "teacher", "老师", hashTestPassword("123456"), "计算机学院", now, now))
	// 多个角色属于需要迁移修复的数据异常
	mock.ExpectQuery("SELECT .* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).
			AddRow(3, 1).
			AddRow(3, 2))
	mock.ExpectQuery("SELECT .* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).
			AddRow(1, "competition_manager", "赛事负责人").
			AddRow(2, "college_admin", "院级管理员"))

	body := map[string]interface{}{
		"username": "teacher",
		"password": "123456",
	}
	_, w, c := buildAuthPOSTJSON(body)
	Login(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "token")
}
