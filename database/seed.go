package database

import (
	"CompeManage_backend/models"
	"log"
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
		// 教师权限
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
	adminRole := models.Role{RoleName: "超级管理员", RoleCode: "admin", Description: "系统最高权限"}
	teacherRole := models.Role{RoleName: "指导教师", RoleCode: "teacher", Description: "负责发布和审核"}
	studentRole := models.Role{RoleName: "学生", RoleCode: "student", Description: "参与竞赛"}

	DB.Create(&adminRole)
	DB.Create(&teacherRole)
	DB.Create(&studentRole)

	// ==========================================
	// 3. 关联角色与权限 (Role-Permission)
	// ==========================================
	// 给管理员：所有权限
	DB.Model(&adminRole).Association("Permissions").Append(&perms)

	// 给教师：发布 + 审核
	// 我们需要从切片里挑出对应的权限
	var teacherPerms []models.Permission
	DB.Where("code IN ?", []string{"comp:add", "comp:audit"}).Find(&teacherPerms)
	DB.Model(&teacherRole).Association("Permissions").Append(&teacherPerms)

	// 给学生：报名
	var studentPerms []models.Permission
	DB.Where("code = ?", "comp:register").Find(&studentPerms)
	DB.Model(&studentRole).Association("Permissions").Append(&studentPerms)

	// ==========================================
	// 4. 初始化用户 (User)
	// ==========================================
	// 统一密码：123456 (必须加密!)
	password := ("123456")

	users := []models.User{
		{Username: "admin", Realname: "系统管理员", Password: password},
		{Username: "1001", Realname: "王老师", Password: password},     // 模拟工号
		{Username: "2023001", Realname: "张三同学", Password: password}, // 模拟学号
	}

	// 创建用户并分配角色
	// 注意：这里我们演示 Many2Many 的分配方式
	for _, u := range users {
		// 先创建用户
		if err := DB.Create(&u).Error; err != nil {
			log.Printf("创建用户 %s 失败: %v", u.Username, err)
			continue
		}

		// 根据用户名分配对应的角色
		var targetRole models.Role
		if u.Username == "admin" {
			targetRole = adminRole
		} else if u.Username == "1001" {
			targetRole = teacherRole
		} else {
			targetRole = studentRole
		}

		// 写入 user_roles 中间表
		err := DB.Model(&u).Association("Roles").Append(&targetRole)
		if err != nil {
			log.Printf("分配角色失败: %v", err)
		}
	}

	log.Println("🎉 模拟数据初始化完成！")
	log.Println("--------------------------------")
	log.Println("管理员账号: admin   密码: 123456")
	log.Println("教师账号:   1001    密码: 123456")
	log.Println("学生账号:   2023001 密码: 123456")
	log.Println("--------------------------------")
}
