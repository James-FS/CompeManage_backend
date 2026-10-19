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
	"gorm.io/gorm/clause"
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
	UpdatedAt   time.Time `json:"update_time"`
	ID          uint      `json:"id"`
	StudentName string    `json:"student_name"`
	StudentID   string    `json:"student_id"`
	CompName    string    `json:"comp_name"`
	AwardLevel  string    `json:"award_level"`
	AwardDate   string    `json:"award_date"`
	Phone       string    `json:"phone"`
	SubmitTime  string    `json:"submit_time"`
	Status      int       `json:"status"`
}

type AwardAuditDetailResp struct {
	UpdatedAt         time.Time `json:"update_time"`
	ID                uint      `json:"id"`
	StudentName       string    `json:"student_name"`
	StudentID         string    `json:"student_id"`
	College           string    `json:"college"`
	Phone             string    `json:"phone"`
	Email             string    `json:"email"`
	CompName          string    `json:"comp_name"`
	AwardLevel        string    `json:"award_level"`
	AwardSpecific     string    `json:"award_specific"`
	AwardDate         string    `json:"award_date"`
	AwardCategory     string    `json:"award_category"`
	ParticipationType string    `json:"participation_type"`
	AdvisorInfo       string    `json:"advisor_info"`
	Remark            string    `json:"remark"`
	TeamName          string    `json:"team_name"`
	Teammates         []gin.H   `json:"teammates"`
	CertImage         string    `json:"cert_image"`
	SubmitTime        string    `json:"submit_time"`
	Status            int       `json:"status"`
	RejectReason      string    `json:"reject_reason"`
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

	// 可选筛选：关键词（赛事名称/编号）、级别、举办年份、进行状态；缺省不过滤
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("comp_name LIKE ? OR comp_code LIKE ?", like, like)
	}
	if level := c.Query("level"); level != "" && level != "全部" {
		db = db.Where("comp_level = ?", level)
	}
	if year := c.Query("year"); year != "" {
		yearInt, err := strconv.Atoi(year)
		if err != nil || len(year) != 4 {
			utils.BadRequest(c, "year 参数须为4位赛事年份")
			return
		}
		db = db.Where("year = ?", yearInt)
	}
	switch status := c.Query("status"); status {
	case "ongoing", "进行中":
		db = db.Where("status <> ?", 2)
	case "ended", "已结束":
		db = db.Where("status = ?", 2)
	}

	db.Count(&total)

	if err := db.Preload("Detail").
		Order("id desc").
		Offset(offset).Limit(pageSize).
		Find(&comps).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	// 年份下拉选项：全部赛事的去重举办年份，倒序
	var years []int
	database.DB.WithContext(c.Request.Context()).Model(&models.CompDirectory{}).
		Distinct().Order("year desc").Pluck("year", &years)

	utils.Success(c, gin.H{"list": comps, "total": total, "years": years})
}

