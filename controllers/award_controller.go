package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/logger"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// DateOnly 解析日期，支持 "2006-01-02" 和 RFC3339 两种格式
type DateOnly struct {
	time.Time
}

func (d *DateOnly) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	if str == "" || str == "null" {
		return nil
	}
	// 先试纯日期格式
	parsed, err := time.Parse("2006-01-02", str)
	if err == nil {
		d.Time = parsed
		return nil
	}
	// 再试 RFC3339
	parsed, err = time.Parse(time.RFC3339, str)
	if err == nil {
		d.Time = parsed
		return nil
	}
	return err
}

type AwardAuditListItem struct {
	ID          uint   `json:"id"`
	StudentName string `json:"student_name"`
	StudentID   string `json:"student_id"`
	CompName    string `json:"comp_name"`
	AwardLevel  string `json:"award_level"`
	AwardDate   string `json:"award_date"`
	Phone       string `json:"phone"`
	SubmitTime  string `json:"submit_time"`
	Status      int    `json:"status"`
}

type AwardAuditDetailResp struct {
	ID            uint    `json:"id"`
	StudentName   string  `json:"student_name"`
	StudentID     string  `json:"student_id"`
	College       string  `json:"college"`
	Phone         string  `json:"phone"`
	Email         string  `json:"email"`
	CompName      string  `json:"comp_name"`
	AwardLevel    string  `json:"award_level"`
	AwardSpecific string  `json:"award_specific"`
	AwardDate     string  `json:"award_date"`
	TeamName      string  `json:"team_name"`
	Teammates     []gin.H `json:"teammates"`
	CertImage     string  `json:"cert_image"`
	SubmitTime    string  `json:"submit_time"`
	Status        int     `json:"status"`
	RejectReason  string  `json:"reject_reason"`
}

type CompSearchResp struct {
	ID       uint   `json:"id"`
	CompName string `json:"comp_name"`
	Year     int    `json:"year"`
}

func mapAwardStatusToInt(status string) int {
	switch status {
	case "approved":
		return 1
	case "rejected":
		return 2
	default:
		return 0
	}
}

func mapAwardStatusFromInt(status int) string {
	switch status {
	case 1:
		return "approved"
	case 2:
		return "rejected"
	default:
		return "draft"
	}
}

func clearAwardCache(ctx context.Context, compID interface{}) {
	rdb := middleware.GetRedisClient()
	cacheKey := fmt.Sprintf("cache:awards:%v", compID)
	rdb.Del(ctx, cacheKey)
}
func getLeaderMember(members []models.RegMember, fallback models.User) (string, string, string, string) {
	for _, m := range members {
		if m.IsLeader {
			return m.Name, m.StudentID, m.Phone, m.Email
		}
	}
	return fallback.Realname, fallback.Username, "", ""
}

func GetAwardCompList(c *gin.Context) {
	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	offset := (page - 1) * pageSize

	var comps []models.CompDirectory
	var total int64

	db := database.DB.WithContext(c.Request.Context()).Model(&models.CompDirectory{})

	if !checkUserIsAdmin(c.Request.Context(), userID) {
		db = db.Where("manager_id = ?", userID)
	}

	db.Count(&total)

	if err := db.Preload("Detail").
		Order("id desc").
		Offset(offset).Limit(pageSize).
		Find(&comps).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	utils.Success(c, gin.H{"list": comps, "total": total})
}

