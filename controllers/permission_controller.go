package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetAllPermissions(c *gin.Context) {
	var perms []models.Permission
	// 查询所有权限，按 ID 排序
	database.DB.Order("id asc").Find(&perms)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "获取成功",
		"data": perms,
	})
}

func GetAllRoles(c *gin.Context) {
	var roles []models.Role
	database.DB.Preload("Permissions").Order("id asc").Find(&roles)
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": roles})
}

// AssignPermReq 分配权限参数结构
type AssignPermReq struct {
	RoleID  uint   `json:"role_id" binding:"required"`
	PermIDs []uint `json:"perm_ids"`
}

// AssignPermissions 给角色分配权限
func AssignPermissions(c *gin.Context) {
	var req AssignPermReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var role models.Role
	if err := database.DB.First(&role, req.RoleID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "角色不存在"})
		return
	}

	var perms []models.Permission
	if len(req.PermIDs) > 0 {
		database.DB.Where("id IN ?", req.PermIDs).Find(&perms)
	}

	// 替换关联
	database.DB.Model(&role).Association("Permissions").Replace(&perms)

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "权限分配成功"})
}
