package datasource

import (
	"testing"
	"time"
)

// 四个定时同步入口：启动后首轮同步在内置延迟后立即执行（外部地址不可达即安全返回）。
// 一次性覆盖四个 Start*Scheduler 的协程体；interval 设大避免重复触发。
func TestStartAllSchedulers_FirstRun(t *testing.T) {
	StartScheduler(time.Hour)
	StartCollegeScheduler(time.Hour)
	StartDepartmentScheduler(time.Hour)
	StartPostgradScheduler(time.Hour)

	// 最大的内置延迟是 33s，等它走完让四条协程都执行首轮
	time.Sleep(36 * time.Second)
}
