package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"time"

	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"
)

const postgradTokenCacheKey = "datahall:postgrad_access_token"

type postgradItem struct {
	StudentID   *string `json:"O_STUDENT_BASIC_STUDENTID"`
	Name        *string `json:"O_STUDENT_BASIC_NAME"`
	Sex         *string `json:"O_STUDENT_BASIC_SEX"`
	CollegeName *string `json:"O_STUDENT_BASIC_COLLEGENAME"`
	CollegeCode *string `json:"O_STUDENT_BASIC_COLLEGECODE"`
	MajorName   *string `json:"O_STUDENT_BASIC_MAJORNAME"`
	MajorType   *string `json:"O_STUDENT_BASIC_MAJORTYPE"`
	EnrollYear  *string `json:"O_STUDENT_BASIC_ENROLLYEAR"`
}

type postgradDataResult struct {
	Data    []postgradItem `json:"data"`
	MaxPage int            `json:"max_page"`
	Page    int            `json:"page"`
	PerPage int            `json:"per_page"`
	Total   int            `json:"total"`
}

type postgradDataResponse struct {
	Code    int                `json:"code"`
	Message string             `json:"message"`
	Result  postgradDataResult `json:"result"`
}

func getPostgradAccessToken() (string, error) {
	if middleware.RedisClient != nil {
		token, err := middleware.RedisClient.Get(context.Background(), postgradTokenCacheKey).Result()
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
		return "", fmt.Errorf("获取研究生 token 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return "", fmt.Errorf("读取研究生 token 响应失败: %w", err)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("解析研究生 token 响应失败: %w", err)
	}

	if tr.Code != 10000 {
		return "", fmt.Errorf("获取研究生 token 失败: code=%d, msg=%s", tr.Code, tr.Message)
	}

	token := tr.Result.AccessToken

	if middleware.RedisClient != nil {
		middleware.RedisClient.Set(context.Background(), postgradTokenCacheKey, token, tokenTTL*time.Second)
	}

	log.Printf("[DataHall-Postgrad] token 获取成功，有效期 %d 秒", tr.Result.ExpiresIn)
	return token, nil
}

func fetchAllPostgrad(token string) ([]postgradItem, error) {
	var allPostgrad []postgradItem
	page := 1
	minGrade := config.AppConfig.DataHall.MinGrade

	for {
		bodyMap := map[string]interface{}{
			"access_token": token,
			"page":         page,
			"per_page":     perPage,
		}
		if minGrade != "" {
			bodyMap["O_STUDENT_BASIC_ENROLLYEAR"] = map[string]string{"gte": minGrade}
		}

		bodyBytes, err := json.Marshal(bodyMap)
		if err != nil {
			return nil, fmt.Errorf("构建请求体失败(page=%d): %w", page, err)
		}

		url := config.AppConfig.DataHall.BaseURL + "/open_api/customization/adsstudentpgbasic_bravo/full"
		resp, err := httpClient.Post(url, "application/json", bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("请求研究生数据失败(page=%d): %w", page, err)
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("读取研究生数据响应失败(page=%d): %w", page, err)
		}

		var sr postgradDataResponse
		if err := json.Unmarshal(respBody, &sr); err != nil {
			preview := string(respBody)
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			log.Printf("[DataHall-Postgrad] 原始响应预览(page=%d): %s", page, preview)
			return nil, fmt.Errorf("解析研究生数据失败(page=%d): %w", page, err)
		}

		if sr.Code != 10000 {
			return nil, fmt.Errorf("API 返回错误(page=%d): code=%d, msg=%s", page, sr.Code, sr.Message)
		}

		allPostgrad = append(allPostgrad, sr.Result.Data...)

		maxPage := sr.Result.MaxPage
		if maxPage == 0 {
			if sr.Result.Total > 0 {
				maxPage = (sr.Result.Total + perPage - 1) / perPage
			}
		}

		log.Printf("[DataHall-Postgrad] 第 %d/%d 页拉取完成，本页 %d 条，累计 %d 条",
			page, maxPage, len(sr.Result.Data), len(allPostgrad))

		if (maxPage > 0 && page >= maxPage) || len(sr.Result.Data) == 0 {
			break
		}
		page++
	}

	return allPostgrad, nil
}

func SyncPostgrad() {
	log.Println("[DataHall-Postgrad] ========== 开始同步研究生数据 ==========")
	startTime := time.Now()

	token, err := getPostgradAccessToken()
	if err != nil {
		log.Printf("[DataHall-Postgrad] 获取 token 失败: %v", err)
		return
	}

	postgrads, err := fetchAllPostgrad(token)
	if err != nil {
		log.Printf("[DataHall-Postgrad] 拉取研究生数据失败: %v", err)
		return
	}

	log.Printf("[DataHall-Postgrad] 拉取完成，共 %d 条研究生记录，开始写入数据库...", len(postgrads))

	var studentRole models.Role
	if err := database.DB.Where("role_code = ?", "student").First(&studentRole).Error; err != nil {
		log.Printf("[DataHall-Postgrad] 未找到 student 角色: %v，新用户将不分配角色", err)
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

	for _, item := range postgrads {
		if item.StudentID == nil || *item.StudentID == "" {
			failed++
			continue
		}
		sid := *item.StudentID

		var user models.User
		result := database.DB.Where("username = ?", sid).First(&user)

		if result.Error != nil {
			hashedPwd, pwdErr := hashPassword(lastNChars(sid, 6))
			if pwdErr != nil {
				log.Printf("[DataHall-Postgrad] 密码加密失败(学号=%s): %v，跳过该用户", sid, pwdErr)
				failed++
				continue
			}

			user = models.User{
				Username:  sid,
				Realname:  derefStr(item.Name),
				Password:  hashedPwd,
				College:   derefStr(item.CollegeName),
				Major:     derefStr(item.MajorName),
				Grade:     derefStr(item.EnrollYear),
				Sex:       item.Sex,
				MajorCode: item.MajorType,
			}

			if err := database.DB.Create(&user).Error; err != nil {
				log.Printf("[DataHall-Postgrad] 创建用户 %s 失败: %v", sid, err)
				failed++
				continue
			}

			if studentRole.ID != 0 {
				database.DB.Model(&user).Association("Roles").Append(&studentRole)
			}

			created++
		} else {
			if !studentSet[user.ID] {
				skipped++
				continue
			}

			newRealname := derefStr(item.Name)
			newCollege := derefStr(item.CollegeName)
			newMajor := derefStr(item.MajorName)
			newGrade := derefStr(item.EnrollYear)
			newSex := derefStr(item.Sex)
			newMajorType := derefStr(item.MajorType)

			if user.Realname == newRealname &&
				user.College == newCollege &&
				user.Major == newMajor &&
				user.Grade == newGrade &&
				derefStr(user.Sex) == newSex &&
				derefStr(user.MajorCode) == newMajorType {
				skipped++
				continue
			}

			updates := map[string]interface{}{
				"realname":   newRealname,
				"college":    newCollege,
				"major":      newMajor,
				"grade":      newGrade,
				"sex":        item.Sex,
				"major_code": item.MajorType,
			}
			if err := database.DB.Model(&user).Updates(updates).Error; err != nil {
				log.Printf("[DataHall-Postgrad] 更新用户 %s 失败: %v", sid, err)
				failed++
				continue
			}
			updated++
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("[DataHall-Postgrad] 同步完成: 总数=%d 新增=%d 更新=%d 跳过=%d 失败=%d 耗时=%v",
		len(postgrads), created, updated, skipped, failed, elapsed)
	log.Println("[DataHall-Postgrad] ========== 研究生数据同步结束 ==========")
}

func StartPostgradScheduler(interval time.Duration) {
	go func() {
		time.Sleep(40 * time.Second)
		SyncPostgrad()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			SyncPostgrad()
		}
	}()
}
