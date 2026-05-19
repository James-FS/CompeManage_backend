package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strconv"
	"time"
)

// GetNoticeList 处理“通知列表+筛选”接口
func GetNoticeList(c *gin.Context) {
	// 1. 获取前端传入的参数（分页+筛选）

	compIDStr := c.Query("compID")
	isLatestStr := c.DefaultQuery("is_latest", "true") // 默认按最新返回
	isLatest, _ := strconv.ParseBool(isLatestStr)
	statusStr := c.Query("status") // 新增：按状态筛选（0/1）

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
	if compIDStr != "" {
		compIDUint64, err := strconv.ParseUint(compIDStr, 10, 32)
		if err != nil {
			utils.BadRequest(c, "compID格式错误，必须是数字")
			return
		}
		compID := uint(compIDUint64)
		dbQuery = dbQuery.Where("competition_detail_id = ?", compID)
	}
	if statusStr != "" {
		status, err := strconv.Atoi(statusStr)
		if err != nil || (status != 0 && status != 1) {
			utils.BadRequest(c, "status只能是0（未发布）或1（已发布）")
			return
		}
		dbQuery = dbQuery.Where("status = ?", status)
	}
	if isLatest {
		dbQuery = dbQuery.Order("publish_time DESC") // 最新在前
	} else {
		dbQuery = dbQuery.Order("publish_time ASC") // 最旧在前
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

// CreateNotice 发布赛事通知（接收前端传入的附件URL，存入Attachment字段）
func CreateNotice(c *gin.Context) {
	// 1. 解析前端传入的通知参数（含附件URL）
	// 前端通过form-data或x-www-form-urlencoded传递参数，用Gin的PostForm获取
	title := c.PostForm("title")
	content := c.PostForm("content")
	compIDStr := c.PostForm("compID")         // 简化后的竞赛ID
	attachmentURL := c.PostForm("attachment") // 前端上传附件后拿到的URL

	// 2. 参数校验（必填项检查）
	if title == "" {
		utils.BadRequest(c, "通知标题不能为空")
		return
	}

	// compID可选，但传了就必须是数字
	var compID uint
	if compIDStr != "" {
		compIDUint64, err := strconv.ParseUint(compIDStr, 10, 32)
		if err != nil {
			utils.BadRequest(c, "compID格式错误，必须是数字")
			return
		}
		compID = uint(compIDUint64)
	}

	// 3. 构建Notice模型，存入附件URL
	notice := models.Notice{
		Title:               title,
		Content:             content,
		CompetitionDetailID: compID,
		Attachment:          attachmentURL, // 核心：将前端传入的附件URL存入字段
		Status:              0,             // 默认未发布
	}

	// 4. 保存到数据库
	if err := database.DB.Create(&notice).Error; err != nil {
		utils.InternalServerError(c, "发布通知失败", err)
		return
	}

	// 5. 返回发布结果（包含附件URL）
	utils.Success(c, gin.H{"notice": notice})
}

func CreateCompNotice(c *gin.Context) {
	// 1. 解析参数（compID强制必填）
	title := c.PostForm("title")
	content := c.PostForm("content")
	compIDStr := c.PostForm("compID") // 赛事页面必须传当前赛事ID
	attachmentURL := c.PostForm("attachment")

	// 2. 基础校验（title/publishTime/compID均必填）
	if title == "" {
		utils.BadRequest(c, "通知标题不能为空")
		return
	}
	if compIDStr == "" {
		utils.BadRequest(c, "必须关联具体赛事，请传入compID")
		return
	}

	// 3. compID格式校验（必传+必须是数字）
	compIDUint64, err := strconv.ParseUint(compIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "compID格式错误，必须是数字")
		return
	}
	compID := uint(compIDUint64)

	// 4. 保存数据库（强制关联赛事ID）
	notice := models.Notice{
		Title:               title,
		Content:             content,
		CompetitionDetailID: compID, // 必传，关联当前赛事
		Attachment:          attachmentURL,
		Status:              0, // 默认未发布
	}
	if err := database.DB.Create(&notice).Error; err != nil {
		fmt.Printf("数据库写入失败: %v\n", err)
		utils.InternalServerError(c, "发布赛事通知失败", err)
		return
	}
	utils.Success(c, gin.H{"notice": notice})
}

// PublishNotice 发布通知（修改status为1，UpdatedAt为发布时间）
func PublishNotice(c *gin.Context) {
	// 1. 获取通知ID
	noticeIDStr := c.Param("id")
	noticeIDUint, err := strconv.ParseUint(noticeIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "通知ID格式错误，必须是数字")
		return
	}
	noticeID := uint(noticeIDUint)

	// 2. 检查通知是否存在
	var notice models.Notice
	if err := database.DB.First(&notice, noticeID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "该通知不存在")
		} else {
			utils.InternalServerError(c, "查询通知失败", err)
		}
		return
	}

	// 3. 检查是否已发布
	if notice.Status == 1 {
		utils.BadRequest(c, "该通知已发布，无需重复操作")
		return
	}

	// 4. 更新状态为已发布，更新时间为当前时间
	currentPublishTime := time.Now().Format("2006-01-02 15:04:05")
	if err := database.DB.Model(&notice).Updates(map[string]interface{}{
		"status":       1,
		"publish_time": currentPublishTime, // 发布时固定publish_time
		"updated_at":   time.Now(),         // 发布时间=更新时间
	}).Error; err != nil {
		utils.InternalServerError(c, "发布通知失败", err)
		return
	}

	// 5. 返回发布结果
	utils.Success(c, gin.H{
		"notice": notice,
		"msg":    "通知发布成功",
	})
}

