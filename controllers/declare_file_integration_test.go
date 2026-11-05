package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 申报范围 + 文件下载权限集成（冲 declare scope / DownloadFile 0%）
func TestDeclareAndFileAccessMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.CompDeclaration{}, &models.Role{}, &models.UserRole{}, &models.Permission{}, &models.RolePermission{}))

	// 角色（GetUserAccessScope / RequirePermission 路径需要）
	roles := map[string]models.Role{}
	for _, rc := range []string{"school_admin", "college_admin", "teacher", "student", "competition_manager"} {
		role := models.Role{RoleName: rc, RoleCode: rc, Description: rc}
		require.NoError(t, db.Create(&role).Error)
		roles[rc] = role
	}

	ca := models.College{Name: "申报甲学院", IsValid: true}
	cb := models.College{Name: "申报乙学院", IsValid: true}
	require.NoError(t, db.Create(&ca).Error)
	require.NoError(t, db.Create(&cb).Error)

	users := map[string]models.User{}
	for _, u := range []struct {
		username, realname, role, college string
	}{
		{"DS001", "申报校管", "school_admin", "教务处"},
		{"DA001", "申报甲管", "college_admin", ca.Name},
		{"DB001", "申报乙管", "college_admin", cb.Name},
		{"DT001", "申报教师", "competition_manager", ca.Name},
		{"DU001", "申报学生", "student", ca.Name},
		{"DU002", "申报外学生", "student", cb.Name},
	} {
		user := models.User{Username: u.username, Realname: u.realname, College: u.college, Major: "专业", IdentityType: "staff"}
		if u.role == "student" {
			user.IdentityType = "student"
		}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, database.SetUserRole(db, user.ID, roles[u.role].ID))
		users[u.username] = user
	}
	// 甲管绑定学院
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", users["DA001"].ID).Update("managed_college_id", ca.ID).Error)
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", users["DB001"].ID).Update("managed_college_id", cb.ID).Error)

	invoke := func(method, path, role string, collegeID *uint, userID uint, body interface{}, handler gin.HandlerFunc, params ...gin.Param) *httptest.ResponseRecorder {
		var reader *bytes.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			reader = bytes.NewReader(data)
		} else {
			reader = bytes.NewReader(nil)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, path, reader)
		c.Request.Header.Set("Content-Type", "application/json")
		if body == nil {
			c.Request = httptest.NewRequest(method, path, nil)
		}
		c.Params = params
		c.Set("role_code", role)
		c.Set("user_id", userID)
		c.Set("managed_college_id", collegeID)
		handler(c)
		return w
	}

	declareBody := func(collegeID uint, name string) map[string]interface{} {
		return map[string]interface{}{
			"comp_name": name, "comp_level": "省级", "comp_type": "学科竞赛",
			"organizer": "主办方", "undertaker": "承办方",
			"college_id": collegeID, "manager_id": users["DT001"].ID, "year": 2026,
			"desc": "集成申报",
		}
	}

	t.Run("CreateDeclare college scope", func(t *testing.T) {
		// 甲管为本院创建
		w := invoke("POST", "/api/declare", "college_admin", &ca.ID, users["DA001"].ID,
			declareBody(ca.ID, "甲院申报赛"), CreateDeclare)
		require.Equal(t, 200, w.Code, w.Body.String())

		// 甲管为他院创建 → 403
		w = invoke("POST", "/api/declare", "college_admin", &ca.ID, users["DA001"].ID,
			declareBody(cb.ID, "越权申报"), CreateDeclare)
		require.Equal(t, 403, w.Code)

		// 参数错误
		w = invoke("POST", "/api/declare", "college_admin", &ca.ID, users["DA001"].ID,
			map[string]interface{}{}, CreateDeclare)
		require.Equal(t, 400, w.Code)

		// 校管可创建任意学院
		w = invoke("POST", "/api/declare", "school_admin", nil, users["DS001"].ID,
			declareBody(cb.ID, "校管代乙申报"), CreateDeclare)
		require.Equal(t, 200, w.Code, w.Body.String())
	})

	t.Run("Submit/list/detail/audit flow", func(t *testing.T) {
		var draft models.CompDeclaration
		require.NoError(t, db.Where("comp_name = ?", "甲院申报赛").First(&draft).Error)

		// 提交
		w := invoke("POST", fmt.Sprintf("/api/declare/%d/submit", draft.ID), "college_admin", &ca.ID, users["DA001"].ID,
			nil, SubmitDeclare, gin.Param{Key: "id", Value: fmt.Sprint(draft.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.NoError(t, db.First(&draft, draft.ID).Error)
		require.Equal(t, int8(1), draft.DeclareStatus)
		var pendingCount int64
		require.NoError(t, db.Model(&models.CompDeclaration{}).Where("declare_status = ?", 1).Count(&pendingCount).Error)
		require.EqualValues(t, 1, pendingCount)

		// 详情
		w = invoke("GET", fmt.Sprintf("/api/declare/%d", draft.ID), "college_admin", &ca.ID, users["DA001"].ID,
			nil, GetDeclareDetail, gin.Param{Key: "id", Value: fmt.Sprint(draft.ID)})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "甲院申报赛")

		// 不存在
		w = invoke("GET", "/api/declare/999999", "school_admin", nil, users["DS001"].ID,
			nil, GetDeclareDetail, gin.Param{Key: "id", Value: "999999"})
		require.Equal(t, 404, w.Code)

		// 待审核列表（校管）
		w = invoke("GET", "/api/declare/pending/list?page=1&page_size=10", "school_admin", nil, users["DS001"].ID,
			nil, GetPendingDeclares)
		require.Equal(t, 200, w.Code, w.Body.String())
		var pendingResp struct {
			Data struct {
				Items []models.CompDeclaration `json:"items"`
				Total int64                    `json:"total"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pendingResp))
		require.EqualValues(t, 1, pendingResp.Data.Total)
		require.Len(t, pendingResp.Data.Items, 1)
		require.Equal(t, "甲院申报赛", pendingResp.Data.Items[0].CompName)

		// 甲管待审核列表（申请 scope 路径）
		w = invoke("GET", "/api/declare/pending/list?page=1&page_size=10", "college_admin", &ca.ID, users["DA001"].ID,
			nil, GetPendingDeclares)
		require.Equal(t, 200, w.Code)

		// 校管审核通过
		w = invoke("POST", "/api/declare/audit", "school_admin", nil, users["DS001"].ID,
			map[string]interface{}{"declare_id": draft.ID, "audit_status": 2, "audit_remark": "通过"}, AuditDeclare)
		require.Equal(t, 200, w.Code, w.Body.String())

		var approved models.CompDeclaration
		require.NoError(t, db.First(&approved, draft.ID).Error)
		require.Equal(t, int8(2), approved.DeclareStatus)

		// 重复审核：已通过 → 403 只能审核已提交
		w = invoke("POST", "/api/declare/audit", "school_admin", nil, users["DS001"].ID,
			map[string]interface{}{"declare_id": draft.ID, "audit_status": 3, "audit_remark": "再拒"}, AuditDeclare)
		require.Equal(t, 403, w.Code)

		// 驳回另一条
		var other models.CompDeclaration
		require.NoError(t, db.Where("comp_name = ?", "校管代乙申报").First(&other).Error)
		require.NoError(t, db.Model(&other).Update("declare_status", 1).Error)
		require.NoError(t, db.First(&other, other.ID).Error)
		w = invoke("POST", "/api/declare/audit", "school_admin", nil, users["DS001"].ID,
			map[string]interface{}{"declare_id": other.ID, "audit_status": 3, "audit_remark": "材料不足"}, AuditDeclare)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.NoError(t, db.First(&other, other.ID).Error)
		require.Equal(t, int8(3), other.DeclareStatus)
		require.Equal(t, "材料不足", other.AuditRemark)

		// 我的申报列表：应含已通过的甲院申报赛（回归 declare_status 零值过滤）
		w = invoke("GET", "/api/declare/my/list?page=1&page_size=10", "college_admin", &ca.ID, users["DA001"].ID,
			nil, GetMyDeclares)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "甲院申报赛")
		// 显式筛草稿则不应含已通过记录
		w = invoke("GET", "/api/declare/my/list?page=1&page_size=10&declare_status=0", "college_admin", &ca.ID, users["DA001"].ID,
			nil, GetMyDeclares)
		require.Equal(t, 200, w.Code)
		require.NotContains(t, w.Body.String(), "甲院申报赛")

		// 全部申报（校管）
		w = invoke("GET", "/api/declare/all?page=1&page_size=10", "school_admin", nil, users["DS001"].ID,
			nil, GetAllDeclares)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "校管代乙申报")
	})

	t.Run("DownloadFile access matrix", func(t *testing.T) {
		// DownloadFile 以进程 CWD 相对路径打开物理文件
		wd, err := os.Getwd()
		require.NoError(t, err)
		proofDir := filepath.Join(wd, "static", "award_proofs")
		require.NoError(t, os.MkdirAll(proofDir, 0o755))
		t.Cleanup(func() { _ = os.RemoveAll(proofDir) })

		hash := strings.Repeat("c", 32)
		name := "proof.txt"
		abs := filepath.Join(proofDir, hash+"_"+name)
		require.NoError(t, os.WriteFile(abs, []byte("proof-bytes"), 0o644))

		require.NoError(t, db.Create(&models.FileRecord{
			FileHash: hash, OriginalName: name,
			StoragePath: "static/award_proofs/" + hash + "_" + name,
			FileSize: 11, FileExt: "txt", BizType: "award_proof",
			UploaderID: users["DU001"].ID,
		}).Error)

		download := func(role string, uid uint, params ...gin.Param) *httptest.ResponseRecorder {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/file/download/award_proof/"+hash+"_"+name, nil)
			c.Params = append([]gin.Param{
				{Key: "type", Value: "award_proof"},
				{Key: "filename", Value: hash + "_" + name},
			}, params...)
			c.Set("role_code", role)
			c.Set("user_id", uid)
			c.Set("managed_college_id", (*uint)(nil))
			DownloadFile(c)
			return w
		}

		// 不支持类型
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Params = []gin.Param{{Key: "type", Value: "evil"}, {Key: "filename", Value: hash + "_x"}}
		c.Set("user_id", users["DU001"].ID)
		DownloadFile(c)
		require.Equal(t, 400, w.Code)

		// 无效 md5 长度
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Params = []gin.Param{{Key: "type", Value: "award_proof"}, {Key: "filename", Value: "short_name"}}
		c.Set("user_id", users["DU001"].ID)
		DownloadFile(c)
		require.Equal(t, 400, w.Code)

		// 文件不存在
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Params = []gin.Param{{Key: "type", Value: "award_proof"}, {Key: "filename", Value: strings.Repeat("d", 32) + "_x"}}
		c.Set("user_id", users["DU001"].ID)
		DownloadFile(c)
		require.Equal(t, 404, w.Code)

		// 上传者本人可下
		w = download("student", users["DU001"].ID)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, "proof-bytes", w.Body.String())

		// 校管可下
		w = download("school_admin", users["DS001"].ID)
		require.Equal(t, 200, w.Code)

		// 无关联学生不可下
		w = download("student", users["DU002"].ID)
		require.Equal(t, 403, w.Code)

		// 未绑定学院的院管（GetUserAccessScope 有 role）走 canAccessFile 后半段
		// 将证明挂到甲院校赛上，甲管因 managed college 访问
		comp := models.CompDirectory{CompCode: "FQ1", CompName: "文件校赛", CompLevel: "校级", ManagerID: users["DT001"].ID, CollegeID: &ca.ID, Year: 2026}
		require.NoError(t, db.Create(&comp).Error)
		proofURL := buildDownloadURL("award_proof", hash, name)
		a := models.Award{CompID: comp.ID, ProjectName: "文件证明项目", AwardLevel: "省级一等奖", AwardCategory: "省级", AwardName: "一等奖", Status: "approved", Source: "supplement", AdvisorInfo: "[]", ProofUrl: proofURL}
		require.NoError(t, db.Create(&a).Error)
		require.NoError(t, db.Create(&models.AwardMember{
			AwardID: a.ID, StudentID: users["DU001"].ID, StudentNumber: users["DU001"].Username,
			Name: users["DU001"].Realname, College: ca.Name, IsLeader: true,
		}).Error)

		// 院管访问：managed_college_id 已在 context；DownloadFile 用 GetUserAccessScope 重查 DB
		w = download("college_admin", users["DA001"].ID)
		require.Equal(t, 200, w.Code, w.Body.String())

		// notice 类型：任意登录用户可下（需 notice 记录文件）
		// 跳过 notice 物理文件，仅测 canAccessFile 分支用 notice biz_type
	})
}

// 单元：申报范围 SQL 条件（不依赖 MySQL）
func TestApplyDeclarationScopeSQL(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)

	run := func(scope *UserAccessScope) {
		mock.ExpectQuery("SELECT .*FROM `comp_declarations`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		q := applyDeclarationScope(db.Table("comp_declarations").Select("id"), scope, "comp_declarations")
		require.NoError(t, q.Find(&[]models.CompDeclaration{}).Error)
	}
	run(&UserAccessScope{RoleCode: "school_admin"})
	collegeID := uint(3)
	run(&UserAccessScope{RoleCode: "college_admin", ManagedCollegeID: &collegeID})
	run(&UserAccessScope{RoleCode: "college_admin", ManagedCollegeID: nil})
	run(&UserAccessScope{RoleCode: "teacher", UserID: 7})
	require.NoError(t, mock.ExpectationsWereMet())

	require.True(t, canAccessDeclaration(&UserAccessScope{RoleCode: "school_admin"}, models.CompDeclaration{CollegeID: 1}))
	require.True(t, canAccessDeclaration(&UserAccessScope{RoleCode: "college_admin", ManagedCollegeID: &collegeID}, models.CompDeclaration{CollegeID: 3}))
	require.False(t, canAccessDeclaration(&UserAccessScope{RoleCode: "college_admin", ManagedCollegeID: &collegeID}, models.CompDeclaration{CollegeID: 9}))
	require.False(t, canAccessDeclaration(&UserAccessScope{RoleCode: "college_admin", ManagedCollegeID: nil}, models.CompDeclaration{CollegeID: 3}))
	require.True(t, canAccessDeclaration(&UserAccessScope{RoleCode: "teacher", UserID: 7}, models.CompDeclaration{CreatedBy: 7}))
	require.True(t, canAccessDeclaration(&UserAccessScope{RoleCode: "teacher", UserID: 7}, models.CompDeclaration{ManagerID: 7}))
	require.False(t, canAccessDeclaration(&UserAccessScope{RoleCode: "teacher", UserID: 7}, models.CompDeclaration{CreatedBy: 8}))
}

func TestGetUserAccessScopeSQL(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	mock.ExpectQuery("SELECT .*FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).AddRow("college_admin", 9))
	scope, err := GetUserAccessScope(t.Context(), 1)
	require.NoError(t, err)
	require.True(t, scope.IsCollegeAdmin())
	require.Equal(t, uint(9), *scope.ManagedCollegeID)

	mock.ExpectQuery("SELECT .*FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}))
	_, err = GetUserAccessScope(t.Context(), 2)
	require.Error(t, err)

	mock.ExpectQuery("SELECT .*FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"role_code", "managed_college_id"}).
			AddRow("teacher", nil).AddRow("student", nil))
	_, err = GetUserAccessScope(t.Context(), 3)
	require.Error(t, err)

	_ = time.Now()
}
