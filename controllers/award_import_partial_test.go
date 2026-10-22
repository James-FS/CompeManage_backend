package controllers

import (
	"CompeManage_backend/models"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAwardPartialImportMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	college := models.College{Name: "部分导入测试学院", IsValid: true}
	require.NoError(t, db.Create(&college).Error)
	users := make([]models.User, 13)
	rows := make([]AwardImportRow, 13)
	for i := range users {
		users[i] = models.User{Username: fmt.Sprintf("P%04d", i), Realname: fmt.Sprintf("学生%d", i), College: college.Name, Major: "测试专业", IdentityType: "student"}
	}
	require.NoError(t, db.Create(&users).Error)
	for i, user := range users {
		rows[i] = AwardImportRow{StudentNumber: user.Username, Name: user.Realname, College: user.College, Major: user.Major, ProjectName: "部分成员先导入", Category: "省级", AwardName: "一等奖", AwardMonth: "2026-10"}
	}
	newComp := func(code string) models.CompDirectory {
		comp := models.CompDirectory{CompCode: code, CompName: code, CompLevel: "校级", ManagerID: users[0].ID}
		require.NoError(t, db.Create(&comp).Error)
		return comp
	}
	type responseData struct {
		Rows     []AwardImportRow `json:"rows"`
		Success  int              `json:"success_count"`
		Existing int              `json:"existing_count"`
		Failed   int              `json:"fail_count"`
		Total    int              `json:"student_count"`
	}
	invoke := func(compID, submitter uint, input []AwardImportRow) responseData {
		body, err := json.Marshal(map[string]interface{}{"rows": input})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", fmt.Sprintf("/api/award/import?comp_id=%d", compID), bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("role_code", "school_admin")
		c.Set("user_id", submitter)
		c.Set("managed_college_id", &college.ID)
		ImportAward(c)
		require.Equal(t, 200, w.Code, w.Body.String())
		var response struct {
			Data responseData `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		return response.Data
	}
	comp := newComp("PARTIAL13")
	badRows := append([]AwardImportRow(nil), rows...)
	for i := 3; i < len(badRows); i++ {
		badRows[i].Name = "不匹配的姓名"
	}
	result := invoke(comp.ID, users[0].ID, badRows)
	require.Equal(t, 13, result.Total)
	require.Equal(t, 3, result.Success)
	require.Equal(t, 10, result.Failed)
	require.Zero(t, result.Existing)
	for i, row := range result.Rows {
		if i < 3 {
			require.Equal(t, "imported", row.ImportStatus)
		} else {
			require.Equal(t, "failed", row.ImportStatus)
			require.Contains(t, row.Error, "学生姓名")
		}
	}
	// Re-import the original file under a different submitter: no duplicates or ownership changes.
	result = invoke(comp.ID, users[1].ID, badRows)
	require.Zero(t, result.Success)
	require.Equal(t, 3, result.Existing)
	require.Equal(t, 10, result.Failed)
	var member models.AwardMember
	require.NoError(t, db.Where("student_id = ?", users[0].ID).First(&member).Error)
	require.Equal(t, users[0].ID, *member.SubmittedBy)
	require.Equal(t, college.ID, *member.SubmittedCollegeID)
	// Corrected members join the same award; the three previously imported students are skipped.
	result = invoke(comp.ID, users[1].ID, rows)
	require.Equal(t, 10, result.Success)
	require.Equal(t, 3, result.Existing)
	require.Zero(t, result.Failed)
	var awards []models.Award
	require.NoError(t, db.Where("comp_id = ?", comp.ID).Preload("Members").Find(&awards).Error)
	require.Len(t, awards, 1)
	require.Len(t, awards[0].Members, 13)
	result = invoke(comp.ID, users[1].ID, rows)
	require.Zero(t, result.Success)
	require.Equal(t, 13, result.Existing)
	// Both members of a conflicting team are skipped, even if the conflicting row has a bad student.
	conflictComp := newComp("PARTIAL_CONFLICT")
	conflictRows := append([]AwardImportRow(nil), rows[:3]...)
	conflictRows[1].AwardName = "二等奖"
	conflictRows[1].Name = "错误姓名"
	conflictRows[2].ProjectName = "无冲突团队"
	result = invoke(conflictComp.ID, users[0].ID, conflictRows)
	require.Equal(t, 1, result.Success)
	require.Equal(t, 2, result.Failed)
	for _, row := range result.Rows[:2] {
		require.Contains(t, row.Error, "同一团队")
	}
	// Invalid shared metadata also blocks the whole team.
	conflictRows[1].AwardName = "一等奖"
	conflictRows[1].AwardMonth = "错误日期"
	result = invoke(newComp("PARTIAL_DATE").ID, users[0].ID, conflictRows)
	require.Equal(t, 1, result.Success)
	require.Equal(t, 2, result.Failed)
	// File duplicates in the same team are skipped instead of being inserted twice.
	result = invoke(newComp("PARTIAL_DUPLICATE").ID, users[0].ID, []AwardImportRow{rows[0], rows[0], rows[1]})
	require.Equal(t, 2, result.Success)
	require.Equal(t, 1, result.Existing)
	require.Zero(t, result.Failed)
	require.True(t, result.Rows[1].Duplicate)
	// A student's competing claims across projects fail together, independent of row order.
	crossTeam := rows[0]
	crossTeam.ProjectName = "另一团队"
	result = invoke(newComp("PARTIAL_CROSS").ID, users[0].ID, []AwardImportRow{rows[0], crossTeam, rows[1]})
	require.Equal(t, 1, result.Success)
	require.Equal(t, 2, result.Failed)
	// Historical team conflicts must not prevent unrelated teams from being imported.
	existingConflict := rows[0]
	existingConflict.AwardName = "二等奖"
	result = invoke(comp.ID, users[1].ID, []AwardImportRow{existingConflict, rows[1]})
	require.Zero(t, result.Success)
	require.Equal(t, 2, result.Failed)
}
