package utils

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"log"
	"time"
)

func StartCompStatusScheduler(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		updateExpiredComps()

		for range ticker.C {
			updateExpiredComps()
		}
	}()
}

func updateExpiredComps() {
	// 查出所有已过截止时间的 comp_id
	var expiredIDs []uint
	err := database.DB.Model(&models.CompDetail{}).
		Where("GREATEST(reg_end_time, submit_end_time) < ?", time.Now()).
		Pluck("comp_id", &expiredIDs).Error

	if err != nil {
		log.Printf("[Scheduler] 查询过期竞赛失败: %v", err)
		return
	}
	if len(expiredIDs) == 0 {
		return
	}

	// 再更新这些 comp_id 中状态还是 1 的
	result := database.DB.Model(&models.CompDirectory{}).
		Where("id IN ? AND status = 1", expiredIDs).
		Update("status", 2)

	if result.Error != nil {
		log.Printf("[Scheduler] 更新竞赛状态失败: %v", result.Error)
		return
	}
	if result.RowsAffected > 0 {
		log.Printf("[Scheduler] 已将 %d 个竞赛状态更新为已结束", result.RowsAffected)
	}
}
