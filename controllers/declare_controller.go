package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DeclareCreateReq struct {
	CompName       string `json:"comp_name" binding:"required"`
	CompLevel      string `json:"comp_level" binding:"required"`
	CompType       string `json:"comp_type" binding:"required"`
	Organizer      string `json:"organizer" binding:"required"`
	Undertaker     string `json:"undertaker" binding:"required"`
	CollegeID      uint   `json:"college_id" binding:"required"`
	ManagerID      uint   `json:"manager_id" binding:"required"`
	Year           int    `json:"year" binding:"required"`
	Desc           string `json:"desc"`
	AttachmentPath string `json:"attachment_path"`
}

type DeclareUpdateReq struct {
	CompName       string  `json:"comp_name"`
	CompLevel      string  `json:"comp_level"`
	CompType       string  `json:"comp_type"`
	Organizer      string  `json:"organizer"`
	Undertaker     string  `json:"undertaker"`
	CollegeID      uint    `json:"college_id"`
	ManagerID      uint    `json:"manager_id"`
	Year           int     `json:"year"`
	Desc           string  `json:"desc"`
	AttachmentPath *string `json:"attachment_path"`
}

type DeclareListReq struct {
	Page          int    `form:"page" binding:"required,min=1"`
	PageSize      int    `form:"page_size" binding:"required,min=1"`
	CompName      string `form:"comp_name"`
	CompLevel     string `form:"comp_level"`
	CompType      string `form:"comp_type"`
	CollegeID     uint   `form:"college_id"`
	DeclareStatus int8   `form:"declare_status"`
}

type DeclareAuditReq struct {
	DeclareID   uint   `json:"declare_id" binding:"required"`
	AuditStatus int8   `json:"audit_status" binding:"required,min=2,max=3"`
	AuditRemark string `json:"audit_remark"`
}

func CreateDeclare(c *gin.Context) {
	var req DeclareCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误")
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未授权")
		return
	}

	declaration := models.CompDeclaration{
		CompName:       req.CompName,
		CompLevel:      req.CompLevel,
		CompType:       req.CompType,
		Organizer:      req.Organizer,
		Undertaker:     req.Undertaker,
		CollegeID:      req.CollegeID,
		ManagerID:      req.ManagerID,
		Year:           req.Year,
		Desc:           req.Desc,
		AttachmentPath: req.AttachmentPath,
		DeclareStatus:  0,
		CreatedBy:      userID.(uint),
	}

	if err := database.DB.Create(&declaration).Error; err != nil {
		utils.InternalServerError(c, "创建申报失败", err)
		return
	}

	utils.SuccessWithMessage(c, "创建成功", declaration)
}

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
			utils.NotFound(c, "申报记录不存在")
			return
		}
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	utils.SuccessWithMessage(c, "查询成功", declaration)
}

func UpdateDeclare(c *gin.Context) {
	declareID := c.Param("id")
	var req DeclareUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	var declaration models.CompDeclaration
	if err := database.DB.First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "申报记录不存在")
			return
		}
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	if declaration.DeclareStatus != 0 && declaration.DeclareStatus != 3 {
		utils.Forbidden(c, "仅草稿和已驳回状态可编辑")
		return
	}

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
	if req.CollegeID != 0 {
		updates["college_id"] = req.CollegeID
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
	if req.AttachmentPath != nil {
		updates["attachment_path"] = *req.AttachmentPath
	}

	if err := database.DB.Model(&declaration).Updates(updates).Error; err != nil {
		utils.InternalServerError(c, "更新失败", err)
		return
	}

	utils.SuccessWithMessage(c, "更新成功", declaration)
}

func SubmitDeclare(c *gin.Context) {
	declareID := c.Param("id")

	var declaration models.CompDeclaration
	if err := database.DB.First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "申报记录不存在")
			return
		}
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	if declaration.DeclareStatus != 0 && declaration.DeclareStatus != 3 {
		utils.Forbidden(c, "只有草稿和已驳回状态可提交")
		return
	}

	if err := database.DB.Model(&declaration).Update("declare_status", 1).Error; err != nil {
		utils.InternalServerError(c, "提交失败", err)
		return
	}

	utils.SuccessWithMessage(c, "申报已提交，等待审核", nil)
}

func RevokeDeclare(c *gin.Context) {
	declareID := c.Param("id")

	var declaration models.CompDeclaration
	if err := database.DB.First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "申报记录不存在")
			return
		}
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	if declaration.DeclareStatus != 1 {
		utils.Forbidden(c, "只有已提交状态可撤回")
		return
	}

	if err := database.DB.Model(&declaration).Update("declare_status", 0).Error; err != nil {
		utils.InternalServerError(c, "撤回失败", err)
		return
	}

	utils.SuccessWithMessage(c, "申报已撤回，可重新编辑", nil)
}

