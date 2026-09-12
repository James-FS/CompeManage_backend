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
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	if scope != nil && scope.IsCollegeAdmin() {
		if req.CollegeID != *scope.ManagedCollegeID {
			utils.Forbidden(c, "院级管理员只能为所管理学院创建申报")
			return
		}
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

	var promotions []RolePromotion
	if err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&declaration).Error; err != nil {
			return err
		}
		// P4：申报负责人若为教师，自动提升为赛事负责人（操作者仅为校/院管理员）。
		oldRoleID, newRoleID, promoteErr := promoteCompetitionManagerIfTeacher(tx, req.ManagerID)
		if promoteErr != nil {
			return promoteErr
		}
		if oldRoleID != 0 {
			promotions = append(promotions, RolePromotion{
				UserID:    req.ManagerID,
				OldRoleID: oldRoleID,
				NewRoleID: newRoleID,
			})
		}
		return nil
	}); err != nil {
		utils.InternalServerError(c, "创建申报失败", err)
		return
	}

	sourceID := declaration.ID
	finalizeRolePromotions(c, promotions, auditReasonDeclare, &sourceID)
	utils.SuccessWithMessage(c, "创建成功", gin.H{
		"declaration":    declaration,
		"promoted_users": promotedUsersResp(c, promotions),
	})
}

func GetDeclareDetail(c *gin.Context) {
	declareID := c.Param("id")
	var declaration models.CompDeclaration
	if err := database.DB.WithContext(c.Request.Context()).
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
	if scope, ok := requestAccessScopeIfAvailable(c); !ok {
		return
	} else if scope != nil && !canAccessDeclaration(scope, declaration) {
		utils.Forbidden(c, "无权查看该申报")
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
	if err := database.DB.WithContext(c.Request.Context()).First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "申报记录不存在")
			return
		}
		utils.InternalServerError(c, "查询失败", err)
		return
	}
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	if scope != nil && !canAccessDeclaration(scope, declaration) {
		utils.Forbidden(c, "无权编辑该申报")
		return
	}
	if scope != nil && scope.IsCollegeAdmin() && req.CollegeID != 0 && req.CollegeID != *scope.ManagedCollegeID {
		utils.Forbidden(c, "院级管理员不能将申报转移到其他学院")
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

	// P4：改派申报负责人时（显式传值且变化），仅提升新负责人；原负责人角色保留（不做降级）。
	var promotions []RolePromotion
	managerChanged := req.ManagerID != 0 && req.ManagerID != declaration.ManagerID
	err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&declaration).Updates(updates).Error; err != nil {
			return err
		}
		if managerChanged {
			oldRoleID, newRoleID, promoteErr := promoteCompetitionManagerIfTeacher(tx, req.ManagerID)
			if promoteErr != nil {
				return promoteErr
			}
			if oldRoleID != 0 {
				promotions = append(promotions, RolePromotion{
					UserID:    req.ManagerID,
					OldRoleID: oldRoleID,
					NewRoleID: newRoleID,
				})
			}
		}
		return nil
	})
	if err != nil {
		utils.InternalServerError(c, "更新失败", err)
		return
	}

	if managerChanged {
		sourceID := declaration.ID
		finalizeRolePromotions(c, promotions, auditReasonDeclare, &sourceID)
	}
	utils.SuccessWithMessage(c, "更新成功", gin.H{
		"declaration":    declaration,
		"promoted_users": promotedUsersResp(c, promotions),
	})
}

func SubmitDeclare(c *gin.Context) {
	declareID := c.Param("id")
	var declaration models.CompDeclaration
	if err := database.DB.WithContext(c.Request.Context()).First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "申报记录不存在")
			return
		}
		utils.InternalServerError(c, "查询失败", err)
		return
	}
	if scope, ok := requestAccessScopeIfAvailable(c); !ok {
		return
	} else if scope != nil && !canAccessDeclaration(scope, declaration) {
		utils.Forbidden(c, "无权提交该申报")
		return
	}
	if declaration.DeclareStatus != 0 && declaration.DeclareStatus != 3 {
		utils.Forbidden(c, "只有草稿和已驳回状态可提交")
		return
	}

	if err := database.DB.WithContext(c.Request.Context()).Model(&declaration).Update("declare_status", 1).Error; err != nil {
		utils.InternalServerError(c, "提交失败", err)
		return
	}

	utils.SuccessWithMessage(c, "申报已提交，等待审核", nil)
}

