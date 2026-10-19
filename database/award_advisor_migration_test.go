package database

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestBackfillAwardAdvisorsUsesOnlyExplicitWorkIDs(t *testing.T) {
	for _, tc := range []struct {
		name, data    string
		existing      int
		keyed, insert bool
	}{
		{"name alone never grants access", `[{"name":"张老师"}]`, 0, false, false},
		{"verified work ID", `[{"username":"T001","name":"张老师"}]`, 0, true, true},
		{"conflicting historical name", `[{"username":"T001","name":"其他老师"}]`, 0, true, false},
		{"existing relations authoritative", `[{"username":"T001","name":"张老师"}]`, 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := DB
			t.Cleanup(func() { DB = original })
			mock := setupUserRoleMigrationDBMock(t)
			mock.ExpectQuery("SELECT `id` FROM `awards`.*NOT EXISTS").WithArgs(0, 200).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT .* FROM `awards`.*FOR UPDATE").WithArgs(7, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "advisor_info"}).AddRow(7, tc.data))
			mock.ExpectQuery("SELECT count.*FROM `award_advisors`").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.existing))
			if tc.keyed {
				mock.ExpectQuery("SELECT .* FROM `users`.*identity_type = 'staff'").WithArgs("T001", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college"}).AddRow(11, "T001", "张老师", "学院甲"))
			}
			if tc.insert {
				mock.ExpectExec("INSERT INTO `award_advisors`").WillReturnResult(sqlmock.NewResult(1, 1))
			}
			mock.ExpectCommit()
			mock.ExpectQuery("SELECT `id` FROM `awards`.*NOT EXISTS").WithArgs(7, 200).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			require.NoError(t, BackfillAwardAdvisors(DB))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
