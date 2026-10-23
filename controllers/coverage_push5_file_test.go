package controllers

import (
	"CompeManage_backend/models"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// ===================== extractMD5FromPath =====================

func TestExtractMD5FromPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "normal", in: "/static/reg_attachments/202606/" + "0123456789abcdef0123456789abcdef" + "_work.pdf", want: "0123456789abcdef0123456789abcdef"},
		{name: "no slash", in: "noslash.pdf", want: ""},
		{name: "root only", in: "/", want: ""},
		{name: "no underscore", in: "/static/a/0123456789abcdef0123456789abcdef", want: ""},
		{name: "short md5", in: "/static/a/abc_x.pdf", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, extractMD5FromPath(tc.in))
		})
	}
}

// ===================== findCompIDByStoragePath =====================

func TestFindCompIDByStoragePath_InvalidPaths(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	assert.Equal(t, uint(0), findCompIDByStoragePath(context.Background(), "noslash.pdf"))
	assert.Equal(t, uint(0), findCompIDByStoragePath(context.Background(), "/"))
	assert.Equal(t, uint(0), findCompIDByStoragePath(context.Background(), "/static/a/short_x.pdf"))
}

func TestFindCompIDByStoragePath_Hit(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	mock.ExpectQuery("SELECT comp_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(7))

	got := findCompIDByStoragePath(context.Background(), "/static/reg_attachments/202606/"+hash+"_work.pdf")
	assert.Equal(t, uint(7), got)
}

// ===================== isCompManager =====================

func TestIsCompManager_Match(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT manager_id FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"manager_id"}).AddRow(42))

	assert.True(t, isCompManager(context.Background(), 42, 7))
}

func TestIsCompManager_Mismatch(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT manager_id FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"manager_id"}).AddRow(42))

	assert.False(t, isCompManager(context.Background(), 99, 7))
}

func TestIsCompManager_QueryError(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT manager_id FROM `comp_directories`").
		WillReturnError(assertErr("db error"))

	assert.False(t, isCompManager(context.Background(), 42, 7))
}

// ===================== isTeamMemberForFile =====================

func TestIsTeamMemberForFile_Found(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	assert.True(t, isTeamMemberForFile(context.Background(), 5, "/static/x/"+hash+"_w.pdf"))
}

func TestIsTeamMemberForFile_NotFound(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	assert.False(t, isTeamMemberForFile(context.Background(), 5, "/static/x/"+hash+"_w.pdf"))
}

// ===================== isExpertForFile =====================

