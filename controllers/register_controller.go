package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	GradeRequirement []int      `json:"grade_requirement"`
	NeedAdvisor      int        `json:"need_advisor"`
	NeedAttachment   int        `json:"need_attachment"`
}

// ApplicationReq 学生提交的报名数据
type ApplicationReq struct {
	CompID        uint        `json:"comp_id" binding:"required"`
	TeamName      string      `json:"team_name"`      // 队伍名称
	Members       []MemberReq `json:"members"`        // 队员列表 (不包含队长)
	AttachmentUrl string      `json:"attachment_url"` // 附件地址
}

// MemberReq 队员信息子结构
type MemberReq struct {
	Name  string `json:"name" binding:"required"`
	StuID string `json:"stu_id" binding:"required"` // 学号
	Phone string `json:"phone"`
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

	// 6. 执行数据库操作
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
				// 其他字段让前端显示默认值即可，或者给 null
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
	var startTime, endTime *time.Time
	if !detail.RegStartTime.IsZero() {
		startTime = &detail.RegStartTime
	}
	if !detail.RegEndTime.IsZero() {
		endTime = &detail.RegEndTime
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
			"reg_start_time":    startTime,
			"reg_end_time":      endTime,
		},
	})
}

// SubmitRegistration 学生提交报名 (简化版：不查赛事规则，只存数据)
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

	// 转换队员列表
	for _, m := range req.Members {
		register.Members = append(register.Members, models.RegMember{
			Name:      m.Name,
			StudentID: m.StuID,
			Phone:     m.Phone,
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
