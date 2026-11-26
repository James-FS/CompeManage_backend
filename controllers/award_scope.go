package controllers

import (
	"CompeManage_backend/models"
	"errors"

	"gorm.io/gorm"
)

func collegeNameForAwardScope(db *gorm.DB, scope *UserAccessScope) (string, error) {
	if scope == nil || !scope.IsCollegeAdmin() {
		return "", nil
	}
	if scope.ManagedCollegeID == nil {
		return "", errors.New("院级管理员尚未配置管理学院")
	}
	var college models.College
	if err := db.Select("id", "name").First(&college, *scope.ManagedCollegeID).Error; err != nil {
		return "", err
	}
	return college.Name, nil
}