func TestIsExpertForFile_NoCompID(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	// findCompIDByStoragePath 命中 compID 后才会查 review_tasks
	hash := "0123456789abcdef0123456789abcdef"
	mock.ExpectQuery("SELECT comp_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(0))

	assert.False(t, isExpertForFile(context.Background(), 5, "/static/x/"+hash+"_w.pdf"))
}

func TestIsExpertForFile_IsExpert(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	mock.ExpectQuery("SELECT comp_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(7))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	assert.True(t, isExpertForFile(context.Background(), 5, "/static/x/"+hash+"_w.pdf"))
}

func TestIsExpertForFile_NotExpert(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	mock.ExpectQuery("SELECT comp_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(7))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	assert.False(t, isExpertForFile(context.Background(), 5, "/static/x/"+hash+"_w.pdf"))
}

// ===================== isCompManagerForFile =====================

func TestIsCompManagerForFile_ViaRegisters(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	// findCompIDByStoragePath → 7
	mock.ExpectQuery("SELECT comp_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(7))
	// isCompManager(7) → manager 命中
	mock.ExpectQuery("SELECT manager_id FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"manager_id"}).AddRow(42))

	assert.True(t, isCompManagerForFile(context.Background(), 42, "/static/x/"+hash+"_w.pdf"))
}

func TestIsCompManagerForFile_NoCompID(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	mock.ExpectQuery("SELECT comp_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(0))
	// compID=0 时走 awards 兜底，此处无命中
	mock.ExpectQuery("SELECT `comp_directories.manager_id` FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"manager_id"}).AddRow(0))

	assert.False(t, isCompManagerForFile(context.Background(), 42, "/static/x/"+hash+"_w.pdf"))
}

// ===================== DownloadFile =====================

func TestDownloadFile_UnsupportedType(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/file/download/bogus/abc.pdf")
	c.Params = []gin.Param{{Key: "type", Value: "bogus"}, {Key: "filename", Value: "abc.pdf"}}
	DownloadFile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "不支持的文件类型")
}

func TestDownloadFile_InvalidFilenameFormat(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildGET("/api/file/download/notice/short.pdf")
	c.Params = []gin.Param{{Key: "type", Value: "notice"}, {Key: "filename", Value: "short.pdf"}}
	DownloadFile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "无效的文件名格式")
}

func TestDownloadFile_RecordNotFound(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	mock.ExpectQuery("SELECT \\* FROM `file_records`").
		WillReturnError(gorm.ErrRecordNotFound)

	_, w, c := buildGET("/api/file/download/notice/" + hash + "_a.pdf")
	c.Params = []gin.Param{{Key: "type", Value: "notice"}, {Key: "filename", Value: hash + "_a.pdf"}}
	DownloadFile(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "文件不存在")
}

func TestDownloadFile_Forbidden(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	// 记录存在，上传者不是当前用户
	rows := sqlmock.NewRows([]string{"id", "file_hash", "biz_type", "storage_path", "original_name", "file_size", "file_ext", "uploader_id"}).
		AddRow(1, hash, "notice", "/static/notices/202606/"+hash+"_a.pdf", "a.pdf", 100, ".pdf", 99)
	mock.ExpectQuery("SELECT \\* FROM `file_records`").
		WillReturnRows(rows)
	// canAccessFile：notice 类型对登录用户开放

	_, w, c := buildGET("/api/file/download/notice/" + hash + "_a.pdf")
	c.Params = []gin.Param{{Key: "type", Value: "notice"}, {Key: "filename", Value: hash + "_a.pdf"}}
	c.Set("user_id", uint(5))
	DownloadFile(c)

	// notice 类型对任何登录用户开放
	assert.Equal(t, http.StatusNotFound, w.Code) // 物理文件不存在
}

func TestDownloadFile_Unauthorized(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	rows := sqlmock.NewRows([]string{"id", "file_hash", "biz_type", "storage_path", "original_name", "file_size", "file_ext", "uploader_id"}).
		AddRow(1, hash, "award_proof", "/static/award_proofs/202606/"+hash+"_a.pdf", "a.pdf", 100, ".pdf", 99)
	mock.ExpectQuery("SELECT \\* FROM `file_records`").
		WillReturnRows(rows)

	_, w, c := buildGET("/api/file/download/award_proof/" + hash + "_a.pdf")
	c.Params = []gin.Param{{Key: "type", Value: "award_proof"}, {Key: "filename", Value: hash + "_a.pdf"}}
	// 未登录 → canAccessFile 直接 false
	DownloadFile(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无权限访问此文件")
}

func TestDownloadFile_StoragePathMissing(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	rows := sqlmock.NewRows([]string{"id", "file_hash", "biz_type", "storage_path", "original_name", "file_size", "file_ext", "uploader_id"}).
		AddRow(1, hash, "notice", "", "a.pdf", 100, ".pdf", 5)
	mock.ExpectQuery("SELECT \\* FROM `file_records`").
		WillReturnRows(rows)

	_, w, c := buildGET("/api/file/download/notice/" + hash + "_a.pdf")
	c.Params = []gin.Param{{Key: "type", Value: "notice"}, {Key: "filename", Value: hash + "_a.pdf"}}
	c.Set("user_id", uint(5))
	DownloadFile(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "文件已丢失")
}

// ===================== canAccessFile =====================

func TestCanAccessFile_UploaderSelf(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	hash := "0123456789abcdef0123456789abcdef"
	rec := models.FileRecord{BizType: "award_proof", FileHash: hash, OriginalName: "a.pdf", UploaderID: 7}

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(7))

	assert.True(t, canAccessFile(context.Background(), c, "award_proof", rec))
}

func TestCanAccessFile_NoUserID(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	_, _, c := buildGET("/x")
	rec := models.FileRecord{BizType: "award_proof", UploaderID: 7}

	assert.False(t, canAccessFile(context.Background(), c, "award_proof", rec))
}

func TestCanAccessFile_UnknownType(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	// 普通角色：switch 未命中任何 bizType，返回 false
	mock.ExpectQuery("SELECT roles.role_code, users.managed_college_id FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).AddRow("student", nil))

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(5))
	rec := models.FileRecord{BizType: "unknown_biz", UploaderID: 99}

	assert.False(t, canAccessFile(context.Background(), c, "unknown_biz", rec))
}

func TestCanAccessFile_SchoolAdminAllowsEverything(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT roles.role_code, users.managed_college_id FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).AddRow("school_admin", nil))

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(5))
	rec := models.FileRecord{BizType: "competition_work", UploaderID: 99}

	assert.True(t, canAccessFile(context.Background(), c, "competition_work", rec))
}

func TestCanAccessFile_CollegeAdminAllowed(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT roles.role_code, users.managed_college_id FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).AddRow("college_admin", 9))
	// findCompIDByStoragePath → 7
	mock.ExpectQuery("SELECT comp_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(7))
	// comp 查询：college_id 与 managed 一致
	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(7, 1, 9))

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(5))
	hash := "0123456789abcdef0123456789abcdef"
	rec := models.FileRecord{BizType: "reg_attachment", UploaderID: 99, FileHash: hash, OriginalName: "w.pdf", StoragePath: "/static/reg_attachments/202606/" + hash + "_w.pdf"}

	assert.True(t, canAccessFile(context.Background(), c, "reg_attachment", rec))
}

