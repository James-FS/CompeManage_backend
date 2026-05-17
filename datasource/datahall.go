package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"

	"golang.org/x/crypto/bcrypt"
)

const (
	tokenCacheKey   = "datahall:access_token"
	tokenTTL        = 7000             // 比实际 7200s 少 200s 安全缓冲
	perPage         = 500              // 每页拉取条数
	httpTimeout     = 30 * time.Second // HTTP 请求超时
	maxResponseSize = 10 << 20         // 响应体最大 10MB
)

var httpClient = &http.Client{Timeout: httpTimeout}

// --- API 响应结构体 ---

type tokenResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Result  struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	} `json:"result"`
}

type studentItem struct {
	StudentID   *string `json:"O_STUDENT_BASIC_STUDENTID"`
	Name        *string `json:"O_STUDENT_BASIC_NAME"`
	Sex         *string `json:"O_STUDENT_BASIC_SEX"`
	CollegeName *string `json:"O_STUDENT_BASIC_COLLEGENAME"`
	MajorCode   *string `json:"O_STUDENT_BASIC_MAJORCODE"`
	MajorName   *string `json:"O_STUDENT_BASIC_MAJORNAME"`
	Grade       *string `json:"O_STUDENT_BASIC_GRADE"`
	ClassCode   *string `json:"O_STUDENT_BASIC_CLASSCODE"`
	ClassName   *string `json:"O_STUDENT_BASIC_CLASSNAME"`
}

