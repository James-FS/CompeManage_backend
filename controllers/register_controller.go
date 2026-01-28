package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ConfigReq 前端提交的数据结构
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
	NeedAdvisor      int        `json:"need_advisor"`
	NeedAttachment   int        `json:"need_attachment"`
}

// ApplicationReq 学生提交的报名数据
type ApplicationReq struct {
	CompID        uint        `json:"comp_id" binding:"required"`
	TeamName      string      `json:"team_name"` // 队伍名称
	Leader        MemberReq   `json:"leader"`
	Members       []MemberReq `json:"members"` // 队员列表 (不包含队长)
	AdvisorID     *uint       `json:"advisor_id"`
	AttachmentUrl string      `json:"attachment_url"` // 附件地址
}

// MemberReq 队员信息子结构
type MemberReq struct {
	Name    string `json:"name" binding:"required"`
	StuID   string `json:"stuID" binding:"required"` // 学号
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	College string `json:"college"`
}

// AuditListResp 用于前端审核列表的行数据
type AuditListResp struct {
	ID            uint   `json:"id"`
	CompID        uint   `json:"comp_id"`
	CompName      string `json:"comp_name"`
	TeamName      string `json:"team_name"`
	LeaderName    string `json:"leader_name"`
	StuID         string `json:"stu_id"`
	Email         string `json:"email"` // 你的前端新增了邮箱筛选
	Phone         string `json:"phone"`
	CreateTime    string `json:"create_time"`
	Status        int8   `json:"status"`
	AttachmentUrl string `json:"attachment_url"`
}

type SubmitWorkReq struct {
	RegID   uint   `json:"reg_id" binding:"required"` // 报名ID
	WorkUrl string `json:"work_attachment_url"`       // 作品文件的URL字符串 (逗号分隔)
}

// removeUploadedFile 根据前端传来的 URL 删除本地文件
// 例如 url: "/static/reg_attachments/xxx.pdf" -> 删除 "./static/reg_attachments/xxx.pdf"
func removeUploadedFile(fileUrl string) {
	if fileUrl == "" {
		return
	}
	// 去掉 URL 开头的 "/" (变为相对路径 static/...)
	relativePath := strings.TrimPrefix(fileUrl, "/")
	// 适配操作系统路径分隔符 (Windows用 \, Linux用 /)
	nativePath := filepath.FromSlash(relativePath)
	if err := os.Remove(nativePath); err != nil {
		fmt.Println("清理垃圾文件失败:", err)
	} else {
		fmt.Println("已清理垃圾文件:", nativePath)
	}
}

// SaveRegConfig 保存逻辑
func SaveRegConfig(c *gin.Context) {
	var req ConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数格式错误", "error": err.Error()})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未登录"})
		return
	}

	var comp models.CompDirectory
	// 查询竞赛是否存在且负责人ID是否匹配
	if err := database.DB.Where("id = ? AND manager_id = ?", req.CompID, userID).First(&comp).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "您无权操作此赛事"})
		return
	}

	gradeJson, err := json.Marshal(req.GradeRequirement)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "年级数据处理失败"})
		return
	}

	// 4. 查询 Detail 表中是否已存在记录 (Upsert 逻辑)
	var detail models.CompDetail
	err = database.DB.Where("comp_id = ?", req.CompID).First(&detail).Error

	// 无论新建还是更新，都统一设置这些字段
	detail.CompID = req.CompID
	detail.ParticipantType = req.ParticipantType
	detail.MinTeamMember = req.MinTeamMember
	detail.MaxTeamMember = req.MaxTeamMember
	detail.NeedAdvisor = req.NeedAdvisor
	detail.NeedAttachment = req.NeedAttachment
	detail.GradeRequirement = string(gradeJson) // 存入转换后的字符串

	// 时间字段判空处理 (防止空指针崩溃)
	if req.RegStartTime != nil {
		detail.RegStartTime = *req.RegStartTime
	}
	if req.RegEndTime != nil {
		detail.RegEndTime = *req.RegEndTime
	}

	if req.SubmitStartTime != nil {
		detail.SubmitStartTime = *req.SubmitStartTime
	}

	if req.SubmitEndTime != nil {
		detail.SubmitEndTime = *req.SubmitEndTime
	}
	//  执行数据库操作
	if err == gorm.ErrRecordNotFound {
		// 情况 A: 记录不存在 -> 创建 (Create)
		if err := database.DB.Create(&detail).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建配置失败", "error": err.Error()})
			return
		}
	} else {
		// 情况 B: 记录已存在 -> 更新 (Save)
		// 注意：这里必须用 Save 而不能用 Updates，因为 Updates 默认会忽略零值 (如 false, 0)
		// 我们需要能够把 NeedAdvisor 更新为 0 (无需指导老师)
		if err := database.DB.Save(&detail).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新配置失败", "error": err.Error()})
			return
		}
	}

	// 7. 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "报名设置保存成功",
		"data": detail,
	})
}

