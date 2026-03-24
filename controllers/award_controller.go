package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/logger"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"context"
	"encoding/json"
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

type CompSearchResp struct {
	ID       uint   `json:"id"`
	CompName string `json:"comp_name"`
	Year     int    `json:"year"`
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

func clearAwardCache(compID interface{}) {
	rdb := middleware.GetRedisClient()
	ctx := context.Background()
	// 清除该赛事对应的奖项缓存
	cacheKey := fmt.Sprintf("cache:awards:%v", compID)
	rdb.Del(ctx, cacheKey)
}
func getLeaderMember(members []models.RegMember, fallback models.User) (string, string, string, string) {
	for _, m := range members {
		if m.IsLeader {
			return m.Name, m.StudentID, m.Phone, m.Email
		}
	}
	return fallback.Realname, fallback.Username, "", ""
}

func GetAwardCompList(c *gin.Context) {
	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	offset := (page - 1) * pageSize

	var comps []models.CompDirectory
	var total int64

	db := database.DB.Model(&models.CompDirectory{})

	if !checkUserIsAdmin(userID) {
		db = db.Where("manager_id = ?", userID)
	}

	db.Count(&total)

	if err := db.Preload("Detail").
		Order("id desc").
		Offset(offset).Limit(pageSize).
		Find(&comps).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	utils.Success(c, gin.H{"list": comps, "total": total})
}

func ExportAwardTemplate(c *gin.Context) {
	compID := c.Query("comp_id")

	var regs []models.Register
	if err := database.DB.
		Preload("Leader").
		Preload("Members").
		Where("comp_id = ? AND status = 1", compID).
		Find(&regs).Error; err != nil {
		utils.InternalServerError(c, "查询数据失败", err)
		return
	}

	f := excelize.NewFile()
	sheet := "获奖录入"
	f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")

	headers := []string{
		"奖项等级", "获奖项目名", "负责人", "学号", "所属学院", "指导老师",
		"成员1", "学号1",
		"成员2", "学号2",
		"成员3", "学号3",
		"成员4", "学号4",
		"成员5", "学号5",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	for i, r := range regs {
		row := i + 2

		leaderName, leaderStuID, _, _ := getLeaderMember(r.Members, r.Leader)

		college := r.Leader.College
		if college == "" {
			for _, m := range r.Members {
				if m.IsLeader {
					college = m.College
					break
				}
			}
		}

		var nonLeaderMembers []models.RegMember
		for _, m := range r.Members {
			if !m.IsLeader {
				nonLeaderMembers = append(nonLeaderMembers, m)
			}
		}

		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "")
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), r.TeamName)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), leaderName)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), leaderStuID)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), college)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), "")

		memberCols := []string{"G", "H", "I", "J", "K", "L", "M", "N", "O", "P"}
		for j := 0; j < 5; j++ {
			memberName := ""
			memberStuID := ""
			if j < len(nonLeaderMembers) {
				memberName = nonLeaderMembers[j].Name
				memberStuID = nonLeaderMembers[j].StudentID
			}
			f.SetCellValue(sheet, fmt.Sprintf("%s%d", memberCols[j*2], row), memberName)
			f.SetCellValue(sheet, fmt.Sprintf("%s%d", memberCols[j*2+1], row), memberStuID)
		}
	}

	fileName := fmt.Sprintf("Award_Template_%s.xlsx", compID)
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Cache-Control", "no-cache")

	if err := f.Write(c.Writer); err != nil {
		fmt.Println("导出流写入中断:", err)
	}
}

