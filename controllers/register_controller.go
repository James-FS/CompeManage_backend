package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const MaxPageSize = 100

type ConfigReq struct {
	CompID           uint       `json:"comp_id" binding:"required"`
	ParticipantType  int8       `json:"participant_type"`
	MinTeamMember    int        `json:"min_team_member"`
	MaxTeamMember    int        `json:"max_team_member"`
	RegStartTime     *time.Time `json:"reg_start_time"`
	RegEndTime       *time.Time `json:"reg_end_time"`
	SubmitStartTime  *time.Time `json:"submit_start_time"`
	SubmitEndTime    *time.Time `json:"submit_end_time"`
	GradeRequirement []int      `json:"grade_requirement"`
	AwardHierarchy   []string   `json:"award_hierarchy"`
	NeedAdvisor      int        `json:"need_advisor"`
	NeedAttachment   int        `json:"need_attachment"`
	NeedRegAudit     *int       `json:"need_reg_audit"`
	Track            []TrackReq `json:"track"`
}

type TrackReq struct {
	TrackName string        `json:"trackName"`
	SubTrack  []SubTrackReq `json:"subTrack"`
}

type SubTrackReq struct {
	Title string `json:"title"`
}

type ApplicationReq struct {
	CompID        uint                `json:"comp_id" binding:"required"`
	TeamName      string              `json:"team_name"`
	Leader        MemberReq           `json:"leader"`
	Members       []MemberReq         `json:"members"`
	AdvisorID     *uint               `json:"advisor_id"`
	AdvisorInfo   *models.AdvisorInfo `json:"advisor_info"`
	AttachmentUrl string              `json:"attachment_url"`
	Track         string              `json:"track"`
}

type MemberReq struct {
	Name    string `json:"name" binding:"required"`
	StuID   string `json:"stuID" binding:"required"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	College string `json:"college"`
}

type AuditListResp struct {
	ID            uint   `json:"id"`
	CompID        uint   `json:"comp_id"`
	CompName      string `json:"comp_name"`
	TeamName      string `json:"team_name"`
	LeaderName    string `json:"leader_name"`
	StuID         string `json:"stu_id"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	CreateTime    string `json:"create_time"`
	Status        int8   `json:"status"`
	AttachmentUrl string `json:"attachment_url"`
}

type SubmitWorkReq struct {
	RegID   uint   `json:"reg_id" binding:"required"`
	WorkUrl string `json:"work_attachment_url"`
}

type UserListReq struct {
	Page     int    `form:"page" binding:"required,min=1"`
	PageSize int    `form:"page_size" binding:"required,min=1,max=100"`
	Role     string `form:"role" binding:"required"`
	Search   string `form:"search"`
}

type UserListResp struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	College  string `json:"college"`
	Grade    string `json:"grade"`
}

func isValidTime(t time.Time) bool {
	return !t.IsZero() && t.Year() > 1970
}

func removeUploadedFile(fileUrl string) {
	if fileUrl == "" {
		return
	}
	relativePath := strings.TrimPrefix(fileUrl, "/")
	nativePath := filepath.FromSlash(relativePath)
	if err := os.Remove(nativePath); err != nil {
		fmt.Println("清理垃圾文件失败:", err)
	} else {
		fmt.Println("已清理垃圾文件:", nativePath)
	}
}

