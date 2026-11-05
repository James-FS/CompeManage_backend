package controllers

import (
	"CompeManage_backend/database"
	"CompeManage_backend/models"
	"CompeManage_backend/utils"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func resolveStudentAwardAdvisors(tx *gorm.DB, input []models.AdvisorInfo) ([]models.AdvisorInfo, error) {
	if len(input) == 0 {
		return nil, errors.New("请选择指导老师")
	}
	pairs := make([]models.AdvisorInfo, 0, len(input))
	seen := map[string]bool{}
	for _, a := range input {
		username := strings.TrimSpace(a.Username)
		if username == "" || seen[username] {
			return nil, errors.New("指导老师工号不能为空或重复")
		}
		seen[username] = true
		user, err := findAwardTeacher(tx, username)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, models.AdvisorInfo{ID: user.ID, Username: user.Username, Name: user.Realname, College: user.College})
	}
	return pairs, nil
}

func saveStudentAwardAdvisors(tx *gorm.DB, awardID uint, pairs []models.AdvisorInfo) error {
	if err := tx.Unscoped().Where("award_id = ?", awardID).Delete(&models.AwardAdvisor{}).Error; err != nil {
		return err
	}
	for i, a := range pairs {
		if err := tx.Create(&models.AwardAdvisor{AwardID: awardID, Username: a.Username, Name: a.Name, College: a.College, Sort: i}).Error; err != nil {
			return err
		}
	}
	return nil
}

// The role grants entry; the advisor relation determines which records are accessible.
func requireAwardAdvisorUser(c *gin.Context) (models.User, bool) {
	id, ok := c.Get("user_id")
	userID, valid := id.(uint)
	if !ok || !valid || userID == 0 {
		utils.Unauthorized(c, "请先登录")
		return models.User{}, false
	}
	role := c.GetString("role_code")
	if role != "teacher" && role != "competition_manager" {
		utils.Forbidden(c, "没有指导获奖权限")
		return models.User{}, false
	}
	var user models.User
	if err := database.DB.WithContext(c.Request.Context()).Select("id", "username", "realname", "identity_type").First(&user, userID).Error; err != nil {
		utils.InternalServerError(c, "查询教师身份失败", err)
		return user, false
	}
	if user.IdentityType != "staff" {
		utils.Forbidden(c, "仅教职工可访问指导获奖")
		return user, false
	}
	return user, true
}

func advisorAwardScope(db *gorm.DB, username string) *gorm.DB {
	return db.Where(`EXISTS (SELECT 1 FROM award_advisors aa WHERE aa.award_id = awards.id AND aa.username = ? AND aa.delete_time IS NULL) AND EXISTS (SELECT 1 FROM comp_directories cd WHERE cd.id = awards.comp_id AND cd.delete_time IS NULL)`, username)
}

func advisorCanEditAward(a models.Award) bool {
	return a.Source == "supplement" && (a.Status == "draft" || a.Status == "rejected")
}

func awardEditConflict(c *gin.Context) {
	c.JSON(409, utils.Response{Code: 409, Message: "填报已被他人修改或审核，请刷新后重试", Timestamp: time.Now().Unix()})
}

// Lock the entire selection before auditing any row. Older clients may omit
// versions; current clients send the versions displayed to the reviewer.
func checkAwardAuditVersions(tx *gorm.DB, ids []uint, versions map[uint]time.Time) error {
	if versions == nil {
		return nil
	}
	var awards []models.Award
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Order("id ASC").Find(&awards).Error; err != nil {
		return err
	}
	seen := map[uint]bool{}
	for _, a := range awards {
		version, exists := versions[a.ID]
		if !exists || a.Status != "draft" || !a.UpdatedAt.Equal(version) {
			return errAwardAuditConflict
		}
		seen[a.ID] = true
	}
	for _, id := range ids {
		if !seen[id] {
			return errAwardAuditConflict
		}
	}
	return nil
}

var errAwardAuditConflict = errors.New("获奖审核版本已变化")

func requireAwardAuditVersions(c *gin.Context, tx *gorm.DB, ids []uint, versions map[uint]time.Time) bool {
	if err := checkAwardAuditVersions(tx, ids, versions); err != nil {
		tx.Rollback()
		if errors.Is(err, errAwardAuditConflict) {
			awardEditConflict(c)
		} else {
			utils.InternalServerError(c, "校验获奖版本失败", err)
		}
		return false
	}
	return true
}

