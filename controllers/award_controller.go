package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type AwardAuditListItem struct {
	ID          uint   `json:"id"`
	StudentName string `json:"student_name"`
	StudentID   string `json:"student_id"`
	CompName    string `json:"comp_name"`
	AwardLevel  string `json:"award_level"`
	AwardDate   string `json:"award_date"`
	Phone       string `json:"phone"`
	SubmitTime  string `json:"submit_time"`
	Status      int    `json:"status"`
}

type AwardAuditDetailResp struct {
	ID            uint    `json:"id"`
	StudentName   string  `json:"student_name"`
	StudentID     string  `json:"student_id"`
	College       string  `json:"college"`
	Phone         string  `json:"phone"`
	Email         string  `json:"email"`
	CompName      string  `json:"comp_name"`
	AwardLevel    string  `json:"award_level"`
	AwardSpecific string  `json:"award_specific"`
	AwardDate     string  `json:"award_date"`
	TeamName      string  `json:"team_name"`
	Teammates     []gin.H `json:"teammates"`
	CertImage     string  `json:"cert_image"`
	SubmitTime    string  `json:"submit_time"`
	Status        int     `json:"status"`
	RejectReason  string  `json:"reject_reason"`
}

func mapAwardStatusToInt(status string) int {
	switch status {
	case "approved":
		return 1
	case "rejected":
		return 2
	default:
		return 0
	}
}

func mapAwardStatusFromInt(status int) string {
	switch status {
	case 1:
		return "approved"
	case 2:
		return "rejected"
	default:
		return "draft"
	}
}

func getLeaderMember(members []models.RegMember, fallback models.User) (string, string, string, string) {
	for _, m := range members {
		if m.IsLeader {
			return m.Name, m.StudentID, m.Phone, m.Email
		}
	}
	return fallback.Realname, fallback.Username, "", ""
}

// 1. 获取获奖管理的赛事列表
// 逻辑：如果是 Admin -> 返回所有赛事；如果是老师 -> 返回 ManagerID=自己的赛事
func GetAwardCompList(c *gin.Context) {
	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	offset := (page - 1) * pageSize

	var comps []models.CompDirectory
	var total int64

	db := database.DB.Model(&models.CompDirectory{})

	// 复用权限检查逻辑 (需要在同包下)
	if !checkUserIsAdmin(userID) {
		db = db.Where("manager_id = ?", userID)
	}

	db.Count(&total)

	// 只查主要字段，提升性能
	if err := db.Select("id, comp_name,comp_type, comp_level, status, year, organizer").
		Order("id desc").
		Offset(offset).Limit(pageSize).
		Find(&comps).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	c.JSON(200, gin.H{
		"code": 200,
		"data": gin.H{"list": comps, "total": total},
	})
}

func ImportAward(c *gin.Context) {
	compIDStr := c.Query("comp_id")
	if compIDStr == "" {
		c.JSON(400, gin.H{"code": 400, "msg": "缺少 comp_id"})
		return
	}
	compID, _ := strconv.Atoi(compIDStr)

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "文件上传失败"})
		return
	}

	f, err := excelize.OpenReader(file)
	if err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "Excel 读取失败"})
		return
	}

	// 获取第一个工作表
	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "内容解析失败"})
		return
	}

	successCount := 0

	tx := database.DB.Begin()

	for i, row := range rows {
		if i == 0 || len(row) < 5 {
			continue
		} // 跳过表头

		regIDStr := row[0]
		awardLevel := row[4] // E列
		awardName := ""
		if len(row) > 5 {
			awardName = row[5]
		} // F列

		if awardLevel == "" || regIDStr == "" {
			continue
		}

		regID, _ := strconv.Atoi(regIDStr)

		// Upsert 逻辑: 有则更新，无则插入
		var award models.Award
		err := tx.Where("reg_id = ?", regID).First(&award).Error

		if err != nil {
			// 不存在 -> 创建
			newAward := models.Award{
				CompID:     uint(compID),
				RegID:      uint(regID),
				AwardLevel: awardLevel,
				AwardName:  awardName,
			}
			if err := tx.Create(&newAward).Error; err == nil {
				successCount++
			}
		} else {
			// 存在 -> 更新
			award.AwardLevel = awardLevel
			award.AwardName = awardName
			if err := tx.Save(&award).Error; err == nil {
				successCount++
			}
		}
	}

	tx.Commit()
	c.JSON(200, gin.H{"code": 200, "msg": fmt.Sprintf("成功处理 %d 条数据", successCount)})
}

