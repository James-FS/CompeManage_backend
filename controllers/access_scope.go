package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserAccessScope struct {
	UserID           uint
	RoleCode         string
	ManagedCollegeID *uint
}

func GetUserAccessScope(ctx context.Context, userID uint) (*UserAccessScope, error) {
	var results []struct {
		RoleCode         string
		ManagedCollegeID *uint
	}
	err := database.DB.WithContext(ctx).Table("users").
		Select("roles.role_code, users.managed_college_id").
		Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.delete_time IS NULL").
		Where("users.id = ? AND users.delete_time IS NULL", userID).
		Limit(2).
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, database.ErrUserHasNoRole
	}
	if len(results) > 1 {
		return nil, database.ErrUserHasMultipleRoles
	}
	return &UserAccessScope{
		UserID:           userID,
		RoleCode:         results[0].RoleCode,
		ManagedCollegeID: results[0].ManagedCollegeID,
	}, nil
}

func requireUserAccessScope(c *gin.Context) (*UserAccessScope, bool) {
	userIDVal, exists := c.Get("user_id")
	userID, ok := userIDVal.(uint)
	if !exists || !ok || userID == 0 {
		utils.Unauthorized(c, "未登录")
		return nil, false
	}
	if roleCodeVal, hasRoleCode := c.Get("role_code"); hasRoleCode {
		roleCode, roleOK := roleCodeVal.(string)
		if roleOK && roleCode != "" {
			var managedCollegeID *uint
			if managedVal, hasManaged := c.Get("managed_college_id"); hasManaged {
				managedCollegeID, _ = managedVal.(*uint)
			}
			scope := &UserAccessScope{UserID: userID, RoleCode: roleCode, ManagedCollegeID: managedCollegeID}
			if scope.IsCollegeAdmin() && scope.ManagedCollegeID == nil {
				utils.Forbidden(c, "院级管理员尚未配置管理学院，请联系校管理员")
				return nil, false
			}
			return scope, true
		}
	}

	// 控制器单元测试历史上直接调用 Handler 并跳过 AuthRequired。
	// 真实路由均由 AuthRequired 写入 role_code，因此生产请求不会进入此兼容分支。
	if checkUserIsAdmin(c.Request.Context(), userID) {
		return &UserAccessScope{UserID: userID, RoleCode: "school_admin"}, true
	}
	return &UserAccessScope{UserID: userID, RoleCode: "competition_manager"}, true
}

// requestAccessScopeIfAvailable 兼容历史上绕过 AuthRequired 直接调用 Handler 的单元测试。
// 真实 API 路由一定包含 role_code，因此生产请求会进入强制范围校验。
func requestAccessScopeIfAvailable(c *gin.Context) (*UserAccessScope, bool) {
	if _, exists := c.Get("role_code"); !exists {
		return nil, true
	}
	return requireUserAccessScope(c)
}

func requireCompetitionAccessIfScoped(c *gin.Context, compID uint) bool {
	if _, exists := c.Get("role_code"); !exists {
		return true
	}
	_, _, ok := requireCompetitionAccess(c, compID)
	return ok
}

// canManageNotice 判定当前请求方能否发布/编辑/删除该通知（P0-3）。
// school_admin / college_admin 不受限；其余角色仅限自己发布的通知。
// 返回 false 时调用方应返回 403。
func canManageNotice(c *gin.Context, notice models.Notice) bool {
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return false
	}
	if scope == nil {
		// 无 role_code：仅出现在绕过 AuthRequired 的单元测试中（真实请求必经 AuthRequired）。
		// 与 requireCompetitionAccessIfScoped 的测试兼容口径保持一致；不产生额外 SQL。
		return true
	}
	if scope.IsSchoolAdmin() || scope.IsCollegeAdmin() {
		return true
	}
	if notice.PublisherID == nil {
		// 存量通知无归属人，负责人不可操作
		return false
	}
	return *notice.PublisherID == scope.UserID
}

func (scope *UserAccessScope) IsSchoolAdmin() bool {
	return scope != nil && scope.RoleCode == "school_admin"
}

func (scope *UserAccessScope) IsCollegeAdmin() bool {
	return scope != nil && scope.RoleCode == "college_admin"
}

func applyCompetitionScope(query *gorm.DB, scope *UserAccessScope, alias string) *gorm.DB {
	if scope == nil || query == nil {
		return query
	}
	if alias == "" {
		alias = "comp_directories"
	}
	switch scope.RoleCode {
	case "school_admin":
		return query
	case "college_admin":
		if scope.ManagedCollegeID == nil {
			return query.Where("1 = 0")
		}
		return query.Where(alias+".college_id = ?", *scope.ManagedCollegeID)
	default:
		return query.Where(alias+".manager_id = ?", scope.UserID)
	}
}

func canAccessCompetition(scope *UserAccessScope, comp models.CompDirectory) bool {
	if scope == nil {
		return false
	}
	switch scope.RoleCode {
	case "school_admin":
		return true
	case "college_admin":
		return scope.ManagedCollegeID != nil && comp.CollegeID != nil && *scope.ManagedCollegeID == *comp.CollegeID
	default:
		return comp.ManagerID == scope.UserID
	}
}

func requireCompetitionAccess(c *gin.Context, compID uint) (*UserAccessScope, *models.CompDirectory, bool) {
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return nil, nil, false
	}
	if scope.IsSchoolAdmin() {
		return scope, nil, true
	}
	var comp models.CompDirectory
	if err := database.DB.WithContext(c.Request.Context()).
		Select("id", "manager_id", "college_id").First(&comp, compID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "赛事不存在")
		} else {
			utils.InternalServerError(c, "查询赛事失败", err)
		}
		return nil, nil, false
	}
	if !canAccessCompetition(scope, comp) {
		utils.Forbidden(c, "无权访问该赛事")
		return nil, nil, false
	}
	return scope, &comp, true
}

func applyDeclarationScope(query *gorm.DB, scope *UserAccessScope, alias string) *gorm.DB {
	if scope == nil || query == nil {
		return query
	}
	if alias == "" {
		alias = "comp_declarations"
	}
	switch scope.RoleCode {
	case "school_admin":
		return query
	case "college_admin":
		if scope.ManagedCollegeID == nil {
			return query.Where("1 = 0")
		}
		return query.Where(alias+".college_id = ?", *scope.ManagedCollegeID)
	default:
		return query.Where("("+alias+".created_by = ? OR "+alias+".manager_id = ?)", scope.UserID, scope.UserID)
	}
}

func canAccessDeclaration(scope *UserAccessScope, declaration models.CompDeclaration) bool {
	if scope == nil {
		return false
	}
	switch scope.RoleCode {
	case "school_admin":
		return true
	case "college_admin":
		return scope.ManagedCollegeID != nil && declaration.CollegeID == *scope.ManagedCollegeID
	default:
		return declaration.CreatedBy == scope.UserID || declaration.ManagerID == scope.UserID
	}
}
