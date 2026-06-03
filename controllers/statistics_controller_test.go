package controllers

import (
	"net/http"
	"testing"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupStatsDBMock(t *testing.T) sqlmock.Sqlmock {
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

// ===================== calcPercent =====================

func TestCalcPercent(t *testing.T) {
	tests := []struct {
		name        string
		numerator   int64
		denominator int64
		expected    float64
	}{
		{name: "zero denominator", numerator: 10, denominator: 0, expected: 0},
		{name: "both zero", numerator: 0, denominator: 0, expected: 0},
		{name: "100 percent", numerator: 100, denominator: 100, expected: 100},
		{name: "50 percent", numerator: 1, denominator: 2, expected: 50},
		{name: "33.33 percent", numerator: 1, denominator: 3, expected: 33.33},
		{name: "0 percent", numerator: 0, denominator: 100, expected: 0},
		{name: "small fraction", numerator: 1, denominator: 3, expected: 33.33},
		{name: "large numbers", numerator: 12345, denominator: 100000, expected: 12.34},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calcPercent(tt.numerator, tt.denominator)
			assert.InDelta(t, tt.expected, result, 0.01)
		})
	}
}

// ===================== maxInt64 =====================

func TestMaxInt64(t *testing.T) {
	tests := []struct {
		name     string
		a        int64
		b        int64
		expected int64
	}{
		{name: "a greater", a: 10, b: 5, expected: 10},
		{name: "b greater", a: 3, b: 7, expected: 7},
		{name: "equal", a: 5, b: 5, expected: 5},
		{name: "both negative", a: -3, b: -7, expected: -3},
		{name: "negative and positive", a: -1, b: 1, expected: 1},
		{name: "both zero", a: 0, b: 0, expected: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, maxInt64(tt.a, tt.b))
		})
	}
}

// ===================== GetStatisticsDashboard =====================

func TestGetStatisticsDashboard_Unauthorized(t *testing.T) {
	mock := setupStatsDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/statistics/dashboard")
	GetStatisticsDashboard(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "未登录")
}