// Shared history endpoint uses the same record scope for students, advisors and auditors.
func GetAwardChanges(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "无效的获奖ID")
		return
	}
	var a models.Award
	if err := database.DB.WithContext(c.Request.Context()).First(&a, uint(id)).Error; err != nil {
		utils.NotFound(c, "获奖记录不存在")
		return
	}
	uid, valid := c.Get("user_id")
	userID, ok := uid.(uint)
	if !valid || !ok || userID == 0 {
		utils.Unauthorized(c, "请先登录")
		return
	}
	role := c.GetString("role_code")
	if role == "student" {
		var count int64
		err = database.DB.WithContext(c.Request.Context()).Model(&models.Award{}).Where("awards.id = ?", a.ID).
			Where(`EXISTS (SELECT 1 FROM award_members am WHERE am.award_id = awards.id AND am.student_id = ? AND am.delete_time IS NULL) OR (NOT EXISTS (SELECT 1 FROM award_members am WHERE am.award_id = awards.id AND am.delete_time IS NULL) AND EXISTS (SELECT 1 FROM registers r WHERE r.id = awards.reg_id AND r.delete_time IS NULL AND (r.leader_id = ? OR EXISTS (SELECT 1 FROM reg_members rm JOIN users u ON u.username = rm.username WHERE rm.reg_id = r.id AND u.id = ?))))`, userID, userID, userID).Count(&count).Error
		if err != nil {
			utils.InternalServerError(c, "校验获奖权限失败", err)
			return
		}
		if count == 0 {
			utils.Forbidden(c, "只能查看自己的获奖修改记录")
			return
		}
	} else if role == "teacher" || role == "competition_manager" {
		user, ok := requireAwardAdvisorUser(c)
		if !ok {
			return
		}
		var count int64
		if err := advisorAwardScope(database.DB.WithContext(c.Request.Context()).Model(&models.Award{}), user.Username).Where("awards.id = ?", a.ID).Count(&count).Error; err != nil {
			utils.InternalServerError(c, "校验指导关系失败", err)
			return
		}
		if count == 0 {
			if role == "teacher" {
				utils.Forbidden(c, "只能查看自己指导的项目")
				return
			}
			if _, _, ok := requireAwardCompetition(c, a.CompID); !ok {
				return
			}
		}
	} else {
		scope, _, ok := requireAwardCompetition(c, a.CompID)
		if !ok {
			return
		}
		if scope.IsCollegeAdmin() {
			collegeName, err := collegeNameForAwardScope(database.DB.WithContext(c.Request.Context()), scope)
			if err != nil {
				utils.InternalServerError(c, "查询管理学院失败", err)
				return
			}
			allowed, err := awardBelongsToCollege(database.DB.WithContext(c.Request.Context()), a.ID, scope, collegeName)
			if err != nil {
				utils.InternalServerError(c, "查询获奖归属失败", err)
				return
			}
			if !allowed {
				utils.Forbidden(c, "无权查看其他学院的获奖修改记录")
				return
			}
		}
	}
	var changes []models.AwardChange
	if err := database.DB.WithContext(c.Request.Context()).Where("award_id = ?", a.ID).Order("id DESC").Find(&changes).Error; err != nil {
		utils.InternalServerError(c, "查询修改记录失败", err)
		return
	}
	utils.Success(c, gin.H{"list": changes})
}

func GetAdvisorAwards(c *gin.Context) {
	user, ok := requireAwardAdvisorUser(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 10
	}
	q := advisorAwardScope(database.DB.WithContext(c.Request.Context()).Model(&models.Award{}), user.Username)
	if status := c.Query("status"); status != "" {
		if status != "draft" && status != "rejected" && status != "approved" {
			utils.BadRequest(c, "无效的审核状态")
			return
		}
		q = q.Where("awards.status = ?", status)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		q = q.Where("(awards.project_name LIKE ? OR EXISTS (SELECT 1 FROM comp_directories cd WHERE cd.id = awards.comp_id AND cd.comp_name LIKE ?))", "%"+keyword+"%", "%"+keyword+"%")
	}
	var total int64
	var awards []models.Award
	if err := q.Count(&total).Error; err != nil {
		utils.InternalServerError(c, "统计指导获奖失败", err)
		return
	}
	if err := q.Preload("Competition").Order("awards.create_time DESC, awards.id DESC").Offset((page - 1) * size).Limit(size).Find(&awards).Error; err != nil {
		utils.InternalServerError(c, "查询指导获奖失败", err)
		return
	}
	list := make([]gin.H, 0, len(awards))
	for _, a := range awards {
		list = append(list, gin.H{"id": a.ID, "comp_name": a.Competition.CompName, "project_name": a.ProjectName, "award_level": a.AwardLevel, "award_time": a.AwardTime, "status": a.Status, "source": a.Source, "can_edit": advisorCanEditAward(a)})
	}
	utils.Success(c, gin.H{"list": list, "total": total})
}

