package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetAllPermissions(c *gin.Context) {
	var perms []models.Permission
	if err := database.DB.WithContext(c.Request.Context()).Order("id asc").Find(&perms).Error; err != nil {
		utils.InternalServerError(c, "获取权限列表失败", err)
		return
	}
	utils.SuccessWithMessage(c, "获取成功", perms)
}

func GetAllRoles(c *gin.Context) {
	var roles []models.Role
	if err := database.DB.WithContext(c.Request.Context()).Preload("Permissions").Order("id asc").Find(&roles).Error; err != nil {
		utils.InternalServerError(c, "获取角色列表失败", err)
		return
	}
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "角色不存在")
		} else {
			utils.InternalServerError(c, "查询角色失败", err)
		}
		return
	}

	permIDs := uniqueUintIDs(req.PermIDs)
	if len(permIDs) != len(req.PermIDs) {
		utils.BadRequest(c, "权限ID无效或重复")
		return
	}

	var perms []models.Permission
	if len(permIDs) > 0 {
		if err := db.Where("id IN ?", permIDs).Find(&perms).Error; err != nil {
			utils.InternalServerError(c, "查询权限失败", err)
			return
		}
		if len(perms) != len(permIDs) {
			utils.BadRequest(c, "部分权限不存在")
			return
		}
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		// UPDATE 同时锁定角色行，串行化同一角色的并发权限替换。
		if err := tx.Model(&role).Updates(map[string]interface{}{
			"update_time":        time.Now(),
			"permission_version": gorm.Expr("permission_version + 1"),
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", role.ID).Delete(&models.RolePermission{}).Error; err != nil {
			return err
		}
		if len(perms) == 0 {
			return nil
		}
		links := make([]models.RolePermission, 0, len(perms))
		for _, permission := range perms {
			links = append(links, models.RolePermission{RoleID: role.ID, PermissionID: permission.ID})
		}
		return tx.Create(&links).Error
	}); err != nil {
		utils.InternalServerError(c, "权限分配失败", err)
		return
	}

	utils.SuccessWithMessage(c, "权限分配成功", nil)
}

func uniqueUintIDs(values []uint) []uint {
	seen := make(map[uint]struct{}, len(values))
	result := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
