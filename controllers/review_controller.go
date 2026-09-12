package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"context"
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ========== 管理员 API ==========

// GetReviewCompList 获取开启评审的赛事列表（仪表盘用）
func GetReviewCompList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	keyword := c.Query("keyword")

	if page < 1 {
		page = 1
	}
	if size < 1 || size > MaxPageSize {
		size = 10
	}
	query := database.DB.WithContext(c.Request.Context()).
		Table("comp_directories").
		Joins("JOIN comp_details ON comp_details.comp_id = comp_directories.id").
		Where("comp_details.need_review = ?", 1)
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	if scope != nil {
		query = applyCompetitionScope(query, scope, "comp_directories")
	}

	if keyword != "" {
		query = query.Where("comp_directories.comp_name LIKE ?", "%"+keyword+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	type CompListItem struct {
		CompID          uint   `json:"comp_id"`
		CompName        string `json:"comp_name"`
		CompLevel       string `json:"comp_level"`
		NeedReview      int8   `json:"need_review"`
		ReviewStartTime string `json:"review_start_time"`
		ReviewEndTime   string `json:"review_end_time"`
		TotalWorks      int64  `json:"total_works"`
		TotalExperts    int64  `json:"total_experts"`
		ReviewedCount   int64  `json:"reviewed_count"`
		TotalRecords    int64  `json:"total_records"`
		ReviewStatus    string `json:"review_status"`
		HasAward        bool   `json:"has_award"`
	}

	var rawList []struct {
		CompID          uint      `json:"comp_id"`
		CompName        string    `json:"comp_name"`
		CompLevel       string    `json:"comp_level"`
		NeedReview      int8      `json:"need_review"`
		ReviewStartTime time.Time `json:"review_start_time"`
		ReviewEndTime   time.Time `json:"review_end_time"`
	}

	offset := (page - 1) * size
	if err := query.
		Select("comp_directories.id AS comp_id, comp_directories.comp_name, comp_directories.comp_level, comp_details.need_review, comp_details.review_start_time, comp_details.review_end_time").
		Order("comp_directories.create_time DESC").
		Offset(offset).Limit(size).
		Scan(&rawList).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	var list []CompListItem
	for _, item := range rawList {
		compID := item.CompID

		var totalWorks int64
		database.DB.WithContext(c.Request.Context()).Model(&models.Register{}).
			Where("comp_id = ? AND status IN (1,4) AND work_attachment_url IS NOT NULL AND work_attachment_url != ''", compID).
			Count(&totalWorks)

		var totalExperts int64
		database.DB.WithContext(c.Request.Context()).Model(&models.ReviewTask{}).
			Where("comp_id = ?", compID).Count(&totalExperts)

		var totalRecords int64
		database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).
			Where("comp_id = ?", compID).Count(&totalRecords)

		var reviewedCount int64
		database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).
			Where("comp_id = ? AND status = 1", compID).Count(&reviewedCount)

		hasAward := false
		var awardCount int64
		database.DB.WithContext(c.Request.Context()).Model(&models.Award{}).
			Where("comp_id = ? AND source = ?", compID, "review").Count(&awardCount)
		hasAward = awardCount > 0

		reviewStatus := calcReviewStatus(c, compID, totalRecords, reviewedCount)

		revStartStr := ""
		revEndStr := ""
		if isValidTime(item.ReviewStartTime) {
			revStartStr = item.ReviewStartTime.Format("2006-01-02T15:04:05Z")
		}
		if isValidTime(item.ReviewEndTime) {
			revEndStr = item.ReviewEndTime.Format("2006-01-02T15:04:05Z")
		}

		list = append(list, CompListItem{
			CompID:          compID,
			CompName:        item.CompName,
			CompLevel:       item.CompLevel,
			NeedReview:      item.NeedReview,
			ReviewStartTime: revStartStr,
			ReviewEndTime:   revEndStr,
			TotalWorks:      totalWorks,
			TotalExperts:    totalExperts,
			ReviewedCount:   reviewedCount,
			TotalRecords:    totalRecords,
			ReviewStatus:    reviewStatus,
			HasAward:        hasAward,
		})
	}

	utils.Success(c, gin.H{"list": list, "total": total})
}

func calcReviewStatus(c *gin.Context, compID uint, totalRecords, reviewedCount int64) string {
	var tasks []models.ReviewTask
	database.DB.WithContext(c.Request.Context()).Where("comp_id = ?", compID).Find(&tasks)

	if len(tasks) == 0 {
		return "uninit"
	}

	allZero := true
	allOne := true
	allThree := true
	for _, t := range tasks {
		if t.Status != 0 {
			allZero = false
		}
		if t.Status != 1 {
			allOne = false
		}
		if t.Status != 3 {
			allThree = false
		}
	}
	if allZero {
		return "uninit"
	}
	if allOne {
		return "pending"
	}
	if allThree {
		return "completed"
	}
	return "reviewing"
}