// 4. 获取获奖公示详情 (只读列表)
func GetCompAwards(c *gin.Context) {
	compID := c.Query("comp_id")

	var awards []models.Award

	// 关联 Register 和 Register.Leader 获取展示信息
	err := database.DB.
		Preload("Register").
		Preload("Register.Leader").
		Where("comp_id = ?", compID).
		Order("level_rank ASC").
		Find(&awards).Error

	if err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	// 组装简单的 DTO 返回给前端
	var list []gin.H
	for _, a := range awards {
		leaderName := "未知"
		if a.Register.Leader.ID != 0 {
			leaderName = a.Register.Leader.Realname
		}

		list = append(list, gin.H{
			"id":          a.ID,
			"team_name":   a.Register.TeamName,
			"leader_name": leaderName,
			"award_level": a.AwardLevel,
			"award_name":  a.AwardName,
		})
	}

	c.JSON(200, gin.H{"code": 200, "data": list})
}

// GetStudentMyAwardList 学生端：获取本人已申报的获奖列表（我的奖项）
func GetStudentMyAwardList(c *gin.Context) {
	// 1. 从登录中间件获取当前学生ID
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(401, gin.H{"code": 401, "msg": "请先登录"})
		return
	}
	studentID, ok := userIDVal.(uint)
	if !ok {
		c.JSON(400, gin.H{"code": 400, "msg": "用户ID格式错误"})
		return
	}

	// 2. 获取前端查询参数
	compIDStr := c.Query("comp_id")                       // 可选：按赛事ID筛选
	status := c.Query("status")                           // 可选：按申报状态筛选（draft/approved/rejected）
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))  // 默认第1页
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10")) // 默认每页10条

	// 3. 分页参数校验（复用现有逻辑，限制最大每页50条）
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 10
	}
	offset := (page - 1) * size

	// 4. 构建数据库查询（核心：关联Award→Register→CompDirectory，仅查当前学生数据）
	// 复用现有关联查询逻辑
	dbQuery := database.DB.Model(&models.Award{}).
		Preload("Register").                                     // 关联报名记录（获取团队名称）
		Preload("Register.Leader").                              // 关联负责人信息（获取学生姓名/学号）
		Preload("Register.CompDirectory").                       // 关联赛事信息（获取赛事名称/年份）
		Joins("JOIN registers ON awards.reg_id = registers.id"). // 关联报名表，通过leader_id筛选学生
		Where("registers.leader_id = ?", studentID)              // 核心筛选：仅当前学生的申报

	// 5. 可选筛选：赛事ID
	if compIDStr != "" {
		compID, err := strconv.ParseUint(compIDStr, 10, 32)
		if err != nil {
			c.JSON(400, gin.H{"code": 400, "msg": "赛事ID格式错误，必须是数字"})
			return
		}
		dbQuery = dbQuery.Where("awards.comp_id = ?", compID)
	}

	// 6. 可选筛选：申报状态
	if status != "" {
		validStatus := map[string]bool{"draft": true, "approved": true, "rejected": true}
		if !validStatus[status] {
			c.JSON(400, gin.H{"code": 400, "msg": "状态参数错误，仅支持draft/approved/rejected"})
			return
		}
		dbQuery = dbQuery.Where("awards.status = ?", status)
	}

	// 7. 排序：按申报时间倒序
	dbQuery = dbQuery.Order("awards.created_at DESC")

	// 8. 查询总数+分页列表（复用现有错误处理风格）
	var total int64
	var myAwardList []models.Award

	// 统计总数（用于前端分页）
	if err := dbQuery.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "统计我的获奖申报总数失败"})
		return
	}

	// 分页查询数据
	if err := dbQuery.Offset(offset).Limit(size).Find(&myAwardList).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "查询我的获奖申报列表失败"})
		return
	}

	// 9. 响应格式
	c.JSON(200, gin.H{
		"code": 200,
		"data": gin.H{
			"list":  myAwardList, // 获奖列表（含所有关联信息，前端可按需取用）
			"total": total,       // 总条数
			"page":  page,        // 当前页码
			"size":  size,        // 每页条数
		},
	})
}

