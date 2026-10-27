package database

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"CompeManage_backend/config"
	"CompeManage_backend/models"

	gosql "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// 隔离 MySQL 集成套件（与 controllers 的 COMPE_AWARD_MYSQL_TEST 约定一致）：
// COMPE_AWARD_MYSQL_TEST=1 go test ./database/ -run MySQL -v
func newSeedMySQLTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("COMPE_AWARD_MYSQL_TEST") != "1" {
		t.Skip("set COMPE_AWARD_MYSQL_TEST=1 for isolated MySQL integration")
	}
	originalDB, originalConfig := DB, *config.AppConfig
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := filepath.Dir(wd)
	require.NoError(t, os.Chdir(root))
	_ = godotenv.Load()
	config.Init()
	require.NoError(t, os.Chdir(wd))
	t.Cleanup(func() { DB = originalDB; *config.AppConfig = originalConfig })

	cfg := config.AppConfig.Database
	dsn := gosql.NewConfig()
	dsn.User = cfg.User
	dsn.Passwd = cfg.Password
	dsn.Net = "tcp"
	dsn.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if override := os.Getenv("COMPE_AWARD_MYSQL_DSN"); override != "" {
		dsn, err = gosql.ParseDSN(override)
		require.NoError(t, err)
	}
	dsn.ParseTime = true
	dsn.Loc = time.Local
	admin, err := gorm.Open(mysql.Open(dsn.FormatDSN()), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	name := fmt.Sprintf("compe_seed_test_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error)
	t.Cleanup(func() {
		require.NoError(t, admin.Exec("DROP DATABASE `"+name+"`").Error)
		sqlDB, _ := admin.DB()
		_ = sqlDB.Close()
	})
	dsn.DBName = name
	db, err := gorm.Open(mysql.Open(dsn.FormatDSN()), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	DB = db
	require.NoError(t, db.Exec("SET FOREIGN_KEY_CHECKS=0").Error)
	require.NoError(t, db.AutoMigrate(
		&models.User{}, &models.Role{}, &models.Permission{}, &models.UserRole{}, &models.College{},
		&models.CompDirectory{}, &models.CompDetail{}, // 迁移决策需要查"是否管理赛事"
	))
	require.NoError(t, db.Exec("SET FOREIGN_KEY_CHECKS=1").Error)
	return db
}

// InitData：空库全量种子 + 二次幂等
func TestInitDataMySQL_FreshSeedThenIdempotent(t *testing.T) {
	db := newSeedMySQLTestDB(t)

	InitData() // 空库 → 全量种子

	var roleCount int64
	db.Model(&models.Role{}).Count(&roleCount)
	require.EqualValues(t, 7, roleCount)

	var permCount int64
	db.Model(&models.Permission{}).Count(&permCount)
	require.Greater(t, permCount, int64(20))

	var userCount int64
	db.Model(&models.User{}).Count(&userCount)
	require.EqualValues(t, 3, userCount)

	var linkCount int64
	db.Table("user_roles").Count(&linkCount)
	require.EqualValues(t, 3, linkCount)

	// 第二次：已有数据 → ensure* 幂等路径
	InitData()

	var userCount2 int64
	db.Model(&models.User{}).Count(&userCount2)
	require.EqualValues(t, 3, userCount2, "重复 InitData 不应重复建用户")

	var permCount2 int64
	db.Model(&models.Permission{}).Count(&permCount2)
	require.GreaterOrEqual(t, permCount2, permCount, "ensure* 最多补齐缺失权限，不应减少")

	// 角色授权仍然存在
	var schoolAdmin models.Role
	require.NoError(t, db.Where("role_code = ?", "school_admin").First(&schoolAdmin).Error)
	var granted int64
	db.Table("role_permissions").Where("role_id = ?", schoolAdmin.ID).Count(&granted)
	require.Greater(t, granted, int64(5))
}

// 用户角色迁移：异常数据 → 报告 → 迁移 → 校验
func TestUserRoleMigrationMySQL_FullCycle(t *testing.T) {
	db := newSeedMySQLTestDB(t)
	InitData()

	var studentRole, teacherRole, collegeAdminRole models.Role
	require.NoError(t, db.Where("role_code = ?", "student").First(&studentRole).Error)
	require.NoError(t, db.Where("role_code = ?", "teacher").First(&teacherRole).Error)
	require.NoError(t, db.Where("role_code = ?", "college_admin").First(&collegeAdminRole).Error)

	// 改造为 legacy 形态：id 自增主键、user_id 无唯一约束（多角色脏数据的前提）
	// 新模型把 user_id 建成自增主键；legacy 形态是独立自增 id + user_id 无约束
	require.NoError(t, db.Exec("ALTER TABLE user_roles MODIFY user_id BIGINT UNSIGNED NOT NULL").Error)
	require.NoError(t, db.Exec("ALTER TABLE user_roles DROP PRIMARY KEY").Error)
	require.NoError(t, db.Exec("ALTER TABLE user_roles ADD COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY FIRST").Error)

	// 异常 1：多角色用户
	multi := models.User{Username: "800001", Realname: "多角色", IdentityType: "staff", College: "计算机科学与网络工程学院"}
	require.NoError(t, db.Create(&multi).Error)
	require.NoError(t, db.Create(&models.UserRole{UserID: multi.ID, RoleID: studentRole.ID}).Error)
	require.NoError(t, db.Create(&models.UserRole{UserID: multi.ID, RoleID: teacherRole.ID}).Error)

	// 异常 2：无角色用户（identity 已填，保持分类独立）
	noRole := models.User{Username: "800002", Realname: "无角色", IdentityType: "student", College: "计算机科学与网络工程学院"}
	require.NoError(t, db.Create(&noRole).Error)

	// 异常 3：缺 identity（测试账号被删掉 identity 即可）
	require.NoError(t, db.Model(&models.User{}).Where("username = ?", "900007").
		Update("identity_type", "").Error)

	// 异常 4：college_admin 无管理学院
	admin := models.User{Username: "800003", Realname: "院管", IdentityType: "staff", College: "计算机科学与网络工程学院"}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&models.UserRole{UserID: admin.ID, RoleID: collegeAdminRole.ID}).Error)

	report, err := BuildUserRoleMigrationReport()
	require.NoError(t, err)
	require.Equal(t, int64(6), report.TotalUsers)
	require.Len(t, report.MultiRoleUsers, 1)
	require.NotEmpty(t, report.MultiRoleUsers[0].SuggestedRole)
	require.Len(t, report.NoRoleUsers, 1)
	require.NotEmpty(t, report.MissingIdentityUsers)
	require.Len(t, report.CollegeAdminsWithoutScope, 1)
	require.False(t, report.UniqueConstraintAlreadyExist)

	after, err := ApplyUserRoleMigration("backup_migration_test")
	require.NoError(t, err, "迁移后一致性检查应全部通过")
	require.Empty(t, after.MultiRoleUsers)
	require.Empty(t, after.NoRoleUsers)
	require.Empty(t, after.MissingIdentityUsers)
	require.Empty(t, after.CollegeAdminsWithoutScope)
	require.Equal(t, "backup_migration_test", after.BackupTable)

	verified, err := VerifyUserRoleMigration()
	require.NoError(t, err)
	require.Empty(t, verified.MultiRoleUsers)

	// 多角色收敛为高优先级角色（staff → teacher）
	var links []models.UserRole
	require.NoError(t, db.Where("user_id = ?", multi.ID).Find(&links).Error)
	require.Len(t, links, 1)
	require.Equal(t, teacherRole.ID, links[0].RoleID)

	// 无角色用户补默认角色（student → student 角色）
	var noRoleLinks []models.UserRole
	require.NoError(t, db.Where("user_id = ?", noRole.ID).Find(&noRoleLinks).Error)
	require.Len(t, noRoleLinks, 1)

	// college_admin 补管理学院
	var fixedAdmin models.User
	require.NoError(t, db.First(&fixedAdmin, admin.ID).Error)
	require.NotNil(t, fixedAdmin.ManagedCollegeID)

	// 备份表存在
	var cnt int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'backup_migration_test'").Scan(&cnt).Error)
	require.EqualValues(t, 1, cnt)
}