func RevokeDeclare(c *gin.Context) {
	declareID := c.Param("id")
	var declaration models.CompDeclaration
	if err := database.DB.WithContext(c.Request.Context()).First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "申报记录不存在")
			return
		}
		utils.InternalServerError(c, "查询失败", err)
		return
	}
	if scope, ok := requestAccessScopeIfAvailable(c); !ok {
		return
	} else if scope != nil && !canAccessDeclaration(scope, declaration) {
		utils.Forbidden(c, "无权撤回该申报")
		return
	}
	if declaration.DeclareStatus != 1 {
		utils.Forbidden(c, "只有已提交状态可撤回")
		return
	}

	if err := database.DB.WithContext(c.Request.Context()).Model(&declaration).Update("declare_status", 0).Error; err != nil {
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

	query := database.DB.WithContext(c.Request.Context()).Where("created_by = ?", userID.(uint))

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

	query := database.DB.WithContext(c.Request.Context()).Where("created_by = ?", userID.(uint)).
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

	query := database.DB.WithContext(c.Request.Context()).Where("created_by = ?", userID.(uint)).
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
	if err := database.DB.WithContext(c.Request.Context()).First(&declaration, declareID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "申报记录不存在")
			return
		}
		utils.InternalServerError(c, "查询失败", err)
		return
	}
	if scope, ok := requestAccessScopeIfAvailable(c); !ok {
		return
	} else if scope != nil && !canAccessDeclaration(scope, declaration) {
		utils.Forbidden(c, "无权删除该申报")
		return
	}
	if declaration.DeclareStatus != 0 {
		utils.Forbidden(c, "仅草稿状态可删除")
		return
	}

	if err := database.DB.WithContext(c.Request.Context()).Unscoped().Delete(&declaration).Error; err != nil {
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

	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	query := database.DB.WithContext(c.Request.Context()).Where("declare_status = ?", 1)
	if scope != nil {
		query = applyDeclarationScope(query, scope, "comp_declarations")
	}

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

	auditorIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未授权")
		return
	}
	auditorID := auditorIDVal.(uint)
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	if scope != nil && !scope.IsSchoolAdmin() {
		utils.Forbidden(c, "仅校级管理员可以审核赛事申报")
		return
	}

	var declaration models.CompDeclaration
	if err := database.DB.WithContext(c.Request.Context()).First(&declaration, req.DeclareID).Error; err != nil {
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
		compCode, codeErr := nextCompetitionCode(database.DB.WithContext(c.Request.Context()), declaration.CompLevel, declaration.Year)
		if codeErr != nil {
			utils.InternalServerError(c, "生成赛事编号失败", codeErr)
			return
		}

		collegeID := declaration.CollegeID
		newComp := models.CompDirectory{
			CompCode:   compCode,
			CompName:   declaration.CompName,
			CompLevel:  declaration.CompLevel,
			CompType:   declaration.CompType,
			Organizer:  declaration.Organizer,
			Undertaker: declaration.Undertaker,
			CollegeID:  &collegeID,
			ManagerID:  declaration.ManagerID,
			Year:       declaration.Year,
			Desc:       declaration.Desc,
			CreatedBy:  auditorID,
			Status:     0,
			Source:     2,
			DeclareID:  &declaration.ID,
		}

		if err := database.DB.WithContext(c.Request.Context()).Create(&newComp).Error; err != nil {
			utils.InternalServerError(c, "创建赛事目录失败", err)
			return
		}

		declaration.DeclareStatus = 2
		declaration.AuditBy = &auditorID
		declaration.AuditAt = &now
		compDirID := newComp.ID
		declaration.CompDirectoryID = &compDirID
		declaration.AuditRemark = "审核通过"
		status = "通过"

	case 3:
		declaration.DeclareStatus = 3
		declaration.AuditBy = &auditorID
		declaration.AuditAt = &now
		declaration.AuditRemark = req.AuditRemark
		status = "拒绝"
	}

	if err := database.DB.WithContext(c.Request.Context()).Save(&declaration).Error; err != nil {
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

	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	query := database.DB.WithContext(c.Request.Context()).Model(&models.CompDeclaration{})
	if scope != nil {
		query = applyDeclarationScope(query, scope, "comp_declarations")
	}

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

	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	query := database.DB.WithContext(c.Request.Context()).Where("declare_status IN ?", []int{2, 3})
	if scope != nil {
		query = applyDeclarationScope(query, scope, "comp_declarations")
	}

	if req.DeclareStatus != 0 {
		query = query.Where("declare_status = ?", req.DeclareStatus)
	}
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
