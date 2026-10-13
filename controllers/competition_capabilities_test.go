package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func setCollegeManagerContext(c *gin.Context, codes ...string) {
	collegeID := uint(2)
	c.Set("user_id", uint(10))
	c.Set("role_code", "college_admin")
	c.Set("managed_college_id", &collegeID)
	c.Set("permission_codes", codes)
}

func TestCompetitionWriteBoundaries(t *testing.T) {
	college, other := uint(2), uint(3)
	scope := &UserAccessScope{UserID: 10, RoleCode: "college_admin", ManagedCollegeID: &college}
	for _, permission := range []string{"comp:update", "reg:config:edit", "summary:edit"} {
		for _, tc := range []struct {
			name    string
			manager uint
			college *uint
			codes   []string
			want    bool
		}{
			{"own", 10, &college, []string{permission + ":own"}, true},
			{"other manager", 11, &college, []string{permission + ":own"}, false},
			{"broad grant still cannot edit other manager", 11, &college, []string{permission}, false},
			{"cross college", 10, &other, []string{permission + ":own"}, false},
			{"unassigned college", 10, nil, []string{permission + ":own"}, false},
			{"revoked", 10, &college, nil, false},
		} {
			t.Run(permission+"/"+tc.name, func(t *testing.T) {
				require.Equal(t, tc.want, canWriteCompetition(scope, models.CompDirectory{ManagerID: tc.manager, CollegeID: tc.college}, tc.codes, permission))
			})
		}
	}
	comp := models.CompDirectory{ManagerID: 10, CollegeID: &college}
	require.True(t, canWriteCompetition(&UserAccessScope{RoleCode: "school_admin"}, comp, []string{"comp:update"}, "comp:update"))
	require.True(t, canWriteCompetition(&UserAccessScope{UserID: 10, RoleCode: "competition_manager"}, comp, []string{"comp:update"}, "comp:update"))
	require.False(t, canWriteCompetition(&UserAccessScope{UserID: 11, RoleCode: "competition_manager"}, comp, []string{"comp:update"}, "comp:update"))
}

func TestCollegeManagerCannotReassignOrMoveCollege(t *testing.T) {
	for _, tc := range []struct {
		name    string
		manager uint
		college string
	}{
		{"reassign", 11, "学院甲"}, {"move college", 10, "学院乙"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := setupCompDBMock(t)
			t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
			mock.ExpectQuery("SELECT .* FROM `comp_directories`").WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 10, 2))
			mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 10, 2))
			if tc.manager == 10 {
				mock.ExpectQuery("SELECT .* FROM `colleges`").WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(3, "学院乙"))
			}
			_, w, c := buildCompPOSTJSON(UpdateCompetitionReq{CompName: "赛事", CompLevel: "省级", ManagerID: tc.manager, College: tc.college})
			c.Params = []gin.Param{{Key: "id", Value: "1"}}
			setCollegeManagerContext(c, "comp:update:own")
			UpdateCompetition(c)
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		})
	}
}

func TestCollegeManagerSummaryOwnOnly(t *testing.T) {
	for _, tc := range []struct {
		name       string
		manager    uint
		permission bool
		archived   bool
		want       int
	}{
		{"own draft", 10, true, false, 200},
		{"other manager", 11, true, false, 403},
		{"revoked", 10, false, false, 403},
		{"archived", 10, true, true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := setupCompDBMock(t)
			t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
			mock.ExpectQuery("SELECT .* FROM `comp_directories`").WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id", "status"}).AddRow(1, tc.manager, 2, 2))
			if tc.manager == 10 && tc.permission {
				rows := sqlmock.NewRows([]string{"id", "status"})
				if tc.archived {
					rows.AddRow(1, 1)
				}
				mock.ExpectQuery("SELECT .* FROM `summaries`").WillReturnRows(rows)
				if !tc.archived {
					mock.ExpectBegin()
					mock.ExpectExec("INSERT INTO `summaries`").WillReturnResult(sqlmock.NewResult(1, 1))
					mock.ExpectCommit()
				}
			}
			_, w, c := buildCompPOSTJSON(SaveSummaryReq{SummaryContent: "总结", Status: 0})
			c.Params = []gin.Param{{Key: "id", Value: "1"}}
			setCollegeManagerContext(c)
			if tc.permission {
				c.Set("permission_codes", []string{"summary:edit:own"})
			}
			SaveSummary(c)
			require.Equal(t, tc.want, w.Code, w.Body.String())
		})
	}
}

func TestCollegeManagerCannotConfigureOtherManagerCompetition(t *testing.T) {
	mock := setupCompDBMock(t)
	t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 11, 2))
	start, end := time.Now(), time.Now().Add(time.Hour)
	_, w, c := buildCompPOSTJSON(ConfigReq{CompID: 1, RegStartTime: &start, RegEndTime: &end})
	setCollegeManagerContext(c, "reg:config:edit:own")
	SaveRegConfig(c)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestCollegeManagerCandidateCollegeValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		college *uint
		valid   bool
	}{
		{"same college", uintPointer(2), true}, {"other college", uintPointer(3), false}, {"no college", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := setupCompDBMock(t)
			t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
			mock.ExpectQuery("SELECT .* FROM `users`.*college_admin").WillReturnRows(sqlmock.NewRows([]string{"id", "managed_college_id"}).AddRow(10, 2))
			mock.ExpectQuery("SELECT .* FROM `user_roles`").WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(10, 2))
			mock.ExpectQuery("SELECT .* FROM `roles`").WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "college_admin"))
			_, _, c := buildCompGET("/")
			setCollegeManagerContext(c)
			err := validateCompetitionManager(c, database.DB, 10, tc.college)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errInvalidCompetitionManager)
			}
		})
	}
}

