package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// requireAwardAdvisorUser 的错误分支
func TestRequireAwardAdvisorUser2_NoUserID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/x")
	_, ok := requireAwardAdvisorUser(c)
	assert.False(t, ok)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAwardAdvisorUser2_QueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `users`").WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildGET("/x")
	c.Set("user_id", uint(11))
	_, ok := requireAwardAdvisorUser(c)
	assert.False(t, ok)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireAwardAdvisorUser2_NotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/x")
	c.Set("user_id", uint(11))
	_, ok := requireAwardAdvisorUser(c)
	assert.False(t, ok)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
