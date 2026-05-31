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
	tokenTTL        = 7000
	perPage         = 500
	httpTimeout     = 30 * time.Second
	maxResponseSize = 10 << 20
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
	if middleware.RedisClient != nil {
		token, err := middleware.RedisClient.Get(context.Background(), tokenCacheKey).Result()
		if err == nil && token != "" {
			return token, nil
		}
	}

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

		maxPage := sr.Result.MaxPage
		if maxPage == 0 {
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

	token, err := getAccessToken()
	if err != nil {
		log.Printf("[DataHall] 获取 token 失败: %v", err)
		return
	}

	students, err := fetchAllStudents(token)
	if err != nil {
		log.Printf("[DataHall] 拉取学生数据失败: %v", err)
		return
	}

	log.Printf("[DataHall] 拉取完成，共 %d 条学生记录，开始写入数据库...", len(students))

	var studentRole models.Role
	if err := database.DB.Where("role_code = ?", "student").First(&studentRole).Error; err != nil {
		log.Printf("[DataHall] 未找到 student 角色: %v，新用户将不分配角色", err)
	}

	var studentUserIDs []uint
	database.DB.Table("user_roles").
		Where("role_id = ?", studentRole.ID).
		Pluck("user_id", &studentUserIDs)
	studentSet := make(map[uint]bool, len(studentUserIDs))
	for _, uid := range studentUserIDs {
		studentSet[uid] = true
	}

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
			// 不存在 → 创建
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

			if studentRole.ID != 0 {
				database.DB.Model(&user).Association("Roles").Append(&studentRole)
			}

			created++
		} else {
			// 已存在 → 仅更新学生账号
			if !studentSet[user.ID] {
				skipped++
				continue
			}

			// 比较字段是否有变化，无变化则跳过
			newRealname := derefStr(item.Name)
			newCollege := derefStr(item.CollegeName)
			newMajor := derefStr(item.MajorName)
			newGrade := derefStr(item.Grade)
			newSex := derefStr(item.Sex)
			newMajorCode := derefStr(item.MajorCode)
			newClassCode := derefStr(item.ClassCode)
			newClassName := derefStr(item.ClassName)

			if user.Realname == newRealname &&
				user.College == newCollege &&
				user.Major == newMajor &&
				user.Grade == newGrade &&
				derefStr(user.Sex) == newSex &&
				derefStr(user.MajorCode) == newMajorCode &&
				derefStr(user.ClassCode) == newClassCode &&
				derefStr(user.ClassName) == newClassName {
				skipped++
				continue
			}

			updates := map[string]interface{}{
				"realname":   newRealname,
				"college":    newCollege,
				"major":      newMajor,
				"grade":      newGrade,
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
		time.Sleep(30 * time.Second)
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
