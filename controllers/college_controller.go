package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"

	"github.com/gin-gonic/gin"
)

// GetCollegeList 获取所有学院列表供下拉框使用
func GetCollegeList(c *gin.Context) {
	var list []models.College
	// 查询所有学院
	if err := database.DB.Find(&list).Error; err != nil {
		utils.InternalServerError(c, "获取学院数据失败", err)
		return
	}

	utils.Success(c, list)
}
