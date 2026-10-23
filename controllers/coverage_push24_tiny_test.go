package controllers

import (
	"net/http"

	"CompeManage_backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ===================== 单行分支补齐 =====================

// currentOperatorID：user_id 非 uint 时返回 0
func TestCurrentOperatorID_NonUint(t *testing.T) {
	_, _, c := buildGET("/x")
	c.Set("user_id", "abc")
	assert.Equal(t, uint(0), currentOperatorID(c))
	assert.Equal(t, uint(0), currentOperatorID(nil))
}

// roleAssignmentBusinessError.Error() 覆盖
func TestRoleAssignmentBusinessError_Error(t *testing.T) {
	e := &roleAssignmentBusinessError{status: 400, message: "自定义消息"}
	assert.Equal(t, "自定义消息", e.Error())
}

// removeUploadedFile：删除真实存在的文件（成功分支）
func TestRemoveUploadedFile_Success(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	assert.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
	removeUploadedFile("/" + filepath.ToSlash(p))
	assert.NoFileExists(t, p)
}

// GetUserList：page_size 超上限裁剪到 50
func TestGetUserList_PageSizeClamp2(t *testing.T) {
	ensureRedisForUserList(t).FlushDB(t.Context())
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=80&role=student")
	GetUserList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// GetMyRegList：page_size 超上限裁剪
func TestGetMyRegList_PageSizeClamp2(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT `username` FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT \\* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, w, c := buildGET("/api/reg/my-reg?page=1&size=999")
	c.Set("user_id", uint(1))
	GetMyRegList(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// 申报三接口：院管未配置管理学院 → 403（requestAccessScopeIfAvailable 失败分支）
func TestDeclareScopeInvalidBranches(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 0))

	_, w1, c1 := buildAuthPOSTJSON(nil)
	c1.Params = []gin.Param{{Key: "id", Value: "1"}}
	c1.Set("user_id", uint(1))
	c1.Set("role_code", "college_admin")
	SubmitDeclare(c1)
	assert.Equal(t, http.StatusForbidden, w1.Code)

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 1))
	_, w2, c2 := buildAuthPOSTJSON(nil)
	c2.Params = []gin.Param{{Key: "id", Value: "1"}}
	c2.Set("user_id", uint(1))
	c2.Set("role_code", "college_admin")
	RevokeDeclare(c2)
	assert.Equal(t, http.StatusForbidden, w2.Code)

	mock.ExpectQuery("SELECT .* FROM `comp_declarations`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "declare_status"}).AddRow(1, 0))
	_, w3, c3 := buildAuthPOSTJSON(nil)
	c3.Params = []gin.Param{{Key: "id", Value: "1"}}
	c3.Set("user_id", uint(1))
	c3.Set("role_code", "college_admin")
	DeleteDeclare(c3)
	assert.Equal(t, http.StatusForbidden, w3.Code)
}

// competitionCapabilities：requireUserAccessScope 失败分支
func TestCompetitionCapabilities_ScopeInvalid(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, _, c := buildGET("/x")
	c.Set("user_id", uint(1))
	c.Set("role_code", "college_admin") // 未配置管理学院
	caps, ok := competitionCapabilities(c, models.CompDirectory{})
	assert.False(t, ok)
	assert.Nil(t, caps)
}
