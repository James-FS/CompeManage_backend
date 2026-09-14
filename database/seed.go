package database

import (
	"CompeManage_backend/models"
	"log"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func hashPassword(password string) string {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("密码加密失败: %v", err)
		return password
	}
	return string(hashedPassword)
}

func grantRolePermissionsByCode(roleCode string, permCodes []string) {
	if len(permCodes) == 0 {
		return
	}

	var role models.Role
	if err := DB.Where("role_code = ?", roleCode).First(&role).Error; err != nil {
		log.Printf("未找到角色 %s，跳过权限补齐: %v", roleCode, err)
		return
	}

	var perms []*models.Permission
	if err := DB.Where("code IN ?", permCodes).Find(&perms).Error; err != nil {
		log.Printf("查询权限失败(%s): %v", roleCode, err)
		return
	}

	if len(perms) == 0 {
		log.Printf("未找到可补齐权限(%s): %v", roleCode, permCodes)
		return
	}

	if err := DB.Model(&role).Association("Permissions").Append(perms); err != nil {
		log.Printf("补齐权限失败(%s): %v", roleCode, err)
		return
	}

	log.Printf("已补齐角色 %s 的通知权限", roleCode)
}

func ensureNoticeManagePermissions() {
	noticeManagePerms := []string{"notice:create", "notice:publish", "notice:delete"}
	grantRolePermissionsByCode("competition_manager", noticeManagePerms)
	grantRolePermissionsByCode("college_admin", noticeManagePerms)
	grantRolePermissionsByCode("school_admin", noticeManagePerms)
}

func ensureCompetitionCorePermissions() {
	var compParent models.Permission
	if err := DB.Where("code = ?", "comp").First(&compParent).Error; err != nil {
		log.Printf("未找到权限目录 comp，跳过竞赛核心权限补齐: %v", err)
		return
	}

	ensurePerm := func(code, name, desc string) {
		var existed models.Permission
		if err := DB.Where("code = ?", code).First(&existed).Error; err == nil {
			return
		}

		perm := models.Permission{
			Name:        name,
			Code:        code,
			Type:        3,
			ParentID:    compParent.ID,
			Description: desc,
		}
		if err := DB.Create(&perm).Error; err != nil {
			log.Printf("补齐权限失败(%s): %v", code, err)
		}
	}

	ensurePerm("comp:detail", "查看竞赛详情", "查看竞赛详情")
	ensurePerm("comp:update", "编辑竞赛", "更新竞赛信息")

	grantRolePermissionsByCode("school_admin", []string{"comp:detail", "comp:update"})
	grantRolePermissionsByCode("college_admin", []string{"comp:detail"})
	grantRolePermissionsByCode("competition_manager", []string{"comp:detail", "comp:update"})
}

func ensureAwardStudentPermissions() {
	var awardParent models.Permission
	if err := DB.Where("code = ?", "award").First(&awardParent).Error; err != nil {
		log.Printf("未找到权限目录 award，跳过学生获奖权限补齐: %v", err)
		return
	}

	ensurePerm := func(code, name, desc string) {
		var existed models.Permission
		if err := DB.Where("code = ?", code).First(&existed).Error; err == nil {
			return
		}

		perm := models.Permission{
			Name:        name,
			Code:        code,
			Type:        3,
			ParentID:    awardParent.ID,
			Description: desc,
		}
		if err := DB.Create(&perm).Error; err != nil {
			log.Printf("补齐权限失败(%s): %v", code, err)
		}
	}

	ensurePerm("award:student:my-list", "查看我的获奖", "学生查看个人获奖申报列表")
	ensurePerm("award:student:supplement", "学生补录获奖", "学生提交获奖补录")

	grantRolePermissionsByCode("student", []string{"award:student:my-list", "award:student:supplement"})
}

func ensureDeclarePermissions() {
	var declareParent models.Permission
	if err := DB.Where("code = ?", "declare").First(&declareParent).Error; err != nil {
		log.Printf("未找到权限目录 declare，跳过申报权限补齐: %v", err)
		return
	}

	ensurePerm := func(code, name, desc string) {
		var existed models.Permission
		if err := DB.Where("code = ?", code).First(&existed).Error; err == nil {
			return
		}

		perm := models.Permission{
			Name:        name,
			Code:        code,
			Type:        3,
			ParentID:    declareParent.ID,
			Description: desc,
		}
		if err := DB.Create(&perm).Error; err != nil {
			log.Printf("补齐权限失败(%s): %v", code, err)
		}
	}

	ensurePerm("declare:audited-list", "查看已审核申报", "查看已审核的申报记录")
	ensurePerm("declare:revoke", "撤回申报", "撤回已提交的申报")

	grantRolePermissionsByCode("school_admin", []string{"declare:audited-list", "declare:revoke"})
	grantRolePermissionsByCode("college_admin", []string{"declare:audited-list", "declare:revoke"})
	grantRolePermissionsByCode("competition_manager", []string{"declare:audited-list"})
}

