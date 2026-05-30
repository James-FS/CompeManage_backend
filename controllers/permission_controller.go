package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"

	"github.com/gin-gonic/gin"
)

func GetAllPermissions(c *gin.Context) {
	var perms []models.Permission
	database.DB.WithContext(c.Request.Context()).Order("id asc").Find(&perms)
	utils.SuccessWithMessage(c, "获取成功", perms)
}

func GetAllRoles(c *gin.Context) {
	var roles []models.Role
	database.DB.WithContext(c.Request.Context()).Preload("Permissions").Order("id asc").Find(&roles)
	utils.SuccessWithMessage(c, "获取成功", roles)
}

type AssignPermReq struct {
	RoleID  uint   `json:"role_id" binding:"required"`
	PermIDs []uint `json:"perm_ids"`
}

func AssignPermissions(c *gin.Context) {
	var req AssignPermReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	db := database.DB.WithContext(c.Request.Context())

	var role models.Role
	if err := db.First(&role, req.RoleID).Error; err != nil {
		utils.NotFound(c, "角色不存在")
		return
	}

	var perms []models.Permission
	if len(req.PermIDs) > 0 {
		db.Where("id IN ?", req.PermIDs).Find(&perms)
	}

	db.Model(&role).Association("Permissions").Replace(&perms)
	utils.SuccessWithMessage(c, "权限分配成功", nil)
}