func ImportAward(c *gin.Context) {
	compIDStr := c.Query("comp_id")
	if compIDStr == "" {
		utils.BadRequest(c, "缺少 comp_id")
		return
	}
	compID, _ := strconv.Atoi(compIDStr)

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		utils.BadRequest(c, "文件上传失败")
		return
	}

	f, err := excelize.OpenReader(file)
	if err != nil {
		utils.BadRequest(c, "Excel 读取失败")
		return
	}

	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		utils.BadRequest(c, "内容解析失败")
		return
	}

	var comp models.CompDirectory
	if err := database.DB.Preload("Detail").First(&comp, compID).Error; err != nil {
		utils.BadRequest(c, "找不到对应赛事")
		return
	}

	var awardHierarchy []string
	if err := json.Unmarshal([]byte(comp.Detail.AwardHierarchy), &awardHierarchy); err != nil || len(awardHierarchy) == 0 {
		utils.BadRequest(c, "奖项等级配置解析失败或未配置")
		return
	}

	successCount := 0
	failCount := 0
	var failReasons []string

	tx := database.DB.Begin()

	for i, row := range rows {
		if i == 0 || len(row) < 2 {
			continue
		}

		levelRankStr := strings.TrimSpace(row[0])
		teamName := strings.TrimSpace(row[1])

		if levelRankStr == "" || teamName == "" {
			continue
		}

		levelRank, err := strconv.Atoi(levelRankStr)
		if err != nil || levelRank < 1 || levelRank > len(awardHierarchy) {
			failCount++
			failReasons = append(failReasons, fmt.Sprintf("第%d行：奖项等级[%s]无效，应为1~%d的数字", i+1, levelRankStr, len(awardHierarchy)))
			continue
		}

		awardName := awardHierarchy[levelRank-1]
		awardLevel := comp.CompLevel + awardName

		var reg models.Register
		if err := tx.Where("comp_id = ? AND team_name = ? AND status = 1", compID, teamName).First(&reg).Error; err != nil {
			failCount++
			failReasons = append(failReasons, fmt.Sprintf("第%d行：找不到团队[%s]的报名记录", i+1, teamName))
			continue
		}

		var award models.Award
		err = tx.Where("reg_id = ?", reg.ID).First(&award).Error
		if err != nil {
			now := time.Now()
			newAward := models.Award{
				CompID:     uint(compID),
				RegID:      reg.ID,
				LevelRank:  levelRank,
				AwardLevel: awardLevel,
				AwardName:  awardName,
				Status:     "approved",
				Source:     "import",
				AuditTime:  &now,
			}
			if err := tx.Create(&newAward).Error; err == nil {
				successCount++
			} else {
				failCount++
				failReasons = append(failReasons, fmt.Sprintf("第%d行：创建奖项失败 - %s", i+1, err.Error()))
			}
		} else {
			now := time.Now()
			award.LevelRank = levelRank
			award.AwardLevel = awardLevel
			award.AwardName = awardName
			award.Status = "approved"
			award.Source = "import"
			award.AuditTime = &now
			award.RejectReason = ""
			if err := tx.Save(&award).Error; err == nil {
				successCount++
			} else {
				failCount++
				failReasons = append(failReasons, fmt.Sprintf("第%d行：更新奖项失败 - %s", i+1, err.Error()))
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}

	resp := gin.H{
		"success_count": successCount,
		"fail_count":    failCount,
	}
	if len(failReasons) > 0 {
		resp["fail_reasons"] = failReasons
	}
	clearAwardCache(compID)
	utils.SuccessWithMessage(c, fmt.Sprintf("成功处理 %d 条，失败 %d 条", successCount, failCount), resp)
}

func SearchCompetition(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" {
		utils.BadRequest(c, "关键词不能为空")
		return
	}

	pageSize := c.DefaultQuery("page_size", "20")
	size := 20
	if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 && ps <= 100 {
		size = ps
	}

	var list []models.CompDirectory
	if err := database.DB.Model(&models.CompDirectory{}).
		Where("comp_name LIKE ?", "%"+keyword+"%").
		Select("id", "comp_name", "year").
		Order("create_time DESC").
		Limit(size).
		Find(&list).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	var results []CompSearchResp
	for _, item := range list {
		results = append(results, CompSearchResp{
			ID:       item.ID,
			CompName: item.CompName,
			Year:     item.Year,
		})
	}

	utils.Success(c, results)
}