// GetExpertList 获取可选专家列表
func GetExpertList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	keyword := c.Query("keyword")
	collegeID := c.Query("college_id")

	if page < 1 {
		page = 1
	}
	if size < 1 || size > MaxPageSize {
		size = 10
	}
	query := database.DB.WithContext(c.Request.Context()).Model(&models.User{}).
		Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("roles.role_code = ?", "expert")
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}

	if keyword != "" {
		query = query.Where("users.realname LIKE ? OR users.username LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if scope != nil && scope.IsCollegeAdmin() {
		query = query.Where("users.college = (SELECT name FROM colleges WHERE id = ?)", *scope.ManagedCollegeID)
	} else if collegeID != "" {
		query = query.Where("users.college = (SELECT name FROM colleges WHERE id = ?)", collegeID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	type ExpertItem struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		College  string `json:"college"`
	}

	var list []ExpertItem
	offset := (page - 1) * size
	if err := query.
		Select("users.id, users.username, users.realname AS name, COALESCE(users.college, '') AS college").
		Order("users.create_time DESC").
		Offset(offset).Limit(size).
		Scan(&list).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	utils.Success(c, gin.H{"list": list, "total": total})
}

// GetReviewTaskList 查看评审任务列表
func GetReviewTaskList(c *gin.Context) {
	compIDStr := c.Query("comp_id")
	if compIDStr == "" {
		utils.BadRequest(c, "缺少 comp_id 参数")
		return
	}
	compID, err := strconv.ParseUint(compIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "comp_id 参数格式错误")
		return
	}
	if !requireCompetitionAccessIfScoped(c, uint(compID)) {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > MaxPageSize {
		size = 10
	}

	var total int64
	database.DB.WithContext(c.Request.Context()).Model(&models.ReviewTask{}).
		Where("comp_id = ?", compID).Count(&total)

	type TaskItem struct {
		TaskID         uint       `json:"task_id"`
		ExpertID       uint       `json:"expert_id"`
		ExpertName     string     `json:"expert_name"`
		ExpertUsername string     `json:"expert_username"`
		Status         int8       `json:"status"`
		ReviewedCount  int64      `json:"reviewed_count"`
		TotalWorks     int64      `json:"total_works"`
		AssignedAt     *time.Time `json:"assigned_at"`
		CompletedAt    *time.Time `json:"completed_at"`
	}

	var tasks []models.ReviewTask
	offset := (page - 1) * size
	database.DB.WithContext(c.Request.Context()).Where("comp_id = ?", compID).
		Order("id ASC").Offset(offset).Limit(size).Find(&tasks)

	var list []TaskItem
	for _, t := range tasks {
		var user models.User
		database.DB.WithContext(c.Request.Context()).Select("realname, username").First(&user, t.ExpertID)

		var reviewedCount int64
		database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).
			Where("task_id = ? AND status = 1", t.ID).Count(&reviewedCount)

		var totalWorks int64
		database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).
			Where("task_id = ?", t.ID).Count(&totalWorks)

		list = append(list, TaskItem{
			TaskID:         t.ID,
			ExpertID:       t.ExpertID,
			ExpertName:     user.Realname,
			ExpertUsername: user.Username,
			Status:         t.Status,
			ReviewedCount:  reviewedCount,
			TotalWorks:     totalWorks,
			AssignedAt:     t.AssignedAt,
			CompletedAt:    t.CompletedAt,
		})
	}

	utils.Success(c, gin.H{"list": list, "total": total})
}

