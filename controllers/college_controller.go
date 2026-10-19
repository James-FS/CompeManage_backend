package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"

	"github.com/gin-gonic/gin"
)

// GetCollegeList 获取有效学院列表供下拉框使用
func GetCollegeList(c *gin.Context) {
	var list []models.College
	if err := database.DB.WithContext(c.Request.Context()).Where("is_valid = ?", true).Find(&list).Error; err != nil {
		utils.InternalServerError(c, "获取学院数据失败", err)
		return
	}

	utils.Success(c, list)
}

// GetDepartmentList 获取有效部门列表供下拉框使用
func GetDepartmentList(c *gin.Context) {
	var list []models.Department
	if err := database.DB.WithContext(c.Request.Context()).Where("is_valid = ?", true).Find(&list).Error; err != nil {
		utils.InternalServerError(c, "获取部门数据失败", err)
		return
	}
	utils.Success(c, list)
}