func GetAdvisorAwardDetail(c *gin.Context) {
	user, ok := requireAwardAdvisorUser(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "无效的获奖ID")
		return
	}
	var a models.Award
	err = advisorAwardScope(database.DB.WithContext(c.Request.Context()), user.Username).Preload("Competition").Preload("Members").Preload("Advisors", func(db *gorm.DB) *gorm.DB { return db.Order("sort ASC") }).First(&a, uint(id)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utils.NotFound(c, "获奖记录不存在或不属于您的指导项目")
		return
	}
	if err != nil {
		utils.InternalServerError(c, "查询指导获奖详情失败", err)
		return
	}
	var changes []models.AwardChange
	if err := database.DB.WithContext(c.Request.Context()).Where("award_id = ?", a.ID).Order("id DESC").Find(&changes).Error; err != nil {
		utils.InternalServerError(c, "查询修改记录失败", err)
		return
	}
	utils.Success(c, gin.H{"award": a, "can_edit": advisorCanEditAward(a), "changes": changes})
}

type advisorAwardEdit struct {
	ProjectName   string    `json:"project_name" binding:"required"`
	AwardCategory string    `json:"award_category" binding:"required"`
	AwardName     string    `json:"award_name" binding:"required"`
	AwardDate     DateOnly  `json:"award_date"`
	ProofURL      string    `json:"proof_url" binding:"required"`
	Remark        string    `json:"remark"`
	UpdatedAt     time.Time `json:"update_time" binding:"required"`
}

func (r *advisorAwardEdit) validate() error {
	r.ProjectName = strings.TrimSpace(r.ProjectName)
	r.ProofURL = strings.TrimSpace(r.ProofURL)
	if r.ProjectName == "" || len([]rune(r.ProjectName)) > 255 {
		return errors.New("项目名称不能为空且不能超过255字")
	}
	if r.AwardCategory != "国家级" && r.AwardCategory != "省级" && r.AwardCategory != "校级" {
		return errors.New("请选择有效的获奖类别")
	}
	if r.AwardName != "一等奖" && r.AwardName != "二等奖" && r.AwardName != "三等奖" {
		return errors.New("请选择有效的获奖等级")
	}
	if r.AwardDate.IsZero() {
		return errors.New("请选择获奖时间")
	}
	if len([]rune(r.Remark)) > 2000 {
		return errors.New("备注不能超过2000字")
	}
	if !(strings.HasPrefix(r.ProofURL, "/api/file/download/award_proof/") || strings.HasPrefix(r.ProofURL, "/api/file/download/award_cert/") || strings.HasPrefix(r.ProofURL, "/api/file/download/temp/") || strings.HasPrefix(r.ProofURL, "/static/")) || strings.Contains(r.ProofURL, "..") || strings.ContainsAny(r.ProofURL, "\\\r\n") || len(r.ProofURL) > 255 {
		return errors.New("请上传有效的获奖证明文件")
	}
	return nil
}

func awardEditSnapshot(a models.Award) string {
	b, _ := json.Marshal(gin.H{"project_name": a.ProjectName, "award_category": a.AwardCategory, "award_name": a.AwardName, "award_time": a.AwardTime, "proof_url": a.ProofUrl, "remark": a.Remark, "status": a.Status})
	return string(b)
}