func GetCompAwards(c *gin.Context) {
	compID := c.Query("comp_id")
	if compID == "" {
		utils.BadRequest(c, "缺少赛事ID")
		return
	}

	cacheKey := fmt.Sprintf("cache:awards:%s", compID)
	rdb := middleware.GetRedisClient()
	ctx := c.Request.Context()

	catchData, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		var resp gin.H
		if json.Unmarshal([]byte(catchData), &resp) == nil {
			logger.Info("获奖列表缓存命中", "cache_key", cacheKey)
			utils.Success(c, resp)
			return
		}
	}

	var comp models.CompDirectory
	if err := database.DB.Preload("Detail").First(&comp, compID).Error; err != nil {
		utils.InternalServerError(c, "赛事不存在", err)
		return
	}

	var awardHierarchy []string
	if err := json.Unmarshal([]byte(comp.Detail.AwardHierarchy), &awardHierarchy); err != nil {
		awardHierarchy = []string{}
	}

	var awards []models.Award

	err = database.DB.
		Preload("Register").
		Preload("Register.Leader").
		Preload("Register.Competition").
		Preload("Register.Members").
		Where("comp_id = ?", compID).
		Order("level_rank ASC").
		Find(&awards).Error

	if err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	var list []gin.H
	for _, a := range awards {
		leaderName := "未知"
		leaderCollege := ""

		if a.Register.Leader.ID != 0 {
			leaderName = a.Register.Leader.Realname
			leaderCollege = a.Register.Leader.College
		}

		if leaderCollege == "" {
			for _, m := range a.Register.Members {
				if m.IsLeader {
					leaderCollege = m.College
					break
				}
			}
		}

		advisorName := ""
		if a.Register.AdvisorInfo != "" {
			var advisorInfo models.AdvisorInfo
			if err := json.Unmarshal([]byte(a.Register.AdvisorInfo), &advisorInfo); err == nil {
				advisorName = advisorInfo.Name
			}
		}

		var members []gin.H
		for _, m := range a.Register.Members {
			if !m.IsLeader {
				members = append(members, gin.H{
					"name":       m.Name,
					"student_id": m.StudentID,
					"phone":      m.Phone,
					"email":      m.Email,
					"college":    m.College,
				})
			}
		}

		list = append(list, gin.H{
			"id":             a.ID,
			"team_name":      a.Register.TeamName,
			"leader_name":    leaderName,
			"leader_college": leaderCollege,
			"award_level":    a.AwardLevel,
			"award_name":     a.AwardName,
			"comp_level":     a.Register.Competition.CompLevel,
			"advisor_name":   advisorName,
			"members":        members,
		})
	}
	go func() {
		data, _ := json.Marshal(list)
		rdb.Set(context.Background(), cacheKey, data, 30*time.Minute).Result()
	}()
	utils.Success(c, gin.H{
		"award_hierarchy": awardHierarchy,
		"list":            list,
	})
}