type studentDataResult struct {
	Data    []studentItem `json:"data"`
	MaxPage int           `json:"max_page"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
	Total   int           `json:"total"`
}

type studentDataResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Result  studentDataResult `json:"result"`
}

// --- Token 管理 ---

func getAccessToken() (string, error) {
	// 先从 Redis 查缓存
	if middleware.RedisClient != nil {
		token, err := middleware.RedisClient.Get(context.Background(), tokenCacheKey).Result()
		if err == nil && token != "" {
			return token, nil
		}
	}

	// 缓存未命中或 Redis 不可用，调 API 获取
	authURL := fmt.Sprintf("%s/open_api/authentication/get_access_token?key=%s&secret=%s",
		config.AppConfig.DataHall.BaseURL,
		url.QueryEscape(config.AppConfig.DataHall.Key),
		url.QueryEscape(config.AppConfig.DataHall.Secret),
	)

	resp, err := httpClient.Get(authURL)
	if err != nil {
		return "", fmt.Errorf("获取 token 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return "", fmt.Errorf("读取 token 响应失败: %w", err)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("解析 token 响应失败: %w", err)
	}

	if tr.Code != 10000 {
		return "", fmt.Errorf("获取 token 失败: code=%d, msg=%s", tr.Code, tr.Message)
	}

	token := tr.Result.AccessToken

	// 写入 Redis 缓存
	if middleware.RedisClient != nil {
		middleware.RedisClient.Set(context.Background(), tokenCacheKey, token, tokenTTL*time.Second)
	}

	log.Printf("[DataHall] token 获取成功，有效期 %d 秒", tr.Result.ExpiresIn)
	return token, nil
}

// --- 数据拉取 ---

func fetchAllStudents(token string) ([]studentItem, error) {
	var allStudents []studentItem
	page := 1

	// 构建年级筛选条件（配置了 min_grade 则用 gte 过滤，如 >= "2022"）
	minGrade := config.AppConfig.DataHall.MinGrade

	for {
		bodyMap := map[string]interface{}{
			"access_token": token,
			"page":         page,
			"per_page":     perPage,
		}
		if minGrade != "" {
			bodyMap["O_STUDENT_BASIC_GRADE"] = map[string]string{"gte": minGrade}
		}
		bodyBytes, err := json.Marshal(bodyMap)
		if err != nil {
			return nil, fmt.Errorf("构建请求体失败(page=%d): %w", page, err)
		}

		url := config.AppConfig.DataHall.BaseURL + "/open_api/customization/adsstudentugbasicnew_alpha/full"
		resp, err := httpClient.Post(url, "application/json", bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("请求学生数据失败(page=%d): %w", page, err)
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("读取学生数据响应失败(page=%d): %w", page, err)
		}

		var sr studentDataResponse
		if err := json.Unmarshal(respBody, &sr); err != nil {
			preview := string(respBody)
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			log.Printf("[DataHall] 原始响应预览(page=%d): %s", page, preview)
			return nil, fmt.Errorf("解析学生数据失败(page=%d): %w", page, err)
		}

		if sr.Code != 10000 {
			return nil, fmt.Errorf("API 返回错误(page=%d): code=%d, msg=%s", page, sr.Code, sr.Message)
		}

		allStudents = append(allStudents, sr.Result.Data...)

		// 判断是否还有下一页
		maxPage := sr.Result.MaxPage
		if maxPage == 0 {
			// 如果 maxPage 无效，通过 total 估算
			if sr.Result.Total > 0 {
				maxPage = (sr.Result.Total + perPage - 1) / perPage
			}
		}

		log.Printf("[DataHall] 第 %d/%d 页拉取完成，本页 %d 条，累计 %d 条",
			page, maxPage, len(sr.Result.Data), len(allStudents))

		if (maxPage > 0 && page >= maxPage) || len(sr.Result.Data) == 0 {
			break
		}
		page++
	}

	return allStudents, nil
}

// --- 数据同步 ---

func SyncStudents() {
	log.Println("[DataHall] ========== 开始同步学生数据 ==========")
	startTime := time.Now()

	// 1. 获取 token
	token, err := getAccessToken()
	if err != nil {
		log.Printf("[DataHall] 获取 token 失败: %v", err)
		return
	}

	// 2. 拉取全量学生数据
	students, err := fetchAllStudents(token)
	if err != nil {
		log.Printf("[DataHall] 拉取学生数据失败: %v", err)
		return
	}

	log.Printf("[DataHall] 拉取完成，共 %d 条学生记录，开始写入数据库...", len(students))

	// 3. 预查 student 角色
	var studentRole models.Role
	if err := database.DB.Where("role_code = ?", "student").First(&studentRole).Error; err != nil {
		log.Printf("[DataHall] 未找到 student 角色: %v，新用户将不分配角色", err)
	}

	// 4. 查出所有已有 student 角色的用户 ID，用于后续判断是否可更新
	var studentUserIDs []uint
	database.DB.Table("user_roles").
		Where("role_id = ?", studentRole.ID).
		Pluck("user_id", &studentUserIDs)
	studentSet := make(map[uint]bool, len(studentUserIDs))
	for _, uid := range studentUserIDs {
		studentSet[uid] = true
	}

	// 5. 逐条 Upsert
	var created, updated, skipped, failed int

	for _, item := range students {
		if item.StudentID == nil || *item.StudentID == "" {
			failed++
			continue
		}
		sid := *item.StudentID

		var user models.User
		result := database.DB.Where("username = ?", sid).First(&user)

		if result.Error != nil {
			// 不存在 → 创建新用户
			hashedPwd, pwdErr := hashPassword(lastNChars(sid, 6))
			if pwdErr != nil {
				log.Printf("[DataHall] 密码加密失败(学号=%s): %v，跳过该用户", sid, pwdErr)
				failed++
				continue
			}

			user = models.User{
				Username:  sid,
				Realname:  derefStr(item.Name),
				Password:  hashedPwd,
				College:   derefStr(item.CollegeName),
				Major:     derefStr(item.MajorName),
				Grade:     derefStr(item.Grade),
				Sex:       item.Sex,
				MajorCode: item.MajorCode,
				ClassCode: item.ClassCode,
				ClassName: item.ClassName,
			}

			if err := database.DB.Create(&user).Error; err != nil {
				log.Printf("[DataHall] 创建用户 %s 失败: %v", sid, err)
				failed++
				continue
			}

			// 分配 student 角色
			if studentRole.ID != 0 {
				database.DB.Model(&user).Association("Roles").Append(&studentRole)
			}

			created++
		} else {
			// 已存在 → 仅当用户是纯学生账号时才更新，防止覆盖教师/管理员信息
			if !studentSet[user.ID] {
				skipped++
				continue
			}
			updates := map[string]interface{}{
				"realname":   derefStr(item.Name),
				"college":    derefStr(item.CollegeName),
				"major":      derefStr(item.MajorName),
				"grade":      derefStr(item.Grade),
				"sex":        item.Sex,
				"major_code": item.MajorCode,
				"class_code": item.ClassCode,
				"class_name": item.ClassName,
			}
			if err := database.DB.Model(&user).Updates(updates).Error; err != nil {
				log.Printf("[DataHall] 更新用户 %s 失败: %v", sid, err)
				failed++
				continue
			}
			updated++
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("[DataHall] 同步完成: 总数=%d 新增=%d 更新=%d 跳过=%d 失败=%d 耗时=%v",
		len(students), created, updated, skipped, failed, elapsed)
	log.Println("[DataHall] ========== 学生数据同步结束 ==========")
}

// --- 定时任务 ---

func StartScheduler(interval time.Duration) {
	go func() {
		time.Sleep(30 * time.Second) // 延迟启动，避免阻塞服务初始化
		SyncStudents()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			SyncStudents()
		}
	}()
}

// --- 工具函数 ---

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func lastNChars(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[len(s)-n:]
}

func hashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt 加密失败: %w", err)
	}
	return string(hashed), nil
}