func UpdateAdvisorAward(c *gin.Context) {
	user, ok := requireAwardAdvisorUser(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		utils.BadRequest(c, "无效的获奖ID")
		return
	}
	var req advisorAwardEdit
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误，请刷新详情后重试")
		return
	}
	if err := req.validate(); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	var compID uint
	responseCode := 500
	responseMessage := "修改指导获奖失败"
	err = database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var a models.Award
		// Lock the award first, then check its current advisor relation and status.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, uint(id)).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				responseCode = 404
				responseMessage = "获奖记录不存在"
			}
			return err
		}
		var count int64
		if err := advisorAwardScope(tx.Model(&models.Award{}), user.Username).Where("awards.id = ?", a.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			responseCode = 403
			responseMessage = "只能修改自己指导的获奖项目"
			return errors.New(responseMessage)
		}
		if !advisorCanEditAward(a) {
			responseCode = 403
			responseMessage = "仅可修改待审核或被退回的学生填报"
			return errors.New(responseMessage)
		}
		if !a.UpdatedAt.Equal(req.UpdatedAt) {
			responseCode = 409
			responseMessage = "填报已被他人修改，请刷新后重试"
			return errors.New(responseMessage)
		}
		if req.ProofURL != a.ProofUrl {
			if err := validateAdvisorProof(tx, user, req.ProofURL); err != nil {
				responseCode = 400
				responseMessage = "请使用本人上传或自己指导项目已有的证明文件"
				return err
			}
		}
		before := awardEditSnapshot(a)
		key := database.AwardProjectKey(req.ProjectName)
		var duplicates int64
		if err := tx.Model(&models.Award{}).Where("comp_id = ? AND id <> ? AND (project_key = ? OR (project_key IS NULL AND BINARY project_name = ?))", a.CompID, a.ID, key, req.ProjectName).Count(&duplicates).Error; err != nil {
			return err
		}
		if duplicates > 0 {
			responseCode = 400
			responseMessage = "该赛事已存在同名获奖项目"
			return errors.New(responseMessage)
		}
		a.ProjectName = req.ProjectName
		a.AwardCategory = req.AwardCategory
		a.AwardName = req.AwardName
		a.AwardLevel = req.AwardCategory + req.AwardName
		a.AwardTime = &req.AwardDate.Time
		a.ProofUrl = req.ProofURL
		a.Remark = req.Remark
		a.Status = "draft"
		if err := tx.Model(&models.Award{}).Where("id = ?", a.ID).Updates(map[string]interface{}{"project_name": a.ProjectName, "project_key": key, "award_category": a.AwardCategory, "award_name": a.AwardName, "award_level": a.AwardLevel, "award_time": a.AwardTime, "proof_url": a.ProofUrl, "remark": a.Remark, "status": "draft", "reject_reason": "", "auditor_id": nil, "audit_time": nil}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.AwardChange{AwardID: a.ID, OperatorID: user.ID, OperatorName: user.Realname, Before: before, After: awardEditSnapshot(a)}).Error; err != nil {
			return err
		}
		if a.RegID != 0 {
			if err := tx.Model(&models.Register{}).Where("id = ? AND status = 5", a.RegID).Update("status", 3).Error; err != nil {
				return err
			}
		}
		compID = a.CompID
		return nil
	})
	if err != nil {
		switch responseCode {
		case 400:
			utils.BadRequest(c, responseMessage)
		case 403:
			utils.Forbidden(c, responseMessage)
		case 404:
			utils.NotFound(c, responseMessage)
		case 409:
			c.JSON(409, utils.Response{Code: 409, Message: responseMessage, Timestamp: time.Now().Unix()})
		default:
			utils.InternalServerError(c, responseMessage, err)
		}
		return
	}
	clearAwardCache(c.Request.Context(), compID)
	utils.SuccessWithMessage(c, "修改成功，等待管理员审核", nil)
}

func validateAdvisorProof(tx *gorm.DB, user models.User, proofURL string) error {
	record, err := findAwardProofRecord(tx, proofURL)
	if err != nil {
		return err
	}
	if record.UploaderID == user.ID {
		return nil
	}
	var count int64
	if err := advisorAwardScope(tx.Model(&models.Award{}), user.Username).Where("awards.proof_url = ?", proofURL).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errors.New("没有证明文件使用权限")
	}
	return nil
}

func validateStudentAwardProof(tx *gorm.DB, userID uint, proofURL string) error {
	record, err := findAwardProofRecord(tx, proofURL)
	if err != nil {
		return err
	}
	if record.UploaderID == userID {
		return nil
	}
	var count int64
	err = tx.Model(&models.Award{}).Where("awards.proof_url = ?", proofURL).
		Where(`EXISTS (SELECT 1 FROM award_members am WHERE am.award_id = awards.id AND am.student_id = ? AND am.delete_time IS NULL) OR EXISTS (SELECT 1 FROM registers r WHERE r.id = awards.reg_id AND r.leader_id = ? AND r.delete_time IS NULL)`, userID, userID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("没有证明文件使用权限")
	}
	return nil
}

func findAwardProofRecord(tx *gorm.DB, proofURL string) (models.FileRecord, error) {
	var record models.FileRecord
	if !strings.HasPrefix(proofURL, "/api/file/download/") {
		return record, errors.New("无效的证明地址")
	}
	parts := strings.SplitN(strings.TrimPrefix(proofURL, "/api/file/download/"), "/", 2)
	if len(parts) != 2 || (parts[0] != "award_proof" && parts[0] != "award_cert" && parts[0] != "temp") {
		return record, errors.New("无效的证明地址")
	}
	filename := strings.SplitN(parts[1], "_", 2)
	if len(filename) != 2 || len(filename[0]) != 32 {
		return record, errors.New("无效的证明文件")
	}
	if err := tx.Where("file_hash = ? AND biz_type = ?", filename[0], parts[0]).First(&record).Error; err != nil {
		return record, err
	}
	if record.StoragePath == "" || buildDownloadURL(record.BizType, record.FileHash, record.OriginalName) != proofURL {
		return record, errors.New("文件未上传完成")
	}
	return record, nil
}
