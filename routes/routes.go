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

	// r.Static("/static", "./static")
	// 2026-06: 注释掉 /static 公开访问，全部走 /api/file/download 受控接口
	// 彻底迁移后再删这行

	// 受控文件下载/预览接口（需登录 + 业务权限）
	file := r.Group("/api/file", middleware.AuthRequired())
	{
		file.GET("/download/:type/:filename", controllers.DownloadFile)
	}

	// CAS/OAuth2.0 统一认证接口（公开）
	cas := r.Group("/api/cas")
	{
		cas.GET("/login", controllers.CasLogin)
		cas.GET("/callback", controllers.CasCallback)
	}

	upload := r.Group("/api/upload", middleware.AuthRequired())
	{
		upload.POST("", controllers.UploadFile)
	}

	apiGroup := r.Group("/api")
	{
		apiGroup.POST("/login", controllers.Login)

		apiGroup.GET("/college/list", controllers.GetCollegeList)
		apiGroup.GET("/department/list", controllers.GetDepartmentList)
	}

	notice := r.Group("/api/notice", middleware.AuthRequired())
	{
		notice.GET("/list",
			middleware.RequirePermission("notice:list"),
			controllers.GetNoticeList) // 通知列表+筛选
		notice.GET("/:id",
			middleware.RequirePermission("notice:detail"),
			controllers.GetNoticeDetail) // 单个通知查看
		notice.POST("/comp/create",
			middleware.RequirePermission("notice:create"),
			controllers.CreateCompNotice)
		notice.PUT("/:id/publish",
			middleware.RequirePermission("notice:publish"),
			controllers.PublishNotice)
		notice.PUT("/:id",
			middleware.RequirePermission("notice:update"),
			controllers.UpdateNotice)
		notice.DELETE("/:id",
			middleware.RequirePermission("notice:delete"),
			controllers.DeleteNotice)
	}

	perm := r.Group("/api/perm", middleware.AuthRequired())
	{
		perm.GET("/permission/list",
			middleware.RequirePermission("perm:list"),
			controllers.GetAllPermissions)
		perm.GET("/role/list",
			middleware.RequirePermission("role:list"),
			controllers.GetAllRoles)
		perm.POST("/role/assign_perm",
			middleware.RequirePermission("perm:assign"),
			controllers.AssignPermissions)
		perm.GET("/member/list",
			middleware.RequireRole("school_admin"),
			middleware.RequirePermission("user:list"),
			controllers.GetAllUsers)
		perm.PUT("/member/:id/role",
			middleware.RequireRole("school_admin"),
			middleware.RequirePermission("user:assign_role"),
			controllers.AssignUserRole)
	}

	comp := r.Group("/api/comp", middleware.AuthRequired())
	{
		comp.GET("/list",
			middleware.RequirePermission("comp:list"),
			controllers.GetCompetitionList)
		comp.POST("/create",
			middleware.RequirePermission("comp:create"),
			controllers.CreateCompetition)
		comp.POST("/batch-import",
			middleware.RequirePermission("comp:batch-import"),
			controllers.BatchImportCompetition)
		comp.DELETE("/:id",
			middleware.RequirePermission("comp:delete"),
			controllers.DeleteCompetition)
		comp.POST("/batch-delete",
			middleware.RequirePermission("comp:batch-delete"),
			controllers.BatchDeleteCompetition)
		comp.PUT("/:id/restore",
			middleware.RequirePermission("comp:restore"),
			controllers.RestoreCompetition)
		comp.GET("/manager/list",
			middleware.RequirePermission("manager:list"),
			controllers.GetManagerList)
		comp.GET("/years",
			middleware.RequirePermission("comp:years:list"),
			controllers.GetCompetitionYears)
		comp.GET("/:id",
			middleware.RequirePermission("comp:detail"),
			controllers.GetCompetitionDetail)
		comp.PUT("/:id",
			middleware.RequirePermission("comp:update"),
			controllers.UpdateCompetition)
	}

	// 赛事申报接口（院级管理员申报）
	declare := r.Group("/api/declare", middleware.AuthRequired())
	{
		// 院级申报
		declare.POST("",
			middleware.RequirePermission("declare:create"),
			controllers.CreateDeclare) // 创建申报
		declare.GET("/:id",
			middleware.RequirePermission("declare:get"),
			controllers.GetDeclareDetail) // 获取申报详情
		declare.PUT("/:id",
			middleware.RequirePermission("declare:update"),
			controllers.UpdateDeclare) // 更新申报信息
		declare.POST("/:id/submit",
			middleware.RequirePermission("declare:submit"),
			controllers.SubmitDeclare) // 提交申报
		declare.POST("/:id/revoke",
			middleware.RequirePermission("declare:revoke"),
			controllers.RevokeDeclare) // 撤回申报
		declare.GET("/my/list",
			middleware.RequirePermission("declare:list"),
			controllers.GetMyDeclares) // 获取我的申报列表
		declare.GET("/my/pending",
			middleware.RequirePermission("declare:list"),
			controllers.GetMyPendingDeclares) // 获取我的待审核申报
		declare.GET("/my/published",
			middleware.RequirePermission("declare:list"),
			controllers.GetMyPublishedDeclares) // 获取我的已发布申报
		declare.DELETE("/:id",
			middleware.RequirePermission("declare:delete"),
			controllers.DeleteDeclare) // 删除申报

		// 校级审核
		declare.GET("/pending/list",
			middleware.RequirePermission("declare:pending-list"),
			controllers.GetPendingDeclares) // 获取待审核申报
		declare.GET("/audited/list",
			middleware.RequirePermission("declare:audited-list"),
			controllers.GetAuditedDeclares) // 获取已审核申报
		declare.POST("/audit",
			middleware.RequirePermission("declare:audit"),
			controllers.AuditDeclare) // 审核申报
		declare.GET("/all",
			middleware.RequirePermission("declare:all-declares"),
			controllers.GetAllDeclares) // 获取所有申报
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
			middleware.RequirePermission("award:list"),
			controllers.GetAwardCompList)
		award.GET("/comp-awards",
			middleware.RequirePermission("award:comp:list"),
			controllers.GetCompAwards)
		award.POST("/import",
			middleware.RequirePermission("award:import"),
			controllers.ImportAward)
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
			middleware.RequirePermission("award:audit"),
			controllers.PassAwardAudit)
		award.PUT("/audit/:id/reject",
			middleware.RequirePermission("award:audit"),
			controllers.RejectAwardAudit)
		award.PUT("/audit/batch/pass",
			middleware.RequirePermission("award:audit"),
			controllers.BatchPassAwardAudit)
		award.PUT("/audit/batch/reject",
			middleware.RequirePermission("award:audit"),
			controllers.BatchRejectAwardAudit)
	}

	summary := r.Group("/api/summary", middleware.AuthRequired())
	{
		summary.GET("/list",
			middleware.RequirePermission("summary:list"),
			controllers.GetSummaryList,
		)
		summary.GET("/:id",
			middleware.RequirePermission("summary:detail"),
			controllers.GetSummaryDetail,
		)
		summary.POST("/:id",
			middleware.RequirePermission("summary:edit"),
			controllers.SaveSummary,
		)
	}

	statistics := r.Group("/api/statistics", middleware.AuthRequired())
	{
		statistics.GET("/dashboard", controllers.GetStatisticsDashboard)
	}

	review := r.Group("/api/review", middleware.AuthRequired())
	{
		// 管理员路由
		review.GET("/comp/list", middleware.RequirePermission("review:comp:list"), controllers.GetReviewCompList)
		review.GET("/expert/list", middleware.RequirePermission("review:expert:list"), controllers.GetExpertList)
		review.GET("/task/list", middleware.RequirePermission("review:task:list"), controllers.GetReviewTaskList)
		review.POST("/task/assign", middleware.RequirePermission("review:task:assign"), controllers.AssignReviewTask)
		review.POST("/task/init", middleware.RequirePermission("review:task:init"), controllers.InitReviewTasks)
		review.DELETE("/task/:id", middleware.RequirePermission("review:task:delete"), controllers.DeleteReviewTask)
		review.GET("/progress", middleware.RequirePermission("review:progress"), controllers.GetReviewProgress)
		review.GET("/result/list", middleware.RequirePermission("review:result:list"), controllers.GetReviewResultList)
		review.POST("/result/confirm", middleware.RequirePermission("review:result:confirm"), controllers.ConfirmReviewResult)

		// 专家路由
		review.GET("/my/tasks", middleware.RequirePermission("review:my:list"), controllers.GetMyReviewTasks)
		review.GET("/my/works", middleware.RequirePermission("review:my:works"), controllers.GetMyReviewWorks)
		review.GET("/my/works/:regId", middleware.RequirePermission("review:my:work:detail"), controllers.GetReviewWorkDetail)
		review.POST("/submit", middleware.RequirePermission("review:my:submit"), controllers.SubmitReview)
		review.PUT("/submit/:id", middleware.RequirePermission("review:my:update"), controllers.UpdateReview)
	}
}
