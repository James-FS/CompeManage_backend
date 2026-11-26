package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errDeclarationStatusChanged = errors.New("申报状态或项目来源已变化，请刷新后重试")
var errDeclarationSourceOutsideManagedCollege = errors.New("账号所属单位需与管理学院一致才可发起赛事申报，请联系校管理员核对")

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

func applyCollegeDeclarationScope(query *gorm.DB, scope *UserAccessScope) *gorm.DB {
	if scope.IsCollegeAdmin() {
		return applyDeclarationScope(query, scope, "comp_declarations")
	}
	return query
}

func CreateDeclare(c *gin.Context) {
	var req DeclareCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误")
		return
	}
	if !validCompetitionYear(req.Year) {
		utils.BadRequest(c, "所属年份须为1000至9999的4位数字")
		return
	}
	if attachmentURLCount(req.AttachmentPath) > maxBusinessAttachments {
		utils.BadRequest(c, "申报最多添加5个附件")
		return
	}
	req.AttachmentPath = attachmentURLsToLegacyString(req.AttachmentPath)

	userID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "未授权")
		return
	}
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
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

	var promotions []RolePromotion
	if err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := validateCompetitionManager(c, tx, req.ManagerID); err != nil {
			return err
		}
		origin, _, err := projectOriginForRequest(c, tx)
		if err != nil {
			return err
		}
		if scope.IsCollegeAdmin() && (origin.ProjectSourceCollegeID == nil ||
			*origin.ProjectSourceCollegeID != *scope.ManagedCollegeID) {
			return errDeclarationSourceOutsideManagedCollege
		}
		declaration.ProjectOrigin = origin
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
		if errors.Is(err, errDeclarationSourceOutsideManagedCollege) {
			utils.Forbidden(c, err.Error())
			return
		}
		respondCompetitionMutationError(c, "创建申报失败", err)
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
	if req.Year != 0 && !validCompetitionYear(req.Year) {
		utils.BadRequest(c, "所属年份须为1000至9999的4位数字")
		return
	}
	if req.AttachmentPath != nil && attachmentURLCount(*req.AttachmentPath) > maxBusinessAttachments {
		utils.BadRequest(c, "申报最多添加5个附件")
		return
	}
	if req.AttachmentPath != nil {
		legacyValue := attachmentURLsToLegacyString(*req.AttachmentPath)
		req.AttachmentPath = &legacyValue
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
		managerID := declaration.ManagerID
		if req.ManagerID != 0 {
			managerID = req.ManagerID
		}
		if err := validateCompetitionManager(c, tx, managerID); err != nil {
			return err
		}
		if len(updates) > 0 {
			query := tx.Model(&declaration).Where("declare_status = ?", declaration.DeclareStatus)
			result := applyCollegeDeclarationScope(query, scope).Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return errDeclarationStatusChanged
			}
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
	if errors.Is(err, errDeclarationStatusChanged) {
		utils.Conflict(c, err.Error())
		return
	}
	if err != nil {
		respondCompetitionMutationError(c, "更新失败", err)
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
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	if scope != nil && !canAccessDeclaration(scope, declaration) {
		utils.Forbidden(c, "无权提交该申报")
		return
	}
	if declaration.DeclareStatus != 0 && declaration.DeclareStatus != 3 {
		utils.Forbidden(c, "只有草稿和已驳回状态可提交")
		return
	}

	query := database.DB.WithContext(c.Request.Context()).Model(&declaration).
		Where("declare_status = ?", declaration.DeclareStatus)
	result := applyCollegeDeclarationScope(query, scope).Update("declare_status", 1)
	if result.Error != nil {
		utils.InternalServerError(c, "提交失败", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		utils.Conflict(c, errDeclarationStatusChanged.Error())
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
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	if scope != nil && !canAccessDeclaration(scope, declaration) {
		utils.Forbidden(c, "无权撤回该申报")
		return
	}
	if declaration.DeclareStatus != 1 {
		utils.Forbidden(c, "只有已提交状态可撤回")
		return
	}

	query := database.DB.WithContext(c.Request.Context()).Model(&declaration).
		Where("declare_status = ?", declaration.DeclareStatus)
	result := applyCollegeDeclarationScope(query, scope).Update("declare_status", 0)
	if result.Error != nil {
		utils.InternalServerError(c, "撤回失败", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		utils.Conflict(c, errDeclarationStatusChanged.Error())
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
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	query = applyCollegeDeclarationScope(query, scope)

	if req.CompName != "" {
		query = query.Where("comp_name LIKE ?", "%"+req.CompName+"%")
	}
	if req.CompLevel != "" {
		query = query.Where("comp_level = ?", req.CompLevel)
	}
	if req.CompType != "" {
		query = query.Where("comp_type = ?", req.CompType)
	}
	// declare_status 缺省不过滤（int8 零值无法区分「未传」与「草稿」）；
	// 显式传 0 才筛草稿。此前 `>= 0` 导致「我的申报」永远只剩草稿。
	if c.Query("declare_status") != "" {
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
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	query = applyCollegeDeclarationScope(query, scope)

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
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	query = applyCollegeDeclarationScope(query, scope)

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
	scope, ok := requestAccessScopeIfAvailable(c)
	if !ok {
		return
	}
	if scope != nil && !canAccessDeclaration(scope, declaration) {
		utils.Forbidden(c, "无权删除该申报")
		return
	}
	if declaration.DeclareStatus != 0 {
		utils.Forbidden(c, "仅草稿状态可删除")
		return
	}

	query := database.DB.WithContext(c.Request.Context()).Unscoped().
		Where("declare_status = ? AND delete_time IS NULL", 0)
	result := applyCollegeDeclarationScope(query, scope).Delete(&declaration)
	if result.Error != nil {
		utils.InternalServerError(c, "删除失败", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		utils.Conflict(c, errDeclarationStatusChanged.Error())
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
	if scope != nil && !scope.IsSchoolAdmin() && !scope.IsCollegeAdmin() {
		utils.Forbidden(c, "无权审核赛事申报")
		return
	}

	req.AuditRemark = strings.TrimSpace(req.AuditRemark)
	if req.AuditStatus == 3 && req.AuditRemark == "" {
		utils.BadRequest(c, "请输入驳回原因")
		return
	}

	errNotSubmitted := errors.New("只能审核已提交的申报")
	errInvalidYear := errors.New("申报年份须为1000至9999的4位数字")
	err := database.TransactionWithDeadlockRetry(database.DB.WithContext(c.Request.Context()), func(tx *gorm.DB) error {
		var declaration models.CompDeclaration
		query := applyCollegeDeclarationScope(tx.Model(&models.CompDeclaration{}), scope)
		if err := query.Clauses(clause.Locking{Strength: "UPDATE"}).First(&declaration, req.DeclareID).Error; err != nil {
			return err
		}
		if declaration.DeclareStatus != 1 {
			return errNotSubmitted
		}

		updates := map[string]interface{}{
			"declare_status": req.AuditStatus,
			"audit_by":       auditorID,
			"audit_at":       time.Now(),
			"audit_remark":   req.AuditRemark,
		}
		if req.AuditStatus == 2 {
			if !validCompetitionYear(declaration.Year) {
				return errInvalidYear
			}
			compCode, err := nextCompetitionCode(tx, declaration.CompLevel, declaration.Year)
			if err != nil {
				return err
			}
			collegeID := &declaration.CollegeID
			if scope != nil {
				var unit string
				if err := tx.Model(&models.User{}).Where("id = ?", declaration.ManagerID).Pluck("college", &unit).Error; err != nil {
					return err
				}
				collegeID, err = resolveUnitCollegeID(tx, unit)
				if err != nil {
					return err
				}
			}
			comp := models.CompDirectory{
				CompCode: compCode, CompName: declaration.CompName,
				CompLevel: declaration.CompLevel, CompType: declaration.CompType,
				Organizer: declaration.Organizer, Undertaker: declaration.Undertaker,
				CollegeID: collegeID, ManagerID: declaration.ManagerID, Year: declaration.Year,
				Desc: declaration.Desc, CreatedBy: declaration.CreatedBy, Source: 2, DeclareID: &declaration.ID,
				ProjectOrigin: declaration.ProjectOrigin,
			}
			if err := tx.Create(&comp).Error; err != nil {
				return err
			}
			updates["comp_directory_id"] = comp.ID
			updates["audit_remark"] = "审核通过"
		}
		result := applyCollegeDeclarationScope(tx.Model(&declaration), scope).Updates(updates)
		if result.Error == nil && result.RowsAffected == 0 && scope.IsCollegeAdmin() {
			return errDeclarationStatusChanged
		}
		return result.Error
	})

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound) && scope.IsCollegeAdmin():
		utils.Forbidden(c, "无权审核该项目来源的赛事申报")
	case errors.Is(err, gorm.ErrRecordNotFound):
		utils.NotFound(c, "申报记录不存在")
	case errors.Is(err, errNotSubmitted):
		utils.Forbidden(c, err.Error())
	case errors.Is(err, errInvalidYear):
		utils.BadRequest(c, err.Error())
	case errors.Is(err, errDeclarationStatusChanged):
		utils.Conflict(c, err.Error())
	case err != nil:
		utils.InternalServerError(c, "审核失败", err)
	case req.AuditStatus == 2:
		clearCompListCache(c.Request.Context())
		utils.SuccessWithMessage(c, "申报审核通过", nil)
	default:
		utils.SuccessWithMessage(c, "申报审核拒绝", nil)
	}
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
