package utils

import (
	"CompeManage_backend/config"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// 定义加密的密钥
// 在正式环境，这个值应该从配置文件(config-dev.yaml)读取
func getJwtSecret() []byte {
	return []byte(config.GetString("jwt.secret"))
}

// MyClaims 自定义载荷
type MyClaims struct {
	UserID   uint   `json:"user_id"`   // 用户ID (对应数据库主键)
	Username string `json:"username"`  // 用户名/学号
	RoleCode string `json:"role_code"` // 关键：角色标识 (admin/teacher/student)
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT令牌
// 参数：用户ID, 用户名, 角色标识
func GenerateToken(userID uint, username string, roleCode string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)

	claims := &MyClaims{
		UserID:   userID,
		Username: username,
		RoleCode: roleCode,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime), // 过期时间
			IssuedAt:  jwt.NewNumericDate(time.Now()),     // 签发时间
			Issuer:    "compe_backend",                    // 签发人
		},
	}

	// 使用 HS256 算法进行签名
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// 使用动态密钥
	return token.SignedString(getJwtSecret())
}

// ParseToken 解析并验证 JWT 令牌
func ParseToken(tokenString string) (*MyClaims, error) {
	// 解析 Token
	token, err := jwt.ParseWithClaims(tokenString, &MyClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 校验签名算法是否一致
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		// 使用动态密钥
		return getJwtSecret(), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*MyClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
