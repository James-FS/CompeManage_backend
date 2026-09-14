package middleware

import (
	"CompeManage_backend/database"
	"CompeManage_backend/logger"
	"CompeManage_backend/models"
	"fmt"
	"time"

	"gorm.io/gorm/clause"
)

const maxCacheInvalidationAttempts = 10

// QueuePermissionCacheInvalidation 以用户为粒度幂等创建或重置缓存失效任务。
func QueuePermissionCacheInvalidation(userID uint, cause error) error {
	now := time.Now()
	lastError := ""
	if cause != nil {
		lastError = cause.Error()
	}
	task := models.PermissionCacheInvalidationTask{
		UserID:      userID,
		Status:      models.CacheInvalidationPending,
		Attempts:    0,
		NextRetryAt: now,
		LastError:   lastError,
		CompletedAt: nil,
	}
	return database.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"status":        models.CacheInvalidationPending,
			"attempts":      0,
			"next_retry_at": now,
			"last_error":    lastError,
			"completed_at":  nil,
			"update_time":   now,
		}),
	}).Create(&task).Error
}

func ProcessPermissionCacheInvalidationTasks(limit int) {
	if limit <= 0 {
		limit = 50
	}
	now := time.Now()
	var tasks []models.PermissionCacheInvalidationTask
	if err := database.DB.Where("status IN ?", []string{
		models.CacheInvalidationPending,
		models.CacheInvalidationFailed,
	}).Where("next_retry_at <= ? AND attempts < ?", now, maxCacheInvalidationAttempts).
		Order("next_retry_at ASC").Limit(limit).Find(&tasks).Error; err != nil {
		logger.Error("查询权限缓存失效任务失败", "error", err)
		return
	}

	for _, task := range tasks {
		err := ClearUserPermissionCache(task.UserID)
		if err == nil {
			completedAt := time.Now()
			if updateErr := database.DB.Model(&models.PermissionCacheInvalidationTask{}).
				Where("id = ?", task.ID).
				Updates(map[string]interface{}{
					"status":       models.CacheInvalidationCompleted,
					"last_error":   "",
					"completed_at": completedAt,
				}).Error; updateErr != nil {
				logger.Error("完成权限缓存失效任务后更新状态失败", "taskID", task.ID, "error", updateErr)
			}
			continue
		}

		attempts := task.Attempts + 1
		delayMinutes := 1 << min(attempts, 6)
		status := models.CacheInvalidationFailed
		if attempts < maxCacheInvalidationAttempts {
			status = models.CacheInvalidationPending
		}
		if updateErr := database.DB.Model(&models.PermissionCacheInvalidationTask{}).
			Where("id = ?", task.ID).
			Updates(map[string]interface{}{
				"status":        status,
				"attempts":      attempts,
				"next_retry_at": time.Now().Add(time.Duration(delayMinutes) * time.Minute),
				"last_error":    err.Error(),
			}).Error; updateErr != nil {
			logger.Error("更新权限缓存失效任务失败", "taskID", task.ID, "error", updateErr)
		}
		logger.Error("权限缓存失效任务执行失败",
			"taskID", task.ID,
			"userID", task.UserID,
			"attempts", attempts,
			"error", fmt.Sprintf("%v", err),
		)
	}
}

func StartPermissionCacheInvalidationScheduler(interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		ProcessPermissionCacheInvalidationTasks(50)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			ProcessPermissionCacheInvalidationTasks(50)
		}
	}()
}
