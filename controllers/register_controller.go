package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"encoding/json"
	"net/http"
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

// SaveCompConfig 保存逻辑
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
	var detail models.CompDetail
	// 使用 First 查询，如果没找到会报错
	if err := database.DB.Where("comp_id = ?", compID).First(&detail).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// ✨ 没查到是正常的（说明还没设置过），返回空数据结构，让前端显示默认表单
			c.JSON(http.StatusOK, gin.H{
				"code": 200,
				"msg":  "暂无配置",
				"data": nil, // 返回 nil，前端会处理
			})
			return
		}
		// 其他数据库错误
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询配置失败"})
		return
	}

	// 3. 数据处理：年级 JSON 字符串 -> int 数组
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
