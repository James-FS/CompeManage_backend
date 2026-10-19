package routes

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdvisorRoutesRegisterAndRequireAuthentication(t *testing.T) {
	r := gin.New()
	require.NotPanics(t, func() { SetupRoutes(r) })
	for _, path := range []string{"/api/award/advisor/list", "/api/award/advisor/7", "/api/award/7/changes"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 401, w.Code)
	}
}
