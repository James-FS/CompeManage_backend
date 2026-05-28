package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const casHTTPTimeout = 15 * time.Second

var casHTTPClient = &http.Client{Timeout: casHTTPTimeout}

// CasLogin 生成CAS授权URL并重定向浏览器到CAS登录页
func CasLogin(c *gin.Context) {
	cfg := config.AppConfig.Cas
	authURL := fmt.Sprintf("%s/oauth2.0/authorize?client_id=%s&redirect_uri=%s&response_type=code",
		cfg.ServerURL,
		url.QueryEscape(cfg.ClientID),
		url.QueryEscape(cfg.RedirectURI),
	)
	c.Redirect(http.StatusFound, authURL)
}

// CasCallback 处理CAS授权回调
func CasCallback(c *gin.Context) {
	cfg := config.AppConfig.Cas
	ctx := c.Request.Context()
	code := c.Query("code")

	// 如果CAS返回了错误（用户取消等），重定向到前端登录页
	if code == "" {
		casErr := c.Query("error")
		redirectWithError(c, cfg.FrontendURL, fmt.Errorf("CAS授权被拒绝: %s", casErr), "授权失败")
		return
	}

	// Step 1: 用 code 换 access_token
	accessToken, err := exchangeCodeForToken(cfg, code)
	if err != nil {
		redirectWithError(c, cfg.FrontendURL, err, "获取access_token失败")
		return
	}

	// Step 2: 用 access_token 获取用户信息
	casUser, err := fetchCasUserProfile(cfg, accessToken)
	if err != nil {
		redirectWithError(c, cfg.FrontendURL, err, "获取用户信息失败")
		return
	}

	// Step 3: 在本地数据库中查找或创建用户
	user, err := findOrCreateUser(ctx, casUser)
	if err != nil {
		redirectWithError(c, cfg.FrontendURL, err, "处理用户信息失败")
		return
	}

	// Step 4: 获取角色
	roleCode := "student"
	if len(user.Roles) == 0 {
		var defaultRole models.Role
		database.DB.WithContext(ctx).Where("role_code = ?", "student").First(&defaultRole)
		if defaultRole.ID > 0 {
			database.DB.WithContext(ctx).Model(&user).Association("Roles").Append(&defaultRole)
			user.Roles = append(user.Roles, &defaultRole)
		}
	}
	if len(user.Roles) > 0 {
		roleCode = user.Roles[0].RoleCode
	}

	// Step 5: 签发JWT
	token, err := utils.GenerateToken(user.ID, user.Username, roleCode)
	if err != nil {
		redirectWithError(c, cfg.FrontendURL, err, "Token生成失败")
		return
	}

	// Step 6: 重定向到前端登录页，通过hash传递token
	frontendRedirect := fmt.Sprintf("%s/#/login?token=%s&username=%s&realname=%s&role=%s&user_id=%d",
		cfg.FrontendURL,
		url.QueryEscape(token),
		url.QueryEscape(user.Username),
		url.QueryEscape(user.Realname),
		url.QueryEscape(roleCode),
		user.ID,
	)
	c.Redirect(http.StatusFound, frontendRedirect)
}

// redirectWithError 将错误信息通过302重定向到前端登录页
// err 为原始错误（记录到日志），userMsg 为展示给用户的友好提示
func redirectWithError(c *gin.Context, frontendURL string, err error, userMsg string) {
	log.Printf("[CAS] 认证错误: %s, 原始错误: %v", userMsg, err)
	c.Redirect(http.StatusFound,
		fmt.Sprintf("%s/#/login?cas_error=%s", frontendURL, url.QueryEscape(userMsg)))
}

// exchangeCodeForToken 用授权码换取 access_token
func exchangeCodeForToken(cfg config.CasConfig, code string) (string, error) {
	tokenURL := fmt.Sprintf("%s/oauth2.0/accessToken", cfg.ServerURL)

	data := url.Values{}
	data.Set("client_id", cfg.ClientID)
	data.Set("client_secret", cfg.ClientSecret)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", cfg.RedirectURI)
	data.Set("code", code)

	resp, err := casHTTPClient.Post(tokenURL, "application/x-www-form-urlencoded",
		strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("CAS返回错误 status=%d, 响应: %s", resp.StatusCode, string(body))
	}

	// 响应格式: access_token=xxx&expires=xxx （不是JSON）
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return "", err
	}

	accessToken := values.Get("access_token")
	if accessToken == "" {
		return "", fmt.Errorf("access_token为空, 响应: %s", string(body))
	}

	return accessToken, nil
}

// casProfileResponse CAS用户信息接口的响应
// 实际返回格式: {"id":"xxx","attributes":[{"KEY":"value"},...]}
type casProfileResponse struct {
	ID         string                   `json:"id"`
	Attributes []map[string]interface{} `json:"attributes"`
}

// getAttr 从 attributes 数组中提取指定字段的值
func (p *casProfileResponse) getAttr(key string) string {
	for _, attr := range p.Attributes {
		if val, ok := attr[key]; ok {
			return fmt.Sprintf("%v", val)
		}
	}
	return ""
}

// fetchCasUserProfile 用 access_token 获取用户在CAS系统中的信息
func fetchCasUserProfile(cfg config.CasConfig, accessToken string) (*casProfileResponse, error) {
	profileURL := fmt.Sprintf("%s/oauth2.0/profile?access_token=%s",
		cfg.ServerURL, url.QueryEscape(accessToken))

	resp, err := casHTTPClient.Post(profileURL, "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CAS返回错误 status=%d, 响应: %s", resp.StatusCode, string(body))
	}

	var profile casProfileResponse
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, fmt.Errorf("解析用户信息JSON失败: %w, 原始响应: %s", err, string(body))
	}

	if profile.getAttr("ID_NUMBER") == "" {
		return nil, fmt.Errorf("用户信息中ID_NUMBER为空, 原始响应: %s", string(body))
	}

	return &profile, nil
}

// findOrCreateUser 根据CAS用户信息查找本地用户，不存在则自动创建
func findOrCreateUser(ctx context.Context, casUser *casProfileResponse) (*models.User, error) {
	idNumber := casUser.getAttr("ID_NUMBER")
	userName := casUser.getAttr("USER_NAME")
	unitName := casUser.getAttr("UNIT_NAME")

	var user models.User

	result := database.DB.WithContext(ctx).Preload("Roles").Where("username = ?", idNumber).First(&user)
	if result.Error == nil {
		if user.Realname == "" && userName != "" {
			database.DB.WithContext(ctx).Model(&user).Update("realname", userName)
		}
		return &user, nil
	}

	randomPassword, _ := bcrypt.GenerateFromPassword(
		[]byte(fmt.Sprintf("cas_%s_%d", idNumber, time.Now().UnixNano())),
		bcrypt.DefaultCost,
	)

	user = models.User{
		Username: idNumber,
		Realname: userName,
		Password: string(randomPassword),
		College:  unitName,
	}

	if err := database.DB.WithContext(ctx).Create(&user).Error; err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}

	return &user, nil
}
