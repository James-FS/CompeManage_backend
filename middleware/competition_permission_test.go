package middleware

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAnyPermissionOwnGrantAndRevocation(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		name := "revoked"
		if allowed {
			name = "own permission"
		}
		t.Run(name, func(t *testing.T) {
			mock := setupAuthMiddlewareDBMock(t)
			t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
			previousRedis := RedisClient
			RedisClient = nil
			t.Cleanup(func() { RedisClient = previousRedis })
			rows := sqlmock.NewRows([]string{"code"})
			if allowed {
				rows.AddRow("summary:edit:own")
			}
			mock.ExpectQuery("SELECT .* FROM `permissions` JOIN role_permissions").WithArgs(uint(2), uint64(5)).WillReturnRows(rows)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/summary/1", nil)
			c.Set("user_id", uint(10))
			c.Set("role_id", uint(2))
			c.Set("permission_version", uint64(5))
			RequireAnyPermission("summary:edit", "summary:edit:own")(c)
			if allowed {
				require.False(t, c.IsAborted())
				codes, err := UserPermissionCodes(c)
				require.NoError(t, err)
				require.Equal(t, []string{"summary:edit:own"}, codes)
			} else {
				require.True(t, c.IsAborted())
				require.Equal(t, http.StatusForbidden, w.Code)
			}
		})
	}
}
