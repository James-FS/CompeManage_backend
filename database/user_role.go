package database

import (
	"CompeManage_backend/models"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

var (
	ErrUserHasNoRole        = errors.New("用户未分配角色")
	ErrUserHasMultipleRoles = errors.New("用户存在多个角色")
)

// TransactionWithDeadlockRetry 执行事务并在 MySQL 死锁（Error 1213）时整体重试。
// 背景：user_roles.user_id 唯一索引建立后，数据大厅多个同步任务并发写角色行
// 会因唯一索引的间隙锁产生死锁；InnoDB 对死锁回滚整个事务，因此必须在
// 事务边界整体重试，而非在事务内重试单条语句。
func TransactionWithDeadlockRetry(db *gorm.DB, fn func(tx *gorm.DB) error) error {
	if db == nil {
		return errors.New("数据库连接为空")
	}
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = db.Transaction(fn)
		if lastErr == nil {
			return nil
		}
		var mysqlErr *mysql.MySQLError
		if errors.As(lastErr, &mysqlErr) && mysqlErr.Number == 1213 && attempt < maxAttempts {
			slog.Warn("事务遇到死锁，准备重试", "attempt", attempt, "error", lastErr)
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			continue
		}
		return lastErr
	}
	return lastErr
}

// SetUserRole 在调用方事务内显式替换用户角色。
// 不使用 GORM Association.Replace，避免 user_id 唯一约束下依赖内部执行顺序。
func SetUserRole(tx *gorm.DB, userID, roleID uint) error {
	if tx == nil {
		return errors.New("数据库连接为空")
	}
	if userID == 0 || roleID == 0 {
		return errors.New("用户ID和角色ID必须大于0")
	}

	if err := tx.Where("user_id = ?", userID).Delete(&models.UserRole{}).Error; err != nil {
		return fmt.Errorf("删除旧用户角色失败: %w", err)
	}
	if err := tx.Create(&models.UserRole{UserID: userID, RoleID: roleID}).Error; err != nil {
		return fmt.Errorf("写入新用户角色失败: %w", err)
	}
	return nil
}

// GetSingleUserRole 查询用户的唯一角色。迁移完成前若存在异常数据，会返回明确错误。
func GetSingleUserRole(tx *gorm.DB, userID uint) (*models.Role, error) {
	var roles []models.Role
	if err := tx.Table("roles").
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ?", userID).
		Where("roles.delete_time IS NULL").
		Order("roles.id ASC").
		Find(&roles).Error; err != nil {
		return nil, err
	}

	switch len(roles) {
	case 0:
		return nil, ErrUserHasNoRole
	case 1:
		return &roles[0], nil
	default:
		return nil, ErrUserHasMultipleRoles
	}
}

// EnsureUserRoleUniqueIndex 供启动时调用（P0-4），幂等创建 user_roles.user_id 唯一索引。
// 该索引是数据库层唯一的单角色兜底：任何绕过 SetUserRole 的写入（直接 SQL、
// GORM Append、导入脚本）都会被拒绝。必须 在 AutoMigrate 创建 user_roles 表之后调用。
// 若库中已存在多角色脏数据，此处会因唯一冲突失败——属预期，需先用迁移工具清理。
func EnsureUserRoleUniqueIndex() error {
	return ensureUserRoleUniqueIndex()
}