func SearchCompetition(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" && c.Query("paginated") != "true" {
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
	if keyword != "" {
		query = query.Where("comp_name LIKE ? OR comp_code LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if year := c.Query("year"); year != "" {
		yearInt, err := strconv.Atoi(year)
		if err != nil || len(year) != 4 {
			utils.BadRequest(c, "年份格式错误")
			return
		}
		query = query.Where("year = ?", yearInt)
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	if c.Query("paginated") == "true" {
		var total int64
		if err := query.Count(&total).Error; err != nil {
			utils.InternalServerError(c, "统计赛事失败", err)
			return
		}
		if err := query.Select("id", "comp_name", "comp_code", "comp_level", "year").Order("year DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
			utils.InternalServerError(c, "查询失败", err)
			return
		}
		utils.Success(c, gin.H{"list": list, "total": total})
		return
	}
	if err := query.Select("id", "comp_name", "year").Order("create_time DESC").Limit(size).Find(&list).Error; err != nil {
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
	if err := db.Where("comp_id = ? AND status IN ?", id, []string{"approved", "1"}).Preload("Members").Preload("Advisors").Preload("Register.Leader").Preload("Register.Members").Order("create_time DESC").Find(&awards).Error; err != nil {
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
		advisors := []gin.H{}
		for _, adv := range a.Advisors {
			advisors = append(advisors, gin.H{"username": adv.Username, "name": adv.Name, "college": adv.College, "sort": adv.Sort})
		}
		studentCount += len(members)
		list = append(list, gin.H{"id": a.ID, "project_name": project, "team_name": project, "award_category": a.AwardCategory, "award_name": a.AwardName, "award_level": a.AwardLevel, "award_month": month, "members": members, "advisors": advisors, "can_edit_team": allowed, "status": a.Status, "advisor_info": a.AdvisorInfo, "remark": a.Remark, "participation_type": a.ParticipationType})
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

	if source := c.Query("source"); source != "" {
		if source != "supplement" && source != "import" {
			utils.BadRequest(c, "来源参数错误，仅支持supplement/import")
			return
		}
		dbQuery = dbQuery.Where("awards.source = ?", source)
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
		hasParticipantSnapshot := false
		for _, m := range a.Members {
			if m.IsLeader {
				hasParticipantSnapshot = true
				break
			}
		}
		if a.Source == "supplement" && hasParticipantSnapshot {
			a.Register.Members = nil
			for _, m := range a.Members {
				a.Register.Members = append(a.Register.Members, models.RegMember{Name: m.Name, StudentID: m.StudentNumber, College: m.College, Phone: m.Phone, Email: m.Email, IsLeader: m.IsLeader})
			}
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
		ParticipationType string                 `json:"participation_type"`
		AwardCategory     string                 `json:"award_category"`
		Advisors          []models.AdvisorInfo   `json:"advisors"`
		Remark            string                 `json:"remark"`
		CompID            uint                   `json:"comp_id" binding:"required"`
		TeamName          string                 `json:"team_name" binding:"required"`
		Members           []models.RegMemberInfo `json:"members" binding:"required,dive"`
		AwardLevel        string                 `json:"award_level" binding:"required"`
		AwardName         string                 `json:"award_name" binding:"required"`
		AwardDate         DateOnly               `json:"award_date"`
		ProofURL          string                 `json:"proof_url" binding:"required"`
		UpdatedAt         *time.Time             `json:"update_time"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	advisorJSON, err := json.Marshal(req.Advisors)
	if err != nil {
		utils.BadRequest(c, "指导教师信息无效")
		return
	}
	if req.Advisors == nil {
		advisorJSON = []byte("[]")
	}

	leaderCount := 0
	memberUsernames := make([]string, 0, len(req.Members))
	seenMemberUsernames := make(map[string]bool, len(req.Members))
	for i := range req.Members {
		member := &req.Members[i]
		member.StudentID = strings.TrimSpace(member.StudentID)
		if member.IsLeader {
			leaderCount++
		}
		if member.Name == "" || member.StudentID == "" || member.Phone == "" || member.College == "" {
			utils.BadRequest(c, fmt.Sprintf("队员[%s]的姓名/学号/手机号/学院不能为空", member.Name))
			return
		}
		if seenMemberUsernames[member.StudentID] {
			utils.BadRequest(c, "队员列表中存在重复学号")
			return
		}
		seenMemberUsernames[member.StudentID] = true
		memberUsernames = append(memberUsernames, member.StudentID)
	}
	if leaderCount != 1 {
		utils.BadRequest(c, "队员列表必须且仅能指定1名队长")
		return
	}
	if req.ParticipationType != "" {
		if err := validateStudentAwardDetails(req.ParticipationType, req.AwardCategory, req.AwardName, req.AwardDate.Time, req.Members, req.Advisors, req.Remark); err != nil {
			utils.BadRequest(c, err.Error())
			return
		}
		req.AwardLevel = req.AwardCategory + req.AwardName
	}

	req.TeamName = strings.TrimSpace(req.TeamName)
	if req.TeamName == "" || len([]rune(req.TeamName)) > 255 {
		utils.BadRequest(c, "项目名称不能为空且不能超过255字")
		return
	}
	projectKey := database.AwardProjectKey(req.TeamName)

	tx := database.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		utils.InternalServerError(c, "开启获奖申报事务失败", tx.Error)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			fmt.Println("事务发生 panic，已回滚:", r)
			return
		}
		_ = tx.Rollback().Error
	}()

	var comp models.CompDirectory
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&comp, req.CompID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "赛事不存在")
			return
		}
		utils.InternalServerError(c, "查询赛事失败", err)
		return
	}

	var authenticatedUser models.User
	// Validate work IDs server-side and store authoritative teacher snapshots.
	resolvedAdvisors, advisorErr := resolveStudentAwardAdvisors(tx, req.Advisors)
	if advisorErr != nil {
		utils.BadRequest(c, advisorErr.Error())
		return
	}
	advisorJSON, _ = json.Marshal(resolvedAdvisors)
	if err := tx.Select("id", "username").First(&authenticatedUser, leaderID).Error; err != nil {
		utils.InternalServerError(c, "查询当前登录学生失败", err)
		return
	}

	var existingReg models.Register
	existingRegErr := tx.Where("comp_id = ? AND leader_id = ?", req.CompID, leaderID).First(&existingReg).Error
	existingRegFound := existingRegErr == nil
	if existingRegErr != nil && !errors.Is(existingRegErr, gorm.ErrRecordNotFound) {
		utils.InternalServerError(c, "查询报名信息失败", existingRegErr)
		return
	}
	regID := uint(0)
	if existingRegFound {
		regID = existingReg.ID
	}

	var existingAward models.Award
	resubmitAwardID := uint(0)
	if existingRegFound {
		err := tx.Where("reg_id = ?", regID).First(&existingAward).Error
		if err == nil {
			switch existingAward.Status {
			case "rejected":
				resubmitAwardID = existingAward.ID
			case "approved":
				utils.BadRequest(c, "记录已存在：该学生在本赛事已有获奖记录；如需更正，请联系管理员修改原记录")
				return
			default:
				utils.BadRequest(c, "记录已存在：该学生已提交本赛事获奖申报，请勿重复提交")
				return
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			utils.InternalServerError(c, "查询已有申报记录失败", err)
			return
		}
	}

	var projectAwards []models.Award
	if err := tx.Where("comp_id = ? AND (project_key = ? OR (project_key IS NULL AND BINARY project_name = ?))", req.CompID, projectKey, req.TeamName).Find(&projectAwards).Error; err != nil {
		utils.InternalServerError(c, "查询项目奖项失败", err)
		return
	}

	// A teammate may resubmit the same rejected award. Reuse its registration,
	// but only when the authenticated student is already one of that award's members.
	if resubmitAwardID == 0 {
		for _, candidate := range projectAwards {
			if candidate.Status != "rejected" || candidate.RegID == 0 {
				continue
			}
			var memberCount int64
			if err := tx.Model(&models.AwardMember{}).Where("award_id = ? AND student_id = ?", candidate.ID, leaderID).Count(&memberCount).Error; err != nil {
				utils.InternalServerError(c, "查询获奖成员失败", err)
				return
			}
			if memberCount == 0 {
				continue
			}
			if err := tx.First(&existingReg, candidate.RegID).Error; err != nil {
				utils.InternalServerError(c, "查询原申报报名失败", err)
				return
			}
			if existingReg.CompID != req.CompID {
				utils.BadRequest(c, "原申报赛事与当前赛事不一致")
				return
			}
			existingRegFound = true
			regID = existingReg.ID
			existingAward = candidate
			resubmitAwardID = candidate.ID
			break
		}
	}

	submittedMembers := make([]models.AwardMember, 0, len(req.Members))
	includesApplicant := false
	for _, member := range req.Members {
		var user models.User
		if err := tx.Where("username = ?", member.StudentID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				utils.BadRequest(c, "参赛成员学号不存在，请重新选择成员")
				return
			}
			utils.InternalServerError(c, "查询申报队员失败", err)
			return
		}
		if user.ID == leaderID {
			includesApplicant = true
		}
		submittedMembers = append(submittedMembers, models.AwardMember{
			StudentID: user.ID, Name: user.Realname, StudentNumber: user.Username,
			College: user.College, Major: user.Major, Phone: member.Phone,
			Email: member.Email, IsLeader: member.IsLeader, Remark: req.Remark,
		})
	}
	if !includesApplicant {
		utils.BadRequest(c, "申报人必须是参赛成员，个人赛只能申报本人")
		return
	}

	allMemberUsernames := append([]string(nil), memberUsernames...)
	if existingRegFound {
		var registeredMemberUsernames []string
		if err := tx.Model(&models.RegMember{}).Where("reg_id = ?", regID).Pluck("username", &registeredMemberUsernames).Error; err != nil {
			utils.InternalServerError(c, "查询报名队员失败", err)
			return
		}
		allMemberUsernames = append(allMemberUsernames, registeredMemberUsernames...)
	}
	allMemberUsernames = append(allMemberUsernames, authenticatedUser.Username)
	uniqueMemberUsernames := make([]string, 0, len(allMemberUsernames))
	seenAllMemberUsernames := make(map[string]bool, len(allMemberUsernames))
	for _, username := range allMemberUsernames {
		if username != "" && !seenAllMemberUsernames[username] {
			seenAllMemberUsernames[username] = true
			uniqueMemberUsernames = append(uniqueMemberUsernames, username)
		}
	}
	var participantUsers []models.User
	if err := tx.Select("id", "username").Where("username IN ?", uniqueMemberUsernames).Find(&participantUsers).Error; err != nil {
		utils.InternalServerError(c, "查询申报队员失败", err)
		return
	}
	participantIDs := make(map[uint]bool, len(participantUsers))
	for _, user := range participantUsers {
		participantIDs[user.ID] = true
	}
	participantIDs[leaderID] = true
	for memberID := range participantIDs {
		count, err := countOtherAwardMemberships(tx, req.CompID, memberID, 0, resubmitAwardID)
		if err != nil {
			utils.InternalServerError(c, "查询赛事获奖记录失败", err)
			return
		}
		if count > 0 {
			utils.BadRequest(c, "记录已存在：该学生在本赛事已有获奖记录；如需更正，请联系管理员修改原记录")
			return
		}
	}

	for _, projectAward := range projectAwards {
		if projectAward.ID != resubmitAwardID {
			utils.BadRequest(c, "该赛事已存在同名项目奖项，不能自动合并；如需更正，请联系管理员")
			return
		}
	}

	// Linking a file must not grant access to another student's private proof.
	if err := validateStudentAwardProof(tx, leaderID, req.ProofURL); err != nil {
		utils.BadRequest(c, "请使用本人上传或本人获奖项目已有的证明文件")
		return
	}
	if !existingRegFound {
		now := time.Now()
		existingReg = models.Register{
			CompID: req.CompID, LeaderID: leaderID, TeamName: req.TeamName,
			Status: 3, SupplementTime: &now, AdvisorInfo: "{}",
		}
		if err := tx.Create(&existingReg).Error; err != nil {
			utils.InternalServerError(c, "创建补录报名失败", err)
			return
		}
		regID = existingReg.ID
		for _, member := range req.Members {
			regMember := models.RegMember{
				RegID: existingReg.ID, Name: member.Name, StudentID: member.StudentID,
				Phone: member.Phone, Email: member.Email, College: member.College,
				IsLeader: member.IsLeader, Year: member.Year,
			}
			if err := tx.Create(&regMember).Error; err != nil {
				utils.InternalServerError(c, fmt.Sprintf("存储队员[%s]失败", member.Name), err)
				return
			}
		}
	}

	var awardTime *time.Time
	if req.AwardDate.Year() > 1 {
		awardTime = &req.AwardDate.Time
	}
	if resubmitAwardID != 0 {
		updates := map[string]interface{}{
			"award_category": req.AwardCategory, "participation_type": req.ParticipationType,
			"advisor_info": string(advisorJSON), "remark": req.Remark,
			"auditor_id": nil, "audit_time": nil,
			"project_name": req.TeamName, "project_key": projectKey,
			"award_time": awardTime, "award_level": req.AwardLevel,
			"award_name": req.AwardName, "status": "draft",
			"proof_url": req.ProofURL, "reject_reason": "",
		}
		update := tx.Model(&models.Award{}).Where("id = ? AND status = ?", resubmitAwardID, "rejected")
		if req.UpdatedAt != nil {
			update = update.Where("update_time = ?", *req.UpdatedAt)
		}
		result := update.Updates(updates)
		if result.Error != nil {
			utils.InternalServerError(c, "重新提交失败", result.Error)
			return
		}
		if result.RowsAffected == 0 {
			awardEditConflict(c)
			return
		}
		if err := saveStudentAwardAdvisors(tx, resubmitAwardID, resolvedAdvisors); err != nil {
			utils.InternalServerError(c, "保存指导教师失败", err)
			return
		}
		if err := saveStudentAwardMembers(tx, resubmitAwardID, regID, submittedMembers); err != nil {
			utils.InternalServerError(c, "保存获奖成员失败", err)
			return
		}
		if err := tx.Model(&models.Register{}).Where("id = ?", regID).Update("status", 3).Error; err != nil {
			utils.InternalServerError(c, "更新报名状态失败", err)
			return
		}
		if err := tx.Commit().Error; err != nil {
			utils.InternalServerError(c, "事务提交失败", err)
			return
		}
		clearAwardCache(c.Request.Context(), req.CompID)
		utils.SuccessWithMessage(c, "重新申报成功，待审核", gin.H{"award_id": resubmitAwardID, "reg_id": regID})
		return
	}

	award := models.Award{
		ParticipationType: req.ParticipationType, AwardCategory: req.AwardCategory,
		AdvisorInfo: string(advisorJSON), Remark: req.Remark,
		ProjectName: req.TeamName, ProjectKey: &projectKey,
		CompID: req.CompID, RegID: regID,
		AwardLevel: req.AwardLevel, AwardName: req.AwardName,
		AwardTime: awardTime, Status: "draft",
		ProofUrl: req.ProofURL, Source: "supplement", LevelRank: 99,
	}
	if err := tx.Create(&award).Error; err != nil {
		utils.InternalServerError(c, "创建补录奖项失败", err)
		return
	}
	if err := saveStudentAwardAdvisors(tx, award.ID, resolvedAdvisors); err != nil {
		utils.InternalServerError(c, "保存指导教师失败", err)
		return
	}
	if err := saveStudentAwardMembers(tx, award.ID, regID, submittedMembers); err != nil {
		utils.InternalServerError(c, "保存获奖成员失败", err)
		return
	}
	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "补录申报失败，请重试", err)
		return
	}

	clearAwardCache(c.Request.Context(), req.CompID)
	utils.SuccessWithMessage(c, "补录申报成功，待审核", gin.H{"award_id": award.ID, "reg_id": regID})
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
			for _, m := range a.Members {
				if m.IsLeader {
					leaderName, leaderID, phone = m.Name, m.StudentNumber, m.Phone
				}
			}
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
			UpdatedAt:   a.UpdatedAt,
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
			teammates = append(teammates, gin.H{"name": m.Name, "student_id": m.StudentNumber, "college": m.College, "phone": m.Phone, "email": m.Email, "is_leader": m.IsLeader})
			if m.IsLeader {
				leaderName, leaderID, college, phone, email = m.Name, m.StudentNumber, m.College, m.Phone, m.Email
			}
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
		UpdatedAt:         award.UpdatedAt,
		ID:                award.ID,
		StudentName:       leaderName,
		StudentID:         leaderID,
		College:           college,
		Phone:             phone,
		Email:             email,
		CompName:          award.Competition.CompName,
		AwardLevel:        award.AwardLevel,
		AwardSpecific:     award.AwardName,
		AwardDate:         awardDate,
		ParticipationType: award.ParticipationType, AwardCategory: award.AwardCategory,
		AdvisorInfo: award.AdvisorInfo, Remark: award.Remark,
		TeamName:     award.ProjectName,
		Teammates:    teammates,
		CertImage:    award.ProofUrl,
		SubmitTime:   submitTime,
		Status:       mapAwardStatusToInt(award.Status),
		RejectReason: award.RejectReason,
	}

	utils.Success(c, resp)
}

// Validate every requested record before any single or batch audit mutation.
func requireAwardAuditAccess(c *gin.Context, ids []uint) bool {
	// Real routes always set role_code through AuthRequired.
	if _, exists := c.Get("role_code"); !exists {
		return true
	}
	var awards []models.Award
	if err := database.DB.WithContext(c.Request.Context()).Where("id IN ?", ids).Find(&awards).Error; err != nil {
		utils.InternalServerError(c, "查询申报记录失败", err)
		return false
	}
	checked := make(map[uint]bool)
	for _, award := range awards {
		if !checked[award.CompID] {
			if _, _, ok := requireAwardCompetition(c, award.CompID); !ok {
				return false
			}
			checked[award.CompID] = true
		}
	}
	return true
}

func PassAwardAudit(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	var reqVersion struct {
		UpdatedAt *time.Time `json:"update_time"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&reqVersion); err != nil {
			utils.BadRequest(c, "参数错误")
			return
		}
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

	if !requireAwardAuditAccess(c, []uint{award.ID}) {
		return
	}
	if reqVersion.UpdatedAt != nil && !award.UpdatedAt.Equal(*reqVersion.UpdatedAt) {
		awardEditConflict(c)
		return
	}

	now := time.Now()
	tx := database.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		utils.InternalServerError(c, "开启审核事务失败", tx.Error)
		return
	}
	result := tx.Model(&award).Where("status = ? AND update_time = ?", "draft", award.UpdatedAt).Updates(map[string]interface{}{
		"status":        "approved",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": "",
	})
	if result.Error != nil {
		tx.Rollback()
		utils.InternalServerError(c, "审核更新失败", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		awardEditConflict(c)
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
		Reason    string     `json:"reason" binding:"required"`
		UpdatedAt *time.Time `json:"update_time"`
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

	if !requireAwardAuditAccess(c, []uint{award.ID}) {
		return
	}
	if req.UpdatedAt != nil && !award.UpdatedAt.Equal(*req.UpdatedAt) {
		awardEditConflict(c)
		return
	}

	now := time.Now()
	tx := database.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		utils.InternalServerError(c, "开启审核事务失败", tx.Error)
		return
	}
	result := tx.Model(&award).Where("status = ? AND update_time = ?", "draft", award.UpdatedAt).Updates(map[string]interface{}{
		"status":        "rejected",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": req.Reason,
	})
	if result.Error != nil {
		tx.Rollback()
		utils.InternalServerError(c, "审核更新失败", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		awardEditConflict(c)
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
		Versions map[uint]time.Time `json:"versions"`
		IDs      []uint             `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "ids 必填")
		return
	}
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	if !requireAwardAuditAccess(c, req.IDs) {
		return
	}

	now := time.Now()
	tx := database.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		utils.InternalServerError(c, "开启审核事务失败", tx.Error)
		return
	}
	if !requireAwardAuditVersions(c, tx, req.IDs, req.Versions) {
		return
	}
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
		Versions map[uint]time.Time `json:"versions"`
		IDs      []uint             `json:"ids" binding:"required"`
		Reason   string             `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "ids 和 reason 必填")
		return
	}
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	if !requireAwardAuditAccess(c, req.IDs) {
		return
	}

	now := time.Now()
	tx := database.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		utils.InternalServerError(c, "开启审核事务失败", tx.Error)
		return
	}
	if !requireAwardAuditVersions(c, tx, req.IDs, req.Versions) {
		return
	}
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

// Keep award participants separate from the original registration.
func saveStudentAwardMembers(tx *gorm.DB, awardID, regID uint, members []models.AwardMember) error {
	if err := tx.Unscoped().Where("award_id = ?", awardID).Delete(&models.AwardMember{}).Error; err != nil {
		return err
	}
	for i := range members {
		members[i].AwardID = awardID
		members[i].RegID = &regID
	}
	if err := tx.Create(&members).Error; err != nil {
		return err
	}
	return tx.Model(&models.Award{}).Where("id = ?", awardID).Update("members_migrated", true).Error
}

func validateStudentAwardDetails(kind, category, level string, date time.Time, members []models.RegMemberInfo, advisors []models.AdvisorInfo, remark string) error {
	if kind != "individual" && kind != "team" {
		return errors.New("请选择个人赛或团队赛")
	}
	if kind == "individual" && len(members) != 1 {
		return errors.New("个人赛只能填写本人信息")
	}
	if kind == "team" && len(members) < 2 {
		return errors.New("团队赛必须填写队长和至少一名团队成员")
	}
	if category != "国家级" && category != "省级" && category != "校级" {
		return errors.New("请选择有效的获奖类别")
	}
	if level != "一等奖" && level != "二等奖" && level != "三等奖" {
		return errors.New("请选择有效的获奖等级")
	}
	if date.IsZero() {
		return errors.New("请选择获奖时间")
	}
	if len([]rune(remark)) > 2000 {
		return errors.New("备注不能超过2000字")
	}
	if len(advisors) == 0 {
		return errors.New("请填写指导老师")
	}
	for _, a := range advisors {
		if strings.TrimSpace(a.Name) == "" || len([]rune(a.Name)) > 100 || len([]rune(a.College)) > 255 {
			return errors.New("指导老师姓名不能为空且不能超过100字，单位不能超过255字")
		}
	}
	seen := map[string]bool{}
	for _, m := range members {
		if seen[m.StudentID] {
			return errors.New("参赛成员不能重复")
		}
		seen[m.StudentID] = true
	}
	return nil
}
