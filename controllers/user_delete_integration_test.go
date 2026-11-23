package controllers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDeleteUserMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Role{}, &models.Permission{}, &models.UserRole{}, &models.RolePermission{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	originalRedis := middleware.RedisClient
	middleware.RedisClient = nil
	t.Cleanup(func() { middleware.RedisClient = originalRedis })

	adminRole := models.Role{RoleCode: "school_admin", RoleName: "校级管理员"}
	studentRole := models.Role{RoleCode: "student", RoleName: "学生"}
	require.NoError(t, db.Create(&adminRole).Error)
	require.NoError(t, db.Create(&studentRole).Error)

	createUser := func(username string, role models.Role) (uint, string) {
		user := models.User{Username: username, Realname: username}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&models.UserRole{UserID: user.ID, RoleID: role.ID}).Error)
		token, err := utils.GenerateToken(user.ID, username, role.RoleCode)
		require.NoError(t, err)
		return user.ID, token
	}
	adminID, adminToken := createUser("delete-admin-a", adminRole)
	otherAdminID, otherAdminToken := createUser("delete-admin-b", adminRole)
	targetID, targetToken := createUser("delete-student", studentRole)

	router := gin.New()
	router.DELETE("/api/perm/member/:id", middleware.AuthRequired(), middleware.RequireRole("school_admin"),
		middleware.RequirePermission("user:assign_role"), DeleteUser)
	router.GET("/session", middleware.AuthRequired(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := func(method, path, token string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		router.ServeHTTP(w, req)
		return w
	}
	deletePath := func(id uint) string { return fmt.Sprintf("/api/perm/member/%d", id) }

	require.Equal(t, http.StatusUnauthorized, request(http.MethodDelete, deletePath(targetID), "").Code)
	require.Equal(t, http.StatusForbidden, request(http.MethodDelete, deletePath(targetID), adminToken).Code)
	permission := models.Permission{Code: "user:assign_role", Name: "分配用户角色"}
	require.NoError(t, db.Create(&permission).Error)
	require.NoError(t, db.Create(&[]models.RolePermission{
		{RoleID: adminRole.ID, PermissionID: permission.ID},
		{RoleID: studentRole.ID, PermissionID: permission.ID},
	}).Error)

	require.Equal(t, http.StatusBadRequest, request(http.MethodDelete, "/api/perm/member/invalid", adminToken).Code)
	require.Equal(t, http.StatusForbidden, request(http.MethodDelete, deletePath(adminID), targetToken).Code)
	require.Equal(t, http.StatusBadRequest, request(http.MethodDelete, deletePath(adminID), adminToken).Code)

	comp := models.CompDirectory{CompCode: "DELETE_USER", CompName: "删除账号测试", ManagerID: adminID}
	require.NoError(t, db.Create(&comp).Error)
	reg := models.Register{CompID: comp.ID, LeaderID: targetID, AdvisorInfo: "[]"}
	require.NoError(t, db.Create(&reg).Error)
	require.Equal(t, http.StatusNoContent, request(http.MethodGet, "/session", targetToken).Code)

	deleted := request(http.MethodDelete, deletePath(targetID), adminToken)
	require.Equal(t, http.StatusOK, deleted.Code, deleted.Body.String())
	require.Equal(t, http.StatusNotFound, request(http.MethodDelete, deletePath(targetID), adminToken).Code)
	require.Equal(t, http.StatusForbidden, request(http.MethodGet, "/session", targetToken).Code)

	var archived models.User
	require.ErrorIs(t, db.First(&archived, targetID).Error, gorm.ErrRecordNotFound)
	require.NoError(t, db.Unscoped().First(&archived, targetID).Error)
	require.True(t, archived.DeletedAt.Valid)
	var retained models.Register
	require.NoError(t, db.First(&retained, reg.ID).Error)
	require.Equal(t, targetID, retained.LeaderID)

	start := make(chan struct{})
	results := make(chan int, 2)
	go func() {
		<-start
		results <- request(http.MethodDelete, deletePath(otherAdminID), adminToken).Code
	}()
	go func() {
		<-start
		results <- request(http.MethodDelete, deletePath(adminID), otherAdminToken).Code
	}()
	close(start)
	require.ElementsMatch(t, []int{http.StatusOK, http.StatusForbidden}, []int{<-results, <-results})

	var remainingAdmins int64
	require.NoError(t, activeRoleMembers(db, adminRole.ID).Count(&remainingAdmins).Error)
	require.EqualValues(t, 1, remainingAdmins)
}
