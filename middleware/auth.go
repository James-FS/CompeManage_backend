package middleware

import (
	"CompeManage_backend/database"
	"CompeManage_backend/utils"
	"context"
	"encoding/json"
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
			RoleID            uint
			PermissionVersion uint64
			RoleCode          string
			ManagedCollegeID  *uint
		}
		if err := database.DB.WithContext(c.Request.Context()).Table("users").
			Select("roles.id AS role_id, roles.permission_version, roles.role_code, users.managed_college_id").
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
		c.Set("role_id", accessRows[0].RoleID)
		c.Set("permission_version", accessRows[0].PermissionVersion)
		c.Set("managed_college_id", accessRows[0].ManagedCollegeID)
		c.Next()
	}
}

// RequirePermission 按当前数据库角色和权限版本校验，不读取旧的用户权限缓存。
func RequirePermission(permCode string) gin.HandlerFunc {
	return RequireAnyPermission(permCode)
}

// RequireAnyPermission checks endpoint permissions; handlers still enforce resource scope.
func RequireAnyPermission(permCodes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get("user_id"); !exists {
			c.AbortWithStatusJSON(401, gin.H{"msg": "未登录"})
			return
		}
		roleID, ok := c.Get("role_id")
		id, validID := roleID.(uint)
		versionValue, versionOK := c.Get("permission_version")
		version, validVersion := versionValue.(uint64)
		if !ok || !validID || id == 0 || !versionOK || !validVersion {
			c.AbortWithStatusJSON(403, gin.H{"msg": "角色信息无效，禁止访问"})
			return
		}

		cacheKey := fmt.Sprintf("perm:v2:role:%d:version:%d", id, version)
		ctx := c.Request.Context()
		rdb := GetRedisClient()
		var codes []string
		cacheHit := false
		if rdb != nil {
			cacheCtx, cancel := context.WithTimeout(ctx, permissionCacheTimeout)
			cached, err := rdb.WithTimeout(permissionCacheTimeout).Get(cacheCtx, cacheKey).Result()
			cancel()
			cacheHit = err == nil && json.Unmarshal([]byte(cached), &codes) == nil
		}
		if !cacheHit {
			// 将版本约束放进同一个查询，避免把更新后的权限写入旧版本缓存。
			codes = make([]string, 0)
			if err := database.DB.WithContext(ctx).Table("permissions").
				Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
				Joins("JOIN roles ON roles.id = role_permissions.role_id").
				Where("roles.id = ? AND roles.permission_version = ? AND roles.delete_time IS NULL", id, version).
				Pluck("permissions.code", &codes).Error; err != nil {
				c.AbortWithStatusJSON(500, gin.H{"code": 500, "msg": "权限验证服务异常"})
				return
			}
			if rdb != nil {
				payload, _ := json.Marshal(codes)
				cacheCtx, cancel := context.WithTimeout(ctx, permissionCacheTimeout)
				_ = rdb.WithTimeout(permissionCacheTimeout).Set(cacheCtx, cacheKey, payload, PermissionCacheTTL).Err()
				cancel()
			}
		}
		c.Set("permission_codes", codes)
		for _, code := range codes {
			for _, permCode := range permCodes {
				if code == permCode {
					c.Next()
					return
				}
			}
		}
		c.AbortWithStatusJSON(403, gin.H{"msg": "权限不足，禁止访问"})
	}
}

// UserPermissionCodes loads permissions once per request, including read endpoints.
func UserPermissionCodes(c *gin.Context) ([]string, error) {
	if value, ok := c.Get("permission_codes"); ok {
		return value.([]string), nil
	}
	roleID, _ := c.Get("role_id")
	version, _ := c.Get("permission_version")
	var codes []string
	err := database.DB.WithContext(c.Request.Context()).Table("permissions").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Joins("JOIN roles ON roles.id = role_permissions.role_id").
		Where("roles.id = ? AND roles.permission_version = ? AND roles.delete_time IS NULL", roleID, version).
		Pluck("permissions.code", &codes).Error
	if err == nil {
		c.Set("permission_codes", codes)
	}
	return codes, err
}

// Redis 是可选加速层，避免故障时拖慢每次权限校验。
const permissionCacheTimeout = 150 * time.Millisecond

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