func SaveRegConfig(c *gin.Context) {
	var req ConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数格式错误")
		return
	}

	if req.RegStartTime == nil || req.RegEndTime == nil {
		utils.BadRequest(c, "报名起止时间不能为空")
		return
	}

	if (req.SubmitStartTime == nil) != (req.SubmitEndTime == nil) {
		utils.BadRequest(c, "作品提交时间必须同时填写开始和结束")
		return
	}

	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	userID := userIDVal.(uint)

	var comp models.CompDirectory
	db := database.DB.Model(&models.CompDirectory{}).Where("id = ?", req.CompID)

	if !checkUserIsAdmin(userID) {
		db = db.Where("manager_id = ?", userID)
	}

	if err := db.First(&comp).Error; err != nil {
		utils.Forbidden(c, "您无权操作此赛事或赛事不存在")
		return
	}

	gradeJson, err := json.Marshal(req.GradeRequirement)
	if err != nil {
		utils.InternalServerError(c, "年级数据处理失败", err)
		return
	}

	hierarchyJson, err := json.Marshal(req.AwardHierarchy)
	if err != nil {
		utils.InternalServerError(c, "奖项信息处理失败", err)
		return
	}

	trackJson, err := json.Marshal(req.Track)
	if err != nil {
		utils.InternalServerError(c, "赛道数据处理失败", err)
		return
	}

	var detail models.CompDetail
	err = database.DB.Where("comp_id = ?", req.CompID).First(&detail).Error

	detail.CompID = req.CompID
	detail.ParticipantType = req.ParticipantType
	detail.MinTeamMember = req.MinTeamMember
	detail.MaxTeamMember = req.MaxTeamMember
	detail.NeedAdvisor = req.NeedAdvisor
	detail.NeedAttachment = req.NeedAttachment
	needRegAudit := 1
	if detail.ID != 0 && (detail.NeedRegAudit == 0 || detail.NeedRegAudit == 1) {
		needRegAudit = detail.NeedRegAudit
	}
	if req.NeedRegAudit != nil {
		if *req.NeedRegAudit != 0 && *req.NeedRegAudit != 1 {
			utils.BadRequest(c, "need_reg_audit 仅支持 0 或 1")
			return
		}
		needRegAudit = *req.NeedRegAudit
	}
	detail.NeedRegAudit = needRegAudit
	detail.GradeRequirement = string(gradeJson)
	detail.AwardHierarchy = string(hierarchyJson)

	detail.Track = string(trackJson)

	// 时间字段判空处理 (防止空指针崩溃)

	if req.RegStartTime != nil {
		detail.RegStartTime = *req.RegStartTime
	}
	if req.RegEndTime != nil {
		detail.RegEndTime = *req.RegEndTime
	}

	if req.SubmitStartTime != nil {
		detail.SubmitStartTime = *req.SubmitStartTime
	} else {
		detail.SubmitStartTime = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	if req.SubmitEndTime != nil {
		detail.SubmitEndTime = *req.SubmitEndTime
	} else {
		detail.SubmitEndTime = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	detail.CompStartTime = *req.RegStartTime

	if req.SubmitEndTime != nil && req.SubmitEndTime.After(*req.RegEndTime) {
		detail.CompEndTime = *req.SubmitEndTime
	} else {
		detail.CompEndTime = *req.RegEndTime
	}

	if err == gorm.ErrRecordNotFound {
		if err := database.DB.Create(&detail).Error; err != nil {
			utils.InternalServerError(c, "创建配置失败", err)
			return
		}
	} else {
		if err := database.DB.Save(&detail).Error; err != nil {
			utils.InternalServerError(c, "更新配置失败", err)
			return
		}
	}

	// 首次创建时 NeedRegAudit=0 可能被数据库默认值(default:1)覆盖，强制回写以保证配置立即生效。
	if err := database.DB.Model(&models.CompDetail{}).
		Where("comp_id = ?", req.CompID).
		Update("need_reg_audit", needRegAudit).Error; err != nil {
		utils.InternalServerError(c, "更新审核开关失败", err)
		return
	}
	detail.NeedRegAudit = needRegAudit

	if detail.NeedRegAudit == 0 {
		if err := database.DB.Model(&models.Register{}).
			Where("comp_id = ? AND status IN ?", req.CompID, []int{0, 3}).
			Update("status", gorm.Expr("CASE WHEN status = 3 THEN 4 ELSE 1 END")).Error; err != nil {
			utils.InternalServerError(c, "更新报名审核状态失败", err)
			return
		}
	}

	now := time.Now()
	newStatus := 0
	if !detail.RegStartTime.After(now) {
		newStatus = 1
	}
	if detail.CompEndTime.Before(now) {
		newStatus = 2
	}

	if err := database.DB.Model(&models.CompDirectory{}).
		Where("id = ?", req.CompID).
		Update("status", newStatus).Error; err != nil {
		utils.InternalServerError(c, "更新赛事状态失败", err)
		return
	}

	clearCompListCache()

	utils.SuccessWithMessage(c, "报名设置保存成功", detail)
}

func GetRegConfig(c *gin.Context) {
	compID := c.Query("comp_id")
	if compID == "" {
		utils.BadRequest(c, "缺少 comp_id 参数")
		return
	}

	id, err := strconv.ParseUint(compID, 10, 32)
	if err != nil {
		utils.BadRequest(c, "comp_id 参数格式错误，必须是正整数")
		return
	}

	var comp models.CompDirectory

	if err := database.DB.Preload("Detail").First(&comp, id).Error; err != nil {
		utils.NotFound(c, "赛事不存在")
		return
	}

	if comp.Detail.ID == 0 {
		utils.Success(c, gin.H{"comp_name": comp.CompName})
		return
	}

	detail := comp.Detail

	var grades []int
	if detail.GradeRequirement != "" {
		_ = json.Unmarshal([]byte(detail.GradeRequirement), &grades)
	} else {
		grades = []int{}
	}

	var awardHierarchy []string
	if detail.AwardHierarchy != "" {
		_ = json.Unmarshal([]byte(detail.AwardHierarchy), &awardHierarchy)
	} else {
		awardHierarchy = []string{}
	}

	var track []TrackReq
	if detail.Track != "" {
		if err := json.Unmarshal([]byte(detail.Track), &track); err != nil {
			track = []TrackReq{}
		}
	} else {
		track = []TrackReq{}
	}

	var regStartTime, regEndTime, submitStartTime, submitEndTime *time.Time
	if !detail.RegStartTime.IsZero() {
		regStartTime = &detail.RegStartTime
	}
	if !detail.RegEndTime.IsZero() {
		regEndTime = &detail.RegEndTime
	}

	if isValidTime(detail.SubmitStartTime) {
		submitStartTime = &detail.SubmitStartTime
	}
	if isValidTime(detail.SubmitEndTime) {
		submitEndTime = &detail.SubmitEndTime
	}

	utils.Success(c, gin.H{
		"comp_name":         comp.CompName,
		"participant_type":  detail.ParticipantType,
		"min_team_member":   detail.MinTeamMember,
		"max_team_member":   detail.MaxTeamMember,
		"grade_requirement": grades,
		"need_advisor":      detail.NeedAdvisor,
		"need_attachment":   detail.NeedAttachment,
		"need_reg_audit":    detail.NeedRegAudit,
		"reg_start_time":    regStartTime,
		"reg_end_time":      regEndTime,
		"submit_start_time": submitStartTime,
		"submit_end_time":   submitEndTime,
		"award_hierarchy":   awardHierarchy,
		"track":             track,
	})
}

func SubmitRegistration(c *gin.Context) {
	var req ApplicationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数格式错误")
		return
	}

	_, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}

	var comp models.CompDirectory

	if err := database.DB.Preload("Detail").First(&comp, req.CompID).Error; err != nil {
		utils.NotFound(c, "赛事不存在")
		return
	}

	if comp.Detail.ID == 0 {
		removeUploadedFile(req.AttachmentUrl)
		utils.BadRequest(c, "赛事配置未完成，暂不接受报名")
		return
	}

	now := time.Now()

	if now.Before(comp.Detail.RegStartTime) {
		removeUploadedFile(req.AttachmentUrl)
		utils.Forbidden(c, "非法请求：报名尚未开始")
		return
	}

	if now.After(comp.Detail.RegEndTime) {
		removeUploadedFile(req.AttachmentUrl)
		utils.Forbidden(c, "非法请求：报名已截止")
		return
	}

	if comp.Detail.NeedAttachment == 2 && req.AttachmentUrl == "" {
		utils.BadRequest(c, "该赛事要求必须上传报名附件/项目文档")
		return
	}

	if comp.Detail.NeedAdvisor == 2 && req.AdvisorInfo == nil {
		removeUploadedFile(req.AttachmentUrl)
		utils.BadRequest(c, "该赛事要求必须填写指导老师")
		return
	}

	var leader models.User
	if err := database.DB.Where("username = ?", req.Leader.StuID).First(&leader).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			removeUploadedFile(req.AttachmentUrl)
			utils.BadRequest(c, "负责人学号不存在")
			return
		}
		removeUploadedFile(req.AttachmentUrl)
		utils.InternalServerError(c, "查询负责人信息失败", err)
		return
	}

	register := models.Register{
		CompID:        req.CompID,
		LeaderID:      leader.ID,
		TeamName:      req.TeamName,
		AttachmentUrl: req.AttachmentUrl,
		Status:        0,
		Members:       make([]models.RegMember, 0),
		Track:         req.Track,
	}
	if comp.Detail.NeedRegAudit == 0 {
		register.Status = 1
	}

	if req.AdvisorInfo != nil {
		advisorJSON, err := json.Marshal(req.AdvisorInfo)
		if err != nil {
			utils.InternalServerError(c, "指导老师信息处理失败", err)
			return
		}
		register.AdvisorInfo = string(advisorJSON)
	}

	leaderMember := models.RegMember{
		Name:      req.Leader.Name,
		StudentID: req.Leader.StuID,
		Phone:     req.Leader.Phone,
		Email:     req.Leader.Email,
		College:   req.Leader.College,
		IsLeader:  true,
	}
	register.Members = append(register.Members, leaderMember)

	for _, m := range req.Members {
		register.Members = append(register.Members, models.RegMember{
			Name:      m.Name,
			StudentID: m.StuID,
			Phone:     m.Phone,
			Email:     m.Email,
			College:   m.College,
			IsLeader:  false,
		})
	}

	if err := database.DB.Create(&register).Error; err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "duplicate") || strings.Contains(errStr, "unique") {
			utils.Error(c, 409, 409, "您已报名过该赛事，请勿重复提交")
			return
		}
		utils.InternalServerError(c, "报名失败", err)
		return
	}

	utils.SuccessWithMessage(c, "报名提交成功", nil)
}

