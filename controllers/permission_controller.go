package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"errors"
	"log/slog"
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
		if err := tx.Model(&role).Update("update_time", time.Now()).Error; err != nil {
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

	if middleware.GetRedisClient() == nil {
		utils.SuccessWithMessage(c, "权限分配成功", nil)
		return
	}

	var userIDs []uint
	if err := db.Table("user_roles").Where("role_id = ?", role.ID).Pluck("user_id", &userIDs).Error; err != nil {
		utils.InternalServerError(c, "权限已更新，但查询受影响用户失败", err)
		return
	}
	failedUsers := make([]uint, 0)
	retryQueuedUsers := make([]uint, 0)
	retryQueueFailedUsers := make([]uint, 0)
	for _, userID := range userIDs {
		if err := middleware.ClearUserPermissionCache(userID); err != nil {
			failedUsers = append(failedUsers, userID)
			if queueErr := middleware.QueuePermissionCacheInvalidation(userID, err); queueErr != nil {
				retryQueueFailedUsers = append(retryQueueFailedUsers, userID)
				slog.Error("创建权限缓存失效重试任务失败", "user_id", userID, "error", queueErr)
			} else {
				retryQueuedUsers = append(retryQueuedUsers, userID)
			}
		}
	}
	if len(failedUsers) > 0 {
		responseData := gin.H{
			"permission_updated":       true,
			"cache_refreshed":          false,
			"affected_users":           len(userIDs),
			"failed_users":             failedUsers,
			"retry_queued_users":       retryQueuedUsers,
			"retry_queue_failed_users": retryQueueFailedUsers,
		}
		if len(retryQueueFailedUsers) > 0 {
			responseData["manual_follow_up_required"] = true
			responseData["cache_ttl_seconds"] = int(middleware.PermissionCacheTTL.Seconds())
			utils.SuccessAccepted(c, "权限已更新，但部分用户缓存清理失败且未能创建重试任务；旧权限将在缓存过期后失效", responseData)
			return
		}
		utils.SuccessAccepted(c, "权限已更新，部分用户缓存正在重试刷新", responseData)
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
