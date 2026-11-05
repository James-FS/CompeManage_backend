package database

import (
	"fmt"
	"testing"

	"CompeManage_backend/config"

	"CompeManage_backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// ===================== db.go 生命周期（隔离 MySQL） =====================

// Init/InitWithoutMigrate/autoMigrate/GetDB：指向隔离库执行，绝不触碰业务库
func TestDBLifecycleMySQL_InitAndMigrate(t *testing.T) {
	db := newSeedMySQLTestDB(t) // 已把 database.DB 指向隔离库

	// 把 config 指向隔离库，随后 Init() 连接的就是它
	// newSeedMySQLTestDB 里创建的库名可从连接里读出
	var dbName string
	assert.NoError(t, db.Raw("SELECT DATABASE()").Scan(&dbName).Error)

	cfg := config.AppConfig
	origName := cfg.Database.Dbname
	cfg.Database.Dbname = dbName
	t.Cleanup(func() { cfg.Database.Dbname = origName })

	InitWithoutMigrate()
	assert.NotNil(t, GetDB())

	Init() // 含 autoMigrate
	assert.NotNil(t, GetDB())

	// autoMigrate 幂等：再跑一遍
	autoMigrate()
}

// EnsureUserRoleUniqueIndex 包装函数
func TestEnsureUserRoleUniqueIndexMySQL(t *testing.T) {
	newSeedMySQLTestDB(t) // 将 DB 指向隔离库
	// 当前模型 user_id 即主键，唯一索引创建仍应幂等成功
	assert.NoError(t, EnsureUserRoleUniqueIndex())
	assert.NoError(t, EnsureUserRoleUniqueIndex())
}

// ===================== seed 错误分支（隔离 MySQL） =====================

// 缺角色/缺父权限时各 ensure* 早退不 panic
func TestInitDataMySQL_MissingParentsAndRoles(t *testing.T) {
	db := newSeedMySQLTestDB(t)
	InitData()

	// 删除父权限与 teacher 角色，触发各 ensure* 的早退/跳过分支
	assert.NoError(t, db.Where("code IN ?", []string{"comp", "award", "declare", "review", "notice", "user"}).
		Delete(&models.Permission{}).Error)
	assert.NoError(t, db.Where("role_code = ?", "teacher").Delete(&models.Role{}).Error)

	InitData() // count>0 → ensure* 路径

	// 仍不应产生 teacher 角色
	var cnt int64
	db.Model(&models.Role{}).Where("role_code = ?", "teacher").Count(&cnt)
	assert.EqualValues(t, 0, cnt)
}

// grantRolePermissionsByCode 的失败分支：角色不存在 / 权限不存在
func TestGrantRolePermissionsByCode_Missing(t *testing.T) {
	newSeedMySQLTestDB(t)
	InitData()

	grantRolePermissionsByCode("not_exist_role", []string{"comp:list"}) // 角色缺失
	grantRolePermissionsByCode("student", []string{"no:such:perm"})    // 权限缺失
	grantRolePermissionsByCode("student", nil)                         // 空列表
}

var _ = fmt.Sprintf
var _ = sqlmock.QueryMatcherRegexp
