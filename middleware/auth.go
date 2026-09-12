package middleware

import (
	"CompeManage_backend/database"
	"CompeManage_backend/utils"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// PermissionCacheTTL 是权限允许/拒绝判断在 Redis 中的最长缓存时间。
const PermissionCacheTTL = 5 * time.Minute

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

		// 从数据库加载实时角色和管理学院，后续业务范围判断不依赖 JWT 中的旧角色。
		var accessRows []struct {
			RoleCode         string
			ManagedCollegeID *uint
		}
		if err := database.DB.WithContext(c.Request.Context()).Table("users").
			Select("roles.role_code, users.managed_college_id").
			Joins("JOIN user_roles ON user_roles.user_id = users.id").
			Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.delete_time IS NULL").
			Where("users.id = ? AND users.delete_time IS NULL", claims.UserID).
			Limit(2).Find(&accessRows).Error; err != nil {
			utils.InternalServerError(c, "加载用户角色失败", err)
			c.Abort()
			return
		}
		if len(accessRows) != 1 {
			utils.Forbidden(c, "用户角色未配置或存在异常，请联系管理员")
			c.Abort()
			return
		}
		c.Set("role_code", accessRows[0].RoleCode)
		c.Set("managed_college_id", accessRows[0].ManagedCollegeID)
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
			rdb.Set(ctx, cacheKey, "1", PermissionCacheTTL)
			c.Next()
		} else {
			// 无权限也缓存，避免缓存穿透攻击
			rdb.Set(ctx, cacheKey, "0", PermissionCacheTTL)
			c.AbortWithStatusJSON(403, gin.H{"msg": "权限不足，禁止访问"})
		}
	}
}

// RequireRole 根据数据库中的当前角色进行校验，不信任 JWT 中可能过期的 RoleCode。
// 该中间件用于用户管理等必须硬性限制到特定角色的高风险接口。
func RequireRole(roleCode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDVal, exists := c.Get("user_id")
		if !exists {
			utils.Unauthorized(c, "未登录")
			c.Abort()
			return
		}
		userID, ok := userIDVal.(uint)
		if !ok || userID == 0 {
			utils.Unauthorized(c, "登录信息无效")
			c.Abort()
			return
		}

		var count int64
		if err := database.DB.WithContext(c.Request.Context()).Table("user_roles").
			Joins("JOIN roles ON roles.id = user_roles.role_id").
			Where("user_roles.user_id = ? AND roles.role_code = ?", userID, roleCode).
			Where("roles.delete_time IS NULL").
			Count(&count).Error; err != nil {
			utils.InternalServerError(c, "角色校验失败", err)
			c.Abort()
			return
		}
		if count != 1 {
			utils.Forbidden(c, "当前角色无权访问该功能")
			c.Abort()
			return
		}

		c.Set("role_code", roleCode)
		c.Next()
	}
}
