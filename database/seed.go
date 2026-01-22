package database

import (
	"CompeManage_backend/models"
	"log"

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

	DB.Create(&sysRoot)  // 插入后 sysRoot.ID 会有值
	DB.Create(&compRoot) // 插入后 compRoot.ID 会有值
	DB.Create(&stuRoot)  // 插入后 stuRoot.ID 会有值

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

	log.Println(">>> 权限树初始化完成")

	// ==========================================
	// 2. 初始化角色 (Role) - 保持不变
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
	DB.Find(&allPerms) // 查询刚才插入的所有权限
	DB.Model(&schoolAdminRole).Association("Permissions").Append(&allPerms)

	// --- B. 院级管理员：系统管理(部分) + 竞赛审核 ---
	// 需要给父节点(目录/菜单)权限，否则前端菜单出不来
	var collegePerms []models.Permission
	DB.Where("code IN ?", []string{
		"sys:root", "sys:user", "sys:user:list", // 系统->用户->查看
		"comp:root", "comp:audit_page", "comp:audit", // 竞赛->审核->操作
	}).Find(&collegePerms)
	DB.Model(&collegeAdminRole).Association("Permissions").Append(&collegePerms)

	// --- C. 赛事负责人：竞赛列表(发布/编辑) + 审核 ---
	var managerPerms []models.Permission
	DB.Where("code IN ?", []string{
		"comp:root",
		"comp:list", "comp:add", "comp:edit", // 竞赛->列表->发布/编辑
		"comp:audit_page", "comp:audit", // 竞赛->审核->操作
	}).Find(&managerPerms)
	DB.Model(&competitionManagerRole).Association("Permissions").Append(&managerPerms)

	// --- D. 学生：学生中心 ---
	var studentPerms []models.Permission
	DB.Where("code IN ?", []string{
		"stu:root", "stu:comp", "comp:register", // 学生->我的竞赛->报名
	}).Find(&studentPerms)
	DB.Model(&studentRole).Association("Permissions").Append(&studentPerms)

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

	log.Println("🎉 树形权限数据初始化完成！")
}
