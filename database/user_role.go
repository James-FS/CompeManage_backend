package database

import (
	"CompeManage_backend/models"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var (
	ErrUserHasNoRole        = errors.New("用户未分配角色")
	ErrUserHasMultipleRoles = errors.New("用户存在多个角色")
)

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
