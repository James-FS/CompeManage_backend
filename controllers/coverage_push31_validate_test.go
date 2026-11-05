package controllers

import (
	"fmt"
	"strings"
	"testing"

	"CompeManage_backend/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// lookup 首查询失败 → 直接报错
func TestValidateAwardRows_LookupError(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectQuery("SELECT users.id, users.username, users.realname, users.college, users.major, requested.lookup_username").
		WillReturnError(fmt.Errorf("db error"))

	_, _, err := validateAwardRows(database.DB, 1, []AwardImportRow{importRow("S001", "项目A")}, false)
	assert.ErrorContains(t, err, "db error")
}

// 行级独有校验（备注超长）——团队级校验不含该条件，可达
func TestValidateAwardRows_RemarkTooLong(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	row := importRow("S001", "项目A")
	row.Remark = strings.Repeat("长", 2001)
	expectImportLookup(mock, userRow(11, "S001", "张三", "计算机学院", "软件工程"),
		sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{row}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "超出长度限制")
}

// 学号超长（50 字符限制）同理
func TestValidateAwardRows_StudentNumberTooLong(t *testing.T) {
	mock := setupRegDBMock(t)
	defer mock.ExpectationsWereMet()

	row := importRow("S001", "项目A")
	row.StudentNumber = strings.Repeat("1", 51)
	expectImportLookup(mock, userRow(11, strings.Repeat("1", 51), "张三", "计算机学院", "软件工程"),
		sqlmock.NewRows([]string{"id", "comp_id", "project_key", "project_name"}))

	result, bad, err := validateAwardRows(database.DB, 1, []AwardImportRow{row}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, bad)
	assert.Contains(t, result[0].Error, "超出长度限制")
}