func GetStudentMyAwardList(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "请先登录")
		return
	}
	studentID, ok := userIDVal.(uint)
	if !ok {
		utils.BadRequest(c, "用户ID格式错误")
		return
	}

	compIDStr := c.Query("comp_id")
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 10
	}
	offset := (page - 1) * size

	dbQuery := database.DB.Model(&models.Award{}).
		Preload("Register").
		Preload("Register.Leader").
		Preload("Register.Competition").
		Joins("JOIN registers ON awards.reg_id = registers.id").
		Where("registers.leader_id = ?", studentID)

	if compIDStr != "" {
		compID, err := strconv.ParseUint(compIDStr, 10, 32)
		if err != nil {
			utils.BadRequest(c, "赛事ID格式错误，必须是数字")
			return
		}
		dbQuery = dbQuery.Where("awards.comp_id = ?", compID)
	}

	if status != "" {
		validStatus := map[string]bool{"draft": true, "approved": true, "rejected": true}
		if !validStatus[status] {
			utils.BadRequest(c, "状态参数错误，仅支持draft/approved/rejected")
			return
		}
		dbQuery = dbQuery.Where("awards.status = ?", status)
	}

	dbQuery = dbQuery.Order("awards.create_time DESC")

	var total int64
	var myAwardList []models.Award

	if err := dbQuery.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "统计我的获奖申报总数失败", err)
		return
	}

	if err := dbQuery.Offset(offset).Limit(size).Find(&myAwardList).Error; err != nil {
		utils.InternalServerError(c, "查询我的获奖申报列表失败", err)
		return
	}

	utils.Success(c, gin.H{
		"list":  myAwardList,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

func SubmitStudentAwardSupplement(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "请先登录")
		return
	}
	leaderID, ok := userIDVal.(uint)
	if !ok {
		utils.BadRequest(c, "用户ID格式错误")
		return
	}

	var req struct {
		CompID     uint                   `json:"comp_id" binding:"required"`
		TeamName   string                 `json:"team_name" binding:"required"`
		Members    []models.RegMemberInfo `json:"members" binding:"required,dive"`
		AwardLevel string                 `json:"award_level" binding:"required"`
		AwardName  string                 `json:"award_name" binding:"required"`
		ProofURL   string                 `json:"proof_url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	leaderCount := 0
	for _, member := range req.Members {
		if member.IsLeader {
			leaderCount++
		}
		if member.Name == "" || member.StudentID == "" || member.Phone == "" || member.College == "" {
			utils.BadRequest(c, fmt.Sprintf("队员[%s]的姓名/学号/手机号/学院不能为空", member.Name))
			return
		}
	}
	if leaderCount == 0 || leaderCount > 1 {
		utils.BadRequest(c, "队员列表必须且仅能指定1名队长")
		return
	}

	var comp models.CompDirectory
	if err := database.DB.First(&comp, req.CompID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "赛事不存在")
			return
		}
		utils.InternalServerError(c, "查询赛事失败", err)
		return
	}

	var existingReg models.Register
	existsErr := database.DB.Where("comp_id = ? AND leader_id = ?", req.CompID, leaderID).First(&existingReg).Error

	var regID uint

	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			fmt.Println("事务发生 panic，已回滚:", r)
		}
	}()

	if errors.Is(existsErr, gorm.ErrRecordNotFound) {
		now := time.Now()
		reg := models.Register{
			CompID:         req.CompID,
			LeaderID:       leaderID,
			TeamName:       req.TeamName,
			Status:         3,
			SupplementTime: &now,
			AdvisorInfo:    "{}",
		}
		if err := tx.Create(&reg).Error; err != nil {
			tx.Rollback()
			fmt.Printf("创建补录报名失败，compID=%d, leaderID=%d, err=%v\n", req.CompID, leaderID, err)
			utils.InternalServerError(c, "创建补录报名失败", err)
			return
		}
		regID = reg.ID

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
				fmt.Printf("存储队员[%s]失败，regID=%d, err=%v\n", member.Name, reg.ID, err)
				utils.InternalServerError(c, fmt.Sprintf("存储队员[%s]失败", member.Name), err)
				return
			}
		}
	} else if existsErr != nil {
		tx.Rollback()
		fmt.Printf("查询报名记录失败，err=%v\n", existsErr)
		utils.InternalServerError(c, "查询报名信息失败", existsErr)
		return
	} else {
		regID = existingReg.ID
		fmt.Printf("检测到已存在的报名记录，regID=%d，跳过补录报名创建\n", regID)
	}

	award := models.Award{
		CompID:     req.CompID,
		RegID:      regID,
		AwardLevel: req.AwardLevel,
		AwardName:  req.AwardName,
		Status:     "draft",
		ProofUrl:   req.ProofURL,
		Source:     "supplement",
		LevelRank:  99,
	}
	if err := tx.Create(&award).Error; err != nil {
		tx.Rollback()
		fmt.Printf("创建补录奖项失败，regID=%d, err=%v\n", regID, err)
		utils.InternalServerError(c, "创建补录奖项失败", err)
		return
	}

	if err := tx.Commit().Error; err != nil {
		fmt.Printf("事务提交失败，regID=%d, err=%v\n", regID, err)
		utils.InternalServerError(c, "补录申报失败，请重试", err)
		return
	}

	clearAwardCache(req.CompID)
	utils.SuccessWithMessage(c, "补录申报成功，待审核", gin.H{
		"award_id": award.ID,
		"reg_id":   regID,
	})
}

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
		utils.InternalServerError(c, "查询失败", err)
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

	utils.Success(c, gin.H{
		"list":  pageList,
		"total": total,
	})
}

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
		utils.NotFound(c, "记录不存在")
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

	utils.Success(c, resp)
}

func PassAwardAudit(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	var award models.Award
	if err := database.DB.Preload("Register").First(&award, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
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
		utils.InternalServerError(c, "审核更新失败", err)
		return
	}

	if award.RegID != 0 {
		tx.Model(&models.Register{}).Where("id = ?", award.RegID).Updates(map[string]interface{}{
			"status":        4,
			"reject_reason": "",
		})
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}
	clearAwardCache(id)
	utils.SuccessWithMessage(c, "审核已通过", nil)
}

func RejectAwardAudit(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	var req struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "驳回原因必填")
		return
	}

	var award models.Award
	if err := database.DB.Preload("Register").First(&award, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
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
		utils.InternalServerError(c, "审核更新失败", err)
		return
	}

	if award.RegID != 0 {
		tx.Model(&models.Register{}).Where("id = ?", award.RegID).Updates(map[string]interface{}{
			"status":        5,
			"reject_reason": req.Reason,
		})
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}

	utils.SuccessWithMessage(c, "已驳回", nil)
}

func BatchPassAwardAudit(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "ids 必填")
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
		utils.InternalServerError(c, "批量更新失败", err)
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
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}

	utils.SuccessWithMessage(c, "批量通过成功", nil)
}

func BatchRejectAwardAudit(c *gin.Context) {
	var req struct {
		IDs    []uint `json:"ids" binding:"required"`
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "ids 和 reason 必填")
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
		utils.InternalServerError(c, "批量更新失败", err)
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
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}

	utils.SuccessWithMessage(c, "批量驳回成功", nil)
}