func ExportAwardTemplate(c *gin.Context) {
	// 1. 创建 Excel 实例
	f := excelize.NewFile()
	sheet := "Sheet1"
	f.SetSheetName("Sheet1", sheet)

	// 2. 定义美化样式 (蓝色背景、白色粗体)
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4F81BD"}, Pattern: 1},
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 12, Family: "微软雅黑"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
		},
	})

	// 3. 设置表头字段顺序
	headers := []string{"序号", "获奖项目", "奖项等级", "负责人", "学号", "团队成员", "所属学院", "指导老师"}
	for i, header := range headers {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		f.SetCellValue(sheet, colName+"1", header)
		f.SetCellStyle(sheet, colName+"1", colName+"1", headerStyle)

		// 设置默认列宽
		f.SetColWidth(sheet, colName, colName, 18)
	}

	// 微调特定列宽
	f.SetColWidth(sheet, "B", "B", 30) // 获奖项目
	f.SetColWidth(sheet, "F", "F", 40) // 团队成员
	f.SetRowHeight(sheet, 1, 25)       // 表头行高

	// 4. 添加一行示例数据 (可选，方便用户参考格式)
	f.SetCellValue(sheet, "A2", "1")
	f.SetCellValue(sheet, "B2", "示例：第十届数学建模大赛")
	f.SetCellValue(sheet, "C2", "一等奖")
	f.SetCellValue(sheet, "D2", "张三")
	f.SetCellValue(sheet, "E2", "202100123")
	f.SetCellValue(sheet, "F2", "张三、李四、王五")
	f.SetCellValue(sheet, "G2", "计算机学院")
	f.SetCellValue(sheet, "H2", "王老师")

	// 5. 设置 HTTP 响应头并下载
	fileName := "获奖名单导入模板.xlsx"
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Header("Content-Transfer-Encoding", "binary")

	if err := f.Write(c.Writer); err != nil {
		utils.InternalServerError(c, "生成模板失败", err)
	}
}

