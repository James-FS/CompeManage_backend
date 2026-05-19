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

		apiGroup.GET("/college/list", controllers.GetCollegeList)
	}

	notice := r.Group("/api/notice", middleware.AuthRequired())
	{
		notice.GET("/list",
			controllers.GetNoticeList,
			middleware.RequirePermission("notice:list")) // 通知列表+筛选
		notice.GET("/:id",
			controllers.GetNoticeDetail,
			middleware.RequirePermission("notice:detail")) // 单个通知查看
		//notice.POST("/create",
		//	controllers.CreateNotice,
		//	middleware.RequirePermission("notice:create"))
		notice.POST("/comp/create",
			controllers.CreateCompNotice,
			middleware.RequirePermission("notice:create"))
		notice.PUT("/:id/publish",
			controllers.PublishNotice,
			middleware.RequirePermission("notice:publish"))
		notice.DELETE("/:id",
			controllers.DeleteNotice,
			middleware.RequirePermission("notice:delete"))
	}

	perm := r.Group("/api/perm", middleware.AuthRequired())
	{
		perm.GET("/permission/list",
			controllers.GetAllPermissions,
			middleware.RequirePermission("perm:list"))
		perm.GET("/role/list",
			controllers.GetAllRoles,
			middleware.RequirePermission("role:list"))
		perm.POST("/role/assign_perm",
			controllers.AssignPermissions,
			middleware.RequirePermission("perm:assign"))
	}

	comp := r.Group("/api/comp", middleware.AuthRequired())
	{
		comp.GET("/list",
			controllers.GetCompetitionList,
			middleware.RequirePermission("comp:list"))
		comp.POST("/create",
			controllers.CreateCompetition,
			middleware.RequirePermission("comp:create"))
		comp.POST("/batch-import",
			controllers.BatchImportCompetition,
			middleware.RequirePermission("comp:batch-import"))
		comp.DELETE("/:id",
			controllers.DeleteCompetition,
			middleware.RequirePermission("comp:delete"))
		comp.POST("/batch-delete",
			controllers.BatchDeleteCompetition,
			middleware.RequirePermission("comp:batch-delete"))
		comp.PUT("/:id/restore",
			controllers.RestoreCompetition,
			middleware.RequirePermission("comp:restore"))
		comp.GET("/manager/list",
			controllers.GetManagerList,
			middleware.RequirePermission("manager:list"))
		comp.GET("/years",
			controllers.GetCompetitionYears,
			middleware.RequirePermission("comp:years:list"))
		comp.GET("/:id",
			middleware.RequirePermission("comp:detail"),
			controllers.GetCompetitionDetail)
		comp.PUT("/:id",
			controllers.UpdateCompetition,
			middleware.RequirePermission("comp:update"))
	}

	// 赛事申报接口（院级管理员申报）
	declare := r.Group("/api/declare", middleware.AuthRequired())
	{
		// 院级申报
		declare.POST("",
			controllers.CreateDeclare,
			middleware.RequirePermission("declare:create")) // 创建申报
		declare.GET("/:id",
			controllers.GetDeclareDetail,
			middleware.RequirePermission("declare:get")) // 获取申报详情
		declare.PUT("/:id",
			controllers.UpdateDeclare,
			middleware.RequirePermission("declare:update")) // 更新申报信息
		declare.POST("/:id/submit",
			controllers.SubmitDeclare,
			middleware.RequirePermission("declare:submit")) // 提交申报
		declare.POST("/:id/revoke",
			controllers.RevokeDeclare,
			middleware.RequirePermission("declare:revoke")) // 撤回申报
		declare.GET("/my/list",
			controllers.GetMyDeclares,
			middleware.RequirePermission("declare:list")) // 获取我的申报列表
		declare.GET("/my/pending",
			controllers.GetMyPendingDeclares,
			middleware.RequirePermission("declare:list")) // 获取我的待审核申报
		declare.GET("/my/published",
			controllers.GetMyPublishedDeclares,
			middleware.RequirePermission("declare:list")) // 获取我的已发布申报
		declare.DELETE("/:id",
			controllers.DeleteDeclare,
			middleware.RequirePermission("declare:delete")) // 删除申报

		// 校级审核
		declare.GET("/pending/list",
			controllers.GetPendingDeclares,
			middleware.RequirePermission("declare:pending-list")) // 获取待审核申报
		declare.GET("/audited/list",
			controllers.GetAuditedDeclares,
			middleware.RequirePermission("declare:audited-list")) // 获取已审核申报
		declare.POST("/audit",
			controllers.AuditDeclare,
			middleware.RequirePermission("declare:audit")) // 审核申报
		declare.GET("/all",
			controllers.GetAllDeclares,
			middleware.RequirePermission("declare:all-declares")) // 获取所有申报
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
		reg.GET("/work/audit/comp/list",
			middleware.RequirePermission("reg:audit:list"),
			controllers.GetWorkAuditCompList)
		reg.GET("/work/audit/student/list",
			middleware.RequirePermission("reg:audit:detail"),
			controllers.GetWorkAuditStudentList)
		reg.GET("/detail",
			middleware.RequirePermission("reg:audit:detail"),
			controllers.GetRegDetail)
		reg.PUT("/audit",
			middleware.RequirePermission("reg:audit:update"),
			controllers.AuditRegister)
		reg.GET("/status",
			middleware.RequirePermission("reg:status"),
			controllers.GetMyRegStatus) // 查状态
		reg.PUT("/resubmit",
			middleware.RequirePermission("reg:resubmit"),
			controllers.ResubmitRegistration) // 重新提交
		reg.GET("/my-reg",
			middleware.RequirePermission("reg:my-reg"),
			controllers.GetMyRegList)
		reg.PUT("/work-submit",
			middleware.RequirePermission("reg:my-reg:submit"),
			controllers.SubmitWork)
		reg.GET("/user/list",
			middleware.RequirePermission("reg:user:list"),
			controllers.GetUserList)
	}

	award := r.Group("/api/award", middleware.AuthRequired())
	{
		award.GET("/list",
			controllers.GetAwardCompList,
			middleware.RequirePermission("award:list"))
		award.GET("/comp-awards",
			controllers.GetCompAwards,
			middleware.RequirePermission("award:comp:list"))
		award.POST("/import",
			controllers.ImportAward,
			middleware.RequirePermission("award:import"))
		award.GET("/export-template",
			controllers.ExportAwardTemplate)
		award.GET("/comp/list",
			controllers.SearchCompetition)
		award.GET("/student/my-awards",
			middleware.RequirePermission("award:student:my-list"),
			controllers.GetStudentMyAwardList)
		award.POST("/student/supplement",
			controllers.SubmitStudentAwardSupplement)
		//middleware.RequirePermission("award:student:supplement") // 权限标识自定义
		award.GET("/audit/list",
			controllers.GetAwardAuditList)
		award.GET("/audit/detail/:id",
			controllers.GetAwardAuditDetail)
		award.PUT("/audit/:id/pass",
			controllers.PassAwardAudit)
		award.PUT("/audit/:id/reject",
			controllers.RejectAwardAudit)
		award.PUT("/audit/batch/pass",
			controllers.BatchPassAwardAudit)
		award.PUT("/audit/batch/reject",
			controllers.BatchRejectAwardAudit)
	}

	summary := r.Group("/api/summary", middleware.AuthRequired())
	{
		summary.GET("/list",
			controllers.GetSummaryList,
			// middleware.RequirePermission("summary:list")
		)
		summary.GET("/:id",
			controllers.GetSummaryDetail,
			// middleware.RequirePermission("summary:detail")
		)
		summary.POST("/:id",
			controllers.SaveSummary,
			// middleware.RequirePermission("summary:edit")
		)
	}

	statistics := r.Group("/api/statistics", middleware.AuthRequired())
	{
		statistics.GET("/dashboard", controllers.GetStatisticsDashboard)
	}
}
