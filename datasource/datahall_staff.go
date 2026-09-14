package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"time"

	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"CompeManage_backend/models"

	"gorm.io/gorm"
)

const (
	staffTokenCacheKey = "datahall:staff_access_token"
)

type staffItem struct {
	StaffID  *string `json:"O_STAFF_BASIC_STAFFID"`
	Name     *string `json:"O_STAFF_BASIC_NAME"`
	Sex      *string `json:"O_STAFF_BASIC_SEX"`
	Org      *string `json:"O_STAFF_BASIC_ORG"`
	TecTitle *string `json:"O_STAFF_BASIC_TECTITLE"`
}

type staffDataResult struct {
	Data    []staffItem `json:"data"`
	MaxPage int         `json:"max_page"`
	Page    int         `json:"page"`
	PerPage int         `json:"per_page"`
	Total   int         `json:"total"`
}

type staffDataResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Result  staffDataResult `json:"result"`
}

func getStaffAccessToken() (string, error) {
	if middleware.RedisClient != nil {
		token, err := middleware.RedisClient.Get(context.Background(), staffTokenCacheKey).Result()
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
		return "", fmt.Errorf("获取教职工 token 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return "", fmt.Errorf("读取教职工 token 响应失败: %w", err)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("解析教职工 token 响应失败: %w", err)
	}

	if tr.Code != 10000 {
		return "", fmt.Errorf("获取教职工 token 失败: code=%d, msg=%s", tr.Code, tr.Message)
	}

	token := tr.Result.AccessToken

	if middleware.RedisClient != nil {
		middleware.RedisClient.Set(context.Background(), staffTokenCacheKey, token, tokenTTL*time.Second)
	}

	log.Printf("[DataHall-Staff] token 获取成功，有效期 %d 秒", tr.Result.ExpiresIn)
	return token, nil
}

func fetchAllStaff(token string) ([]staffItem, error) {
	var allStaff []staffItem
	page := 1

	for {
		bodyMap := map[string]interface{}{
			"access_token": token,
			"page":         page,
			"per_page":     perPage,
		}

		bodyBytes, err := json.Marshal(bodyMap)
		if err != nil {
			return nil, fmt.Errorf("构建请求体失败(page=%d): %w", page, err)
		}

		url := config.AppConfig.DataHall.BaseURL + "/open_api/customization/adsstaffbasic_alpha/full"
		resp, err := httpClient.Post(url, "application/json", bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("请求教职工数据失败(page=%d): %w", page, err)
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("读取教职工数据响应失败(page=%d): %w", page, err)
		}

		var sr staffDataResponse
		if err := json.Unmarshal(respBody, &sr); err != nil {
			preview := string(respBody)
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			log.Printf("[DataHall-Staff] 原始响应预览(page=%d): %s", page, preview)
			return nil, fmt.Errorf("解析教职工数据失败(page=%d): %w", page, err)
		}

		if sr.Code != 10000 {
			return nil, fmt.Errorf("API 返回错误(page=%d): code=%d, msg=%s", page, sr.Code, sr.Message)
		}

		allStaff = append(allStaff, sr.Result.Data...)

		maxPage := sr.Result.MaxPage
		if maxPage == 0 {
			if sr.Result.Total > 0 {
				maxPage = (sr.Result.Total + perPage - 1) / perPage
			}
		}

		log.Printf("[DataHall-Staff] 第 %d/%d 页拉取完成，本页 %d 条，累计 %d 条",
			page, maxPage, len(sr.Result.Data), len(allStaff))

		if (maxPage > 0 && page >= maxPage) || len(sr.Result.Data) == 0 {
			break
		}
		page++
	}

	return allStaff, nil
}

func SyncStaff() {
	log.Println("[DataHall-Staff] ========== 开始同步教职工数据 ==========")
	startTime := time.Now()

	token, err := getStaffAccessToken()
	if err != nil {
		log.Printf("[DataHall-Staff] 获取 token 失败: %v", err)
		return
	}

	staff, err := fetchAllStaff(token)
	if err != nil {
		log.Printf("[DataHall-Staff] 拉取教职工数据失败: %v", err)
		return
	}

	log.Printf("[DataHall-Staff] 拉取完成，共 %d 条教职工记录，开始写入数据库...", len(staff))

	var competitionManagerRole models.Role
	if err := database.DB.Where("role_code = ?", "competition_manager").First(&competitionManagerRole).Error; err != nil {
		log.Printf("[DataHall-Staff] 未找到 competition_manager 角色: %v，终止本次同步", err)
		return
	}

	var created, updated, skipped, failed int

	for _, item := range staff {
		if item.StaffID == nil || *item.StaffID == "" {
			failed++
			continue
		}
		sid := *item.StaffID

		var user models.User
		result := database.DB.Where("username = ?", sid).First(&user)

		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			hashedPwd, pwdErr := hashPassword(lastNChars(sid, 6))
			if pwdErr != nil {
				log.Printf("[DataHall-Staff] 密码加密失败(职工号=%s): %v，跳过该用户", sid, pwdErr)
				failed++
				continue
			}

			user = models.User{
				Username:     sid,
				Realname:     derefStr(item.Name),
				Password:     hashedPwd,
				College:      derefStr(item.Org),
				Sex:          item.Sex,
				Title:        item.TecTitle,
				IdentityType: "staff",
			}

			if err := database.DB.Transaction(func(tx *gorm.DB) error {
				if err := tx.Create(&user).Error; err != nil {
					return err
				}
				return database.SetUserRole(tx, user.ID, competitionManagerRole.ID)
			}); err != nil {
				log.Printf("[DataHall-Staff] 创建用户 %s 失败: %v", sid, err)
				failed++
				continue
			}

			created++
		} else if result.Error != nil {
			log.Printf("[DataHall-Staff] 查询用户 %s 失败: %v", sid, result.Error)
			failed++
			continue
		} else {
			if user.IdentityType != "" && user.IdentityType != "staff" {
				skipped++
				continue
			}

			newRealname := derefStr(item.Name)
			newCollege := derefStr(item.Org)
			newSex := derefStr(item.Sex)
			newTitle := derefStr(item.TecTitle)

			if user.IdentityType == "staff" &&
				user.Realname == newRealname &&
				user.College == newCollege &&
				derefStr(user.Sex) == newSex &&
				derefStr(user.Title) == newTitle {
				skipped++
				continue
			}

			updates := map[string]interface{}{
				"identity_type": "staff",
				"realname":      newRealname,
				"college":       newCollege,
				"sex":           item.Sex,
				"title":         item.TecTitle,
			}
			if err := database.DB.Model(&user).Updates(updates).Error; err != nil {
				log.Printf("[DataHall-Staff] 更新用户 %s 失败: %v", sid, err)
				failed++
				continue
			}
			updated++
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("[DataHall-Staff] 同步完成: 总数=%d 新增=%d 更新=%d 跳过=%d 失败=%d 耗时=%v",
		len(staff), created, updated, skipped, failed, elapsed)
	log.Println("[DataHall-Staff] ========== 教职工数据同步结束 ==========")
}

func StartStaffScheduler(interval time.Duration) {
	go func() {
		time.Sleep(35 * time.Second)
		SyncStaff()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			SyncStaff()
		}
	}()
}
