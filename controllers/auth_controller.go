package controllers

import (
	"net/http"

	// 1. 引入你的数据库包
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// LoginInput 接收前端传过来的 JSON 参数
type LoginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 登录接口处理函数
func Login(c *gin.Context) {
	var input LoginInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	var user models.User
	result := database.DB.Preload("Roles").Where("username = ?", input.Username).First(&user)

	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "用户不存在"})
		return
	}

	// 校验密码
	if !checkPassword(user.Password, input.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "密码错误"})
		return
	}

	// 获取角色
	roleCode := "user"
	if len(user.Roles) > 0 {
		roleCode = user.Roles[0].RoleCode
	}

	// 生成 Token
	token, err := utils.GenerateToken(user.ID, user.Username, roleCode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "系统错误: Token生成失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "登录成功",
		"data": gin.H{
			"token": token,
			"userInfo": gin.H{
				"id":       user.ID,
				"username": user.Username,
				"realname": user.Realname,
				"role":     roleCode,
			},
		},
	})
}

// 辅助函数：校验密码
func checkPassword(dbPassword, inputPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(dbPassword), []byte(inputPassword))
	if err == nil {
		return true
	}
	// 兼容明文 (正式上线需删除)
	if dbPassword == inputPassword {
		return true
	}
	return false
}
