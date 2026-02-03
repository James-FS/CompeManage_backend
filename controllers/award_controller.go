package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// 1. 获取获奖管理的赛事列表
// 逻辑：如果是 Admin -> 返回所有赛事；如果是老师 -> 返回 ManagerID=自己的赛事
func GetAwardCompList(c *gin.Context) {
	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	offset := (page - 1) * pageSize

	var comps []models.CompDirectory
	var total int64

	db := database.DB.Model(&models.CompDirectory{})

	// 复用权限检查逻辑 (需要在同包下)
	if !checkUserIsAdmin(userID) {
		db = db.Where("manager_id = ?", userID)
	}

	db.Count(&total)

	// 只查主要字段，提升性能
	if err := db.Select("id, comp_name,comp_type, comp_level, status, year, organizer").
		Order("id desc").
		Offset(offset).Limit(pageSize).
		Find(&comps).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	c.JSON(200, gin.H{
		"code": 200,
		"data": gin.H{"list": comps, "total": total},
	})
}

// ExportAwardTemplate 导出接口：内存生成 -> 流式返回
func ExportAwardTemplate(c *gin.Context) {
	compID := c.Query("comp_id")

	// 1. 数据查询
	var regs []models.Register
	if err := database.DB.Preload("Leader").Where("comp_id = ? AND status = 1", compID).Find(&regs).Error; err != nil {
		// ❌ 失败时：返回 JSON
		c.JSON(500, gin.H{"code": 500, "msg": "查询数据失败"})
		return
	}

	// 2. 生成 Excel (利用 excelize)
	f := excelize.NewFile()
	sheet := "获奖录入"
	f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")

	headers := []string{"报名ID (勿改)", "团队名称", "负责人", "学号", "获奖等级 (必填)", "具体奖项名 (选填)"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	for i, r := range regs {
		row := i + 2
		leaderName := "未知"
		stuID := ""
		if r.Leader.ID != 0 {
			leaderName = r.Leader.Realname
			stuID = r.Leader.Username
		}
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), r.ID)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), r.TeamName)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), leaderName)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), stuID)
	}

	// 3. ✅ 成功时：返回二进制流 (不要用 c.JSON 包裹)
	fileName := fmt.Sprintf("Award_Template_%s.xlsx", compID)

	// 设置响应头，告诉浏览器这是个文件
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	c.Header("Content-Transfer-Encoding", "binary")
	// 防止缓存
	c.Header("Cache-Control", "no-cache")

	// 将流直接写入 ResponseWriter
	if err := f.Write(c.Writer); err != nil {
		// 如果流写到一半断了，我们也无能为力，因为 Header 已经发出去了
		fmt.Println("导出流写入中断:", err)
	}
}

func ImportAward(c *gin.Context) {
	compIDStr := c.Query("comp_id")
	if compIDStr == "" {
		c.JSON(400, gin.H{"code": 400, "msg": "缺少 comp_id"})
		return
	}
	compID, _ := strconv.Atoi(compIDStr)

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "文件上传失败"})
		return
	}

	f, err := excelize.OpenReader(file)
	if err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "Excel 读取失败"})
		return
	}

	// 获取第一个工作表
	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "内容解析失败"})
		return
	}

	successCount := 0

	tx := database.DB.Begin()

	for i, row := range rows {
		if i == 0 || len(row) < 5 {
			continue
		} // 跳过表头

		regIDStr := row[0]
		awardLevel := row[4] // E列
		awardName := ""
		if len(row) > 5 {
			awardName = row[5]
		} // F列

		if awardLevel == "" || regIDStr == "" {
			continue
		}

		regID, _ := strconv.Atoi(regIDStr)

		// Upsert 逻辑: 有则更新，无则插入
		var award models.Award
		err := tx.Where("reg_id = ?", regID).First(&award).Error

		if err != nil {
			// 不存在 -> 创建
			newAward := models.Award{
				CompID:     uint(compID),
				RegID:      uint(regID),
				AwardLevel: awardLevel,
				AwardName:  awardName,
			}
			if err := tx.Create(&newAward).Error; err == nil {
				successCount++
			}
		} else {
			// 存在 -> 更新
			award.AwardLevel = awardLevel
			award.AwardName = awardName
			if err := tx.Save(&award).Error; err == nil {
				successCount++
			}
		}
	}

	tx.Commit()
	c.JSON(200, gin.H{"code": 200, "msg": fmt.Sprintf("成功处理 %d 条数据", successCount)})
}