// GetRegConfig 获取报名设置 (用于页面回显)
func GetRegConfig(c *gin.Context) {
	// 1. 获取参数
	compID := c.Query("comp_id")
	if compID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "缺少 comp_id 参数"})
		return
	}

	// 2. 查询数据库
	var comp models.CompDirectory

	// 1. 查询赛事主表，同时预加载 Detail
	if err := database.DB.Preload("Detail").First(&comp, compID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "赛事不存在"})
		return
	}

	// 检查 Detail 是否存在 (即是否已经保存过配置)
	// 如果 Detail.ID 为 0，说明还没配置过 (数据库里没这条记录)
	if comp.Detail.ID == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code": 200,
			"msg":  "暂无配置",
			"data": gin.H{
				"comp_name": comp.CompName,
			},
		})
		return
	}

	detail := comp.Detail // 取出预加载好的 Detail

	// 年级 JSON 字符串 -> int 数组
	var grades []int
	if detail.GradeRequirement != "" {
		// 忽略错误，如果解析失败就给空数组
		_ = json.Unmarshal([]byte(detail.GradeRequirement), &grades)
	} else {
		grades = []int{}
	}

	// 4. 数据处理：时间零值处理 (Go 的 0001-01-01 给前端会显示乱码)
	var regStartTime, regEndTime, submitStartTime, submitEndTime *time.Time
	if !detail.RegStartTime.IsZero() {
		regStartTime = &detail.RegStartTime
	}
	if !detail.RegEndTime.IsZero() {
		regEndTime = &detail.RegEndTime
	}

	if !detail.RegStartTime.IsZero() {
		submitStartTime = &detail.SubmitStartTime
	}
	if !detail.RegEndTime.IsZero() {
		submitEndTime = &detail.SubmitEndTime
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "获取成功",
		"data": gin.H{
			"comp_name":         comp.CompName,
			"participant_type":  detail.ParticipantType,
			"min_team_member":   detail.MinTeamMember,
			"max_team_member":   detail.MaxTeamMember,
			"grade_requirement": grades,
			"need_advisor":      detail.NeedAdvisor,
			"need_attachment":   detail.NeedAttachment,
			"reg_start_time":    regStartTime,
			"reg_end_time":      regEndTime,
			"submit_start_time": submitStartTime,
			"submit_end_time":   submitEndTime,
		},
	})
}

// SubmitRegistration 学生提交报名
func SubmitRegistration(c *gin.Context) {
	// 1. 绑定参数
	var req ApplicationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数格式错误", "error": err.Error()})
		return
	}

	// 2. 获取当前登录用户 (队长/本人)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未登录"})
		return
	}
	uid := userID.(uint)

	var comp models.CompDirectory
	if err := database.DB.Preload("Detail").First(&comp, req.CompID).Error; err != nil {
		c.JSON(404, gin.H{"code": 404, "msg": "赛事不存在"})
		fmt.Println("截止时间:", comp.Detail.RegEndTime)
		return
	}

	now := time.Now() // 获取服务器当前时间

	// 校验 A: 还没开始
	if now.Before(comp.Detail.RegStartTime) {
		removeUploadedFile(req.AttachmentUrl)
		c.JSON(403, gin.H{"code": 403, "msg": "非法请求：报名尚未开始"})
		return
	}

	// 校验 B: 已经结束
	if now.After(comp.Detail.RegEndTime) {
		removeUploadedFile(req.AttachmentUrl)
		c.JSON(403, gin.H{"code": 403, "msg": "非法请求：报名已截止"})
		return
	}

	//  防重复报名校验
	var count int64
	database.DB.Model(&models.Register{}).Where("comp_id = ? AND leader_id = ?", req.CompID, uid).Count(&count)
	if count > 0 {
		removeUploadedFile(req.AttachmentUrl)
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "您已报名过该赛事，请勿重复提交"})
		return
	}

	register := models.Register{
		CompID:        req.CompID,
		LeaderID:      uid,
		TeamName:      req.TeamName,
		AttachmentUrl: req.AttachmentUrl,
		Status:        0, // 默认 0:待审核
		Members:       make([]models.RegMember, 0),
	}

	leaderMember := models.RegMember{
		Name:      req.Leader.Name,
		StudentID: req.Leader.StuID,
		Phone:     req.Leader.Phone,
		Email:     req.Leader.Email,   // ✨ 保存邮箱
		College:   req.Leader.College, // ✨ 保存学院
		IsLeader:  true,               // ✨✨✨ 标记为队长
	}
	register.Members = append(register.Members, leaderMember)
	// 转换队员列表
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

	// 5. 写入数据库 (GORM 会自动在一个事务里插入 Register 和 RegisterMembers)
	if err := database.DB.Create(&register).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "报名失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "报名提交成功"})
}

