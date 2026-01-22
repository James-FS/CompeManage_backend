package middleware

import (
	"CompeManage_backend/database"
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
		c.Set("username", claims.Username)
		c.Next()
	}
}

// RequirePermission
// 作用：拦截请求，检查当前用户是否拥有指定权限 code
func RequirePermission(permCode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.AbortWithStatusJSON(401, gin.H{"msg": "未登录"})
			return
		}

		// 2. 核心逻辑：去数据库查一下，这个 Role 到底有没有 permCode 这个权限？
		// (为了性能，这一步通常走 Redis 缓存，但直接查库也能用)
		var count int64
		err := database.DB.Table("user_roles").
			Joins("JOIN role_permissions ON role_permissions.role_id = user_roles.role_id").
			Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
			Where("user_roles.user_id= ? AND permissions.code = ?", userID, permCode).
			Count(&count)

		if err != nil {
			// 数据库查询出错
			c.AbortWithStatusJSON(500, gin.H{"code": 500, "msg": "权限验证服务异常"})
			return
		}
		if count > 0 {
			// 有权限，放行
			c.Next()
		} else {
			// 没权限，直接拦截
			c.AbortWithStatusJSON(403, gin.H{"msg": "权限不足，禁止访问"})
		}
	}
}
