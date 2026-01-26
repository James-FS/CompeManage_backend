package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strconv"
)

// GetNoticeList 处理“通知列表+筛选”接口（支持分页、时间筛选）
func GetNoticeList(c *gin.Context) {
	// 1. 获取前端传入的参数（分页+筛选）
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))           // 默认第1页
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10")) // 默认每页10条
	startTime := c.Query("start_time")                             // 筛选：发布开始时间（如“2026.1.1”）
	endTime := c.Query("end_time")                                 // 筛选：发布结束时间（如“2026.2.28”）

	// 2. 处理分页偏移量（补充参数合法性校验）
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 { // 限制每页最大条数，避免查询过多
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	// 3. 构建数据库查询（含筛选）
	dbQuery := database.DB.Model(&models.Notice{})
	// 时间筛选：只查指定时间段内的通知
	if startTime != "" {
		dbQuery = dbQuery.Where("publish_time >= ?", startTime)
	}
	if endTime != "" {
		dbQuery = dbQuery.Where("publish_time <= ?", endTime)
	}

	// 4. 查询列表+总数（用于前端分页，补充错误处理）
	var noticeList []models.Notice
	var total int64

	// 统计总数：捕获数据库错误，返回服务器错误
	if err := dbQuery.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "统计通知总数失败", err)
		return
	}

	// 分页查询列表：捕获数据库错误，返回服务器错误
	if err := dbQuery.Order("publish_time DESC").Offset(offset).Limit(pageSize).Find(&noticeList).Error; err != nil {
		utils.InternalServerError(c, "查询通知列表失败", err)
		return
	}

	// 5. 返回响应（用你项目已有的utils.Success统一格式）
	utils.Success(c, gin.H{
		"list":  noticeList, // 通知列表
		"total": total,      // 总条数
		"page":  page,       // 当前页码
	})
}

// GetNoticeDetail 处理“单个通知查看”接口
func GetNoticeDetail(c *gin.Context) {
	// 1. 获取URL中的通知ID
	noticeIDStr := c.Param("id")
	noticeIDUint, err := strconv.ParseUint(noticeIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "通知ID格式错误，必须是数字")
		return
	}
	noticeID := uint(noticeIDUint)

	// 2. 查询指定ID的通知
	var notice models.Notice
	err = database.DB.First(&notice, noticeID).Error
	if err != nil {
		// 区分错误类型：记录不存在 → NotFound；其他错误 → 服务器错误
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "该通知不存在")
		} else {
			utils.InternalServerError(c, "查询通知详情失败", err)
		}
		return
	}

	// 3. 返回通知详情
	utils.Success(c, gin.H{"notice": notice})
}
