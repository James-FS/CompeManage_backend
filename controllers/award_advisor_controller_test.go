package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func advisorTestContext(method, role string, body interface{}) (*gin.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/api/award/advisor/7", bytes.NewReader(b))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", uint(11))
	c.Set("role_code", role)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	return c, w
}

func expectAdvisorUser(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT .* FROM `users`").WithArgs(11, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "identity_type"}).AddRow(11, "T001", "张老师", "staff"))
}

func TestAdvisorUpdateScopeAndStatus(t *testing.T) {
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, role, status, source string
		related                    int
		version                    time.Time
		want                       int
	}{
		{"unrelated teacher", "teacher", "draft", "supplement", 0, now, 403},
		{"approved locked", "teacher", "approved", "supplement", 1, now, 403},
		{"import locked", "teacher", "draft", "import", 1, now, 403},
		{"concurrent change", "teacher", "draft", "supplement", 1, now.Add(-time.Second), 409},
		{"student denied", "student", "draft", "supplement", 1, now, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := database.DB
			t.Cleanup(func() { database.DB = original })
			mock := setupAwardDBMock(t)
			c, w := advisorTestContext("PUT", tc.role, map[string]interface{}{"project_name": "项目", "award_category": "省级", "award_name": "二等奖", "award_date": "2026-10-12", "proof_url": "/api/file/download/award_proof/proof.pdf", "update_time": tc.version})
			if tc.role != "student" {
				expectAdvisorUser(mock)
				mock.ExpectBegin()
				mock.ExpectQuery("SELECT .* FROM `awards`.*FOR UPDATE").WithArgs(7, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "status", "source", "update_time"}).AddRow(7, 5, tc.status, tc.source, now))
				mock.ExpectQuery("SELECT count.*FROM `awards`.*aa.username.*cd.delete_time").WithArgs("T001", 7).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.related))
				mock.ExpectRollback()
			}
			UpdateAdvisorAward(c)
			require.Equal(t, tc.want, w.Code, w.Body.String())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAdvisorUpdateResubmitsAndLogsAtomically(t *testing.T) {
	original := database.DB
	t.Cleanup(func() { database.DB = original })
	mock := setupAwardDBMock(t)
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	c, w := advisorTestContext("PUT", "competition_manager", map[string]interface{}{"project_name": "修正项目", "award_category": "省级", "award_name": "二等奖", "award_date": "2026-10-12", "proof_url": "/api/file/download/award_proof/proof.pdf", "remark": "更正", "update_time": now, "members": []string{"cannot change"}, "status": "approved"})
	expectAdvisorUser(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `awards`.*FOR UPDATE").WithArgs(7, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "reg_id", "project_name", "status", "source", "update_time", "proof_url"}).AddRow(7, 5, 2, "原项目", "rejected", "supplement", now, "/api/file/download/award_proof/proof.pdf"))
	mock.ExpectQuery("SELECT count.*FROM `awards`.*aa.username.*cd.delete_time").WithArgs("T001", 7).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT count.*FROM `awards`.*project_key").WithArgs(5, 7, database.AwardProjectKey("修正项目"), "修正项目").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Neither members nor advisors can be changed through this endpoint.
	mock.ExpectExec("UPDATE `awards` SET `audit_time`=.*,`auditor_id`=.*,`award_category`=.*,`award_level`=.*,`award_name`=.*,`award_time`=.*,`project_key`=.*,`project_name`=.*,`proof_url`=.*,`reject_reason`=.*,`remark`=.*,`status`=.*,`update_time`=.*").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `award_changes`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE `registers` SET `status`=.*").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	UpdateAdvisorAward(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStudentAdvisorResolutionRejectsMissingAndDuplicateIDs(t *testing.T) {
	original := database.DB
	t.Cleanup(func() { database.DB = original })
	mock := setupAwardDBMock(t)
	_, err := resolveStudentAwardAdvisors(database.DB, []models.AdvisorInfo{{Name: "张老师"}})
	require.ErrorContains(t, err, "工号")
	mock.ExpectQuery("SELECT .* FROM `users`.*identity_type = 'staff'").WithArgs("T001", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college", "identity_type"}).AddRow(11, "T001", "张老师", "学院甲", "staff"))
	_, err = resolveStudentAwardAdvisors(database.DB, []models.AdvisorInfo{{Username: "T001", Name: "伪造姓名"}, {Username: "T001"}})
	require.ErrorContains(t, err, "重复")
	mock.ExpectQuery("SELECT .* FROM `users`.*identity_type = 'staff'").WithArgs("T001", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "realname", "college", "identity_type"}).AddRow(11, "T001", "张老师", "学院甲", "staff"))
	pairs, err := resolveStudentAwardAdvisors(database.DB, []models.AdvisorInfo{{Username: "T001", Name: "伪造姓名", College: "伪造学院"}})
	require.NoError(t, err)
	require.Equal(t, "张老师", pairs[0].Name)
	require.Equal(t, "学院甲", pairs[0].College)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdvisorDetailCannotReadOtherProject(t *testing.T) {
	original := database.DB
	t.Cleanup(func() { database.DB = original })
	mock := setupAwardDBMock(t)
	expectAdvisorUser(mock)
	mock.ExpectQuery("SELECT .* FROM `awards`.*aa.username.*cd.delete_time").WithArgs("T001", 7, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	c, w := advisorTestContext("GET", "teacher", nil)
	GetAdvisorAwardDetail(c)
	require.Equal(t, 404, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdvisorCannotAttachUnrelatedProof(t *testing.T) {
	original := database.DB
	t.Cleanup(func() { database.DB = original })
	mock := setupAwardDBMock(t)
	hash := "01234567890123456789012345678901"
	url := "/api/file/download/award_proof/" + hash + "_certificate.pdf"
	mock.ExpectQuery("SELECT .* FROM `file_records`").WithArgs(hash, "award_proof", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "file_hash", "biz_type", "original_name", "storage_path", "uploader_id"}).AddRow(1, hash, "award_proof", "certificate.pdf", "/static/award_proofs/certificate.pdf", 22))
	mock.ExpectQuery("SELECT count.*FROM `awards`.*aa.username.*proof_url").WithArgs("T001", url).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	require.ErrorContains(t, validateAdvisorProof(database.DB, models.User{BaseModel: models.BaseModel{ID: 11}, Username: "T001"}, url), "权限")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLegacyAwardCertificateAccessUsesExactAwardRelation(t *testing.T) {
	for _, related := range []int{0, 1} {
		t.Run(string(rune('0'+related)), func(t *testing.T) {
			original := database.DB
			t.Cleanup(func() { database.DB = original })
			mock := setupAwardDBMock(t)
			record := models.FileRecord{BizType: "award_cert", FileHash: "01234567890123456789012345678901", OriginalName: "certificate.pdf", StoragePath: "/static/temp/certificate.pdf"}
			mock.ExpectQuery("SELECT count.*FROM `awards`.*proof_url IN.*aa.award_id").WithArgs(buildDownloadURL(record.BizType, record.FileHash, record.OriginalName), record.StoragePath, 11, 11, 11, true, 11).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(related))
			require.Equal(t, related == 1, canAccessAwardProof(t.Context(), 11, &UserAccessScope{UserID: 11, RoleCode: "teacher"}, record))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStudentCannotAttachUnrelatedProof(t *testing.T) {
	original := database.DB
	t.Cleanup(func() { database.DB = original })
	mock := setupAwardDBMock(t)
	hash := "01234567890123456789012345678901"
	url := "/api/file/download/award_cert/" + hash + "_certificate.pdf"
	mock.ExpectQuery("SELECT .* FROM `file_records`").WithArgs(hash, "award_cert", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "file_hash", "biz_type", "original_name", "storage_path", "uploader_id"}).AddRow(1, hash, "award_cert", "certificate.pdf", "/static/temp/certificate.pdf", 22))
	mock.ExpectQuery("SELECT count.*FROM `awards`.*proof_url.*am.student_id").WithArgs(url, 11, 11).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	require.ErrorContains(t, validateStudentAwardProof(database.DB, 11, url), "权限")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchAuditRejectsStaleOrIncompleteVersionsBeforeWriting(t *testing.T) {
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	for _, versions := range []map[uint]time.Time{{7: now.Add(-time.Second)}, {}} {
		original := database.DB
		mock := setupAwardDBMock(t)
		mock.ExpectQuery("SELECT .* FROM `awards`.*ORDER BY id ASC.*FOR UPDATE").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"id", "status", "update_time"}).AddRow(7, "draft", now))
		require.ErrorIs(t, checkAwardAuditVersions(database.DB, []uint{7}, versions), errAwardAuditConflict)
		require.NoError(t, mock.ExpectationsWereMet())
		database.DB = original
	}
}

func TestSingleAuditDoesNotOverwriteConcurrentAdvisorEdit(t *testing.T) {
	for _, handler := range []gin.HandlerFunc{PassAwardAudit, RejectAwardAudit} {
		original := database.DB
		mock := setupAwardDBMock(t)
		now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
		c, w := advisorTestContext("PUT", "school_admin", map[string]interface{}{"reason": "更正", "update_time": now})
		// Scope-free unit harness: exercise only the atomic update after the access check.
		delete(c.Keys, "role_code")
		mock.ExpectQuery("SELECT .* FROM `awards`").WithArgs(7, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "status", "update_time"}).AddRow(7, "draft", now))
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE `awards`.*status = .* AND update_time =").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()
		handler(c)
		require.Equal(t, 409, w.Code, w.Body.String())
		require.NoError(t, mock.ExpectationsWereMet())
		database.DB = original
	}
}