func checkUserIsAdmin(userID uint) bool {
	var count int64

	err := database.DB.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.role_code LIKE ?", userID, "%admin%").
		Count(&count).Error

	if err != nil {
		return false
	}

	return count > 0
}

func GetRegList(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("size", "10"))
	if err != nil || pageSize < 1 {
		pageSize = 10
	}

	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	keyword := c.Query("keyword")
	email := c.Query("email")
	phone := c.Query("phone")
	status := c.Query("status")
	compName := c.Query("comp_name")
	pType := c.Query("participant_type")

	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	userID := userIDVal.(uint)

	query := database.DB.Model(&models.Register{}).
		Joins("LEFT JOIN comp_directories ON comp_directories.id = registers.comp_id").
		Joins("LEFT JOIN comp_details ON comp_details.comp_id = registers.comp_id").
		Preload("Competition").
		Preload("Competition.Detail").
		Preload("Leader").
		Preload("Members")

	if !checkUserIsAdmin(userID) {
		query = query.Where("comp_directories.manager_id = ?", userID)
	}

	if compName != "" {
		query = query.Where("comp_directories.comp_name LIKE ?", "%"+compName+"%")
	}

	if status != "" {
		query = query.Where("registers.status = ?", status)
	}

	if pType != "" {
		query = query.Where("comp_details.participant_type = ?", pType)
	}

	needUserJoin := keyword != "" || email != ""
	if needUserJoin {
		query = query.Joins("LEFT JOIN users ON users.id = registers.leader_id")
	}

	if keyword != "" {
		query = query.Where(
			"registers.team_name LIKE ? OR users.realname LIKE ? OR users.username LIKE ?",
			"%"+keyword+"%",
			"%"+keyword+"%",
			"%"+keyword+"%",
		)
	}

	if email != "" {
		query = query.Where(
			"EXISTS (SELECT 1 FROM reg_members WHERE reg_members.reg_id = registers.id AND reg_members.email LIKE ? AND reg_members.is_leader = ?)",
			"%"+email+"%",
			true,
		)
	}

	if phone != "" {
		query = query.Where(
			"EXISTS (SELECT 1 FROM reg_members WHERE reg_members.reg_id = registers.id AND reg_members.phone LIKE ? AND reg_members.is_leader = ?)",
			"%"+phone+"%",
			true,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	var list []models.Register
	offset := (page - 1) * pageSize

	if err := query.Order("registers.create_time desc").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		utils.InternalServerError(c, "获取数据失败", err)
		return
	}

	type AuditListResp struct {
		ID            uint               `json:"id"`
		CompID        uint               `json:"comp_id"`
		CompName      string             `json:"comp_name"`
		TeamName      string             `json:"team_name"`
		LeaderName    string             `json:"leader_name"`
		StuID         string             `json:"stu_id"`
		Email         string             `json:"email"`
		Phone         string             `json:"phone"`
		CreateTime    string             `json:"create_time"`
		Status        int8               `json:"status"`
		AttachmentUrl string             `json:"attachment_url"`
		Members       []models.RegMember `json:"members"`
		AdvisorInfo   string             `json:"advisor_info"`
	}

	var respList []AuditListResp
	for _, item := range list {
		leaderName := "未知"
		stuID := ""
		leaderEmail := ""
		leaderPhone := ""

		for _, m := range item.Members {
			if m.IsLeader {
				leaderEmail = m.Email
				leaderPhone = m.Phone

				if leaderName == "未知" || leaderName == "" {
					leaderName = m.Name
				}
				break
			}
		}

		respList = append(respList, AuditListResp{
			ID:         item.ID,
			CompID:     item.CompID,
			CompName:   item.Competition.CompName,
			TeamName:   item.TeamName,
			LeaderName: leaderName,
			StuID:      stuID,
			Email:      leaderEmail,
			Phone:      leaderPhone,
			CreateTime: item.CreatedAt.Format("2006-01-02 15:04"),
			Status: func() int8 {
				if item.Status == 0 && item.Competition.Detail.NeedRegAudit == 0 {
					return 1
				}
				if item.Status == 3 && item.Competition.Detail.NeedRegAudit == 0 {
					return 4
				}
				return item.Status
			}(),
			AttachmentUrl: item.AttachmentUrl,
			Members:       item.Members,
			AdvisorInfo:   item.AdvisorInfo,
		})
	}

	utils.Success(c, gin.H{
		"list":  respList,
		"total": total,
	})
}

