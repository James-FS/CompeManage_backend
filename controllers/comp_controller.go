package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CompListReq 定义列表查询参数结构体
type CompListReq struct {
	Page      int    `form:"page" binding:"required,min=1"`      // 页码
	PageSize  int    `form:"page_size" binding:"required,min=1"` // 每页数量
	CompName  string `form:"comp_name"`                          // 模糊搜索：竞赛名称
	Manager   string `form:"manager"`                            // 模糊搜索：负责人名称
	Status    string `form:"status"`                             // 筛选：状态 (未开始/进行中/已结束)
	CompLevel string `form:"comp_level"`                         // 筛选：级别 (校级/省级/国家级)
	College   string `form:"college"`                            // 筛选：所属学院名称
	Year      string `form:"year"`                               // 筛选：举办年份
	IsMy      bool   `form:"is_my"`                              // 筛选：仅看我发布的 (用于管理员/老师后台)
	IsReg     bool   `form:"is_reg"`
}

// GetCompetitionList 获取竞赛目录列表
func GetCompetitionList(c *gin.Context) {
	var req CompListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	query := database.DB.Model(&models.CompDirectory{})

	// 按名称模糊搜索
	if req.CompName != "" {
		query = query.Where("comp_name LIKE ?", "%"+req.CompName+"%")
	}

	// 按负责人模糊搜索
	if req.Manager != "" {
		query = query.Where("manager LIKE ?", "%"+req.Manager+"%")
	}

	// 按状态筛选 (未开始/进行中/已结束)
	if req.Status != "" && req.Status != "all" {
		now := time.Now() // 获取服务器当前精准时间
		// 注意：GORM 的 Joins 默认是 Inner Join，这里为了防止没详情的报错，可以用 Left Join
		// 但通常发布的赛事都有详情，所以直接用 Joins 也没问题
		query = query.Joins("LEFT JOIN comp_details ON comp_details.comp_id = comp_directories.id")

		switch req.Status {
		case "upcoming": // 未开始 (当前时间 < 报名开始时间)
			query = query.Where("comp_details.reg_start_time > ?", now)

		case "ongoing": // 进行中 (报名开始 <= 当前 <= 报名结束)
			query = query.Where("comp_details.reg_start_time <= ? AND comp_details.reg_end_time >= ?", now, now)

		case "ended": // 已结束 (当前时间 > 报名结束)
			query = query.Where("comp_details.reg_end_time < ?", now)

		default:

		}
	}

	// 按级别筛选 (校级/省级/国家级)
	if req.CompLevel != "" && req.CompLevel != "全部" {
		query = query.Where("comp_level = ?", req.CompLevel)
	}

	// 按学院名称筛选（通过关联 College 表）
	if req.College != "" {
		query = query.Joins("LEFT JOIN colleges ON comp_directories.college_id = colleges.id").Where("colleges.name = ?", req.College)
	}

	// 按年份筛选
	if req.Year != "" {
		query = query.Where("YEAR(create_time) = ?", req.Year)
	}

	// 仅查看"我负责的" (从 Token 获取当前用户ID)
	if req.IsMy {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未登录"})
			return
		}
		uid := userID.(uint)
		if !checkUserIsAdmin(uid) {
			// 不是管理员：只返回当前用户负责的赛事
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
	}

	// 计算总数 (用于分页)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	// 执行分页查询
	var list []models.CompDirectory
	offset := (req.Page - 1) * req.PageSize

	// 按创建时间倒序排列 (最新的在前面)
	// 同时也查出关联的 Detail 信息、Manager 信息、College 信息
	if err := query.Preload("Detail").Preload("Manager").Preload("CollegeInfo").Order("create_time desc").Offset(offset).Limit(req.PageSize).Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询数据失败"})
		return
	}

	// 5. 返回结果
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "获取成功",
		"data": gin.H{
			"list":  list,
			"total": total,
			"page":  req.Page,
			"size":  req.PageSize,
		},
	})
}

// CreateCompetitionReq 创建竞赛目录
type CreateCompetitionReq struct {
	CompName   string `json:"comp_name" binding:"required"`  // 竞赛名称
	CompLevel  string `json:"comp_level" binding:"required"` // 竞赛级别
	CompType   string `json:"comp_type"`                     // 竞赛类别
	Organizer  string `json:"organizer"`                     // 主办方
	Undertaker string `json:"undertaker"`                    // 承办方
	ManagerID  uint   `json:"manager_id" binding:"required"` // 赛事负责人ID
	College    string `json:"college" binding:"required"`    // 所属学院
	Desc       string `json:"desc"`                          // 描述说明
	Year       string `json:"year"`                          // 举办年份
}

// CreateCompetition 新增赛事目录
func CreateCompetition(c *gin.Context) {
	// 绑定并校验参数
	var req CreateCompetitionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	// 获取学院ID (根据学院名称查询)
	var college models.College
	if err := database.DB.Where("name = ?", req.College).First(&college).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "学院不存在"})
		return
	}

	// 将年份字符串转换为整数
	year := 0
	if req.Year != "" {
		fmt.Sscanf(req.Year, "%d", &year)
	}

	// 构造数据库模型
	compDir := models.CompDirectory{
		CompName:   req.CompName,
		CompLevel:  req.CompLevel,
		CompType:   req.CompType,
		Organizer:  req.Organizer,
		Undertaker: req.Undertaker,
		ManagerID:  req.ManagerID,
		CollegeID:  college.ID,
		Year:       year,
		Desc:       req.Desc,
		Status:     0, // 0: 草稿状态
	}

	if err := database.DB.Create(&compDir).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "创建成功",
		"data": compDir,
	})
}

