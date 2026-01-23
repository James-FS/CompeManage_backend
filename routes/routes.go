package routes

import (
	"CompeManage_backend/controllers"
	"CompeManage_backend/middleware"
	"CompeManage_backend/utils"

	"github.com/gin-gonic/gin"
)

// SetupRoutes 注册所有路由
func SetupRoutes(r *gin.Engine) {
	// 健康检查接口（公开）
	r.GET("/health", func(c *gin.Context) {
		utils.Success(c, gin.H{"status": "ok"})
	})

	r.Static("/static", "./static")
	upload := r.Group("/api/upload", middleware.AuthRequired())
	{
		upload.POST("", controllers.UploadFile)
	}

	apiGroup := r.Group("/api")
	{
		apiGroup.POST("/login", controllers.Login)
		apiGroup.GET("/permission/list", controllers.GetAllPermissions)
		apiGroup.GET("/role/list", controllers.GetAllRoles)
		apiGroup.POST("/role/assign_perm", controllers.AssignPermissions)
	}

	comp := r.Group("/api/comp", middleware.AuthRequired())
	{
		comp.GET("/list", controllers.GetCompetitionList)
	} // 其他业务路由可按此方式扩展

	reg := r.Group("/api/reg", middleware.AuthRequired())
	{
		reg.POST("/config",
			middleware.RequirePermission("reg:config:edit"),
			controllers.SaveRegConfig)
		reg.GET("/config/get",
			middleware.RequirePermission("reg:config:view"),
			controllers.GetRegConfig)
		reg.POST("/submit",
			controllers.SubmitRegistration)
	}
}
