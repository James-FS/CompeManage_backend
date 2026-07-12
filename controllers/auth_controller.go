package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type LoginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func Login(c *gin.Context) {
	var input LoginInput

	if err := c.ShouldBindJSON(&input); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}

	var user models.User
	result := database.DB.WithContext(c.Request.Context()).Preload("Roles").Where("username = ?", input.Username).First(&user)

	if result.Error != nil {
		utils.Unauthorized(c, "用户不存在")
		return
	}

	if !checkPassword(user.Password, input.Password) {
		utils.Unauthorized(c, "密码错误")
		return
	}

	if len(user.Roles) == 0 {
		utils.Forbidden(c, "账号尚未分配角色，请联系管理员")
		return
	}
	if len(user.Roles) > 1 {
		utils.InternalServerError(c, "账号存在多个角色，请联系管理员处理", database.ErrUserHasMultipleRoles)
		return
	}
	roleCode := user.Roles[0].RoleCode

	token, err := utils.GenerateToken(user.ID, user.Username, roleCode)
	if err != nil {
		utils.InternalServerError(c, "系统错误: Token生成失败", err)
		return
	}

	utils.Success(c, gin.H{
		"token": token,
		"userInfo": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"realname": user.Realname,
			"role":     roleCode,
		},
	})
}

func checkPassword(dbPassword, inputPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(dbPassword), []byte(inputPassword))
	return err == nil
}