func ImportAward(c *gin.Context) {
	// 1. 参数校验
	compIDStr := c.Query("comp_id")
	if compIDStr == "" {
		utils.BadRequest(c, "缺少 comp_id")
		return
	}
	compID, _ := strconv.Atoi(compIDStr)

	// 2. 文件读取
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		utils.BadRequest(c, "文件上传失败")
		return
	}
	defer file.Close()

	f, err := excelize.OpenReader(file)
	if err != nil {
		utils.BadRequest(c, "Excel 读取失败")
		return
	}

	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		utils.BadRequest(c, "内容解析失败")
		return
	}

	// 3. 获取赛事信息及奖项配置
	var comp models.CompDirectory
	if err := database.DB.WithContext(c.Request.Context()).Preload("Detail").First(&comp, compID).Error; err != nil {
		utils.BadRequest(c, "找不到对应赛事")
		return
	}

	var awardHierarchy []string
	if err := json.Unmarshal([]byte(comp.Detail.AwardHierarchy), &awardHierarchy); err != nil || len(awardHierarchy) == 0 {
		utils.BadRequest(c, "奖项等级配置解析失败")
		return
	}

	successCount := 0
	failCount := 0
	var failReasons []string

	tx := database.DB.WithContext(c.Request.Context()).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 4. 遍历 Excel 行（跳过表头）
	for i, row := range rows {
		if i == 0 || len(row) < 4 { // 至少需要 4 列：等级、名称、负责人、学号
			continue
		}

		levelRankStr := strings.TrimSpace(row[0])  // 对应 Award.LevelRank
		teamNameExcel := strings.TrimSpace(row[1]) // 对应 Register.TeamName
		studentID := strings.TrimSpace(row[3])     // 用于锁定负责人

		if levelRankStr == "" || studentID == "" {
			failCount++
			failReasons = append(failReasons, fmt.Sprintf("第%d行：奖项等级或学号不能为空", i+1))
			continue
		}

		levelRank, _ := strconv.Atoi(levelRankStr)
		if levelRank < 1 || levelRank > len(awardHierarchy) {
			failCount++
			failReasons = append(failReasons, fmt.Sprintf("第%d行：奖项等级数字无效", i+1))
			continue
		}

		// A. 通过学号找到 UserID
		var leader models.User
		if err := tx.Where("username = ?", studentID).First(&leader).Error; err != nil {
			failCount++
			failReasons = append(failReasons, fmt.Sprintf("第%d行：学号[%s]用户不存在", i+1, studentID))
			continue
		}

		// B. 锁定唯一的报名记录
		var reg models.Register
		// 根据你的描述：通过学号和 compID 锁定
		if err := tx.Where("comp_id = ? AND leader_id = ? AND status IN (1, 4)", uint(compID), leader.ID).
			Preload("Members").
			First(&reg).Error; err != nil {
			failCount++
			failReasons = append(failReasons, fmt.Sprintf("第%d行：未找到学号[%s]在该赛事的审核通过记录", i+1, studentID))
			continue
		}

		// C. 团队名称填充逻辑 (根据你的业务逻辑)
		// 如果 Excel 里的名称为空，且报名成员数 > 1，则填充为“个人参赛”
		if teamNameExcel == "" {
			if len(reg.Members) > 1 {
				teamNameExcel = "个人参赛"
			} else {
				// 如果是 1 个人且 Excel 为空，可保持原报名表名称或逻辑补充
				teamNameExcel = reg.TeamName
			}
		}

		// 更新报名表的 TeamName（同步你要求的逻辑到数据库）
		if teamNameExcel != "" && reg.TeamName != teamNameExcel {
			tx.Model(&reg).Update("team_name", teamNameExcel)
		}

		// D. 准备 Award 数据
		awardName := awardHierarchy[levelRank-1]
		awardLevelStr := comp.CompLevel + awardName
		now := time.Now()

		var award models.Award
		// 根据 reg_id 查找是否已有奖项记录（保持唯一性）
		err = tx.Where("reg_id = ?", reg.ID).First(&award).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 创建新奖项
			newAward := models.Award{
				CompID:     uint(compID),
				RegID:      reg.ID,
				LevelRank:  levelRank,
				AwardLevel: awardLevelStr,
				AwardName:  awardName,
				Status:     "approved",
				Source:     "import",
				AuditTime:  &now,
			}
			if err := tx.Create(&newAward).Error; err != nil {
				failCount++
				failReasons = append(failReasons, fmt.Sprintf("第%d行：创建获奖记录失败", i+1))
				continue
			}
		} else {
			// 更新现有奖项
			updates := map[string]interface{}{
				"level_rank":  levelRank,
				"award_level": awardLevelStr,
				"award_name":  awardName,
				"status":      "approved",
				"audit_time":  &now,
				"source":      "import",
			}
			if err := tx.Model(&award).Updates(updates).Error; err != nil {
				failCount++
				failReasons = append(failReasons, fmt.Sprintf("第%d行：更新获奖记录失败", i+1))
				continue
			}
		}

		// E. 更新报名状态（可选：4 代表补录或已获记录状态）
		tx.Model(&reg).Update("status", 4)
		successCount++
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}

	clearAwardCache(c.Request.Context(), compID)
	utils.SuccessWithMessage(c, fmt.Sprintf("成功处理 %d 条，失败 %d 条", successCount, failCount), gin.H{
		"success_count": successCount,
		"fail_count":    failCount,
		"fail_reasons":  failReasons,
	})
}
func SearchCompetition(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" {
		utils.BadRequest(c, "关键词不能为空")
		return
	}

	pageSize := c.DefaultQuery("page_size", "20")
	size := 20
	if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 && ps <= 100 {
		size = ps
	}

	var list []models.CompDirectory
	if err := database.DB.WithContext(c.Request.Context()).Model(&models.CompDirectory{}).
		Where("comp_name LIKE ?", "%"+keyword+"%").
		Select("id", "comp_name", "year").
		Order("create_time DESC").
		Limit(size).
		Find(&list).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	var results []CompSearchResp
	for _, item := range list {
		results = append(results, CompSearchResp{
			ID:       item.ID,
			CompName: item.CompName,
			Year:     item.Year,
		})
	}

	utils.Success(c, results)
}