// checkUserIsAdmin 通过 SQL 判断是否存在 admin 关联
func checkUserIsAdmin(userID uint) bool {
	var count int64

	// 逻辑：在 user_roles 中间表中查找，
	// 连接 roles 表，
	// 条件：user_id 是当前用户 AND role_code 是 admin
	err := database.DB.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.role_code LIKE ?", userID, "%admin%").
		Count(&count).Error

	if err != nil {
		return false
	}

	return count > 0
}

// GetRegList 获取报名审核列表
// 支持：赛事名模糊搜索、邮箱筛选、动态权限过滤
func GetRegList(c *gin.Context) {
	// 1. 获取参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	keyword := c.Query("keyword") // 搜负责人姓名 或 队伍名
	email := c.Query("email")     // 搜邮箱
	phone := c.Query("phone")
	status := c.Query("status")      // 搜状态 (0,1,2)
	compName := c.Query("comp_name") // 搜赛事名称
	//advisor := c.Query("advisor")        // 搜指导老师
	pType := c.Query("participant_type") // 搜赛制 (1:个人, 2:团队)
	// 2. 获取当前用户
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(401, gin.H{"code": 401, "msg": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	// 3. 构建查询
	// 基础查询：关联赛事表(用于权限和搜名字)、关联用户表(Leader)、关联队员表
	query := database.DB.Model(&models.Register{}).
		Joins("LEFT JOIN comp_directories ON comp_directories.id = registers.comp_id").
		Joins("LEFT JOIN comp_details ON comp_details.comp_id = registers.comp_id").
		Preload("Competition").
		Preload("Leader").
		Preload("Members")

	// 如果不是管理员，限制：只能查 manager_id = 当前用户 的赛事报名
	if !checkUserIsAdmin(userID) {
		query = query.Where("comp_directories.manager_id = ?", userID)
	}

	// --- 条件筛选 ---

	// 1. 搜赛事名称
	if compName != "" {
		query = query.Where("comp_directories.comp_name LIKE ?", "%"+compName+"%")
	}

	// 2. 搜状态
	if status != "" {
		query = query.Where("registers.status = ?", status)
	}
	// 搜赛制（个人/团队）
	if pType != "" {
		query = query.Where("comp_details.participant_type = ?", pType)
	}

	// 3. 搜 队伍名 或 负责人姓名
	if keyword != "" {
		// 注意：这里手动 Join users 表用于搜索，GORM 的 Preload 只是为了取值，不能用于 Where 条件
		query = query.Joins("LEFT JOIN users ON users.id = registers.leader_id").
			Where("registers.team_name LIKE ? OR users.realname LIKE ? OR users.username LIKE ?",
				"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 4. 搜邮箱
	if email != "" {
		// 防止重复 Join (如果 keyword 为空，上面没 Join 过，这里需要 Join)
		if keyword == "" {
			query = query.Joins("LEFT JOIN users ON users.id = registers.leader_id")
		}
		query = query.Where("EXISTS (SELECT 1 FROM reg_members WHERE reg_members.reg_id = registers.id AND reg_members.email LIKE ? AND reg_members.is_leader = ?)", "%"+email+"%", true)
	}

	if phone != "" {
		// 使用 EXISTS 子查询来搜 members 表里的电话
		query = query.Where("EXISTS (SELECT 1 FROM reg_members WHERE reg_members.reg_id = registers.id AND reg_members.phone LIKE ? AND reg_members.is_leader = ?)", "%"+phone+"%", true)
	}
	// --- 分页执行 ---
	var total int64
	// Count 时 GORM 会自动忽略 Preload，但保留 Joins 和 Where
	if err := query.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "查询失败", "err": err.Error()})
		return
	}

	var list []models.Register
	offset := (page - 1) * pageSize
	// Order 指定表名防止字段歧义
	if err := query.Order("registers.create_time desc").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "获取数据失败"})
		return
	}

	// --- 组装返回数据 (DTO) ---
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
	}

	var respList []AuditListResp
	for _, item := range list {
		// 处理空指针默认值
		leaderName := "未知"
		stuID := ""
		leaderEmail := "" // 最终显示的邮箱
		leaderPhone := "" // 最终显示的电话
		//if item.Leader.ID != 0 {
		//	leaderName = item.Leader.Realname
		//	if leaderName == "" {
		//		leaderName = item.Leader.Username
		//	}
		//	stuID = item.Leader.Username
		//}

		for _, m := range item.Members {
			if m.IsLeader {
				// 找到了队长！提取填表时的最新信息
				leaderEmail = m.Email
				leaderPhone = m.Phone

				if leaderName == "未知" || leaderName == "" {
					leaderName = m.Name
				}
				break
			}
		}

		respList = append(respList, AuditListResp{
			ID:            item.ID,
			CompID:        item.CompID,
			CompName:      item.Competition.CompName,
			TeamName:      item.TeamName,
			LeaderName:    leaderName,
			StuID:         stuID,
			Email:         leaderEmail,
			Phone:         leaderPhone,
			CreateTime:    item.CreatedAt.Format("2006-01-02 15:04"),
			Status:        item.Status,
			AttachmentUrl: item.AttachmentUrl,
			Members:       item.Members,
		})
	}

	c.JSON(200, gin.H{
		"code":  200,
		"msg":   "ok",
		"data":  respList,
		"total": total,
	})
}

