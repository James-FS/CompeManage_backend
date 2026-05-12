package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	viper.Set("jwt.secret", "test-jwt-secret-key")
}

func TestGenerateToken_Success(t *testing.T) {
	token, err := GenerateToken(1, "testuser", "student")

	assert.NoError(t, err)
	assert.NotEmpty(t, token)
	parts := strings.Split(token, ".")
	assert.Equal(t, 3, len(parts), "JWT 应该包含 header.payload.signature 三段")
}

func TestGenerateToken_ContainsCorrectClaims(t *testing.T) {
	token, err := GenerateToken(42, "stu001", "teacher")

	assert.NoError(t, err)

	claims, err := ParseToken(token)
	assert.NoError(t, err)
	assert.Equal(t, uint(42), claims.UserID)
	assert.Equal(t, "stu001", claims.Username)
	assert.Equal(t, "teacher", claims.RoleCode)
	assert.Equal(t, "compe_backend", claims.Issuer)
}

func TestGenerateToken_DifferentUsers(t *testing.T) {
	token1, err1 := GenerateToken(1, "user1", "student")
	token2, err2 := GenerateToken(2, "user2", "admin")

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEqual(t, token1, token2, "不同用户应该生成不同的 Token")
}

func TestParseToken_Success(t *testing.T) {
	token, err := GenerateToken(100, "testuser", "admin")
	assert.NoError(t, err)

	claims, err := ParseToken(token)

	assert.NoError(t, err)
	assert.NotNil(t, claims)
	assert.Equal(t, uint(100), claims.UserID)
	assert.Equal(t, "testuser", claims.Username)
	assert.Equal(t, "admin", claims.RoleCode)
}

func TestParseToken_Expired(t *testing.T) {
	expirationTime := time.Now().Add(-1 * time.Hour)
	claims := &MyClaims{
		UserID:   1,
		Username: "expired_user",
		RoleCode: "student",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			Issuer:    "compe_backend",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	expiredToken, err := token.SignedString([]byte("test-jwt-secret-key"))
	assert.NoError(t, err)

	_, err = ParseToken(expiredToken)
	assert.Error(t, err, "过期 Token 解析应该失败")
}

func TestParseToken_InvalidString(t *testing.T) {
	_, err := ParseToken("not.a.valid.token")

	assert.Error(t, err, "非法字符串解析应该失败")
}

func TestParseToken_EmptyString(t *testing.T) {
	_, err := ParseToken("")

	assert.Error(t, err, "空字符串解析应该失败")
}

func TestParseToken_WrongSecret(t *testing.T) {
	viper.Set("jwt.secret", "secret-a")
	token, err := GenerateToken(1, "user", "student")
	assert.NoError(t, err)

	viper.Set("jwt.secret", "secret-b")
	_, err = ParseToken(token)

	assert.Error(t, err, "密钥不匹配时解析应该失败")
}

func TestParseToken_TamperedPayload(t *testing.T) {
	viper.Set("jwt.secret", "test-jwt-secret-key")
	token, err := GenerateToken(1, "user", "student")
	assert.NoError(t, err)

	parts := strings.Split(token, ".")
	assert.Equal(t, 3, len(parts))

	// 修改 payload 部分（中间段），破坏签名有效性
	tamperedToken := parts[0] + "." + parts[1] + "tampered" + "." + parts[2]

	_, err = ParseToken(tamperedToken)
	assert.Error(t, err, "篡改后的 Token 解析应该失败")
}

func TestParseToken_UnexpectedSigningMethod(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	claims := &MyClaims{
		UserID:   1,
		Username: "user",
		RoleCode: "student",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "compe_backend",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	rs256Token, err := token.SignedString(privateKey)
	require.NoError(t, err)

	_, err = ParseToken(rs256Token)

	assert.Error(t, err, "非 HMAC 签名算法应该被拒绝")
	assert.Contains(t, err.Error(), "unexpected signing method")
}
