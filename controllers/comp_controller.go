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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const competitionListCacheVersionKey = "cache:comp_list_version"
const pendingCompetitionCodePrefix = "P"

func clearCompListCache(ctx context.Context) {
	rdb := middleware.GetRedisClient()
	if rdb == nil {
		return
	}
	// Readers that started before the mutation can only populate the old version.
	if err := rdb.Incr(ctx, competitionListCacheVersionKey).Err(); err != nil {
		logger.Error("赛事目录缓存失效失败", "error", err)
	}
	iter := rdb.Scan(ctx, 0, "cache:stats:dashboard:*", 0).Iterator()
	for iter.Next(ctx) {
		rdb.Del(ctx, iter.Val())
	}
}

func levelPrefix(level string) (string, bool) {
	switch strings.TrimSpace(level) {
	case "国家级":
		return "G", true
	case "省级", "省部级":
		return "S", true
	case "校级":
		return "X", true
	case "国际级":
		return "I", true
	default:
		return "", false
	}
}

func resolveCompetitionYear(yearText string, defaultYear int) (int, bool) {
	if strings.TrimSpace(yearText) == "" {
		return defaultYear, validCompetitionYear(defaultYear)
	}
	return parseDirectoryYear(strings.TrimSpace(yearText))
}

func competitionCodeBase(level string, year int) (string, error) {
	prefix, ok := levelPrefix(level)
	if !ok {
		return "", fmt.Errorf("不支持的竞赛级别: %s", level)
	}
	return fmt.Sprintf("%s%04d", prefix, year), nil
}