// GetRegDetail 获取单条报名详情 (适配 auditDetail.vue)
func GetRegDetail(c *gin.Context) {
	// 获取参数
	regID := c.Query("id") // 对应 api.getRegDetail(id)
	if regID == "" {
		c.JSON(400, gin.H{"code": 400, "msg": "缺少 id 参数"})
		return
	}

	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	// 2. 数据库查询 (关联赛事、负责人User表、成员表)
	var reg models.Register
	err := database.DB.
		Preload("Competition"). // 为了拿 CompName 和 ManagerID
		Preload("Leader").      // 为了拿 User 表里的真实姓名/邮箱
		Preload("Members").     // 为了拿队员列表
		First(&reg, regID).Error

	if err != nil {
		c.JSON(404, gin.H{"code": 404, "msg": "报名记录不存在"})
		return
	}

	// 3. 权限校验
	// 只有管理员 或 该赛事的负责人 才能看
	if !checkUserIsAdmin(userID) {
		if reg.Competition.ManagerID != userID {
			c.JSON(403, gin.H{"code": 403, "msg": "无权查看此记录"})
			return
		}
	}

	// A. 处理负责人信息 (优先用 User 表，如果没有则用 Members 表兜底)
	leaderName := "未知"
	stuID := ""
	email := ""
	phone := ""

	// 从 User 表取基础信息
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
			// 如果 User 表里没填真实姓名，用报名表里的名字兜底
			if email == "" {
				email = m.Email
			}

			if reg.Leader.Realname == "" {
				leaderName = m.Name
			}
			break
		}
	}

	// 5. 返回 JSON
	c.JSON(200, gin.H{
		"code": 200,
		"msg":  "ok",
		"data": gin.H{
			"id":             reg.ID,
			"comp_name":      reg.Competition.CompName, // 自动关联获取
			"team_name":      reg.TeamName,
			"leader_name":    leaderName,
			"stu_id":         stuID,
			"phone":          phone,
			"email":          email,
			"update_time":    reg.UpdatedAt.Format("2006-01-02 15:04:05"),
			"status":         reg.Status,
			"attachment_url": reg.AttachmentUrl,
			"members":        reg.Members,
			"reject_reason":  reg.RejectReason,
		},
	})
}

