package controllers

import (
	"CompeManage_backend/database"
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
	"gorm.io/gorm"
)

// DateOnly 解析日期，支持 "2006-01-02" 和 RFC3339 两种格式
type DateOnly struct {
	time.Time
}

func (d *DateOnly) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	if str == "" || str == "null" {
		return nil
	}
	// 先试纯日期格式
	parsed, err := time.Parse("2006-01-02", str)
	if err == nil {
		d.Time = parsed
		return nil
	}
	// 再试 RFC3339
	parsed, err = time.Parse(time.RFC3339, str)
	if err == nil {
		d.Time = parsed
		return nil
	}
	return err
}

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

func clearAwardCache(ctx context.Context, compID interface{}) {
	rdb := middleware.GetRedisClient()
	cacheKey := fmt.Sprintf("cache:awards:%v", compID)
	if rdb != nil {
		rdb.Del(ctx, cacheKey)
		keys, err := rdb.Keys(ctx, "cache:stats:dashboard:*").Result()
		if err == nil && len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}
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

func GetAwardCompList(c *gin.Context) {
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	offset := (page - 1) * pageSize

	var comps []models.CompDirectory
	var total int64

	db := database.DB.WithContext(c.Request.Context()).Model(&models.CompDirectory{})

	db = awardCompetitionScope(db, scope)

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
	query := database.DB.WithContext(c.Request.Context()).Model(&models.CompDirectory{})
	if role, _ := c.Get("role_code"); role == "college_admin" {
		scope, ok := requireUserAccessScope(c)
		if !ok {
			return
		}
		query = awardCompetitionScope(query, scope)
	}
	if err := query.
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
	id, err := strconv.ParseUint(c.Query("comp_id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "赛事ID无效")
		return
	}
	scope, comp, ok := requireAwardCompetition(c, uint(id))
	if !ok {
		return
	}
	db := database.DB.WithContext(c.Request.Context())
	var awards []models.Award
	if err := db.Where("comp_id = ?", id).Preload("Members").Preload("Register.Leader").Preload("Register.Members").Order("create_time DESC").Find(&awards).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}
	collegeIDs := []uint{}
	collegeNames := map[uint]string{}
	for _, a := range awards {
		for _, m := range a.Members {
			if m.SubmittedCollegeID != nil {
				collegeIDs = append(collegeIDs, *m.SubmittedCollegeID)
			}
		}
	}
	if len(collegeIDs) > 0 {
		var colleges []models.College
		if err := db.Where("id IN ?", collegeIDs).Find(&colleges).Error; err != nil {
			utils.InternalServerError(c, "查询填报学院失败", err)
			return
		}
		for _, college := range colleges {
			collegeNames[college.ID] = college.Name
		}
	}

	list := []gin.H{}
	studentCount := 0
	for _, a := range awards {
		members := []gin.H{}
		for _, m := range a.Members {
			submittedCollegeName := ""
			if m.SubmittedCollegeID != nil {
				submittedCollegeName = collegeNames[*m.SubmittedCollegeID]
			}
			members = append(members, gin.H{"id": m.ID, "name": m.Name, "student_id": m.StudentNumber, "college": m.College, "major": m.Major, "remark": m.Remark, "submitted_college_id": m.SubmittedCollegeID, "submitted_college_name": submittedCollegeName, "submitted_by": m.SubmittedBy, "can_edit": canEditAwardMember(scope, m)})
		}
		allowed, err := canEditAwardTeam(db, scope, a.ID)
		if err != nil {
			utils.InternalServerError(c, "查询团队权限失败", err)
			return
		}
		month := ""
		if a.AwardTime != nil {
			month = a.AwardTime.Format("2006-01")
		}
		project := a.ProjectName
		if project == "" {
			project = a.Register.TeamName
		}
		studentCount += len(members)
		list = append(list, gin.H{"id": a.ID, "project_name": project, "team_name": project, "award_category": a.AwardCategory, "award_name": a.AwardName, "award_level": a.AwardLevel, "award_month": month, "members": members, "can_edit_team": allowed, "status": a.Status})
	}
	var hierarchy []string
	_ = json.Unmarshal([]byte(comp.Detail.AwardHierarchy), &hierarchy)
	utils.Success(c, gin.H{"list": list, "award_hierarchy": hierarchy, "student_count": studentCount, "team_count": len(list)})
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

	dbQuery := database.DB.WithContext(c.Request.Context()).Model(&models.Award{}).
		Preload("Register").
		Preload("Register.Leader").
		Preload("Register.Competition").
		Preload("Register.Members").
		Preload("Members").Preload("Competition").
		Where("EXISTS (SELECT 1 FROM award_members WHERE award_members.award_id = awards.id AND award_members.student_id = ? AND award_members.delete_time IS NULL) OR (NOT EXISTS (SELECT 1 FROM award_members WHERE award_members.award_id = awards.id AND award_members.delete_time IS NULL) AND EXISTS (SELECT 1 FROM registers WHERE registers.id = awards.reg_id AND registers.delete_time IS NULL AND (registers.leader_id = ? OR EXISTS (SELECT 1 FROM reg_members WHERE reg_members.reg_id = registers.id AND reg_members.username = (SELECT username FROM users WHERE id = ?)))))", studentID, studentID, studentID)

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

	for i := range myAwardList {
		a := &myAwardList[i]
		if a.ProjectName != "" {
			a.Register.TeamName = a.ProjectName
		}
		a.Register.Competition = a.Competition
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
		AwardDate  DateOnly               `json:"award_date"`
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
	if err := database.DB.WithContext(c.Request.Context()).First(&comp, req.CompID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "赛事不存在")
			return
		}
		utils.InternalServerError(c, "查询赛事失败", err)
		return
	}

	req.TeamName = strings.TrimSpace(req.TeamName)
	if req.TeamName == "" {
		utils.BadRequest(c, "项目名称不能为空")
		return
	}
	projectKey := database.AwardProjectKey(req.TeamName)
	var projectAwards []models.Award
	if err := database.DB.WithContext(c.Request.Context()).Where("comp_id = ? AND (project_key = ? OR (project_key IS NULL AND BINARY project_name = ?))", req.CompID, projectKey, req.TeamName).Find(&projectAwards).Error; err != nil {
		utils.InternalServerError(c, "查询项目奖项失败", err)
		return
	}
	for _, existing := range projectAwards {
		var memberCount int64
		if err := database.DB.WithContext(c.Request.Context()).Model(&models.AwardMember{}).Where("award_id = ? AND student_id = ?", existing.ID, leaderID).Count(&memberCount).Error; err != nil {
			utils.InternalServerError(c, "查询获奖成员失败", err)
			return
		}
		if existing.Status != "rejected" || memberCount == 0 {
			utils.BadRequest(c, "该赛事已存在同名项目奖项，请联系管理员处理，不能重复申报")
			return
		}
	}

	var existingReg models.Register
	existsErr := database.DB.WithContext(c.Request.Context()).Where("comp_id = ? AND leader_id = ?", req.CompID, leaderID).First(&existingReg).Error

	var regID uint

	tx := database.DB.WithContext(c.Request.Context()).Begin()
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

	var awardTime *time.Time
	if req.AwardDate.Year() > 1 {
		awardTime = &req.AwardDate.Time
	}

	var existingAward models.Award
	if err := tx.Where("reg_id = ?", regID).First(&existingAward).Error; err == nil {
		switch existingAward.Status {
		case "rejected":
			if err := tx.Model(&existingAward).Updates(map[string]interface{}{
				"project_name":  req.TeamName,
				"project_key":   projectKey,
				"award_time":    awardTime,
				"award_level":   req.AwardLevel,
				"award_name":    req.AwardName,
				"status":        "draft",
				"proof_url":     req.ProofURL,
				"reject_reason": "",
			}).Error; err != nil {
				tx.Rollback()
				utils.InternalServerError(c, "重新提交失败", err)
				return
			}
			if existingAward.RegID != 0 {
				if err := tx.Model(&models.Register{}).Where("id = ?", existingAward.RegID).Updates(map[string]interface{}{
					"status": 3,
				}).Error; err != nil {
					tx.Rollback()
					utils.InternalServerError(c, "更新报名状态失败", err)
					return
				}
			}
			if err := tx.Commit().Error; err != nil {
				utils.InternalServerError(c, "事务提交失败", err)
				return
			}
			clearAwardCache(c.Request.Context(), req.CompID)
			utils.SuccessWithMessage(c, "重新申报成功，待审核", gin.H{
				"award_id": existingAward.ID,
				"reg_id":   regID,
			})
			return
		case "approved":
			tx.Rollback()
			utils.BadRequest(c, "该赛事奖项已通过审核，无法重复申报")
			return
		default:
			tx.Rollback()
			utils.BadRequest(c, "该赛事您已提交过获奖申报，请勿重复提交")
			return
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		tx.Rollback()
		utils.InternalServerError(c, "查询已有申报记录失败", err)
		return
	}

	award := models.Award{
		ProjectName: req.TeamName,
		ProjectKey:  &projectKey,
		CompID:      req.CompID,
		RegID:       regID,
		AwardLevel:  req.AwardLevel,
		AwardName:   req.AwardName,
		AwardTime:   awardTime,
		Status:      "draft",
		ProofUrl:    req.ProofURL,
		Source:      "supplement",
		LevelRank:   99,
	}
	if err := tx.Create(&award).Error; err != nil {
		tx.Rollback()
		fmt.Printf("创建补录奖项失败，regID=%d, err=%v\n", regID, err)
		utils.InternalServerError(c, "创建补录奖项失败", err)
		return
	}

	if err := database.EnsureAwardMembers(tx, award.ID); err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "保存获奖成员失败", err)
		return
	}

	if err := tx.Commit().Error; err != nil {
		fmt.Printf("事务提交失败，regID=%d, err=%v\n", regID, err)
		utils.InternalServerError(c, "补录申报失败，请重试", err)
		return
	}

	clearAwardCache(c.Request.Context(), req.CompID)
	utils.SuccessWithMessage(c, "补录申报成功，待审核", gin.H{
		"award_id": award.ID,
		"reg_id":   regID,
	})
}