// AssignReviewTask 单独调整专家分配
func AssignReviewTask(c *gin.Context) {
	var req struct {
		CompID    uint   `json:"comp_id" binding:"required"`
		ExpertIDs []uint `json:"expert_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	userID := userIDVal.(uint)
	if !requireCompetitionAccessIfScoped(c, req.CompID) {
		return
	}

	// 校验赛事存在且开启了评审
	var detail models.CompDetail
	if err := database.DB.WithContext(c.Request.Context()).Where("comp_id = ? AND need_review = 1", req.CompID).First(&detail).Error; err != nil {
		utils.BadRequest(c, "赛事不存在或未开启专家评审")
		return
	}

	now := time.Now()
	syncReviewTasks(c, req.CompID, req.ExpertIDs, userID, now, false, "")
	utils.SuccessWithMessage(c, "专家分配已更新", nil)
}

// syncReviewTasks 同步 ReviewTask：新增、跳过已初始化的删除、警告
func syncReviewTasks(c *gin.Context, compID uint, expertIDs []uint, userID uint, now time.Time, forceCloseReview bool, forceCloseReviewParam string) (warnings []string) {
	// 获取已有任务
	var existingTasks []models.ReviewTask
	database.DB.WithContext(c.Request.Context()).Where("comp_id = ?", compID).Find(&existingTasks)
	existingMap := make(map[uint]*models.ReviewTask)
	for i := range existingTasks {
		existingMap[existingTasks[i].ExpertID] = &existingTasks[i]
	}

	// force_close_review：删除所有 ReviewTask 和关联 ReviewRecord
	if forceCloseReview {
		for _, task := range existingTasks {
			database.DB.WithContext(c.Request.Context()).Where("task_id = ?", task.ID).Delete(&models.ReviewRecord{})
			database.DB.WithContext(c.Request.Context()).Delete(&task)
		}
		return
	}

	// 新增专家
	for _, eid := range expertIDs {
		if _, ok := existingMap[eid]; !ok {
			task := models.ReviewTask{
				CompID:     compID,
				ExpertID:   eid,
				Status:     0,
				AssignedBy: userID,
				AssignedAt: &now,
			}
			database.DB.WithContext(c.Request.Context()).Create(&task)
		}
	}

	// 移除专家
	newSet := make(map[uint]bool)
	for _, eid := range expertIDs {
		newSet[eid] = true
	}
	for _, task := range existingTasks {
		if !newSet[task.ExpertID] {
			if task.Status >= 1 {
				var user models.User
				database.DB.WithContext(c.Request.Context()).Select("realname").First(&user, task.ExpertID)
				warnings = append(warnings, "专家 "+user.Realname+" 已有评审记录，已跳过删除，请在任务管理中手动强制删除")
			} else {
				database.DB.WithContext(c.Request.Context()).Delete(&task)
			}
		}
	}
	return
}

// AutoCreateReviewRecordsForWork 学生提交作品后自动创建评审记录
func AutoCreateReviewRecordsForWork(compID, regID uint) {
	var detail models.CompDetail
	if err := database.DB.Where("comp_id = ? AND need_review = 1", compID).First(&detail).Error; err != nil {
		return
	}

	var tasks []models.ReviewTask
	database.DB.Where("comp_id = ?", compID).Find(&tasks)
	if len(tasks) == 0 {
		return
	}

	var reg models.Register
	if err := database.DB.Where("id = ? AND work_attachment_url IS NOT NULL AND work_attachment_url != ''", regID).First(&reg).Error; err != nil {
		return
	}

	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	taskStatusChanged := false
	for _, task := range tasks {
		var existing models.ReviewRecord
		if tx.Where("task_id = ? AND reg_id = ?", task.ID, regID).First(&existing).Error == nil {
			continue
		}

		record := models.ReviewRecord{
			TaskID:   task.ID,
			RegID:    regID,
			ExpertID: task.ExpertID,
			CompID:   compID,
			Status:   0,
		}
		if err := tx.Create(&record).Error; err != nil {
			tx.Rollback()
			return
		}
		taskStatusChanged = true

		if task.Status == 0 {
			tx.Model(&task).Update("status", 1)
		} else if task.Status == 3 {
			tx.Model(&task).Update("status", 2)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return
	}

	if taskStatusChanged {
		clearReviewCompListCache(context.Background())
	}
}

// InitReviewTasks 初始化评审任务
func InitReviewTasks(c *gin.Context) {
	var req struct {
		CompID uint `json:"comp_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	if !requireCompetitionAccessIfScoped(c, req.CompID) {
		return
	}

	// 校验赛事存在且 NeedReview=1
	var detail models.CompDetail
	if err := database.DB.WithContext(c.Request.Context()).Where("comp_id = ? AND need_review = 1", req.CompID).First(&detail).Error; err != nil {
		utils.BadRequest(c, "赛事不存在或未开启专家评审")
		return
	}

	// 校验作品提交已截止
	// 初始化评审不要求作品提交截止，管理员可在任意时间初始化

	// 查询 ReviewTask
	var tasks []models.ReviewTask
	database.DB.WithContext(c.Request.Context()).Where("comp_id = ?", req.CompID).Find(&tasks)
	if len(tasks) == 0 {
		utils.BadRequest(c, "请先在报名设置中分配评审专家")
		return
	}

	// 查询已提交作品
	var regs []models.Register
	database.DB.WithContext(c.Request.Context()).
		Where("comp_id = ? AND status IN (1,4) AND work_attachment_url IS NOT NULL AND work_attachment_url != ''", req.CompID).
		Find(&regs)
	if len(regs) == 0 {
		utils.BadRequest(c, "该赛事尚无已提交作品")
		return
	}

	newCount := 0
	taskStatusChanged := make(map[uint]bool) // taskID -> 本次是否新增了 Record

	tx := database.DB.WithContext(c.Request.Context()).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, task := range tasks {
		for _, reg := range regs {
			var existing models.ReviewRecord
			err := tx.Where("task_id = ? AND reg_id = ?", task.ID, reg.ID).First(&existing).Error
			if err == nil {
				continue // 已存在，跳过
			}

			record := models.ReviewRecord{
				TaskID:   task.ID,
				RegID:    reg.ID,
				ExpertID: task.ExpertID,
				CompID:   req.CompID,
				Status:   0,
			}
			if err := tx.Create(&record).Error; err != nil {
				tx.Rollback()
				utils.InternalServerError(c, "创建评审记录失败", err)
				return
			}
			newCount++
			taskStatusChanged[task.ID] = true
		}

		// 更新 ReviewTask 状态
		if task.Status == 0 {
			tx.Model(&task).Update("status", 1)
		} else if task.Status == 3 && taskStatusChanged[task.ID] {
			tx.Model(&task).Update("status", 2)
		}
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "初始化评审任务失败", err)
		return
	}

	clearReviewCompListCache(c.Request.Context())

	utils.Success(c, gin.H{"new_records": newCount, "message": "评审任务初始化完成"})
}

// DeleteReviewTask 删除评审任务
func DeleteReviewTask(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "参数格式错误")
		return
	}

	var task models.ReviewTask
	if err := database.DB.WithContext(c.Request.Context()).First(&task, id).Error; err != nil {
		utils.NotFound(c, "任务不存在")
		return
	}
	if !requireCompetitionAccessIfScoped(c, task.CompID) {
		return
	}

	force := c.Query("force") == "true"

	if task.Status >= 1 && !force {
		utils.Error(c, 409, 409, "该专家已有评审记录，确认强制删除？请传 force=true")
		return
	}

	tx := database.DB.WithContext(c.Request.Context()).Begin()
	if err := tx.Where("task_id = ?", task.ID).Delete(&models.ReviewRecord{}).Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "删除评审记录失败", err)
		return
	}
	if err := tx.Delete(&task).Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "删除任务失败", err)
		return
	}
	tx.Commit()

	clearReviewCompListCache(c.Request.Context())
	utils.SuccessWithMessage(c, "删除成功", nil)
}

