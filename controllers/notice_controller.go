package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strconv"
)

// clearNoticeListCache 删除通知列表相关的所有缓存
func clearNoticeListCache(ctx context.Context) {
	rdb := middleware.GetRedisClient()
	iter := rdb.Scan(ctx, 0, "cache:notice_list:*", 0).Iterator()
	for iter.Next(ctx) {
		rdb.Del(ctx, iter.Val())
	}
}

// GetNoticeList 处理"通知列表+筛选"接口
func GetNoticeList(c *gin.Context) {
	compIDStr := c.Query("compID")
	isLatestStr := c.DefaultQuery("is_latest", "true")
	isLatest, _ := strconv.ParseBool(isLatestStr)
	statusStr := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}

	// 构造缓存 key（包含所有筛选参数）
	cacheKeyRaw := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d",
		compIDStr, isLatestStr, statusStr, startTime, endTime, page, pageSize)
	cacheKey := fmt.Sprintf("cache:notice_list:%x", md5.Sum([]byte(cacheKeyRaw)))

	rdb := middleware.GetRedisClient()
	ctx := c.Request.Context()

	// 先查 Redis 缓存
	cacheData, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil && cacheData != "" {
		var resp gin.H
		if json.Unmarshal([]byte(cacheData), &resp) == nil {
			utils.Success(c, resp)
			return
		}
	}

	// 构建数据库查询
	dbQuery := database.DB.WithContext(ctx).Model(&models.Notice{})
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
		dbQuery = dbQuery.Order("publish_time DESC")
	} else {
		dbQuery = dbQuery.Order("publish_time ASC")
	}

	offset := (page - 1) * pageSize
	var noticeList []models.Notice
	var total int64

	if err := dbQuery.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "统计通知总数失败", err)
		return
	}

	if err := dbQuery.Order("publish_time DESC").Offset(offset).Limit(pageSize).Find(&noticeList).Error; err != nil {
		utils.InternalServerError(c, "查询通知列表失败", err)
		return
	}

	// 异步写入 Redis 缓存
	respData := gin.H{"list": noticeList, "total": total, "page": page}
	go func() {
		asyncCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		data, err := json.Marshal(respData)
		if err != nil {
			return
		}
		rdb.Set(asyncCtx, cacheKey, data, 5*time.Minute)
	}()

	utils.Success(c, respData)
}

// GetNoticeDetail 处理"单个通知查看"接口
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
	err = database.DB.WithContext(c.Request.Context()).First(&notice, noticeID).Error
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
	if err := database.DB.WithContext(c.Request.Context()).Create(&notice).Error; err != nil {
		utils.InternalServerError(c, "发布通知失败", err)
		return
	}

	// 5. 返回发布结果（包含附件URL）
	clearNoticeListCache(c.Request.Context())
	clearNoticeListCache(c.Request.Context())
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
	// P0-3：写入归属人（nil 安全——测试上下文无 user_id 时保持 NULL，生产请求必经 AuthRequired）。
	var publisherID *uint
	if v, ok := c.Get("user_id"); ok {
		if uid, ok2 := v.(uint); ok2 && uid != 0 {
			publisherID = &uid
		}
	}
	notice := models.Notice{
		Title:               title,
		Content:             content,
		CompetitionDetailID: compID, // 必传，关联当前赛事
		Attachment:          attachmentURL,
		Status:              0, // 默认未发布
		PublisherID:         publisherID,
	}
	if err := database.DB.WithContext(c.Request.Context()).Create(&notice).Error; err != nil {
		fmt.Printf("数据库写入失败: %v\n", err)
		utils.InternalServerError(c, "发布赛事通知失败", err)
		return
	}
	clearNoticeListCache(c.Request.Context())
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
	if err := database.DB.WithContext(c.Request.Context()).First(&notice, noticeID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "该通知不存在")
		} else {
			utils.InternalServerError(c, "查询通知失败", err)
		}
		return
	}

	// 3. 归属校验（P0-3）：管理员不受限；其他角色仅限自己发布的通知。
	if !canManageNotice(c, notice) {
		utils.Forbidden(c, "仅可操作自己发布的通知")
		return
	}

	// 4. 检查是否已发布
	if notice.Status == 1 {
		utils.BadRequest(c, "该通知已发布，无需重复操作")
		return
	}

	// 5. 更新状态为已发布，更新时间为当前时间
	currentPublishTime := time.Now().Format("2006-01-02 15:04:05")
	updates := map[string]interface{}{
		"status":       1,
		"publish_time": currentPublishTime, // 发布时固定publish_time
		"updated_at":   time.Now(),         // 发布时间=更新时间
	}
	// P0-3：发布时刷新归属人（nil 安全——上下文无 user_id 时保持原值，不覆盖）。
	if uid := currentOperatorID(c); uid != 0 {
		updates["publisher_id"] = uid
	}
	if err := database.DB.WithContext(c.Request.Context()).Model(&notice).Updates(updates).Error; err != nil {
		utils.InternalServerError(c, "发布通知失败", err)
		return
	}

	// 5. 返回发布结果
	clearNoticeListCache(c.Request.Context())
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
	if err := database.DB.WithContext(c.Request.Context()).First(&notice, noticeID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "该通知不存在")
		} else {
			utils.InternalServerError(c, "查询通知失败", err)
		}
		return
	}

	// 2.5 归属校验（P0-3）：管理员不受限；其他角色仅限自己发布的通知。
	// A-1 已把 notice:update 授予 competition_manager，此校验不可省略，
	// 否则负责人可编辑任意通知、绕过「只能管自己发布的」决策。
	if !canManageNotice(c, notice) {
		utils.Forbidden(c, "仅可操作自己发布的通知")
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
	if err := database.DB.WithContext(c.Request.Context()).Model(&notice).Updates(updates).Error; err != nil {
		utils.InternalServerError(c, "修改通知失败", err)
		return
	}

	// 7. 返回更新后的通知
	database.DB.WithContext(c.Request.Context()).First(&notice, noticeID)
	clearNoticeListCache(c.Request.Context())
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
	if err := database.DB.WithContext(c.Request.Context()).First(&notice, noticeID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "该通知不存在")
		} else {
			utils.InternalServerError(c, "查询通知失败", err)
		}
		return
	}

	// 2.5 归属校验（P0-3）：管理员不受限；其他角色仅限自己发布的通知。
	if !canManageNotice(c, notice) {
		utils.Forbidden(c, "仅可操作自己发布的通知")
		return
	}

	// 3. 删除通知（GORM软删除，会自动填充DeletedAt字段）
	if err := database.DB.WithContext(c.Request.Context()).Delete(&notice).Error; err != nil {
		utils.InternalServerError(c, "删除通知失败", err)
		return
	}

	// 4. 返回删除结果
	clearNoticeListCache(c.Request.Context())
	utils.Success(c, gin.H{"msg": "通知删除成功"})
}
