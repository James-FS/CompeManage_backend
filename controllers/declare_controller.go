package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DeclareCreateReq 创建申报请求
type DeclareCreateReq struct {
	CompName   string `json:"comp_name" binding:"required"`
	CompLevel  string `json:"comp_level" binding:"required"`
	CompType   string `json:"comp_type" binding:"required"`
	Organizer  string `json:"organizer" binding:"required"`
	Undertaker string `json:"undertaker" binding:"required"`
	CollegeID  uint   `json:"college_id" binding:"required"`
	ManagerID  uint   `json:"manager_id" binding:"required"`
	Year       int    `json:"year" binding:"required"`
	Desc       string `json:"desc"`
}

// DeclareUpdateReq 更新申报请求
type DeclareUpdateReq struct {
	CompName   string `json:"comp_name"`
	CompLevel  string `json:"comp_level"`
	CompType   string `json:"comp_type"`
	Organizer  string `json:"organizer"`
	Undertaker string `json:"undertaker"`
	ManagerID  uint   `json:"manager_id"`
	Year       int    `json:"year"`
	Desc       string `json:"desc"`
}

// DeclareListReq 申报列表查询请求
type DeclareListReq struct {
	Page          int    `form:"page" binding:"required,min=1"`
	PageSize      int    `form:"page_size" binding:"required,min=1"`
	CompName      string `form:"comp_name"`
	CompLevel     string `form:"comp_level"`
	CompType      string `form:"comp_type"`
	CollegeID     uint   `form:"college_id"`
	DeclareStatus int8   `form:"declare_status"` // -1:全部, 0:草稿, 1:已提交, 2:已通过, 3:已拒绝
}

// DeclareAuditReq 审核申报请求
type DeclareAuditReq struct {
	DeclareID   uint   `json:"declare_id" binding:"required"`
	AuditStatus int8   `json:"audit_status" binding:"required,min=2,max=3"` // 2:通过, 3:拒绝
	AuditRemark string `json:"audit_remark"`
}

// ============ 院级申报接口 ============

// CreateDeclare 创建赛事申报(保存为草稿)
func CreateDeclare(c *gin.Context) {
	// 绑定并校验请求参数
	var req DeclareCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请求参数错误", "error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未授权"})
		return
	}

	declaration := models.CompDeclaration{
		CompName:      req.CompName,
		CompLevel:     req.CompLevel,
		CompType:      req.CompType,
		Organizer:     req.Organizer,
		Undertaker:    req.Undertaker,
		CollegeID:     req.CollegeID,
		ManagerID:     req.ManagerID,
		Year:          req.Year,
		Desc:          req.Desc,
		DeclareStatus: 0, // 草稿状态
		CreatedBy:     userID.(uint),
	}

	// 保存到数据库
	if err := database.DB.Create(&declaration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建申报失败", "error": err.Error()})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": declaration})
}

// GetDeclareDetail 获取赛事申报详情
func GetDeclareDetail(c *gin.Context) {
	declareID := c.Param("id")

	var declaration models.CompDeclaration
	if err := database.DB.
		Preload("CollegeInfo").
		Preload("Manager").
		Preload("Declarer").
		Preload("Auditor").
		First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "申报记录不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败", "error": err.Error()})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "查询成功", "data": declaration})
}

// UpdateDeclare 更新赛事申报信息(仅草稿状态)
func UpdateDeclare(c *gin.Context) {
	declareID := c.Param("id")
	var req DeclareUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	// 检查申报是否存在且为草稿状态
	var declaration models.CompDeclaration
	if err := database.DB.First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "申报记录不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	// 只有草稿状态可以编辑
	if declaration.DeclareStatus != 0 {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "仅草稿状态可编辑"})
		return
	}

	// 更新字段
	updates := map[string]interface{}{}
	if req.CompName != "" {
		updates["comp_name"] = req.CompName
	}
	if req.CompLevel != "" {
		updates["comp_level"] = req.CompLevel
	}
	if req.CompType != "" {
		updates["comp_type"] = req.CompType
	}
	if req.Organizer != "" {
		updates["organizer"] = req.Organizer
	}
	if req.Undertaker != "" {
		updates["undertaker"] = req.Undertaker
	}
	if req.ManagerID != 0 {
		updates["manager_id"] = req.ManagerID
	}
	if req.Year != 0 {
		updates["year"] = req.Year
	}
	if req.Desc != "" {
		updates["desc"] = req.Desc
	}

	if err := database.DB.Model(&declaration).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "更新成功", "data": declaration})
}

// SubmitDeclare 提交申报（状态：草稿 → 已提交）
func SubmitDeclare(c *gin.Context) {
	declareID := c.Param("id")

	var declaration models.CompDeclaration
	if err := database.DB.First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "申报记录不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	// 只有草稿状态可以提交
	if declaration.DeclareStatus != 0 {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "只有草稿状态可提交"})
		return
	}

	// 更新为已提交状态
	if err := database.DB.Model(&declaration).Update("declare_status", 1).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "提交失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "申报已提交，等待审核"})
}

