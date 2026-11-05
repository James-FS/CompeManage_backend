package database

import (
	"testing"

	"CompeManage_backend/models"

	"github.com/stretchr/testify/assert"
)

// 备份表已存在 → 迁移在备份阶段失败
func TestApplyUserRoleMigration_BackupTableExists(t *testing.T) {
	db := newSeedMySQLTestDB(t)
	InitData()
	assert.NoError(t, db.Exec("CREATE TABLE backup_dup (id INT)").Error)

	_, err := ApplyUserRoleMigration("backup_dup")
	assert.ErrorContains(t, err, "创建角色关联备份表失败")
}

// 迁移后一致性检查失败：college_admin 的学院无法匹配 → managed_college_id 补不上
func TestApplyUserRoleMigration_ConsistencyFailure(t *testing.T) {
	db := newSeedMySQLTestDB(t)
	InitData()
	assert.NoError(t, db.Exec("ALTER TABLE user_roles MODIFY user_id BIGINT UNSIGNED NOT NULL").Error)
	assert.NoError(t, db.Exec("ALTER TABLE user_roles DROP PRIMARY KEY").Error)
	assert.NoError(t, db.Exec("ALTER TABLE user_roles ADD COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY FIRST").Error)

	var caRole models.Role
	assert.NoError(t, db.Where("role_code = ?", "college_admin").First(&caRole).Error)
	admin := models.User{Username: "800010", Realname: "院管", IdentityType: "staff", College: "查无此院"}
	assert.NoError(t, db.Create(&admin).Error)
	assert.NoError(t, db.Create(&models.UserRole{UserID: admin.ID, RoleID: caRole.ID}).Error)

	_, err := ApplyUserRoleMigration("backup_consistency_test")
	assert.ErrorContains(t, err, "迁移后一致性检查失败")
}
