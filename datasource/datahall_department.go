package datasource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"CompeManage_backend/models"
)

type deptItem struct {
	OrgCode  *string `json:"D_STATIC_ORG_CODE"`
	OrgName  *string `json:"D_STATIC_ORG_NAME"`
	OrgEname *string `json:"D_STATIC_ORG_ENAME"`
}

type deptDataResult struct {
	Data    []deptItem `json:"data"`
	MaxPage int        `json:"max_page"`
	Page    int        `json:"page"`
	PerPage int        `json:"per_page"`
	Total   int        `json:"total"`
}

type deptDataResponse struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Result  deptDataResult `json:"result"`
}

func fetchAllDepartments(token string) ([]deptItem, error) {
	var allDepts []deptItem
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

		var dr deptDataResponse
		if err := json.Unmarshal(respBody, &dr); err != nil {
			preview := string(respBody)
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			log.Printf("[DataHall-Dept] 原始响应预览(page=%d): %s", page, preview)
			return nil, fmt.Errorf("解析机构数据失败(page=%d): %w", page, err)
		}

		if dr.Code != 10000 {
			return nil, fmt.Errorf("API 返回错误(page=%d): code=%d, msg=%s", page, dr.Code, dr.Message)
		}

		allDepts = append(allDepts, dr.Result.Data...)

		maxPage := dr.Result.MaxPage
		if maxPage == 0 {
			if dr.Result.Total > 0 {
				maxPage = (dr.Result.Total + perPage - 1) / perPage
			}
		}

		log.Printf("[DataHall-Dept] 第 %d/%d 页拉取完成，本页 %d 条，累计 %d 条",
			page, maxPage, len(dr.Result.Data), len(allDepts))

		if (maxPage > 0 && page >= maxPage) || len(dr.Result.Data) == 0 {
			break
		}
		page++
	}

	return allDepts, nil
}

func SyncDepartments() {
	log.Println("[DataHall-Dept] ========== 开始同步部门数据 ==========")
	startTime := time.Now()

	token, err := getAccessToken()
	if err != nil {
		log.Printf("[DataHall-Dept] 获取 token 失败: %v", err)
		return
	}

	depts, err := fetchAllDepartments(token)
	if err != nil {
		log.Printf("[DataHall-Dept] 拉取机构数据失败: %v", err)
		return
	}

	if len(depts) == 0 {
		log.Println("[DataHall-Dept] 警告: API 返回空数据，跳过本次同步")
		return
	}

	log.Printf("[DataHall-Dept] 拉取完成，共 %d 条机构记录，开始写入数据库...", len(depts))

	var created, updated, skipped, failed int

	for _, item := range depts {
		name := derefStr(item.OrgName)
		code := derefStr(item.OrgCode)
		if name == "" {
			log.Printf("[DataHall-Dept] 跳过 Name 为空的记录 (code=%s)", code)
			failed++
			continue
		}
		ename := derefStr(item.OrgEname)

		var dept models.Department
		found := false

		// Phase 1: 按 Code 匹配
		if code != "" {
			err := database.DB.Where("code = ?", code).First(&dept).Error
			if err == nil {
				found = true
			}
		}

		// Phase 2: 按 Name 匹配
		if !found {
			err := database.DB.Where("name = ?", name).First(&dept).Error
			if err == nil {
				found = true
			}
		}

		if found {
			if dept.Name == name && dept.Code == code && dept.Ename == ename {
				skipped++
				continue
			}
			if err := database.DB.Model(&dept).Updates(map[string]interface{}{
				"name":  name,
				"code":  code,
				"ename": ename,
			}).Error; err != nil {
				log.Printf("[DataHall-Dept] 更新部门 %s 失败: %v", name, err)
				failed++
				continue
			}
			updated++
		} else {
			if err := database.DB.Create(&models.Department{
				Name:  name,
				Code:  code,
				Ename: ename,
			}).Error; err != nil {
				log.Printf("[DataHall-Dept] 创建部门 %s 失败: %v", name, err)
				failed++
				continue
			}
			created++
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("[DataHall-Dept] 同步完成: 总数=%d 新增=%d 更新=%d 跳过=%d 失败=%d 耗时=%v",
		len(depts), created, updated, skipped, failed, elapsed)
	log.Println("[DataHall-Dept] ========== 部门数据同步结束 ==========")
}

func StartDepartmentScheduler(interval time.Duration) {
	go func() {
		time.Sleep(32 * time.Second)
		SyncDepartments()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			SyncDepartments()
		}
	}()
}
