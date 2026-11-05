package controllers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"CompeManage_backend/database"
	"CompeManage_backend/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// ===================== ResubmitRegistration 分支补齐 =====================

func resubmitBody() map[string]interface{} {
	return map[string]interface{}{
		"comp_id":  1,
		"team_name": "重报项目",
		"leader": map[string]interface{}{
			"name": "张三", "stuID": "S001", "phone": "138", "college": "计算机学院",
		},
	}
}

func TestResubmitRegistration_UserIDTypeError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	_, w, c := buildAuthPOSTJSON(resubmitBody())
	c.Set("user_id", "not-a-uint")
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestResubmitRegistration_MissingRegID(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	body := resubmitBody()
	body["participation_type"] = "team"
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "缺少原报名记录ID")
}

func TestResubmitRegistration_DetailQueryError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).AddRow(5, 1, 1, 0))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnError(fmt.Errorf("db error"))

	_, w, c := buildAuthPOSTJSON(resubmitBody())
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestResubmitRegistration_ParticipationInvalid(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).AddRow(5, 1, 1, 0))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "participant_type", "min_team_member", "max_team_member"}).
			AddRow(1, 1, 1, 1, 5))

	body := resubmitBody()
	body["participation_type"] = "team" // 个人赛不允许团队形式
	body["reg_id"] = 5
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "个人赛只能由本人单独参赛")
}

func TestResubmitRegistration_LeaderNotFound(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).AddRow(5, 1, 1, 0))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "participant_type", "min_team_member", "max_team_member"}).
			AddRow(1, 1, 2, 1, 5))
	// validateRegistrationParticipation：申请人在成员中
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))
	// 队长学号回查失败
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := resubmitBody()
	body["participation_type"] = "individual"
	body["reg_id"] = 5
	body["leader"] = map[string]interface{}{"name": "张三", "stuID": "S001", "phone": "138", "college": "计算机学院"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "队长学号不存在")
}

func TestResubmitRegistration_TrackNameMatchThenAttachmentRequired(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).AddRow(5, 1, 1, 0))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "participant_type", "min_team_member", "max_team_member", "track", "need_attachment", "reg_start_time", "reg_end_time"}).
			AddRow(1, 1, 1, 1, 5, `[{"trackName":"软件赛道","subTrack":[{"title":"Web开发"}]}]`, 2,
				time.Now().Add(-time.Hour), time.Now().Add(time.Hour)))

	body := resubmitBody()
	body["track"] = "软件赛道" // 命中赛道名分支
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须上传报名附件")
}

func TestResubmitRegistration_SubTrackFormatMatchThenAdvisorRequired(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).AddRow(5, 1, 1, 0))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "participant_type", "min_team_member", "max_team_member", "track", "need_advisor", "reg_start_time", "reg_end_time"}).
			AddRow(1, 1, 1, 1, 5, `[{"trackName":"软件赛道","subTrack":[{"title":"Web开发"}]}]`, 2,
				time.Now().Add(-time.Hour), time.Now().Add(time.Hour)))

	body := resubmitBody()
	body["track"] = "软件赛道 / Web开发" // 命中子赛道格式分支
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "必须填写指导老师")
}

