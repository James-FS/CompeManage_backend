// statistics_controller.go
package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/logger"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type competitionStatItem struct {
	ID             uint   `json:"id"`
	CompName       string `json:"comp_name"`
	CompLevel      string `json:"comp_level"`
	CollegeName    string `json:"college_name"`
	RegCount       int64  `json:"reg_count"`   // 报名人数
	AwardCount     int64  `json:"award_count"` // 获奖人数
	AwardTeamCount int64  `json:"award_team_count"`
	SummaryStatus  int8   `json:"summary_status"` // 总结状态：0未归档 1已归档
}

type levelDistributionItem struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

type collegeRankItem struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

type funnelItem struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

type todoItem struct {
	Title string `json:"title"`
	Value int64  `json:"value"`
	Path  string `json:"path"`
}

func calcPercent(numerator int64, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(int((float64(numerator)/float64(denominator))*10000)) / 100
}

// GetStatisticsDashboard 统计看板数据聚合接口
func GetStatisticsDashboard(c *gin.Context) {
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return
	}
	managedCollegeID := uint(0)
	if scope.ManagedCollegeID != nil {
		managedCollegeID = *scope.ManagedCollegeID
	}
	months, _ := strconv.Atoi(c.DefaultQuery("months", "6"))
	if months <= 0 || months > 24 {
		months = 6
	}
	rdb := middleware.GetRedisClient()
	ctx := c.Request.Context()
	generation := ""
	if scope.IsCollegeAdmin() {
		// The source can change when the manager's profile is synchronized.
		rdb = nil
	}
	if rdb != nil {
		generation, _ = rdb.Get(ctx, "cache:stats:dashboard:generation").Result()
	}
	cacheKey := fmt.Sprintf("cache:stats:dashboard:v6:%s:%d:%s:%d:m%d", generation, scope.UserID, scope.RoleCode, managedCollegeID, months)
	includeCompetitionStats := c.Query("include_competition_stats") != "false"
	if !includeCompetitionStats {
		cacheKey += ":overview"
	}

	cacheData := ""
	if rdb != nil {
		cacheData, _ = rdb.Get(ctx, cacheKey).Result()
	}
	if cacheData != "" {
		var cachedResp gin.H
		if json.Unmarshal([]byte(cacheData), &cachedResp) == nil {
			logger.Info("统计看板缓存命中", "cache_key", cacheKey)
			c.JSON(http.StatusOK, cachedResp)
			return
		}
	}
	newCompBase := func() *gorm.DB {
		q := database.DB.WithContext(ctx).Model(&models.CompDirectory{})
		return applyCompetitionScope(q, scope, "comp_directories")
	}

	newRegBase := func() *gorm.DB {
		q := database.DB.WithContext(ctx).Model(&models.Register{}).
			Joins("JOIN comp_directories ON comp_directories.id = registers.comp_id AND comp_directories.delete_time IS NULL")
		return applyCompetitionScope(q, scope, "comp_directories")
	}

	newAwardBase := func() *gorm.DB {
		q := database.DB.WithContext(ctx).Model(&models.Award{}).
			Joins("JOIN comp_directories ON comp_directories.id = awards.comp_id AND comp_directories.delete_time IS NULL")
		return applyCompetitionScope(q, scope, "comp_directories")
	}

	newSummaryBase := func() *gorm.DB {
		q := database.DB.WithContext(ctx).Model(&models.Summary{}).
			Joins("JOIN comp_directories ON comp_directories.id = summaries.comp_id AND comp_directories.delete_time IS NULL")
		return applyCompetitionScope(q, scope, "comp_directories")
	}

	newDeclareBase := func() *gorm.DB {
		q := database.DB.WithContext(ctx).Model(&models.CompDeclaration{})
		switch scope.RoleCode {
		case "school_admin":
			return q
		case "college_admin":
			return q.Where("college_id = ?", *scope.ManagedCollegeID)
		default:
			return q.Where("created_by = ?", scope.UserID)
		}
	}

	var totalCompetitions int64
	if err := newCompBase().Count(&totalCompetitions).Error; err != nil {
		utils.InternalServerError(c, "查询赛事总数失败", err)
		return
	}

	var totalRegistrations int64
	if err := newRegBase().Count(&totalRegistrations).Error; err != nil {
		utils.InternalServerError(c, "查询报名总数失败", err)
		return
	}

	var regPending int64
	_ = newRegBase().Where("registers.status IN ?", []int{0, 3}).Count(&regPending).Error
	var regPassed int64
	_ = newRegBase().Where("registers.status IN ?", []int{1, 4}).Count(&regPassed).Error

	var declarePending int64
	_ = newDeclareBase().Where("declare_status = ?", 1).Count(&declarePending).Error
	var declareTotal int64
	_ = newDeclareBase().Count(&declareTotal).Error

	var awardPending int64
	_ = newAwardBase().Where("awards.status IN ?", []string{"draft", "pending", "0"}).Count(&awardPending).Error
	var awardApproved int64
	_ = newAwardBase().Where("awards.status IN ?", []string{"approved", "1"}).Count(&awardApproved).Error
	var awardStudents int64
	_ = newAwardBase().Joins("JOIN award_members ON award_members.award_id = awards.id AND award_members.delete_time IS NULL").Where("awards.status IN ?", []string{"approved", "1"}).Count(&awardStudents).Error

	var summaryArchived int64
	_ = newSummaryBase().Where("summaries.status = ?", 1).Count(&summaryArchived).Error

	pendingAudits := declarePending + regPending + awardPending

	levelRows := make([]levelDistributionItem, 0)
	if err := newCompBase().Select("COALESCE(NULLIF(TRIM(comp_level), ''), '未分类') as name, COUNT(*) as value").
		Group("name").
		Order("value desc").
		Scan(&levelRows).Error; err != nil {
		levelRows = []levelDistributionItem{}
	}

	collegeRows := make([]collegeRankItem, 0)
	if err := newRegBase().
		Joins("LEFT JOIN users ON users.id = registers.leader_id").
		Select("COALESCE(users.college, '未知学院') as name, COUNT(*) as value").
		Group("users.college").
		Order("value desc").
		Limit(8).
		Scan(&collegeRows).Error; err != nil {
		collegeRows = []collegeRankItem{}
	}

	monthLabels := make([]string, 0, months)
	monthStartMap := make(map[string]time.Time, months)
	now := time.Now()
	firstMonth := time.Date(now.Year(), now.Month()-time.Month(months-1), 1, 0, 0, 0, 0, time.Local)
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.Local)
	for i := months - 1; i >= 0; i-- {
		m := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.Local)
		key := m.Format("2006-01")
		monthLabels = append(monthLabels, key)
		monthStartMap[key] = m
	}

	regMonthMap := map[string]int64{}
	awardMonthMap := map[string]int64{}

	type monthCount struct {
		Month string `json:"month"`
		Value int64  `json:"value"`
	}

	var regMonthRows []monthCount
	regTrendQuery := newRegBase().Select("DATE_FORMAT(registers.create_time, '%Y-%m') as month, COUNT(*) as value").
		Where("registers.create_time >= ? AND registers.create_time < ?", firstMonth, nextMonth).
		Group("DATE_FORMAT(registers.create_time, '%Y-%m')")
	_ = regTrendQuery.Scan(&regMonthRows).Error
	for _, item := range regMonthRows {
		if _, ok := monthStartMap[item.Month]; ok {
			regMonthMap[item.Month] = item.Value
		}
	}

	var awardMonthRows []monthCount
	awardTrendQuery := newAwardBase().Select("DATE_FORMAT(awards.create_time, '%Y-%m') as month, COUNT(*) as value").
		Where("awards.create_time >= ? AND awards.create_time < ?", firstMonth, nextMonth).
		Group("DATE_FORMAT(awards.create_time, '%Y-%m')")
	_ = awardTrendQuery.Scan(&awardMonthRows).Error
	for _, item := range awardMonthRows {
		if _, ok := monthStartMap[item.Month]; ok {
			awardMonthMap[item.Month] = item.Value
		}
	}

	regSeries := make([]int64, 0, len(monthLabels))
	awardSeries := make([]int64, 0, len(monthLabels))
	for _, month := range monthLabels {
		regSeries = append(regSeries, regMonthMap[month])
		awardSeries = append(awardSeries, awardMonthMap[month])
	}

	funnel := []funnelItem{
		{Name: "已申报", Value: declareTotal},
		{Name: "已报名", Value: totalRegistrations},
		{Name: "已获奖", Value: awardApproved},
		{Name: "已归档总结", Value: summaryArchived},
	}

	todos := []todoItem{
		{Title: "赛事申报待审核", Value: declarePending, Path: "/competition/audit"},
		{Title: "报名待审核", Value: regPending, Path: "/register/audit"},
		{Title: "获奖待审核", Value: awardPending, Path: "/award/audit"},
		{Title: "总结待归档", Value: maxInt64(totalCompetitions-summaryArchived, 0), Path: "/summary/summary-list"},
	}

	responseData := gin.H{
		"code": 200,
		"msg":  "获取成功",
		"data": gin.H{
			"summary": gin.H{
				"total_competitions":     totalCompetitions,
				"total_registrations":    totalRegistrations,
				"pending_audits":         pendingAudits,
				"total_awards":           awardApproved,
				"total_award_students":   awardStudents,
				"registration_pass_rate": calcPercent(regPassed, totalRegistrations),
				"summary_archive_rate":   calcPercent(summaryArchived, totalCompetitions),
			},
			"distributions": gin.H{
				"level":   levelRows,
				"college": collegeRows,
			},
			"trend": gin.H{
				"months":        monthLabels,
				"registrations": regSeries,
				"awards":        awardSeries,
			},
			"funnel": funnel,
			"todos":  todos,
		},
	}
	if includeCompetitionStats {
		compStats := make([]competitionStatItem, 0)
		if err := dashboardCompetitionStats(newCompBase().
			Joins("LEFT JOIN users comp_manager ON comp_manager.id = comp_directories.manager_id AND comp_manager.delete_time IS NULL")).Scan(&compStats).Error; err != nil {
			logger.Error("查询单项赛事统计失败", "error", err)
			compStats = []competitionStatItem{}
		}
		responseData["data"].(gin.H)["competition_stats"] = compStats
	} else {
		colleges := make([]string, 0)
		if err := newCompBase().Joins("LEFT JOIN users comp_manager ON comp_manager.id = comp_directories.manager_id AND comp_manager.delete_time IS NULL").
			Distinct("COALESCE(NULLIF(TRIM(comp_manager.college), ''), '未知学院')").
			Pluck("COALESCE(NULLIF(TRIM(comp_manager.college), ''), '未知学院')", &colleges).Error; err != nil {
			logger.Error("查询赛事所属单位失败", "error", err)
		}
		responseData["data"].(gin.H)["competition_colleges"] = colleges
	}

	// 4. 异步写入 Redis，设置 60 秒过期（2026-10-29：原 10 分钟在目录批量导入后
	// 长时间显示陈旧的"赛事总数=1"，配合 InvalidateStatisticsDashboard 即时失效）
	go func() {
		asyncCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		data, err := json.Marshal(responseData)
		if err != nil {
			logger.Error("异步缓存JSON序列化失败", "cache_key", cacheKey, "error", err)
			return
		}
		if rdb == nil {
			return
		}
		if err := rdb.Set(asyncCtx, cacheKey, data, 60*time.Second).Err(); err != nil {
			logger.Error("异步缓存写入失败", "cache_key", cacheKey, "error", err)
		}
	}()

	// responseData 已经是完整响应信封。与 Redis 缓存命中时直接返回的结构保持一致，
	// 避免缓存未命中时被 utils.Success 再包一层，导致前端读不到 data.summary。
	c.JSON(http.StatusOK, responseData)
}