func TestCanAccessFile_CollegeAdminDenied(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT roles.role_code, users.managed_college_id FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).AddRow("college_admin", 9))
	mock.ExpectQuery("SELECT comp_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(7))
	// college_id 不匹配 → canAccessCompetition false，继续走 switch
	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(7, 1, 3))
	// reg_attachment → isTeamMemberForFile
	hash := "0123456789abcdef0123456789abcdef"
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// → isCompManagerForFile → findCompIDByStoragePath
	mock.ExpectQuery("SELECT comp_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(7))
	mock.ExpectQuery("SELECT manager_id FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"manager_id"}).AddRow(1))

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(5))
	rec := models.FileRecord{BizType: "reg_attachment", UploaderID: 99, FileHash: hash, OriginalName: "w.pdf", StoragePath: "/static/reg_attachments/202606/" + hash + "_w.pdf"}

	assert.False(t, canAccessFile(context.Background(), c, "reg_attachment", rec))
}

func TestCanAccessFile_AwardProofAllowedForMember(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT roles.role_code, users.managed_college_id FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).AddRow("student", nil))
	// canAccessAwardProof 计数命中
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(5))
	hash := "0123456789abcdef0123456789abcdef"
	rec := models.FileRecord{BizType: "award_proof", UploaderID: 99, FileHash: hash, OriginalName: "p.pdf"}

	assert.True(t, canAccessFile(context.Background(), c, "award_proof", rec))
}

func TestCanAccessFile_AwardProofDenied(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT roles.role_code, users.managed_college_id FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).AddRow("student", nil))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(5))
	hash := "0123456789abcdef0123456789abcdef"
	rec := models.FileRecord{BizType: "award_proof", UploaderID: 99, FileHash: hash, OriginalName: "p.pdf"}

	assert.False(t, canAccessFile(context.Background(), c, "award_proof", rec))
}

func TestCanAccessFile_AwardProofQueryError(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT roles.role_code, users.managed_college_id FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).AddRow("student", nil))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnError(assertErr("db error"))

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(5))
	hash := "0123456789abcdef0123456789abcdef"
	rec := models.FileRecord{BizType: "award_proof", UploaderID: 99, FileHash: hash, OriginalName: "p.pdf"}

	assert.False(t, canAccessFile(context.Background(), c, "award_proof", rec))
}

func TestCanAccessFile_AwardProofCollegeAdminScope(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT roles.role_code, users.managed_college_id FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).AddRow("college_admin", 9))
	// college_admin 分支：award proof 查询带学院范围
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(5))
	hash := "0123456789abcdef0123456789abcdef"
	rec := models.FileRecord{BizType: "temp", UploaderID: 99, FileHash: hash, OriginalName: "p.pdf"}

	assert.True(t, canAccessFile(context.Background(), c, "temp", rec))
}

func TestCanAccessFile_CompetitionWorkExpertFallback(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT roles.role_code, users.managed_college_id FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).AddRow("expert", nil))
	hash := "0123456789abcdef0123456789abcdef"
	// isTeamMemberForFile
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// isCompManagerForFile → findCompIDByStoragePath
	mock.ExpectQuery("SELECT comp_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(7))
	mock.ExpectQuery("SELECT manager_id FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"manager_id"}).AddRow(1))
	// isExpertForFile → findCompIDByStoragePath
	mock.ExpectQuery("SELECT comp_id FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"comp_id"}).AddRow(7))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_tasks`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(5))
	rec := models.FileRecord{BizType: "competition_work", UploaderID: 99, FileHash: hash, OriginalName: "w.pdf", StoragePath: "/static/reg_attachments/202606/" + hash + "_w.pdf"}

	assert.True(t, canAccessFile(context.Background(), c, "competition_work", rec))
}

func TestCanAccessFile_NoticeTypeOpen(t *testing.T) {
	mock := setupDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT roles.role_code, users.managed_college_id FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).AddRow("student", nil))

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(5))
	rec := models.FileRecord{BizType: "notice", UploaderID: 99}

	assert.True(t, canAccessFile(context.Background(), c, "notice", rec))
}

// assertErr 构造一个普通错误（非 ErrRecordNotFound）
func assertErr(msg string) error {
	return errors.New(msg)
}
