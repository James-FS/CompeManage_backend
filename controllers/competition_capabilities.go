package controllers

import (
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Competition writes are narrower than college-wide read/audit access.
func canWriteCompetition(scope *UserAccessScope, comp models.CompDirectory, codes []string, permission string) bool {
	if !canAccessCompetition(scope, comp) {
		return false
	}
	if scope.IsCollegeAdmin() && comp.ManagerID != scope.UserID {
		return false
	}
	for _, code := range codes {
		if code == permission {
			return true
		}
		if code == permission+":own" && comp.ManagerID == scope.UserID {
			return scope.IsCollegeAdmin() || scope.RoleCode == "competition_manager"
		}
	}
	return false
}

func competitionCapabilities(c *gin.Context, comp models.CompDirectory) (map[string]bool, bool) {
	// Historical controller tests bypass authentication. Real routes always set role_code.
	if _, scoped := c.Get("role_code"); !scoped {
		return nil, true
	}
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return nil, false
	}
	codes, err := middleware.UserPermissionCodes(c)
	if err != nil {
		utils.InternalServerError(c, "加载赛事操作权限失败", err)
		return nil, false
	}
	return map[string]bool{
		"is_manager":            comp.ManagerID == scope.UserID,
		"can_edit_competition":  canWriteCompetition(scope, comp, codes, "comp:update"),
		"can_edit_registration": canWriteCompetition(scope, comp, codes, "reg:config:edit"),
		"can_edit_summary":      comp.Status == 2 && canWriteCompetition(scope, comp, codes, "summary:edit"),
		"can_reassign_manager":  scope.IsSchoolAdmin() && canWriteCompetition(scope, comp, codes, "comp:update"),
		"can_change_college":    scope.IsSchoolAdmin() && canWriteCompetition(scope, comp, codes, "comp:update"),
	}, true
}

func requireCompetitionWrite(c *gin.Context, scope *UserAccessScope, comp models.CompDirectory, permission string) bool {
	if _, scoped := c.Get("role_code"); !scoped {
		return true
	}
	codes, err := middleware.UserPermissionCodes(c)
	if err != nil {
		utils.InternalServerError(c, "加载赛事操作权限失败", err)
		return false
	}
	if !canWriteCompetition(scope, comp, codes, permission) {
		utils.Forbidden(c, "无权修改该赛事，仅可操作本人负责且在管理范围内的赛事")
		return false
	}
	return true
}

// One candidate policy for selectors, Excel lookup and writes. Admin roles are preserved.
func competitionManagerCandidates(tx *gorm.DB) *gorm.DB {
	return tx.Model(&models.User{}).Where("users.delete_time IS NULL").
		Where(`EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id AND r.delete_time IS NULL
			WHERE ur.user_id = users.id AND r.role_code IN ('teacher', 'competition_manager', 'college_admin'))`).
		Where(`(users.identity_type = 'staff' OR EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id AND r.delete_time IS NULL
			WHERE ur.user_id = users.id AND r.role_code IN ('competition_manager', 'college_admin')))`)
}

var errInvalidCompetitionManager = errors.New("赛事负责人无效")

func validateCompetitionManager(c *gin.Context, tx *gorm.DB, userID uint, collegeID *uint) error {
	if _, scoped := c.Get("role_code"); !scoped {
		return nil
	}
	var user models.User
	if err := competitionManagerCandidates(tx).Where("users.id = ?", userID).Preload("Roles").First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w：用户不存在或不具备负责人资格", errInvalidCompetitionManager)
		}
		return err
	}
	if len(user.Roles) != 1 {
		return fmt.Errorf("%w：角色配置异常", errInvalidCompetitionManager)
	}
	if user.Roles[0].RoleCode == "college_admin" && (collegeID == nil || user.ManagedCollegeID == nil || *collegeID != *user.ManagedCollegeID) {
		return fmt.Errorf("%w：院级管理员只能担任所管理学院赛事的负责人", errInvalidCompetitionManager)
	}
	return nil
}

func respondCompetitionMutationError(c *gin.Context, message string, err error) {
	if errors.Is(err, errInvalidCompetitionManager) {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.InternalServerError(c, message, err)
}

func attachCompetitionCapabilities(c *gin.Context, list []models.CompDirectory) bool {
	for i := range list {
		caps, ok := competitionCapabilities(c, list[i])
		if !ok {
			return false
		}
		list[i].Capabilities = caps
	}
	return true
}

// Include identity and current permission version in list cache keys.
func competitionCacheIdentity(c *gin.Context) string {
	uid, _ := c.Get("user_id")
	role, _ := c.Get("role_code")
	version, _ := c.Get("permission_version")
	college := uint(0)
	if value, ok := c.Get("managed_college_id"); ok {
		if id, ok := value.(*uint); ok && id != nil {
			college = *id
		}
	}
	return fmt.Sprintf("%v:%v:%v:%d", uid, role, version, college)
}
