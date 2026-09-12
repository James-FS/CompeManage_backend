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
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type competitionStatItem struct {
	ID            uint   `json:"id"`
	CompName      string `json:"comp_name"`
	CompLevel     string `json:"comp_level"`
	CollegeName   string `json:"college_name"`
	RegCount      int64  `json:"reg_count"`      // 报名人数
	AwardCount    int64  `json:"award_count"`    // 获奖人数
	SummaryStatus int8   `json:"summary_status"` // 总结状态：0未归档 1已归档
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
	cacheKey := fmt.Sprintf("cache:stats:dashboard:%d:%s:%d", scope.UserID, scope.RoleCode, managedCollegeID)
	rdb := middleware.GetRedisClient()
	ctx := c.Request.Context()

	cacheData, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		var cachedResp gin.H
		if json.Unmarshal([]byte(cacheData), &cachedResp) == nil {
			logger.Info("统计看板缓存命中", "cache_key", cacheKey)
			c.JSON(http.StatusOK, cachedResp)
			return
		}
	}
	months, _ := strconv.Atoi(c.DefaultQuery("months", "6"))
	if months <= 0 || months > 24 {
		months = 6
	}

	newCompBase := func() *gorm.DB {
		q := database.DB.WithContext(ctx).Model(&models.CompDirectory{})
		return applyCompetitionScope(q, scope, "comp_directories")
	}

	newRegBase := func() *gorm.DB {
		q := database.DB.WithContext(ctx).Model(&models.Register{}).
			Joins("JOIN comp_directories ON comp_directories.id = registers.comp_id")
		return applyCompetitionScope(q, scope, "comp_directories")
	}

	newAwardBase := func() *gorm.DB {
		q := database.DB.WithContext(ctx).Model(&models.Award{}).
			Joins("JOIN registers ON registers.id = awards.reg_id").
			Joins("JOIN comp_directories ON comp_directories.id = registers.comp_id")
		return applyCompetitionScope(q, scope, "comp_directories")
	}

	newSummaryBase := func() *gorm.DB {
		q := database.DB.WithContext(ctx).Model(&models.Summary{}).
			Joins("JOIN comp_directories ON comp_directories.id = summaries.comp_id")
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

	var summaryArchived int64
	_ = newSummaryBase().Where("summaries.status = ?", 1).Count(&summaryArchived).Error

	pendingAudits := declarePending + regPending + awardPending

	levelRows := make([]levelDistributionItem, 0)
	if err := newCompBase().Select("COALESCE(comp_level, '未分类') as name, COUNT(*) as value").
		Group("comp_level").
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
		Group("DATE_FORMAT(registers.create_time, '%Y-%m')")
	_ = regTrendQuery.Scan(&regMonthRows).Error
	for _, item := range regMonthRows {
		if _, ok := monthStartMap[item.Month]; ok {
			regMonthMap[item.Month] = item.Value
		}
	}

	var awardMonthRows []monthCount
	awardTrendQuery := newAwardBase().Select("DATE_FORMAT(awards.create_time, '%Y-%m') as month, COUNT(*) as value").
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

	sort.SliceStable(funnel, func(i, j int) bool {
		return funnel[i].Value > funnel[j].Value
	})

	todos := []todoItem{
		{Title: "赛事申报待审核", Value: declarePending, Path: "/competition/audit"},
		{Title: "报名待审核", Value: regPending, Path: "/register/audit"},
		{Title: "获奖待审核", Value: awardPending, Path: "/award/audit"},
		{Title: "总结待归档", Value: maxInt64(totalCompetitions-summaryArchived, 0), Path: "/summary/summary-list"},
	}

	var compStats []competitionStatItem
	// 这里通过子查询或 Join 来获取每个赛事的报名数和获奖数
	err = newCompBase().
		Joins("LEFT JOIN colleges ON colleges.id = comp_directories.college_id").
		Select("comp_directories.id, comp_directories.comp_name, comp_directories.comp_level, COALESCE(colleges.name, '未知学院') as college_name, " +
			"(SELECT COUNT(*) FROM registers WHERE registers.comp_id = comp_directories.id) as reg_count, " +
			"(SELECT COUNT(*) FROM awards JOIN registers ON awards.reg_id = registers.id WHERE registers.comp_id = comp_directories.id AND (awards.status = '1' OR awards.status = 'approved')) as award_count, " +
			"COALESCE((SELECT status FROM summaries WHERE summaries.comp_id = comp_directories.id LIMIT 1), 0) as summary_status").
		Scan(&compStats).Error

	if err != nil {
		logger.Error("查询单项赛事统计失败", "error", err)
		compStats = []competitionStatItem{}
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
			"funnel":            funnel,
			"todos":             todos,
			"competition_stats": compStats,
		},
	}

	// 4. 异步写入 Redis，设置 10 分钟过期
	go func() {
		asyncCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		data, err := json.Marshal(responseData)
		if err != nil {
			logger.Error("异步缓存JSON序列化失败", "cache_key", cacheKey, "error", err)
			return
		}
		if err := rdb.Set(asyncCtx, cacheKey, data, 10*time.Minute).Err(); err != nil {
			logger.Error("异步缓存写入失败", "cache_key", cacheKey, "error", err)
		}
	}()

	utils.Success(c, responseData)
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
