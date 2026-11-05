package controllers

import (
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// 回归：请求缺 need_review/评审时间时，评审时间必须落 1970 哨兵而非 time.Time 零值——
// 零值会被写成 '0000-00-00'，MySQL 严格模式拒绝（Error 1292），保存配置直接 500。
func TestSaveRegConfig_NoReviewFieldsUsesEpochSentinel(t *testing.T) {
	mock := setupUnorderedDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT \\* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "college_id"}).AddRow(1, 1, 1))
	mock.ExpectQuery("SELECT \\* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // 不存在 → 走创建分支
	// 创建分支：评审时间未提供也必须写入哨兵值而非零值
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `comp_details`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_details` SET `need_reg_audit`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comp_directories` SET `status`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// 请求体不带 need_review / review_start_time / review_end_time
	body := ConfigReq{
		CompID:       1,
		RegStartTime: timePtr(time.Now().Add(time.Hour)),
		RegEndTime:   timePtr(time.Now().Add(2 * time.Hour)),
	}
	_, w, c := buildAuthPOSTJSON(body)
	c.Set("user_id", uint(1))
	c.Set("role_code", "school_admin")
	c.Set("permission_codes", []string{"reg:config:edit"})
	SaveRegConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
}
