package main

import (
	"CompeManage_backend/config"
	"CompeManage_backend/database"
	"CompeManage_backend/middleware"
	"CompeManage_backend/routes"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Println("提示：未找到.env文件，将使用默认配置")
	}

	// 初始化配置
	config.Init()

	// 连接数据库
	database.Init()

	// 创建Gin引擎（开发环境用gin.Default，生产可改为gin.ReleaseMode）
	r := gin.Default()

	// 注册中间件（跨域优先）
	r.Use(middleware.CORS())

	// 注册路由
	routes.SetupRoutes(r)

	// 启动服务
	port := config.GetString("server.port")
	log.Printf("服务器启动：http://localhost:%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("服务器启动失败：%v", err)
	}
}
