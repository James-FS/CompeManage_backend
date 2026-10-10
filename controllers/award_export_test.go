package controllers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
