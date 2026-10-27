package controllers

import (
	"testing"

	"CompeManage_backend/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 回归：赛事编号序号跨 999→1000 时旧实现取末 3 位 + 字符串排序 → 生成已存在编号
// → 创建赛事持续 500（上线阻断）。修复后按 base 后完整序号 + 长度优先排序取最大。
func TestNextCompetitionCodeMySQL_Crosses999(t *testing.T) {
	db := newAwardMySQLTestDB(t)

	// comp_directories.manager_id 有外键，先建用户
	mgr := models.User{Username: "CODEMGR1", Realname: "编号测试负责人", College: "计算机学院", IdentityType: "teacher"}
	require.NoError(t, db.Create(&mgr).Error)
	newComp := func(code, name, level string, year int) models.CompDirectory {
		return models.CompDirectory{CompCode: code, CompName: name, CompLevel: level, Year: year, ManagerID: mgr.ID}
	}

	c1 := newComp("X2026999", "编号测试甲", "校级", 2026)
	require.NoError(t, db.Create(&c1).Error)
	c2 := newComp("X20261000", "编号测试乙", "校级", 2026)
	require.NoError(t, db.Create(&c2).Error)

	code, err := nextCompetitionCode(db, "校级", 2026)
	require.NoError(t, err)
	assert.Equal(t, "X20261001", code)

	// 3 位序号内的常规续号不受影响
	c3 := newComp("S2025005", "编号测试丙", "省级", 2025)
	require.NoError(t, db.Create(&c3).Error)
	code2, err := nextCompetitionCode(db, "省级", 2025)
	require.NoError(t, err)
	assert.Equal(t, "S2025006", code2)

	// 空表年份 → 001
	code3, err := nextCompetitionCode(db, "国家级", 2027)
	require.NoError(t, err)
	assert.Equal(t, "G2027001", code3)
}

// 级别不支持
func TestNextCompetitionCodeMySQL_BadLevel(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	_, err := nextCompetitionCode(db, "市级", 2026)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不支持的竞赛级别")
}
