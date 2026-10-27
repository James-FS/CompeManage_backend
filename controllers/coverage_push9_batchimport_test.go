package controllers

import (
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// ===================== BatchImportCompetition =====================

// 缺少负责人 → 400
func TestBatchImportCompetition_MissingManager(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	req := struct {
		Items []CreateCompetitionReq `json:"items"`
	}{
		Items: []CreateCompetitionReq{
			{CompName: "赛事1", CompLevel: "校级", ManagerID: 0},
		},
	}
	_, w, c := buildCompPOSTJSON(req)
	BatchImportCompetition(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "缺少赛事负责人")
}

// 生成编号失败（不支持的级别）→ 500
func TestBatchImportCompetition_CodeGenerationFails(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectRollback()

	req := struct {
		Items []CreateCompetitionReq `json:"items"`
	}{
		Items: []CreateCompetitionReq{
			{CompName: "赛事1", CompLevel: "不支持的级别", ManagerID: 1},
		},
	}
	_, w, c := buildCompPOSTJSON(req)
	BatchImportCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// 负责人校验失败（无资格）→ 400
func TestBatchImportCompetition_InvalidManager(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	// validateCompetitionManager：候选存在但无角色 → "角色配置异常"
	mock.ExpectQuery("SELECT \\* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).AddRow(1, "T001", "staff"))
	mock.ExpectQuery("SELECT \\* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))
	mock.ExpectRollback()

	req := struct {
		Items []CreateCompetitionReq `json:"items"`
	}{
		Items: []CreateCompetitionReq{
			{CompName: "赛事1", CompLevel: "校级", ManagerID: 1},
		},
	}
	_, w, c := buildCompPOSTJSON(req)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	BatchImportCompetition(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "赛事负责人无效")
}

// 创建赛事失败 → 500
func TestBatchImportCompetition_CreateFails(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	// validateCompetitionManager：命中候选
	mock.ExpectQuery("SELECT \\* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).AddRow(1, "T001", "staff"))
	mock.ExpectQuery("SELECT \\* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 3))
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(3, "teacher", "老师"))
	// nextCompetitionCode：查最新编号
	mock.ExpectQuery("SELECT `comp_code` FROM `comp_directories`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec("INSERT INTO `comp_directories`").
		WillReturnError(assertErr("insert failed"))
	mock.ExpectRollback()

	req := struct {
		Items []CreateCompetitionReq `json:"items"`
	}{
		Items: []CreateCompetitionReq{
			{CompName: "赛事1", CompLevel: "校级", ManagerID: 1},
		},
	}
	_, w, c := buildCompPOSTJSON(req)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	BatchImportCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "服务器内部错误")
}

// 成功导入：学院解析 + 编号 + 创建 + 教师提升
func TestBatchImportCompetition_Success(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	// 学院解析
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(9, "计算机学院"))
	// 负责人候选校验（先于编号生成）
	mock.ExpectQuery("SELECT \\* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).AddRow(1, "T001", "staff"))
	mock.ExpectQuery("SELECT \\* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 3))
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(3, "teacher", "老师"))
	// 编号生成
	mock.ExpectQuery("SELECT `comp_code` FROM `comp_directories`").
		WillReturnError(gorm.ErrRecordNotFound)
	// 创建赛事
	mock.ExpectExec("INSERT INTO `comp_directories`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// promoteCompetitionManagerIfTeacher：当前已是负责人 → 不提升
	mock.ExpectQuery("SELECT \\* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 3))
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(3, "competition_manager", "赛事负责人"))
	mock.ExpectCommit()
	// finalizeRolePromotions → 审计/角色更新（空 promotions 时不应产生 SQL）

	req := struct {
		Items []CreateCompetitionReq `json:"items"`
	}{
		Items: []CreateCompetitionReq{
			{CompName: "赛事1", CompLevel: "校级", ManagerID: 1, College: "计算机学院"},
		},
	}
	_, w, c := buildCompPOSTJSON(req)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	BatchImportCompetition(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "批量导入成功")
}

// 同一学院只解析一次（缓存命中）；promote 仅在第一条后执行一次
func TestBatchImportCompetition_CollegeResolvedOnce(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	// 学院仅查询一次（第二次命中缓存）
	mock.ExpectQuery("SELECT .* FROM `colleges`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(9, "计算机学院"))
	// 第 1 条：校验 + 编号 + 创建
	mock.ExpectQuery("SELECT \\* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).AddRow(1, "T001", "staff"))
	mock.ExpectQuery("SELECT \\* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 3))
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(3, "teacher", "老师"))
	mock.ExpectQuery("SELECT `comp_code` FROM `comp_directories`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec("INSERT INTO `comp_directories`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// promote（仅第一次）：已是负责人 → 不提升
	mock.ExpectQuery("SELECT \\* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 3))
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(3, "competition_manager", "赛事负责人"))
	// 第 2 条：校验 + 编号 + 创建；promote 不再执行
	mock.ExpectQuery("SELECT \\* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).AddRow(1, "T001", "staff"))
	mock.ExpectQuery("SELECT \\* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 3))
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(3, "teacher", "老师"))
	mock.ExpectQuery("SELECT `comp_code` FROM `comp_directories`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec("INSERT INTO `comp_directories`").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	req := struct {
		Items []CreateCompetitionReq `json:"items"`
	}{
		Items: []CreateCompetitionReq{
			{CompName: "赛事1", CompLevel: "校级", ManagerID: 1, College: "计算机学院"},
			{CompName: "赛事2", CompLevel: "校级", ManagerID: 1, College: "计算机学院"},
		},
	}
	_, w, c := buildCompPOSTJSON(req)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	BatchImportCompetition(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"count\":2")
}

// 教师提升失败 → 回滚
func TestBatchImportCompetition_PromotionFails(t *testing.T) {
	mock := setupCompDBMock(t)
	defer mock.ExpectationsWereMet()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "identity_type"}).AddRow(1, "T001", "staff"))
	mock.ExpectQuery("SELECT \\* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 3))
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role_code", "role_name"}).AddRow(3, "teacher", "老师"))
	mock.ExpectQuery("SELECT `comp_code` FROM `comp_directories`").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec("INSERT INTO `comp_directories`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// promote 链失败：user_roles 查询后 roles(id) 报错
	mock.ExpectQuery("SELECT \\* FROM `user_roles`").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}).AddRow(1, 3))
	mock.ExpectQuery("SELECT \\* FROM `roles`").
		WillReturnError(assertErr("db error"))
	mock.ExpectRollback()

	req := struct {
		Items []CreateCompetitionReq `json:"items"`
	}{
		Items: []CreateCompetitionReq{
			{CompName: "赛事1", CompLevel: "校级", ManagerID: 1},
		},
	}
	_, w, c := buildCompPOSTJSON(req)
	c.Set("role_code", "school_admin")
	c.Set("user_id", uint(1))
	BatchImportCompetition(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