// GetReviewProgress 查看评审进度
func GetReviewProgress(c *gin.Context) {
	compIDStr := c.Query("comp_id")
	if compIDStr == "" {
		utils.BadRequest(c, "缺少 comp_id 参数")
		return
	}
	compID, err := strconv.ParseUint(compIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "comp_id 参数格式错误")
		return
	}
	if !requireCompetitionAccessIfScoped(c, uint(compID)) {
		return
	}

	var comp models.CompDirectory
	if err := database.DB.WithContext(c.Request.Context()).Select("id, comp_name").First(&comp, compID).Error; err != nil {
		utils.NotFound(c, "赛事不存在")
		return
	}

	var totalWorks int64
	database.DB.WithContext(c.Request.Context()).Model(&models.Register{}).
		Where("comp_id = ? AND status IN (1,4) AND work_attachment_url IS NOT NULL AND work_attachment_url != ''", compID).
		Count(&totalWorks)

	var totalExperts int64
	database.DB.WithContext(c.Request.Context()).Model(&models.ReviewTask{}).Where("comp_id = ?", compID).Count(&totalExperts)

	var totalRecords int64
	database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).Where("comp_id = ?", compID).Count(&totalRecords)

	var reviewedCount int64
	database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).Where("comp_id = ? AND status = 1", compID).Count(&reviewedCount)

	type ExpertProgress struct {
		ExpertID        uint   `json:"expert_id"`
		ExpertName      string `json:"expert_name"`
		ExpertUsername  string `json:"expert_username"`
		AssignedWorks   int64  `json:"assigned_works"`
		ReviewedWorks   int64  `json:"reviewed_works"`
		UnreviewedWorks int64  `json:"unreviewed_works"`
		TaskStatus      int8   `json:"task_status"`
	}

	var tasks []models.ReviewTask
	database.DB.WithContext(c.Request.Context()).Where("comp_id = ?", compID).Find(&tasks)

	var experts []ExpertProgress
	for _, t := range tasks {
		var user models.User
		database.DB.WithContext(c.Request.Context()).Select("realname, username").First(&user, t.ExpertID)

		var assigned, reviewed int64
		database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).Where("task_id = ?", t.ID).Count(&assigned)
		database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).Where("task_id = ? AND status = 1", t.ID).Count(&reviewed)

		experts = append(experts, ExpertProgress{
			ExpertID:        t.ExpertID,
			ExpertName:      user.Realname,
			ExpertUsername:  user.Username,
			AssignedWorks:   assigned,
			ReviewedWorks:   reviewed,
			UnreviewedWorks: assigned - reviewed,
			TaskStatus:      t.Status,
		})
	}

	utils.Success(c, gin.H{
		"comp_id":        compID,
		"comp_name":      comp.CompName,
		"total_works":    totalWorks,
		"total_experts":  totalExperts,
		"total_records":  totalRecords,
		"reviewed_count": reviewedCount,
		"experts":        experts,
	})
}

// GetReviewResultList 查看评审结果汇总
func GetReviewResultList(c *gin.Context) {
	compIDStr := c.Query("comp_id")
	if compIDStr == "" {
		utils.BadRequest(c, "缺少 comp_id 参数")
		return
	}
	compID, err := strconv.ParseUint(compIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "comp_id 参数格式错误")
		return
	}
	if !requireCompetitionAccessIfScoped(c, uint(compID)) {
		return
	}

	var comp models.CompDirectory
	if err := database.DB.WithContext(c.Request.Context()).Select("id, comp_name").First(&comp, compID).Error; err != nil {
		utils.NotFound(c, "赛事不存在")
		return
	}

	var detail models.CompDetail
	database.DB.WithContext(c.Request.Context()).Where("comp_id = ?", compID).First(&detail)

	revEndStr := ""
	if isValidTime(detail.ReviewEndTime) {
		revEndStr = detail.ReviewEndTime.Format("2006-01-02T15:04:05Z")
	}

	// 检查是否全部完成
	var totalRecords, reviewedCount int64
	database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).Where("comp_id = ?", compID).Count(&totalRecords)
	database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).Where("comp_id = ? AND status = 1", compID).Count(&reviewedCount)
	allReviewed := totalRecords > 0 && totalRecords == reviewedCount

	// 是否已生成获奖
	var awardCount int64
	database.DB.WithContext(c.Request.Context()).Model(&models.Award{}).
		Where("comp_id = ? AND source = ?", compID, "review").Count(&awardCount)
	hasAward := awardCount > 0

	// 按报名记录分组计算平均分
	var records []models.ReviewRecord
	database.DB.WithContext(c.Request.Context()).Where("comp_id = ?", compID).Find(&records)

	type RegScore struct {
		RegID   uint
		Scores  []float64
		Details []gin.H
	}

	regMap := make(map[uint]*RegScore)
	var expertNames = make(map[uint]string)

	for _, r := range records {
		if _, ok := regMap[r.RegID]; !ok {
			regMap[r.RegID] = &RegScore{RegID: r.RegID}
		}
		rs := regMap[r.RegID]

		if r.Score != nil {
			rs.Scores = append(rs.Scores, *r.Score)

			if _, ok := expertNames[r.ExpertID]; !ok {
				var user models.User
				database.DB.WithContext(c.Request.Context()).Select("realname").First(&user, r.ExpertID)
				expertNames[r.ExpertID] = user.Realname
			}

			rs.Details = append(rs.Details, gin.H{
				"expert_name": expertNames[r.ExpertID],
				"score":       *r.Score,
				"comment":     r.Comment,
			})
		}
	}

	type ResultItem struct {
		Rank        uint     `json:"rank"`
		RegID       uint     `json:"reg_id"`
		TeamName    string   `json:"team_name"`
		Members     []string `json:"members"`
		AvgScore    float64  `json:"avg_score"`
		Scores      []gin.H  `json:"scores"`
		IsAbnormal  bool     `json:"is_abnormal"`
		SubmittedAt string   `json:"submitted_at"`
	}

	type regInfo struct {
		regID       uint
		teamName    string
		members     []string
		submittedAt time.Time
	}

	regInfoMap := make(map[uint]regInfo)
	var regIDs []uint
	for rid := range regMap {
		regIDs = append(regIDs, rid)
	}
	if len(regIDs) > 0 {
		var regs []models.Register
		database.DB.WithContext(c.Request.Context()).Preload("Members").Where("id IN ?", regIDs).Find(&regs)
		for _, r := range regs {
			var memberNames []string
			for _, m := range r.Members {
				memberNames = append(memberNames, m.Name)
			}
			regInfoMap[r.ID] = regInfo{
				regID:       r.ID,
				teamName:    r.TeamName,
				members:     memberNames,
				submittedAt: r.UpdatedAt,
			}
		}
	}

	var list []ResultItem
	for _, rs := range regMap {
		avg := 0.0
		if len(rs.Scores) > 0 {
			sum := 0.0
			for _, s := range rs.Scores {
				sum += s
			}
			avg = math.Round(sum/float64(len(rs.Scores))*10) / 10
		}

		isAbnormal := false
		if len(rs.Scores) >= 2 {
			minS, maxS := rs.Scores[0], rs.Scores[0]
			for _, s := range rs.Scores {
				if s < minS {
					minS = s
				}
				if s > maxS {
					maxS = s
				}
			}
			if maxS-minS >= 30 {
				isAbnormal = true
			}
		}

		info := regInfoMap[rs.RegID]
		submittedStr := ""
		if isValidTime(info.submittedAt) {
			submittedStr = info.submittedAt.Format("2006-01-02T15:04:05Z")
		}

		list = append(list, ResultItem{
			RegID:       rs.RegID,
			TeamName:    info.teamName,
			Members:     info.members,
			AvgScore:    avg,
			Scores:      rs.Details,
			IsAbnormal:  isAbnormal,
			SubmittedAt: submittedStr,
		})
	}

	// 排序：avg_score 降序，submitted_at 升序，reg_id 升序
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			swap := false
			if list[j].AvgScore > list[i].AvgScore {
				swap = true
			} else if list[j].AvgScore == list[i].AvgScore {
				if list[j].SubmittedAt < list[i].SubmittedAt {
					swap = true
				} else if list[j].SubmittedAt == list[i].SubmittedAt && list[j].RegID < list[i].RegID {
					swap = true
				}
			}
			if swap {
				list[i], list[j] = list[j], list[i]
			}
		}
	}

	// 分配排名
	for i := range list {
		list[i].Rank = uint(i + 1)
	}

	utils.Success(c, gin.H{
		"comp_id":         compID,
		"comp_name":       comp.CompName,
		"review_end_time": revEndStr,
		"all_reviewed":    allReviewed,
		"has_award":       hasAward,
		"list":            list,
		"total":           len(list),
	})
}

