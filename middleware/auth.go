package middleware

import (
	"CompeManage_backend/database"
	"CompeManage_backend/utils"
	"fmt"
	"strings"
	"time"

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
// 使用 Redis 缓存优化，避免每次请求都查 MySQL
func RequirePermission(permCode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.AbortWithStatusJSON(401, gin.H{"msg": "未登录"})
			return
		}

		// 1. 先查 Redis 缓存
		cacheKey := fmt.Sprintf("perm:%d:%s", userID, permCode)
		ctx := c.Request.Context()
		rdb := GetRedisClient()

		// 缓存值同时表达允许和拒绝，不能只根据键是否存在放行。
		cached, err := rdb.Get(ctx, cacheKey).Result()
		if err == nil {
			if cached == "1" {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(403, gin.H{"msg": "权限不足，禁止访问"})
			return
		}

		// 2. 缓存未命中，查 MySQL
		var count int64
		result := database.DB.Table("user_roles").
			Joins("JOIN role_permissions ON role_permissions.role_id = user_roles.role_id").
			Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
			Where("user_roles.user_id = ? AND permissions.code = ?", userID, permCode).
			Count(&count)

		if result.Error != nil {
			// 数据库查询出错
			c.AbortWithStatusJSON(500, gin.H{"code": 500, "msg": "权限验证服务异常"})
			return
		}

		// 3. 结果写入 Redis (过期时间 5 分钟)
		if count > 0 {
			// 有权限，缓存结果
			rdb.Set(ctx, cacheKey, "1", 5*time.Minute)
			c.Next()
		} else {
			// 无权限也缓存，避免缓存穿透攻击
			rdb.Set(ctx, cacheKey, "0", 5*time.Minute)
			c.AbortWithStatusJSON(403, gin.H{"msg": "权限不足，禁止访问"})
		}
	}
}