func SubmitStudentAwardSupplement(c *gin.Context) {
	// 1. 获取当前登录学生ID
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(401, gin.H{"code": 401, "msg": "请先登录"})
		return
	}
	leaderID, ok := userIDVal.(uint)
	if !ok {
		c.JSON(400, gin.H{"code": 400, "msg": "用户ID格式错误"})
		return
	}

	// 2. 解析并校验请求参数（补录场景证明URL必填）
	var req struct {
		CompID     uint                   `json:"comp_id" binding:"required"`
		TeamName   string                 `json:"team_name" binding:"required"`
		Members    []models.RegMemberInfo `json:"members" binding:"required,dive"`
		AwardLevel string                 `json:"award_level" binding:"required"`
		AwardName  string                 `json:"award_name" binding:"required"`
		ProofURL   string                 `json:"proof_url" binding:"required"` // 补录必须传证明
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "参数错误：" + err.Error()})
		return
	}

	// 3. 队员信息强校验（补录场景必填）
	leaderCount := 0
	for _, member := range req.Members {
		if member.IsLeader {
			leaderCount++
		}
		if member.Name == "" || member.StudentID == "" || member.Phone == "" || member.College == "" {
			c.JSON(400, gin.H{"code": 400, "msg": fmt.Sprintf("队员[%s]的姓名/学号/手机号/学院不能为空", member.Name)})
			return
		}
	}
	if leaderCount == 0 || leaderCount > 1 {
		c.JSON(400, gin.H{"code": 400, "msg": "队员列表必须且仅能指定1名队长"})
		return
	}

	// 4. 校验赛事存在
	var comp models.CompDirectory
	if err := database.DB.First(&comp, req.CompID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"code": 404, "msg": "赛事不存在"})
			return
		}
		c.JSON(500, gin.H{"code": 500, "msg": "查询赛事失败：" + err.Error()})
		return
	}

	// 5. 开启事务
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 6. 创建补录报名记录（status=3）
	now := time.Now()
	reg := models.Register{
		CompID:         req.CompID,
		LeaderID:       leaderID,
		TeamName:       req.TeamName,
		Status:         3,    // 补录待审核
		SupplementTime: &now, // 记录补录时间
	}
	if err := tx.Create(&reg).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"code": 500, "msg": "创建补录报名失败：" + err.Error()})
		return
	}

	// 7. 批量存储队员
	for _, member := range req.Members {
		regMember := models.RegMember{
			RegID:     reg.ID,
			Name:      member.Name,
			StudentID: member.StudentID,
			Phone:     member.Phone,
			Email:     member.Email,
			College:   member.College,
			IsLeader:  member.IsLeader,
			Year:      member.Year,
		}
		if err := tx.Create(&regMember).Error; err != nil {
			tx.Rollback()
			c.JSON(500, gin.H{"code": 500, "msg": fmt.Sprintf("存储队员[%s]失败：%s", member.Name, err.Error())})
			return
		}
	}

	// 8. 创建补录奖项（标记source=supplement）
	award := models.Award{
		CompID:     req.CompID,
		RegID:      reg.ID,
		AwardLevel: req.AwardLevel,
		AwardName:  req.AwardName,
		Status:     "draft",
		ProofUrl:   req.ProofURL,
		Source:     "supplement", // 标记为学生补录
		LevelRank:  99,
	}
	if err := tx.Create(&award).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"code": 500, "msg": "创建补录奖项失败：" + err.Error()})
		return
	}

	// 9. 提交事务
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"code": 500, "msg": "事务提交失败：" + err.Error()})
		return
	}

	// 10. 返回响应
	c.JSON(200, gin.H{
		"code": 200,
		"data": gin.H{
			"award_id": award.ID,
			"reg_id":   reg.ID,
			"msg":      "补录申报成功，待审核",
		},
	})
}

