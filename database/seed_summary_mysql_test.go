package database

import (
	"testing"

	"CompeManage_backend/models"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// 存量库缺 summary 权限体系时必须幂等补齐（否则 /api/summary/* 全员 403）
func TestEnsureSummaryPermissionsMySQL_Backfills(t *testing.T) {
	db := newSeedMySQLTestDB(t)

	// 模拟历史库：只有 competition 目录和三个角色，无 summary 相关权限
	assert.NoError(t, db.Create(&models.Permission{Name: "竞赛管理", Code: "competition", Type: 1, ParentID: 0}).Error)
	for _, code := range []string{"school_admin", "college_admin", "competition_manager"} {
		assert.NoError(t, db.Create(&models.Role{RoleName: code, RoleCode: code}).Error)
	}

	ensureSummaryPermissions()

	var summaryDir models.Permission
	assert.NoError(t, db.Where("code = ?", "summary").First(&summaryDir).Error)
	var parent models.Permission
	assert.NoError(t, db.First(&parent, summaryDir.ParentID).Error)
	assert.Equal(t, "competition", parent.Code)

	for _, code := range []string{"summary:list", "summary:detail", "summary:edit"} {
		var perm models.Permission
		assert.NoError(t, db.Where("code = ?", code).First(&perm).Error, code)
	}

	assert.Greater(t, rolePermissionCount(t, db, "school_admin"), int64(0))
	assert.Greater(t, rolePermissionCount(t, db, "college_admin"), int64(0))
	assert.Greater(t, rolePermissionCount(t, db, "competition_manager"), int64(0))

	// 幂等：再跑不重复建权限
	ensureSummaryPermissions()
	var cnt int64
	db.Model(&models.Permission{}).Where("code LIKE ?", "summary%").Count(&cnt)
	assert.EqualValues(t, 4, cnt) // summary 目录 + 3 个子权限
}

func rolePermissionCount(t *testing.T, db *gorm.DB, roleCode string) int64 {
	t.Helper()
	var role models.Role
	assert.NoError(t, db.Where("role_code = ?", roleCode).First(&role).Error)
	var cnt int64
	assert.NoError(t, db.Table("role_permissions").Where("role_id = ?", role.ID).Count(&cnt).Error)
	return cnt
}