func GetMyDeclares(c *gin.Context) {
	var req DeclareListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未授权")
		return
	}

	query := database.DB.Where("created_by = ?", userID.(uint))

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
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	utils.Success(c, gin.H{
		"total": total,
		"items": declarations,
	})
}

func GetMyPendingDeclares(c *gin.Context) {
	var req DeclareListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未授权")
		return
	}

	query := database.DB.Where("created_by = ?", userID.(uint)).
		Where("declare_status IN ?", []int{0, 1, 3})

	if req.CompName != "" {
		query = query.Where("comp_name LIKE ?", "%"+req.CompName+"%")
	}
	if req.CompLevel != "" {
		query = query.Where("comp_level = ?", req.CompLevel)
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
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	utils.Success(c, gin.H{
		"total": total,
		"items": declarations,
	})
}

func GetMyPublishedDeclares(c *gin.Context) {
	var req DeclareListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未授权")
		return
	}

	query := database.DB.Where("created_by = ?", userID.(uint)).
		Where("declare_status = ?", 2)

	if req.CompName != "" {
		query = query.Where("comp_name LIKE ?", "%"+req.CompName+"%")
	}
	if req.CompLevel != "" {
		query = query.Where("comp_level = ?", req.CompLevel)
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
		Order("audit_at DESC").
		Find(&declarations).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	utils.Success(c, gin.H{
		"total": total,
		"items": declarations,
	})
}

func DeleteDeclare(c *gin.Context) {
	declareID := c.Param("id")

	var declaration models.CompDeclaration
	if err := database.DB.First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "申报记录不存在")
			return
		}
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	if declaration.DeclareStatus != 0 {
		utils.Forbidden(c, "仅草稿状态可删除")
		return
	}

	if err := database.DB.Unscoped().Delete(&declaration).Error; err != nil {
		utils.InternalServerError(c, "删除失败", err)
		return
	}

	utils.SuccessWithMessage(c, "删除成功", nil)
}

func GetPendingDeclares(c *gin.Context) {
	var req DeclareListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

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
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	utils.Success(c, gin.H{
		"total": total,
		"items": declarations,
	})
}

func AuditDeclare(c *gin.Context) {
	var req DeclareAuditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	auditorID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未授权")
		return
	}

	var declaration models.CompDeclaration
	if err := database.DB.First(&declaration, req.DeclareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "申报记录不存在")
			return
		}
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	if declaration.DeclareStatus != 1 {
		utils.Forbidden(c, "只能审核已提交的申报")
		return
	}

	now := time.Now()
	var status string

	switch req.AuditStatus {
	case 2:
		collegeID := declaration.CollegeID
		newComp := models.CompDirectory{
			CompName:   declaration.CompName,
			CompLevel:  declaration.CompLevel,
			CompType:   declaration.CompType,
			Organizer:  declaration.Organizer,
			Undertaker: declaration.Undertaker,
			CollegeID:  &collegeID,
			ManagerID:  declaration.ManagerID,
			Year:       declaration.Year,
			Desc:       declaration.Desc,
			CreatedBy:  auditorID.(uint),
			Status:     0,
			Source:     2,
			DeclareID:  &declaration.ID,
		}

		if err := database.DB.Create(&newComp).Error; err != nil {
			utils.InternalServerError(c, "创建赛事目录失败", err)
			return
		}

		declaration.DeclareStatus = 2
		auditorID_uint := auditorID.(uint)
		declaration.AuditBy = &auditorID_uint
		declaration.AuditAt = &now
		compDirID := newComp.ID
		declaration.CompDirectoryID = &compDirID
		declaration.AuditRemark = "审核通过"
		status = "通过"

	case 3:
		declaration.DeclareStatus = 3
		auditorID_uint := auditorID.(uint)
		declaration.AuditBy = &auditorID_uint
		declaration.AuditAt = &now
		declaration.AuditRemark = req.AuditRemark
		status = "拒绝"
	}

	if err := database.DB.Save(&declaration).Error; err != nil {
		utils.InternalServerError(c, "审核失败", err)
		return
	}

	utils.SuccessWithMessage(c, "申报审核"+status, nil)
}

func GetAllDeclares(c *gin.Context) {
	var req DeclareListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
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
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	utils.Success(c, gin.H{
		"total": total,
		"items": declarations,
	})
}

func GetAuditedDeclares(c *gin.Context) {
	var req DeclareListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	query := database.DB.Where("declare_status IN ?", []int{2, 3})

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
		Preload("Auditor").
		Offset((req.Page - 1) * req.PageSize).
		Limit(req.PageSize).
		Order("audit_at DESC").
		Find(&declarations).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	utils.Success(c, gin.H{
		"total": total,
		"items": declarations,
	})
}
