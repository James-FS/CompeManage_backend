package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"CompeManage_backend/database"
	"CompeManage_backend/models"
)

// 回归 2026-10-29：buildAwardSummaryRows 的 Join 别名是 cd/cn/cdp，
// 旧实现经 awardCompetitionScope 拼 comp_directories.* 前缀（表名/别名不一致 →
// 恒过滤为空 → 导出汇总 sheet 无数据）。修复后 school_admin 可见省级以上全部获奖行。
func TestBuildAwardSummaryRows_AliasFix(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	mgr := models.User{Username: "SUMMGR1", Realname: "汇总负责人", College: "计算机学院", IdentityType: "staff"}
	require.NoError(t, db.Create(&mgr).Error)
	comp := models.CompDirectory{CompCode: "S2026001", CompName: "汇总修复赛", CompLevel: "省级", Year: 2026, ManagerID: mgr.ID}
	require.NoError(t, db.Create(&comp).Error)
	award := models.Award{
		CompID: comp.ID, ProjectName: "汇总修复项目", ParticipationType: "team",
		AwardName: "一等奖", AwardLevel: "省级一等奖", AwardCategory: "省级", Status: "approved", AdvisorInfo: "[]",
	}
	require.NoError(t, db.Create(&award).Error)

	scope := &UserAccessScope{UserID: 1, RoleCode: "school_admin"}
	rows, err := buildAwardSummaryRows(context.Background(), scope, "")
	require.NoError(t, err)
	require.Len(t, rows, 1, "修复前此处为 0 行（恒过滤）")
	assert.Equal(t, "汇总修复赛", rows[0][2])
}