// UpdateNotice 修改通知
func UpdateNotice(c *gin.Context) {
	// 1. 获取通知ID
	noticeIDStr := c.Param("id")
	noticeIDUint, err := strconv.ParseUint(noticeIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "通知ID格式错误，必须是数字")
		return
	}
	noticeID := uint(noticeIDUint)

	// 2. 检查通知是否存在
	var notice models.Notice
	if err := database.DB.First(&notice, noticeID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "该通知不存在")
		} else {
			utils.InternalServerError(c, "查询通知失败", err)
		}
		return
	}

	// 3. 解析要修改的字段
	title := c.PostForm("title")
	content := c.PostForm("content")
	compIDStr := c.PostForm("compID")
	attachmentURL := c.PostForm("attachment")

	// 4. 构建更新字段
	updates := make(map[string]interface{})
	if title != "" {
		updates["title"] = title
	}
	if content != "" {
		updates["content"] = content
	}
	if compIDStr != "" {
		compIDUint64, err := strconv.ParseUint(compIDStr, 10, 32)
		if err != nil {
			utils.BadRequest(c, "compID格式错误，必须是数字")
			return
		}
		updates["competition_detail_id"] = uint(compIDUint64)
	}
	if attachmentURL != "" {
		updates["attachment"] = attachmentURL
	}

	// 5. 如果没有要更新的字段
	if len(updates) == 0 {
		utils.BadRequest(c, "没有要修改的字段")
		return
	}

	// 6. 执行更新
	if err := database.DB.Model(&notice).Updates(updates).Error; err != nil {
		utils.InternalServerError(c, "修改通知失败", err)
		return
	}

	// 7. 返回更新后的通知
	database.DB.First(&notice, noticeID)
	utils.Success(c, gin.H{"notice": notice})
}

// DeleteNotice 删除通知（物理删除/逻辑删除可选，这里用GORM软删除）
func DeleteNotice(c *gin.Context) {
	// 1. 获取通知ID
	noticeIDStr := c.Param("id")
	noticeIDUint, err := strconv.ParseUint(noticeIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "通知ID格式错误，必须是数字")
		return
	}
	noticeID := uint(noticeIDUint)

	// 2. 检查通知是否存在
	var notice models.Notice
	if err := database.DB.First(&notice, noticeID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "该通知不存在")
		} else {
			utils.InternalServerError(c, "查询通知失败", err)
		}
		return
	}

	// 3. 删除通知（GORM软删除，会自动填充DeletedAt字段）
	if err := database.DB.Delete(&notice).Error; err != nil {
		utils.InternalServerError(c, "删除通知失败", err)
		return
	}

	// 4. 返回删除结果
	utils.Success(c, gin.H{"msg": "通知删除成功"})
}
