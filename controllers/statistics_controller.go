package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未登录"})
		return
	}
	userID := userIDVal.(uint)
	isAdmin := checkUserIsAdmin(userID)

	months, _ := strconv.Atoi(c.DefaultQuery("months", "6"))
	if months <= 0 || months > 24 {
		months = 6
	}

	newCompBase := func() *gorm.DB {
		q := database.DB.Model(&models.CompDirectory{})
		if !isAdmin {
			q = q.Where("manager_id = ?", userID)
		}
		return q
	}

	newRegBase := func() *gorm.DB {
		q := database.DB.Model(&models.Register{}).
			Joins("JOIN comp_directories ON comp_directories.id = registers.comp_id")
		if !isAdmin {
			q = q.Where("comp_directories.manager_id = ?", userID)
		}
		return q
	}

	newAwardBase := func() *gorm.DB {
		q := database.DB.Model(&models.Award{}).
			Joins("JOIN registers ON registers.id = awards.reg_id").
			Joins("JOIN comp_directories ON comp_directories.id = registers.comp_id")
		if !isAdmin {
			q = q.Where("comp_directories.manager_id = ?", userID)
		}
		return q
	}

	newSummaryBase := func() *gorm.DB {
		q := database.DB.Model(&models.Summary{}).
			Joins("JOIN comp_directories ON comp_directories.id = summaries.comp_id")
		if !isAdmin {
			q = q.Where("comp_directories.manager_id = ?", userID)
		}
		return q
	}

	newDeclareBase := func() *gorm.DB {
		q := database.DB.Model(&models.CompDeclaration{})
		if !isAdmin {
			q = q.Where("created_by = ?", userID)
		}
		return q
	}

	var totalCompetitions int64
	if err := newCompBase().Count(&totalCompetitions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询赛事总数失败", "error": err.Error()})
		return
	}

	var totalRegistrations int64
	if err := newRegBase().Count(&totalRegistrations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询报名总数失败", "error": err.Error()})
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

	c.JSON(http.StatusOK, gin.H{
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
			"funnel": funnel,
			"todos":  todos,
		},
	})
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
