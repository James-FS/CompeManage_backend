package controllers

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"CompeManage_backend/database"
	"CompeManage_backend/models"
)

// 回归 2026-10-29 需求：指导老师候选=全校各部门教职工（identity_type='staff'），
// 按工号/姓名均可检索；仅 teacher 角色或无角色的先任教师不得被排除
// （旧实现 NOT EXISTS 防御过滤会整体排除，出现"工号 102971 查不到本人"）。
func TestGetUserList_TeacherScopeAllStaff(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	onlyTeacher := models.User{Username: "102971", Realname: "先任教师", College: "计算机学院", IdentityType: "staff"}
	noRole := models.User{Username: "102972", Realname: "无角色教师", College: "数学学院", IdentityType: "staff"}
	adminStaff := models.User{Username: "T2023001", Realname: "管理员兼课", College: "计算机学院", IdentityType: "staff"}
	student := models.User{Username: "S2024001", Realname: "学生甲", College: "计算机学院", IdentityType: "external"}
	for _, u := range []*models.User{&onlyTeacher, &noRole, &adminStaff, &student} {
		require.NoError(t, db.Create(u).Error)
	}
	role := models.Role{RoleCode: "teacher", RoleName: "教师"}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&models.UserRole{UserID: onlyTeacher.ID, RoleID: role.ID}).Error)

	// 按工号检索（命中曾经"查不到"的本人）
	_, w, c := buildGET("/api/reg/user/list?page=1&page_size=10&role=teacher&search=102971")
	GetUserList(c)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "先任教师")
	assert.NotContains(t, w.Body.String(), "学生甲")

	// 按姓名检索
	_, w2, c2 := buildGET("/api/reg/user/list?page=1&page_size=10&role=teacher&search=" + url.QueryEscape("无角色教师"))
	GetUserList(c2)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "无角色教师")

	// 空搜索：全校教职工均在（含管理员兼课），学生不在
	_, w3, c3 := buildGET("/api/reg/user/list?page=1&page_size=10&role=teacher")
	GetUserList(c3)
	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Contains(t, w3.Body.String(), "管理员兼课")
	assert.Contains(t, w3.Body.String(), "无角色教师")
	assert.NotContains(t, w3.Body.String(), "学生甲")
}

// 回归：findAwardTeacher 全校教职工口径（Excel 解析/校验同源）
func TestFindAwardTeacher_AllStaff(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	onlyTeacher := models.User{Username: "102971", Realname: "先任教师", College: "计算机学院", IdentityType: "staff"}
	require.NoError(t, db.Create(&onlyTeacher).Error)
	student := models.User{Username: "S2024001", Realname: "学生甲", College: "计算机学院", IdentityType: "external"}
	require.NoError(t, db.Create(&student).Error)

	u, err := findAwardTeacher(db, "102971")
	require.NoError(t, err)
	assert.Equal(t, "先任教师", u.Realname)

	_, err = findAwardTeacher(db, "S2024001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不存在或不是教职工")
}
