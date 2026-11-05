package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// GetAdvisorAwards 查询错误分支
func TestGetAdvisorAwards2_QueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	expectAdvisorUser(mock)
	mock.ExpectQuery("SELECT .* FROM `awards`").WillReturnError(fmt.Errorf("db error"))

	c, w := advisorTestContext("GET", "teacher", nil)
	GetAdvisorAwards(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