// GetAwardAuditList 获取获奖补录审核列表
func GetAwardAuditList(c *gin.Context) {
	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if pageSize == 0 {
		pageSize, _ = strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	keyword := c.Query("keyword")
	statusStr := c.Query("status")
	awardLevel := c.Query("award_level")
	compName := c.Query("comp_name")
	source := c.DefaultQuery("source", "supplement")

	db := database.DB.Model(&models.Award{}).
		Joins("JOIN comp_directories ON comp_directories.id = awards.comp_id").
		Preload("Register").
		Preload("Register.Competition").
		Preload("Register.Members")

	if source != "all" {
		db = db.Where("awards.source = ?", source)
	}
	if statusStr != "" {
		statusInt, _ := strconv.Atoi(statusStr)
		db = db.Where("awards.status = ?", mapAwardStatusFromInt(statusInt))
	}
	if awardLevel != "" {
		db = db.Where("awards.award_level = ?", awardLevel)
	}
	if compName != "" {
		db = db.Where("comp_directories.comp_name LIKE ?", "%"+compName+"%")
	}

	if !checkUserIsAdmin(userID) {
		db = db.Where("comp_directories.manager_id = ?", userID)
	}

	var awards []models.Award
	if err := db.Order("awards.create_time DESC").Find(&awards).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	var list []AwardAuditListItem
	for _, a := range awards {
		leaderName, leaderID, phone, _ := getLeaderMember(a.Register.Members, a.Register.Leader)
		if keyword != "" {
			if !strings.Contains(leaderName, keyword) && !strings.Contains(leaderID, keyword) {
				continue
			}
		}

		submitTime := a.CreatedAt.Format("2006-01-02 15:04")
		if a.Register.SupplementTime != nil {
			submitTime = a.Register.SupplementTime.Format("2006-01-02 15:04")
		}

		list = append(list, AwardAuditListItem{
			ID:          a.ID,
			StudentName: leaderName,
			StudentID:   leaderID,
			CompName:    a.Register.Competition.CompName,
			AwardLevel:  a.AwardLevel,
			AwardDate:   "-",
			Phone:       phone,
			SubmitTime:  submitTime,
			Status:      mapAwardStatusToInt(a.Status),
		})
	}

	// 分页
	total := len(list)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	pageList := list[start:end]

	c.JSON(200, gin.H{
		"code": 200,
		"data": gin.H{
			"list":  pageList,
			"total": total,
		},
	})
}

// GetAwardAuditDetail 获取获奖补录审核详情
func GetAwardAuditDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	var award models.Award
	if err := database.DB.
		Preload("Register").
		Preload("Register.Competition").
		Preload("Register.Members").
		Preload("Register.Leader").
		First(&award, id).Error; err != nil {
		c.JSON(404, gin.H{"code": 404, "msg": "记录不存在"})
		return
	}

	leaderName, leaderID, phone, email := getLeaderMember(award.Register.Members, award.Register.Leader)
	college := award.Register.Leader.College
	if college == "" {
		for _, m := range award.Register.Members {
			if m.IsLeader {
				college = m.College
				break
			}
		}
	}

	teammates := make([]gin.H, 0)
	for _, m := range award.Register.Members {
		teammates = append(teammates, gin.H{
			"name":       m.Name,
			"student_id": m.StudentID,
			"college":    m.College,
		})
	}

	submitTime := award.CreatedAt.Format("2006-01-02 15:04:05")
	if award.Register.SupplementTime != nil {
		submitTime = award.Register.SupplementTime.Format("2006-01-02 15:04:05")
	}

	resp := AwardAuditDetailResp{
		ID:            award.ID,
		StudentName:   leaderName,
		StudentID:     leaderID,
		College:       college,
		Phone:         phone,
		Email:         email,
		CompName:      award.Register.Competition.CompName,
		AwardLevel:    award.AwardLevel,
		AwardSpecific: award.AwardName,
		AwardDate:     "-",
		TeamName:      award.Register.TeamName,
		Teammates:     teammates,
		CertImage:     award.ProofUrl,
		SubmitTime:    submitTime,
		Status:        mapAwardStatusToInt(award.Status),
		RejectReason:  award.RejectReason,
	}

	c.JSON(200, gin.H{"code": 200, "data": resp})
}

