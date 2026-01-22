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
	// - 学生中心 (目录)
	//   - 我的竞赛 (菜单) -> 报名

	// --- Level 1: 根目录 (Type=1, ParentID=0) ---
	sysRoot := models.Permission{Name: "系统管理", Code: "sys:root", Type: 1, ParentID: 0, Description: "系统基础配置目录"}
	compRoot := models.Permission{Name: "竞赛业务", Code: "comp:root", Type: 1, ParentID: 0, Description: "竞赛核心业务目录"}
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
	DB.Create(&compListMenu)
	DB.Create(&auditMenu)

	// 1.3 学生中心下的菜单
	myCompMenu := models.Permission{Name: "我的竞赛", Code: "stu:comp", Type: 2, ParentID: stuRoot.ID, Description: "学生参赛记录页面"}
	DB.Create(&myCompMenu)

	// --- Level 3: 按钮/API功能 (Type=3, ParentID=Menu.ID) ---

	// 用户管理下的功能
	perms := []models.Permission{
		{Name: "查看用户", Code: "sys:user:list", Type: 3, ParentID: userMenu.ID},
		{Name: "新增用户", Code: "sys:user:add", Type: 3, ParentID: userMenu.ID},

		// 角色管理下的功能
		{Name: "查看角色", Code: "sys:role:list", Type: 3, ParentID: roleMenu.ID},
		{Name: "分配权限", Code: "sys:role:assign", Type: 3, ParentID: roleMenu.ID},

		// 竞赛列表下的功能
		{Name: "发布竞赛", Code: "comp:add", Type: 3, ParentID: compListMenu.ID},
		{Name: "编辑竞赛", Code: "comp:edit", Type: 3, ParentID: compListMenu.ID},

		// 审核页面下的功能
		{Name: "审核操作", Code: "comp:audit", Type: 3, ParentID: auditMenu.ID},

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

	// --- A. 校级管理员：给所有权限 ---
	var allPerms []models.Permission
	DB.Find(&allPerms)
	DB.Model(&schoolAdminRole).Association("Permissions").Append(&allPerms)

	// --- B. 院级管理员：系统管理(部分) + 竞赛审核 ---
	var collegePerms []models.Permission
	DB.Where("code IN ?", []string{
		"sys:root", "sys:user", "sys:user:list",
		"comp:root", "comp:audit_page", "comp:audit",
	}).Find(&collegePerms)
	DB.Model(&collegeAdminRole).Association("Permissions").Append(&collegePerms)

	// --- C. 赛事负责人：竞赛列表(发布/编辑) + 审核 ---
	var managerPerms []models.Permission
	DB.Where("code IN ?", []string{
		"comp:root",
		"comp:list", "comp:add", "comp:edit",
		"comp:audit_page", "comp:audit",
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
	}

	userRoleMap := map[string]*models.Role{
		"admin":   &schoolAdminRole,
		"yuan":    &collegeAdminRole,
		"teacher": &competitionManagerRole,
		"student": &studentRole,
		"expert":  &expertRole,
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
	// 5. 初始化竞赛目录与详情 (NEW: 新增部分)
	// ==========================================

	// 5.1 获取“张老师”的ID，作为负责人
	var teacherUser models.User
	DB.Where("username = ?", "teacher").First(&teacherUser)

	// 5.2 定义时间辅助变量
	now := time.Now()
	oneMonthLater := now.AddDate(0, 1, 0)
	twoMonthsLater := now.AddDate(0, 2, 0)

	// 5.3 创建竞赛数据
	// 注意：这里同时创建 CompDirectory 和 CompDetail
	// GORM 会自动将 CompDirectory 的 ID 填入 CompDetail 的 CompID 中
	competitions := []models.CompDirectory{
		{
			CompCode:   "NCD-2026",
			CompName:   "2026年全国大学生计算机设计大赛",
			CompType:   "A类",
			CompLevel:  "国家级",
			Organizer:  "教育部计算机相关教指委",
			Undertaker: "厦门大学",
			CollegeID:  1, // 假设计算机学院ID为1
			Year:       2026,
			ManagerID:  teacherUser.ID,
			Status:     1, // 状态: 发布
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       now,
				RegEndTime:         oneMonthLater,
				CompStartTime:      oneMonthLater,
				CompEndTime:        twoMonthsLater,
				ParticipantType:    2, // 团队
				MaxTeamMember:      3,
				MinTeamMember:      1,
				GradeRequirement:   "2023,2024,2025",
				RegistrationMethod: "请各参赛队登录国赛官网报名，并在此系统提交校内审核材料。",
				NeedAttachment:     1, // 需要附件
				NeedAdvisor:        1, // 需要指导老师
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
			Status:     1, // 状态: 发布
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime:       now.AddDate(0, 0, -10), // 10天前开始
				RegEndTime:         now.AddDate(0, 0, 20),
				CompStartTime:      oneMonthLater,
				CompEndTime:        oneMonthLater,
				ParticipantType:    1, // 个人
				MaxTeamMember:      1,
				MinTeamMember:      1,
				GradeRequirement:   "不限",
				RegistrationMethod: "个人赛，C/C++或Java组，直接在线报名。",
				NeedAttachment:     0, // 不需要附件
				NeedAdvisor:        0, // 不需要指导老师
			},
		},
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
			Status:     0, // 状态: 草稿
			CreatedBy:  teacherUser.ID,
			Detail: models.CompDetail{
				RegStartTime: now,
				RegEndTime:   oneMonthLater,

				// ✨✨ 修复点：必须加上这两个字段，防止出现 0000-00-00 错误 ✨✨
				CompStartTime: oneMonthLater, // 暂定一个月后
				CompEndTime:   oneMonthLater, // 暂定一个月后

				ParticipantType:    1,
				RegistrationMethod: "待定...",
			},
		},
	}

	// 批量创建竞赛 (GORM 会自动处理 Detail 的关联插入)
	if err := DB.Create(&competitions).Error; err != nil {
		log.Printf("创建竞赛数据失败: %v", err)
	}

	log.Println("🎉 树形权限与竞赛测试数据初始化完成！")
}