// AuditRegister 审核接口
func AuditRegister(c *gin.Context) {
	type AuditReq struct {
		ID     uint   `json:"id" binding:"required"`
		Status int8   `json:"status"` // 1:通过 2:驳回
		Reason string `json:"reason"` // 驳回理由
	}
	var req AuditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	// 参数合法性校验
	// 防止前端传 3, 4, 99 这种非法值
	if req.Status != 1 && req.Status != 2 {
		c.JSON(400, gin.H{"code": 400, "msg": "非法的审核状态"})
		return
	}

	// 2. 驳回时必填理由
	if req.Status == 2 && req.Reason == "" {
		c.JSON(400, gin.H{"code": 400, "msg": "驳回时必须填写原因"})
		return
	}

	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	var reg models.Register
	err := database.DB.Preload("Competition").First(&reg, req.ID).Error

	if err != nil {
		c.JSON(404, gin.H{"code": 404, "msg": "记录不存在"})
		return
	}

	// 4. 【新增】状态流转校验 (防止重复审核)
	// 如果已经是 1(通过) 或 2(驳回)，就不应该再审核了 (视业务需求而定)
	if reg.Status != 0 {
		c.JSON(409, gin.H{"code": 409, "msg": "该记录已被审核，请勿重复操作"})
		return
	}

	// 5. 权限校验
	if !checkUserIsAdmin(userID) {
		if reg.Competition.ManagerID != userID {
			c.JSON(403, gin.H{"code": 403, "msg": "您无权审核此条记录"})
			return
		}
	}

	// 6. 执行更新
	updateMap := map[string]interface{}{
		"status": req.Status,
	}

	// 保存驳回理由
	if req.Status == 2 {
		updateMap["reject_reason"] = req.Reason
	} else {
		updateMap["reject_reason"] = ""
	}

	// 7. 更新数据库
	if err := database.DB.Model(&reg).Updates(updateMap).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "数据库更新失败"})
		return
	}

	c.JSON(200, gin.H{"code": 200, "msg": "审核完成"})
}

func GetMyRegStatus(c *gin.Context) {
	compID := c.Query("comp_id")
	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	var reg models.Register
	// 查询记录，同时预加载成员和队长信息，方便前端回显
	err := database.DB.
		Preload("Members").
		Preload("Leader").
		Where("comp_id = ? AND leader_id = ?", compID, userID).
		First(&reg).Error

	if err != nil {
		// 没查到，说明还没报名 (返回 null data)
		c.JSON(200, gin.H{"code": 200, "data": nil})
		return
	}

	// 查到了，返回详细信息供回显
	c.JSON(200, gin.H{
		"code": 200,
		"msg":  "获取成功",
		"data": gin.H{
			"id":             reg.ID,
			"team_name":      reg.TeamName,
			"status":         reg.Status,       // 0:待审 1:通过 2:驳回
			"reject_reason":  reg.RejectReason, // 驳回理由
			"attachment_url": reg.AttachmentUrl,
			"members":        reg.Members, //这里包含队长和队员
			"advisor_id":     reg.AdvisorID,
		},
	})
}

// ResubmitRegistration 重新提交报名 (用于驳回修改)
func ResubmitRegistration(c *gin.Context) {
	var req ApplicationReq
	// 这里复用 ApplicationReq 结构体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	var reg models.Register
	// 查找原记录
	if err := database.DB.Where("comp_id = ? AND leader_id = ?", req.CompID, userID).First(&reg).Error; err != nil {
		c.JSON(404, gin.H{"code": 404, "msg": "未找到原报名记录"})
		return
	}

	// 🔒 只有 "已驳回(2)" 或 "待审核(0)" 允许修改，"已通过(1)" 禁止修改
	if reg.Status == 1 {
		c.JSON(403, gin.H{"code": 403, "msg": "审核已通过，无法修改信息"})
		return
	}

	// 开启事务
	tx := database.DB.Begin()

	// 1. 更新主表
	reg.TeamName = req.TeamName
	reg.AttachmentUrl = req.AttachmentUrl
	reg.AdvisorID = req.AdvisorID
	reg.Status = 0        // ✨ 重点：状态重置为待审核
	reg.RejectReason = "" // 清空驳回理由

	if err := tx.Save(&reg).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"code": 500, "msg": "更新失败"})
		return
	}

	// 2. 更新成员 (策略：全删全加)
	if err := tx.Where("reg_id = ?", reg.ID).Delete(&models.RegMember{}).Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"code": 500, "msg": "清理旧成员失败"})
		return
	}

	// 3. 重新插入成员
	var newMembers []models.RegMember
	// 3.1 插入队长
	newMembers = append(newMembers, models.RegMember{
		RegID:     reg.ID,
		Name:      req.Leader.Name,
		StudentID: req.Leader.StuID,
		Phone:     req.Leader.Phone,
		Email:     req.Leader.Email,
		College:   req.Leader.College,
		IsLeader:  true,
	})
	// 3.2 插入队员
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
		c.JSON(500, gin.H{"code": 500, "msg": "保存成员失败"})
		return
	}

	tx.Commit()
	c.JSON(200, gin.H{"code": 200, "msg": "重新提交成功"})
}