func GetRegDetail(c *gin.Context) {
	regID := c.Query("id")
	if regID == "" {
		utils.BadRequest(c, "缺少 id 参数")
		return
	}

	regid, err := strconv.Atoi(regID)
	if err != nil {
		utils.BadRequest(c, "id 参数格式错误，必须是正整数")
		return
	}

	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	var reg models.Register
	err = database.DB.
		Preload("Competition").
		Preload("Leader").
		Preload("Members").
		First(&reg, regid).Error

	if err != nil {
		utils.NotFound(c, "报名记录不存在")
		return
	}

	if !checkUserIsAdmin(userID) {
		if reg.Competition.ManagerID != userID {
			utils.Forbidden(c, "无权查看此记录")
			return
		}
	}

	leaderName := "未知"
	stuID := ""
	email := ""
	phone := ""

	if reg.Leader.ID != 0 {
		leaderName = reg.Leader.Realname
		if leaderName == "" {
			leaderName = reg.Leader.Username
		}
		stuID = reg.Leader.Username
	}

	for _, m := range reg.Members {
		if m.IsLeader {
			phone = m.Phone
			email = m.Email

			if reg.Leader.Realname == "" {
				leaderName = m.Name
			}
			break
		}
	}

	utils.Success(c, gin.H{
		"id":             reg.ID,
		"comp_name":      reg.Competition.CompName,
		"team_name":      reg.TeamName,
		"leader_name":    leaderName,
		"stu_id":         stuID,
		"phone":          phone,
		"email":          email,
		"update_time":    reg.UpdatedAt.Format("2006-01-02 15:04:05"),
		"status":         reg.Status,
		"attachment_url": reg.AttachmentUrl,
		"work_url":       reg.WorkAttachmentUrl,
		"members":        reg.Members,
		"advisor_info":   reg.AdvisorInfo,
		"reject_reason":  reg.RejectReason,
		"track":          reg.Track,
	})
}

