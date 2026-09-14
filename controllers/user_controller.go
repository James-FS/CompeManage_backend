package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserManageListReq struct {
	Page             int    `form:"page"`
	PageSize         int    `form:"page_size"`
	Search           string `form:"search"`
	RoleID           uint   `form:"role_id"`
	IdentityType     string `form:"identity_type"`
	ManagedCollegeID uint   `form:"managed_college_id"`
}

type RoleBrief struct {
	ID       uint   `json:"id"`
	RoleName string `json:"role_name"`
	RoleCode string `json:"role_code"`
}

type CollegeBrief struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type UserWithRoleResp struct {
	ID               uint          `json:"id"`
	Username         string        `json:"username"`
	Realname         string        `json:"realname"`
	College          string        `json:"college"`
	Grade            string        `json:"grade"`
	IdentityType     string        `json:"identity_type"`
	Role             *RoleBrief    `json:"role"`
	RoleConflict     bool          `json:"role_conflict"`
	ManagedCollege   *CollegeBrief `json:"managed_college"`
	ManagedCollegeID *uint         `json:"managed_college_id"`
}

func GetAllUsers(c *gin.Context) {
	var req UserManageListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 50 {
		req.PageSize = 50
	}
	req.Search = strings.TrimSpace(req.Search)
	req.IdentityType = strings.TrimSpace(req.IdentityType)
	if req.IdentityType != "" && !validIdentityType(req.IdentityType) {
		utils.BadRequest(c, "人员身份类型无效")
		return
	}

	query := database.DB.WithContext(c.Request.Context()).Model(&models.User{})
	if req.Search != "" {
		keyword := "%" + req.Search + "%"
		query = query.Where("username LIKE ? OR realname LIKE ? OR college LIKE ?", keyword, keyword, keyword)
	}
	if req.RoleID != 0 {
		query = query.Where("EXISTS (SELECT 1 FROM user_roles WHERE user_roles.user_id = users.id AND user_roles.role_id = ?)", req.RoleID)
	}
	if req.IdentityType != "" {
		query = query.Where("identity_type = ?", req.IdentityType)
	}
	if req.ManagedCollegeID != 0 {
		query = query.Where("managed_college_id = ?", req.ManagedCollegeID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "统计用户数量失败", err)
		return
	}

	var users []models.User
	if err := query.
		Preload("Roles", func(db *gorm.DB) *gorm.DB { return db.Order("roles.id ASC") }).
		Preload("ManagedCollege").
		Order("users.create_time DESC, users.id DESC").
		Offset((req.Page - 1) * req.PageSize).
		Limit(req.PageSize).
		Find(&users).Error; err != nil {
		utils.InternalServerError(c, "查询用户列表失败", err)
		return
	}

	list := make([]UserWithRoleResp, 0, len(users))
	for _, user := range users {
		item := UserWithRoleResp{
			ID:               user.ID,
			Username:         user.Username,
			Realname:         user.Realname,
			College:          user.College,
			Grade:            user.Grade,
			IdentityType:     user.IdentityType,
			RoleConflict:     len(user.Roles) > 1,
			ManagedCollegeID: user.ManagedCollegeID,
		}
		if len(user.Roles) == 1 {
			item.Role = &RoleBrief{
				ID:       user.Roles[0].ID,
				RoleName: user.Roles[0].RoleName,
				RoleCode: user.Roles[0].RoleCode,
			}
		}
		if user.ManagedCollege != nil {
			item.ManagedCollege = &CollegeBrief{ID: user.ManagedCollege.ID, Name: user.ManagedCollege.Name}
		}
		list = append(list, item)
	}

	utils.Success(c, gin.H{
		"list":      list,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

type AssignUserRoleReq struct {
	RoleID           uint  `json:"role_id" binding:"required"`
	ManagedCollegeID *uint `json:"managed_college_id"`
}

type roleAssignmentBusinessError struct {
	status  int
	message string
}

func (e *roleAssignmentBusinessError) Error() string { return e.message }

func AssignUserRole(c *gin.Context) {
	targetID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || targetID64 == 0 {
		utils.BadRequest(c, "用户ID无效")
		return
	}
	targetUserID := uint(targetID64)

	var req AssignUserRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	operatorVal, exists := c.Get("user_id")
	operatorID, ok := operatorVal.(uint)
	if !exists || !ok || operatorID == 0 {
		utils.Unauthorized(c, "未登录")
		return
	}

	var targetUser models.User
	var targetRole models.Role
	var managedCollege *models.College
	var changed bool
	var oldRoleID *uint
	var oldManagedCollegeID *uint
	requestIP := c.ClientIP()

	err = database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&targetUser, targetUserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &roleAssignmentBusinessError{status: http.StatusNotFound, message: "用户不存在"}
			}
			return err
		}
		if err := tx.First(&targetRole, req.RoleID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &roleAssignmentBusinessError{status: http.StatusBadRequest, message: "角色不存在"}
			}
			return err
		}

		var schoolAdminRole models.Role
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("role_code = ?", "school_admin").First(&schoolAdminRole).Error; err != nil {
			return fmt.Errorf("查询校管理员角色失败: %w", err)
		}

		var currentLinks []models.UserRole
		if err := tx.Where("user_id = ?", targetUserID).Find(&currentLinks).Error; err != nil {
			return err
		}
		if len(currentLinks) > 1 {
			return &roleAssignmentBusinessError{status: http.StatusConflict, message: "用户存在多个角色，请先执行单角色迁移"}
		}
		if len(currentLinks) == 1 {
			old := currentLinks[0].RoleID
			oldRoleID = &old
		}
		if targetUser.ManagedCollegeID != nil {
			old := *targetUser.ManagedCollegeID
			oldManagedCollegeID = &old
		}

		if targetRole.RoleCode == "college_admin" {
			if req.ManagedCollegeID == nil || *req.ManagedCollegeID == 0 {
				return &roleAssignmentBusinessError{status: http.StatusBadRequest, message: "院级管理员必须指定管理学院"}
			}
			var college models.College
			if err := tx.First(&college, *req.ManagedCollegeID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return &roleAssignmentBusinessError{status: http.StatusBadRequest, message: "管理学院不存在"}
				}
				return err
			}
			managedCollege = &college
		} else if req.ManagedCollegeID != nil {
			return &roleAssignmentBusinessError{status: http.StatusBadRequest, message: "非院级管理员不能设置管理学院"}
		}

		if targetUserID == operatorID && targetRole.RoleCode != "school_admin" {
			return &roleAssignmentBusinessError{status: http.StatusBadRequest, message: "不能修改自己的校级管理员角色"}
		}

		if oldRoleID != nil && *oldRoleID == schoolAdminRole.ID && targetRole.ID != schoolAdminRole.ID {
			var activeAdminLinks []models.UserRole
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Model(&models.UserRole{}).
				Joins("JOIN users ON users.id = user_roles.user_id AND users.delete_time IS NULL").
				Where("user_roles.role_id = ?", schoolAdminRole.ID).
				Find(&activeAdminLinks).Error; err != nil {
				return err
			}
			if len(activeAdminLinks) <= 1 {
				return &roleAssignmentBusinessError{status: http.StatusBadRequest, message: "系统必须至少保留一个校级管理员"}
			}
		}

		newManagedCollegeID := req.ManagedCollegeID
		if targetRole.RoleCode != "college_admin" {
			newManagedCollegeID = nil
		}
		if oldRoleID != nil && *oldRoleID == targetRole.ID && equalUintPointers(oldManagedCollegeID, newManagedCollegeID) {
			return nil
		}

		if err := database.SetUserRole(tx, targetUserID, targetRole.ID); err != nil {
			return err
		}
		if err := tx.Model(&models.User{}).Where("id = ?", targetUserID).
			Update("managed_college_id", newManagedCollegeID).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.UserRoleAudit{
			OperatorID:          operatorID,
			TargetUserID:        targetUserID,
			OldRoleID:           oldRoleID,
			NewRoleID:           targetRole.ID,
			OldManagedCollegeID: oldManagedCollegeID,
			NewManagedCollegeID: newManagedCollegeID,
			RequestIP:           requestIP,
		}).Error; err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		var businessErr *roleAssignmentBusinessError
		if errors.As(err, &businessErr) {
			utils.Error(c, businessErr.status, businessErr.status, businessErr.message)
			return
		}
		utils.InternalServerError(c, "角色分配失败", err)
		return
	}

	responseData := gin.H{
		"role_updated":       changed,
		"cache_refreshed":    true,
		"cache_retry_queued": false,
		"role": RoleBrief{
			ID:       targetRole.ID,
			RoleName: targetRole.RoleName,
			RoleCode: targetRole.RoleCode,
		},
		"managed_college": managedCollege,
	}
	if !changed {
		utils.SuccessWithMessage(c, "角色配置未发生变化", responseData)
		return
	}

	if cacheErr := middleware.ClearUserPermissionCache(targetUserID); cacheErr != nil {
		responseData["cache_refreshed"] = false
		if queueErr := middleware.QueuePermissionCacheInvalidation(targetUserID, cacheErr); queueErr != nil {
			responseData["manual_follow_up_required"] = true
			responseData["cache_ttl_seconds"] = int(middleware.PermissionCacheTTL.Seconds())
			slog.Error("权限缓存刷新失败且创建重试任务失败",
				"target_user_id", targetUserID,
				"cache_error", cacheErr,
				"queue_error", queueErr,
			)
			utils.SuccessAccepted(c, "角色已更新，但权限缓存清理失败且未能创建重试任务；旧权限将在缓存过期后失效", responseData)
			return
		}
		responseData["cache_retry_queued"] = true
		utils.SuccessAccepted(c, "角色已更新，权限缓存正在重试刷新", responseData)
		return
	}

	slog.Info("用户角色变更",
		"operator_id", operatorID,
		"target_user_id", targetUserID,
		"old_role_id", nullableUintLogValue(oldRoleID),
		"new_role_id", targetRole.ID,
		"old_managed_college_id", nullableUintLogValue(oldManagedCollegeID),
		"new_managed_college_id", nullableUintLogValue(req.ManagedCollegeID),
		"ip", requestIP,
	)
	utils.SuccessWithMessage(c, "角色分配成功，请通知目标用户重新登录", responseData)
}

func validIdentityType(value string) bool {
	switch value {
	case "staff", "student", "postgraduate", "external":
		return true
	default:
		return false
	}
}

func equalUintPointers(left, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func nullableUintLogValue(value *uint) interface{} {
	if value == nil {
		return nil
	}
	return *value
}
