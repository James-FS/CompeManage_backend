package datasource

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// 四个定时同步入口：启动后首轮同步在内置延迟后立即执行（外部地址不可达即安全返回）。
// 子进程隔离避免其后台协程与同包测试对全局数据大厅配置的修改发生竞态。
func TestStartAllSchedulers_FirstRun(t *testing.T) {
	const subprocessFlag = "COMPE_TEST_START_ALL_SCHEDULERS_SUBPROCESS"
	if os.Getenv(subprocessFlag) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestStartAllSchedulers_FirstRun$")
		env := os.Environ()
		flagFound := false
		for i, value := range env {
			if strings.HasPrefix(value, subprocessFlag+"=") {
				env[i] = subprocessFlag + "=1"
				flagFound = true
			}
		}
		if !flagFound {
			env = append(env, subprocessFlag+"=1")
		}
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("scheduler subprocess failed: %v\n%s", err, output)
		}
		return
	}

	StartScheduler(time.Hour)
	StartCollegeScheduler(time.Hour)
	StartDepartmentScheduler(time.Hour)
	StartPostgradScheduler(time.Hour)

	// 最大的内置延迟是 33s，等它走完让四条协程都执行首轮
	time.Sleep(36 * time.Second)
}
