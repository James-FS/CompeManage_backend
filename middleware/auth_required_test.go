package middleware

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"CompeManage_backend/utils"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func authContext(t *testing.T, header string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)
	if header != "" {
		c.Request.Header.Set("Authorization", header)
	}
	return c, w
}

func validBearer(t *testing.T) string {
	t.Helper()
	viper.Set("jwt.secret", "unit-test-secret")
	tok, err := utils.GenerateToken(7, "U007", "student")
	assert.NoError(t, err)
	return "Bearer " + tok
}

func TestAuthRequired_MissingHeader(t *testing.T) {
	c, w := authContext(t, "")
	AuthRequired()(c)
	assert.Equal(t, 401, w.Code)
	assert.Contains(t, w.Body.String(), "请先登录")
}

func TestAuthRequired_MalformedHeader(t *testing.T) {
	c, w := authContext(t, "Token abc")
	AuthRequired()(c)
	assert.Equal(t, 401, w.Code)
	assert.Contains(t, w.Body.String(), "Token格式错误")
}

func TestAuthRequired_InvalidToken(t *testing.T) {
	c, w := authContext(t, "Bearer not-a-jwt")
	AuthRequired()(c)
	assert.Equal(t, 401, w.Code)
	assert.Contains(t, w.Body.String(), "Token无效或已过期")
}

func TestAuthRequired_DBError(t *testing.T) {
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `users`").WillReturnError(fmt.Errorf("db error"))

	c, w := authContext(t, validBearer(t))
	AuthRequired()(c)
	assert.Equal(t, 500, w.Code)
}

func TestAuthRequired_RoleCountInvalid(t *testing.T) {
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()

	cols := []string{"role_id", "permission_version", "role_code", "managed_college_id"}
	// 2 行异常
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows(cols).AddRow(1, uint64(1), "student", nil).AddRow(2, uint64(1), "teacher", nil))
	// 0 行异常
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows(cols))

	c, w := authContext(t, validBearer(t))
	AuthRequired()(c)
	assert.Equal(t, 403, w.Code)

	c2, w2 := authContext(t, validBearer(t))
	AuthRequired()(c2)
	assert.Equal(t, 403, w2.Code)
}

func TestAuthRequired_Success(t *testing.T) {
	mock := setupPermTaskDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_id", "permission_version", "role_code", "managed_college_id"}).
			AddRow(3, uint64(5), "student", nil))

	c, w := authContext(t, validBearer(t))
	AuthRequired()(c)
	assert.Equal(t, 200, w.Code)
	assert.False(t, c.IsAborted())
	uid, _ := c.Get("user_id")
	assert.Equal(t, uint(7), uid)
	rc, _ := c.Get("role_code")
	assert.Equal(t, "student", rc)
}
