package controllers

import (
	"CompeManage_backend/middleware"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCompetitionListFiltersHostingYear(t *testing.T) {
	mock := setupCompDBMock(t)
	mr := miniredis.RunT(t)
	previous := middleware.RedisClient
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	middleware.RedisClient = client
	t.Cleanup(func() { client.Close(); middleware.RedisClient = previous })
	// 年度筛选必须使用举办年份，而非创建时间；空结果不需要加载关联表。
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories` WHERE comp_directories.year = \\?").
		WithArgs("2025").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `comp_directories` WHERE comp_directories.year = \\?").
		WithArgs("2025", 10).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, w, c := buildCompGET("/api/comp/list?page=1&page_size=10&year=2025")
	GetCompetitionList(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestParseDirectoryYear(t *testing.T) {
	for _, value := range []string{"", "0000", "0999", "202", "20260", "abcd", "+026", "２０２６", "2026 "} {
		_, valid := parseDirectoryYear(value)
		require.False(t, valid, value)
	}
	for _, value := range []string{"1000", "2026", "9999"} {
		_, valid := parseDirectoryYear(value)
		require.True(t, valid, value)
	}
}

func expectYearCompetitions(mock sqlmock.Sqlmock, year int, occupied bool) {
	// 精确匹配：不得使用软删除条件，已删除赛事也应阻止修改。
	rows := sqlmock.NewRows([]string{"id"})
	if occupied {
		rows.AddRow(42)
	}
	mock.ExpectQuery("SELECT `id` FROM `comp_directories` WHERE year = \\? FOR UPDATE").
		WithArgs(year).WillReturnRows(rows)
}

func TestCreateCompetitionYear(t *testing.T) {
	for _, name := range []string{"empty", "competition_exists", "directory_exists"} {
		t.Run(name, func(t *testing.T) {
			mock := setupCompDBMock(t)
			mock.ExpectBegin()
			expectYearCompetitions(mock, 2027, name == "competition_exists")
			if name != "competition_exists" {
				exec := mock.ExpectExec("INSERT INTO `competition_years`").WithArgs(2027)
				if name == "directory_exists" {
					exec.WillReturnError(&mysql.MySQLError{Number: 1062})
				} else {
					exec.WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			if name == "empty" {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			_, w, c := buildCompPOSTJSON(map[string]string{"year": "2027"})
			CreateCompetitionYear(c)
			if name == "empty" {
				require.Equal(t, http.StatusOK, w.Code)
			} else {
				require.Equal(t, http.StatusBadRequest, w.Code)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestMutateCompetitionYear(t *testing.T) {
	for _, deleting := range []bool{false, true} {
		for _, name := range []string{"empty", "occupied", "missing", "target_occupied", "target_exists"} {
			if deleting && (name == "target_occupied" || name == "target_exists") {
				continue
			}
			t.Run(name+map[bool]string{true: "_delete", false: "_rename"}[deleting], func(t *testing.T) {
				mock := setupCompDBMock(t)
				mock.ExpectBegin()
				rows := sqlmock.NewRows([]string{"year"})
				if name != "missing" {
					rows.AddRow(2027)
				}
				mock.ExpectQuery("SELECT \\* FROM `competition_years` WHERE year = \\?.*FOR UPDATE").
					WithArgs(2027, 1).WillReturnRows(rows)
				expectYearCompetitions(mock, 2027, name == "occupied")
				if name == "empty" || name == "target_occupied" || name == "target_exists" {
					if deleting {
						mock.ExpectExec("DELETE FROM `competition_years`").WithArgs(2027).WillReturnResult(sqlmock.NewResult(0, 1))
					} else {
						expectYearCompetitions(mock, 2028, name == "target_occupied")
						if name != "target_occupied" {
							exec := mock.ExpectExec("UPDATE `competition_years` SET `year`=\\?").WithArgs(2028, 2027)
							if name == "target_exists" {
								exec.WillReturnError(&mysql.MySQLError{Number: 1062})
							} else {
								exec.WillReturnResult(sqlmock.NewResult(0, 1))
							}
						}
					}
				}
				if name == "empty" {
					mock.ExpectCommit()
				} else {
					mock.ExpectRollback()
				}
				_, w, c := buildCompPOSTJSON(map[string]string{"year": "2028"})
				c.Params = gin.Params{{Key: "year", Value: "2027"}}
				mutateCompetitionYear(c, deleting)
				status := http.StatusBadRequest
				if name == "empty" {
					status = http.StatusOK
				}
				if name == "missing" {
					status = http.StatusNotFound
				}
				require.Equal(t, status, w.Code, w.Body.String())
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestDirectoryInvalidRequests(t *testing.T) {
	mock := setupCompDBMock(t)
	for _, body := range []interface{}{map[string]string{"year": "0000"}, map[string]int{"year": 2026}} {
		_, w, c := buildCompPOSTJSON(body)
		CreateCompetitionYear(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	}
	_, w, c := buildCompGET("/api/comp/years/abcd")
	c.Params = gin.Params{{Key: "year", Value: "abcd"}}
	DeleteCompetitionYear(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
