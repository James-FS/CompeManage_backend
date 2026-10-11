package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errYearExists   = errors.New("该年度目录已存在")
	errYearOccupied = errors.New("该年度包含赛事（包括可恢复的已删除赛事），不能修改或删除")
)

func parseDirectoryYear(value string) (int, bool) {
	if len(value) != 4 {
		return 0, false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
	}
	year, err := strconv.Atoi(value)
	return year, err == nil && year >= 1000 && year <= 9999
}

func bindDirectoryYear(c *gin.Context) (int, bool) {
	var req struct {
		Year string `json:"year"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请提供4位年份字符串")
		return 0, false
	}
	year, valid := parseDirectoryYear(req.Year)
	if !valid {
		utils.BadRequest(c, "年份须为1000至9999的4位数字")
	}
	return year, valid
}

// 锁定年份索引范围，将检查和修改放在同一事务内；包括软删除数据。
func directoryHasCompetitions(tx *gorm.DB, year int) (bool, error) {
	var ids []uint
	err := tx.Unscoped().Model(&models.CompDirectory{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).Where("year = ?", year).Pluck("id", &ids).Error
	return len(ids) > 0, err
}

func respondDirectoryError(c *gin.Context, err error) {
	var mysqlErr *mysql.MySQLError
	switch {
	case errors.Is(err, errYearExists), errors.As(err, &mysqlErr) && mysqlErr.Number == 1062:
		utils.BadRequest(c, errYearExists.Error())
	case errors.Is(err, errYearOccupied):
		utils.BadRequest(c, errYearOccupied.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		utils.NotFound(c, "年度目录不存在，请刷新列表")
	default:
		utils.InternalServerError(c, "年度目录操作失败", err)
	}
}

func CreateCompetitionYear(c *gin.Context) {
	year, valid := bindDirectoryYear(c)
	if !valid {
		return
	}
	err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		occupied, err := directoryHasCompetitions(tx, year)
		if err != nil {
			return err
		}
		if occupied {
			return errYearExists
		}
		return tx.Create(&models.CompetitionYear{Year: year}).Error
	})
	if err != nil {
		respondDirectoryError(c, err)
		return
	}
	utils.Success(c, gin.H{"year": year})
}

func UpdateCompetitionYear(c *gin.Context) {
	mutateCompetitionYear(c, false)
}

func DeleteCompetitionYear(c *gin.Context) {
	mutateCompetitionYear(c, true)
}

func mutateCompetitionYear(c *gin.Context, deleting bool) {
	oldYear, valid := parseDirectoryYear(c.Param("year"))
	if !valid {
		utils.BadRequest(c, "年份须为1000至9999的4位数字")
		return
	}
	newYear := oldYear
	if !deleting {
		newYear, valid = bindDirectoryYear(c)
		if !valid {
			return
		}
	}
	err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var directory models.CompetitionYear
		lookupErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&directory, "year = ?", oldYear).Error
		if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}
		occupied, err := directoryHasCompetitions(tx, oldYear)
		if err != nil {
			return err
		}
		if occupied {
			return errYearOccupied
		}
		if lookupErr != nil {
			return lookupErr
		}
		if deleting {
			return tx.Delete(&directory).Error
		}
		if newYear == oldYear {
			return nil
		}
		occupied, err = directoryHasCompetitions(tx, newYear)
		if err != nil {
			return err
		}
		if occupied {
			return errYearExists
		}
		return tx.Model(&directory).Update("year", newYear).Error
	})
	if err != nil {
		respondDirectoryError(c, err)
		return
	}
	utils.Success(c, nil)
}