func ensureReviewPermissions() {
	var reviewParent models.Permission
	if err := DB.Where("code = ?", "review").First(&reviewParent).Error; err != nil {
		// 创建 review 目录权限
		reviewParent = models.Permission{
			Name:        "专家评审",
			Code:        "review",
			Type:        1,
			ParentID:    0,
			Description: "专家评审相关功能",
		}
		if err := DB.Create(&reviewParent).Error; err != nil {
			log.Printf("创建 review 权限目录失败: %v", err)
			return
		}
		log.Println("已创建 review 权限目录")
	}

	ensurePerm := func(code, name, desc string) {
		var existed models.Permission
		if err := DB.Where("code = ?", code).First(&existed).Error; err == nil {
			return
		}

		perm := models.Permission{
			Name:        name,
			Code:        code,
			Type:        3,
			ParentID:    reviewParent.ID,
			Description: desc,
		}
		if err := DB.Create(&perm).Error; err != nil {
			log.Printf("补齐权限失败(%s): %v", code, err)
		}
	}

	// 管理员评审权限
	ensurePerm("review:comp:list", "评审赛事列表", "查看评审赛事列表")
	ensurePerm("review:expert:list", "获取专家列表", "获取可选的评审专家列表")
	ensurePerm("review:task:list", "查看评审任务", "查看评审任务列表")
	ensurePerm("review:task:assign", "分配评审任务", "分配评审专家")
	ensurePerm("review:task:init", "初始化评审任务", "初始化评审任务")
	ensurePerm("review:task:delete", "删除评审任务", "删除评审任务")
	ensurePerm("review:progress", "查看评审进度", "查看评审进度")
	ensurePerm("review:result:list", "查看评审结果", "查看评审结果汇总")
	ensurePerm("review:result:confirm", "确认评审结果", "确认评审结果生成获奖")

	// 专家评审权限
	ensurePerm("review:my:list", "我的评审任务", "查看我的评审任务列表")
	ensurePerm("review:my:works", "待评审作品", "查看待评审作品列表")
	ensurePerm("review:my:work:detail", "作品详情", "查看作品详情")
	ensurePerm("review:my:submit", "提交评审", "提交评审打分")
	ensurePerm("review:my:update", "修改评审", "修改已提交的评审结果")

	// 为已有角色补齐评审权限
	grantRolePermissionsByCode("school_admin", []string{
		"review:comp:list", "review:expert:list", "review:task:list", "review:task:assign",
		"review:task:init", "review:task:delete", "review:progress", "review:result:list",
		"review:result:confirm", "review:my:list", "review:my:works", "review:my:work:detail",
		"review:my:submit", "review:my:update",
	})
	grantRolePermissionsByCode("college_admin", []string{
		"review:comp:list", "review:expert:list", "review:task:list", "review:task:assign",
		"review:task:init", "review:task:delete", "review:progress", "review:result:list",
		"review:result:confirm",
	})
	grantRolePermissionsByCode("competition_manager", []string{
		"review:comp:list", "review:expert:list", "review:task:list", "review:task:assign",
		"review:task:init", "review:task:delete", "review:progress", "review:result:list",
		"review:result:confirm",
	})
	grantRolePermissionsByCode("expert", []string{
		"review:my:list", "review:my:works", "review:my:work:detail", "review:my:submit",
		"review:my:update",
	})
}

func ensureUserManagePermissions() {
	var permParent models.Permission
	if err := DB.Where("code = ?", "perm").First(&permParent).Error; err != nil {
		log.Printf("未找到权限目录 perm，跳过用户管理权限补齐: %v", err)
		return
	}

	definitions := []models.Permission{
		{Name: "查看用户列表", Code: "user:list", Type: 3, ParentID: permParent.ID, Description: "查看系统用户、角色和管理学院"},
		{Name: "分配用户角色", Code: "user:assign_role", Type: 3, ParentID: permParent.ID, Description: "修改用户角色和院管理员管理学院"},
	}
	for _, definition := range definitions {
		var existing models.Permission
		if err := DB.Where("code = ?", definition.Code).First(&existing).Error; err == nil {
			continue
		}
		if err := DB.Create(&definition).Error; err != nil {
			log.Printf("补齐用户管理权限失败(%s): %v", definition.Code, err)
		}
	}
	grantRolePermissionsByCode("school_admin", []string{"user:list", "user:assign_role"})
}