// 4. 获取获奖公示详情 (只读列表)
func GetCompAwards(c *gin.Context) {
	compID := c.Query("comp_id")

	var awards []models.Award

	// 关联 Register 和 Register.Leader 获取展示信息
	err := database.DB.
		Preload("Register").
		Preload("Register.Leader").
		Where("comp_id = ?", compID).
		Order("level_rank ASC").
		Find(&awards).Error

	if err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "查询失败"})
		return
	}

	// 组装简单的 DTO 返回给前端
	var list []gin.H
	for _, a := range awards {
		leaderName := "未知"
		if a.Register.Leader.ID != 0 {
			leaderName = a.Register.Leader.Realname
		}

		list = append(list, gin.H{
			"id":          a.ID,
			"team_name":   a.Register.TeamName,
			"leader_name": leaderName,
			"award_level": a.AwardLevel,
			"award_name":  a.AwardName,
		})
	}

	c.JSON(200, gin.H{"code": 200, "data": list})
}

// GetStudentMyAwardList 学生端：获取本人已申报的获奖列表（我的奖项）
func GetStudentMyAwardList(c *gin.Context) {
	// 1. 从登录中间件获取当前学生ID
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(401, gin.H{"code": 401, "msg": "请先登录"})
		return
	}
	studentID, ok := userIDVal.(uint)
	if !ok {
		c.JSON(400, gin.H{"code": 400, "msg": "用户ID格式错误"})
		return
	}

	// 2. 获取前端查询参数
	compIDStr := c.Query("comp_id")                       // 可选：按赛事ID筛选
	status := c.Query("status")                           // 可选：按申报状态筛选（draft/approved/rejected）
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))  // 默认第1页
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10")) // 默认每页10条

	// 3. 分页参数校验（复用现有逻辑，限制最大每页50条）
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 10
	}
	offset := (page - 1) * size

	// 4. 构建数据库查询（核心：关联Award→Register→CompDirectory，仅查当前学生数据）
	// 复用现有关联查询逻辑
	dbQuery := database.DB.Model(&models.Award{}).
		Preload("Register").                                     // 关联报名记录（获取团队名称）
		Preload("Register.Leader").                              // 关联负责人信息（获取学生姓名/学号）
		Preload("Register.CompDirectory").                       // 关联赛事信息（获取赛事名称/年份）
		Joins("JOIN registers ON awards.reg_id = registers.id"). // 关联报名表，通过leader_id筛选学生
		Where("registers.leader_id = ?", studentID)              // 核心筛选：仅当前学生的申报

	// 5. 可选筛选：赛事ID
	if compIDStr != "" {
		compID, err := strconv.ParseUint(compIDStr, 10, 32)
		if err != nil {
			c.JSON(400, gin.H{"code": 400, "msg": "赛事ID格式错误，必须是数字"})
			return
		}
		dbQuery = dbQuery.Where("awards.comp_id = ?", compID)
	}

	// 6. 可选筛选：申报状态
	if status != "" {
		validStatus := map[string]bool{"draft": true, "approved": true, "rejected": true}
		if !validStatus[status] {
			c.JSON(400, gin.H{"code": 400, "msg": "状态参数错误，仅支持draft/approved/rejected"})
			return
		}
		dbQuery = dbQuery.Where("awards.status = ?", status)
	}

	// 7. 排序：按申报时间倒序
	dbQuery = dbQuery.Order("awards.created_at DESC")

	// 8. 查询总数+分页列表（复用现有错误处理风格）
	var total int64
	var myAwardList []models.Award

	// 统计总数（用于前端分页）
	if err := dbQuery.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "统计我的获奖申报总数失败"})
		return
	}

	// 分页查询数据
	if err := dbQuery.Offset(offset).Limit(size).Find(&myAwardList).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "查询我的获奖申报列表失败"})
		return
	}

	// 9. 响应格式
	c.JSON(200, gin.H{
		"code": 200,
		"data": gin.H{
			"list":  myAwardList, // 获奖列表（含所有关联信息，前端可按需取用）
			"total": total,       // 总条数
			"page":  page,        // 当前页码
			"size":  size,        // 每页条数
		},
	})
}
