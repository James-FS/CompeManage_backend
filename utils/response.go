package utils

import (
	"CompeManage_backend/config"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// 定义业务错误码（与HTTP状态码分离）
const (
	SuccessCode        = 200  // 成功（行业惯例用0表示）
	BadRequestCode     = 400  // 参数错误
	UnauthorizedCode   = 401  // 未授权
	ForbiddenCode      = 403  // 禁止访问
	NotFoundCode       = 404  // 资源不存在
	InternalServerCode = 500  // 服务器错误
	TokenExpiredCode   = 1001 // 细分业务码：Token过期
	PhoneInvalidCode   = 1002 // 细分业务码：手机号无效
)

// Response 统一响应结构体
type Response struct {
	Code      int         `json:"code"`           // 业务错误码
	Message   string      `json:"message"`        // 提示信息
	Data      interface{} `json:"data,omitempty"` // 响应数据
	Timestamp int64       `json:"timestamp"`      // 响应时间戳
}

// 初始化基础响应（自动填充时间戳）
func newResponse(c *gin.Context, code int, message string, data interface{}) Response {
	// 1. 记录日志逻辑
	if code >= 400 {
		// 错误级别日志
		slog.Warn("接口响应错误",
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
			"biz_code", code,
			"msg", message,
			"ip", c.ClientIP(),
		)
	} else if code >= 500 {
		slog.Warn("接口响应错误",
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
			"biz_code", code,
			"msg", message,
			"ip", c.ClientIP(),
		)
	} else {
		// 成功级别日志 (可选，如果不希望日志太多可以删掉这部分)
		slog.Info("接口响应成功",
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
			"biz_code", code,
		)
	}

	// 返回结构体供 c.JSON 使用
	return Response{
		Code:      code,
		Message:   message,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}
}

// Success 成功响应（默认200 OK）
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, newResponse(c, SuccessCode, "success", data))
}

// SuccessWithMessage 成功响应带自定义消息
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, newResponse(c, SuccessCode, message, data))
}

// SuccessCreated 成功创建资源（201 Created）
func SuccessCreated(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, newResponse(c, SuccessCode, "created", data))
}

// 内部通用错误响应
func errorResponse(c *gin.Context, httpStatus int, code int, message string) {
	resp := newResponse(c, code, message, nil)
	c.JSON(httpStatus, resp)
}

// BadRequest 参数错误（400 Bad Request）
func BadRequest(c *gin.Context, message string) {
	errorResponse(c, http.StatusBadRequest, BadRequestCode, message)
}

// Unauthorized 未授权（401 Unauthorized）
func Unauthorized(c *gin.Context, message string) {
	errorResponse(c, http.StatusUnauthorized, UnauthorizedCode, message)
}

// Forbidden 禁止访问（403 Forbidden）
func Forbidden(c *gin.Context, message string) {
	errorResponse(c, http.StatusForbidden, ForbiddenCode, message)
}

// NotFound 资源不存在（404 Not Found）
func NotFound(c *gin.Context, message string) {
	errorResponse(c, http.StatusNotFound, NotFoundCode, message)
}

// InternalServerError 服务器错误（500 Internal Server Error）
// 支持传入原始错误，根据环境决定是否展示详情
func InternalServerError(c *gin.Context, message string, err error) {
	if err != nil {
		slog.Error("服务器内部错误",
			"path", c.Request.URL.Path,
			"msg", message,
			"error", err.Error(),
		)
	}
	var showMsg string
	if config.GetString("app.env") == "development" { // 开发环境显示详细错误
		showMsg = message + ": " + err.Error()
	} else { // 生产环境隐藏敏感信息
		showMsg = "服务器内部错误，请稍后重试"
	}
	errorResponse(c, http.StatusInternalServerError, InternalServerCode, showMsg)
}

// Error 通用错误响应（供高级使用）
func Error(c *gin.Context, httpStatus int, code int, message string) {
	errorResponse(c, httpStatus, code, message)
}
