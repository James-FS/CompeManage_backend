package database

import (
	"testing"

	"CompeManage_backend/models"

	"github.com/stretchr/testify/assert"
)

// 存量库补角色：缺失的 teacher 等必须被幂等补齐
func TestEnsureCoreRolesMySQL_BackfillsMissingRoles(t *testing.T) {
	db := newSeedMySQLTestDB(t)

	// 模拟历史库：只有 student，缺 teacher/school_admin 等
	assert.NoError(t, db.Create(&models.Role{RoleName: "学生", RoleCode: "student", Description: "参与竞赛"}).Error)

	ensureCoreRoles()

	var teacher models.Role
	assert.NoError(t, db.Where("role_code = ?", "teacher").First(&teacher).Error, "teacher 角色必须被补齐")

	var count int64
	db.Model(&models.Role{}).Count(&count)
	assert.EqualValues(t, 7, count)

	// 幂等：再跑一次不重复建
	ensureCoreRoles()
	db.Model(&models.Role{}).Count(&count)
	assert.EqualValues(t, 7, count)
}
