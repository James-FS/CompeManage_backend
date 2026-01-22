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
	Name      string `form:"name"`                               // 模糊搜索：竞赛名称
	Status    *int8  `form:"status"`                             // 筛选：状态 (使用指针是为了区分前端是否传了0)
	CompLevel string `form:"level"`                              // 筛选：级别 (国家级/省级)
	CompType  string `form:"type"`                               // 筛选：类别 (A类/B类)
	CollegeID uint   `form:"college_id"`                         // 筛选：所属学院
	IsMy      bool   `form:"is_my"`                              // 筛选：仅看我发布的 (用于管理员/老师后台)
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
	if req.Name != "" {
		query = query.Where("comp_name LIKE ?", "%"+req.Name+"%")
	}

	// 按状态筛选 (0:草稿 1:发布 2:结束)
	// 注意：这里使用指针判断，否则无法筛选 status=0 的草稿
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}

	// 按级别筛选
	if req.CompLevel != "" {
		query = query.Where("comp_level = ?", req.CompLevel)
	}

	// 按类别筛选
	if req.CompType != "" {
		query = query.Where("comp_type = ?", req.CompType)
	}

	// 按学院筛选
	if req.CollegeID > 0 {
		query = query.Where("college_id = ?", req.CollegeID)
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
	// 同时也查出关联的 Detail 信息
	if err := query.Order("create_time desc").Offset(offset).Limit(req.PageSize).Find(&list).Error; err != nil {
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
