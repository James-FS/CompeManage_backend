package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SummaryListReq struct {
	Page          int    `form:"page"`
	PageSize      int    `form:"page_size"`
	CompName      string `form:"comp_name"`
	Organizer     string `form:"organizer"`
	Manager       string `form:"manager"`
	SummaryStatus string `form:"summary_status"`
	EndTime       string `form:"end_time"`
	Year          string `form:"year"`
}

type SummaryListItem struct {
	ID            uint       `json:"id"`
	CompName      string     `json:"comp_name"`
	Organizer     string     `json:"organizer"`
	Undertaker    string     `json:"undertaker"`
	CollegeName   string     `json:"college_name"`
	ManagerName   string     `json:"manager"`
	EndTime       *time.Time `json:"end_time"`
	SummaryStatus int8       `json:"summary_status"`
}

// GetSummaryList 获取赛事总结列表（只看已结束赛事）
func GetSummaryList(c *gin.Context) {
	var req SummaryListReq
	_ = c.ShouldBindQuery(&req)

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 50 {
		req.PageSize = 10
	}
	offset := (req.Page - 1) * req.PageSize

	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	userID := userIDVal.(uint)

	query := database.DB.Table("comp_directories").
		Joins("LEFT JOIN comp_details ON comp_details.comp_id = comp_directories.id").
		Joins("LEFT JOIN users ON users.id = comp_directories.manager_id").
		Joins("LEFT JOIN colleges ON colleges.id = comp_directories.college_id").
		Joins("LEFT JOIN summaries ON summaries.comp_id = comp_directories.id").
		Where("comp_directories.status = ?", 2)

	// 非管理员：只看自己负责的赛事
	if !checkUserIsAdmin(userID) {
		query = query.Where("comp_directories.manager_id = ?", userID)
	}

	if req.CompName != "" {
		query = query.Where("comp_directories.comp_name LIKE ?", "%"+req.CompName+"%")
	}
	if req.Organizer != "" {
		query = query.Where("comp_directories.organizer LIKE ?", "%"+req.Organizer+"%")
	}
	if req.Manager != "" {
		query = query.Where("(users.realname LIKE ? OR users.username LIKE ?)", "%"+req.Manager+"%", "%"+req.Manager+"%")
	}
	if req.SummaryStatus != "" {
		switch req.SummaryStatus {
		case "0":
			query = query.Where("summaries.id IS NULL OR summaries.status = 0")
		case "1":
			query = query.Where("summaries.status = 1")
		default:
			utils.BadRequest(c, "summary_status 只能是 0 或 1")
			return
		}
	}
	if req.EndTime != "" {
		endDate, err := time.ParseInLocation("2006-01-02", req.EndTime, time.Local)
		if err != nil {
			utils.BadRequest(c, "end_time 格式错误，应为 YYYY-MM-DD")
			return
		}
		endDate = endDate.Add(24*time.Hour - time.Nanosecond)
		query = query.Where("comp_details.comp_end_time <= ?", endDate)
	}
	if req.Year != "" {
		yearInt, err := strconv.Atoi(req.Year)
		if err != nil {
			utils.BadRequest(c, "year 只能是数字")
			return
		}
		query = query.Where("(comp_directories.year = ? OR YEAR(comp_details.comp_end_time) = ?)", yearInt, yearInt)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "统计失败", err)
		return
	}

	var list []SummaryListItem
	if err := query.Select(`
		comp_directories.id as id,
		comp_directories.comp_name,
		comp_directories.organizer,
		comp_directories.undertaker,
		colleges.name as college_name,
		users.realname as manager_name,
		comp_details.comp_end_time as end_time,
		COALESCE(summaries.status, 0) as summary_status
	`).
		Order("comp_details.comp_end_time desc, comp_directories.id desc").
		Offset(offset).Limit(req.PageSize).
		Scan(&list).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		endTimeStr := ""
		if item.EndTime != nil && !item.EndTime.IsZero() {
			endTimeStr = item.EndTime.Format("2006-01-02")
		}
		respList = append(respList, gin.H{
			"id":             item.ID,
			"comp_name":      item.CompName,
			"organizer":      item.Organizer,
			"undertaker":     item.Undertaker,
			"college_info":   gin.H{"name": item.CollegeName},
			"manager":        item.ManagerName,
			"end_time":       endTimeStr,
			"summary_status": item.SummaryStatus,
		})
	}

	utils.Success(c, gin.H{
		"list":  respList,
		"total": total,
		"page":  req.Page,
		"size":  req.PageSize,
	})
}

