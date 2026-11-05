package database

import (
	"testing"

	"CompeManage_backend/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 迁移工具深分支（Verify/Rollback/备份冲突）——覆盖 Apply→Verify→Rollback 全链路

func seedMigrationAnomalies(t *testing.T) {
	t.Helper()
	// assignMissingRoles 需要核心角色（guest 等），先补齐
	ensureCoreRoles()
	// managed_college_id 有外键、backfill 按学院名匹配——先建学院
	for _, name := range []string{"计算机学院", "数学学院", "外语学院"} {
		require.NoError(t, DB.Create(&models.College{Name: name}).Error)
	}
	// 迁移工具面向 legacy 形态：user_roles 以独立 id 为主键（当前模型是 user_id 主键，
	// 无法表达多角色异常）。先 ALTER 回 legacy 形态再注入异常。
	require.NoError(t, DB.Exec("ALTER TABLE user_roles MODIFY user_id BIGINT UNSIGNED NOT NULL").Error)
	require.NoError(t, DB.Exec("ALTER TABLE user_roles DROP PRIMARY KEY").Error)
	require.NoError(t, DB.Exec("ALTER TABLE user_roles ADD COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY FIRST").Error)
	// 多角色用户
	u1 := models.User{Username: "MIG1001", Realname: "多角色甲", College: "计算机学院", IdentityType: "student", Grade: "2024"}
	require.NoError(t, DB.Create(&u1).Error)
	require.NoError(t, DB.Create(&models.Role{RoleName: "校管理员", RoleCode: "mig_school_admin"}).Error)
	require.NoError(t, DB.Create(&models.Role{RoleName: "学生", RoleCode: "mig_student"}).Error)
	var r1, r2 models.Role
	require.NoError(t, DB.Where("role_code = ?", "mig_school_admin").First(&r1).Error)
	require.NoError(t, DB.Where("role_code = ?", "mig_student").First(&r2).Error)
	require.NoError(t, DB.Exec("INSERT INTO user_roles (user_id, role_id) VALUES (?, ?), (?, ?)", u1.ID, r1.ID, u1.ID, r2.ID).Error)

	// 无角色用户
	require.NoError(t, DB.Create(&models.User{Username: "MIG1002", Realname: "无角色乙", College: "数学学院"}).Error)

	// 缺身份用户（无 role 且 identity 空）
	require.NoError(t, DB.Create(&models.User{Username: "MIG1003", Realname: "缺身份丙"}).Error)

	// 院管理员未绑定管理学院
	u4 := models.User{Username: "MIG1004", Realname: "院管丁", College: "外语学院", IdentityType: "teacher"}
	require.NoError(t, DB.Create(&u4).Error)
	// 院管理员异常识别按规范角色码 college_admin（ensureCoreRoles 已创建）
	var r4 models.Role
	require.NoError(t, DB.Where("role_code = ?", "college_admin").First(&r4).Error)
	require.NoError(t, DB.Create(&models.UserRole{UserID: u4.ID, RoleID: r4.ID}).Error)
}

func TestVerifyUserRoleMigrationMySQL_Branches(t *testing.T) {
	newSeedMySQLTestDB(t)
	seedMigrationAnomalies(t)

	// ① 存在异常 → 用户角色或人员身份仍存在异常
	report, err := VerifyUserRoleMigration()
	require.ErrorContains(t, err, "用户角色或人员身份仍存在异常")
	assert.NotNil(t, report)

	// ② 异常清干净但无唯一索引 → 唯一约束尚未创建
	require.NoError(t, DB.Exec("DELETE FROM user_roles").Error)
	require.NoError(t, DB.Exec("UPDATE users SET identity_type='student'").Error)
	// 每人恰好一个角色（无多角色/无缺角色）
	var allUsers []models.User
	require.NoError(t, DB.Find(&allUsers).Error)
	var oneRole models.Role
	require.NoError(t, DB.Where("role_code = ?", "mig_student").First(&oneRole).Error)
	for _, u := range allUsers {
		require.NoError(t, DB.Create(&models.UserRole{UserID: u.ID, RoleID: oneRole.ID}).Error)
	}
	report, err = VerifyUserRoleMigration()
	require.ErrorContains(t, err, "user_roles.user_id 唯一约束尚未创建")

	// ③ 院管理员未绑定管理学院（有索引）
	require.NoError(t, ensureUserRoleUniqueIndex())
	var u4 models.User
	require.NoError(t, DB.Where("username = ?", "MIG1004").First(&u4).Error)
	var r4 models.Role
	require.NoError(t, DB.Where("role_code = ?", "college_admin").First(&r4).Error)
	// u4 保持单角色（mig_college_admin），避免多角色异常先于院管分支触发
	require.NoError(t, DB.Exec("DELETE FROM user_roles WHERE user_id = ?", u4.ID).Error)
	require.NoError(t, DB.Create(&models.UserRole{UserID: u4.ID, RoleID: r4.ID}).Error)
	require.NoError(t, DB.Exec("UPDATE users SET identity_type='teacher', managed_college_id=NULL").Error)
	report, err = VerifyUserRoleMigration()
	require.ErrorContains(t, err, "仍有院管理员未绑定管理学院")

	// ④ 全部干净 → nil（managed_college_id 必须指向真实学院）
	var seededCollege models.College
	require.NoError(t, DB.Where("name = ?", "外语学院").First(&seededCollege).Error)
	require.NoError(t, DB.Exec("UPDATE users SET managed_college_id=?", seededCollege.ID).Error)
	report, err = VerifyUserRoleMigration()
	require.NoError(t, err)
	assert.True(t, report.UniqueConstraintAlreadyExist)
}

func TestApplyAndRollbackUserRoleMigrationMySQL_RoundTrip(t *testing.T) {
	newSeedMySQLTestDB(t)
	seedMigrationAnomalies(t)

	// 迁移前：异常齐全
	before, err := BuildUserRoleMigrationReport()
	require.NoError(t, err)
	assert.NotEmpty(t, before.MultiRoleUsers)
	assert.NotEmpty(t, before.NoRoleUsers)
	assert.NotEmpty(t, before.MissingIdentityUsers)
	assert.NotEmpty(t, before.CollegeAdminsWithoutScope)

	// Apply：备份+回填+归一+唯一索引，收尾一致性检查通过
	report, err := ApplyUserRoleMigration("user_roles_backup_push40")
	require.NoError(t, err)
	assert.Equal(t, "user_roles_backup_push40", report.BackupTable)
	assert.Empty(t, report.MultiRoleUsers)
	assert.Empty(t, report.NoRoleUsers)
	assert.Empty(t, report.MissingIdentityUsers)
	assert.Empty(t, report.CollegeAdminsWithoutScope)

	// Verify：迁移后应干净
	_, err = VerifyUserRoleMigration()
	require.NoError(t, err)

	// Rollback：数据回到迁移前（多角色等异常恢复）
	require.NoError(t, RollbackUserRoleMigration("user_roles_backup_push40"))
	after, err := BuildUserRoleMigrationReport()
	require.NoError(t, err)
	assert.NotEmpty(t, after.MultiRoleUsers)
	assert.NotEmpty(t, after.NoRoleUsers)
}

func TestApplyUserRoleMigrationMySQL_BackupCollision(t *testing.T) {
	newSeedMySQLTestDB(t)
	seedMigrationAnomalies(t)

	_, err := ApplyUserRoleMigration("user_roles_backup_dup")
	require.NoError(t, err)
	// 同名备份表二次迁移 → 创建备份表失败
	_, err = ApplyUserRoleMigration("user_roles_backup_dup")
	require.ErrorContains(t, err, "创建角色关联备份表失败")
}

func TestRollbackUserRoleMigrationMySQL_MetaMissing(t *testing.T) {
	newSeedMySQLTestDB(t)
	err := RollbackUserRoleMigration("user_roles_backup_ghost")
	require.ErrorContains(t, err, "读取迁移备份元数据失败")
}
