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
	now := time.Now()

	// 1) 未开始 -> 进行中：报名开始后自动进入进行中。
	var startedIDs []uint
	err := database.DB.Model(&models.CompDetail{}).
		Where("reg_start_time <= ?", now).
		Pluck("comp_id", &startedIDs).Error
	if err != nil {
		log.Printf("[Scheduler] 查询可开始竞赛失败: %v", err)
		return
	}

	if len(startedIDs) > 0 {
		startResult := database.DB.Model(&models.CompDirectory{}).
			Where("id IN ? AND status = 0", startedIDs).
			Update("status", 1)

		if startResult.Error != nil {
			log.Printf("[Scheduler] 更新竞赛状态 0->1 失败: %v", startResult.Error)
			return
		}
		if startResult.RowsAffected > 0 {
			log.Printf("[Scheduler] 已将 %d 个竞赛状态更新为进行中", startResult.RowsAffected)
		}
	}

	// 2) 进行中 -> 已结束：报名与作品提交都过期后自动结束。
	var expiredIDs []uint
	err = database.DB.Model(&models.CompDetail{}).
		Where("GREATEST(reg_end_time, submit_end_time) < ?", now).
		Pluck("comp_id", &expiredIDs).Error
	if err != nil {
		log.Printf("[Scheduler] 查询过期竞赛失败: %v", err)
		return
	}

	if len(expiredIDs) > 0 {
		endResult := database.DB.Model(&models.CompDirectory{}).
			Where("id IN ? AND status = 1", expiredIDs).
			Update("status", 2)

		if endResult.Error != nil {
			log.Printf("[Scheduler] 更新竞赛状态 1->2 失败: %v", endResult.Error)
			return
		}
		if endResult.RowsAffected > 0 {
			log.Printf("[Scheduler] 已将 %d 个竞赛状态更新为已结束", endResult.RowsAffected)
		}
	}
}
