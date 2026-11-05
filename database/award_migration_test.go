package database

import (
	"fmt"
	"testing"

	"CompeManage_backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// ===================== AwardProjectKey =====================

func TestAwardProjectKey(t *testing.T) {
	k1 := AwardProjectKey("  项目A  ")
	k2 := AwardProjectKey("项目A")
	assert.Equal(t, k1, k2, "应忽略首尾空白")
	assert.Equal(t, 64, len(k1), "sha256 hex 长度")
	assert.NotEqual(t, k1, AwardProjectKey("项目B"))
}

// ===================== EnsureAwardMembers =====================

func TestEnsureAwardMembers_AwardNotFound(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectQuery("SELECT .* FROM `awards`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	assert.Error(t, EnsureAwardMembers(DB, 9))
}

func TestEnsureAwardMembers_AlreadyMigrated(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "members_migrated"}).AddRow(1, true))
	assert.NoError(t, EnsureAwardMembers(DB, 1))
}

// 全流程：历史项目名回退 + 队长合并 + 未匹配成员 + 成员入库 + 标记迁移
func TestEnsureAwardMembers_FullFlow(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id", "project_name", "members_migrated"}).
			AddRow(1, 5, "", false))
	// Preload：Register → Leader(users) → Members(reg_members)
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_name", "leader_id"}).AddRow(5, "", 2))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(2, "S2"))
	mock.ExpectQuery("SELECT .* FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id", "student_id"}).
			AddRow(1, 5, "S1").AddRow(2, 5, "S9"))
	// 回退历史项目名
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// 三个成员：S1 匹配、S9 未匹配、S2(队长合并)
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname"}).AddRow(11, "S1", "学生一"))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT (IGNORE )?INTO `award_members`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname"}).AddRow(12, "S2", "学生二"))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT (IGNORE )?INTO `award_members`").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()
	// 标记已迁移
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	assert.NoError(t, EnsureAwardMembers(DB, 1))
}

// 项目名取报名队名；成员与队长为空时只做标记
func TestEnsureAwardMembers_UsesTeamNameAndNoMembers(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id", "project_name", "members_migrated"}).
			AddRow(1, 5, "", false))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_name", "leader_id"}).AddRow(5, "组队名", 0))
	mock.ExpectQuery("SELECT .* FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `awards` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	assert.NoError(t, EnsureAwardMembers(DB, 1))
}

func TestEnsureAwardMembers_MemberQueryError(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id", "project_name", "members_migrated"}).
			AddRow(1, 5, "项目A", false))
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_name", "leader_id"}).AddRow(5, "队", 0))
	mock.ExpectQuery("SELECT .* FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id", "student_id"}).AddRow(1, 5, "S1"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("conn lost"))

	assert.ErrorContains(t, EnsureAwardMembers(DB, 1), "conn lost")
}

// ===================== BackfillAwardMembers =====================

func TestBackfillAwardMembers_EmptyList(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT (IGNORE )?INTO `award_import_settings`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT `id` FROM `awards`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()

	assert.NoError(t, BackfillAwardMembers(DB))
}

func TestBackfillAwardMembers_SettingError(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT (IGNORE )?INTO `award_import_settings`").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	assert.Error(t, BackfillAwardMembers(DB))
}

func TestBackfillAwardMembers_PluckError(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT (IGNORE )?INTO `award_import_settings`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT `id` FROM `awards`").WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	assert.Error(t, BackfillAwardMembers(DB))
}

func TestBackfillAwardMembers_EnsureError(t *testing.T) {
	mock := setupUserDBMock(t)
	defer mock.ExpectationsWereMet()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT (IGNORE )?INTO `award_import_settings`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT `id` FROM `awards`").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `awards`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	assert.Error(t, BackfillAwardMembers(DB))
}

var _ = models.AwardMember{}
