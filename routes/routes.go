package routes

import (
	"CompeManage_backend/controllers"
	"CompeManage_backend/utils"

	"github.com/gin-gonic/gin"
)

// SetupRoutes 注册所有路由
func SetupRoutes(r *gin.Engine) {
	// 健康检查接口（公开）
	r.GET("/health", func(c *gin.Context) {
		utils.Success(c, gin.H{"status": "ok"})
	})
	apiGroup := r.Group("/api")
	{
		apiGroup.POST("/login", controllers.Login)
		apiGroup.GET("/permission/list", controllers.GetAllPermissions)
		apiGroup.GET("/role/list", controllers.GetAllRoles)
		apiGroup.POST("/role/assign_perm", controllers.AssignPermissions)
		apiGroup.GET("/notice/list", controllers.GetNoticeList)  // 通知列表+筛选
		apiGroup.GET("/notice/:id", controllers.GetNoticeDetail) // 单个通知查看
	}
	// 其他业务路由可按此方式扩展
}