func GetAwardAuditList(c *gin.Context) {
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

	db := database.DB.WithContext(c.Request.Context()).Model(&models.Award{}).
		Joins("JOIN comp_directories ON comp_directories.id = awards.comp_id").
		Preload("Members").Preload("Competition").Preload("Register").
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

	scope, ok := requireUserAccessScope(c)
	if !ok {
		return
	}
	db = awardCompetitionScope(db, scope)

	var awards []models.Award
	if err := db.Order("awards.create_time DESC").Find(&awards).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	var list []AwardAuditListItem
	for _, a := range awards {
		leaderName, leaderID, phone, _ := getLeaderMember(a.Register.Members, a.Register.Leader)
		if len(a.Members) > 0 {
			leaderName = a.Members[0].Name
			leaderID = a.Members[0].StudentNumber
		}
		if keyword != "" {
			if !strings.Contains(leaderName, keyword) && !strings.Contains(leaderID, keyword) {
				continue
			}
		}

		submitTime := a.CreatedAt.Format("2006-01-02 15:04")
		if a.Register.SupplementTime != nil {
			submitTime = a.Register.SupplementTime.Format("2006-01-02 15:04")
		}

		listAwardDate := "-"
		if a.AwardTime != nil {
			listAwardDate = a.AwardTime.Format("2006-01-02")
		}

		list = append(list, AwardAuditListItem{
			ID:          a.ID,
			StudentName: leaderName,
			StudentID:   leaderID,
			CompName:    a.Competition.CompName,
			AwardLevel:  a.AwardLevel,
			AwardDate:   listAwardDate,
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
	if err := database.DB.WithContext(c.Request.Context()).
		Preload("Members").Preload("Competition").Preload("Register").
		Preload("Register.Competition").
		Preload("Register.Members").
		Preload("Register.Leader").
		First(&award, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}

	if _, scoped := c.Get("role_code"); scoped {
		if _, _, ok := requireAwardCompetition(c, award.CompID); !ok {
			return
		}
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

	if len(award.Members) > 0 {
		leaderName = award.Members[0].Name
		leaderID = award.Members[0].StudentNumber
		college = award.Members[0].College
		teammates = make([]gin.H, 0, len(award.Members))
		for _, m := range award.Members {
			teammates = append(teammates, gin.H{"name": m.Name, "student_id": m.StudentNumber, "college": m.College})
		}
	}

	submitTime := award.CreatedAt.Format("2006-01-02 15:04:05")
	if award.Register.SupplementTime != nil {
		submitTime = award.Register.SupplementTime.Format("2006-01-02 15:04:05")
	}

	awardDate := "-"
	if award.AwardTime != nil {
		awardDate = award.AwardTime.Format("2006-01-02")
	}

	resp := AwardAuditDetailResp{
		ID:            award.ID,
		StudentName:   leaderName,
		StudentID:     leaderID,
		College:       college,
		Phone:         phone,
		Email:         email,
		CompName:      award.Competition.CompName,
		AwardLevel:    award.AwardLevel,
		AwardSpecific: award.AwardName,
		AwardDate:     awardDate,
		TeamName:      award.ProjectName,
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
	if err := database.DB.WithContext(c.Request.Context()).Preload("Register").First(&award, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}

	if award.Status != "draft" {
		utils.BadRequest(c, "该记录已审核，无法重复操作")
		return
	}

	now := time.Now()
	tx := database.DB.WithContext(c.Request.Context()).Begin()
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
		if err := tx.Model(&models.Register{}).Where("id = ?", award.RegID).Updates(map[string]interface{}{
			"status":        4,
			"reject_reason": "",
		}).Error; err != nil {
			tx.Rollback()
			utils.InternalServerError(c, "更新报名状态失败", err)
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}
	clearAwardCache(c.Request.Context(), award.CompID)
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
	if err := database.DB.WithContext(c.Request.Context()).Preload("Register").First(&award, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}

	if award.Status != "draft" {
		utils.BadRequest(c, "该记录已审核，无法重复操作")
		return
	}

	now := time.Now()
	tx := database.DB.WithContext(c.Request.Context()).Begin()
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
		if err := tx.Model(&models.Register{}).Where("id = ?", award.RegID).Updates(map[string]interface{}{
			"status":        5,
			"reject_reason": req.Reason,
		}).Error; err != nil {
			tx.Rollback()
			utils.InternalServerError(c, "更新报名状态失败", err)
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}

	clearAwardCache(c.Request.Context(), award.CompID)
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
	tx := database.DB.WithContext(c.Request.Context()).Begin()
	result := tx.Model(&models.Award{}).Where("id IN ? AND status = ?", req.IDs, "draft").Updates(map[string]interface{}{
		"status":        "approved",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": "",
	})
	if result.Error != nil {
		tx.Rollback()
		utils.InternalServerError(c, "批量更新失败", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		utils.BadRequest(c, "没有可审核的记录（可能已审核或不存在）")
		return
	}

	var awards []models.Award
	if err := tx.Where("id IN ?", req.IDs).Find(&awards).Error; err == nil {
		compIDSet := make(map[uint]bool)
		for _, a := range awards {
			compIDSet[a.CompID] = true
			if a.RegID != 0 {
				if err := tx.Model(&models.Register{}).Where("id = ?", a.RegID).Updates(map[string]interface{}{
					"status":        4,
					"reject_reason": "",
				}).Error; err != nil {
					tx.Rollback()
					utils.InternalServerError(c, "更新报名状态失败", err)
					return
				}
			}
		}
		defer func() {
			for compID := range compIDSet {
				clearAwardCache(c.Request.Context(), compID)
			}
		}()
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
	tx := database.DB.WithContext(c.Request.Context()).Begin()
	result := tx.Model(&models.Award{}).Where("id IN ? AND status = ?", req.IDs, "draft").Updates(map[string]interface{}{
		"status":        "rejected",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": req.Reason,
	})
	if result.Error != nil {
		tx.Rollback()
		utils.InternalServerError(c, "批量更新失败", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		utils.BadRequest(c, "没有可审核的记录（可能已审核或不存在）")
		return
	}

	var awards []models.Award
	if err := tx.Where("id IN ?", req.IDs).Find(&awards).Error; err == nil {
		compIDSet := make(map[uint]bool)
		for _, a := range awards {
			compIDSet[a.CompID] = true
			if a.RegID != 0 {
				if err := tx.Model(&models.Register{}).Where("id = ?", a.RegID).Updates(map[string]interface{}{
					"status":        5,
					"reject_reason": req.Reason,
				}).Error; err != nil {
					tx.Rollback()
					utils.InternalServerError(c, "更新报名状态失败", err)
					return
				}
			}
		}
		defer func() {
			for compID := range compIDSet {
				clearAwardCache(c.Request.Context(), compID)
			}
		}()
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}

	utils.SuccessWithMessage(c, "批量驳回成功", nil)
}
