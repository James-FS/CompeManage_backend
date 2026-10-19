package controllers

import (
	"CompeManage_backend/models"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdvisorsMirrorAndRowsJoin(t *testing.T) {
	require.Equal(t, "刘涛 曾岑 王满四", normalizeAdvisorList("刘涛  曾岑	王满四"))
	// 镜像 JSON 保留三元组
	pairs := []models.AdvisorInfo{
		{Username: "T2023010", Name: "刘涛", College: "新闻学院"},
		{Username: "T2023011", Name: "曾岑", College: "创新创业学院"},
	}
	mirror := advisorsMirrorJSON(pairs)
	require.Contains(t, mirror, "T2023010")
	require.Contains(t, mirror, "新闻学院")
	require.Empty(t, advisorsMirrorJSON(nil))
	// 多行数据拼接：姓名按序、学院去重保序
	names, colleges := awardExportAdvisorsFromRows([]models.AwardAdvisor{
		{Name: "刘涛", College: "新闻学院"},
		{Name: "曾岑", College: "创新创业学院"},
		{Name: "王满四", College: "新闻学院"},
	})
	require.Equal(t, "刘涛、曾岑、王满四", names)
	require.Equal(t, "新闻学院、创新创业学院", colleges)
}

func TestAwardExportAdvisors(t *testing.T) {
	// 奖项自带指导教师：姓名拼接、学院去重保序
	names, colleges := awardExportAdvisors(`[{"name":"刘涛","college":"新闻学院"},{"name":"曾岑","college":"创新创业学院"},{"name":"王满四","college":"新闻学院"}]`, "")
	require.Equal(t, "刘涛、曾岑、王满四", names)
	require.Equal(t, "新闻学院、创新创业学院", colleges)
	// 奖项为空时回退报名（单对象格式）
	names, colleges = awardExportAdvisors("", `{"name":"白鹤云飞","college":"地理学院"}`)
	require.Equal(t, "白鹤云飞", names)
	require.Equal(t, "地理学院", colleges)
	// 空对象/空数组/缺失 均无输出
	names, colleges = awardExportAdvisors("{}", "[]")
	require.Empty(t, names)
	require.Empty(t, colleges)
}

func TestAwardMergedLevel(t *testing.T) {
	// 导入时 award_level 已拼接完整，直接使用
	require.Equal(t, "国家级一等奖", awardMergedLevel("国家级一等奖", "国家级", "一等奖"))
	// 旧数据缺失 award_level 时回退拼接
	require.Equal(t, "省级二等奖", awardMergedLevel("", "省级", "二等奖"))
	require.Equal(t, "校赛三等奖", awardMergedLevel("", "校赛", "三等奖"))
}

func TestAwardParticipantLabel(t *testing.T) {
	team := int8(2)
	individual := int8(1)
	other := int8(9)
	require.Equal(t, "团体", awardParticipantLabel(&team))
	require.Equal(t, "个人", awardParticipantLabel(&individual))
	require.Equal(t, "个人", awardParticipantLabel(&other))
	require.Equal(t, "", awardParticipantLabel(nil))
}

func TestAwardJoinMembers(t *testing.T) {
	// 同项目多学院学生：姓名全拼接，学院去重保序
	names, colleges := awardJoinMembers([]awardExportMember{
		{AwardID: 1, Name: "金楠", College: "新闻与传播学院"},
		{AwardID: 1, Name: "李秋荣", College: "公共管理学院"},
		{AwardID: 1, Name: "王菲樱", College: "公共管理学院"},
		{AwardID: 1, Name: "罗思卓", College: "管理学院"},
	})
	require.Equal(t, "金楠、李秋荣、王菲樱、罗思卓", names)
	require.Equal(t, "新闻与传播学院、公共管理学院、管理学院", colleges)

	// 空成员与空学院
	names, colleges = awardJoinMembers(nil)
	require.Empty(t, names)
	require.Empty(t, colleges)
	names, colleges = awardJoinMembers([]awardExportMember{{AwardID: 1, Name: "张三"}})
	require.Equal(t, "张三", names)
	require.Empty(t, colleges)
}