const dashboardAwardMemberRows = `FROM award_members
	JOIN awards ON awards.id = award_members.award_id
	WHERE awards.comp_id = comp_directories.id AND award_members.delete_time IS NULL
		AND awards.delete_time IS NULL AND awards.status IN ('approved', '1')`

const dashboardAwardMemberCount = "(SELECT COUNT(*) " + dashboardAwardMemberRows + ")"
const dashboardRegistrationCount = `(SELECT COUNT(*) FROM registers
	WHERE registers.comp_id = comp_directories.id AND registers.delete_time IS NULL)`
const dashboardAwardTeamCount = `(SELECT COUNT(*) FROM awards WHERE awards.comp_id = comp_directories.id
	AND awards.delete_time IS NULL AND awards.status IN ('approved', '1'))`

func dashboardCompetitionStats(q *gorm.DB) *gorm.DB {
	return q.Select(`comp_directories.id, comp_directories.comp_name, comp_directories.comp_level,
		COALESCE(NULLIF(TRIM(comp_manager.college), ''), '未知学院') AS college_name,
		` + dashboardRegistrationCount + ` AS reg_count,
		` + dashboardAwardMemberCount + ` AS award_count,
		` + dashboardAwardTeamCount + ` AS award_team_count,
		COALESCE((SELECT status FROM summaries WHERE summaries.comp_id = comp_directories.id
			AND summaries.delete_time IS NULL LIMIT 1), 0) AS summary_status`)
}