type ExpenseItem struct {
	Usage  string  `json:"usage"`
	Amount float64 `json:"amount"`
	Remark string  `json:"remark"`
}

type AttachmentItem struct {
	Name string `json:"name"`
	Url  string `json:"url"`
}

type AwardStatItem struct {
	Level     string `json:"level"`
	Count     int64  `json:"count"`
	LevelRank int    `json:"-"`
}

type SaveSummaryReq struct {
	SummaryContent string           `json:"summary_content"`
	Expenses       []ExpenseItem    `json:"expenses"`
	Attachments    []AttachmentItem `json:"attachments"`
	Status         int8             `json:"status"` // 0:草稿 1:归档
}

// GetSummaryDetail 获取赛事总结详情（含统计数据）
func GetSummaryDetail(c *gin.Context) {
	compIDStr := c.Param("id")
	compIDUint64, err := strconv.ParseUint(compIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "赛事ID格式错误")
		return
	}
	compID := uint(compIDUint64)

	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	userID := userIDVal.(uint)

	// 1. 查询赛事基本信息
	var comp models.CompDirectory
	if err := database.DB.Preload("Detail").Preload("Manager").Preload("CollegeInfo").First(&comp, compID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "赛事不存在")
			return
		}
		utils.InternalServerError(c, "查询赛事失败", err)
		return
	}

	// 权限检查
	if !checkUserIsAdmin(userID) && comp.ManagerID != userID {
		utils.Forbidden(c, "无权查看该赛事总结")
		return
	}

	// 2. 查询总结记录
	var summary models.Summary
	summaryFound := true
	if err := database.DB.Where("comp_id = ?", compID).First(&summary).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			summaryFound = false
		} else {
			utils.InternalServerError(c, "查询总结失败", err)
			return
		}
	}

	// 3. 处理经费和附件
	expenses := []ExpenseItem{}
	attachments := []AttachmentItem{}
	expenseTotal := 0.0
	if summaryFound {
		if summary.Expenses != "" {
			_ = json.Unmarshal([]byte(summary.Expenses), &expenses)
			// 遍历计算总金额
			for _, item := range expenses {
				expenseTotal += item.Amount
			}
		}
		if summary.Attachments != "" {
			_ = json.Unmarshal([]byte(summary.Attachments), &attachments)
		}
	}

	// 4. 统计参赛人数 (status=1 表示审核通过)
	participantCount := int64(0)
	_ = database.DB.Table("reg_members").
		Joins("JOIN registers ON registers.id = reg_members.reg_id").
		Where("registers.comp_id = ? AND registers.status = ?", compID, 1).
		Count(&participantCount).Error

	// 5. 统计获奖情况
	var awardStats []AwardStatItem
	awardTotal := int64(0) //
	_ = database.DB.Model(&models.Award{}).
		Select("award_level as level, count(*) as count, MIN(level_rank) as level_rank").
		Where("comp_id = ?", compID).
		Group("award_level").
		Order("level_rank asc").
		Scan(&awardStats).Error

	// 计算获奖总数
	for _, stat := range awardStats {
		awardTotal += stat.Count
	}

	// 6. 格式化时间范围和名称
	timeRange := ""
	startTime := comp.Detail.CompStartTime
	endTime := comp.Detail.CompEndTime
	startStr := ""
	endStr := ""
	if !startTime.IsZero() {
		startStr = startTime.Format("2006-01-02")
	}
	if !endTime.IsZero() {
		endStr = endTime.Format("2006-01-02")
	}
	if startStr != "" && endStr != "" {
		timeRange = startStr + " 至 " + endStr
	} else if endStr != "" {
		timeRange = endStr
	} else if startStr != "" {
		timeRange = startStr
	}

	managerName := ""
	if comp.Manager.ID != 0 {
		managerName = comp.Manager.Realname
	}
	collegeName := ""
	if comp.CollegeInfo != nil {
		collegeName = comp.CollegeInfo.Name
	}

	status := int8(0)
	summaryContent := ""
	if summaryFound {
		status = summary.Status
		summaryContent = summary.SummaryContent
	}

	utils.Success(c, gin.H{
		"comp_id":           comp.ID,
		"comp_name":         comp.CompName,
		"organizer":         comp.Organizer,
		"undertaker":        comp.Undertaker,
		"college_info":      gin.H{"name": collegeName},
		"manager":           managerName,
		"time_range":        timeRange,
		"participant_count": participantCount,
		"award_total":       awardTotal,
		"award_stats":       awardStats,
		"expense_total":     expenseTotal,
		"expenses":          expenses,
		"summary_content":   summaryContent,
		"attachments":       attachments,
		"file_count":        len(attachments),
		"summary_status":    status,
	})
}

