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
	// 1. 初始化权限 (Permission)
	// ==========================================
	perms := []models.Permission{
		// 管理员权限
		{Name: "用户管理", Code: "sys:user:list", Type: 1, Description: "查看用户列表"},
		{Name: "角色管理", Code: "sys:role:list", Type: 1, Description: "查看角色列表"},
		// 赛事负责人权限
		{Name: "发布竞赛", Code: "comp:add", Type: 2, Description: "发布新的学科竞赛"},
		{Name: "审核报名", Code: "comp:audit", Type: 2, Description: "审核学生报名信息"},
		// 学生权限
		{Name: "竞赛报名", Code: "comp:register", Type: 2, Description: "报名参加竞赛"},
	}
	// 批量创建权限
	if err := DB.Create(&perms).Error; err != nil {
		log.Printf("初始化权限失败: %v", err)
	}

	// ==========================================
	// 2. 初始化角色 (Role)
	// ==========================================
	schoolAdminRole := models.Role{RoleName: "校级管理员", RoleCode: "school_admin", Description: "校级系统管理员"}
	collegeAdminRole := models.Role{RoleName: "院级管理员", RoleCode: "college_admin", Description: "学院管理员"}
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
	// 给校级管理员：所有权限
	DB.Model(&schoolAdminRole).Association("Permissions").Append(&perms)

	// 给院级管理员：管理和审核权限
	var collegePerms []models.Permission
	DB.Where("code IN ?", []string{"sys:user:list", "comp:audit"}).Find(&collegePerms)
	DB.Model(&collegeAdminRole).Association("Permissions").Append(&collegePerms)

	// 给赛事负责人：发布 + 审核
	var competitionPerms []models.Permission
	DB.Where("code IN ?", []string{"comp:add", "comp:audit"}).Find(&competitionPerms)
	DB.Model(&competitionManagerRole).Association("Permissions").Append(&competitionPerms)

	// 给学生：报名
	var studentPerms []models.Permission
	DB.Where("code = ?", "comp:register").Find(&studentPerms)
	DB.Model(&studentRole).Association("Permissions").Append(&studentPerms)

	// ==========================================
	// 4. 初始化用户 (User) - 对应前端模拟数据
	// ==========================================
	// 密码统一为 "123"，使用 bcrypt 加密
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

	// 定义用户和角色的映射关系
	userRoleMap := map[string]*models.Role{
		"admin":   &schoolAdminRole,
		"yuan":    &collegeAdminRole,
		"teacher": &competitionManagerRole,
		"student": &studentRole,
		"expert":  &expertRole,
	}

	// 创建用户并分配角色
	for _, u := range users {
		// 先创建用户
		if err := DB.Create(&u).Error; err != nil {
			log.Printf("创建用户 %s 失败: %v", u.Username, err)
			continue
		}

		// 根据用户名分配对应的角色
		if role, ok := userRoleMap[u.Username]; ok {
			err := DB.Model(&u).Association("Roles").Append(role)
			if err != nil {
				log.Printf("为用户 %s 分配角色失败: %v", u.Username, err)
			}
		}
	}

	log.Println("🎉 模拟数据初始化完成！")
	log.Println("--------------------------------")
	log.Println("校级管理员: admin    密码: 123")
	log.Println("院级管理员: yuan     密码: 123")
	log.Println("赛事负责人: teacher  密码: 123")
	log.Println("学生:     student   密码: 123")
	log.Println("专家:     expert    密码: 123")
	log.Println("--------------------------------")
}