// BatchImportCompetition 批量导入竞赛目录
func BatchImportCompetition(c *gin.Context) {
	var req struct {
		Items []CreateCompetitionReq `json:"items" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	// 构造批量插入的数据
	var compDirs []models.CompDirectory
	for _, item := range req.Items {
		// 获取学院ID
		var college models.College
		if err := database.DB.Where("name = ?", item.College).First(&college).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "学院不存在: " + item.College})
			return
		}

		// 将年份字符串转换为整数
		year := 0
		if item.Year != "" {
			fmt.Sscanf(item.Year, "%d", &year)
		}

		compDirs = append(compDirs, models.CompDirectory{
			CompName:   item.CompName,
			CompLevel:  item.CompLevel,
			CompType:   item.CompType,
			Organizer:  item.Organizer,
			Undertaker: item.Undertaker,
			ManagerID:  item.ManagerID,
			CollegeID:  college.ID,
			Year:       year,
			Desc:       item.Desc,
			Status:     0, // 0: 草稿状态
		})
	}

	if err := database.DB.CreateInBatches(compDirs, 100).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "导入失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "批量导入成功",
		"data": gin.H{"count": len(compDirs)},
	})
}

// ManagerListReq 获取赛事负责人列表的查询参数
type ManagerListReq struct {
	Name     string `form:"name"`                               // 模糊搜索：姓名
	WorkID   string `form:"work_id"`                            // 模糊搜索：工号
	College  string `form:"college"`                            // 精确搜索：所属学院
	Page     int    `form:"page" binding:"required,min=1"`      // 页码
	PageSize int    `form:"page_size" binding:"required,min=1"` // 每页数量
}

// ManagerResp 赛事负责人响应结构
type ManagerResp struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`    // 真实姓名
	WorkID  string `json:"work_id"` // 工号（username）
	College string `json:"college"`
}

// GetManagerList 获取赛事负责人列表
// 从 role 表查询赛事负责人角色 -> 从 user_roles 联接表找到相关用户 -> 返回用户信息
func GetManagerList(c *gin.Context) {
	var req ManagerListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	// 1. 查询赛事负责人角色（假设角色标识为 "competition_manager"）
	var role models.Role
	if err := database.DB.Where("role_code = ?", "competition_manager").First(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取角色信息失败"})
		return
	}

	// 2. 构建用户查询条件
	query := database.DB.Model(&models.User{}).
		Joins("JOIN user_roles ON users.id = user_roles.user_id").
		Where("user_roles.role_id = ?", role.ID)

	// 名称模糊搜索
	if req.Name != "" {
		query = query.Where("users.realname LIKE ?", "%"+req.Name+"%")
	}

	// 工号模糊搜索
	if req.WorkID != "" {
		query = query.Where("users.username LIKE ?", "%"+req.WorkID+"%")
	}

	// 学院精确搜索
	if req.College != "" {
		query = query.Where("users.college = ?", req.College)
	}

	// 3. 计算总数（用于分页）
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	// 4. 执行分页查询
	offset := (req.Page - 1) * req.PageSize
	var users []models.User
	if err := query.
		Select("users.id", "users.realname", "users.username", "users.college").
		Order("users.id").
		Offset(offset).
		Limit(req.PageSize).
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询用户数据失败"})
		return
	}

	// 5. 转换为响应结构
	var managers []ManagerResp
	for _, user := range users {
		managers = append(managers, ManagerResp{
			ID:      user.ID,
			Name:    user.Realname,
			WorkID:  user.Username,
			College: user.College,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "获取成功",
		"data": gin.H{
			"list":  managers,
			"total": total,
			"page":  req.Page,
			"size":  req.PageSize,
		},
	})
}

// GetCompetitionYears 获取赛事中存在的所有年份（用于往年复用的年份选择）
// SQL: SELECT DISTINCT year FROM comp_directories WHERE year > 0 ORDER BY year DESC
func GetCompetitionYears(c *gin.Context) {
	var years []int
	if err := database.DB.
		Model(&models.CompDirectory{}).
		Where("year > ?", 0).
		Distinct("year").
		Order("year DESC").
		Pluck("year", &years).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询年份失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "获取成功",
		"data": gin.H{
			"years": years,
		},
	})
}

// DeleteCompetition 删除赛事（软删除）
func DeleteCompetition(c *gin.Context) {
	id := c.Param("id")

	// 检查赛事是否存在
	var comp models.CompDirectory
	if err := database.DB.First(&comp, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "赛事不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		}
		return
	}

	// 执行软删除
	if err := database.DB.Delete(&comp).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "删除失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "删除成功",
	})
}

// RestoreCompetition 恢复已删除的赛事
func RestoreCompetition(c *gin.Context) {
	id := c.Param("id")

	// 检查赛事是否存在（包括已删除的）
	var comp models.CompDirectory
	if err := database.DB.Unscoped().First(&comp, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "赛事不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询失败"})
		}
		return
	}

	// 检查是否已经被删除
	if comp.DeletedAt.Time.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "赛事未被删除"})
		return
	}

	// 执行恢复操作
	if err := database.DB.Model(&comp).Update("delete_time", nil).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "恢复失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "恢复成功",
	})
}

// BatchDeleteReq 批量删除请求参数
type BatchDeleteReq struct {
	IDs []uint `json:"ids" binding:"required,min=1"` // 要删除的赛事ID列表
}

// BatchDeleteCompetition 批量删除赛事（软删除）
func BatchDeleteCompetition(c *gin.Context) {
	var req BatchDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误", "error": err.Error()})
		return
	}

	// 执行批量软删除
	result := database.DB.Delete(&models.CompDirectory{}, req.IDs)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "删除失败", "error": result.Error.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "删除成功",
		"data": gin.H{
			"deleted_count": result.RowsAffected,
		},
	})
}