func GetDashboardCompetitionStats(c *gin.Context) {
	scope, ok := requireUserAccessScope(c)
	if !ok {
		return
	}
	page, size := awardListPagination(c.Query("page"), c.Query("page_size"))
	sortExpression := dashboardAwardMemberCount
	switch c.Query("sort") {
	case "reg_count":
		sortExpression = dashboardRegistrationCount
	case "award_team_count":
		sortExpression = dashboardAwardTeamCount
	}
	direction := " DESC"
	if c.Query("order") == "ascending" {
		direction = " ASC"
	}

	db := database.DB.WithContext(c.Request.Context()).Model(&models.CompDirectory{}).
		Joins("LEFT JOIN users comp_manager ON comp_manager.id = comp_directories.manager_id AND comp_manager.delete_time IS NULL")
	db = applyCompetitionScope(db, scope, "comp_directories")
	if name := strings.ToLower(strings.TrimSpace(c.Query("name"))); name != "" {
		db = db.Where("INSTR(CAST(LOWER(TRIM(comp_directories.comp_name)) AS BINARY), ?) > 0", name)
	}
	if level := strings.TrimSpace(c.Query("level")); level != "" {
		db = db.Where("COALESCE(NULLIF(TRIM(comp_directories.comp_level), ''), '未分类') = ?", level)
	}
	if college := strings.TrimSpace(c.Query("college")); college == "-" {
		db = db.Where("COALESCE(TRIM(comp_manager.college), '') IN ('', '未知学院')")
	} else if college != "" {
		db = db.Where("TRIM(comp_manager.college) = ?", college)
	}
	ranking := c.Query("ranking") == "true"
	if ranking {
		db = db.Where("EXISTS (SELECT 1 " + dashboardAwardMemberRows + ")")
		direction = " DESC"
	}

	var total int64
	if err := db.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		utils.InternalServerError(c, "查询赛事统计总数失败", err)
		return
	}
	list := make([]competitionStatItem, 0, size)
	if total > 0 && int64(page-1) <= (total-1)/int64(size) {
		offset := (page - 1) * size
		var query *gorm.DB
		if ranking {
			query = db.Select("comp_directories.id, comp_directories.comp_name, " + dashboardAwardMemberCount + " AS award_count").
				Order("award_count DESC, comp_directories.id ASC").Offset(offset).Limit(size)
		} else {
			// Sort the filtered IDs by one metric, then calculate the remaining columns only for the page.
			pageIDs := db.Select("comp_directories.id, " + sortExpression + " AS sort_value").
				Order("sort_value" + direction + ", comp_directories.id ASC").Offset(offset).Limit(size)
			query = dashboardCompetitionStats(database.DB.WithContext(c.Request.Context()).Model(&models.CompDirectory{}).
				Joins("JOIN (?) dashboard_page ON dashboard_page.id = comp_directories.id", pageIDs).
				Joins("LEFT JOIN users comp_manager ON comp_manager.id = comp_directories.manager_id AND comp_manager.delete_time IS NULL")).
				Order("dashboard_page.sort_value" + direction + ", comp_directories.id ASC")
		}
		if err := query.Scan(&list).Error; err != nil {
			utils.InternalServerError(c, "查询赛事统计失败", err)
			return
		}
	}
	utils.Success(c, gin.H{"list": list, "total": total})
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// InvalidateStatisticsDashboard 批量导入/审核等数据变更后即时失效统计看板缓存。
// 与 comp_controller 的缓存清扫保持一致（generation 代际）。
func InvalidateStatisticsDashboard() {
	rdb := middleware.GetRedisClient()
	if rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rdb.Incr(ctx, "cache:stats:dashboard:generation")
}