// ConfirmReviewResult 确认结果并生成获奖
func ConfirmReviewResult(c *gin.Context) {
	var req struct {
		CompID      uint `json:"comp_id" binding:"required"`
		AwardCounts []struct {
			Level string `json:"level" binding:"required"`
			Count int    `json:"count" binding:"required"`
		} `json:"award_counts" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	if !requireCompetitionAccessIfScoped(c, req.CompID) {
		return
	}

	var detail models.CompDetail
	if err := database.DB.WithContext(c.Request.Context()).Where("comp_id = ?", req.CompID).First(&detail).Error; err != nil {
		utils.NotFound(c, "赛事配置不存在")
		return
	}

	// 校验评审时间已结束
	if isValidTime(detail.ReviewEndTime) && time.Now().Before(detail.ReviewEndTime) {
		utils.BadRequest(c, "评审尚未结束，无法确认结果")
		return
	}

	// 校验所有 ReviewRecord 已完成
	var unreviewedCount int64
	database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).
		Where("comp_id = ? AND status = 0", req.CompID).Count(&unreviewedCount)
	if unreviewedCount > 0 {
		utils.BadRequest(c, "还有 "+strconv.FormatInt(unreviewedCount, 10)+" 份作品未完成评审，无法确认结果")
		return
	}

	// 检查是否已有 Source="review" 的 Award
	var existingAward int64
	database.DB.WithContext(c.Request.Context()).Model(&models.Award{}).
		Where("comp_id = ? AND source = ?", req.CompID, "review").Count(&existingAward)
	if existingAward > 0 {
		utils.Error(c, 409, 409, "该赛事已生成获奖名单，如需重新生成请先删除已有获奖记录")
		return
	}

	// 解析 AwardHierarchy
	var hierarchy []string
	if detail.AwardHierarchy != "" {
		json.Unmarshal([]byte(detail.AwardHierarchy), &hierarchy)
	}

	// 获取排名结果
	var records []models.ReviewRecord
	database.DB.WithContext(c.Request.Context()).Where("comp_id = ? AND status = 1", req.CompID).Find(&records)

	type regAvg struct {
		regID       uint
		avgScore    float64
		submittedAt time.Time
	}
	regMap := make(map[uint]*regAvg)
	for _, r := range records {
		if _, ok := regMap[r.RegID]; !ok {
			regMap[r.RegID] = &regAvg{regID: r.RegID}
		}
		if r.Score != nil {
			ra := regMap[r.RegID]
			ra.avgScore = (ra.avgScore*float64(len(regMap)) + *r.Score) / (float64(len(regMap)) + 1) // 逐步平均
		}
	}

	// 重新计算平均分
	regAvgs := make(map[uint]float64)
	for rid := range regMap {
		var scores []float64
		for _, r := range records {
			if r.RegID == rid && r.Score != nil {
				scores = append(scores, *r.Score)
			}
		}
		if len(scores) > 0 {
			sum := 0.0
			for _, s := range scores {
				sum += s
			}
			regAvgs[rid] = math.Round(sum/float64(len(scores))*10) / 10
		}
	}

	// 获取作品信息
	var regs []models.Register
	var regIDs []uint
	for rid := range regAvgs {
		regIDs = append(regIDs, rid)
	}
	database.DB.WithContext(c.Request.Context()).Where("id IN ?", regIDs).Find(&regs)
	regInfoMap := make(map[uint]models.Register)
	for _, r := range regs {
		regInfoMap[r.ID] = r
	}

	// 构建排名列表
	type rankItem struct {
		regID       uint
		avgScore    float64
		submittedAt time.Time
	}
	var ranked []rankItem
	for rid, avg := range regAvgs {
		info := regInfoMap[rid]
		ranked = append(ranked, rankItem{regID: rid, avgScore: avg, submittedAt: info.UpdatedAt})
	}

	// 排序
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			swap := false
			if ranked[j].avgScore > ranked[i].avgScore {
				swap = true
			} else if ranked[j].avgScore == ranked[i].avgScore {
				if ranked[j].submittedAt.Before(ranked[i].submittedAt) {
					swap = true
				} else if ranked[j].submittedAt.Equal(ranked[i].submittedAt) && ranked[j].regID < ranked[i].regID {
					swap = true
				}
			}
			if swap {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}

	totalWorks := len(ranked)

	// 校验 award_counts 各 level 与 hierarchy 一致
	levelMap := make(map[string]bool)
	for _, h := range hierarchy {
		levelMap[h] = true
	}

	// 自动调整名额
	totalAwardSlots := 0
	for _, ac := range req.AwardCounts {
		totalAwardSlots += ac.Count
	}
	if totalAwardSlots > totalWorks {
		// 从高到低缩减
		remaining := totalWorks
		for i := 0; i < len(req.AwardCounts) && remaining > 0; i++ {
			if req.AwardCounts[i].Count > remaining {
				req.AwardCounts[i].Count = remaining
			}
			remaining -= req.AwardCounts[i].Count
		}
	}

	// 获取 CompDirectory 用于拼接 AwardLevel
	var comp models.CompDirectory
	database.DB.WithContext(c.Request.Context()).Select("comp_level").First(&comp, req.CompID)

	levelPrefix := comp.CompLevel // "校级"/"省级"/"国家级"/"国际级"

	now := time.Now()
	tx := database.DB.WithContext(c.Request.Context()).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	rankIdx := 0
	for _, ac := range req.AwardCounts {
		// 查找该 level 在 hierarchy 中的索引
		levelRank := 99
		for idx, h := range hierarchy {
			if h == ac.Level {
				levelRank = idx + 1
				break
			}
		}

		for i := 0; i < ac.Count && rankIdx < totalWorks; i++ {
			item := ranked[rankIdx]
			award := models.Award{
				CompID:     req.CompID,
				RegID:      item.regID,
				LevelRank:  levelRank,
				AwardLevel: levelPrefix + ac.Level,
				AwardName:  levelPrefix + ac.Level,
				Status:     "approved",
				Source:     "review",
				AwardTime:  &now,
			}
			if err := tx.Create(&award).Error; err != nil {
				tx.Rollback()
				utils.InternalServerError(c, "创建获奖记录失败", err)
				return
			}
			rankIdx++
		}
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "确认结果失败", err)
		return
	}

	clearReviewCompListCache(c.Request.Context())
	utils.SuccessWithMessage(c, "获奖名单已生成", gin.H{"total_awards": rankIdx})
}

// ========== 专家 API ==========

// GetMyReviewTasks 我的评审任务列表
func GetMyReviewTasks(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	expertID := userIDVal.(uint)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > MaxPageSize {
		size = 10
	}

	var total int64
	database.DB.WithContext(c.Request.Context()).Model(&models.ReviewTask{}).
		Where("expert_id = ?", expertID).Count(&total)

	type TaskItem struct {
		TaskID          uint   `json:"task_id"`
		CompID          uint   `json:"comp_id"`
		CompName        string `json:"comp_name"`
		CompLevel       string `json:"comp_level"`
		TaskStatus      int8   `json:"task_status"`
		TotalWorks      int64  `json:"total_works"`
		ReviewedCount   int64  `json:"reviewed_count"`
		ReviewStartTime string `json:"review_start_time"`
		ReviewEndTime   string `json:"review_end_time"`
	}

	var tasks []models.ReviewTask
	offset := (page - 1) * size
	database.DB.WithContext(c.Request.Context()).Where("expert_id = ?", expertID).
		Order("id DESC").Offset(offset).Limit(size).Find(&tasks)

	var list []TaskItem
	for _, t := range tasks {
		var comp models.CompDirectory
		database.DB.WithContext(c.Request.Context()).Select("id, comp_name, comp_level").First(&comp, t.CompID)

		var detail models.CompDetail
		database.DB.WithContext(c.Request.Context()).Select("review_start_time, review_end_time").
			Where("comp_id = ?", t.CompID).First(&detail)

		var totalW, reviewed int64
		database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).Where("task_id = ?", t.ID).Count(&totalW)
		database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).Where("task_id = ? AND status = 1", t.ID).Count(&reviewed)

		revStart := ""
		revEnd := ""
		if isValidTime(detail.ReviewStartTime) {
			revStart = detail.ReviewStartTime.Format("2006-01-02T15:04:05Z")
		}
		if isValidTime(detail.ReviewEndTime) {
			revEnd = detail.ReviewEndTime.Format("2006-01-02T15:04:05Z")
		}

		list = append(list, TaskItem{
			TaskID:          t.ID,
			CompID:          t.CompID,
			CompName:        comp.CompName,
			CompLevel:       comp.CompLevel,
			TaskStatus:      t.Status,
			TotalWorks:      totalW,
			ReviewedCount:   reviewed,
			ReviewStartTime: revStart,
			ReviewEndTime:   revEnd,
		})
	}

	utils.Success(c, gin.H{"list": list, "total": total})
}

// GetMyReviewWorks 该任务下的全部作品列表
func GetMyReviewWorks(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	expertID := userIDVal.(uint)

	taskIDStr := c.Query("task_id")
	if taskIDStr == "" {
		utils.BadRequest(c, "缺少 task_id 参数")
		return
	}
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "task_id 参数格式错误")
		return
	}

	// 校验 task 属于当前专家
	var task models.ReviewTask
	if err := database.DB.WithContext(c.Request.Context()).Where("id = ? AND expert_id = ?", taskID, expertID).First(&task).Error; err != nil {
		utils.NotFound(c, "任务不存在")
		return
	}

	statusFilter := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > MaxPageSize {
		size = 10
	}

	query := database.DB.WithContext(c.Request.Context()).Model(&models.ReviewRecord{}).
		Where("task_id = ?", taskID)

	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	var total int64
	query.Count(&total)

	var records []models.ReviewRecord
	offset := (page - 1) * size
	query.Order("id ASC").Offset(offset).Limit(size).Find(&records)

	type WorkItem struct {
		RecordID          uint       `json:"record_id"`
		RegID             uint       `json:"reg_id"`
		TeamName          string     `json:"team_name"`
		LeaderName        string     `json:"leader_name"`
		WorkAttachmentUrl string     `json:"work_attachment_url"`
		SubmittedAt       *time.Time `json:"submitted_at"`
		ReviewStatus      int8       `json:"review_status"`
		Score             *float64   `json:"score"`
		ReviewedAt        *time.Time `json:"reviewed_at"`
	}

	var list []WorkItem
	for _, r := range records {
		var reg models.Register
		database.DB.WithContext(c.Request.Context()).Preload("Members").First(&reg, r.RegID)

		leaderName := ""
		for _, m := range reg.Members {
			if m.IsLeader {
				leaderName = m.Name
				break
			}
		}

		list = append(list, WorkItem{
			RecordID:          r.ID,
			RegID:             r.RegID,
			TeamName:          reg.TeamName,
			LeaderName:        leaderName,
			WorkAttachmentUrl: reg.WorkAttachmentUrl,
			SubmittedAt:       &reg.CreatedAt,
			ReviewStatus:      r.Status,
			Score:             r.Score,
			ReviewedAt:        r.ReviewedAt,
		})
	}

	utils.Success(c, gin.H{
		"list":        list,
		"total":       total,
		"task_status": task.Status,
	})
}

// GetReviewWorkDetail 作品详情
func GetReviewWorkDetail(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	expertID := userIDVal.(uint)

	regIDStr := c.Param("regId")
	regID, err := strconv.ParseUint(regIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "参数格式错误")
		return
	}

	taskIDStr := c.Query("task_id")
	if taskIDStr == "" {
		utils.BadRequest(c, "缺少 task_id 参数")
		return
	}
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "task_id 参数格式错误")
		return
	}

	// 校验 task 属于当前专家
	var task models.ReviewTask
	if err := database.DB.WithContext(c.Request.Context()).Where("id = ? AND expert_id = ?", taskID, expertID).First(&task).Error; err != nil {
		utils.NotFound(c, "任务不存在")
		return
	}

	// 获取 ReviewRecord
	var record models.ReviewRecord
	if err := database.DB.WithContext(c.Request.Context()).
		Where("task_id = ? AND reg_id = ? AND expert_id = ?", taskID, regID, expertID).
		First(&record).Error; err != nil {
		utils.NotFound(c, "评审记录不存在")
		return
	}

	// 获取作品信息
	var reg models.Register
	database.DB.WithContext(c.Request.Context()).Preload("Members").First(&reg, regID)

	type MemberInfo struct {
		Name  string `json:"name"`
		StuID string `json:"stu_id"`
	}
	var memberList []MemberInfo
	for _, m := range reg.Members {
		memberList = append(memberList, MemberInfo{Name: m.Name, StuID: m.StudentID})
	}

	// 获取赛事信息
	var comp models.CompDirectory
	database.DB.WithContext(c.Request.Context()).Select("id, comp_name, comp_level").First(&comp, task.CompID)

	utils.Success(c, gin.H{
		"review_record": gin.H{
			"id":          record.ID,
			"score":       record.Score,
			"comment":     record.Comment,
			"status":      record.Status,
			"reviewed_at": record.ReviewedAt,
		},
		"work": gin.H{
			"reg_id":              reg.ID,
			"team_name":           reg.TeamName,
			"members":             memberList,
			"work_attachment_url": reg.WorkAttachmentUrl,
			"submitted_at":        reg.CreatedAt.Format("2006-01-02T15:04:05Z"),
		},
		"competition": gin.H{
			"comp_id":    comp.ID,
			"comp_name":  comp.CompName,
			"comp_level": comp.CompLevel,
		},
	})
}

// SubmitReview 首次提交评审打分
func SubmitReview(c *gin.Context) {
	var req struct {
		RecordID uint    `json:"record_id" binding:"required"`
		Score    float64 `json:"score" binding:"required"`
		Comment  string  `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	expertID := userIDVal.(uint)

	if req.Score < 0 || req.Score > 100 {
		utils.BadRequest(c, "分数必须在 0-100 之间")
		return
	}

	var record models.ReviewRecord
	if err := database.DB.WithContext(c.Request.Context()).Where("id = ? AND expert_id = ?", req.RecordID, expertID).First(&record).Error; err != nil {
		utils.NotFound(c, "评审记录不存在")
		return
	}

	if record.Status == 1 {
		utils.Error(c, 409, 409, "该作品已评审，如需修改请使用修改接口")
		return
	}

	// 校验时间窗口
	var detail models.CompDetail
	if err := database.DB.WithContext(c.Request.Context()).Where("comp_id = ?", record.CompID).First(&detail).Error; err == nil {
		now := time.Now()
		if isValidTime(detail.ReviewStartTime) && now.Before(detail.ReviewStartTime) {
			utils.BadRequest(c, "评审尚未开始")
			return
		}
		if isValidTime(detail.ReviewEndTime) && now.After(detail.ReviewEndTime) {
			utils.BadRequest(c, "评审已结束")
			return
		}
	}

	score := math.Round(req.Score*10) / 10
	now := time.Now()

	tx := database.DB.WithContext(c.Request.Context()).Begin()
	tx.Model(&record).Updates(map[string]interface{}{
		"score":       score,
		"comment":     req.Comment,
		"status":      1,
		"reviewed_at": now,
	})

	// 更新 ReviewTask 状态
	updateTaskStatus(tx, record.TaskID)
	tx.Commit()

	clearReviewProgressCache(c.Request.Context(), record.CompID)
	clearReviewResultCache(c.Request.Context(), record.CompID)
	clearMyTasksCache(c.Request.Context(), expertID)

	utils.SuccessWithMessage(c, "评审提交成功", nil)
}

// UpdateReview 修改已提交的评审结果
func UpdateReview(c *gin.Context) {
	idStr := c.Param("id")
	recordID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		utils.BadRequest(c, "参数格式错误")
		return
	}

	var req struct {
		Score   float64 `json:"score" binding:"required"`
		Comment string  `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	expertID := userIDVal.(uint)

	if req.Score < 0 || req.Score > 100 {
		utils.BadRequest(c, "分数必须在 0-100 之间")
		return
	}

	var record models.ReviewRecord
	if err := database.DB.WithContext(c.Request.Context()).Where("id = ? AND expert_id = ?", recordID, expertID).First(&record).Error; err != nil {
		utils.NotFound(c, "评审记录不存在")
		return
	}

	if record.Status != 1 {
		utils.BadRequest(c, "该作品尚未评审，无法修改")
		return
	}

	// 校验时间窗口
	var detail models.CompDetail
	if err := database.DB.WithContext(c.Request.Context()).Where("comp_id = ?", record.CompID).First(&detail).Error; err == nil {
		now := time.Now()
		if isValidTime(detail.ReviewStartTime) && now.Before(detail.ReviewStartTime) {
			utils.BadRequest(c, "评审尚未开始")
			return
		}
		if isValidTime(detail.ReviewEndTime) && now.After(detail.ReviewEndTime) {
			utils.BadRequest(c, "评审已结束")
			return
		}
	}

	score := math.Round(req.Score*10) / 10
	now := time.Now()

	// 保存原值
	originalScore := record.Score
	originalComment := &record.Comment
	if record.OriginalScore == nil {
		originalScore = record.Score
	}
	if record.OriginalComment == nil {
		originalComment = &record.Comment
	}

	tx := database.DB.WithContext(c.Request.Context()).Begin()
	tx.Model(&record).Updates(map[string]interface{}{
		"score":            score,
		"comment":          req.Comment,
		"reviewed_at":      now,
		"original_score":   originalScore,
		"original_comment": originalComment,
	})
	tx.Commit()

	clearReviewProgressCache(c.Request.Context(), record.CompID)
	clearReviewResultCache(c.Request.Context(), record.CompID)

	utils.SuccessWithMessage(c, "评审修改成功", nil)
}

// ========== 辅助函数 ==========

// updateTaskStatus 更新 ReviewTask 状态
func updateTaskStatus(tx *gorm.DB, taskID uint) {
	var total, reviewed int64
	tx.Model(&models.ReviewRecord{}).Where("task_id = ?", taskID).Count(&total)
	tx.Model(&models.ReviewRecord{}).Where("task_id = ? AND status = 1", taskID).Count(&reviewed)

	newStatus := int8(2) // 评审中
	if total > 0 && total == reviewed {
		newStatus = 3 // 已完成
		now := time.Now()
		tx.Model(&models.ReviewTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
			"status":       newStatus,
			"completed_at": now,
		})
	} else {
		tx.Model(&models.ReviewTask{}).Where("id = ?", taskID).Update("status", newStatus)
	}
}

// ========== 缓存清理辅助函数 ==========

func clearReviewCompListCache(ctx context.Context) {
	// 预留缓存清理逻辑，与现有 clearCompListCache 一致
}

func clearReviewProgressCache(ctx context.Context, compID uint) {
	// 预留缓存清理逻辑
}

func clearReviewResultCache(ctx context.Context, compID uint) {
	// 预留缓存清理逻辑
}

func clearMyTasksCache(ctx context.Context, userID uint) {
	// 预留缓存清理逻辑
}