func AuditRegister(c *gin.Context) {
	type AuditReq struct {
		ID     uint   `json:"id" binding:"required"`
		Status int8   `json:"status"`
		Reason string `json:"reason"`
	}
	var req AuditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	if req.Status != 1 && req.Status != 2 {
		utils.BadRequest(c, "非法的审核状态")
		return
	}

	if req.Status == 2 && req.Reason == "" {
		utils.BadRequest(c, "驳回时必须填写原因")
		return
	}

	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	var reg models.Register
	err := database.DB.Preload("Competition").Preload("Competition.Detail").First(&reg, req.ID).Error

	if err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}

	if reg.Competition.Detail.NeedRegAudit == 0 {
		if reg.Status == 0 {
			_ = database.DB.Model(&reg).Updates(map[string]interface{}{"status": 1, "reject_reason": ""}).Error
		}
		utils.BadRequest(c, "该赛事已设置为免审核，报名会自动通过")
		return
	}

	if reg.Status != 0 {
		utils.Error(c, 409, 409, "该记录已被审核，请勿重复操作")
		return
	}

	if !checkUserIsAdmin(userID) {
		if reg.Competition.ManagerID != userID {
			utils.Forbidden(c, "您无权审核此条记录")
			return
		}
	}

	updateMap := map[string]interface{}{
		"status": req.Status,
	}

	if req.Status == 2 {
		updateMap["reject_reason"] = req.Reason
	} else {
		updateMap["reject_reason"] = ""
	}

	if err := database.DB.Model(&reg).Updates(updateMap).Error; err != nil {
		utils.InternalServerError(c, "数据库更新失败", err)
		return
	}

	utils.SuccessWithMessage(c, "审核完成", nil)
}

