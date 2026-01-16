package middleware

import (
	"CompeManage_backend/utils"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthRequired 登录认证中间件
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 读取Authorization头
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			utils.Unauthorized(c, "请先登录")
			c.Abort()
			return
		}

		// 解析Bearer Token
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenStr == authHeader {
			utils.Unauthorized(c, "Token格式错误")
			c.Abort()
			return
		}

		// 验证Token
		claims, err := utils.ParseToken(tokenStr)
		if err != nil {
			utils.Unauthorized(c, "Token无效或已过期")
			c.Abort()
			return
		}

		// 将用户ID存入上下文
		c.Set("user_id", claims.UserID)
		c.Next()
	}
}