func latestCompetitionSequence(tx *gorm.DB, base string) (int, error) {
	var latest models.CompDirectory
	err := tx.Unscoped().Model(&models.CompDirectory{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("comp_code").
		Where("comp_code LIKE ?", base+"%").
		// 序号跨位（999→1000）后纯字符串排序会把 X2026999 排在 X20261000 之后，
		// 导致生成已存在的编号、创建赛事持续 500。按长度优先排序保证数值序。
		Order("LENGTH(comp_code) DESC, comp_code DESC").
		Limit(1).
		Take(&latest).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	seq := 0
	if latest.CompCode != "" && len(latest.CompCode) > len(base) {
		// 取 base 之后的完整序号段（旧实现取末 3 位，4 位序号会被截断成 000）
		suffix := latest.CompCode[len(base):]
		if n, convErr := strconv.Atoi(suffix); convErr == nil {
			seq = n
		}
	}
	return seq, nil
}

func nextCompetitionCode(tx *gorm.DB, level string, year int) (string, error) {
	base, err := competitionCodeBase(level, year)
	if err != nil {
		return "", err
	}
	seq, err := latestCompetitionSequence(tx, base)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%03d", base, seq+1), nil
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

type CompetitionFilters struct {
	CompName  string `form:"comp_name"`
	Manager   string `form:"manager"`
	Status    string `form:"status"`
	CompLevel string `form:"comp_level"`
	College   string `form:"college"`
	Year      string `form:"year"`
}

type CompListReq struct {
	CompetitionFilters
	Page     int  `form:"page" binding:"required,min=1"`
	PageSize int  `form:"page_size" binding:"required,min=1"`
	IsMy     bool `form:"is_my"`
	IsReg    bool `form:"is_reg"`
}

func GetCompetitionList(c *gin.Context) {
	var req CompListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	reqBytes, _ := json.Marshal(req)
	rdb := middleware.GetRedisClient()
	ctx := c.Request.Context()

	role, _ := c.Get("role_code")
	// Management responses must reflect reassignment immediately; cache browsing data only.
	useCache := rdb != nil && role != "school_admin" && role != "college_admin" && role != "competition_manager"
	var cacheKey, cacheData string
	if useCache {
		version, err := rdb.Get(ctx, competitionListCacheVersionKey).Result()
		useCache = err == nil || errors.Is(err, redis.Nil)
		if useCache {
			cacheKey = fmt.Sprintf("cache:comp_list:project_origin_v1:%s:%s:%x", version, competitionCacheIdentity(c), md5.Sum(reqBytes))
			cacheData, _ = rdb.Get(ctx, cacheKey).Result()
		}
	}
	if cacheData != "" {
		var resp struct {
			List  []models.CompDirectory `json:"list"`
			Total int64                  `json:"total"`
			Page  int                    `json:"page"`
			Size  int                    `json:"size"`
		}
		if json.Unmarshal([]byte(cacheData), &resp) == nil {
			if !attachCompetitionCapabilities(c, resp.List) {
				return
			}
			logger.Info("赛事列表缓存命中", "cache_key", cacheKey)
			utils.Success(c, resp)
			return
		}
	}
	logger.Debug("赛事列表缓存未命中", "cache_key", cacheKey)
	query, ok := competitionDirectoryQuery(c, req)
	if !ok {
		return
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	var list []models.CompDirectory
	offset := (req.Page - 1) * req.PageSize

	if err := query.Preload("Detail").Preload("Manager").Preload("CollegeInfo").Order("comp_directories.create_time DESC, comp_directories.id DESC").Offset(offset).Limit(req.PageSize).Find(&list).Error; err != nil {
		utils.InternalServerError(c, "查询数据失败", err)
		return
	}
	responseData := gin.H{
		"list":  list,
		"total": total,
		"page":  req.Page,
		"size":  req.PageSize,
	}
	data, marshalErr := json.Marshal(responseData)
	if useCache && marshalErr == nil {
		go func() {
			asyncCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := rdb.Set(asyncCtx, cacheKey, data, 5*time.Minute).Err(); err != nil {
				logger.Error("异步缓存写入失败", "cache_key", cacheKey, "error", err)
			}
		}()
	}
	if !attachCompetitionCapabilities(c, list) {
		return
	}
	utils.Success(c, responseData)
}

func competitionDirectoryQuery(c *gin.Context, req CompListReq) (*gorm.DB, bool) {
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
		query = query.Where(`EXISTS (SELECT 1 FROM users unit_manager
			WHERE unit_manager.id = comp_directories.manager_id AND unit_manager.delete_time IS NULL AND unit_manager.college = ?)`, req.College)
	}

	if req.Year != "" {
		query = query.Where("comp_directories.year = ?", req.Year)
	}

	// 2026-11-26 修复：赛事负责人只看自己负责的赛事（之前不传 is_my 会看到全部）。
	// 非校级管理员一律按 scope 过滤：college_admin 看本单位，competition_manager 看自己负责的。
	role := c.GetString("role_code")
	if req.IsMy || role == "college_admin" || role == "competition_manager" {
		scope, ok := requireUserAccessScope(c)
		if !ok {
			return nil, false
		}
		query = applyCompetitionScope(query, scope, "comp_directories")
	}

	if req.IsReg && !req.IsMy {
		query = query.Where("EXISTS (SELECT 1 FROM " +
			"comp_details WHERE comp_details.comp_id = comp_directories.id " +
			"AND YEAR(comp_details.reg_start_time) > 1970 AND YEAR(comp_details.reg_end_time) > 1970)")
	}

	return query, true
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

// unitBaseName 去掉单位名的括号补充说明。
// 人事系统（users.college）习惯写简称"教务处"，机构表写全称"教务处（含非学历教育办学管理办公室）"，
// 两侧命名习惯不同，直接等值比较必然落空。
func unitBaseName(unit string) string {
	if i := strings.IndexAny(unit, "（("); i > 0 {
		return strings.TrimSpace(unit[:i])
	}
	return unit
}

// Some departments only exist in the staff directory. Their names still
// display from the manager profile even when no college foreign key exists.
//
// 先按全名精确匹配（既有行为，优先保证不误判）；未命中时再用括号前的主名兜底，
// 让简称能落到对应的有效机构上。兜底命中多条（主名相同但确属不同机构）时不猜，仍返回 nil，
// 避免把一个学院的数据错误授权给另一个学院。
func resolveUnitCollegeID(tx *gorm.DB, unit string) (*uint, error) {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return nil, nil
	}
	var college models.College
	err := tx.Select("id").Where("name = ? AND is_valid = ?", unit, true).First(&college).Error
	if err == nil {
		return &college.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	base := unitBaseName(unit)
	if base == "" {
		return nil, nil
	}
	var matches []models.College
	if queryErr := tx.Select("id").
		Where("is_valid = ?", true).
		Where("SUBSTRING_INDEX(SUBSTRING_INDEX(name, '（', 1), '(', 1) = ?", base).
		Limit(2).
		Find(&matches).Error; queryErr != nil {
		return nil, queryErr
	}
	if len(matches) != 1 {
		return nil, nil
	}
	return &matches[0].ID, nil
}

func CreateCompetition(c *gin.Context) {
	var req CreateCompetitionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	req.CompName = strings.TrimSpace(req.CompName)
	if req.CompName == "" {
		utils.BadRequest(c, "赛事名称不能为空")
		return
	}
	year, validYear := resolveCompetitionYear(req.Year, time.Now().Year())
	if !validYear {
		utils.BadRequest(c, "所属年份须为1000至9999的4位数字")
		return
	}

	var created models.CompDirectory
	var promotions []RolePromotion
	err := database.TransactionWithDeadlockRetry(database.DB.WithContext(c.Request.Context()), func(tx *gorm.DB) error {
		promotions = nil
		origin, createdBy, err := projectOriginForRequest(c, tx)
		if err != nil {
			return err
		}
		manager, err := findCompetitionManager(c, tx, req.ManagerID)
		if err != nil {
			return err
		}
		var collegeID *uint
		if _, scoped := c.Get("role_code"); scoped {
			collegeID, err = resolveUnitCollegeID(tx, manager.College)
		} else {
			collegeID, err = resolveCollegeIDByName(tx, req.College)
		}
		if err != nil {
			return err
		}
		compCode, codeErr := nextCompetitionCode(tx, req.CompLevel, year)
		if codeErr != nil {
			return codeErr
		}

		compDir := models.CompDirectory{
			CompCode:      compCode,
			CompName:      req.CompName,
			CompLevel:     req.CompLevel,
			CompType:      req.CompType,
			Organizer:     req.Organizer,
			Undertaker:    req.Undertaker,
			ManagerID:     req.ManagerID,
			CollegeID:     collegeID,
			Year:          year,
			Desc:          req.Desc,
			Status:        0,
			CreatedBy:     createdBy,
			ProjectOrigin: origin,
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
		respondCompetitionMutationError(c, "创建失败", err)
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
		Items           []CreateCompetitionReq `json:"items" binding:"required"`
		AllowIncomplete bool                   `json:"allow_incomplete"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	if req.AllowIncomplete && c.GetString("role_code") != "school_admin" {
		utils.Forbidden(c, "仅校级管理员可以导入待补全赛事")
		return
	}

	years := make([]int, len(req.Items))
	defaultYear := time.Now().Year()
	for i := range req.Items {
		item := &req.Items[i]
		item.CompName = strings.TrimSpace(item.CompName)
		if item.CompName == "" {
			utils.BadRequest(c, fmt.Sprintf("第%d项赛事名称不能为空", i+1))
			return
		}
		if item.ManagerID == 0 && !req.AllowIncomplete {
			utils.BadRequest(c, fmt.Sprintf("第%d项(%s)缺少赛事负责人", i+1, item.CompName))
			return
		}
		item.CompLevel = strings.TrimSpace(item.CompLevel)
		year, valid := resolveCompetitionYear(item.Year, defaultYear)
		if !valid {
			utils.BadRequest(c, fmt.Sprintf("第%d项(%s): 所属年份须为1000至9999的4位数字", i+1, item.CompName))
			return
		}
		years[i] = year
	}

	var compDirs []models.CompDirectory
	var promotions []RolePromotion
	err := database.TransactionWithDeadlockRetry(database.DB.WithContext(c.Request.Context()), func(tx *gorm.DB) error {
		promotions = nil
		origin, createdBy, err := projectOriginForRequest(c, tx)
		if err != nil {
			return err
		}
		processedManagers := make(map[uint]bool)
		collegeIDs := make([]*uint, len(req.Items))
		codeGroups := make(map[string][]int)
		codeGroupBases := make([]string, 0)

		// Validate each distinct manager once and reuse the result for the unit lookup.
		_, managerValidationEnabled := c.Get("role_code")
		managerByID := make(map[uint]models.User)
		if managerValidationEnabled {
			managerIDs := make([]uint, 0, len(req.Items))
			seenManagerIDs := make(map[uint]struct{}, len(req.Items))
			for _, item := range req.Items {
				if item.ManagerID == 0 {
					continue
				}
				if _, exists := seenManagerIDs[item.ManagerID]; exists {
					continue
				}
				seenManagerIDs[item.ManagerID] = struct{}{}
				managerIDs = append(managerIDs, item.ManagerID)
			}

			if len(managerIDs) > 0 {
				var managers []models.User
				if err := competitionManagerCandidates(tx).
					Where("users.id IN ?", managerIDs).
					Preload("Roles").
					Find(&managers).Error; err != nil {
					return err
				}
				for _, manager := range managers {
					managerByID[manager.ID] = manager
				}
			}
		}

		unitFor := func(item CreateCompetitionReq) string {
			if managerValidationEnabled {
				return strings.TrimSpace(managerByID[item.ManagerID].College)
			}
			return strings.TrimSpace(item.College)
		}
		// Resolve all referenced colleges with one query.
		collegeNames := make([]string, 0)
		seenCollegeNames := make(map[string]struct{}, len(req.Items))
		for _, item := range req.Items {
			name := unitFor(item)
			if name == "" {
				continue
			}
			if _, exists := seenCollegeNames[name]; exists {
				continue
			}
			seenCollegeNames[name] = struct{}{}
			collegeNames = append(collegeNames, name)
		}
		collegeIDByName := make(map[string]uint, len(collegeNames))
		if len(collegeNames) > 0 {
			var colleges []models.College
			if err := tx.Where("name IN ? AND is_valid = ?", collegeNames, true).Find(&colleges).Error; err != nil {
				return err
			}
			for _, college := range colleges {
				collegeIDByName[college.Name] = college.ID
			}
		}

		for i, item := range req.Items {
			collegeName := unitFor(item)
			if collegeName != "" {
				collegeID, exists := collegeIDByName[collegeName]
				if exists {
					collegeIDs[i] = &collegeID
				} else if !managerValidationEnabled {
					return fmt.Errorf("第%d项(%s): 学院不存在: %s", i+1, item.CompName, collegeName)
				}
			}

			if managerValidationEnabled && item.ManagerID != 0 {
				manager, exists := managerByID[item.ManagerID]
				if !exists {
					return fmt.Errorf("第%d项: %w：用户不存在或不具备负责人资格", i+1, errInvalidCompetitionManager)
				}
				if len(manager.Roles) != 1 {
					return fmt.Errorf("第%d项: %w：角色配置异常", i+1, errInvalidCompetitionManager)
				}
			}

			base := fmt.Sprintf("%s%04d", pendingCompetitionCodePrefix, years[i])
			if !req.AllowIncomplete || item.CompLevel != "" {
				var codeErr error
				base, codeErr = competitionCodeBase(item.CompLevel, years[i])
				if codeErr != nil {
					return fmt.Errorf("第%d项(%s): %w", i+1, item.CompName, codeErr)
				}
			}
			if _, exists := codeGroups[base]; !exists {
				codeGroupBases = append(codeGroupBases, base)
			}
			codeGroups[base] = append(codeGroups[base], i)
		}

		// Lock and read the current sequence once per level/year, then assign codes in memory.
		// Sorting makes multi-group imports acquire locks in a stable order.
		sort.Strings(codeGroupBases)
		compDirs = make([]models.CompDirectory, len(req.Items))
		for _, base := range codeGroupBases {
			sequence, codeErr := latestCompetitionSequence(tx, base)
			if codeErr != nil {
				firstIndex := codeGroups[base][0]
				return fmt.Errorf("第%d项(%s): 生成编号失败: %w", firstIndex+1, req.Items[firstIndex].CompName, codeErr)
			}
			for _, i := range codeGroups[base] {
				sequence++
				item := req.Items[i]
				compDirs[i] = models.CompDirectory{
					CompCode:      fmt.Sprintf("%s%03d", base, sequence),
					CompName:      item.CompName,
					CompLevel:     item.CompLevel,
					CompType:      item.CompType,
					Organizer:     item.Organizer,
					Undertaker:    item.Undertaker,
					ManagerID:     item.ManagerID,
					CollegeID:     collegeIDs[i],
					CreatedBy:     createdBy,
					ProjectOrigin: origin,
					Year:          years[i],
					Desc:          item.Desc,
					Status:        0,
				}
			}
		}

		for start := 0; start < len(compDirs); {
			end := start + 1
			managerMissing := compDirs[start].ManagerID == 0
			for end < len(compDirs) && (compDirs[end].ManagerID == 0) == managerMissing {
				end++
			}
			insert := tx
			if managerMissing {
				// The nullable foreign key must use SQL NULL for unassigned managers.
				insert = tx.Omit("ManagerID")
			}
			batch := compDirs[start:end]
			if createErr := insert.CreateInBatches(&batch, 200).Error; createErr != nil {
				return fmt.Errorf("批量创建赛事失败: %w", createErr)
			}
			start = end
		}

		// 同一批导入中同一负责人只提升一次；教师自动提升为赛事负责人。
		for _, item := range req.Items {
			if item.ManagerID == 0 || processedManagers[item.ManagerID] {
				continue
			}
			processedManagers[item.ManagerID] = true
			oldRoleID, newRoleID, promoteErr := promoteCompetitionManagerIfTeacher(tx, item.ManagerID)
			if promoteErr != nil {
				return promoteErr
			}
			if oldRoleID != 0 {
				promotions = append(promotions, RolePromotion{
					UserID:    item.ManagerID,
					OldRoleID: oldRoleID,
					NewRoleID: newRoleID,
				})
			}
		}
		return nil
	})

	if err != nil {
		respondCompetitionMutationError(c, "导入失败", err)
		return
	}

	clearCompListCache(c.Request.Context())
	finalizeRolePromotions(c, promotions, auditReasonImport, nil)
	InvalidateStatisticsDashboard()
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
	RoleCode string `json:"role_code"` // 当前角色：teacher / competition_manager / college_admin / school_admin，供前端展示角色
}

type ManagerLookupReq struct {
	WorkIDs []string `json:"work_ids" binding:"required,min=1,max=5000,dive,required"`
}

// LookupManagersByWorkIDs 批量按工号查询负责人，供赛事 Excel 导入使用，避免逐条请求。
func LookupManagersByWorkIDs(c *gin.Context) {
	var req ManagerLookupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "工号列表参数错误")
		return
	}

	workIDs := make([]string, 0, len(req.WorkIDs))
	seen := make(map[string]struct{}, len(req.WorkIDs))
	for _, workID := range req.WorkIDs {
		workID = strings.TrimSpace(workID)
		if workID == "" {
			continue
		}
		if _, exists := seen[workID]; exists {
			continue
		}
		seen[workID] = struct{}{}
		workIDs = append(workIDs, workID)
	}
	if len(workIDs) == 0 {
		utils.Success(c, gin.H{"list": []ManagerResp{}})
		return
	}

	query := competitionManagerCandidates(database.DB.WithContext(c.Request.Context())).Where("users.username IN ?", workIDs)

	var users []models.User
	if err := query.Select("users.id", "users.realname", "users.username", "users.college").
		Order("users.id").Find(&users).Error; err != nil {
		utils.InternalServerError(c, "批量查询负责人失败", err)
		return
	}

	roleByUser := make(map[uint]string, len(users))
	if len(users) > 0 {
		ids := make([]uint, 0, len(users))
		for _, user := range users {
			ids = append(ids, user.ID)
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
			utils.InternalServerError(c, "批量查询负责人角色失败", err)
			return
		}
		for _, row := range roleRows {
			roleByUser[row.UserID] = row.RoleCode
		}
	}

	managers := make([]ManagerResp, 0, len(users))
	for _, user := range users {
		managers = append(managers, ManagerResp{
			ID: user.ID, Name: user.Realname, WorkID: user.Username,
			College: user.College, RoleCode: roleByUser[user.ID],
		})
	}
	utils.Success(c, gin.H{"list": managers})
}

func GetManagerList(c *gin.Context) {
	var req ManagerListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	// 教师、赛事负责人、院管理员共用候选规则。
	query := competitionManagerCandidates(database.DB.WithContext(c.Request.Context()))

	if req.ManagerOnly {
		query = query.Where(`EXISTS (SELECT 1 FROM comp_directories cd WHERE cd.manager_id = users.id AND cd.delete_time IS NULL)
			OR EXISTS (SELECT 1 FROM comp_declarations d WHERE d.manager_id = users.id AND d.delete_time IS NULL)
			OR EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id AND r.delete_time IS NULL WHERE ur.user_id = users.id AND r.role_code = 'competition_manager')`)
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
	years := make([]int, 0)
	// 合并赛事年份，使新增、导入、申报审核及编辑产生的年份立即可见。
	// 不过滤软删除赛事，保留其恢复时需要的年度目录。
	if err := database.DB.WithContext(c.Request.Context()).Raw(`
		SELECT year FROM competition_years
		UNION SELECT year FROM comp_directories WHERE year > 0
		ORDER BY year DESC`).Scan(&years).Error; err != nil {
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
	var promotions []RolePromotion
	errActiveCompetition := errors.New("赛事未被删除")
	err := database.TransactionWithDeadlockRetry(database.DB.WithContext(c.Request.Context()), func(tx *gorm.DB) error {
		comp = models.CompDirectory{}
		promotions = nil
		if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&comp, id).Error; err != nil {
			return err
		}
		if !comp.DeletedAt.Valid {
			return errActiveCompetition
		}
		result := tx.Unscoped().Model(&comp).Where("delete_time IS NOT NULL").Update("delete_time", nil)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errActiveCompetition
		}
		// 恢复时重新确认负责人角色，与恢复记录一起提交。
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
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		utils.NotFound(c, "赛事不存在")
		return
	case errors.Is(err, errActiveCompetition):
		utils.BadRequest(c, err.Error())
		return
	case err != nil:
		utils.InternalServerError(c, "恢复失败", err)
		return
	}
	clearCompListCache(c.Request.Context())
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
	if c.GetString("role_code") == "college_admin" {
		_, _, ok := requireCompetitionAccess(c, comp.ID)
		if !ok {
			return
		}
	}

	caps, ok := competitionCapabilities(c, comp)
	if !ok {
		return
	}
	comp.Capabilities = caps
	utils.Success(c, comp)
}

type UpdateCompetitionReq struct {
	AllowIncomplete bool   `json:"allow_incomplete"`
	CompName        string `json:"comp_name" binding:"required"`
	CompLevel       string `json:"comp_level" binding:"required_unless=AllowIncomplete true"`
	CompType        string `json:"comp_type"`
	Organizer       string `json:"organizer"`
	Undertaker      string `json:"undertaker"`
	ManagerID       uint   `json:"manager_id" binding:"required_unless=AllowIncomplete true"`
	College         string `json:"college"`
	Desc            string `json:"desc"`
	Year            string `json:"year"`
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

	// 院管理员按项目来源授权，赛事负责人按本人负责的赛事授权。
	if !requireCompetitionAccessIfScoped(c, comp.ID) {
		return
	}

	// 正式赛事仅校级管理员可以改派负责人；院管理员兼任仍只可编辑基础信息。
	// 院管理员在申报阶段沿用原有负责人修改流程。
	// 本次不做自动降级：改派只提升新负责人，原负责人角色保留。
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return
	}
	if !requireCompetitionWrite(c, scope, comp, "comp:update") {
		return
	}
	canReassign := scope.IsSchoolAdmin()
	if req.AllowIncomplete && !canReassign && !scope.IsCollegeAdmin() {
		utils.Forbidden(c, "仅校级管理员可以保存待补全赛事")
		return
	}
	req.CompLevel = strings.TrimSpace(req.CompLevel)
	if (req.CompLevel == "" && comp.CompLevel != "") || (req.ManagerID == 0 && comp.ManagerID != 0) {
		utils.BadRequest(c, "已填写的赛事级别和负责人不能为空")
		return
	}
	managerChanged := req.ManagerID != comp.ManagerID
	if managerChanged && !canReassign {
		utils.Forbidden(c, "仅校级管理员可以变更赛事负责人")
		return
	}

	_, scoped := c.Get("role_code")
	req.CompName = strings.TrimSpace(req.CompName)
	if req.CompName == "" {
		utils.BadRequest(c, "赛事名称不能为空")
		return
	}
	year, validYear := resolveCompetitionYear(req.Year, comp.Year)
	if !validYear {
		utils.BadRequest(c, "所属年份须为1000至9999的4位数字")
		return
	}

	var promotions []RolePromotion
	err := database.TransactionWithDeadlockRetry(database.DB.WithContext(c.Request.Context()), func(tx *gorm.DB) error {
		promotions = nil
		var collegeID *uint
		if !scoped {
			var err error
			collegeID, err = resolveCollegeIDByName(tx, req.College)
			if err != nil {
				return err
			}
		} else if req.ManagerID != 0 && canReassign {
			var unit string
			if managerChanged {
				manager, err := findCompetitionManager(c, tx, req.ManagerID)
				if err != nil {
					return err
				}
				unit = manager.College
			} else if err := tx.Model(&models.User{}).Where("id = ?", req.ManagerID).Pluck("college", &unit).Error; err != nil {
				return err
			}
			var err error
			collegeID, err = resolveUnitCollegeID(tx, unit)
			if err != nil {
				return err
			}
		}
		updates := map[string]interface{}{
			"comp_name":  req.CompName,
			"comp_level": req.CompLevel,
			"comp_type":  req.CompType,
			"organizer":  req.Organizer,
			"undertaker": req.Undertaker,
			"manager_id": nil,
			"college_id": nil,
			"year":       year,
			"desc":       req.Desc,
		}
		if req.ManagerID != 0 {
			updates["manager_id"] = req.ManagerID
		}
		if collegeID != nil {
			updates["college_id"] = *collegeID
		}
		if !canReassign {
			delete(updates, "manager_id")
			delete(updates, "college_id")
		}
		if strings.HasPrefix(comp.CompCode, pendingCompetitionCodePrefix) && req.CompLevel != "" {
			code, codeErr := nextCompetitionCode(tx, req.CompLevel, year)
			if codeErr != nil {
				return codeErr
			}
			updates["comp_code"] = code
		}

		update := tx.Model(&comp)
		if scope.IsCollegeAdmin() {
			update = applyCompetitionScope(update, scope, "comp_directories")
		}
		incomplete := comp.CompCode != "" && (comp.CompLevel == "" || comp.ManagerID == 0)
		if incomplete {
			// A concurrent edit may have completed one of the missing fields.
			update = update.Where("comp_code = ? AND COALESCE(comp_level, '') = ? AND COALESCE(manager_id, 0) = ?",
				comp.CompCode, comp.CompLevel, comp.ManagerID)
		}
		result := update.Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 && (incomplete || scope.IsCollegeAdmin()) {
			return errCompetitionEditConflict
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
		if errors.Is(err, errCompetitionEditConflict) {
			utils.BadRequest(c, err.Error())
			return
		}
		respondCompetitionMutationError(c, "更新失败", err)
		return
	}
	clearCompListCache(c.Request.Context())
	sourceID := comp.ID
	finalizeRolePromotions(c, promotions, auditReasonCompetition, &sourceID)
	utils.SuccessWithMessage(c, "更新成功", gin.H{
		"promoted_users": promotedUsersResp(c, promotions),
	})
}

var errCompetitionEditConflict = errors.New("赛事信息已更新，请刷新页面后再编辑")
