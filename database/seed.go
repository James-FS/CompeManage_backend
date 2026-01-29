package database

import (
	"CompeManage_backend/models"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
)

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
		return
	}

	log.Println("正在初始化扩展版模拟数据...")

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
	//   - 报名配置 (菜单) -> 查看、编辑
	// - 学生中心 (目录)
	//   - 我的竞赛 (菜单) -> 报名

	// --- Level 1: 根目录 (Type=1, ParentID=0) ---
	sysRoot := models.Permission{Name: "系统管理", Code: "sys:root", Type: 1, ParentID: 0, Description: "系统基础配置目录"}
	compRoot := models.Permission{Name: "竞赛业务", Code: "comp:root", Type: 1, ParentID: 0, Description: "竞赛核心���务目录"}
	stuRoot := models.Permission{Name: "学生中心", Code: "stu:root", Type: 1, ParentID: 0, Description: "学生个人中心目录"}

	DB.Create(&sysRoot)
	DB.Create(&compRoot)
	DB.Create(&stuRoot)

	// --- Level 2: 菜单页面 (Type=2, ParentID=Root.ID) ---

	// 1.1 系统管理下的菜单
	userMenu := models.Permission{Name: "用户管理", Code: "sys:user", Type: 2, ParentID: sysRoot.ID, Description: "用户管理页面"}
	roleMenu := models.Permission{Name: "角色管理", Code: "sys:role", Type: 2, ParentID: sysRoot.ID, Description: "角色管理页面"}
	DB.Create(&userMenu)
	DB.Create(&roleMenu)

	// 1.2 竞赛业务下的菜单
	compListMenu := models.Permission{Name: "竞赛列表", Code: "comp:list", Type: 2, ParentID: compRoot.ID, Description: "竞赛信息列表页面"}
	auditMenu := models.Permission{Name: "报名审核", Code: "comp:audit_page", Type: 2, ParentID: compRoot.ID, Description: "学生报名审核页面"}
	regConfigMenu := models.Permission{Name: "报名配置", Code: "reg:config", Type: 2, ParentID: compRoot.ID, Description: "报名配置管理页面"}
	DB.Create(&compListMenu)
	DB.Create(&auditMenu)
	DB.Create(&regConfigMenu)

	// 1.3 学生中心下的菜单
	myCompMenu := models.Permission{Name: "我的竞赛", Code: "stu:comp", Type: 2, ParentID: stuRoot.ID, Description: "学生参赛记录页面"}
	DB.Create(&myCompMenu)

	// --- Level 3: 按钮/API功能 (Type=3, ParentID=Menu.ID) ---
	// 用户管理下的功能
	perms := []models.Permission{
		// 用户管理 & 角色管理 (保持不变) ...
		{Name: "查看用户", Code: "sys:user:list", Type: 3, ParentID: userMenu.ID},
		{Name: "新增用户", Code: "sys:user:add", Type: 3, ParentID: userMenu.ID},
		{Name: "查看角色", Code: "sys:role:list", Type: 3, ParentID: roleMenu.ID},
		{Name: "分配权限", Code: "sys:role:assign", Type: 3, ParentID: roleMenu.ID},

		// 竞赛列表 (保持不变)
		{Name: "发布竞赛", Code: "comp:add", Type: 3, ParentID: compListMenu.ID},
		{Name: "编辑竞赛", Code: "comp:edit", Type: 3, ParentID: compListMenu.ID},

		// ⚠️【修改】审核页面下的功能 (匹配 routes.go)
		{Name: "审核列表", Code: "reg:audit:list", Type: 3, ParentID: auditMenu.ID},   // 对应 GET /api/reg/list
		{Name: "审核详情", Code: "reg:audit:detail", Type: 3, ParentID: auditMenu.ID}, // 对应 GET /api/reg/detail
		{Name: "审核操作", Code: "reg:audit:update", Type: 3, ParentID: auditMenu.ID}, // 对应 PUT /api/reg/audit

		// 报名配置 (保持不变)
		{Name: "编辑报名配置", Code: "reg:config:edit", Type: 3, ParentID: regConfigMenu.ID},
		{Name: "查看报名配置", Code: "reg:config:view", Type: 3, ParentID: regConfigMenu.ID},

		// ⚠️【修改】我的竞赛下的功能 (匹配 routes.go)
		{Name: "立即报名", Code: "reg:config:submit", Type: 3, ParentID: myCompMenu.ID}, // 对应 POST /api/reg/submit
	}
	DB.Create(&perms)

	// ==========================================
	// 2. 初始化角色 (Role)
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
	// 3. 关联角色与权限 (Role-Permission)
	// ==========================================

	// --- A. 校级管理员：给所有权限 ---
	var allPerms []models.Permission
	DB.Find(&allPerms)
	DB.Model(&schoolAdminRole).Association("Permissions").Append(&allPerms)

	// --- B. 院级管理员：增加审核相关权限 ---
	var collegePerms []models.Permission
	DB.Where("code IN ?", []string{
		"sys:root", "sys:user", "sys:user:list",
		"comp:root", "comp:audit_page",
		// 👇 修改这里：使用新的 code
		"reg:audit:list", "reg:audit:detail", "reg:audit:update",
		"reg:config", "reg:config:view",
	}).Find(&collegePerms)
	DB.Model(&collegeAdminRole).Association("Permissions").Append(&collegePerms)

	// --- C. 赛事负责人：增加审核相关权限 ---
	var managerPerms []models.Permission
	DB.Where("code IN ?", []string{
		"comp:root",
		"comp:list", "comp:add", "comp:edit",
		"comp:audit_page",
		// 👇 修改这里：使用新的 code
		"reg:audit:list", "reg:audit:detail", "reg:audit:update",
		"reg:config", "reg:config:view", "reg:config:edit",
	}).Find(&managerPerms)
	DB.Model(&competitionManagerRole).Association("Permissions").Append(&managerPerms)

	// --- D. 学生：修改报名权限 ---
	var studentPerms []models.Permission
	DB.Where("code IN ?", []string{
		"stu:root", "stu:comp",
		// 👇 修改这里：使用新的 code
		"reg:config:submit",
	}).Find(&studentPerms)
	DB.Model(&studentRole).Association("Permissions").Append(&studentPerms)

	// --- E. 专家：竞赛查看权限 ---
	var expertPerms []models.Permission
	DB.Where("code IN ?", []string{
		"comp:root", "comp:list",
	}).Find(&expertPerms)
	DB.Model(&expertRole).Association("Permissions").Append(&expertPerms)

	// --- F. 访客：无特殊权限 ---
	// 不分配任何权限

	// ==========================================
	// 4. 初始化用户 (User) - 每个角色至少一个账号
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
		// ✨新增：无权限测试用户
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
	// 5. 初始化竞赛目录与详情 (扩展为20条数据)
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
	for compCode, detail := range detailUpdates {
		var comp models.CompDirectory
		// 通过CompCode查询竞赛
		if err := DB.Where("comp_code = ?", compCode).First(&comp).Error; err != nil {
			log.Printf("⚠️  查询竞赛 %s 失败: %v", compCode, err)
			continue
		}

		// 检查是否已经有详情
		var existingDetail models.CompDetail
		if err := DB.Where("comp_id = ?", comp.ID).First(&existingDetail).Error; err == nil {
			// 详情已存在，跳过
			log.Printf("   竞赛 %s 的详情已存在，跳过创建", compCode)
			detailCount++
			continue
		}

		// 创建新详情
		detail.CompID = comp.ID
		if err := DB.Create(&detail).Error; err != nil {
			log.Printf("❌ 创建竞赛详情 %s 失败: %v", compCode, err)
		} else {
			log.Printf("✅ 创建竞赛详情 %s 成功 (CompID: %d)", compCode, comp.ID)
			detailCount++
		}
	}
	log.Printf("✅ 竞赛详情创建完成: %d/%d 条成功", detailCount, len(detailUpdates))

	log.Println("🎉 树形权限与竞赛测试数据初始化完成！")
	log.Println("================================")
	log.Println("📋 用户账号信息：")
	log.Println("  校级管理员: T2023001    密码: 123 (拥有所有权限)")
	log.Println("  院级管理员: T2023002     密码: 123 (用户管理+审核+报名配置查看)")
	log.Println("  赛事负责人: T2023003  密码: 123 (竞赛管理+审核+报名配置编辑)")
	log.Println("  学生:       S2024001  密码: 123 (仅报名权限)")
	log.Println("  专家:       E2023001   密码: 123 (无特殊权限)")
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
	log.Println("🔐 路由权限对应关系 (RequirePermission 中间件)：")
	log.Println("  POST /api/reg/config     -> reg:config:edit (赛事负责人+校级管理员)")
	log.Println("  GET  /api/reg/config/get -> reg:config:view (赛事负责人+院级管理员+校级管理员)")
	log.Println("================================")
	log.Println("🧪 权限验证测试建议：")
	log.Println("  1. teacher 登录   -> 可访问 POST/GET /api/reg/config*")
	log.Println("  2. yuan 登录      -> 只能 GET /api/reg/config/get")
	log.Println("  3. student 登录   -> 无法访问 /api/reg/* (403)")
	log.Println("  4. guest 登录     -> 无法访问任何受保护路由 (403)")
	log.Println("================================")
}