// ensureTestUsers 确保测试账号存在（用于人工登录测试）
func ensureTestUsers() {
	type testUser struct {
		User     models.User
		RoleCode string
	}

	testUsers := []testUser{
		{User: models.User{Username: "900001", Realname: "测试校管理员", Password: hashPassword("Pw@yy03"), College: "教务处", Grade: "教职员工", Major: "管理", IdentityType: "staff"}, RoleCode: "school_admin"},
		{User: models.User{Username: "900006", Realname: "测试学生", Password: hashPassword("Pw@yy03"), College: "计算机科学与网络工程学院", Grade: "2024级", Major: "计算机科学与技术", IdentityType: "student"}, RoleCode: "student"},
	}

	for _, tu := range testUsers {
		var existing models.User
		if DB.Where("username = ?", tu.User.Username).First(&existing).Error == nil {
			continue
		}
		var role models.Role
		if DB.Where("role_code = ?", tu.RoleCode).First(&role).Error != nil {
			log.Printf("未找到角色 %s，跳过测试用户 %s 角色分配", tu.RoleCode, tu.User.Username)
			continue
		}
		if err := DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&tu.User).Error; err != nil {
				return err
			}
			return SetUserRole(tx, tu.User.ID, role.ID)
		}); err != nil {
			log.Printf("创建测试用户 %s 失败: %v", tu.User.Username, err)
			continue
		}
		log.Printf("已创建测试用户: %s (角色: %s)", tu.User.Username, tu.RoleCode)
	}
}

// ensureExpertUsers 确保种子专家用户存在（增量更新时不会丢失新增的专家账号）
func ensureExpertUsers() {
	var expertRole models.Role
	if err := DB.Where("role_code = ?", "expert").First(&expertRole).Error; err != nil {
		return
	}

	experts := []models.User{
		{Username: "E2023001", Realname: "周杰", Password: hashPassword("123"), College: "电子信息工程学院", Grade: "教职员工", Major: "电子工程", IdentityType: "external"},
		{Username: "E2023002", Realname: "李专家", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "教职员工", Major: "计算机科学与技术", IdentityType: "external"},
	}

	for _, u := range experts {
		var existing models.User
		if DB.Where("username = ?", u.Username).First(&existing).Error == nil {
			continue
		}
		if err := DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&u).Error; err != nil {
				return err
			}
			return SetUserRole(tx, u.ID, expertRole.ID)
		}); err != nil {
			log.Printf("创建专家用户 %s 失败: %v", u.Username, err)
			continue
		}
		log.Printf("已补创建专家用户: %s", u.Username)
	}
}

