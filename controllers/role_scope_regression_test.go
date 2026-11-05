package controllers

import (
	"CompeManage_backend/middleware"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetCompetitionListAlwaysScopesCollegeAdmin(t *testing.T) {
	mock := setupCompDBMock(t)
	managedCollegeID := uint(17)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories` WHERE .*comp_directories\\.college_id = \\?").
		WithArgs(managedCollegeID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `comp_directories` WHERE .*comp_directories\\.college_id = \\?").
		WithArgs(managedCollegeID, 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildCompGET("/api/comp/list?page=1&page_size=10")
	c.Set("user_id", uint(101))
	c.Set("role_code", "college_admin")
	c.Set("managed_college_id", &managedCollegeID)
	GetCompetitionList(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetUserListCollegeAdminLimitsStudentAndTeacherCandidates(t *testing.T) {
	previousRedis := middleware.RedisClient
	middleware.RedisClient = nil
	t.Cleanup(func() { middleware.RedisClient = previousRedis })

	for _, role := range []string{"student", "teacher"} {
		t.Run(role, func(t *testing.T) {
			mock := setupRegDBMock(t)
			managedCollegeID := uint(27)
			collegeName := "管理学院"
			mock.ExpectQuery("SELECT .* FROM `colleges`.*`colleges`\\.`id` = \\?.*LIMIT \\?").
				WithArgs(managedCollegeID, 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(managedCollegeID, collegeName))

			countPattern := "SELECT count\\(\\*\\) FROM `users`.*users\\.college = \\?"
			listPattern := "SELECT .* FROM `users`.*users\\.college = \\?"
			if role == "student" {
				mock.ExpectQuery(countPattern).WithArgs(role, collegeName).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(listPattern).WithArgs(role, collegeName, 10).
					WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college", "grade"}).
						AddRow(201, "本院学生", "S201", collegeName, "2026"))
			} else {
				mock.ExpectQuery(countPattern).WithArgs(collegeName).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(listPattern).WithArgs(collegeName, 10).
					WillReturnRows(sqlmock.NewRows([]string{"id", "realname", "username", "college", "grade"}).
						AddRow(202, "本院教师", "T202", collegeName, "教职员工"))
			}

			_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=" + role)
			c.Set("user_id", uint(102))
			c.Set("role_code", "college_admin")
			c.Set("managed_college_id", &managedCollegeID)
			GetUserList(c)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), collegeName)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