func TestResubmitRegistration_AdvisorMarshalThenSaveError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "leader_id", "status"}).AddRow(5, 1, 1, 0))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "participant_type", "min_team_member", "max_team_member", "need_reg_audit", "reg_start_time", "reg_end_time"}).
			AddRow(1, 1, 1, 1, 5, 1, time.Now().Add(-time.Hour), time.Now().Add(time.Hour)))
	mock.ExpectBegin()
	// Save(&reg) 无预加载关联 → 直接主表更新
	mock.ExpectExec("UPDATE `registers` SET").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	body := resubmitBody()
	body["advisor_info"] = map[string]interface{}{"username": "T003", "name": "王指导"}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	ResubmitRegistration(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===================== validateRegistrationParticipation（直调） =====================

func participationReq() *ApplicationReq {
	return &ApplicationReq{
		CompID:            1,
		TeamName:          "参赛项目",
		ParticipationType: "individual",
		Leader:            MemberReq{Name: "张三", StuID: "S001"},
	}
}

func TestValidateParticipation_TeamNameEmpty(t *testing.T) {
	req := participationReq()
	req.TeamName = " "
	err := validateRegistrationParticipation(database.DB, req, models.CompDetail{}, 1)
	assert.ErrorContains(t, err, "参赛项目名称不能为空且不能超过255字")
}

func TestValidateParticipation_InvalidType(t *testing.T) {
	req := participationReq()
	req.ParticipationType = "group"
	err := validateRegistrationParticipation(database.DB, req, models.CompDetail{}, 1)
	assert.ErrorContains(t, err, "请选择个人参赛或团队参赛")
}

func TestValidateParticipation_IndividualRaceNotAlone(t *testing.T) {
	req := participationReq()
	req.Members = []MemberReq{{Name: "李四", StuID: "S002"}}
	err := validateRegistrationParticipation(database.DB, req, models.CompDetail{ParticipantType: 1}, 1)
	assert.ErrorContains(t, err, "个人赛只能由本人单独参赛")
}

func TestValidateParticipation_IndividualNotAllowed(t *testing.T) {
	req := participationReq()
	err := validateRegistrationParticipation(database.DB, req, models.CompDetail{ParticipantType: 2, MinTeamMember: 2, MaxTeamMember: 5}, 1)
	assert.ErrorContains(t, err, "该赛事不允许个人参赛")
}

func TestValidateParticipation_TeamCountOutOfRange(t *testing.T) {
	req := participationReq()
	req.ParticipationType = "team"
	err := validateRegistrationParticipation(database.DB, req, models.CompDetail{ParticipantType: 2, MinTeamMember: 3, MaxTeamMember: 5}, 1)
	assert.ErrorContains(t, err, "团队参赛人数须为3-5人")
}

func TestValidateParticipation_ApplicantLookupError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnError(fmt.Errorf("db error"))

	req := participationReq()
	err := validateRegistrationParticipation(database.DB, req, models.CompDetail{ParticipantType: 1}, 1)
	assert.ErrorContains(t, err, "无法获取申报人信息")
}

func TestValidateParticipation_IndividualNotSelf(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S999"))

	req := participationReq()
	err := validateRegistrationParticipation(database.DB, req, models.CompDetail{ParticipantType: 1}, 1)
	assert.ErrorContains(t, err, "个人参赛只能填写本人信息")
}

func TestValidateParticipation_MemberFieldsEmpty(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))

	req := participationReq()
	req.ParticipationType = "team"
	req.Members = []MemberReq{{Name: "", StuID: "S002"}}
	err := validateRegistrationParticipation(database.DB, req, models.CompDetail{ParticipantType: 2, MinTeamMember: 1, MaxTeamMember: 5}, 1)
	assert.ErrorContains(t, err, "请完整填写参赛成员的姓名和学号")
}

func TestValidateParticipation_MemberDuplicate(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S001"))

	req := participationReq()
	req.ParticipationType = "team"
	req.Members = []MemberReq{{Name: "张三", StuID: "S001"}}
	err := validateRegistrationParticipation(database.DB, req, models.CompDetail{ParticipantType: 2, MinTeamMember: 1, MaxTeamMember: 5}, 1)
	assert.ErrorContains(t, err, "参赛成员不能重复")
}

func TestValidateParticipation_ApplicantNotInMembers(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "S999"))

	req := participationReq()
	req.ParticipationType = "team"
	req.Leader = MemberReq{Name: "李四", StuID: "S002"}
	req.Members = []MemberReq{{Name: "王五", StuID: "S003"}}
	err := validateRegistrationParticipation(database.DB, req, models.CompDetail{ParticipantType: 2, MinTeamMember: 1, MaxTeamMember: 5}, 1)
	assert.ErrorContains(t, err, "申报人必须在参赛成员中")
}

var _ = gorm.ErrRecordNotFound
var _ = time.Now