func GetMyRegStatus(c *gin.Context) {
	compid := c.Query("comp_id")
	if compid == "" {
		utils.BadRequest(c, "缺少 comp_id 参数")
		return
	}

	compID, err := strconv.ParseUint(compid, 10, 32)
	if err != nil {
		utils.BadRequest(c, "comp_id 参数格式错误")
		return
	}

	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}

	userID, ok := userIDVal.(uint)
	if !ok {
		utils.InternalServerError(c, "用户信息异常", errors.New("用户ID类型错误"))
		return
	}

	var user models.User
	if err := database.DB.Select("username").First(&user, userID).Error; err != nil {
		utils.InternalServerError(c, "获取用户信息失败", err)
		return
	}

	studentID := user.Username

	var reg models.Register
	err = database.DB.
		Preload("Members").
		Preload("Competition").
		Preload("Competition.Detail").
		Preload("Leader").
		Joins("INNER JOIN reg_members ON reg_members.reg_id = registers.id").
		Where("registers.comp_id = ? AND reg_members.username = ?", compID, studentID).
		First(&reg).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.Success(c, nil)
		} else {
			utils.InternalServerError(c, "查询失败", err)
		}
		return
	}

	if reg.Status == 0 && reg.Competition.Detail.NeedRegAudit == 0 {
		reg.Status = 1
		_ = database.DB.Model(&models.Register{}).Where("id = ?", reg.ID).Update("status", 1).Error
	}

	type MemberResp struct {
		ID       uint   `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
		Phone    string `json:"phone"`
		Email    string `json:"email"`
		College  string `json:"college"`
		IsLeader bool   `json:"is_leader"`
	}

	var membersResp []MemberResp
	for _, m := range reg.Members {
		membersResp = append(membersResp, MemberResp{
			ID:       m.ID,
			Name:     m.Name,
			Username: m.StudentID,
			Phone:    m.Phone,
			Email:    m.Email,
			College:  m.College,
			IsLeader: m.IsLeader,
		})
	}

	utils.Success(c, gin.H{
		"id":             reg.ID,
		"team_name":      reg.TeamName,
		"status":         reg.Status,
		"reject_reason":  reg.RejectReason,
		"attachment_url": reg.AttachmentUrl,
		"members":        membersResp,
		"advisor_info":   reg.AdvisorInfo,
		"advisor_id":     reg.AdvisorID,
		"track":          reg.Track,
	})
}

func ResubmitRegistration(c *gin.Context) {
	var req ApplicationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}

	userID, ok := userIDVal.(uint)
	if !ok {
		utils.InternalServerError(c, "用户信息异常", errors.New("用户ID类型错误"))
		return
	}

	var reg models.Register
	if err := database.DB.Where("comp_id = ? AND leader_id = ?", req.CompID, userID).First(&reg).Error; err != nil {
		utils.NotFound(c, "未找到原报名记录")
		return
	}

	if reg.Status == 1 {
		utils.Forbidden(c, "审核已通过，无法修改信息")
		return
	}

	var detail models.CompDetail
	if err := database.DB.Where("comp_id = ?", req.CompID).First(&detail).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.BadRequest(c, "赛事配置不存在")
		} else {
			utils.InternalServerError(c, "查询赛事配置失败", err)
		}
		return
	}

	now := time.Now()
	if now.Before(detail.RegStartTime) {
		utils.BadRequest(c, "报名尚未开始")
		return
	}

	if now.After(detail.RegEndTime) {
		utils.BadRequest(c, "报名已截止，无法重新提交")
		return
	}

	if detail.NeedAttachment == 2 && req.AttachmentUrl == "" {
		utils.BadRequest(c, "该赛事要求必须上传报名附件，请勿删除附件")
		return
	}

	if detail.NeedAdvisor == 2 && req.AdvisorInfo == nil {
		utils.BadRequest(c, "该赛事要求必须填写指导老师")
		return
	}

	tx := database.DB.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			fmt.Println("事务发生回滚 ", r)
		}
	}()

	reg.TeamName = req.TeamName
	reg.AttachmentUrl = req.AttachmentUrl
	reg.AdvisorID = req.AdvisorID
	reg.Track = req.Track

	if req.AdvisorInfo != nil {
		advisorJSON, err := json.Marshal(req.AdvisorInfo)
		if err != nil {
			utils.InternalServerError(c, "指导老师信息处理失败", err)
			return
		}
		reg.AdvisorInfo = string(advisorJSON)
	}

	reg.Status = 0
	if detail.NeedRegAudit == 0 {
		reg.Status = 1
	}
	reg.RejectReason = ""

	if err := tx.Save(&reg).Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "更新失败", err)
		return
	}

	if err := tx.Where("reg_id = ?", reg.ID).Delete(&models.RegMember{}).Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "清理旧成员失败", err)
		return
	}

	var newMembers []models.RegMember
	newMembers = append(newMembers, models.RegMember{
		RegID:     reg.ID,
		Name:      req.Leader.Name,
		StudentID: req.Leader.StuID,
		Phone:     req.Leader.Phone,
		Email:     req.Leader.Email,
		College:   req.Leader.College,
		IsLeader:  true,
	})

	for _, m := range req.Members {
		newMembers = append(newMembers, models.RegMember{
			RegID:     reg.ID,
			Name:      m.Name,
			StudentID: m.StuID,
			Phone:     m.Phone,
			Email:     m.Email,
			College:   m.College,
			IsLeader:  false,
		})
	}

	if err := tx.Create(&newMembers).Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "保存成员失败", err)
		return
	}

	if err := tx.Commit().Error; err != nil {
		fmt.Println("事务提交失败:", err)
		utils.InternalServerError(c, "提交失败，请重试", err)
		return
	}

	utils.SuccessWithMessage(c, "重新提交成功", nil)
}

func GetMyRegList(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未登录")
		return
	}
	userID := userIDVal.(uint)

	var user models.User
	if err := database.DB.Select("username").First(&user, userID).Error; err != nil {
		utils.InternalServerError(c, "获取用户信息失败", err)
		return
	}
	studentID := user.Username

	targetRegID := c.Query("id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	offset := (page - 1) * pageSize

	var regs []models.Register
	var total int64

	subQuery := database.DB.Model(&models.RegMember{}).
		Select("reg_id").
		Where("username = ?", studentID)

	query := database.DB.Model(&models.Register{}).
		Where("id IN (?)", subQuery).
		Preload("Competition").
		Preload("Competition.Detail")

	if targetRegID != "" {
		query = query.Where("id = ?", targetRegID)
	}

	query.Count(&total)

	if err := query.Order("create_time desc").Offset(offset).Limit(pageSize).Find(&regs).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	var list []gin.H
	for _, r := range regs {
		compName := "未知赛事"
		submitStartStr := ""
		submitEndStr := ""

		if r.Competition.ID != 0 {
			compName = r.Competition.CompName
			if r.Competition.Detail.ID != 0 {
				if isValidTime(r.Competition.Detail.SubmitStartTime) {
					submitStartStr = r.Competition.Detail.SubmitStartTime.Format("2006-01-02 15:04:05")
				}
				if isValidTime(r.Competition.Detail.SubmitEndTime) {
					submitEndStr = r.Competition.Detail.SubmitEndTime.Format("2006-01-02 15:04:05")
				}
			}
		}

		regAttachment := ""
		if targetRegID != "" {
			regAttachment = r.AttachmentUrl
		}

		list = append(list, gin.H{
			"id":        r.ID,
			"comp_id":   r.CompID,
			"comp_name": compName,
			"status": func() int8 {
				if r.Status == 0 && r.Competition.Detail.NeedRegAudit == 0 {
					return 1
				}
				if r.Status == 3 && r.Competition.Detail.NeedRegAudit == 0 {
					return 4
				}
				return r.Status
			}(),
			"reg_url":           regAttachment,
			"work_url":          r.WorkAttachmentUrl,
			"submit_start_time": submitStartStr,
			"submit_end_time":   submitEndStr,
		})
	}

	utils.Success(c, gin.H{
		"list":  list,
		"total": total,
	})
}

func SubmitWork(c *gin.Context) {
	var req SubmitWorkReq
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

	var user models.User
	if err := database.DB.Select("username").First(&user, userID).Error; err != nil {
		utils.InternalServerError(c, "获取用户信息失败", err)
		return
	}
	currentStuID := user.Username

	var reg models.Register
	if err := database.DB.Preload("Competition.Detail").First(&reg, req.RegID).Error; err != nil {
		utils.NotFound(c, "报名记录不存在")
		return
	}

	var count int64
	database.DB.Model(&models.RegMember{}).
		Where("reg_id = ? AND username = ?", req.RegID, currentStuID).
		Count(&count)

	if count == 0 {
		utils.Forbidden(c, "无权操作：您不是该团队的成员")
		return
	}

	if reg.Status == 0 && reg.Competition.Detail.NeedRegAudit == 0 {
		reg.Status = 1
		_ = database.DB.Model(&reg).Update("status", 1).Error
	}

	if reg.Status != 1 && reg.Status != 4 {
		utils.BadRequest(c, "您的报名未通过审核，无法提交作品")
		return
	}

	now := time.Now()
	detail := reg.Competition.Detail

	if !isValidTime(detail.SubmitStartTime) || !isValidTime(detail.SubmitEndTime) {
		utils.Forbidden(c, "该赛事暂未开放作品提交")
		return
	}

	if now.Before(detail.SubmitStartTime) {
		utils.Forbidden(c, "作品提交通道尚未开启")
		return
	}

	if now.After(detail.SubmitEndTime) {
		utils.Forbidden(c, "作品提交已截止")
		return
	}

	if err := database.DB.Model(&reg).Update("work_attachment_url", req.WorkUrl).Error; err != nil {
		utils.InternalServerError(c, "保存失败", err)
		return
	}

	utils.SuccessWithMessage(c, "作品已成功保存", nil)
}

func GetUserList(c *gin.Context) {
	var req UserListReq

	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	if req.PageSize > 50 {
		req.PageSize = 50
	}

	query := database.DB.Model(&models.User{}).
		Joins("LEFT JOIN user_roles ON user_roles.user_id = users.id").
		Joins("LEFT JOIN roles ON roles.id = user_roles.role_id").
		Where("roles.role_code = ?", req.Role).
		Select("users.id, users.realname, users.username, users.college, users.grade")

	if req.Search != "" {
		query = query.Where(
			"users.realname LIKE ? OR users.username LIKE ?",
			"%"+req.Search+"%",
			"%"+req.Search+"%",
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "统计用户数失败", err)
		return
	}

	var users []models.User
	offset := (req.Page - 1) * req.PageSize

	if err := query.
		Order("users.create_time DESC").
		Offset(offset).
		Limit(req.PageSize).
		Find(&users).Error; err != nil {
		utils.InternalServerError(c, "查询用户列表失败", err)
		return
	}

	var respList []UserListResp
	for _, user := range users {
		respList = append(respList, UserListResp{
			ID:       user.ID,
			Name:     user.Realname,
			Username: user.Username,
			College:  user.College,
			Grade:    user.Grade,
		})
	}

	utils.Success(c, gin.H{
		"list":      respList,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}
