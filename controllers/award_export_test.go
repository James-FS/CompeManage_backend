package controllers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdvisorPairsJSONAndNormalize(t *testing.T) {
	// 空格分隔多人，按序与学院对应
	require.Equal(t, "刘涛 曾岑", normalizeAdvisorList("刘涛  曾岑	王满四")[:len("刘涛 曾岑")]+""[:0])
	require.Equal(t, "刘涛 曾岑 王满四", normalizeAdvisorList("刘涛  曾岑	王满四"))
	// 学院不足时补空串
	got := advisorPairsJSON("刘涛 曾岑", "新闻学院")
	require.JSONEq(t, `[{"id":0,"username":"","name":"刘涛","phone":"","email":"","college":"新闻学院"},{"id":0,"username":"","name":"曾岑","phone":"","email":"","college":""}]`, got)
	// 无姓名返回空
	require.Empty(t, advisorPairsJSON("", ""))
}

func TestAwardExportAdvisors(t *testing.T) {
	// 奖项自带指导教师：姓名拼接、学院去重保序
	names, colleges := awardExportAdvisors(`[{"name":"刘涛","college":"新闻学院"},{"name":"曾岑","college":"创新创业学院"},{"name":"王满四","college":"新闻学院"}]`, "")
	require.Equal(t, "刘涛 曾岑 王满四", names)
	require.Equal(t, "新闻学院 创新创业学院", colleges)
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
	require.Equal(t, "金楠 李秋荣 王菲樱 罗思卓", names)
	require.Equal(t, "新闻与传播学院 公共管理学院 管理学院", colleges)

	// 空成员与空学院
	names, colleges = awardJoinMembers(nil)
	require.Empty(t, names)
	require.Empty(t, colleges)
	names, colleges = awardJoinMembers([]awardExportMember{{AwardID: 1, Name: "张三"}})
	require.Equal(t, "张三", names)
	require.Empty(t, colleges)
}
