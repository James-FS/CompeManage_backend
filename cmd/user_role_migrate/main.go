package main

import (
	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	mode := flag.String("mode", "dry-run", "dry-run | apply | verify | rollback")
	backupTable := flag.String("backup-table", "", "apply时可指定备份表名，rollback时必须指定")
	confirm := flag.String("confirm", "", "apply填写APPLY，rollback填写ROLLBACK")
	flag.Parse()

	_ = godotenv.Load()
	config.Init()
	database.InitWithoutMigrate()

	switch *mode {
	case "dry-run":
		report, err := database.BuildUserRoleMigrationReport()
		printReport(report)
		fatalIf(err)
	case "apply":
		if *confirm != "APPLY" {
			log.Fatal("执行apply必须显式传入 --confirm APPLY")
		}
		report, err := database.ApplyUserRoleMigration(*backupTable)
		printReport(report)
		fatalIf(err)
	case "verify":
		report, err := database.VerifyUserRoleMigration()
		printReport(report)
		fatalIf(err)
	case "rollback":
		if *confirm != "ROLLBACK" {
			log.Fatal("执行rollback必须显式传入 --confirm ROLLBACK")
		}
		fatalIf(database.RollbackUserRoleMigration(*backupTable))
		fmt.Println("回滚完成")
	default:
		log.Fatalf("未知模式: %s", *mode)
	}
}

func printReport(report interface{}) {
	if report == nil {
		return
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Printf("序列化报告失败: %v", err)
		return
	}
	fmt.Println(string(data))
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