// PassAwardAudit 审核通过
func PassAwardAudit(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	var award models.Award
	if err := database.DB.Preload("Register").First(&award, id).Error; err != nil {
		c.JSON(404, gin.H{"code": 404, "msg": "记录不存在"})
		return
	}

	now := time.Now()
	tx := database.DB.Begin()
	if err := tx.Model(&award).Updates(map[string]interface{}{
		"status":        "approved",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": "",
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"code": 500, "msg": "审核更新失败"})
		return
	}

	if award.RegID != 0 {
		tx.Model(&models.Register{}).Where("id = ?", award.RegID).Updates(map[string]interface{}{
			"status":        4,
			"reject_reason": "",
		})
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(200, gin.H{"code": 200, "msg": "审核已通过"})
}

// RejectAwardAudit 审核驳回
func RejectAwardAudit(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	var req struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "驳回原因必填"})
		return
	}

	var award models.Award
	if err := database.DB.Preload("Register").First(&award, id).Error; err != nil {
		c.JSON(404, gin.H{"code": 404, "msg": "记录不存在"})
		return
	}

	now := time.Now()
	tx := database.DB.Begin()
	if err := tx.Model(&award).Updates(map[string]interface{}{
		"status":        "rejected",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": req.Reason,
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"code": 500, "msg": "审核更新失败"})
		return
	}

	if award.RegID != 0 {
		tx.Model(&models.Register{}).Where("id = ?", award.RegID).Updates(map[string]interface{}{
			"status":        5,
			"reject_reason": req.Reason,
		})
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(200, gin.H{"code": 200, "msg": "已驳回"})
}

// BatchPassAwardAudit 批量通过
func BatchPassAwardAudit(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(400, gin.H{"code": 400, "msg": "ids 必填"})
		return
	}
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	now := time.Now()
	tx := database.DB.Begin()
	if err := tx.Model(&models.Award{}).Where("id IN ?", req.IDs).Updates(map[string]interface{}{
		"status":        "approved",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": "",
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"code": 500, "msg": "批量更新失败"})
		return
	}

	var awards []models.Award
	if err := tx.Where("id IN ?", req.IDs).Find(&awards).Error; err == nil {
		for _, a := range awards {
			if a.RegID != 0 {
				tx.Model(&models.Register{}).Where("id = ?", a.RegID).Updates(map[string]interface{}{
					"status":        4,
					"reject_reason": "",
				})
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(200, gin.H{"code": 200, "msg": "批量通过成功"})
}

// BatchRejectAwardAudit 批量驳回
func BatchRejectAwardAudit(c *gin.Context) {
	var req struct {
		IDs    []uint `json:"ids" binding:"required"`
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(400, gin.H{"code": 400, "msg": "ids 和 reason 必填"})
		return
	}
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	now := time.Now()
	tx := database.DB.Begin()
	if err := tx.Model(&models.Award{}).Where("id IN ?", req.IDs).Updates(map[string]interface{}{
		"status":        "rejected",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": req.Reason,
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"code": 500, "msg": "批量更新失败"})
		return
	}

	var awards []models.Award
	if err := tx.Where("id IN ?", req.IDs).Find(&awards).Error; err == nil {
		for _, a := range awards {
			if a.RegID != 0 {
				tx.Model(&models.Register{}).Where("id = ?", a.RegID).Updates(map[string]interface{}{
					"status":        5,
					"reject_reason": req.Reason,
				})
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(200, gin.H{"code": 200, "msg": "批量驳回成功"})
}
