package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/logger"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func clearCompListCache(ctx context.Context) {
	rdb := middleware.GetRedisClient()
	iter := rdb.Scan(ctx, 0, "cache:comp_list:*", 0).Iterator()
	for iter.Next(ctx) {
		rdb.Del(ctx, iter.Val())
	}
}

func levelPrefix(level string) (string, bool) {
	switch strings.TrimSpace(level) {
	case "国家级":
		return "G", true
	case "省级":
		return "S", true
	case "校级":
		return "X", true
	case "国际级":
		return "I", true
	default:
		return "", false
	}
}

func resolveCompetitionYear(yearText string) int {
	year := 0
	if strings.TrimSpace(yearText) != "" {
		fmt.Sscanf(yearText, "%d", &year)
	}
	if year <= 0 {
		year = time.Now().Year()
	}
	return year
}

func nextCompetitionCode(tx *gorm.DB, level string, year int) (string, error) {
	prefix, ok := levelPrefix(level)
	if !ok {
		return "", fmt.Errorf("不支持的竞赛级别: %s", level)
	}

	base := fmt.Sprintf("%s%04d", prefix, year)

	var latest models.CompDirectory
	err := tx.Unscoped().Model(&models.CompDirectory{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("comp_code").
		Where("comp_code LIKE ?", base+"%").
		Order("comp_code DESC").
		Limit(1).
		Take(&latest).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	seq := 1
	lastCode := latest.CompCode
	if lastCode != "" && len(lastCode) >= len(base)+3 {
		suffix := lastCode[len(lastCode)-3:]
		if n, convErr := strconv.Atoi(suffix); convErr == nil {
			seq = n + 1
		}
	}

	return fmt.Sprintf("%s%03d", base, seq), nil
}

func hasCompetitionStarted(ctx context.Context, compID uint, now time.Time) (bool, error) {
	var detail models.CompDetail
	err := database.DB.WithContext(ctx).Select("comp_id", "comp_start_time").Where("comp_id = ?", compID).First(&detail).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	if detail.CompStartTime.IsZero() {
		return false, nil
	}

	return !now.Before(detail.CompStartTime), nil
}

type CompListReq struct {
	Page      int    `form:"page" binding:"required,min=1"`
	PageSize  int    `form:"page_size" binding:"required,min=1"`
	CompName  string `form:"comp_name"`
	Manager   string `form:"manager"`
	Status    string `form:"status"`
	CompLevel string `form:"comp_level"`
	College   string `form:"college"`
	Year      string `form:"year"`
	IsMy      bool   `form:"is_my"`
	IsReg     bool   `form:"is_reg"`
}

func GetCompetitionList(c *gin.Context) {
	var req CompListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	reqBytes, _ := json.Marshal(req)
	cacheKey := fmt.Sprintf("cache:comp_list:%x", md5.Sum(reqBytes))
	rdb := middleware.GetRedisClient()
	ctx := c.Request.Context()

	cacheData, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		var resp gin.H
		if json.Unmarshal([]byte(cacheData), &resp) == nil {
			logger.Info("赛事列表缓存命中", "cache_key", cacheKey)
			utils.Success(c, resp)
			return
		}
	}
	logger.Debug("赛事列表缓存未命中", "cache_key", cacheKey)
	query := database.DB.WithContext(c.Request.Context()).Model(&models.CompDirectory{})

	if req.CompName != "" {
		query = query.Where("comp_name LIKE ?", "%"+req.CompName+"%")
	}

	if req.Manager != "" {
		query = query.Where("manager_id IN (SELECT id FROM users WHERE realname LIKE ?)", "%"+req.Manager+"%")
	}

	if req.Status != "" && req.Status != "all" {
		switch req.Status {
		case "upcoming", "未开始":
			query = query.Where("status = 0")
		case "ongoing", "进行中":
			query = query.Where("status = 1")
		case "ended", "已结束":
			query = query.Where("status = 2")
		}
	}

	if req.CompLevel != "" && req.CompLevel != "全部" {
		query = query.Where("comp_level = ?", req.CompLevel)
	}

	if req.College != "" {
		query = query.Joins("LEFT JOIN colleges ON comp_directories.college_id = colleges.id").Where("colleges.name = ?", req.College)
	}

	if req.Year != "" {
		query = query.Where("YEAR(create_time) = ?", req.Year)
	}

	if req.IsMy {
		scope, ok := requireUserAccessScope(c)
		if !ok {
			return
		}
		query = applyCompetitionScope(query, scope, "comp_directories")
		if !scope.IsSchoolAdmin() {
			query = query.Preload("Detail", func(db *gorm.DB) *gorm.DB {
				return db.Select("comp_id", "reg_start_time", "reg_end_time", "participant_type")
			})
		}

		query = query.Preload("Detail", func(db *gorm.DB) *gorm.DB {
			return db.Select("comp_id", "reg_start_time", "reg_end_time", "participant_type")
		})
	}

	if req.IsReg {
		query = query.Preload("Detail")
		if !req.IsMy {
			query = query.Where("EXISTS (SELECT 1 FROM " +
				"comp_details WHERE comp_details.comp_id = comp_directories.id " +
				"AND YEAR(comp_details.reg_start_time) > 1970 AND YEAR(comp_details.reg_end_time) > 1970)")
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	var list []models.CompDirectory
	offset := (req.Page - 1) * req.PageSize

	if err := query.Preload("Detail").Preload("Manager").Preload("CollegeInfo").Order("create_time desc").Offset(offset).Limit(req.PageSize).Find(&list).Error; err != nil {
		utils.InternalServerError(c, "查询数据失败", err)
		return
	}
	responseData := gin.H{
		"list":  list,
		"total": total,
		"page":  req.Page,
		"size":  req.PageSize,
	}
	go func() {
		asyncCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		data, err := json.Marshal(responseData)
		if err != nil {
			logger.Error("异步缓存JSON序列化失败", "cache_key", cacheKey, "error", err)
			return
		}
		if err := rdb.Set(asyncCtx, cacheKey, data, 5*time.Minute).Err(); err != nil {
			logger.Error("异步缓存写入失败", "cache_key", cacheKey, "error", err)
		}
	}()
	utils.Success(c, responseData)

}

type CreateCompetitionReq struct {
	CompName   string `json:"comp_name" binding:"required"`
	CompLevel  string `json:"comp_level" binding:"required"`
	CompType   string `json:"comp_type"`
	Organizer  string `json:"organizer"`
	Undertaker string `json:"undertaker"`
	ManagerID  uint   `json:"manager_id" binding:"required"`
	College    string `json:"college"`
	Desc       string `json:"desc"`
	Year       string `json:"year"`
}

func resolveCollegeIDByName(tx *gorm.DB, collegeName string) (*uint, error) {
	name := strings.TrimSpace(collegeName)
	if name == "" {
		return nil, nil
	}

	var college models.College
	if err := tx.Where("name = ?", name).First(&college).Error; err != nil {
		return nil, fmt.Errorf("学院不存在: %s", name)
	}

	collegeID := college.ID
	return &collegeID, nil
}

func CreateCompetition(c *gin.Context) {
	var req CreateCompetitionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	year := resolveCompetitionYear(req.Year)

	var created models.CompDirectory
	var promotions []RolePromotion
	err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		collegeID, collegeErr := resolveCollegeIDByName(tx, req.College)
		if collegeErr != nil {
			return collegeErr
		}

		compCode, codeErr := nextCompetitionCode(tx, req.CompLevel, year)
		if codeErr != nil {
			return codeErr
		}

		compDir := models.CompDirectory{
			CompCode:   compCode,
			CompName:   req.CompName,
			CompLevel:  req.CompLevel,
			CompType:   req.CompType,
			Organizer:  req.Organizer,
			Undertaker: req.Undertaker,
			ManagerID:  req.ManagerID,
			CollegeID:  collegeID,
			Year:       year,
			Desc:       req.Desc,
			Status:     0,
		}

		if createErr := tx.Create(&compDir).Error; createErr != nil {
			return createErr
		}

		// 选中的负责人若为教师，自动提升为赛事负责人（与赛事创建同事务）。
		// 该接口仅 school_admin 持有 comp:create，无越权提升风险。
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

		created = compDir
		return nil
	})

	if err != nil {
		utils.InternalServerError(c, "创建失败", err)
		return
	}
	clearCompListCache(c.Request.Context())
	sourceID := created.ID
	finalizeRolePromotions(c, promotions, auditReasonCompetition, &sourceID)
	utils.SuccessWithMessage(c, "创建成功", gin.H{
		"competition":    created,
		"promoted_users": promotedUsersResp(c, promotions),
	})
}

func BatchImportCompetition(c *gin.Context) {
	var req struct {
		Items []CreateCompetitionReq `json:"items" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	// 校验负责人非 0（validator 对 slice 元素不递归校验，required 在批量路径不生效）。
	// 未匹配到负责人的行应在前端补充完整，而不是以 0 落库。
	for i, item := range req.Items {
		if item.ManagerID == 0 {
			utils.BadRequest(c, fmt.Sprintf("第%d项(%s)缺少赛事负责人", i+1, item.CompName))
			return
		}
	}

	var compDirs []models.CompDirectory
	var promotions []RolePromotion
	err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		promotedSet := make(map[uint]bool)
		for i, item := range req.Items {
			collegeID, collegeErr := resolveCollegeIDByName(tx, item.College)
			if collegeErr != nil {
				return fmt.Errorf("第%d项(%s): %w", i+1, item.CompName, collegeErr)
			}

			year := resolveCompetitionYear(item.Year)
			compCode, codeErr := nextCompetitionCode(tx, item.CompLevel, year)
			if codeErr != nil {
				return fmt.Errorf("第%d项(%s): 生成编号失败: %w", i+1, item.CompName, codeErr)
			}

			compDir := models.CompDirectory{
				CompCode:   compCode,
				CompName:   item.CompName,
				CompLevel:  item.CompLevel,
				CompType:   item.CompType,
				Organizer:  item.Organizer,
				Undertaker: item.Undertaker,
				ManagerID:  item.ManagerID,
				CollegeID:  collegeID,
				Year:       year,
				Desc:       item.Desc,
				Status:     0,
			}

			if createErr := tx.Create(&compDir).Error; createErr != nil {
				return fmt.Errorf("第%d项(%s): 创建失败: %w", i+1, item.CompName, createErr)
			}

			compDirs = append(compDirs, compDir)

			// 同一批导入中同一负责人只提升一次；教师自动提升为赛事负责人。
			if !promotedSet[item.ManagerID] {
				oldRoleID, newRoleID, promoteErr := promoteCompetitionManagerIfTeacher(tx, item.ManagerID)
				if promoteErr != nil {
					return promoteErr
				}
				if oldRoleID != 0 {
					promotedSet[item.ManagerID] = true
					promotions = append(promotions, RolePromotion{
						UserID:    item.ManagerID,
						OldRoleID: oldRoleID,
						NewRoleID: newRoleID,
					})
				}
			}
		}
		return nil
	})

	if err != nil {
		utils.InternalServerError(c, "导入失败", err)
		return
	}

	finalizeRolePromotions(c, promotions, auditReasonImport, nil)
	utils.SuccessWithMessage(c, "批量导入成功", gin.H{
		"count":          len(compDirs),
		"promoted_users": promotedUsersResp(c, promotions),
	})
}

type ManagerListReq struct {
	Name        string `form:"name"`
	WorkID      string `form:"work_id"`
	College     string `form:"college"`
	ManagerOnly bool   `form:"manager_only"` // 仅看已是赛事负责人（可选筛选）
	Page        int    `form:"page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1"`
}

type ManagerResp struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	WorkID   string `json:"work_id"`
	College  string `json:"college"`
	RoleCode string `json:"role_code"` // 当前角色：teacher / competition_manager，供前端展示提权预判
}

func GetManagerList(c *gin.Context) {
	var req ManagerListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	// 负责人池 = 教职工身份，或已持有 competition_manager 角色；
	// 排除管理员（school_admin / college_admin）与专家（expert）、访客（guest）。
	query := database.DB.WithContext(c.Request.Context()).Model(&models.User{}).
		Where("users.delete_time IS NULL").
		Where(`(
			users.identity_type = 'staff'
			OR EXISTS (
				SELECT 1 FROM user_roles ur
				JOIN roles r ON r.id = ur.role_id AND r.delete_time IS NULL
				WHERE ur.user_id = users.id AND r.role_code = 'competition_manager'
			)
		)`).
		Where(`NOT EXISTS (
			SELECT 1 FROM user_roles ur2
			JOIN roles r2 ON r2.id = ur2.role_id AND r2.delete_time IS NULL
			WHERE ur2.user_id = users.id
				AND r2.role_code IN ('school_admin', 'college_admin')
		)`).
		Where(`NOT EXISTS (
			SELECT 1 FROM user_roles ur3
			JOIN roles r3 ON r3.id = ur3.role_id AND r3.delete_time IS NULL
			WHERE ur3.user_id = users.id
				AND r3.role_code IN ('expert', 'guest')
		)`)

	if req.ManagerOnly {
		query = query.Where(`EXISTS (
			SELECT 1 FROM user_roles ur4
			JOIN roles r4 ON r4.id = ur4.role_id AND r4.delete_time IS NULL
			WHERE ur4.user_id = users.id AND r4.role_code = 'competition_manager'
		)`)
	}

	if req.Name != "" {
		query = query.Where("users.realname LIKE ?", "%"+req.Name+"%")
	}

	if req.WorkID != "" {
		// 精确匹配：供 Excel 批量导入按工号自动匹配使用，避免 LIKE 误命中子串。
		query = query.Where("users.username = ?", req.WorkID)
	}

	if req.College != "" {
		// 注意：users.college 实际存的是部门名称（与 departments 表对应），不是学院。
		query = query.Where("users.college = ?", req.College)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	offset := (req.Page - 1) * req.PageSize
	var users []models.User
	if err := query.
		Select("users.id", "users.realname", "users.username", "users.college").
		Order("users.id").
		Offset(offset).
		Limit(req.PageSize).
		Find(&users).Error; err != nil {
		utils.InternalServerError(c, "查询用户数据失败", err)
		return
	}

	// 批量取本页用户的当前角色（每人至多一个角色，由唯一索引保证）。
	roleByUser := make(map[uint]string, len(users))
	if len(users) > 0 {
		ids := make([]uint, 0, len(users))
		for _, u := range users {
			ids = append(ids, u.ID)
		}
		var roleRows []struct {
			UserID   uint
			RoleCode string
		}
		if err := database.DB.WithContext(c.Request.Context()).
			Table("user_roles").
			Select("user_roles.user_id AS user_id, roles.role_code AS role_code").
			Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.delete_time IS NULL").
			Where("user_roles.user_id IN ?", ids).
			Scan(&roleRows).Error; err != nil {
			// 角色列仅为前端预判展示，查询失败不阻断列表，但记录日志便于发现
			slog.Warn("查询负责人当前角色失败", "error", err)
		}
		for _, rr := range roleRows {
			roleByUser[rr.UserID] = rr.RoleCode
		}
	}

	managers := make([]ManagerResp, 0, len(users))
	for _, user := range users {
		managers = append(managers, ManagerResp{
			ID:       user.ID,
			Name:     user.Realname,
			WorkID:   user.Username,
			College:  user.College,
			RoleCode: roleByUser[user.ID],
		})
	}

	utils.Success(c, gin.H{
		"list":  managers,
		"total": total,
		"page":  req.Page,
		"size":  req.PageSize,
	})
}

func GetCompetitionYears(c *gin.Context) {
	var years []int
	if err := database.DB.WithContext(c.Request.Context()).
		Model(&models.CompDirectory{}).
		Where("year > ?", 0).
		Distinct("year").
		Order("year DESC").
		Pluck("year", &years).Error; err != nil {
		utils.InternalServerError(c, "查询年份失败", err)
		return
	}

	utils.Success(c, gin.H{"years": years})
}

func DeleteCompetition(c *gin.Context) {
	id := c.Param("id")

	var comp models.CompDirectory
	if err := database.DB.WithContext(c.Request.Context()).First(&comp, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "赛事不存在")
		} else {
			utils.InternalServerError(c, "查询失败", err)
		}
		return
	}

	started, err := hasCompetitionStarted(c.Request.Context(), comp.ID, time.Now())
	if err != nil {
		utils.InternalServerError(c, "校验赛事时间失败", err)
		return
	}
	if started {
		utils.BadRequest(c, "赛事已开始，不能删除")
		return
	}

	if err := database.DB.WithContext(c.Request.Context()).Delete(&comp).Error; err != nil {
		utils.InternalServerError(c, "删除失败", err)
		return
	}
	clearCompListCache(c.Request.Context())
	utils.SuccessWithMessage(c, "删除成功", nil)
}

func RestoreCompetition(c *gin.Context) {
	id := c.Param("id")

	var comp models.CompDirectory
	if err := database.DB.WithContext(c.Request.Context()).Unscoped().First(&comp, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "赛事不存在")
		} else {
			utils.InternalServerError(c, "���询失败", err)
		}
		return
	}

	if comp.DeletedAt.Time.IsZero() {
		utils.BadRequest(c, "赛事未被删除")
		return
	}

	if err := database.DB.WithContext(c.Request.Context()).Model(&comp).Update("delete_time", nil).Error; err != nil {
		utils.InternalServerError(c, "恢复失败", err)
		return
	}
	clearCompListCache(c.Request.Context())

	// 幂等兜底：恢复的赛事负责人若为教师则自动提升（覆盖「删除前被回收」的场景；
	// 因本次不做自动降级，通常为 no-op）。
	var promotions []RolePromotion
	err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		oldRoleID, newRoleID, promoteErr := promoteCompetitionManagerIfTeacher(tx, comp.ManagerID)
		if promoteErr != nil {
			return promoteErr
		}
		if oldRoleID != 0 {
			promotions = append(promotions, RolePromotion{
				UserID:    comp.ManagerID,
				OldRoleID: oldRoleID,
				NewRoleID: newRoleID,
			})
		}
		return nil
	})
	if err != nil {
		utils.InternalServerError(c, "恢复负责人角色失败", err)
		return
	}
	sourceID := comp.ID
	finalizeRolePromotions(c, promotions, auditReasonCompetition, &sourceID)
	utils.SuccessWithMessage(c, "恢复成功", gin.H{
		"promoted_users": promotedUsersResp(c, promotions),
	})
}

type BatchDeleteReq struct {
	IDs []uint `json:"ids" binding:"required,min=1"`
}

func BatchDeleteCompetition(c *gin.Context) {
	var req BatchDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	now := time.Now()
	var comps []models.CompDirectory
	if err := database.DB.WithContext(c.Request.Context()).Select("id", "comp_name").Where("id IN ?", req.IDs).Find(&comps).Error; err != nil {
		utils.InternalServerError(c, "查询赛事失败", err)
		return
	}

	if len(comps) > 0 {
		var details []models.CompDetail
		if err := database.DB.WithContext(c.Request.Context()).Select("comp_id", "comp_start_time").Where("comp_id IN ?", req.IDs).Find(&details).Error; err != nil {
			utils.InternalServerError(c, "校验赛事时间失败", err)
			return
		}

		startedCompIDs := make(map[uint]bool, len(details))
		for _, detail := range details {
			if !detail.CompStartTime.IsZero() && !now.Before(detail.CompStartTime) {
				startedCompIDs[detail.CompID] = true
			}
		}

		startedCompNames := make([]string, 0)
		for _, comp := range comps {
			if startedCompIDs[comp.ID] {
				startedCompNames = append(startedCompNames, comp.CompName)
			}
		}

		if len(startedCompNames) > 0 {
			utils.BadRequest(c, fmt.Sprintf("以下赛事已开始，不能删除：%s", strings.Join(startedCompNames, "、")))
			return
		}
	}

	result := database.DB.WithContext(c.Request.Context()).Delete(&models.CompDirectory{}, req.IDs)
	if result.Error != nil {
		utils.InternalServerError(c, "删除失败", result.Error)
		return
	}
	clearCompListCache(c.Request.Context())
	utils.Success(c, gin.H{"deleted_count": result.RowsAffected})
}

func GetCompetitionDetail(c *gin.Context) {
	id := c.Param("id")
	var comp models.CompDirectory

	if err := database.DB.WithContext(c.Request.Context()).Preload("Manager").Preload("CollegeInfo").First(&comp, id).Error; err != nil {
		utils.NotFound(c, "赛事不存在")
		return
	}

	utils.Success(c, comp)
}

type UpdateCompetitionReq struct {
	CompName   string `json:"comp_name" binding:"required"`
	CompLevel  string `json:"comp_level" binding:"required"`
	CompType   string `json:"comp_type"`
	Organizer  string `json:"organizer"`
	Undertaker string `json:"undertaker"`
	ManagerID  uint   `json:"manager_id" binding:"required"`
	College    string `json:"college"`
	Desc       string `json:"desc"`
	Year       string `json:"year"`
}

func UpdateCompetition(c *gin.Context) {
	id := c.Param("id")
	var req UpdateCompetitionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	var comp models.CompDirectory
	if err := database.DB.WithContext(c.Request.Context()).First(&comp, id).Error; err != nil {
		utils.NotFound(c, "赛事不存在")
		return
	}

	// P0-1 范围校验：school_admin 全部；college_admin 限本学院；其他角色限自己负责的赛事。
	if !requireCompetitionAccessIfScoped(c, comp.ID) {
		return
	}

	// 改派约束（已决策）：仅 school_admin / college_admin 可变更负责人；
	// competition_manager 等其他角色提交不同负责人时拒绝（防 API 直调提权）。
	// 本次不做自动降级：改派只提升新负责人，原负责人角色保留。
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return
	}
	canReassign := scope.IsSchoolAdmin() || scope.IsCollegeAdmin()
	managerChanged := req.ManagerID != comp.ManagerID
	if managerChanged && !canReassign {
		utils.Forbidden(c, "仅管理员可以变更赛事负责人")
		return
	}

	collegeID, collegeErr := resolveCollegeIDByName(database.DB.WithContext(c.Request.Context()), req.College)
	if collegeErr != nil {
		utils.BadRequest(c, collegeErr.Error())
		return
	}

	year := 0
	if req.Year != "" {
		fmt.Sscanf(req.Year, "%d", &year)
	}

	var promotions []RolePromotion
	err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"comp_name":  req.CompName,
			"comp_level": req.CompLevel,
			"comp_type":  req.CompType,
			"organizer":  req.Organizer,
			"undertaker": req.Undertaker,
			"manager_id": req.ManagerID,
			"college_id": nil,
			"year":       year,
			"desc":       req.Desc,
		}
		if collegeID != nil {
			updates["college_id"] = *collegeID
		}

		if err := tx.Model(&comp).Updates(updates).Error; err != nil {
			return err
		}

		// 仅提升新负责人（若为教师）；原负责人角色保留（不做自动降级）。
		if managerChanged && req.ManagerID != 0 {
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
	clearCompListCache(c.Request.Context())
	sourceID := comp.ID
	finalizeRolePromotions(c, promotions, auditReasonCompetition, &sourceID)
	utils.SuccessWithMessage(c, "更新成功", gin.H{
		"promoted_users": promotedUsersResp(c, promotions),
	})
}