// GetMyRegList 获取当前用户相关的报名赛事列表
func GetMyRegList(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(401, gin.H{"code": 401, "msg": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	// 获取学号
	var user models.User
	if err := database.DB.Select("username").First(&user, userID).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "获取用户信息失败"})
		return
	}
	studentID := user.Username

	targetRegID := c.Query("id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	offset := (page - 1) * pageSize

	var regs []models.Register
	var total int64

	//  子查询：找出该学生参与的所有报名ID
	subQuery := database.DB.Model(&models.RegMember{}).
		Select("reg_id").
		Where("username = ?", studentID)

	//  主查询
	query := database.DB.Model(&models.Register{}).
		Where("id IN (?)", subQuery).
		Preload("Competition").
		Preload("Competition.Detail")

	// 如果传了ID，则只查这一条
	if targetRegID != "" {
		query = query.Where("id = ?", targetRegID)
	}

	query.Count(&total)

	if err := query.Order("create_time desc").Offset(offset).Limit(pageSize).Find(&regs).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	// 组装数据
	var list []gin.H
	for _, r := range regs {
		compName := "未知赛事"
		submitStartStr := ""
		submitEndStr := ""

		if r.Competition.ID != 0 {
			compName = r.Competition.CompName
			if r.Competition.Detail.ID != 0 {
				if !r.Competition.Detail.SubmitStartTime.IsZero() {
					submitStartStr = r.Competition.Detail.SubmitStartTime.Format("2006-01-02 15:04:05")
				}
				if !r.Competition.Detail.SubmitEndTime.IsZero() {
					submitEndStr = r.Competition.Detail.SubmitEndTime.Format("2006-01-02 15:04:05")
				}
			}
		}

		regAttachment := ""
		if targetRegID != "" {
			// 只有在详情模式下，才返回报名附件
			regAttachment = r.AttachmentUrl
		}

		list = append(list, gin.H{
			"id":                r.ID,
			"comp_id":           r.CompID,
			"comp_name":         compName,
			"status":            r.Status,
			"reg_url":           regAttachment,       // 报名附件 (列表页为空，详情页有值)
			"work_url":          r.WorkAttachmentUrl, // ✨ 作品附件 (你需要确保Register model里有这个字段)
			"submit_start_time": submitStartStr,
			"submit_end_time":   submitEndStr,
		})
	}

	c.JSON(200, gin.H{
		"code": 200,
		"msg":  "获取成功",
		"data": gin.H{
			"list":  list,
			"total": total,
		},
	})
}

// SubmitWork 提交/保存参赛作品 (允许团队任何成员提交)
func SubmitWork(c *gin.Context) {
	var req SubmitWorkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(401, gin.H{"code": 401, "msg": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	// 获取当前用户的学号
	var user models.User
	if err := database.DB.Select("username").First(&user, userID).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "获取用户信息失败"})
		return
	}
	currentStuID := user.Username

	// 2查询报名记录 (预加载赛事详情时间)
	var reg models.Register
	if err := database.DB.Preload("Competition.Detail").First(&reg, req.RegID).Error; err != nil {
		c.JSON(404, gin.H{"code": 404, "msg": "报名记录不存在"})
		return
	}

	// 检查当前用户的学号，是否在 reg_members 表中对应 req.RegID 的记录里
	var count int64
	database.DB.Model(&models.RegMember{}).
		Where("reg_id = ? AND username = ?", req.RegID, currentStuID).
		Count(&count)

	if count == 0 {
		// 既不是队长，也不是队员
		c.JSON(403, gin.H{"code": 403, "msg": "无权操作：您不是该团队的成员"})
		return
	}

	// 时间校验
	now := time.Now()
	detail := reg.Competition.Detail

	if detail.SubmitStartTime.IsZero() || detail.SubmitEndTime.IsZero() {
		c.JSON(403, gin.H{"code": 403, "msg": "该赛事暂未开放作品提交"})
		return
	}
	if now.Before(detail.SubmitStartTime) {
		c.JSON(403, gin.H{"code": 403, "msg": "作品提交通道尚未开启"})
		return
	}
	if now.After(detail.SubmitEndTime) {
		c.JSON(403, gin.H{"code": 403, "msg": "作品提交已截止"})
		return
	}

	//  执行更新
	if err := database.DB.Model(&reg).Update("work_attachment_url", req.WorkUrl).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "保存失败"})
		return
	}

	c.JSON(200, gin.H{"code": 200, "msg": "作品已成功保存"})
}