// SaveSummary 保存/归档赛事总结
func SaveSummary(c *gin.Context) {
	compIDStr := c.Param("id")
	compIDUint64, err := strconv.ParseUint(compIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "赛事ID格式错误")
		return
	}
	compID := uint(compIDUint64)

	var req SaveSummaryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	if req.Status != 0 && req.Status != 1 {
		utils.BadRequest(c, "status 只能是 0 或 1")
		return
	}

	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	userID := userIDVal.(uint)

	var comp models.CompDirectory
	if err := database.DB.First(&comp, compID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "赛事不存在")
			return
		}
		utils.InternalServerError(c, "查询赛事失败", err)
		return
	}

	if !checkUserIsAdmin(userID) && comp.ManagerID != userID {
		utils.Forbidden(c, "无权操作该赛事总结")
		return
	}
	if comp.Status != 2 {
		utils.BadRequest(c, "赛事未结束，无法提交总结")
		return
	}

	expensesJSON, err := json.Marshal(req.Expenses)
	if err != nil {
		utils.InternalServerError(c, "经费数据序列化失败", err)
		return
	}
	attachmentsJSON, err := json.Marshal(req.Attachments)
	if err != nil {
		utils.InternalServerError(c, "附件数据序列化失败", err)
		return
	}

	var summary models.Summary
	err = database.DB.Where("comp_id = ?", compID).First(&summary).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		utils.InternalServerError(c, "查询总结失败", err)
		return
	}

	if err == nil && summary.Status == 1 {
		utils.Forbidden(c, "总结已归档，无法修改")
		return
	}

	if err == gorm.ErrRecordNotFound {
		summary = models.Summary{
			CompID:         compID,
			SummaryContent: req.SummaryContent,
			Expenses:       string(expensesJSON),
			Attachments:    string(attachmentsJSON),
			Status:         req.Status,
			CreatedBy:      userID,
			UpdatedBy:      userID,
		}
		if req.Status == 1 {
			now := time.Now()
			summary.ArchivedAt = &now
		}
		if err := database.DB.Create(&summary).Error; err != nil {
			utils.InternalServerError(c, "保存总结失败", err)
			return
		}
	} else {
		summary.SummaryContent = req.SummaryContent
		summary.Expenses = string(expensesJSON)
		summary.Attachments = string(attachmentsJSON)
		summary.Status = req.Status
		summary.UpdatedBy = userID
		if req.Status == 1 {
			now := time.Now()
			summary.ArchivedAt = &now
		}
		if err := database.DB.Save(&summary).Error; err != nil {
			utils.InternalServerError(c, "更新总结失败", err)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "保存成功",
		"data": summary,
	})
}
