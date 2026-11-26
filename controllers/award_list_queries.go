package controllers

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func awardListPagination(pageText, sizeText string) (page, size int) {
	page, _ = strconv.Atoi(pageText)
	size, _ = strconv.Atoi(sizeText)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > MaxPageSize {
		size = 10
	}
	return page, size
}

func filterAwardAuditLeader(q *gorm.DB, keyword string) *gorm.DB {
	if keyword == "" {
		return q
	}
	awardMember := `SELECT am.id FROM award_members am
		WHERE am.award_id = awards.id AND am.delete_time IS NULL`
	regMember := "SELECT rm.id FROM reg_members rm WHERE rm.reg_id = audit_reg.id AND rm.is_leader = 1"
	regUser := "LEFT JOIN users audit_user ON audit_user.id = audit_reg.leader_id AND audit_user.delete_time IS NULL"

	// Match the displayed person, including legacy rows with several leaders:
	// the last visible award leader (or first member), then the registration leader.
	return q.Joins("LEFT JOIN registers audit_reg ON audit_reg.id = awards.reg_id AND audit_reg.delete_time IS NULL").
		Joins("LEFT JOIN award_members audit_member ON audit_member.id = ("+awardMember+
			" ORDER BY CASE WHEN am.is_leader = 1 THEN am.id END DESC, am.id ASC LIMIT 1)").
		Joins("LEFT JOIN reg_members audit_reg_member ON audit_reg_member.id = ("+regMember+
			" ORDER BY rm.id ASC LIMIT 1)").
		Joins(regUser).
		Where(`(INSTR(CAST(CASE
			WHEN audit_member.id IS NOT NULL THEN audit_member.name
			WHEN audit_reg_member.id IS NOT NULL THEN audit_reg_member.name
			ELSE audit_user.realname END AS BINARY), ?) > 0
		OR INSTR(CAST(CASE
			WHEN audit_member.id IS NOT NULL THEN audit_member.student_number
			WHEN audit_reg_member.id IS NOT NULL THEN audit_reg_member.username
			ELSE audit_user.username END AS BINARY), ?) > 0)`, keyword, keyword)
}

func filterCompetitionAwards(q *gorm.DB, c *gin.Context) *gorm.DB {
	switch category := c.Query("category"); category {
	case "校级":
		q = q.Where("awards.award_category IN ?", []string{"校级", "校赛"})
	case "国家级":
		q = q.Where("awards.award_category IN ?", []string{"国家级", "国际级"})
	case "":
	default:
		q = q.Where("awards.award_category = ?", category)
	}
	if level := c.Query("level"); level != "" {
		var aliases []string
		switch level {
		case "一等奖", "金奖", "冠军":
			aliases = []string{"一等", "冠军", "金奖", "特等奖"}
		case "二等奖", "银奖", "亚军":
			aliases = []string{"二等", "亚军", "银奖"}
		case "三等奖", "铜奖", "季军":
			aliases = []string{"三等", "季军", "铜奖"}
		default:
			aliases = []string{level}
		}
		conditions := make([]string, len(aliases))
		args := make([]any, len(aliases))
		for i, alias := range aliases {
			conditions[i] = "INSTR(CAST(REGEXP_REPLACE(awards.award_name, '[[:space:]　（）()]', '') AS BINARY), ?) > 0"
			args[i] = alias
		}
		q = q.Where("("+strings.Join(conditions, " OR ")+")", args...)
	}
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
	if keyword == "" {
		return q
	}

	memberMatch := `SELECT 1 FROM award_members search_member
		WHERE search_member.award_id = awards.id AND search_member.delete_time IS NULL
		AND INSTR(CAST(LOWER(CONCAT(COALESCE(search_member.name, ''), ' ',
			COALESCE(search_member.student_number, ''), ' ', COALESCE(search_member.college, ''))) AS BINARY), ?) > 0`
	advisorText := `SELECT GROUP_CONCAT(CONCAT(COALESCE(search_advisor.name, ''), ' ',
			COALESCE(search_advisor.username, ''), ' ', COALESCE(search_advisor.college, ''))
			ORDER BY search_advisor.sort, search_advisor.id SEPARATOR ' ')
		FROM award_advisors search_advisor
		WHERE search_advisor.award_id = awards.id AND search_advisor.delete_time IS NULL`
	legacyAdvisors := `CASE JSON_TYPE(awards.advisor_info)
		WHEN 'ARRAY' THEN awards.advisor_info
		WHEN 'OBJECT' THEN JSON_ARRAY(awards.advisor_info)
		ELSE JSON_ARRAY() END`
	legacyText := `SELECT GROUP_CONCAT(COALESCE(search_legacy.name, '') ORDER BY search_legacy.ordinal_no SEPARATOR '、')
		FROM JSON_TABLE(` + legacyAdvisors + `, '$[*]' COLUMNS(
		ordinal_no FOR ORDINALITY, name VARCHAR(255) PATH '$.name'
	)) search_legacy`

	return q.Where(`(INSTR(CAST(LOWER(COALESCE(NULLIF(awards.project_name, ''),
		(SELECT r.team_name FROM registers r WHERE r.id = awards.reg_id AND r.delete_time IS NULL))) AS BINARY), ?) > 0
		OR EXISTS (`+memberMatch+`)
		OR INSTR(CAST(LOWER(CONCAT(COALESCE((`+legacyText+`), ''), ' ',
			COALESCE((`+advisorText+`), ''))) AS BINARY), ?) > 0)`, keyword, keyword, keyword)
}