func uintPointer(value uint) *uint { return &value }

func TestCompetitionCapabilitiesFollowManagerReassignment(t *testing.T) {
	_, _, c := buildCompGET("/")
	setCollegeManagerContext(c, "comp:update:own", "reg:config:edit:own", "summary:edit:own")
	comp := models.CompDirectory{ManagerID: 10, CollegeID: uintPointer(2), Status: 2}
	caps, ok := competitionCapabilities(c, comp)
	require.True(t, ok)
	require.True(t, caps["can_edit_competition"])
	require.True(t, caps["can_edit_registration"])
	require.True(t, caps["can_edit_summary"])
	require.False(t, caps["can_reassign_manager"])
	comp.ManagerID = 11
	caps, ok = competitionCapabilities(c, comp)
	require.True(t, ok)
	require.False(t, caps["can_edit_competition"])
	require.False(t, caps["can_edit_registration"])
	require.False(t, caps["can_edit_summary"])
}

func TestManagementCompetitionListIgnoresStaleCachedOwnership(t *testing.T) {
	mock := setupCompDBMock(t)
	t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
	mr := miniredis.RunT(t)
	previousRedis := middleware.RedisClient
	middleware.RedisClient = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = middleware.RedisClient.Close(); middleware.RedisClient = previousRedis })
	_, w, c := buildCompGET("/api/comp/list?page=1&page_size=10&is_my=true")
	setCollegeManagerContext(c, "comp:update:own")
	c.Set("permission_version", uint64(1))
	req := CompListReq{Page: 1, PageSize: 10, IsMy: true}
	encoded, err := json.Marshal(req)
	require.NoError(t, err)
	key := fmt.Sprintf("cache:comp_list:duty_v3:%s:%x", competitionCacheIdentity(c), md5.Sum(encoded))
	require.NoError(t, middleware.RedisClient.Set(context.Background(), key, `{"list":[{"id":99,"comp_name":"stale owner"}],"total":1}`, time.Minute).Err())
	mock.ExpectQuery("SELECT count.*FROM `comp_directories`.*college_id = ").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .*FROM `comp_directories`.*college_id = ").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	GetCompetitionList(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "stale owner")
}

func TestCollegeManagerCanSaveOwnCompetition(t *testing.T) {
	mock := setupCompDBMock(t)
	t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
	previousRedis := middleware.RedisClient
	middleware.RedisClient = nil
	t.Cleanup(func() { middleware.RedisClient = previousRedis })
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 10, 2))
	mock.ExpectQuery("SELECT `id`,`manager_id`,`college_id` FROM `comp_directories`").WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 10, 2))
	mock.ExpectQuery("SELECT .* FROM `colleges`").WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(2, "学院甲"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `users`.*college_admin").WillReturnRows(sqlmock.NewRows([]string{"id", "managed_college_id"}).AddRow(10, 2))
	mock.ExpectQuery("SELECT .* FROM `user_roles`").WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(10, 2))
	mock.ExpectQuery("SELECT .* FROM `roles`").WillReturnRows(sqlmock.NewRows([]string{"id", "role_code"}).AddRow(2, "college_admin"))
	mock.ExpectExec("UPDATE `comp_directories`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	_, w, c := buildCompPOSTJSON(UpdateCompetitionReq{CompName: "更新的赛事", CompLevel: "省级", ManagerID: 10, College: "学院甲", Year: "2026"})
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	setCollegeManagerContext(c, "comp:update:own")
	UpdateCompetition(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	// No user-role UPDATE is expected: editing preserves the administrator role.
}

func TestCollegeManagerCanConfigureOwnCompetition(t *testing.T) {
	mock := setupCompDBMock(t)
	t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
	previousRedis := middleware.RedisClient
	middleware.RedisClient = nil
	t.Cleanup(func() { middleware.RedisClient = previousRedis })
	mock.ExpectQuery("SELECT .* FROM `comp_directories`").WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 10, 2))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	for _, sql := range []string{"INSERT INTO `comp_details`", "UPDATE `comp_details`", "UPDATE `comp_directories`"} {
		mock.ExpectBegin()
		mock.ExpectExec(sql).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
	}
	start, end := time.Now().Add(time.Hour), time.Now().Add(2*time.Hour)
	_, w, c := buildCompPOSTJSON(ConfigReq{CompID: 1, ParticipantType: 1, RegStartTime: &start, RegEndTime: &end})
	setCollegeManagerContext(c, "reg:config:edit:own")
	SaveRegConfig(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
