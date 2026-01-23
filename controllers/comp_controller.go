package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"net/http"

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
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	// 按级别筛选 (校级/省级/国家级)
	if req.CompLevel != "" {
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
		if exists {
			query = query.Where("manager_id = ?", userID)
			query = query.Preload("Detail", func(db *gorm.DB) *gorm.DB {
				return db.Select("comp_id", "reg_start_time", "reg_end_time", "participant_type")
			})
		}
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
