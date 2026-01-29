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
		apiGroup.GET("/notice/list", controllers.GetNoticeList)  // 通知列表+筛选
		apiGroup.GET("/notice/:id", controllers.GetNoticeDetail) // 单个通知查看
		apiGroup.POST("/notice/create", controllers.CreateNotice)
		apiGroup.POST("/notice/comp/create", controllers.CreateCompNotice)
		apiGroup.GET("/college/list", controllers.GetCollegeList)
	}

	comp := r.Group("/api/comp", middleware.AuthRequired())
	{
		comp.GET("/list", controllers.GetCompetitionList)
		comp.POST("/create", controllers.CreateCompetition)
		comp.POST("/batch-import", controllers.BatchImportCompetition)
		comp.DELETE("/:id", controllers.DeleteCompetition)
		comp.POST("/batch-delete", controllers.BatchDeleteCompetition)
		comp.PUT("/:id/restore", controllers.RestoreCompetition)
		comp.GET("/manager/list", controllers.GetManagerList)
		comp.GET("/years", controllers.GetCompetitionYears)
	}

	// 赛事申报接口（院级管理员申报）
	declare := r.Group("/api/declare", middleware.AuthRequired())
	{
		// 院级申报
		declare.POST("", controllers.CreateDeclare)            // 创建申报
		declare.GET("/:id", controllers.GetDeclareDetail)      // 获取申报详情
		declare.PUT("/:id", controllers.UpdateDeclare)         // 更新申报信息
		declare.POST("/:id/submit", controllers.SubmitDeclare) // 提交申报
		declare.GET("/my/list", controllers.GetMyDeclares)     // 获取我的申报列表
		declare.DELETE("/:id", controllers.DeleteDeclare)      // 删除申报

		// 校级审核
		declare.GET("/pending/list", controllers.GetPendingDeclares) // 获取待审核申报
		declare.POST("/audit", controllers.AuditDeclare)             // 审核申报
		declare.GET("/all", controllers.GetAllDeclares)              // 获取所有申报
	}

	reg := r.Group("/api/reg", middleware.AuthRequired())
	{
		reg.POST("/config",
			middleware.RequirePermission("reg:config:edit"),
			controllers.SaveRegConfig)
		reg.GET("/config/get",
			middleware.RequirePermission("reg:config:view"),
			controllers.GetRegConfig)
		reg.POST("/submit",
			middleware.RequirePermission("reg:config:submit"),
			controllers.SubmitRegistration)
		reg.GET("/list",
			middleware.RequirePermission("reg:audit:list"),
			controllers.GetRegList)
		reg.GET("/detail",
			middleware.RequirePermission("reg:audit:detail"),
			controllers.GetRegDetail)
		reg.PUT("/audit",
			middleware.RequirePermission("reg:audit:update"),
			controllers.AuditRegister)
		reg.GET("/status", controllers.GetMyRegStatus)         // 查状态
		reg.PUT("/resubmit", controllers.ResubmitRegistration) // 重新提交
		reg.GET("/my-reg", controllers.GetMyRegList)
		reg.PUT("/work-submit", controllers.SubmitWork)
	}

	award := r.Group("/api/award", middleware.AuthRequired())
	{
		award.GET("/list", controllers.GetAwardCompList)
		award.GET("/comp-awards", controllers.GetCompAwards)
		award.GET("/export-template", controllers.ExportAwardTemplate)
		award.POST("/import", controllers.ImportAward)
	}
}
