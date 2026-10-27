package controllers

import (
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// 多行结果覆盖排序三分支（均分高者前 / 同分早交者前 / 同分同时间小 reg_id 前）+ 分差异常
func TestGetReviewResultList_SortBranches(t *testing.T) {
	mock := setupReviewDBMock(t)
	defer mock.ExpectationsWereMet()

	t2 := time.Date(2026, 1, 2, 10, 0, 0, 0, time.Local)
	t1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.Local)

	mock.ExpectQuery("SELECT .* FROM `comp_directories`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_name"}).AddRow(1, "测试赛事"))
	mock.ExpectQuery("SELECT .* FROM `comp_details`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comp_id", "review_end_time"}).
			AddRow(1, 1, time.Time{}))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(8))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(8))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `awards`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// 8 条记录 → 4 个作品，构造排序分支
	mock.ExpectQuery("SELECT \\* FROM `review_records`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "reg_id", "expert_id", "comp_id", "score", "comment", "status"}).
			AddRow(1, 1, 105, 10, 1, 75.0, "", 1).
			AddRow(2, 2, 105, 11, 1, 75.0, "", 1).
			AddRow(3, 3, 101, 10, 1, 75.0, "", 1).
			AddRow(4, 4, 101, 11, 1, 75.0, "", 1).
			AddRow(5, 5, 102, 10, 1, 80.0, "", 1).
			AddRow(6, 6, 102, 11, 1, 80.0, "", 1).
			AddRow(7, 7, 103, 10, 1, 75.0, "", 1).
			AddRow(8, 8, 103, 11, 1, 75.0, "", 1))
	// 专家姓名（10/11）
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(10, "张教授"))
	mock.ExpectQuery("SELECT .* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "realname"}).AddRow(11, "李教授"))
	// 报名预载：105/101 同时间，103 早交
	mock.ExpectQuery("SELECT .* FROM `registers`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_name", "leader_id", "create_time", "submit_time"}).
			AddRow(105, "队五", 1, t2, t2).
			AddRow(101, "队一", 1, t2, t2).
			AddRow(102, "队二", 1, t2, t2).
			AddRow(103, "队三", 1, t1, t1))
	mock.ExpectQuery("SELECT .* FROM `reg_members`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "reg_id", "name", "is_leader"}).
			AddRow(1, 105, "王同学", true).
			AddRow(2, 101, "李同学", true).
			AddRow(3, 102, "赵同学", true).
			AddRow(4, 103, "钱同学", true))

	_, w, c := buildGET("/api/review/result/list?comp_id=1")
	GetReviewResultList(c)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "队二") // 均分 80 排最前
	assert.Contains(t, body, "\"rank\":1")
}