// ensureCompetitionRegConfigForAll 为已有赛事补齐报名设置的关键字段：
// 报名开始/结束时间 + 个人赛/团队赛(ParticipantType)。
// InitData 扩展版测试数据初始化
// 功能：
// 1. 扩充竞赛数据（新增更多竞赛信息）
// 2. 每个角色至少对应一个账号
// 3. 自动注入 routes 中 RequirePermission 中间件配置的权限
// 4. 竞赛负责人自动获取对应路由权限
func InitData() {
	// 1. 先检查是否已经有数据了，防止重复插入
	var count int64
	DB.Model(&models.User{}).Count(&count)
	if count > 0 {
		log.Println("数据库已有数据，跳过初始化...")
		ensureNoticeManagePermissions()
		ensureCompetitionCorePermissions()
		ensureAwardStudentPermissions()
		ensureDeclarePermissions()
		ensureReviewPermissions()
		ensureUserManagePermissions()
		ensureExpertUsers()
		ensureTestUsers()
		return
	}

	log.Println("正在初始化扩展版模拟数据...")

	// ==========================================
	// 1. 初始化权限 (Permission) - 按SQL结构构建树形
	// ==========================================

	// --- Level 1: 大分类（顶级父目录）Type=1, ParentID=0 ---
	competitionDir := models.Permission{Name: "竞赛管理", Code: "competition", Type: 1, ParentID: 0, Description: "竞赛、申报、获奖相关功能"}
	registrationDir := models.Permission{Name: "报名管理", Code: "registration", Type: 1, ParentID: 0, Description: "报名配置、审核、提交相关功能"}
	noticeDir := models.Permission{Name: "通知管理", Code: "notice", Type: 1, ParentID: 0, Description: "通知发布和管理功能"}
	systemDir := models.Permission{Name: "系统管理", Code: "system", Type: 1, ParentID: 0, Description: "权限、角色、基础数据管理"}
	reviewDir := models.Permission{Name: "专家评审", Code: "review", Type: 1, ParentID: 0, Description: "专家评审相关功能"}

	DB.Create(&competitionDir)
	DB.Create(&registrationDir)
	DB.Create(&noticeDir)
	DB.Create(&systemDir)
	DB.Create(&reviewDir)

	// --- Level 2: 子分类（中间层父目录）Type=1(作为目录), ParentID=Level1.ID ---

	// 竞赛管理下的子分类
	compSub := models.Permission{Name: "竞赛目录", Code: "comp", Type: 1, ParentID: competitionDir.ID, Description: "竞赛基础数据管理"}
	declareSub := models.Permission{Name: "赛事申报", Code: "declare", Type: 1, ParentID: competitionDir.ID, Description: "赛事申报审核管理"}
	awardSub := models.Permission{Name: "获奖管理", Code: "award", Type: 1, ParentID: competitionDir.ID, Description: "获奖信息导入和管理"}
	summarySub := models.Permission{Name: "赛事总结", Code: "summary", Type: 1, ParentID: competitionDir.ID, Description: "赛事总结填写与归档"}

	// 报名管理下的子分类
	regConfigSub := models.Permission{Name: "报名配置", Code: "reg:config", Type: 1, ParentID: registrationDir.ID, Description: "报名时间、规则配置"}
	regAuditSub := models.Permission{Name: "报名审核", Code: "reg:audit", Type: 1, ParentID: registrationDir.ID, Description: "报名信息审核"}
	regSubmitSub := models.Permission{Name: "报名提交", Code: "reg:submit", Type: 1, ParentID: registrationDir.ID, Description: "学生报名和作品提交"}

	// 系统管理下的子分类
	permSub := models.Permission{Name: "权限管理", Code: "perm", Type: 1, ParentID: systemDir.ID, Description: "权限和角色配置"}
	basicSub := models.Permission{Name: "基础数据", Code: "basic", Type: 1, ParentID: systemDir.ID, Description: "学院、文件上传等基础数据"}

	DB.Create(&compSub)
	DB.Create(&declareSub)
	DB.Create(&awardSub)
	DB.Create(&summarySub)
	DB.Create(&regConfigSub)
	DB.Create(&regAuditSub)
	DB.Create(&regSubmitSub)
	DB.Create(&permSub)
	DB.Create(&basicSub)

	// --- Level 3: 具体权限（叶子节点）Type=3(API/按钮), ParentID=Level2.ID ---
	// 注意：通知管理下的权限直接挂到 noticeDir 下，没有中间层

	perms := []models.Permission{
		// 竞赛目录权限 (parent: compSub)
		{Name: "查看竞赛列表", Code: "comp:list", Type: 3, ParentID: compSub.ID, Description: "查看竞赛目录列表"},
		{Name: "创建竞赛", Code: "comp:create", Type: 3, ParentID: compSub.ID, Description: "创建新的竞赛"},
		{Name: "批量导入竞赛", Code: "comp:batch-import", Type: 3, ParentID: compSub.ID, Description: "批量导入竞赛"},
		{Name: "删除竞赛", Code: "comp:delete", Type: 3, ParentID: compSub.ID, Description: "删除竞赛信息"},
		{Name: "批量删除竞赛", Code: "comp:batch-delete", Type: 3, ParentID: compSub.ID, Description: "批量删除竞赛"},
		{Name: "恢复竞赛", Code: "comp:restore", Type: 3, ParentID: compSub.ID, Description: "恢复已删除的竞赛"},
		{Name: "查看竞赛详情", Code: "comp:detail", Type: 3, ParentID: compSub.ID, Description: "查看竞赛详情"},
		{Name: "编辑竞赛", Code: "comp:update", Type: 3, ParentID: compSub.ID, Description: "更新竞赛信息"},
		{Name: "查看竞赛年份", Code: "comp:years:list", Type: 3, ParentID: compSub.ID, Description: "查看竞赛年份列表"},
		{Name: "查看赛事负责人", Code: "manager:list", Type: 3, ParentID: compSub.ID, Description: "查看赛事负责人列表"},

		// 赛事总结权限 (parent: summarySub)
		{Name: "查看总结列表", Code: "summary:list", Type: 3, ParentID: summarySub.ID, Description: "查看赛事总结列表"},
		{Name: "查看总结详情", Code: "summary:detail", Type: 3, ParentID: summarySub.ID, Description: "查看赛事总结详情"},
		{Name: "编辑赛事总结", Code: "summary:edit", Type: 3, ParentID: summarySub.ID, Description: "填写/归档赛事总结"},

		// 赛事申报权限 (parent: declareSub)
		{Name: "创建申报", Code: "declare:create", Type: 3, ParentID: declareSub.ID, Description: "创建新的赛事申报"},
		{Name: "查看申报详情", Code: "declare:get", Type: 3, ParentID: declareSub.ID, Description: "查看申报信息详情"},
		{Name: "编辑申报", Code: "declare:update", Type: 3, ParentID: declareSub.ID, Description: "编辑申报信息"},
		{Name: "提交申报", Code: "declare:submit", Type: 3, ParentID: declareSub.ID, Description: "提交赛事申报"},
		{Name: "查看我的申报", Code: "declare:list", Type: 3, ParentID: declareSub.ID, Description: "查看自己的申报列表"},
		{Name: "删除申报", Code: "declare:delete", Type: 3, ParentID: declareSub.ID, Description: "删除申报信息"},
		{Name: "查看待审核申报", Code: "declare:pending-list", Type: 3, ParentID: declareSub.ID, Description: "查看待审核申报列表"},
		{Name: "审核申报", Code: "declare:audit", Type: 3, ParentID: declareSub.ID, Description: "审核赛事申报"},
		{Name: "查看已审核申报", Code: "declare:audited-list", Type: 3, ParentID: declareSub.ID, Description: "查看已审核的申报记录"},
		{Name: "撤回申报", Code: "declare:revoke", Type: 3, ParentID: declareSub.ID, Description: "撤回已提交的申报"},
		{Name: "查看所有申报", Code: "declare:all-declares", Type: 3, ParentID: declareSub.ID, Description: "查看所有申报信息"},

		// 获奖管理权限 (parent: awardSub)
		{Name: "查看获奖赛事列表", Code: "award:list", Type: 3, ParentID: awardSub.ID, Description: "查看获奖赛事列表"},
		{Name: "查看赛事获奖信息", Code: "award:comp:list", Type: 3, ParentID: awardSub.ID, Description: "查看具体赛事的获奖信息"},
		{Name: "导入获奖信息", Code: "award:import", Type: 3, ParentID: awardSub.ID, Description: "导入获奖信息"},
		{Name: "查看我的获奖", Code: "award:student:my-list", Type: 3, ParentID: awardSub.ID, Description: "学生查看个人获奖申报列表"},
		{Name: "学生补录获奖", Code: "award:student:supplement", Type: 3, ParentID: awardSub.ID, Description: "学生提交获奖补录"},

		// 报名配置权限 (parent: regConfigSub)
		{Name: "编辑报名配置", Code: "reg:config:edit", Type: 3, ParentID: regConfigSub.ID, Description: "编辑报名时间和规则配置"},
		{Name: "查看报名配置", Code: "reg:config:view", Type: 3, ParentID: regConfigSub.ID, Description: "查看报名配置"},

		// 报名审核权限 (parent: regAuditSub)
		{Name: "查看报名列表", Code: "reg:audit:list", Type: 3, ParentID: regAuditSub.ID, Description: "查看报名信息列表"},
		{Name: "查看报名详情", Code: "reg:audit:detail", Type: 3, ParentID: regAuditSub.ID, Description: "查看报名详细信息"},
		{Name: "审核报名", Code: "reg:audit:update", Type: 3, ParentID: regAuditSub.ID, Description: "审核报名信息（通过/驳回）"},

		// 报名提交权限 (parent: regSubmitSub)
		{Name: "学生报名", Code: "reg:config:submit", Type: 3, ParentID: regSubmitSub.ID, Description: "学生提交报名信息"},
		{Name: "查看报名状态", Code: "reg:status", Type: 3, ParentID: regSubmitSub.ID, Description: "查看个人报名状态"},
		{Name: "重新提交报名", Code: "reg:resubmit", Type: 3, ParentID: regSubmitSub.ID, Description: "驳回后重新提交报名"},
		{Name: "查看我的报名", Code: "reg:my-reg", Type: 3, ParentID: regSubmitSub.ID, Description: "查看个人报名信息"},
		{Name: "提交作品", Code: "reg:my-reg:submit", Type: 3, ParentID: regSubmitSub.ID, Description: "提交参赛作品"},
		{Name: "查看学生列表", Code: "reg:user:list", Type: 3, ParentID: regSubmitSub.ID, Description: "选取学生填入报名信息"},

		// 通知管理权限 (parent: noticeDir，直接挂在大分类下)
		{Name: "查看通知列表", Code: "notice:list", Type: 3, ParentID: noticeDir.ID, Description: "查看通知列表"},
		{Name: "查看通知详情", Code: "notice:detail", Type: 3, ParentID: noticeDir.ID, Description: "查看通知详细内容"},
		{Name: "创建通知", Code: "notice:create", Type: 3, ParentID: noticeDir.ID, Description: "创建新通知"},
		{Name: "发布通知", Code: "notice:publish", Type: 3, ParentID: noticeDir.ID, Description: "发布通知"},
		{Name: "删除通知", Code: "notice:delete", Type: 3, ParentID: noticeDir.ID, Description: "删除通知"},

		// 权限管理权限 (parent: permSub)
		{Name: "查看权限列表", Code: "perm:list", Type: 3, ParentID: permSub.ID, Description: "查看系统权限列表"},
		{Name: "查看角色列表", Code: "role:list", Type: 3, ParentID: permSub.ID, Description: "查看系统角色列表"},
		{Name: "分配权限", Code: "perm:assign", Type: 3, ParentID: permSub.ID, Description: "给角色分配权限"},
		{Name: "查看用户列表", Code: "user:list", Type: 3, ParentID: permSub.ID, Description: "查看系统用户、角色和管理学院"},
		{Name: "分配用户角色", Code: "user:assign_role", Type: 3, ParentID: permSub.ID, Description: "修改用户角色和院管理员管理学院"},

		// 基础数据权限 (parent: basicSub)
		{Name: "查看学院列表", Code: "college:list", Type: 3, ParentID: basicSub.ID, Description: "查看学院列表"},
		{Name: "文件上传", Code: "upload:file", Type: 3, ParentID: basicSub.ID, Description: "上传文件"},

		// 专家评审权限 (parent: reviewDir)
		{Name: "评审赛事列表", Code: "review:comp:list", Type: 3, ParentID: reviewDir.ID, Description: "查看评审赛事列表"},
		{Name: "获取专家列表", Code: "review:expert:list", Type: 3, ParentID: reviewDir.ID, Description: "获取可选的评审专家列表"},
		{Name: "查看评审任务", Code: "review:task:list", Type: 3, ParentID: reviewDir.ID, Description: "查看评审任务列表"},
		{Name: "分配评审任务", Code: "review:task:assign", Type: 3, ParentID: reviewDir.ID, Description: "分配评审专家"},
		{Name: "初始化评审任务", Code: "review:task:init", Type: 3, ParentID: reviewDir.ID, Description: "初始化评审任务"},
		{Name: "删除评审任务", Code: "review:task:delete", Type: 3, ParentID: reviewDir.ID, Description: "删除评审任务"},
		{Name: "查看评审进度", Code: "review:progress", Type: 3, ParentID: reviewDir.ID, Description: "查看评审进度"},
		{Name: "查看评审结果", Code: "review:result:list", Type: 3, ParentID: reviewDir.ID, Description: "查看评审结果汇总"},
		{Name: "确认评审结果", Code: "review:result:confirm", Type: 3, ParentID: reviewDir.ID, Description: "确认评审结果生成获奖"},
		{Name: "我的评审任务", Code: "review:my:list", Type: 3, ParentID: reviewDir.ID, Description: "查看我的评审任务列表"},
		{Name: "待评审作品", Code: "review:my:works", Type: 3, ParentID: reviewDir.ID, Description: "查看待评审作品列表"},
		{Name: "作品详情", Code: "review:my:work:detail", Type: 3, ParentID: reviewDir.ID, Description: "查看作品详情"},
		{Name: "提交评审", Code: "review:my:submit", Type: 3, ParentID: reviewDir.ID, Description: "提交评审打分"},
		{Name: "修改评审", Code: "review:my:update", Type: 3, ParentID: reviewDir.ID, Description: "修改已提交的评审结果"},
	}
	DB.Create(&perms)

	// ==========================================
	// 2. 初始化角色 (Role) - 保持不变
	// ==========================================
	schoolAdminRole := models.Role{RoleName: "校级管理员", RoleCode: "school_admin", Description: "校级系统管理员，拥有所有权限"}
	collegeAdminRole := models.Role{RoleName: "院级管理员", RoleCode: "college_admin", Description: "学院管理员，负责用户和审核"}
	competitionManagerRole := models.Role{RoleName: "赛事负责人", RoleCode: "competition_manager", Description: "负责发布和审核竞赛，拥有报名配置权限"}
	teacherRole := models.Role{RoleName: "老师", RoleCode: "teacher", Description: "教师角色，用于指导学生报名和担任指导老师"}
	studentRole := models.Role{RoleName: "学生", RoleCode: "student", Description: "参与竞赛"}
	expertRole := models.Role{RoleName: "专家", RoleCode: "expert", Description: "评审竞赛"}
	guestRole := models.Role{RoleName: "访客", RoleCode: "guest", Description: "访客角色，仅可浏览"}

	DB.Create(&schoolAdminRole)
	DB.Create(&collegeAdminRole)
	DB.Create(&competitionManagerRole)
	DB.Create(&teacherRole)
	DB.Create(&studentRole)
	DB.Create(&expertRole)
	DB.Create(&guestRole)

	// ==========================================
	// 3. 关联角色与权限 (Role-Permission) - 按新权限结构更新
	// ==========================================

	// --- A. 校级管理员：给所有权限 ---
	var allPerms []models.Permission
	DB.Find(&allPerms)
	DB.Model(&schoolAdminRole).Association("Permissions").Append(&allPerms)

	// --- B. 院级管理员：竞赛查看、申报审核、报名审核、基础数据查看、专家评审管理 ---
	var collegePerms []models.Permission
	DB.Where("code IN ?", []string{
		// 大分类（目录权限）
		"competition", "registration", "notice", "system", "review",
		// 子分类（目录权限）
		"comp", "declare", "award", "summary", "reg:config", "reg:audit", "basic",
		// 具体权限
		"comp:list", "comp:detail", "comp:years:list", "manager:list",
		"declare:create", "declare:get", "declare:update", "declare:submit", "declare:list", "declare:delete", "declare:pending-list", "declare:audited-list", "declare:revoke", "declare:all-declares",
		"award:list", "award:comp:list",
		"summary:list", "summary:detail",
		"reg:config:view",
		"reg:audit:list", "reg:audit:detail", "reg:audit:update",
		"notice:list", "notice:detail", "notice:create", "notice:publish", "notice:delete",
		"college:list",
		// 专家评审管理
		"review:comp:list", "review:expert:list", "review:task:list", "review:task:assign", "review:task:init", "review:task:delete", "review:progress", "review:result:list", "review:result:confirm",
	}).Find(&collegePerms)
	DB.Model(&collegeAdminRole).Association("Permissions").Append(&collegePerms)

	// --- C. 赛事负责人：竞赛目录/赛事申报仅查看，其他模块保持原有权限、专家评审管理 ---
	var managerPerms []models.Permission
	DB.Where("code IN ?", []string{
		// 大分类
		"competition", "registration", "notice", "review",
		// 子分类
		"comp", "declare", "award", "summary", "reg:config", "reg:audit", "reg:submit",
		// 竞赛目录（仅查看）
		"comp:list", "comp:detail", "comp:years:list", "manager:list",
		// 赛事申报（仅查看）
		"declare:get", "declare:list", "declare:pending-list", "declare:audited-list", "declare:all-declares",
		// 获奖管理
		"award:list", "award:comp:list", "award:import",
		// 赛事总结
		"summary:list", "summary:detail", "summary:edit",
		// 报名配置（全权限）
		"reg:config:edit", "reg:config:view",
		// 报名审核（全权限）
		"reg:audit:list", "reg:audit:detail", "reg:audit:update",
		// 通知管理
		"notice:list", "notice:detail", "notice:create", "notice:publish", "notice:delete",
		// 基础数据
		"college:list", "upload:file",
		// 专家评审管理
		"review:comp:list", "review:expert:list", "review:task:list", "review:task:assign", "review:task:init", "review:task:delete", "review:progress", "review:result:list", "review:result:confirm",
	}).Find(&managerPerms)
	DB.Model(&competitionManagerRole).Association("Permissions").Append(&managerPerms)

	// --- D. 老师：可查看竞赛/通知，供指导教师选择使用 ---
	var teacherPerms []models.Permission
	DB.Where("code IN ?", []string{
		// 大分类
		"competition", "registration", "notice",
		// 子分类
		"comp", "declare", "reg:submit",
		// 竞赛查看
		"comp:list", "comp:detail", "comp:years:list", "manager:list",
		// 申报查看
		"declare:get", "declare:list",
		// 通知查看
		"notice:list", "notice:detail",
		// 基础数据
		"college:list", "upload:file",
	}).Find(&teacherPerms)
	DB.Model(&teacherRole).Association("Permissions").Append(&teacherPerms)

	// --- E. 学生：报名提交相关、通知查看 ---
	var studentPerms []models.Permission
	DB.Where("code IN ?", []string{
		// 大分类
		"competition", "registration", "notice",
		// 子分类
		"award", "reg:submit",
		// 报名提交权限
		"reg:config:submit", "reg:status", "reg:resubmit", "reg:my-reg", "reg:my-reg:submit", "reg:user:list",
		// 学生获奖权限
		"award:student:my-list", "award:student:supplement",
		// 通知查看
		"notice:list", "notice:detail",
		// 基础数据
		"college:list", "upload:file",
	}).Find(&studentPerms)
	DB.Model(&studentRole).Association("Permissions").Append(&studentPerms)

	// --- F. 专家：竞赛查看、申报查看、获奖查看、通知查看、专家评审 ---
	var expertPerms []models.Permission
	DB.Where("code IN ?", []string{
		// 大分类
		"competition", "notice", "review",
		// 子分类
		"comp", "declare", "award",
		// 竞赛查看
		"comp:list", "comp:years:list", "manager:list",
		// 申报查看
		"declare:get", "declare:list", "declare:all-declares",
		// 获奖查看
		"award:list", "award:comp:list",
		// 通知查看
		"notice:list", "notice:detail",
		// 专家评审
		"review:my:list", "review:my:works", "review:my:work:detail", "review:my:submit", "review:my:update",
	}).Find(&expertPerms)
	DB.Model(&expertRole).Association("Permissions").Append(&expertPerms)

	ensureNoticeManagePermissions()
	ensureCompetitionCorePermissions()
	ensureAwardStudentPermissions()
	ensureUserManagePermissions()

	// --- G. 访客：无特殊权限 ---
	// 不分配任何权限

	// ==========================================
	// 4. 初始化用户 (User) - 保持不变
	// ==========================================
	users := []models.User{
		{Username: "T2023001", Realname: "李老师", Password: hashPassword("123"), College: "教务处", Grade: "教职员工", Major: "管理"},
		{Username: "T2023002", Realname: "王老师", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "教职员工", Major: "管理"},
		{Username: "T2023003", Realname: "张伟", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "教职员工", Major: "计算机科学"},
		{Username: "T2023004", Realname: "李华", Password: hashPassword("123"), College: "电子信息工程学院", Grade: "教职员工", Major: "电子信息"},
		{Username: "T2023005", Realname: "王强", Password: hashPassword("123"), College: "经济管理学院", Grade: "教职员工", Major: "经济管理"},
		{Username: "T2023006", Realname: "赵敏", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "教职员工", Major: "软件工程"},
		{Username: "T2023010", Realname: "陈老师", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "教职员工", Major: "软件工程"},
		{Username: "T2023011", Realname: "刘老师", Password: hashPassword("123"), College: "数学学院", Grade: "教职员工", Major: "数学与应用数学"},
		{Username: "T2023012", Realname: "何老师", Password: hashPassword("123"), College: "电子信息工程学院", Grade: "教职员工", Major: "电子科学与技术"},
		{Username: "S2024001", Realname: "林晓明", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "2024级", Major: "计算机科学与技术"},
		{Username: "S2024002", Realname: "陈思思", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "2024级", Major: "软件工程"},
		{Username: "E2023001", Realname: "周杰", Password: hashPassword("123"), College: "电子信息工程学院", Grade: "教职员工", Major: "电子工程"},
		{Username: "E2023002", Realname: "李专家", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "教职员工", Major: "计算机科学与技术"},
		// 新增：无权限测试用户
		{Username: "guest", Realname: "访客用户", Password: hashPassword("123"), College: "其他", Grade: "访客", Major: ""},
	}

	// 用户-角色映射
	userRoleMap := map[string]*models.Role{
		"T2023001": &schoolAdminRole,        // 李校长 - 校级管理员
		"T2023002": &collegeAdminRole,       // 王院长 - 院级管理员
		"T2023003": &competitionManagerRole, // 张伟 - 赛事负责人
		"T2023004": &competitionManagerRole, // 李华 - 赛事负责人
		"T2023005": &competitionManagerRole, // 王强 - 赛事负责人
		"T2023006": &competitionManagerRole, // 赵敏 - 赛事负责人
		"T2023010": &teacherRole,            // 陈老师 - 老师
		"T2023011": &teacherRole,            // 刘老师 - 老师
		"T2023012": &teacherRole,            // 何老师 - 老师
		"S2024001": &studentRole,            // 林晓明 - 学生
		"S2024002": &studentRole,            // 陈思思 - 学生
		"E2023001": &expertRole,             // 周杰 - 专家
		"E2023002": &expertRole,             // 李专家 - 专家
		"guest":    &guestRole,              // 访客账号使用 guest 角色
	}

	for _, u := range users {
		switch {
		case strings.HasPrefix(u.Username, "S"):
			u.IdentityType = "student"
		case strings.HasPrefix(u.Username, "E") || u.Username == "guest":
			u.IdentityType = "external"
		default:
			u.IdentityType = "staff"
		}
		role, ok := userRoleMap[u.Username]
		if !ok {
			log.Printf("用户 %s 未配置默认角色，跳过创建", u.Username)
			continue
		}
		if err := DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&u).Error; err != nil {
				return err
			}
			return SetUserRole(tx, u.ID, role.ID)
		}); err != nil {
			log.Printf("创建用户 %s 失败: %v", u.Username, err)
			continue
		}
	}

	// 初始化学院数据
	colleges := []models.College{
		{ID: 1, Name: "计算机科学与网络工程学院"},
		{ID: 2, Name: "数学学院"},
		{ID: 3, Name: "机械工程学院"},
		{ID: 4, Name: "电子信息工程学院"},
		{ID: 5, Name: "经济管理学院"},
		{ID: 6, Name: "外国语学院"},
		{ID: 7, Name: "法学院"},
		{ID: 8, Name: "艺术学院"},
		{ID: 9, Name: "体育学院"},
		{ID: 10, Name: "生命科学学院"},
	}

	// 这里建议用 Save，如果 ID 已存在则更新，不存在则创建
	for _, col := range colleges {
		if err := DB.Save(&col).Error; err != nil {
			log.Printf("初始化学院数据失败: %v", err)
		}
	}
	// 示例院管理员绑定管理学院。
	DB.Model(&models.User{}).Where("username = ?", "T2023002").
		Update("managed_college_id", uint(1))
	log.Println("学院数据初始化完成")

	log.Println("🎉 基础数据初始化完成（未写入任何比赛相关种子）！")
	log.Println("================================")
	log.Println("📋 用户账号信息：")
	log.Println("  校级管理员: T2023001    密码: 123 (拥有所有权限)")
	log.Println("  院级管理员: T2023002     密码: 123 (本学院竞赛/申报查看+报名审核+报名配置查看)")
	log.Println("  赛事负责人: T2023003  密码: 123 (竞赛管理+申报管理+报名配置编辑+报名审核)")
	log.Println("  老师:       T2023010/T2023011/T2023012  密码: 123 (用于指导教师选择)")
	log.Println("  学生:       S2024001  密码: 123 (报名提交+通知查看)")
	log.Println("  专家:       E2023001/E2023002   密码: 123 (竞赛查看+申报查看+获奖查看+通知查看)")
	log.Println("  访客:       guest    密码: 123 (无任何权限-测试用)")
	log.Println("================================")
	log.Println("🔐 新权限结构说明：")
	log.Println("  第1层: 竞赛管理(competition)、报名管理(registration)、通知管理(notice)、系统管理(system)")
	log.Println("  第2层: 竞赛目录(comp)、赛事申报(declare)、获奖管理(award)等子分类")
	log.Println("  第3层: 具体的操作权限(如 comp:list, reg:audit:update 等)")
	log.Println("================================")
	ensureTestUsers()
}