// GetMyDeclares 获取我的申报列表（院级管理员）
func GetMyDeclares(c *gin.Context) {
	var req DeclareListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未授权"})
		return
	}

	query := database.DB.Where("created_by = ?", userID.(uint))

	// 筛选条件
	if req.CompName != "" {
		query = query.Where("comp_name LIKE ?", "%"+req.CompName+"%")
	}
	if req.CompLevel != "" {
		query = query.Where("comp_level = ?", req.CompLevel)
	}
	if req.CompType != "" {
		query = query.Where("comp_type = ?", req.CompType)
	}
	if req.DeclareStatus >= 0 {
		query = query.Where("declare_status = ?", req.DeclareStatus)
	}

	var total int64
	query.Model(&models.CompDeclaration{}).Count(&total)

	var declarations []models.CompDeclaration
	if err := query.
		Preload("CollegeInfo").
		Preload("Manager").
		Preload("Declarer").
		Preload("Auditor").
		Offset((req.Page - 1) * req.PageSize).
		Limit(req.PageSize).
		Order("create_time DESC").
		Find(&declarations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "查询成功",
		"data": gin.H{
			"total": total,
			"items": declarations,
		},
	})
}

// DeleteDeclare 删除申报（仅草稿状态可删除）
func DeleteDeclare(c *gin.Context) {
	declareID := c.Param("id")

	var declaration models.CompDeclaration
	if err := database.DB.First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "申报记录不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	// 只有草稿状态可以删除
	if declaration.DeclareStatus != 0 {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "仅草稿状态可删除"})
		return
	}

	// 草稿使用硬删除
	if err := database.DB.Unscoped().Delete(&declaration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "删除失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

// ============ 校级审核接口 ============

// GetPendingDeclares 获取待审核申报列表（校级管理员）
func GetPendingDeclares(c *gin.Context) {
	var req DeclareListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	// 查询所有已提交的申报（status=1）
	query := database.DB.Where("declare_status = ?", 1)

	if req.CompName != "" {
		query = query.Where("comp_name LIKE ?", "%"+req.CompName+"%")
	}
	if req.CompLevel != "" {
		query = query.Where("comp_level = ?", req.CompLevel)
	}
	if req.CollegeID != 0 {
		query = query.Where("college_id = ?", req.CollegeID)
	}

	var total int64
	query.Model(&models.CompDeclaration{}).Count(&total)

	var declarations []models.CompDeclaration
	if err := query.
		Preload("CollegeInfo").
		Preload("Manager").
		Preload("Declarer").
		Offset((req.Page - 1) * req.PageSize).
		Limit(req.PageSize).
		Order("create_time ASC").
		Find(&declarations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "查询成功",
		"data": gin.H{
			"total": total,
			"items": declarations,
		},
	})
}

// AuditDeclare 审核申报（通过/拒绝）
func AuditDeclare(c *gin.Context) {
	var req DeclareAuditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	auditorID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未授权"})
		return
	}

	var declaration models.CompDeclaration
	if err := database.DB.First(&declaration, req.DeclareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "申报记录不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	// 只有已提交状态的申报可以审核
	if declaration.DeclareStatus != 1 {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "只能审核已提交的申报"})
		return
	}

	now := time.Now()
	var status string

	switch req.AuditStatus {
	case 2: // 审核通过
		// 创建赛事目录
		newComp := models.CompDirectory{
			CompName:   declaration.CompName,
			CompLevel:  declaration.CompLevel,
			CompType:   declaration.CompType,
			Organizer:  declaration.Organizer,
			Undertaker: declaration.Undertaker,
			CollegeID:  declaration.CollegeID,
			ManagerID:  declaration.ManagerID,
			Year:       declaration.Year,
			Desc:       declaration.Desc,
			CreatedBy:  auditorID.(uint),
			Status:     0, // 草稿状态
			Source:     2, // 来源：申报通过创建
			DeclareID:  declaration.ID,
		}

		if err := database.DB.Create(&newComp).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建赛事目录失败", "error": err.Error()})
			return
		}

		// 更新申报状态
		declaration.DeclareStatus = 2
		auditorID_uint := auditorID.(uint)
		declaration.AuditBy = &auditorID_uint
		declaration.AuditAt = &now
		compDirID := newComp.ID
		declaration.CompDirectoryID = &compDirID
		declaration.AuditRemark = "审核通过"
		status = "通过"

	case 3: // 审核拒绝
		declaration.DeclareStatus = 3
		auditorID_uint := auditorID.(uint)
		declaration.AuditBy = &auditorID_uint
		declaration.AuditAt = &now
		declaration.AuditRemark = req.AuditRemark
		status = "拒绝"
	}

	if err := database.DB.Save(&declaration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "审核失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "申报审核" + status})
}

// GetAllDeclares 获取所有申报（可选筛选条件）
func GetAllDeclares(c *gin.Context) {
	var req DeclareListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	query := database.DB.Model(&models.CompDeclaration{})

	if req.CompName != "" {
		query = query.Where("comp_name LIKE ?", "%"+req.CompName+"%")
	}
	if req.CompLevel != "" {
		query = query.Where("comp_level = ?", req.CompLevel)
	}
	if req.CollegeID != 0 {
		query = query.Where("college_id = ?", req.CollegeID)
	}

	var total int64
	query.Count(&total)

	var declarations []models.CompDeclaration
	if err := query.
		Preload("CollegeInfo").
		Preload("Manager").
		Preload("Declarer").
		Preload("Auditor").
		Offset((req.Page - 1) * req.PageSize).
		Limit(req.PageSize).
		Order("create_time DESC").
		Find(&declarations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "查询成功",
		"data": gin.H{
			"total": total,
			"items": declarations,
		},
	})
}
