package datasource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"CompeManage_backend/models"
)

type collegeItem struct {
	OrgCode  *string `json:"D_STATIC_ORG_CODE"`
	OrgName  *string `json:"D_STATIC_ORG_NAME"`
	OrgEname *string `json:"D_STATIC_ORG_ENAME"`
}

type collegeDataResult struct {
	Data    []collegeItem `json:"data"`
	MaxPage int           `json:"max_page"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
	Total   int           `json:"total"`
}

type collegeDataResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Result  collegeDataResult `json:"result"`
}

func fetchAllColleges(token string) ([]collegeItem, error) {
	var allColleges []collegeItem
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

		url := config.AppConfig.DataHall.BaseURL + "/open_api/customization/adsdcsorganization/full"
		resp, err := httpClient.Post(url, "application/json", bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("请求机构数据失败(page=%d): %w", page, err)
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("读取机构数据响应失败(page=%d): %w", page, err)
		}

		var cr collegeDataResponse
		if err := json.Unmarshal(respBody, &cr); err != nil {
			preview := string(respBody)
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			log.Printf("[DataHall-College] 原始响应预览(page=%d): %s", page, preview)
			return nil, fmt.Errorf("解析机构数据失败(page=%d): %w", page, err)
		}

		if cr.Code != 10000 {
			return nil, fmt.Errorf("API 返回错误(page=%d): code=%d, msg=%s", page, cr.Code, cr.Message)
		}

		allColleges = append(allColleges, cr.Result.Data...)

		maxPage := cr.Result.MaxPage
		if maxPage == 0 {
			if cr.Result.Total > 0 {
				maxPage = (cr.Result.Total + perPage - 1) / perPage
			}
		}

		log.Printf("[DataHall-College] 第 %d/%d 页拉取完成，本页 %d 条，累计 %d 条",
			page, maxPage, len(cr.Result.Data), len(allColleges))

		if (maxPage > 0 && page >= maxPage) || len(cr.Result.Data) == 0 {
			break
		}
		page++
	}

	return allColleges, nil
}

func SyncColleges() {
	log.Println("[DataHall-College] ========== 开始同步组织机构数据 ==========")
	startTime := time.Now()

	token, err := getAccessToken()
	if err != nil {
		log.Printf("[DataHall-College] 获取 token 失败: %v", err)
		return
	}

	colleges, err := fetchAllColleges(token)
	if err != nil {
		log.Printf("[DataHall-College] 拉取机构数据失败: %v", err)
		return
	}

	if len(colleges) == 0 {
		log.Println("[DataHall-College] 警告: API 返回空数据，跳过本次同步")
		return
	}

	log.Printf("[DataHall-College] 拉取完成，共 %d 条机构记录，开始写入数据库...", len(colleges))

	var created, updated, skipped, failed int

	for _, item := range colleges {
		name := derefStr(item.OrgName)
		code := derefStr(item.OrgCode)
		if name == "" {
			log.Printf("[DataHall-College] 跳过 Name 为空的记录 (code=%s)", code)
			failed++
			continue
		}
		if !strings.Contains(name, "学院") || code < "0201" || code > "0298" {
			skipped++
			continue
		}
		ename := derefStr(item.OrgEname)

		var college models.College
		found := false

		// Phase 1: 按 Code 匹配（稳定标识符，处理改名场景）
		if code != "" {
			err := database.DB.Where("code = ?", code).First(&college).Error
			if err == nil {
				found = true
			}
		}

		// Phase 2: 按 Name 匹配（Code 为空或 Phase 1 未命中时的回退）
		if !found {
			err := database.DB.Where("name = ?", name).First(&college).Error
			if err == nil {
				found = true
			}
		}

		if found {
			if college.Name == name && college.Code == code && college.Ename == ename {
				skipped++
				continue
			}
			if err := database.DB.Model(&college).Updates(map[string]interface{}{
				"name":  name,
				"code":  code,
				"ename": ename,
			}).Error; err != nil {
				log.Printf("[DataHall-College] 更新机构 %s 失败: %v", name, err)
				failed++
				continue
			}
			updated++
		} else {
			if err := database.DB.Create(&models.College{
				Name:  name,
				Code:  code,
				Ename: ename,
			}).Error; err != nil {
				log.Printf("[DataHall-College] 创建机构 %s 失败: %v", name, err)
				failed++
				continue
			}
			created++
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("[DataHall-College] 同步完成: 总数=%d 新增=%d 更新=%d 跳过=%d 失败=%d 耗时=%v",
		len(colleges), created, updated, skipped, failed, elapsed)
	log.Println("[DataHall-College] ========== 组织机构数据同步结束 ==========")
}

func StartCollegeScheduler(interval time.Duration) {
	go func() {
		time.Sleep(33 * time.Second)
		SyncColleges()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			SyncColleges()
		}
	}()
}
