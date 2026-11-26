package database

import (
	"CompeManage_backend/models"
	"errors"
	"log"

	"gorm.io/gorm"
)

// Only new permission rows receive default grants, preserving later manual revocations.
func ensureCollegeManagerPermissions() {
	err := DB.Transaction(func(tx *gorm.DB) error {
		var role models.Role
		if err := tx.Where("role_code = ?", "college_admin").First(&role).Error; err != nil {
			return err
		}
		changed := false
		for _, item := range []struct{ Code, Name, Parent, Description string }{
			{"comp:update:own", "编辑本人负责的赛事", "comp", "限本人负责且处于授权范围内的赛事"},
			{"reg:config:edit:own", "配置本人负责的赛事报名", "reg:config", "限本人负责且处于授权范围内的赛事"},
			{"summary:edit:own", "填写本人负责的赛事总结", "summary", "限本人负责且处于授权范围内的赛事"},
			{"comp:update:source", "编辑本院来源的赛事", "comp", "限项目来源为管理学院的赛事"},
			{"reg:config:edit:source", "配置本院来源赛事的报名", "reg:config", "限项目来源为管理学院的赛事"},
			{"summary:edit:source", "填写本院来源赛事的总结", "summary", "限项目来源为管理学院的赛事"},
			{"declare:audit:source", "审核本院来源的赛事申报", "declare", "限项目来源为管理学院的赛事申报"},
		} {
			var permission models.Permission
			err := tx.Where("code = ?", item.Code).First(&permission).Error
			if err == nil {
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			var parent models.Permission
			if err := tx.Where("code = ?", item.Parent).First(&parent).Error; err != nil {
				return err
			}
			permission = models.Permission{Name: item.Name, Code: item.Code, Type: 3, ParentID: parent.ID, Description: item.Description}
			if err := tx.Create(&permission).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
				return err
			}
			changed = true
		}
		if changed {
			return tx.Model(&role).UpdateColumn("permission_version", gorm.Expr("permission_version + 1")).Error
		}
		return nil
	})
	if err != nil {
		log.Printf("补齐院管理员本人赛事权限失败: %v", err)
	}
}
