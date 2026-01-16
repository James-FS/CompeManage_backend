package utils

import (
	"CompeManage_backend/config"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 自定义claims
type Claims struct {
	UserID string `json:"user_id"`
	OpenID string `json:"open_id"`
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT令牌
func GenerateToken(userID string, openID string) (string, error) {
	//设置过期时间为7天
	expirationTime := time.Now().Add(7 * 24 * time.Hour)

	claims := &Claims{
		UserID: userID,
		OpenID: openID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.GetString("jwt.secret")))
}

// ParseToken 验证JWT令牌
func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.GetString("jwt.secret")), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrTokenInvalidClaims
}
