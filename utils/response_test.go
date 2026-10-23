package utils

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"CompeManage_backend/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func respContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)
	return c, w
}

func decodeResp(t *testing.T, w *httptest.ResponseRecorder) Response {
	t.Helper()
	var r Response
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
	return r
}

func TestNewResponse_AllLogBranches(t *testing.T) {
	// 三个日志分支：>=500 Error、>=400 Warn、其余 Info
	c, _ := respContext(t)
	r := newResponse(c, InternalServerCode, "服务器错误", nil)
	assert.Equal(t, InternalServerCode, r.Code)
	assert.NotZero(t, r.Timestamp)

	r = newResponse(c, BadRequestCode, "参数错误", nil)
	assert.Equal(t, BadRequestCode, r.Code)

	r = newResponse(c, SuccessCode, "success", gin.H{"a": 1})
	assert.Equal(t, SuccessCode, r.Code)
	assert.NotNil(t, r.Data)
}

func TestSuccessFamily(t *testing.T) {
	c, w := respContext(t)
	Success(c, gin.H{"k": 1})
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, 200, decodeResp(t, w).Code)

	c, w = respContext(t)
	SuccessWithMessage(c, "导入成功", nil)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "导入成功", decodeResp(t, w).Message)

	c, w = respContext(t)
	SuccessCreated(c, nil)
	assert.Equal(t, 201, w.Code)
	assert.Equal(t, "created", decodeResp(t, w).Message)

	c, w = respContext(t)
	SuccessAccepted(c, "后台处理中", nil)
	assert.Equal(t, 202, w.Code)
	assert.Equal(t, "后台处理中", decodeResp(t, w).Message)
}

func TestErrorFamily(t *testing.T) {
	c, w := respContext(t)
	BadRequest(c, "参数错误")
	assert.Equal(t, 400, w.Code)
	assert.Equal(t, BadRequestCode, decodeResp(t, w).Code)

	c, w = respContext(t)
	Unauthorized(c, "未登录")
	assert.Equal(t, 401, w.Code)
	assert.Equal(t, UnauthorizedCode, decodeResp(t, w).Code)

	c, w = respContext(t)
	Forbidden(c, "无权限")
	assert.Equal(t, 403, w.Code)
	assert.Equal(t, ForbiddenCode, decodeResp(t, w).Code)

	c, w = respContext(t)
	NotFound(c, "不存在")
	assert.Equal(t, 404, w.Code)
	assert.Equal(t, NotFoundCode, decodeResp(t, w).Code)

	c, w = respContext(t)
	Error(c, 418, 999, "teapot")
	assert.Equal(t, 418, w.Code)
	assert.Equal(t, 999, decodeResp(t, w).Code)
}

// 生产环境隐藏内部错误详情；开发环境透出
func TestInternalServerError_HidesDetailInProd(t *testing.T) {
	_ = config.GetString("app.env") // 保证 config 依赖可达
	c, w := respContext(t)
	InternalServerError(c, "数据库炸了", errors.New("conn refused"))
	assert.Equal(t, 500, w.Code)
	body := decodeResp(t, w)
	assert.Equal(t, InternalServerCode, body.Code)
	assert.NotContains(t, body.Message, "conn refused")
}

func TestInternalServerError_NilErr(t *testing.T) {
	c, w := respContext(t)
	InternalServerError(c, "内部错误", nil)
	assert.Equal(t, 500, w.Code)
}
