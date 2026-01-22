package database

import (
	"CompeManage_backend/models"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// InitData 初始化测试数据
func InitData() {
	// 1. 先检查是否已经有数据了，防止重复插入
	var count int64
	DB.Model(&models.User{}).Count(&count)
	if count > 0 {
		log.Println("数据库已有数据，跳过初始化...")
		return
	}

	log.Println("正在初始化模拟数据...")

	// ==========================================
	// 1. 初始化权限 (Permission) - 构建树形结构
	// ==========================================
	// 结构设计：
	// - 系统管理 (目录)
	//   - 用户管理 (菜单) -> 查看、新增
	//   - 角色管理 (菜单) -> 查看、分配
	// - 竞赛业务 (目录)
	//   - 竞赛列表 (菜单) -> 发布、编辑
	//   - 报名审核 (菜单) -> 审核
	//   - 报名配置 (菜单) -> 查看、编辑  ✨新增
	// - 学生中心 (目录)
	//   - 我的竞赛 (菜单) -> 报名

	// --- Level 1: 根目录 (Type=1, ParentID=0) ---
	sysRoot := models.Permission{Name: "系统管理", Code: "sys: root", Type: 1, ParentID: 0, Description: "系统基础配置目录"}
	compRoot := models.Permission{Name: "竞赛业务", Code: "comp:root", Type: 1, ParentID: 0, Description: "竞赛核心业务目录"}
	stuRoot := models.Permission{Name: "学生中心", Code: "stu:root", Type: 1, ParentID: 0, Description: "学生个人中心目录"}

	DB.Create(&sysRoot)
	DB.Create(&compRoot)
	DB.Create(&stuRoot)

	// --- Level 2: 菜单页面 (Type=2, ParentID=Root. ID) ---

	// 1.1 系统管理下的菜单
	userMenu := models.Permission{Name: "用户管理", Code: "sys:user", Type: 2, ParentID: sysRoot.ID, Description: "用户管理页面"}
	roleMenu := models.Permission{Name: "角色管理", Code: "sys:role", Type: 2, ParentID: sysRoot.ID, Description: "角色管理页面"}
	DB.Create(&userMenu)
	DB.Create(&roleMenu)

	// 1.2 竞赛业务下的菜单
	compListMenu := models.Permission{Name: "竞赛列表", Code: "comp:list", Type: 2, ParentID: compRoot.ID, Description: "竞赛信息列表页面"}
	auditMenu := models.Permission{Name: "报名审核", Code: "comp:audit_page", Type: 2, ParentID: compRoot.ID, Description: "学生报名审核页面"}
	regConfigMenu := models.Permission{Name: "报名配置", Code: "reg:config", Type: 2, ParentID: compRoot.ID, Description: "报名配置管理页面"} // ✨新增
	DB.Create(&compListMenu)
	DB.Create(&auditMenu)
	DB.Create(&regConfigMenu)

	// 1.3 学生中心下的菜单
	myCompMenu := models.Permission{Name: "我的竞赛", Code: "stu:comp", Type: 2, ParentID: stuRoot.ID, Description: "学生参赛记录页面"}
	DB.Create(&myCompMenu)

	// --- Level 3: 按钮/API功能 (Type=3, ParentID=Menu.ID) ---

	// 用户管理下的功能
	perms := []models.Permission{
		{Name: "查看用户", Code: "sys:user:list", Type: 3, ParentID: userMenu.ID},
		{Name: "新增用户", Code: "sys: user:add", Type: 3, ParentID: userMenu.ID},

		// 角色管理下的功能
		{Name: "查看角色", Code: "sys:role:list", Type: 3, ParentID: roleMenu.ID},
		{Name: "分配权限", Code: "sys:role:assign", Type: 3, ParentID: roleMenu.ID},

		// 竞赛列表下的功能
		{Name: "发布竞赛", Code: "comp:add", Type: 3, ParentID: compListMenu.ID},
		{Name: "编辑竞赛", Code: "comp:edit", Type: 3, ParentID: compListMenu.ID},

		// 审核页面下的功能
		{Name: "审核操作", Code: "comp:audit", Type: 3, ParentID: auditMenu.ID},

		// ✨新增：报名配置下的功能 (对应路由中间件保护)
		{Name: "查看报名配置", Code: "reg:config:view", Type: 3, ParentID: regConfigMenu.ID},
		{Name: "编辑报名配置", Code: "reg:config:edit", Type: 3, ParentID: regConfigMenu.ID},

		// 我的竞赛下的功能
		{Name: "立即报名", Code: "comp:register", Type: 3, ParentID: myCompMenu.ID},
	}
	DB.Create(&perms)

	// ==========================================
	// 2. 初始化角色 (Role)
	// ==========================================
	schoolAdminRole := models.Role{RoleName: "校级管理员", RoleCode: "school_admin", Description: "校级系统管理员，拥有所有权限"}
	collegeAdminRole := models.Role{RoleName: "院级管理员", RoleCode: "college_admin", Description: "学院管理员，负责用户和审核"}
	competitionManagerRole := models.Role{RoleName: "赛事负责人", RoleCode: "competition_manager", Description: "负责发布和审核竞赛"}
	studentRole := models.Role{RoleName: "学生", RoleCode: "student", Description: "参与竞赛"}
	expertRole := models.Role{RoleName: "专家", RoleCode: "expert", Description: "评审竞赛"}

	DB.Create(&schoolAdminRole)
	DB.Create(&collegeAdminRole)
	DB.Create(&competitionManagerRole)
	DB.Create(&studentRole)
	DB.Create(&expertRole)

	// ==========================================
	// 3. 关联角色与权限 (Role-Permission)
	// ==========================================

	// --- A.  校级管理员：给所有权限 ---
	var allPerms []models.Permission
	DB.Find(&allPerms)
	DB.Model(&schoolAdminRole).Association("Permissions").Append(&allPerms)

	// --- B. 院级管理员：系统管理(部分) + 竞赛审核 + 报名配置查看 ---
	var collegePerms []models.Permission
	DB.Where("code IN ? ", []string{
		"sys:root", "sys:user", "sys:user:list",
		"comp:root", "comp:audit_page", "comp:audit",
		"reg:config", "reg:config:view", // ✨新增：可以查看报名配置
	}).Find(&collegePerms)
	DB.Model(&collegeAdminRole).Association("Permissions").Append(&collegePerms)

	// --- C. 赛事负责人：竞赛列表(发布/编辑) + 审核 + 报名配置(查看/编辑) ---
	var managerPerms []models.Permission
	DB.Where("code IN ? ", []string{
		"comp:root",
		"comp:list", "comp:add", "comp: edit",
		"comp:audit_page", "comp:audit",
		"reg:config", "reg:config:view", "reg:config:edit", // ✨新增：可以查看和编辑报名配置
	}).Find(&managerPerms)
	DB.Model(&competitionManagerRole).Association("Permissions").Append(&managerPerms)

	// --- D. 学生：学生中心 ---
	var studentPerms []models.Permission
	DB.Where("code IN ?", []string{
		"stu:root", "stu:comp", "comp:register",
	}).Find(&studentPerms)
	DB.Model(&studentRole).Association("Permissions").Append(&studentPerms)

	// ==========================================
	// 4. 初始化用户 (User)
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
		{Username: "admin", Realname: "校级管理员", Password: hashPassword("123")},
		{Username: "yuan", Realname: "计算机学院管理员", Password: hashPassword("123")},
		{Username: "teacher", Realname: "张老师(赛事负责人)", Password: hashPassword("123")},
		{Username: "student", Realname: "李同学", Password: hashPassword("123")},
		{Username: "expert", Realname: "王专家", Password: hashPassword("123")},
		// ✨新增：无权限测试用户
		{Username: "guest", Realname: "访客用户", Password: hashPassword("123")},
	}

	userRoleMap := map[string]*models.Role{
		"admin":   &schoolAdminRole,
		"yuan":    &collegeAdminRole,
		"teacher": &competitionManagerRole,
		"student": &studentRole,
		"expert":  &expertRole,
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

	// ==========================================
	// 5. 初始化竞赛目录与详情 (扩展为10条数据)
	// ==========================================

	// 5.1 获取"张老师"的ID，作为负责人
	var teacherUser models.User
	DB.Where("username = ?", "teacher").First(&teacherUser)

	// 5.2 定义 2026 年的时间点 (基于当前日期 2026-01-22)
	// 使用固定日期确保数据有效性
	baseTime := time.Date(2026, 1, 22, 0, 0, 0, 0, time.Local)

	// 5.3 创建竞赛数据 - 扩展为10条，覆盖各种状态和时间段
	competitions := []models.CompDirectory{
		// ========== 状态=1 (已发布) - 正在报名中 ==========
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
				RegStartTime:       baseTime.AddDate(0, 0, -10), // 10天前开始报名
				RegEndTime:         baseTime.AddDate(0, 1, 0),   // 1个月后结束报名
				CompStartTime:      baseTime.AddDate(0, 2, 0),   // 2个月后开始比赛
				CompEndTime:        baseTime.AddDate(0, 3, 0),   // 3个月后结束比赛
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
				RegStartTime:       baseTime.AddDate(0, 0, -20), // 20天前开始
				RegEndTime:         baseTime.AddDate(0, 0, 10),  // 10天后结束
				CompStartTime:      baseTime.AddDate(0, 1, 15),  // 1个半月后比赛
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
			CollegeID:  2, // 数学学院
			Year:       2026,
			ManagerID:  teacherUser.ID,
			Status:     1,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, 0, -5),
				RegEndTime:         baseTime.AddDate(0, 0, 20),
				CompStartTime:      baseTime.AddDate(0, 1, 0),
				CompEndTime:        baseTime.AddDate(0, 1, 4), // 4天比赛
				ParticipantType:    2,
				MaxTeamMember:      3,
				MinTeamMember:      3,
				GradeRequirement:   "[2022,2023,2024]",
				RegistrationMethod: "三人组队，需有指导老师，提交英文论文。",
				NeedAttachment:     1,
				NeedAdvisor:        2,
			},
		},
		// ========== 状态=1 (已发布) - 报名即将开始 ==========
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
				RegStartTime:       baseTime.AddDate(0, 0, 30), // 30天后开始报名
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
		// ========== 状态=1 (已发布) - 报名已结束，比赛进行中 ==========
		{
			CompCode:   "ROBOCON-2026",
			CompName:   "2026年全国大学生机器人大赛RoboMaster",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "共青团中央",
			Undertaker: "大疆创新",
			CollegeID:  3, // 机械学院
			Year:       2026,
			ManagerID:  teacherUser.ID,
			Status:     1,
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       baseTime.AddDate(0, -2, 0), // 2个月前开始
				RegEndTime:         baseTime.AddDate(0, -1, 0), // 1个月前结束
				CompStartTime:      baseTime.AddDate(0, 0, -5), // 5天前开始比赛
				CompEndTime:        baseTime.AddDate(0, 0, 10), // 10天后结束
				ParticipantType:    2,
				MaxTeamMember:      20,
				MinTeamMember:      10,
				GradeRequirement:   "[]",
				RegistrationMethod: "战队形式参赛，需提交技术方案书。",
				NeedAttachment:     2,
				NeedAdvisor:        2,
			},
		},
		// ========== 状态=2 (已结束) ==========
		{
			CompCode:   "CUMCM-2025",
			CompName:   "2025年全国大学生数学建模竞赛",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "中国工业与应用数学学会",
			Undertaker: "数学建模组委会",
			CollegeID:  2,
			Year:       2025,
			ManagerID:  teacherUser.ID,
			Status:     2, // 已结束
			CreatedBy:  teacherUser.ID,
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
		// ========== 状态=0 (草稿) ==========
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
				RegistrationMethod: "待定.. .",
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
			ManagerID:  teacherUser.ID,
			Status:     0,
			CreatedBy:  teacherUser.ID,
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
			ManagerID:  teacherUser.ID,
			Status:     0,
			CreatedBy:  teacherUser.ID,
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
	}

	// 批量创建竞赛 (GORM 会自动处理 Detail 的关联插入)
	if err := DB.Create(&competitions).Error; err != nil {
		log.Printf("创建竞赛数据失败: %v", err)
	}

	log.Println("🎉 树形权限与竞赛测试数据初始化完成！")
	log.Println("================================")
	log.Println("📋 用户账号信息：")
	log.Println("  校级管理员:  admin    密码: 123 (拥有所有权限)")
	log.Println("  院级管理员: yuan     密码: 123 (用户管理+审核+报名配置查看)")
	log.Println("  赛事负责人: teacher  密码: 123 (竞赛管理+审核+报名配置编辑)")
	log.Println("  学生:        student  密码: 123 (仅报名权限)")
	log.Println("  专家:       expert   密码: 123 (无特殊权限)")
	log.Println("  访客:       guest    密码:  123 (无任何权限-测试用)")
	log.Println("================================")
	log.Println("🔐 权限验证测试建议：")
	log.Println("  1. 用 teacher 登录 -> 可访问 /api/reg/config")
	log.Println("  2. 用 yuan 登录    -> 只能 GET /api/reg/config/get")
	log.Println("  3. 用 student 登录 -> 无法访问 /api/reg/* (403)")
	log.Println("  4. 用 guest 登录   -> 无法访问任何受保护路由 (403)")
	log.Println("================================")
}
