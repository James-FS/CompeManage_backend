package main

import "testing"

func TestPrintReport(t *testing.T) {
	// nil 报告：直接返回
	printReport(nil)

	// 正常报告
	printReport(map[string]interface{}{"total_users": 1, "ok": true})

	// 无法序列化：走错误分支
	printReport(make(chan int))

	// fatalIf(nil) 不退出
	fatalIf(nil)
}