func GetCompAwards(c *gin.Context) {
	compID := c.Query("comp_id")
	if compID == "" {
		utils.BadRequest(c, "缺少赛事ID")
		return
	}

	cacheKey := fmt.Sprintf("cache:awards:%s", compID)
	rdb := middleware.GetRedisClient()
	ctx := c.Request.Context()

	catchData, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		var resp gin.H
		if json.Unmarshal([]byte(catchData), &resp) == nil {
			logger.Info("获奖列表缓存命中", "cache_key", cacheKey)
			utils.Success(c, resp)
			return
		}
	}

	var comp models.CompDirectory
	if err := database.DB.WithContext(c.Request.Context()).Preload("Detail").First(&comp, compID).Error; err != nil {
		utils.InternalServerError(c, "赛事不存在", err)
		return
	}

	var awardHierarchy []string
	if err := json.Unmarshal([]byte(comp.Detail.AwardHierarchy), &awardHierarchy); err != nil {
		awardHierarchy = []string{}
	}

	var awards []models.Award

	err = database.DB.WithContext(c.Request.Context()).
		Preload("Register").
		Preload("Register.Leader").
		Preload("Register.Competition").
		Preload("Register.Members").
		Where("comp_id = ?", compID).
		Order("level_rank ASC").
		Find(&awards).Error

	if err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	var list []gin.H
	for _, a := range awards {
		leaderName := "未知"
		leaderCollege := ""

		if a.Register.Leader.ID != 0 {
			leaderName = a.Register.Leader.Realname
			leaderCollege = a.Register.Leader.College
		}

		if leaderCollege == "" {
			for _, m := range a.Register.Members {
				if m.IsLeader {
					leaderCollege = m.College
					break
				}
			}
		}

		advisorName := ""
		if a.Register.AdvisorInfo != "" {
			var advisorInfo models.AdvisorInfo
			if err := json.Unmarshal([]byte(a.Register.AdvisorInfo), &advisorInfo); err == nil {
				advisorName = advisorInfo.Name
			}
		}

		var members []gin.H
		for _, m := range a.Register.Members {
			if !m.IsLeader {
				members = append(members, gin.H{
					"name":       m.Name,
					"student_id": m.StudentID,
					"phone":      m.Phone,
					"email":      m.Email,
					"college":    m.College,
				})
			}
		}

		list = append(list, gin.H{
			"id":             a.ID,
			"team_name":      a.Register.TeamName,
			"leader_name":    leaderName,
			"leader_college": leaderCollege,
			"award_level":    a.AwardLevel,
			"award_name":     a.AwardName,
			"comp_level":     a.Register.Competition.CompLevel,
			"advisor_name":   advisorName,
			"members":        members,
		})
	}
	go func() {
		cacheData := gin.H{
			"award_hierarchy": awardHierarchy,
			"list":            list,
		}
		asyncCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		data, err := json.Marshal(cacheData)
		if err != nil {
			logger.Error("异步缓存JSON序列化失败", "cache_key", cacheKey, "error", err)
			return
		}
		if err := rdb.Set(asyncCtx, cacheKey, data, 30*time.Minute).Err(); err != nil {
			logger.Error("异步缓存写入失败", "cache_key", cacheKey, "error", err)
		}
	}()
	utils.Success(c, gin.H{
		"award_hierarchy": awardHierarchy,
		"list":            list,
	})
}

