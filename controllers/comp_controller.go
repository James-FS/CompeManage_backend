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
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func clearCompListCache() {
	rdb := middleware.GetRedisClient()
	ctx := context.Background()
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
	err := tx.Model(&models.CompDirectory{}).
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

func hasCompetitionStarted(compID uint, now time.Time) (bool, error) {
	var detail models.CompDetail
	err := database.DB.Select("comp_id", "comp_start_time").Where("comp_id = ?", compID).First(&detail).Error
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
	query := database.DB.Model(&models.CompDirectory{})

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
		userID, exists := c.Get("user_id")
		if !exists {
			utils.Unauthorized(c, "未登录")
			return
		}
		uid := userID.(uint)
		if !checkUserIsAdmin(uid) {
			query = query.Where("manager_id = ?", userID)
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
		data, _ := json.Marshal(responseData)
		rdb.Set(context.Background(), cacheKey, data, 5*time.Minute).Result()
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
	err := database.DB.Transaction(func(tx *gorm.DB) error {
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

		created = compDir
		return nil
	})

	if err != nil {
		utils.InternalServerError(c, "创建失败", err)
		return
	}
	clearCompListCache()
	utils.SuccessWithMessage(c, "创建成功", created)
}

func BatchImportCompetition(c *gin.Context) {
	var req struct {
		Items []CreateCompetitionReq `json:"items" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	var compDirs []models.CompDirectory
	err := database.DB.Transaction(func(tx *gorm.DB) error {
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
		}
		return nil
	})

	if err != nil {
		utils.InternalServerError(c, "导入失败", err)
		return
	}

	utils.SuccessWithMessage(c, "批量导入成功", gin.H{"count": len(compDirs)})
}

type ManagerListReq struct {
	Name     string `form:"name"`
	WorkID   string `form:"work_id"`
	College  string `form:"college"`
	Page     int    `form:"page" binding:"required,min=1"`
	PageSize int    `form:"page_size" binding:"required,min=1"`
}

type ManagerResp struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	WorkID  string `json:"work_id"`
	College string `json:"college"`
}

func GetManagerList(c *gin.Context) {
	var req ManagerListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	var role models.Role
	if err := database.DB.Where("role_code = ?", "competition_manager").First(&role).Error; err != nil {
		utils.InternalServerError(c, "获取角色信息失败", err)
		return
	}

	query := database.DB.Model(&models.User{}).
		Joins("JOIN user_roles ON users.id = user_roles.user_id").
		Where("user_roles.role_id = ?", role.ID)

	if req.Name != "" {
		query = query.Where("users.realname LIKE ?", "%"+req.Name+"%")
	}

	if req.WorkID != "" {
		query = query.Where("users.username LIKE ?", "%"+req.WorkID+"%")
	}

	if req.College != "" {
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

	var managers []ManagerResp
	for _, user := range users {
		managers = append(managers, ManagerResp{
			ID:      user.ID,
			Name:    user.Realname,
			WorkID:  user.Username,
			College: user.College,
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
	if err := database.DB.
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
	if err := database.DB.First(&comp, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "赛事不存在")
		} else {
			utils.InternalServerError(c, "查询失败", err)
		}
		return
	}

	started, err := hasCompetitionStarted(comp.ID, time.Now())
	if err != nil {
		utils.InternalServerError(c, "校验赛事时间失败", err)
		return
	}
	if started {
		utils.BadRequest(c, "赛事已开始，不能删除")
		return
	}

	if err := database.DB.Delete(&comp).Error; err != nil {
		utils.InternalServerError(c, "删除失败", err)
		return
	}
	clearCompListCache()
	utils.SuccessWithMessage(c, "删除成功", nil)
}

func RestoreCompetition(c *gin.Context) {
	id := c.Param("id")

	var comp models.CompDirectory
	if err := database.DB.Unscoped().First(&comp, id).Error; err != nil {
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

	if err := database.DB.Model(&comp).Update("delete_time", nil).Error; err != nil {
		utils.InternalServerError(c, "恢复失败", err)
		return
	}
	clearCompListCache()
	utils.SuccessWithMessage(c, "恢复成功", nil)
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
	if err := database.DB.Select("id", "comp_name").Where("id IN ?", req.IDs).Find(&comps).Error; err != nil {
		utils.InternalServerError(c, "查询赛事失败", err)
		return
	}

	if len(comps) > 0 {
		var details []models.CompDetail
		if err := database.DB.Select("comp_id", "comp_start_time").Where("comp_id IN ?", req.IDs).Find(&details).Error; err != nil {
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

	result := database.DB.Delete(&models.CompDirectory{}, req.IDs)
	if result.Error != nil {
		utils.InternalServerError(c, "删除失败", result.Error)
		return
	}
	clearCompListCache()
	utils.Success(c, gin.H{"deleted_count": result.RowsAffected})
}

func GetCompetitionDetail(c *gin.Context) {
	id := c.Param("id")
	var comp models.CompDirectory

	if err := database.DB.Preload("Manager").Preload("CollegeInfo").First(&comp, id).Error; err != nil {
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
	if err := database.DB.First(&comp, id).Error; err != nil {
		utils.NotFound(c, "赛事不存在")
		return
	}

	collegeID, collegeErr := resolveCollegeIDByName(database.DB, req.College)
	if collegeErr != nil {
		utils.BadRequest(c, collegeErr.Error())
		return
	}

	year := 0
	if req.Year != "" {
		fmt.Sscanf(req.Year, "%d", &year)
	}

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

	if err := database.DB.Model(&comp).Updates(updates).Error; err != nil {
		utils.InternalServerError(c, "更新失败", err)
		return
	}
	clearCompListCache()
	utils.SuccessWithMessage(c, "更新成功", nil)
}
