package database

import (
	"CompeManage_backend/models"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm/clause"
)

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

func seedNotices(baseTime time.Time) {
	var noticeCount int64
	DB.Model(&models.Notice{}).Count(&noticeCount)
	if noticeCount > 0 {
		log.Printf("通知数据已存在（%d条），跳过通知初始化", noticeCount)
		return
	}

	var detailIDs []uint
	DB.Model(&models.CompDetail{}).Order("id ASC").Limit(3).Pluck("id", &detailIDs)

	getDetailID := func(index int) uint {
		if index >= 0 && index < len(detailIDs) {
			return detailIDs[index]
		}
		return 0
	}

	notices := []models.Notice{
		{
			Title:               "关于举办2026年校级程序设计竞赛的通知",
			Content:             "请各学院按要求组织报名，具体时间安排见赛事系统。",
			CompetitionDetailID: getDetailID(0),
			Status:              1,
			PublishTime:         baseTime.AddDate(0, 0, -12).Format("2006-01-02 15:04:05"),
			Attachment:          "/static/notices/programming_notice_2026.pdf",
		},
		{
			Title:               "关于开展2026年度学科竞赛报名工作的通知",
			Content:             "各学院请在报名截止前完成团队信息提交，逾期不再受理。",
			CompetitionDetailID: getDetailID(1),
			Status:              1,
			PublishTime:         baseTime.AddDate(0, 0, -7).Format("2006-01-02 15:04:05"),
			Attachment:          "/static/notices/enroll_guide_2026.docx",
		},
		{
			Title:               "蓝桥杯校赛赛前培训安排通知",
			Content:             "培训面向参赛学生开放，请按通知时间参加线上答疑。",
			CompetitionDetailID: getDetailID(2),
			Status:              1,
			PublishTime:         baseTime.AddDate(0, 0, -3).Format("2006-01-02 15:04:05"),
			Attachment:          "/static/notices/lanqiao_training_2026.pdf",
		},
		{
			Title:               "2026年大学生创新创业训练计划申报提醒",
			Content:             "项目负责人请尽快完善申报材料，系统将于月底关闭。",
			CompetitionDetailID: getDetailID(0),
			Status:              0,
			PublishTime:         "",
			Attachment:          "",
		},
		{
			Title:               "关于竞赛材料归档规范的补充说明",
			Content:             "请按统一模板上传材料，文件命名需包含学院与队伍名称。",
			CompetitionDetailID: getDetailID(1),
			Status:              1,
			PublishTime:         baseTime.AddDate(0, 0, -1).Format("2006-01-02 15:04:05"),
			Attachment:          "/static/notices/archive_rule_2026.pdf",
		},
	}

	if err := DB.Create(&notices).Error; err != nil {
		log.Printf("❌ 初始化通知数据失败: %v", err)
		return
	}

	log.Printf("✅ 初始化通知数据: %d 条", len(notices))
}

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
		seedNotices(time.Date(2026, 1, 23, 0, 0, 0, 0, time.Local))
		ensureNoticeManagePermissions()
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

	DB.Create(&competitionDir)
	DB.Create(&registrationDir)
	DB.Create(&noticeDir)
	DB.Create(&systemDir)

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
		{Name: "查看所有申报", Code: "declare:all-declares", Type: 3, ParentID: declareSub.ID, Description: "查看所有申报信息"},

		// 获奖管理权限 (parent: awardSub)
		{Name: "查看获奖赛事列表", Code: "award:list", Type: 3, ParentID: awardSub.ID, Description: "查看获奖赛事列表"},
		{Name: "查看赛事获奖信息", Code: "award:comp:list", Type: 3, ParentID: awardSub.ID, Description: "查看具体赛事的获奖信息"},
		{Name: "导入获奖信息", Code: "award:import", Type: 3, ParentID: awardSub.ID, Description: "导入获奖信息"},

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

		// 基础数据权限 (parent: basicSub)
		{Name: "查看学院列表", Code: "college:list", Type: 3, ParentID: basicSub.ID, Description: "查看学院列表"},
		{Name: "文件上传", Code: "upload:file", Type: 3, ParentID: basicSub.ID, Description: "上传文件"},
	}
	DB.Create(&perms)

	// ==========================================
	// 2. 初始化角色 (Role) - 保持不变
	// ==========================================
	schoolAdminRole := models.Role{RoleName: "校级管理员", RoleCode: "school_admin", Description: "校级系统管理员，拥有所有权限"}
	collegeAdminRole := models.Role{RoleName: "院级管理员", RoleCode: "college_admin", Description: "学院管理员，负责用户和审核"}
	competitionManagerRole := models.Role{RoleName: "赛事负责人", RoleCode: "competition_manager", Description: "负责发布和审核竞赛，拥有报名配置权限"}
	studentRole := models.Role{RoleName: "学生", RoleCode: "student", Description: "参与竞赛"}
	expertRole := models.Role{RoleName: "专家", RoleCode: "expert", Description: "评审竞赛"}
	guestRole := models.Role{RoleName: "访客", RoleCode: "guest", Description: "访客角色，仅可浏览"}

	DB.Create(&schoolAdminRole)
	DB.Create(&collegeAdminRole)
	DB.Create(&competitionManagerRole)
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

	// --- B. 院级管理员：竞赛查看、申报审核、报名审核、基础数据查看 ---
	var collegePerms []models.Permission
	DB.Where("code IN ?", []string{
		// 大分类（目录权限）
		"competition", "registration", "notice", "system",
		// 子分类（目录权限）
		"comp", "declare", "award", "summary", "reg:config", "reg:audit", "basic",
		// 具体权限
		"comp:list", "comp:years:list", "manager:list",
		"declare:get", "declare:list", "declare:pending-list", "declare:audit", "declare:all-declares",
		"award:list", "award:comp:list",
		"summary:list", "summary:detail",
		"reg:config:view",
		"reg:audit:list", "reg:audit:detail", "reg:audit:update",
		"notice:list", "notice:detail", "notice:create", "notice:publish", "notice:delete",
		"college:list",
	}).Find(&collegePerms)
	DB.Model(&collegeAdminRole).Association("Permissions").Append(&collegePerms)

	// --- C. 赛事负责人：竞赛管理、申报管理、报名配置、报名审核 ---
	var managerPerms []models.Permission
	DB.Where("code IN ?", []string{
		// 大分类
		"competition", "registration", "notice",
		// 子分类
		"comp", "declare", "award", "summary", "reg:config", "reg:audit", "reg:submit",
		// 竞赛目录（全权限）
		"comp:list", "comp:create", "comp:batch-import", "comp:delete", "comp:batch-delete", "comp:restore", "comp:years:list", "manager:list",
		// 赛事申报（除删除外）
		"declare:create", "declare:get", "declare:update", "declare:submit", "declare:list", "declare:pending-list", "declare:audit", "declare:all-declares",
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
	}).Find(&managerPerms)
	DB.Model(&competitionManagerRole).Association("Permissions").Append(&managerPerms)

	// --- D. 学生：报名提交相关、通知查看 ---
	var studentPerms []models.Permission
	DB.Where("code IN ?", []string{
		// 大分类
		"registration", "notice",
		// 子分类
		"reg:submit",
		// 报名提交权限
		"reg:config:submit", "reg:status", "reg:resubmit", "reg:my-reg", "reg:my-reg:submit", "reg:user:list",
		// 通知查看
		"notice:list", "notice:detail",
		// 基础数据
		"college:list", "upload:file",
	}).Find(&studentPerms)
	DB.Model(&studentRole).Association("Permissions").Append(&studentPerms)

	// --- E. 专家：竞赛查看、申报查看、获奖查看、通知查看 ---
	var expertPerms []models.Permission
	DB.Where("code IN ?", []string{
		// 大分类
		"competition", "notice",
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
	}).Find(&expertPerms)
	DB.Model(&expertRole).Association("Permissions").Append(&expertPerms)

	ensureNoticeManagePermissions()

	// --- F. 访客：无特殊权限 ---
	// 不分配任何权限

	// ==========================================
	// 4. 初始化用户 (User) - 保持不变
	// ==========================================
	hashPassword := func(password string) string {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("密码加密失败: %v", err)
			return password
		}
		return string(hashedPassword)
	}

	users := []models.User{
		{Username: "T2023001", Realname: "李老师", Password: hashPassword("123"), College: "教务处", Grade: "教职员工", Major: "管理"},
		{Username: "T2023002", Realname: "王老师", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "教职员工", Major: "管理"},
		{Username: "T2023003", Realname: "张伟", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "教职员工", Major: "计算机科学"},
		{Username: "T2023004", Realname: "李华", Password: hashPassword("123"), College: "电子信息工程学院", Grade: "教职员工", Major: "电子信息"},
		{Username: "T2023005", Realname: "王强", Password: hashPassword("123"), College: "经济管理学院", Grade: "教职员工", Major: "经济管理"},
		{Username: "T2023006", Realname: "赵敏", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "教职员工", Major: "软件工程"},
		{Username: "S2024001", Realname: "林晓明", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "2024级", Major: "计算机科学与技术"},
		{Username: "S2024002", Realname: "陈思思", Password: hashPassword("123"), College: "计算机科学与网络工程学院", Grade: "2024级", Major: "软件工程"},
		{Username: "E2023001", Realname: "周杰", Password: hashPassword("123"), College: "电子信息工程学院", Grade: "教职员工", Major: "电子工程"},
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
		"S2024001": &studentRole,            // 林晓明 - 学生
		"S2024002": &studentRole,            // 陈思思 - 学生
		"E2023001": &expertRole,             // 周杰 - 专家
		// guest 用户不分配角色，用于测试无权限访问
	}

	for _, u := range users {
		if err := DB.Create(&u).Error; err != nil {
			log.Printf("创建用户 %s 失败: %v", u.Username, err)
			continue
		}
		if role, ok := userRoleMap[u.Username]; ok {
			DB.Model(&u).Association("Roles").Append(role)
		}
	}

	// 初始化学院数据
	colleges := []models.College{
		{ID: 1, Name: "计算机学院"},
		{ID: 2, Name: "数学学院"},
		{ID: 3, Name: "机械工程学院"},
	}

	// 这里建议用 Save，如果 ID 已存在则更新，不存在则创建
	for _, col := range colleges {
		if err := DB.Save(&col).Error; err != nil {
			log.Printf("初始化学院数据失败: %v", err)
		}
	}
	log.Println("学院数据初始化完成")

	// ==========================================
	// 5. 初始化竞赛目录与详情 (保持不变)
	// ==========================================

	// 5.1 获取赛事负责人的ID（张伟 T2023003），作为负责人
	var teacherUser models.User
	DB.Where("username = ?", "T2023003").First(&teacherUser)

	// 5.2 获取另一位赛事负责人（李华 T2023004）
	var teacherUser2 models.User
	DB.Where("username = ?", "T2023004").First(&teacherUser2)

	// 5.2.1 获取王老师（王老师 T2023002）
	var teacherWang models.User
	DB.Where("username = ?", "T2023002").First(&teacherWang)

	// 5.2.2 获取赛事负责人（赵敏 T2023006）
	var teacherLi models.User
	DB.Where("username = ?", "T2023006").First(&teacherLi)

	// 5.3 定义 2026 年的时间点 (基于当前日期 2026-01-23)
	// 使用固定日期确保数据有效性
	baseTime := time.Date(2026, 1, 23, 0, 0, 0, 0, time.Local)

	// 5.3 创建竞赛数据 - 扩展为20条，覆盖各种状态和时间段
	competitions := []models.CompDirectory{
		// ==========================================
		// 状态=1 (已发布) - 正在报名中
		// ==========================================
		{
			CompCode:   "NCD-2026",
			CompName:   "2026年全国大学生计算机设计大赛",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "教育部计算机相关教指委",
			Undertaker: "厦门大学",
			CollegeID:  1,
			Year:       2026,
			ManagerID:  teacherUser.ID,
			Status:     1,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 0, -10),
				RegEndTime:         baseTime.AddDate(0, 1, 0),
				CompStartTime:      baseTime.AddDate(0, 2, 0),
				CompEndTime:        baseTime.AddDate(0, 3, 0),
				ParticipantType:    2,
				MaxTeamMember:      5,
				MinTeamMember:      2,
				GradeRequirement:   "[2022,2023,2024,2025]",
				RegistrationMethod: "请各参赛队登录国赛官网报名，并在此系统提交校内审核材料。",
				NeedAttachment:     2,
				NeedAdvisor:        2,
			},
		},
		{
			CompCode:   "LQB-2026",
			CompName:   "第十七届蓝桥杯全国软件和信息技术专业人才大赛",
			CompType:   "B类",
			CompLevel:  "省级",
			Organizer:  "工信部人才交流中心",
			Undertaker: "本校教务处",
			CollegeID:  1,
			Year:       2026,
			ManagerID:  teacherUser.ID,
			Status:     1,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 0, -20),
				RegEndTime:         baseTime.AddDate(0, 0, 10),
				CompStartTime:      baseTime.AddDate(0, 1, 15),
				CompEndTime:        baseTime.AddDate(0, 1, 15),
				ParticipantType:    1,
				MaxTeamMember:      1,
				MinTeamMember:      1,
				GradeRequirement:   "[]",
				RegistrationMethod: "个人赛，C/C++或Java组，直接在线报名。",
				NeedAttachment:     0,
				NeedAdvisor:        0,
			},
		},
		{
			CompCode:   "MCM-2026",
			CompName:   "2026年美国大学生数学建模竞赛(MCM/ICM)",
			CompType:   "A类",
			CompLevel:  "国际级",
			Organizer:  "COMAP",
			Undertaker: "数学建模协会",
			CollegeID:  2,
			Year:       2026,
			ManagerID:  teacherWang.ID,
			Status:     1,
			CreatedBy:  teacherWang.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 0, -5),
				RegEndTime:         baseTime.AddDate(0, 0, 20),
				CompStartTime:      baseTime.AddDate(0, 1, 0),
				CompEndTime:        baseTime.AddDate(0, 1, 4),
				ParticipantType:    2,
				MaxTeamMember:      3,
				MinTeamMember:      3,
				GradeRequirement:   "[2022,2023,2024]",
				RegistrationMethod: "三人组队，需有指导老师，提交英文论文。",
				NeedAttachment:     1,
				NeedAdvisor:        2,
			},
		},
		{
			CompCode:   "HUAWEI-2026",
			CompName:   "2026年华为ICT大赛",
			CompType:   "B类",
			CompLevel:  "国家级",
			Organizer:  "华为技术有限公司",
			Undertaker: "计算机学院",
			CollegeID:  1,
			Year:       2026,
			ManagerID:  teacherUser.ID,
			Status:     1,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 0, -15),
				RegEndTime:         baseTime.AddDate(0, 0, 15),
				CompStartTime:      baseTime.AddDate(0, 1, 20),
				CompEndTime:        baseTime.AddDate(0, 1, 22),
				ParticipantType:    1,
				MaxTeamMember:      1,
				MinTeamMember:      1,
				GradeRequirement:   "[2022,2023,2024,2025]",
				RegistrationMethod: "个人赛，包括网络、云计算、人工智能三个赛道。",
				NeedAttachment:     0,
				NeedAdvisor:        0,
			},
		},
		{
			CompCode:   "CHALLENGE-2026",
			CompName:   "2026年中国高校计算机大赛-网络技术挑战赛",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "全国高等学校计算机教育研究会",
			Undertaker: "杭州电子科技大学",
			CollegeID:  1,
			Year:       2026,
			ManagerID:  teacherUser.ID,
			Status:     1,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 0, -8),
				RegEndTime:         baseTime.AddDate(0, 0, 25),
				CompStartTime:      baseTime.AddDate(0, 2, 10),
				CompEndTime:        baseTime.AddDate(0, 2, 12),
				ParticipantType:    2,
				MaxTeamMember:      4,
				MinTeamMember:      2,
				GradeRequirement:   "[2022,2023,2024]",
				RegistrationMethod: "团队参赛，需提交网络拓扑设计方案。",
				NeedAttachment:     2,
				NeedAdvisor:        1,
			},
		},

		// ==========================================
		// 状态=1 (已发布) - 报名即将开始
		// ==========================================
		{
			CompCode:   "ACM-2026",
			CompName:   "2026年ACM-ICPC亚洲区域赛选拔",
			CompType:   "A类",
			CompLevel:  "国际级",
			Organizer:  "ACM",
			Undertaker: "计算机学院ACM俱乐部",
			CollegeID:  1,
			Year:       2026,
			ManagerID:  teacherUser.ID,
			Status:     1,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 0, 30),
				RegEndTime:         baseTime.AddDate(0, 2, 0),
				CompStartTime:      baseTime.AddDate(0, 3, 0),
				CompEndTime:        baseTime.AddDate(0, 3, 0),
				ParticipantType:    2,
				MaxTeamMember:      3,
				MinTeamMember:      3,
				GradeRequirement:   "[2023,2024,2025]",
				RegistrationMethod: "三人组队，通过校内选拔赛获得参赛资格。",
				NeedAttachment:     0,
				NeedAdvisor:        1,
			},
		},
		{
			CompCode:   "CTFCHAMP-2026",
			CompName:   "2026年全国大学生信息安全竞赛-创新实践能力赛",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "教育部高等学校信息安全专业教指委",
			Undertaker: "西安电子科技大学",
			CollegeID:  1,
			Year:       2026,
			ManagerID:  teacherUser.ID,
			Status:     1,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 1, 0),
				RegEndTime:         baseTime.AddDate(0, 2, 15),
				CompStartTime:      baseTime.AddDate(0, 3, 10),
				CompEndTime:        baseTime.AddDate(0, 3, 12),
				ParticipantType:    2,
				MaxTeamMember:      4,
				MinTeamMember:      2,
				GradeRequirement:   "[2022,2023,2024,2025]",
				RegistrationMethod: "CTF竞赛形式，需提交战队信息。",
				NeedAttachment:     1,
				NeedAdvisor:        1,
			},
		},
		{
			CompCode:   "ENGMATH-2026",
			CompName:   "2026年全国大学生工程数学竞赛",
			CompType:   "B类",
			CompLevel:  "国家级",
			Organizer:  "中国工业与应用数学学会",
			Undertaker: "数学学院",
			CollegeID:  2,
			Year:       2026,
			ManagerID:  teacherWang.ID,
			Status:     1,
			CreatedBy:  teacherWang.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 1, 15),
				RegEndTime:         baseTime.AddDate(0, 2, 20),
				CompStartTime:      baseTime.AddDate(0, 3, 15),
				CompEndTime:        baseTime.AddDate(0, 3, 15),
				ParticipantType:    1,
				MaxTeamMember:      1,
				MinTeamMember:      1,
				GradeRequirement:   "[2022,2023,2024]",
				RegistrationMethod: "个人赛，工程数学综合应用。",
				NeedAttachment:     0,
				NeedAdvisor:        0,
			},
		},

		// ==========================================
		// 状态=1 (已发布) - 报名已结束，比赛进行中
		// ==========================================
		{
			CompCode:   "ROBOCON-2026",
			CompName:   "2026年全国大学生机器人大赛RoboMaster",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "共青团中央",
			Undertaker: "大疆创新",
			CollegeID:  3,
			Year:       2026,
			ManagerID:  teacherLi.ID,
			Status:     1,
			CreatedBy:  teacherLi.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, -2, 0),
				RegEndTime:         baseTime.AddDate(0, -1, 0),
				CompStartTime:      baseTime.AddDate(0, 0, -5),
				CompEndTime:        baseTime.AddDate(0, 0, 10),
				ParticipantType:    2,
				MaxTeamMember:      20,
				MinTeamMember:      10,
				GradeRequirement:   "[]",
				RegistrationMethod: "战队形式参赛，需提交技术方案书。",
				NeedAttachment:     2,
				NeedAdvisor:        2,
			},
		},
		{
			CompCode:   "MECHDESIGN-2026",
			CompName:   "2026年全国大学生机械创新设计大赛",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "教育部高等学校机械基础课程教指委",
			Undertaker: "浙江大学",
			CollegeID:  3,
			Year:       2026,
			ManagerID:  teacherLi.ID,
			Status:     1,
			CreatedBy:  teacherLi.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, -3, 0),
				RegEndTime:         baseTime.AddDate(0, -1, 15),
				CompStartTime:      baseTime.AddDate(0, 0, -10),
				CompEndTime:        baseTime.AddDate(0, 0, 5),
				ParticipantType:    2,
				MaxTeamMember:      5,
				MinTeamMember:      3,
				GradeRequirement:   "[2022,2023,2024]",
				RegistrationMethod: "需提交机械创新设计作品及说明书。",
				NeedAttachment:     2,
				NeedAdvisor:        2,
			},
		},

		// ==========================================
		// 状态=2 (已结束)
		// ==========================================
		{
			CompCode:   "CUMCM-2025",
			CompName:   "2025年全国大学生数学建模竞赛",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "中国工业与应用数学学会",
			Undertaker: "数学建模组委会",
			CollegeID:  2,
			Year:       2025,
			ManagerID:  teacherWang.ID,
			Status:     2,
			CreatedBy:  teacherWang.ID,
			Detail: models.CompDetail{
				RegStartTime:       time.Date(2025, 7, 1, 0, 0, 0, 0, time.Local),
				RegEndTime:         time.Date(2025, 8, 31, 0, 0, 0, 0, time.Local),
				CompStartTime:      time.Date(2025, 9, 14, 0, 0, 0, 0, time.Local),
				CompEndTime:        time.Date(2025, 9, 17, 0, 0, 0, 0, time.Local),
				ParticipantType:    2,
				MaxTeamMember:      3,
				MinTeamMember:      3,
				GradeRequirement:   "[2022,2023,2024]",
				RegistrationMethod: "已结束。三人组队参赛。",
				NeedAttachment:     2,
				NeedAdvisor:        2,
			},
		},
		{
			CompCode:   "CCPC-2025",
			CompName:   "2025年中国大学生程序设计竞赛(CCPC)总决赛",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "中国计算机学会",
			Undertaker: "哈尔滨工业大学",
			CollegeID:  1,
			Year:       2025,
			ManagerID:  teacherUser.ID,
			Status:     2,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       time.Date(2025, 10, 1, 0, 0, 0, 0, time.Local),
				RegEndTime:         time.Date(2025, 11, 15, 0, 0, 0, 0, time.Local),
				CompStartTime:      time.Date(2025, 12, 10, 0, 0, 0, 0, time.Local),
				CompEndTime:        time.Date(2025, 12, 10, 0, 0, 0, 0, time.Local),
				ParticipantType:    2,
				MaxTeamMember:      3,
				MinTeamMember:      3,
				GradeRequirement:   "[2022,2023,2024,2025]",
				RegistrationMethod: "已结束。通过区域赛晋级。",
				NeedAttachment:     0,
				NeedAdvisor:        1,
			},
		},
		{
			CompCode:   "BIGDATA-2025",
			CompName:   "2025年全国高校大数据挑战赛",
			CompType:   "B类",
			CompLevel:  "国家级",
			Organizer:  "阿里云",
			Undertaker: "计算机学院",
			CollegeID:  1,
			Year:       2025,
			ManagerID:  teacherUser.ID,
			Status:     2,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       time.Date(2025, 3, 1, 0, 0, 0, 0, time.Local),
				RegEndTime:         time.Date(2025, 4, 15, 0, 0, 0, 0, time.Local),
				CompStartTime:      time.Date(2025, 5, 1, 0, 0, 0, 0, time.Local),
				CompEndTime:        time.Date(2025, 6, 30, 0, 0, 0, 0, time.Local),
				ParticipantType:    2,
				MaxTeamMember:      4,
				MinTeamMember:      1,
				GradeRequirement:   "[2021,2022,2023,2024]",
				RegistrationMethod: "已结束。数据分析与算法实现。",
				NeedAttachment:     2,
				NeedAdvisor:        1,
			},
		},
		{
			CompCode:   "ROBOT-2025",
			CompName:   "2025年中国机器人大赛暨RoboCup机器人世界杯中国赛",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "中国自动化学会",
			Undertaker: "机械工程学院",
			CollegeID:  3,
			Year:       2025,
			ManagerID:  teacherLi.ID,
			Status:     2,
			CreatedBy:  teacherLi.ID,
			Detail: models.CompDetail{
				RegStartTime:       time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
				RegEndTime:         time.Date(2025, 5, 31, 0, 0, 0, 0, time.Local),
				CompStartTime:      time.Date(2025, 7, 15, 0, 0, 0, 0, time.Local),
				CompEndTime:        time.Date(2025, 7, 20, 0, 0, 0, 0, time.Local),
				ParticipantType:    2,
				MaxTeamMember:      8,
				MinTeamMember:      4,
				GradeRequirement:   "[]",
				RegistrationMethod: "已结束。机器人足球、救援等多个赛道。",
				NeedAttachment:     2,
				NeedAdvisor:        2,
			},
		},

		// ==========================================
		// 状态=0 (草稿)
		// ==========================================
		{
			CompCode:   "ICPC-SCHOOL-2026",
			CompName:   "2026校内程序设计天梯赛（草稿）",
			CompType:   "C类",
			CompLevel:  "校级",
			Organizer:  "计算机学院",
			Undertaker: "ACM俱乐部",
			CollegeID:  1,
			Year:       2026,
			ManagerID:  teacherUser.ID,
			Status:     0,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 1, 0),
				RegEndTime:         baseTime.AddDate(0, 2, 0),
				CompStartTime:      baseTime.AddDate(0, 2, 15),
				CompEndTime:        baseTime.AddDate(0, 2, 15),
				ParticipantType:    1,
				MaxTeamMember:      1,
				MinTeamMember:      1,
				GradeRequirement:   "[]",
				RegistrationMethod: "待定...",
				NeedAttachment:     0,
				NeedAdvisor:        0,
			},
		},
		{
			CompCode:   "AI-2026",
			CompName:   "2026年人工智能创新应用大赛（草稿）",
			CompType:   "B类",
			CompLevel:  "省级",
			Organizer:  "省教育厅",
			Undertaker: "人工智能学院",
			CollegeID:  1,
			Year:       2026,
			ManagerID:  teacherUser2.ID, // 李华作为负责人
			Status:     0,
			CreatedBy:  teacherUser2.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 2, 0),
				RegEndTime:         baseTime.AddDate(0, 3, 0),
				CompStartTime:      baseTime.AddDate(0, 4, 0),
				CompEndTime:        baseTime.AddDate(0, 4, 15),
				ParticipantType:    2,
				MaxTeamMember:      4,
				MinTeamMember:      2,
				GradeRequirement:   "[2023,2024,2025]",
				RegistrationMethod: "待定... 需提交AI项目作品。",
				NeedAttachment:     2,
				NeedAdvisor:        1,
			},
		},
		{
			CompCode:   "IOT-2026",
			CompName:   "2026年物联网设计竞赛（草稿）",
			CompType:   "B类",
			CompLevel:  "国家级",
			Organizer:  "教育部高等学校计算机类专业教指委",
			Undertaker: "待定",
			CollegeID:  3,
			Year:       2026,
			ManagerID:  teacherLi.ID,
			Status:     0,
			CreatedBy:  teacherLi.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 3, 0),
				RegEndTime:         baseTime.AddDate(0, 4, 0),
				CompStartTime:      baseTime.AddDate(0, 5, 0),
				CompEndTime:        baseTime.AddDate(0, 5, 0),
				ParticipantType:    2,
				MaxTeamMember:      4,
				MinTeamMember:      2,
				GradeRequirement:   "[]",
				RegistrationMethod: "待定...",
				NeedAttachment:     1,
				NeedAdvisor:        1,
			},
		},
		{
			CompCode:   "BLOCKCHAIN-2026",
			CompName:   "2026年全国高校区块链应用创新大赛（草稿）",
			CompType:   "B类",
			CompLevel:  "国家级",
			Organizer:  "中国区块链技术协会",
			Undertaker: "计算机学院",
			CollegeID:  1,
			Year:       2026,
			ManagerID:  teacherUser.ID,
			Status:     0,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 2, 15),
				RegEndTime:         baseTime.AddDate(0, 3, 15),
				CompStartTime:      baseTime.AddDate(0, 4, 10),
				CompEndTime:        baseTime.AddDate(0, 4, 12),
				ParticipantType:    2,
				MaxTeamMember:      5,
				MinTeamMember:      2,
				GradeRequirement:   "[2022,2023,2024,2025]",
				RegistrationMethod: "待定... 需提交区块链应用设计方案。",
				NeedAttachment:     2,
				NeedAdvisor:        1,
			},
		},
		{
			CompCode:   "SMARTCAR-2026",
			CompName:   "2026年全国大学生智能汽车竞赛（草稿）",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "教育部高等学校自动化类专业教指委",
			Undertaker: "机械工程学院",
			CollegeID:  3,
			Year:       2026,
			ManagerID:  teacherLi.ID,
			Status:     0,
			CreatedBy:  teacherLi.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 2, 0),
				RegEndTime:         baseTime.AddDate(0, 3, 30),
				CompStartTime:      baseTime.AddDate(0, 5, 0),
				CompEndTime:        baseTime.AddDate(0, 5, 5),
				ParticipantType:    2,
				MaxTeamMember:      4,
				MinTeamMember:      2,
				GradeRequirement:   "[2022,2023,2024,2025]",
				RegistrationMethod: "待定... 智能车模设计与调试。",
				NeedAttachment:     2,
				NeedAdvisor:        2,
			},
		},
		{
			CompCode:   "MATHMODEL-SCHOOL-2026",
			CompName:   "2026校内数学建模选拔赛（草稿）",
			CompType:   "C类",
			CompLevel:  "校级",
			Organizer:  "数学学院",
			Undertaker: "数学建模协会",
			CollegeID:  2,
			Year:       2026,
			ManagerID:  teacherWang.ID,
			Status:     0,
			CreatedBy:  teacherWang.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 3, 0),
				RegEndTime:         baseTime.AddDate(0, 4, 0),
				CompStartTime:      baseTime.AddDate(0, 4, 15),
				CompEndTime:        baseTime.AddDate(0, 4, 18),
				ParticipantType:    2,
				MaxTeamMember:      3,
				MinTeamMember:      3,
				GradeRequirement:   "[2023,2024,2025]",
				RegistrationMethod: "待定... 为全国数学建模竞赛进行校内选拔。",
				NeedAttachment:     1,
				NeedAdvisor:        0,
			},
		},
	}

	// 批量创建竞赛 (先创建竞赛主体，不创建关联的Detail)
	// 使用Omit忽略Detail字段，避免自动创建空的detail记录
	if err := DB.Omit("Detail").Create(&competitions).Error; err != nil {
		log.Printf("❌ 创建竞赛数据失败: %v", err)
		return
	}

	// 验证竞赛数据是否创建成功
	var compCount int64
	DB.Model(&models.CompDirectory{}).Count(&compCount)
	log.Printf("✅ 成功创建竞赛目录数据: %d 条", compCount)

	if compCount == 0 {
		log.Printf("❌ 竞赛目录创建失败，数据库中没有数据")
		return
	}

	// 单独创建竞赛详情，确保CompID正确对应
	// 注意：这里需要根据CompCode来匹配并创建详情
	detailUpdates := map[string]models.CompDetail{
		"NCD-2026": {
			RegStartTime:       baseTime.AddDate(0, 0, -10),
			RegEndTime:         baseTime.AddDate(0, 1, 0),
			CompStartTime:      baseTime.AddDate(0, 2, 0),
			CompEndTime:        baseTime.AddDate(0, 3, 0),
			SubmitStartTime:    baseTime.AddDate(0, 1, 5),
			SubmitEndTime:      baseTime.AddDate(0, 1, 25),
			ParticipantType:    2,
			MaxTeamMember:      5,
			MinTeamMember:      2,
			GradeRequirement:   "[2022,2023,2024,2025]",
			RegistrationMethod: "请各参赛队登录国赛官网报名，并在此系统提交校内审核材料。",
			NeedAttachment:     2,
			NeedAdvisor:        2,
		},
		"LQB-2026": {
			RegStartTime:       baseTime.AddDate(0, 0, -20),
			RegEndTime:         baseTime.AddDate(0, 0, 10),
			CompStartTime:      baseTime.AddDate(0, 1, 15),
			CompEndTime:        baseTime.AddDate(0, 1, 15),
			SubmitStartTime:    baseTime.AddDate(0, 0, 12),
			SubmitEndTime:      baseTime.AddDate(0, 1, 10),
			ParticipantType:    1,
			MaxTeamMember:      1,
			MinTeamMember:      1,
			GradeRequirement:   "[]",
			RegistrationMethod: "个人赛，C/C++或Java组，直接在线报名。",
			NeedAttachment:     0,
			NeedAdvisor:        0,
		},
		"MCM-2026": {
			RegStartTime:       baseTime.AddDate(0, 0, -5),
			RegEndTime:         baseTime.AddDate(0, 0, 20),
			CompStartTime:      baseTime.AddDate(0, 1, 0),
			CompEndTime:        baseTime.AddDate(0, 1, 4),
			SubmitStartTime:    baseTime.AddDate(0, 0, 8),
			SubmitEndTime:      baseTime.AddDate(0, 0, 28),
			ParticipantType:    2,
			MaxTeamMember:      3,
			MinTeamMember:      3,
			GradeRequirement:   "[2022,2023,2024]",
			RegistrationMethod: "三人组队，需有指导老师，提交英文论文。",
			NeedAttachment:     1,
			NeedAdvisor:        2,
		},
		"HUAWEI-2026": {
			RegStartTime:       baseTime.AddDate(0, 0, -15),
			RegEndTime:         baseTime.AddDate(0, 0, 15),
			CompStartTime:      baseTime.AddDate(0, 1, 20),
			CompEndTime:        baseTime.AddDate(0, 1, 22),
			SubmitStartTime:    baseTime.AddDate(0, 0, 5),
			SubmitEndTime:      baseTime.AddDate(0, 1, 15),
			ParticipantType:    1,
			MaxTeamMember:      1,
			MinTeamMember:      1,
			GradeRequirement:   "[2022,2023,2024,2025]",
			RegistrationMethod: "个人赛，包括网络、云计算、人工智能三个赛道。",
			NeedAttachment:     0,
			NeedAdvisor:        0,
		},
		"CHALLENGE-2026": {
			RegStartTime:       baseTime.AddDate(0, 0, -8),
			RegEndTime:         baseTime.AddDate(0, 0, 25),
			CompStartTime:      baseTime.AddDate(0, 2, 10),
			CompEndTime:        baseTime.AddDate(0, 2, 12),
			SubmitStartTime:    baseTime.AddDate(0, 0, 15),
			SubmitEndTime:      baseTime.AddDate(0, 2, 5),
			ParticipantType:    2,
			MaxTeamMember:      4,
			MinTeamMember:      2,
			GradeRequirement:   "[2022,2023,2024]",
			RegistrationMethod: "团队参赛，需提交网络拓扑设计方案。",
			NeedAttachment:     2,
			NeedAdvisor:        1,
		},
		"ACM-2026": {
			RegStartTime:       baseTime.AddDate(0, 0, 30),
			RegEndTime:         baseTime.AddDate(0, 2, 0),
			CompStartTime:      baseTime.AddDate(0, 3, 0),
			CompEndTime:        baseTime.AddDate(0, 3, 0),
			SubmitStartTime:    baseTime.AddDate(0, 2, 15),
			SubmitEndTime:      baseTime.AddDate(0, 2, 28),
			ParticipantType:    2,
			MaxTeamMember:      3,
			MinTeamMember:      3,
			GradeRequirement:   "[2023,2024,2025]",
			RegistrationMethod: "三人组队，通过校内选拔赛获得参赛资格。",
			NeedAttachment:     0,
			NeedAdvisor:        1,
		},
		"CTFCHAMP-2026": {
			RegStartTime:       baseTime.AddDate(0, 1, 0),
			RegEndTime:         baseTime.AddDate(0, 2, 15),
			CompStartTime:      baseTime.AddDate(0, 3, 10),
			CompEndTime:        baseTime.AddDate(0, 3, 12),
			SubmitStartTime:    baseTime.AddDate(0, 2, 20),
			SubmitEndTime:      baseTime.AddDate(0, 3, 5),
			ParticipantType:    2,
			MaxTeamMember:      4,
			MinTeamMember:      2,
			GradeRequirement:   "[2022,2023,2024,2025]",
			RegistrationMethod: "CTF竞赛形式，需提交战队信息。",
			NeedAttachment:     1,
			NeedAdvisor:        1,
		},
		"ENGMATH-2026": {
			RegStartTime:       baseTime.AddDate(0, 1, 15),
			RegEndTime:         baseTime.AddDate(0, 2, 20),
			CompStartTime:      baseTime.AddDate(0, 3, 15),
			CompEndTime:        baseTime.AddDate(0, 3, 15),
			SubmitStartTime:    baseTime.AddDate(0, 2, 28),
			SubmitEndTime:      baseTime.AddDate(0, 3, 10),
			ParticipantType:    1,
			MaxTeamMember:      1,
			MinTeamMember:      1,
			GradeRequirement:   "[2022,2023,2024]",
			RegistrationMethod: "个人赛，工程数学综合应用。",
			NeedAttachment:     0,
			NeedAdvisor:        0,
		},
		"ROBOCON-2026": {
			RegStartTime:       baseTime.AddDate(0, -2, 0),
			RegEndTime:         baseTime.AddDate(0, -1, 0),
			CompStartTime:      baseTime.AddDate(0, 0, -5),
			CompEndTime:        baseTime.AddDate(0, 0, 10),
			SubmitStartTime:    baseTime.AddDate(0, -1, 15),
			SubmitEndTime:      baseTime.AddDate(0, 0, 0),
			ParticipantType:    2,
			MaxTeamMember:      20,
			MinTeamMember:      10,
			GradeRequirement:   "[]",
			RegistrationMethod: "战队形式参赛，需提交技术方案书。",
			NeedAttachment:     2,
			NeedAdvisor:        2,
		},
		"MECHDESIGN-2026": {
			RegStartTime:       baseTime.AddDate(0, -3, 0),
			RegEndTime:         baseTime.AddDate(0, -1, 15),
			CompStartTime:      baseTime.AddDate(0, 0, -10),
			CompEndTime:        baseTime.AddDate(0, 0, 5),
			SubmitStartTime:    baseTime.AddDate(0, -2, 10),
			SubmitEndTime:      baseTime.AddDate(0, -1, 5),
			ParticipantType:    2,
			MaxTeamMember:      5,
			MinTeamMember:      3,
			GradeRequirement:   "[2022,2023,2024]",
			RegistrationMethod: "需提交机械创新设计作品及说明书。",
			NeedAttachment:     2,
			NeedAdvisor:        2,
		},
		"CUMCM-2025": {
			RegStartTime:       time.Date(2025, 7, 1, 0, 0, 0, 0, time.Local),
			RegEndTime:         time.Date(2025, 8, 31, 23, 59, 59, 0, time.Local),
			CompStartTime:      time.Date(2025, 9, 14, 8, 0, 0, 0, time.Local),
			CompEndTime:        time.Date(2025, 9, 17, 20, 0, 0, 0, time.Local),
			SubmitStartTime:    time.Date(2025, 8, 10, 0, 0, 0, 0, time.Local),
			SubmitEndTime:      time.Date(2025, 9, 10, 0, 0, 0, 0, time.Local),
			ParticipantType:    2,
			MaxTeamMember:      3,
			MinTeamMember:      3,
			GradeRequirement:   "[2022,2023,2024]",
			RegistrationMethod: "已结束。三人组队参赛。",
			NeedAttachment:     2,
			NeedAdvisor:        2,
		},
		"CCPC-2025": {
			RegStartTime:       time.Date(2025, 10, 1, 0, 0, 0, 0, time.Local),
			RegEndTime:         time.Date(2025, 11, 15, 23, 59, 59, 0, time.Local),
			CompStartTime:      time.Date(2025, 12, 10, 9, 0, 0, 0, time.Local),
			CompEndTime:        time.Date(2025, 12, 10, 14, 0, 0, 0, time.Local),
			SubmitStartTime:    time.Date(2025, 11, 20, 0, 0, 0, 0, time.Local),
			SubmitEndTime:      time.Date(2025, 12, 5, 0, 0, 0, 0, time.Local),
			ParticipantType:    2,
			MaxTeamMember:      3,
			MinTeamMember:      3,
			GradeRequirement:   "[2022,2023,2024,2025]",
			RegistrationMethod: "已结束。通过区域赛晋级。",
			NeedAttachment:     0,
			NeedAdvisor:        1,
		},
		"BIGDATA-2025": {
			RegStartTime:       time.Date(2025, 3, 1, 0, 0, 0, 0, time.Local),
			RegEndTime:         time.Date(2025, 4, 15, 0, 0, 0, 0, time.Local),
			CompStartTime:      time.Date(2025, 5, 1, 0, 0, 0, 0, time.Local),
			CompEndTime:        time.Date(2025, 6, 30, 0, 0, 0, 0, time.Local),
			SubmitStartTime:    time.Date(2025, 4, 20, 0, 0, 0, 0, time.Local),
			SubmitEndTime:      time.Date(2025, 5, 25, 0, 0, 0, 0, time.Local),
			ParticipantType:    2,
			MaxTeamMember:      4,
			MinTeamMember:      1,
			GradeRequirement:   "[2021,2022,2023,2024]",
			RegistrationMethod: "已结束。数据分析与算法实现。",
			NeedAttachment:     2,
			NeedAdvisor:        1,
		},
		"ROBOT-2025": {
			RegStartTime:       time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			RegEndTime:         time.Date(2025, 5, 31, 0, 0, 0, 0, time.Local),
			CompStartTime:      time.Date(2025, 7, 15, 0, 0, 0, 0, time.Local),
			CompEndTime:        time.Date(2025, 7, 20, 0, 0, 0, 0, time.Local),
			SubmitStartTime:    time.Date(2025, 6, 1, 0, 0, 0, 0, time.Local),
			SubmitEndTime:      time.Date(2025, 7, 10, 0, 0, 0, 0, time.Local),
			ParticipantType:    2,
			MaxTeamMember:      8,
			MinTeamMember:      4,
			GradeRequirement:   "[]",
			RegistrationMethod: "已结束。机器人足球、救援等多个赛道。",
			NeedAttachment:     2,
			NeedAdvisor:        2,
		},
		"ICPC-SCHOOL-2026": {
			RegStartTime:       baseTime.AddDate(0, 1, 0),
			RegEndTime:         baseTime.AddDate(0, 2, 0),
			CompStartTime:      baseTime.AddDate(0, 2, 15),
			CompEndTime:        baseTime.AddDate(0, 2, 15),
			SubmitStartTime:    baseTime.AddDate(0, 1, 20),
			SubmitEndTime:      baseTime.AddDate(0, 2, 10),
			ParticipantType:    1,
			MaxTeamMember:      1,
			MinTeamMember:      1,
			GradeRequirement:   "[]",
			RegistrationMethod: "待定...",
			NeedAttachment:     0,
			NeedAdvisor:        0,
		},
		"AI-2026": {
			RegStartTime:       baseTime.AddDate(0, 2, 0),
			RegEndTime:         baseTime.AddDate(0, 3, 0),
			CompStartTime:      baseTime.AddDate(0, 4, 0),
			CompEndTime:        baseTime.AddDate(0, 4, 15),
			SubmitStartTime:    baseTime.AddDate(0, 3, 10),
			SubmitEndTime:      baseTime.AddDate(0, 3, 28),
			ParticipantType:    2,
			MaxTeamMember:      4,
			MinTeamMember:      2,
			GradeRequirement:   "[2023,2024,2025]",
			RegistrationMethod: "待定... 需提交AI项目作品。",
			NeedAttachment:     2,
			NeedAdvisor:        1,
		},
		"IOT-2026": {
			RegStartTime:       baseTime.AddDate(0, 3, 0),
			RegEndTime:         baseTime.AddDate(0, 4, 0),
			CompStartTime:      baseTime.AddDate(0, 5, 0),
			CompEndTime:        baseTime.AddDate(0, 5, 0),
			SubmitStartTime:    baseTime.AddDate(0, 4, 10),
			SubmitEndTime:      baseTime.AddDate(0, 4, 28),
			ParticipantType:    2,
			MaxTeamMember:      4,
			MinTeamMember:      2,
			GradeRequirement:   "[]",
			RegistrationMethod: "待定...",
			NeedAttachment:     1,
			NeedAdvisor:        1,
		},
		"BLOCKCHAIN-2026": {
			RegStartTime:       baseTime.AddDate(0, 2, 15),
			RegEndTime:         baseTime.AddDate(0, 3, 15),
			CompStartTime:      baseTime.AddDate(0, 4, 10),
			CompEndTime:        baseTime.AddDate(0, 4, 12),
			SubmitStartTime:    baseTime.AddDate(0, 3, 20),
			SubmitEndTime:      baseTime.AddDate(0, 4, 5),
			ParticipantType:    2,
			MaxTeamMember:      5,
			MinTeamMember:      2,
			GradeRequirement:   "[2022,2023,2024,2025]",
			RegistrationMethod: "待定... 需提交区块链应用设计方案。",
			NeedAttachment:     2,
			NeedAdvisor:        1,
		},
		"SMARTCAR-2026": {
			RegStartTime:       baseTime.AddDate(0, 2, 0),
			RegEndTime:         baseTime.AddDate(0, 3, 30),
			CompStartTime:      baseTime.AddDate(0, 5, 0),
			CompEndTime:        baseTime.AddDate(0, 5, 5),
			SubmitStartTime:    baseTime.AddDate(0, 4, 5),
			SubmitEndTime:      baseTime.AddDate(0, 4, 28),
			ParticipantType:    2,
			MaxTeamMember:      4,
			MinTeamMember:      2,
			GradeRequirement:   "[2022,2023,2024,2025]",
			RegistrationMethod: "待定... 智能车模设计与调试。",
			NeedAttachment:     2,
			NeedAdvisor:        2,
		},
		"MATHMODEL-SCHOOL-2026": {
			RegStartTime:       baseTime.AddDate(0, 3, 0),
			RegEndTime:         baseTime.AddDate(0, 4, 0),
			CompStartTime:      baseTime.AddDate(0, 4, 15),
			CompEndTime:        baseTime.AddDate(0, 4, 18),
			SubmitStartTime:    baseTime.AddDate(0, 4, 1),
			SubmitEndTime:      baseTime.AddDate(0, 4, 12),
			ParticipantType:    2,
			MaxTeamMember:      3,
			MinTeamMember:      3,
			GradeRequirement:   "[2023,2024,2025]",
			RegistrationMethod: "待定... 为全国数学建模竞赛进行校内选拔。",
			NeedAttachment:     1,
			NeedAdvisor:        0,
		},
	}

	// 按CompCode逐个创建竞赛详情
	detailCount := 0
	failedComps := []string{}

	for compCode, detail := range detailUpdates {
		var comp models.CompDirectory
		// 通过CompCode查询竞赛
		if err := DB.Where("comp_code = ?", compCode).First(&comp).Error; err != nil {
			log.Printf("⚠️  查询竞赛 %s 失败: %v", compCode, err)
			failedComps = append(failedComps, compCode)
			continue
		}
		if detail.AwardHierarchy == "" {
			detail.AwardHierarchy = `["一等奖","二等奖","三等奖"]` // 设置默认奖项
		}
		// ✅ 移除之前的检查逻辑，直接创建或更新
		detail.CompID = comp.ID

		// 使用 FirstOrCreate 确保记录存在
		if err := DB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "comp_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"reg_start_time", "reg_end_time",
				"submit_start_time", "submit_end_time",
				"participant_type", "max_team_member", "min_team_member",
				"grade_requirement", "registration_method",
				"need_attachment", "need_advisor",
			}),
		}).Create(&detail).Error; err != nil {
			log.Printf("❌ 创建或更新竞赛详情 %s 失败: %v", compCode, err)
			failedComps = append(failedComps, compCode)
		} else {
			log.Printf("✅ 创建竞赛详情 %s 成功 (CompID: %d)", compCode, comp.ID)
			detailCount++
		}
	}

	log.Printf("✅ 竞赛详情创建完成: %d/%d 条成功", detailCount, len(detailUpdates))
	if len(failedComps) > 0 {
		log.Printf("❌ 失败的竞赛: %v", failedComps)
	}

	// ==========================================
	// 6. 初始化报名与获奖数据
	// ==========================================
	var student1, student2 models.User
	DB.Where("username = ?", "S2024001").First(&student1)
	DB.Where("username = ?", "S2024002").First(&student2)

	getCompByCode := func(code string) *models.CompDirectory {
		var comp models.CompDirectory
		if err := DB.Where("comp_code = ?", code).First(&comp).Error; err != nil {
			log.Printf("⚠️  查询竞赛 %s 失败: %v", code, err)
			return nil
		}
		return &comp
	}

	compNCD := getCompByCode("NCD-2026")
	compLQB := getCompByCode("LQB-2026")
	compMCM := getCompByCode("MCM-2026")

	if compNCD != nil && compLQB != nil && compMCM != nil {
		regList := []models.Register{
			{
				CompID:        compNCD.ID,
				LeaderID:      student1.ID,
				TeamName:      "智创先锋队",
				Status:        1,
				AttachmentUrl: "/static/reg_attachments/demo_reg_1.pdf",
			},
			{
				CompID:         compNCD.ID,
				LeaderID:       student2.ID,
				TeamName:       "云智团队",
				Status:         3,
				SupplementTime: func() *time.Time { t := baseTime.AddDate(0, 0, -1); return &t }(),
				AttachmentUrl:  "/static/reg_attachments/demo_reg_2.pdf",
			},
			{
				CompID:        compLQB.ID,
				LeaderID:      student1.ID,
				TeamName:      "",
				Status:        1,
				AttachmentUrl: "",
			},
			{
				CompID:        compMCM.ID,
				LeaderID:      student2.ID,
				TeamName:      "数模三人组",
				Status:        1,
				AttachmentUrl: "/static/reg_attachments/demo_reg_3.pdf",
			},
		}

		if err := DB.Create(&regList).Error; err != nil {
			log.Printf("❌ 创建报名数据失败: %v", err)
		} else {
			log.Printf("✅ 创建报名数据: %d 条", len(regList))
		}

		// 绑定成员（包含队长）
		members := []models.RegMember{}
		if len(regList) >= 1 {
			members = append(members,
				models.RegMember{RegID: regList[0].ID, Name: student1.Realname, StudentID: student1.Username, Phone: "13800000001", Email: "linxm@example.com", College: student1.College, IsLeader: true, Year: "2024"},
				models.RegMember{RegID: regList[0].ID, Name: "王小明", StudentID: "S2024010", Phone: "13800000010", Email: "wxm@example.com", College: "计算机科学与网络工程学院", IsLeader: false, Year: "2024"},
			)
		}
		if len(regList) >= 2 {
			members = append(members,
				models.RegMember{RegID: regList[1].ID, Name: student2.Realname, StudentID: student2.Username, Phone: "13800000002", Email: "css@example.com", College: student2.College, IsLeader: true, Year: "2024"},
				models.RegMember{RegID: regList[1].ID, Name: "赵一", StudentID: "S2024020", Phone: "13800000020", Email: "zy@example.com", College: "计算机科学与网络工程学院", IsLeader: false, Year: "2024"},
			)
		}
		if len(regList) >= 3 {
			members = append(members,
				models.RegMember{RegID: regList[2].ID, Name: student1.Realname, StudentID: student1.Username, Phone: "13800000001", Email: "linxm@example.com", College: student1.College, IsLeader: true, Year: "2024"},
			)
		}
		if len(regList) >= 4 {
			members = append(members,
				models.RegMember{RegID: regList[3].ID, Name: student2.Realname, StudentID: student2.Username, Phone: "13800000002", Email: "css@example.com", College: student2.College, IsLeader: true, Year: "2024"},
				models.RegMember{RegID: regList[3].ID, Name: "孙二", StudentID: "S2024021", Phone: "13800000021", Email: "se@example.com", College: "数学学院", IsLeader: false, Year: "2024"},
				models.RegMember{RegID: regList[3].ID, Name: "钱三", StudentID: "S2024022", Phone: "13800000022", Email: "qs@example.com", College: "数学学院", IsLeader: false, Year: "2024"},
			)
		}

		if len(members) > 0 {
			if err := DB.Create(&members).Error; err != nil {
				log.Printf("❌ 创建报名成员失败: %v", err)
			}
		}

		// 创建获奖数据（含导入与补录）
		awards := []models.Award{}
		if len(regList) >= 1 {
			awards = append(awards, models.Award{
				CompID:     compNCD.ID,
				RegID:      regList[0].ID,
				AwardLevel: "国家级一等奖",
				AwardName:  "金奖",
				Status:     "approved",
				Source:     "import",
				LevelRank:  1,
			})
		}
		if len(regList) >= 2 {
			awards = append(awards, models.Award{
				CompID:     compNCD.ID,
				RegID:      regList[1].ID,
				AwardLevel: "国家级二等奖",
				AwardName:  "银奖",
				Status:     "draft",
				Source:     "supplement",
				ProofUrl:   "/static/award_proofs/demo_award_1.png",
				LevelRank:  2,
			})
		}
		if len(regList) >= 3 {
			awards = append(awards, models.Award{
				CompID:       compLQB.ID,
				RegID:        regList[2].ID,
				AwardLevel:   "省级二等奖",
				AwardName:    "二等奖",
				Status:       "rejected",
				Source:       "import",
				RejectReason: "证书信息不清晰",
				LevelRank:    3,
			})
		}
		if len(regList) >= 4 {
			awards = append(awards, models.Award{
				CompID:     compMCM.ID,
				RegID:      regList[3].ID,
				AwardLevel: "国家级三等奖",
				AwardName:  "三等奖",
				Status:     "approved",
				Source:     "import",
				LevelRank:  3,
			})
		}

		if len(awards) > 0 {
			if err := DB.Create(&awards).Error; err != nil {
				log.Printf("❌ 创建获奖数据失败: %v", err)
			} else {
				log.Printf("✅ 创建获奖数据: %d 条", len(awards))
			}
		}
	}

	seedNotices(baseTime)

	log.Println("🎉 树形权限与竞赛测试数据初始化完成！")
	log.Println("================================")
	log.Println("📋 用户账号信息：")
	log.Println("  校级管理员: T2023001    密码: 123 (拥有所有权限)")
	log.Println("  院级管理员: T2023002     密码: 123 (竞赛查看+申报审核+报名审核+报名配置查看)")
	log.Println("  赛事负责人: T2023003  密码: 123 (竞赛管理+申报管理+报名配置编辑+报名审核)")
	log.Println("  学生:       S2024001  密码: 123 (报名提交+通知查看)")
	log.Println("  专家:       E2023001   密码: 123 (竞赛查看+申报查看+获奖查看+通知查看)")
	log.Println("  访客:       guest    密码: 123 (无任何权限-测试用)")
	log.Println("================================")
	log.Println("📊 竞赛数据统计：")
	log.Println("  已发布(报名中): 5 条")
	log.Println("  已发布(即将开始): 3 条")
	log.Println("  已发布(进行中): 2 条")
	log.Println("  已结束:         4 条")
	log.Println("  草稿:           6 条")
	log.Println("  总计:          20 条")
	log.Println("================================")
	log.Println("🔐 新权限结构说明：")
	log.Println("  第1层: 竞赛管理(competition)、报名管理(registration)、通知管理(notice)、系统管理(system)")
	log.Println("  第2层: 竞赛目录(comp)、赛事申报(declare)、获奖管理(award)等子分类")
	log.Println("  第3层: 具体的操作权限(如 comp:list, reg:audit:update 等)")
	log.Println("================================")
}
