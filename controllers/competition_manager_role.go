package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RolePromotion 记录一次事务内发生的自动提权，供事务提交后写审计与清缓存。
type RolePromotion struct {
	UserID    uint
	OldRoleID uint
	NewRoleID uint
}

// promoteCompetitionManagerIfTeacher 仅在目标用户当前唯一角色为 teacher 时提升为 competition_manager。
// 返回 (oldRoleID, newRoleID)；未提升时 oldRoleID 为 0。
// 硬约束：管理员/学生/专家/访客等非 teacher 角色一律不动，防止误降级管理员。
func promoteCompetitionManagerIfTeacher(tx *gorm.DB, userID uint) (oldRoleID uint, newRoleID uint, err error) {
	if userID == 0 {
		return 0, 0, nil
	}
	var links []models.UserRole
	if err := tx.Where("user_id = ?", userID).Limit(2).Find(&links).Error; err != nil {
		return 0, 0, err
	}
	if len(links) != 1 {
		// 多角色或无角色：不在自动流程中处理
		return 0, 0, nil
	}
	var role models.Role
	if err := tx.First(&role, links[0].RoleID).Error; err != nil {
		return 0, 0, err
	}
	if role.RoleCode != "teacher" {
		// 管理员/学生/专家/访客一律不动
		return 0, 0, nil
	}
	var managerRole models.Role
	if err := tx.Where("role_code = ?", "competition_manager").First(&managerRole).Error; err != nil {
		return 0, 0, err
	}
	if err := database.SetUserRole(tx, userID, managerRole.ID); err != nil {
		return 0, 0, err
	}
	return role.ID, managerRole.ID, nil
}

// demoteCompetitionManagerIfIdle 若该教职工已不再负责任何赛事与申报，退回 teacher。
//
// ⚠ 本次不启用（已决策：不做自动降级）：函数保留但不在任何路径挂载、不写测试。
// 将来启用时必须先增加来源标记（UserRoleAudit.Reason），仅回收非 manual 来源的用户，
// 避免误降级管理员手工授权的账号。
func demoteCompetitionManagerIfIdle(tx *gorm.DB, userID uint) (oldRoleID uint, err error) {
	if userID == 0 {
		return 0, nil
	}
	var user models.User
	if err := tx.First(&user, userID).Error; err != nil {
		return 0, err
	}
	if user.IdentityType != "staff" {
		return 0, nil // 只回收教职工，避免误动管理员/专家
	}
	var links []models.UserRole
	if err := tx.Where("user_id = ?", userID).Limit(2).Find(&links).Error; err != nil {
		return 0, err
	}
	if len(links) != 1 {
		return 0, nil
	}
	var role models.Role
	if err := tx.First(&role, links[0].RoleID).Error; err != nil {
		return 0, err
	}
	if role.RoleCode != "competition_manager" {
		return 0, nil
	}

	var cnt int64
	if err := tx.Model(&models.CompDirectory{}).
		Where("manager_id = ?", userID).Count(&cnt).Error; err != nil {
		return 0, err
	}
	if cnt > 0 {
		return 0, nil
	}
	if err := tx.Model(&models.CompDeclaration{}).
		Where("manager_id = ?", userID).Count(&cnt).Error; err != nil {
		return 0, err
	}
	if cnt > 0 {
		return 0, nil
	}

	var teacherRole models.Role
	if err := tx.Where("role_code = ?", "teacher").First(&teacherRole).Error; err != nil {
		return 0, err
	}
	if err := database.SetUserRole(tx, userID, teacherRole.ID); err != nil {
		return 0, err
	}
	return role.ID, nil
}

// 审计来源常量（写入 UserRoleAudit.Reason）。
const (
	auditReasonManual      = "manual"
	auditReasonCompetition = "competition"
	auditReasonImport      = "import"
	auditReasonDeclare     = "declare"
)

// finalizeRolePromotions 在事务提交后执行：写审计记录并清理被提升用户的权限缓存。
// 必须在数据库事务成功提交之后调用；缓存清理失败时入队重试（沿用既有机制）。
func finalizeRolePromotions(c *gin.Context, promotions []RolePromotion, reason string, sourceID *uint) {
	if len(promotions) == 0 {
		return
	}
	operatorID := currentOperatorID(c)
	requestIP := c.ClientIP()

	for _, p := range promotions {
		old := p.OldRoleID
		if err := database.DB.Create(&models.UserRoleAudit{
			OperatorID:   operatorID,
			TargetUserID: p.UserID,
			OldRoleID:    &old,
			NewRoleID:    p.NewRoleID,
			RequestIP:    requestIP,
			Reason:       reason,
			SourceID:     sourceID,
		}).Error; err != nil {
			slog.Error("写入角色变更审计失败",
				"operator_id", operatorID,
				"target_user_id", p.UserID,
				"old_role_id", p.OldRoleID,
				"new_role_id", p.NewRoleID,
				"reason", reason,
				"error", err,
			)
		}
		if err := middleware.ClearUserPermissionCache(p.UserID); err != nil {
			if qErr := middleware.QueuePermissionCacheInvalidation(p.UserID, err); qErr != nil {
				slog.Error("权限缓存清理与入队均失败",
					"user_id", p.UserID,
					"cache_error", err,
					"queue_error", qErr,
				)
			}
		}
		slog.Info("自动提升赛事负责人",
			"operator_id", operatorID,
			"target_user_id", p.UserID,
			"old_role_id", p.OldRoleID,
			"new_role_id", p.NewRoleID,
			"reason", reason,
			"source_id", sourceID,
		)
	}
}

// currentOperatorID 从上下文取当前操作者ID；缺失时返回 0（测试或未认证场景）。
func currentOperatorID(c *gin.Context) uint {
	if c == nil {
		return 0
	}
	if v, ok := c.Get("user_id"); ok {
		if uid, ok2 := v.(uint); ok2 {
			return uid
		}
	}
	return 0
}

// promotedUsersResp 组装接口响应中的 promoted_users 字段，供前端提示重新登录。
func promotedUsersResp(c *gin.Context, promotions []RolePromotion) []gin.H {
	if len(promotions) == 0 {
		return []gin.H{}
	}
	list := make([]gin.H, 0, len(promotions))
	for _, p := range promotions {
		var user models.User
		item := gin.H{"id": p.UserID}
		if err := database.DB.Select("id", "username", "realname").First(&user, p.UserID).Error; err == nil {
			item["username"] = user.Username
			item["realname"] = user.Realname
		}
		list = append(list, item)
	}
	return list
}