func GetStudentMyAwardList(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "请先登录")
		return
	}
	studentID, ok := userIDVal.(uint)
	if !ok {
		utils.BadRequest(c, "用户ID格式错误")
		return
	}

	compIDStr := c.Query("comp_id")
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 10
	}
	offset := (page - 1) * size

	dbQuery := database.DB.WithContext(c.Request.Context()).Model(&models.Award{}).
		Preload("Register").
		Preload("Register.Leader").
		Preload("Register.Competition").
		Preload("Register.Members").
		Joins("JOIN registers ON awards.reg_id = registers.id").
		Where("registers.leader_id = ?", studentID)

	if compIDStr != "" {
		compID, err := strconv.ParseUint(compIDStr, 10, 32)
		if err != nil {
			utils.BadRequest(c, "赛事ID格式错误，必须是数字")
			return
		}
		dbQuery = dbQuery.Where("awards.comp_id = ?", compID)
	}

	if status != "" {
		validStatus := map[string]bool{"draft": true, "approved": true, "rejected": true}
		if !validStatus[status] {
			utils.BadRequest(c, "状态参数错误，仅支持draft/approved/rejected")
			return
		}
		dbQuery = dbQuery.Where("awards.status = ?", status)
	}

	dbQuery = dbQuery.Order("awards.create_time DESC")

	var total int64
	var myAwardList []models.Award

	if err := dbQuery.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "统计我的获奖申报总数失败", err)
		return
	}

	if err := dbQuery.Offset(offset).Limit(size).Find(&myAwardList).Error; err != nil {
		utils.InternalServerError(c, "查询我的获奖申报列表失败", err)
		return
	}

	utils.Success(c, gin.H{
		"list":  myAwardList,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

func SubmitStudentAwardSupplement(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "请先登录")
		return
	}
	leaderID, ok := userIDVal.(uint)
	if !ok {
		utils.BadRequest(c, "用户ID格式错误")
		return
	}

	var req struct {
		CompID     uint                   `json:"comp_id" binding:"required"`
		TeamName   string                 `json:"team_name" binding:"required"`
		Members    []models.RegMemberInfo `json:"members" binding:"required,dive"`
		AwardLevel string                 `json:"award_level" binding:"required"`
		AwardName  string                 `json:"award_name" binding:"required"`
		AwardDate  DateOnly               `json:"award_date"`
		ProofURL   string                 `json:"proof_url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	leaderCount := 0
	for _, member := range req.Members {
		if member.IsLeader {
			leaderCount++
		}
		if member.Name == "" || member.StudentID == "" || member.Phone == "" || member.College == "" {
			utils.BadRequest(c, fmt.Sprintf("队员[%s]的姓名/学号/手机号/学院不能为空", member.Name))
			return
		}
	}
	if leaderCount == 0 || leaderCount > 1 {
		utils.BadRequest(c, "队员列表必须且仅能指定1名队长")
		return
	}

	var comp models.CompDirectory
	if err := database.DB.WithContext(c.Request.Context()).First(&comp, req.CompID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "赛事不存在")
			return
		}
		utils.InternalServerError(c, "查询赛事失败", err)
		return
	}

	var existingReg models.Register
	existsErr := database.DB.WithContext(c.Request.Context()).Where("comp_id = ? AND leader_id = ?", req.CompID, leaderID).First(&existingReg).Error

	var regID uint

	tx := database.DB.WithContext(c.Request.Context()).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			fmt.Println("事务发生 panic，已回滚:", r)
		}
	}()

	if errors.Is(existsErr, gorm.ErrRecordNotFound) {
		now := time.Now()
		reg := models.Register{
			CompID:         req.CompID,
			LeaderID:       leaderID,
			TeamName:       req.TeamName,
			Status:         3,
			SupplementTime: &now,
			AdvisorInfo:    "{}",
		}
		if err := tx.Create(&reg).Error; err != nil {
			tx.Rollback()
			fmt.Printf("创建补录报名失败，compID=%d, leaderID=%d, err=%v\n", req.CompID, leaderID, err)
			utils.InternalServerError(c, "创建补录报名失败", err)
			return
		}
		regID = reg.ID

		for _, member := range req.Members {
			regMember := models.RegMember{
				RegID:     reg.ID,
				Name:      member.Name,
				StudentID: member.StudentID,
				Phone:     member.Phone,
				Email:     member.Email,
				College:   member.College,
				IsLeader:  member.IsLeader,
				Year:      member.Year,
			}
			if err := tx.Create(&regMember).Error; err != nil {
				tx.Rollback()
				fmt.Printf("存储队员[%s]失败，regID=%d, err=%v\n", member.Name, reg.ID, err)
				utils.InternalServerError(c, fmt.Sprintf("存储队员[%s]失败", member.Name), err)
				return
			}
		}
	} else if existsErr != nil {
		tx.Rollback()
		fmt.Printf("查询报名记录失败，err=%v\n", existsErr)
		utils.InternalServerError(c, "查询报名信息失败", existsErr)
		return
	} else {
		regID = existingReg.ID
		fmt.Printf("检测到已存在的报名记录，regID=%d，跳过补录报名创建\n", regID)
	}

<<<<<<< HEAD
	var awardTime *time.Time
	if req.AwardDate.Year() > 1 {
		awardTime = &req.AwardDate.Time
=======
	var existingAward models.Award
	if err := tx.Where("reg_id = ?", regID).First(&existingAward).Error; err == nil {
		switch existingAward.Status {
		case "rejected":
			if err := tx.Model(&existingAward).Updates(map[string]interface{}{
				"award_level":   req.AwardLevel,
				"award_name":    req.AwardName,
				"status":        "draft",
				"proof_url":     req.ProofURL,
				"reject_reason": "",
			}).Error; err != nil {
				tx.Rollback()
				utils.InternalServerError(c, "重新提交失败", err)
				return
			}
				if existingAward.RegID != 0 {
					if err := tx.Model(&models.Register{}).Where("id = ?", existingAward.RegID).Updates(map[string]interface{}{
						"status": 3,
					}).Error; err != nil {
						tx.Rollback()
						utils.InternalServerError(c, "更新报名状态失败", err)
						return
					}
				}
			if err := tx.Commit().Error; err != nil {
				utils.InternalServerError(c, "事务提交失败", err)
				return
			}
			clearAwardCache(c.Request.Context(), req.CompID)
			utils.SuccessWithMessage(c, "重新申报成功，待审核", gin.H{
				"award_id": existingAward.ID,
				"reg_id":   regID,
			})
			return
		case "approved":
			tx.Rollback()
			utils.BadRequest(c, "该赛事奖项已通过审核，无法重复申报")
			return
		default:
			tx.Rollback()
			utils.BadRequest(c, "该赛事您已提交过获奖申报，请勿重复提交")
			return
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		tx.Rollback()
		utils.InternalServerError(c, "查询已有申报记录失败", err)
		return
>>>>>>> develop
	}

	award := models.Award{
		CompID:     req.CompID,
		RegID:      regID,
		AwardLevel: req.AwardLevel,
		AwardName:  req.AwardName,
		AwardTime:  awardTime,
		Status:     "draft",
		ProofUrl:   req.ProofURL,
		Source:     "supplement",
		LevelRank:  99,
	}
	if err := tx.Create(&award).Error; err != nil {
		tx.Rollback()
		fmt.Printf("创建补录奖项失败，regID=%d, err=%v\n", regID, err)
		utils.InternalServerError(c, "创建补录奖项失败", err)
		return
	}

	if err := tx.Commit().Error; err != nil {
		fmt.Printf("事务提交失败，regID=%d, err=%v\n", regID, err)
		utils.InternalServerError(c, "补录申报失败，请重试", err)
		return
	}

	clearAwardCache(c.Request.Context(), req.CompID)
	utils.SuccessWithMessage(c, "补录申报成功，待审核", gin.H{
		"award_id": award.ID,
		"reg_id":   regID,
	})
}

func GetAwardAuditList(c *gin.Context) {
	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if pageSize == 0 {
		pageSize, _ = strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	keyword := c.Query("keyword")
	statusStr := c.Query("status")
	awardLevel := c.Query("award_level")
	compName := c.Query("comp_name")
	source := c.DefaultQuery("source", "supplement")

	db := database.DB.WithContext(c.Request.Context()).Model(&models.Award{}).
		Joins("JOIN comp_directories ON comp_directories.id = awards.comp_id").
		Preload("Register").
		Preload("Register.Competition").
		Preload("Register.Members")

	if source != "all" {
		db = db.Where("awards.source = ?", source)
	}
	if statusStr != "" {
		statusInt, _ := strconv.Atoi(statusStr)
		db = db.Where("awards.status = ?", mapAwardStatusFromInt(statusInt))
	}
	if awardLevel != "" {
		db = db.Where("awards.award_level = ?", awardLevel)
	}
	if compName != "" {
		db = db.Where("comp_directories.comp_name LIKE ?", "%"+compName+"%")
	}

	if !checkUserIsAdmin(c.Request.Context(), userID) {
		db = db.Where("comp_directories.manager_id = ?", userID)
	}

	var awards []models.Award
	if err := db.Order("awards.create_time DESC").Find(&awards).Error; err != nil {
		utils.InternalServerError(c, "查询失败", err)
		return
	}

	var list []AwardAuditListItem
	for _, a := range awards {
		leaderName, leaderID, phone, _ := getLeaderMember(a.Register.Members, a.Register.Leader)
		if keyword != "" {
			if !strings.Contains(leaderName, keyword) && !strings.Contains(leaderID, keyword) {
				continue
			}
		}

		submitTime := a.CreatedAt.Format("2006-01-02 15:04")
		if a.Register.SupplementTime != nil {
			submitTime = a.Register.SupplementTime.Format("2006-01-02 15:04")
		}

		listAwardDate := "-"
		if a.AwardTime != nil {
			listAwardDate = a.AwardTime.Format("2006-01-02")
		}

		list = append(list, AwardAuditListItem{
			ID:          a.ID,
			StudentName: leaderName,
			StudentID:   leaderID,
			CompName:    a.Register.Competition.CompName,
			AwardLevel:  a.AwardLevel,
			AwardDate:   listAwardDate,
			Phone:       phone,
			SubmitTime:  submitTime,
			Status:      mapAwardStatusToInt(a.Status),
		})
	}

	total := len(list)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	pageList := list[start:end]

	utils.Success(c, gin.H{
		"list":  pageList,
		"total": total,
	})
}

func GetAwardAuditDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	var award models.Award
	if err := database.DB.WithContext(c.Request.Context()).
		Preload("Register").
		Preload("Register.Competition").
		Preload("Register.Members").
		Preload("Register.Leader").
		First(&award, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}

	leaderName, leaderID, phone, email := getLeaderMember(award.Register.Members, award.Register.Leader)
	college := award.Register.Leader.College
	if college == "" {
		for _, m := range award.Register.Members {
			if m.IsLeader {
				college = m.College
				break
			}
		}
	}

	teammates := make([]gin.H, 0)
	for _, m := range award.Register.Members {
		teammates = append(teammates, gin.H{
			"name":       m.Name,
			"student_id": m.StudentID,
			"college":    m.College,
		})
	}

	submitTime := award.CreatedAt.Format("2006-01-02 15:04:05")
	if award.Register.SupplementTime != nil {
		submitTime = award.Register.SupplementTime.Format("2006-01-02 15:04:05")
	}

	awardDate := "-"
	if award.AwardTime != nil {
		awardDate = award.AwardTime.Format("2006-01-02")
	}

	resp := AwardAuditDetailResp{
		ID:            award.ID,
		StudentName:   leaderName,
		StudentID:     leaderID,
		College:       college,
		Phone:         phone,
		Email:         email,
		CompName:      award.Register.Competition.CompName,
		AwardLevel:    award.AwardLevel,
		AwardSpecific: award.AwardName,
		AwardDate:     awardDate,
		TeamName:      award.Register.TeamName,
		Teammates:     teammates,
		CertImage:     award.ProofUrl,
		SubmitTime:    submitTime,
		Status:        mapAwardStatusToInt(award.Status),
		RejectReason:  award.RejectReason,
	}

	utils.Success(c, resp)
}

func PassAwardAudit(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	var award models.Award
	if err := database.DB.WithContext(c.Request.Context()).Preload("Register").First(&award, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}

	if award.Status != "draft" {
		utils.BadRequest(c, "该记录已审核，无法重复操作")
		return
	}

	now := time.Now()
	tx := database.DB.WithContext(c.Request.Context()).Begin()
	if err := tx.Model(&award).Updates(map[string]interface{}{
		"status":        "approved",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": "",
	}).Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "审核更新失败", err)
		return
	}

	if award.RegID != 0 {
		if err := tx.Model(&models.Register{}).Where("id = ?", award.RegID).Updates(map[string]interface{}{
			"status":        4,
			"reject_reason": "",
		}).Error; err != nil {
			tx.Rollback()
			utils.InternalServerError(c, "更新报名状态失败", err)
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}
	clearAwardCache(c.Request.Context(), award.CompID)
	utils.SuccessWithMessage(c, "审核已通过", nil)
}

func RejectAwardAudit(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	var req struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "驳回原因必填")
		return
	}

	var award models.Award
	if err := database.DB.WithContext(c.Request.Context()).Preload("Register").First(&award, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}

	if award.Status != "draft" {
		utils.BadRequest(c, "该记录已审核，无法重复操作")
		return
	}

	now := time.Now()
	tx := database.DB.WithContext(c.Request.Context()).Begin()
	if err := tx.Model(&award).Updates(map[string]interface{}{
		"status":        "rejected",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": req.Reason,
	}).Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "审核更新失败", err)
		return
	}

	if award.RegID != 0 {
		if err := tx.Model(&models.Register{}).Where("id = ?", award.RegID).Updates(map[string]interface{}{
			"status":        5,
			"reject_reason": req.Reason,
		}).Error; err != nil {
			tx.Rollback()
			utils.InternalServerError(c, "更新报名状态失败", err)
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}

	clearAwardCache(c.Request.Context(), award.CompID)
	utils.SuccessWithMessage(c, "已驳回", nil)
}

func BatchPassAwardAudit(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "ids 必填")
		return
	}
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	now := time.Now()
	tx := database.DB.WithContext(c.Request.Context()).Begin()
	result := tx.Model(&models.Award{}).Where("id IN ? AND status = ?", req.IDs, "draft").Updates(map[string]interface{}{
		"status":        "approved",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": "",
	})
	if result.Error != nil {
		tx.Rollback()
		utils.InternalServerError(c, "批量更新失败", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		utils.BadRequest(c, "没有可审核的记录（可能已审核或不存在）")
		return
	}

	var awards []models.Award
	if err := tx.Where("id IN ?", req.IDs).Find(&awards).Error; err == nil {
		compIDSet := make(map[uint]bool)
		for _, a := range awards {
			compIDSet[a.CompID] = true
			if a.RegID != 0 {
				if err := tx.Model(&models.Register{}).Where("id = ?", a.RegID).Updates(map[string]interface{}{
					"status":        4,
					"reject_reason": "",
				}).Error; err != nil {
					tx.Rollback()
					utils.InternalServerError(c, "更新报名状态失败", err)
					return
				}
			}
		}
		defer func() {
			for compID := range compIDSet {
				clearAwardCache(c.Request.Context(), compID)
			}
		}()
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}

	utils.SuccessWithMessage(c, "批量通过成功", nil)
}

func BatchRejectAwardAudit(c *gin.Context) {
	var req struct {
		IDs    []uint `json:"ids" binding:"required"`
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "ids 和 reason 必填")
		return
	}
	userIDVal, _ := c.Get("user_id")
	auditorID := userIDVal.(uint)

	now := time.Now()
	tx := database.DB.WithContext(c.Request.Context()).Begin()
	result := tx.Model(&models.Award{}).Where("id IN ? AND status = ?", req.IDs, "draft").Updates(map[string]interface{}{
		"status":        "rejected",
		"auditor_id":    auditorID,
		"audit_time":    &now,
		"reject_reason": req.Reason,
	})
	if result.Error != nil {
		tx.Rollback()
		utils.InternalServerError(c, "批量更新失败", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		utils.BadRequest(c, "没有可审核的记录（可能已审核或不存在）")
		return
	}

	var awards []models.Award
	if err := tx.Where("id IN ?", req.IDs).Find(&awards).Error; err == nil {
		compIDSet := make(map[uint]bool)
		for _, a := range awards {
			compIDSet[a.CompID] = true
			if a.RegID != 0 {
				if err := tx.Model(&models.Register{}).Where("id = ?", a.RegID).Updates(map[string]interface{}{
					"status":        5,
					"reject_reason": req.Reason,
				}).Error; err != nil {
					tx.Rollback()
					utils.InternalServerError(c, "更新报名状态失败", err)
					return
				}
			}
		}
		defer func() {
			for compID := range compIDSet {
				clearAwardCache(c.Request.Context(), compID)
			}
		}()
	}

	if err := tx.Commit().Error; err != nil {
		utils.InternalServerError(c, "事务提交失败", err)
		return
	}

	utils.SuccessWithMessage(c, "批量驳回成功", nil)
}
