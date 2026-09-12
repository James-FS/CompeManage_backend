package database

import (
	"CompeManage_backend/models"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

const userRoleUniqueIndex = "uk_user_roles_user_id"

var safeSQLIdentifier = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type UserRoleAnomaly struct {
	UserID         uint   `json:"user_id"`
	Username       string `json:"username"`
	Roles          string `json:"roles"`
	Count          int    `json:"count"`
	SuggestedRole  string `json:"suggested_role"`
	DecisionReason string `json:"decision_reason"`
}

type migrationRoleDecision struct {
	RoleID   uint
	RoleCode string
	Reason   string
}

type UserIdentityAnomaly struct {
	UserID       uint   `json:"user_id"`
	Username     string `json:"username"`
	IdentityType string `json:"identity_type"`
	College      string `json:"college"`
	Grade        string `json:"grade"`
}

type UserRoleMigrationReport struct {
	TotalUsers                   int64                 `json:"total_users"`
	MultiRoleUsers               []UserRoleAnomaly     `json:"multi_role_users"`
	NoRoleUsers                  []UserIdentityAnomaly `json:"no_role_users"`
	MissingIdentityUsers         []UserIdentityAnomaly `json:"missing_identity_users"`
	CollegeAdminsWithoutScope    []UserIdentityAnomaly `json:"college_admins_without_scope"`
	UniqueConstraintAlreadyExist bool                  `json:"unique_constraint_already_exists"`
	BackupTable                  string                `json:"backup_table,omitempty"`
}

func BuildUserRoleMigrationReport() (*UserRoleMigrationReport, error) {
	report := &UserRoleMigrationReport{}
	if err := DB.Model(&models.User{}).Count(&report.TotalUsers).Error; err != nil {
		return nil, err
	}

	if err := DB.Raw(`
		SELECT users.id AS user_id, users.username,
		       GROUP_CONCAT(roles.role_code ORDER BY roles.role_code SEPARATOR ',') AS roles,
		       COUNT(*) AS count
		FROM users
		JOIN user_roles ON user_roles.user_id = users.id
		JOIN roles ON roles.id = user_roles.role_id AND roles.delete_time IS NULL
		WHERE users.delete_time IS NULL
		GROUP BY users.id, users.username
		HAVING COUNT(*) > 1
		ORDER BY users.id`).Scan(&report.MultiRoleUsers).Error; err != nil {
		return nil, err
	}
	if len(report.MultiRoleUsers) > 0 {
		decisions, err := buildMigrationRoleDecisions(report.MultiRoleUsers)
		if err != nil {
			return nil, err
		}
		for i := range report.MultiRoleUsers {
			decision := decisions[report.MultiRoleUsers[i].UserID]
			report.MultiRoleUsers[i].SuggestedRole = decision.RoleCode
			report.MultiRoleUsers[i].DecisionReason = decision.Reason
		}
	}

	if err := DB.Raw(`
		SELECT users.id AS user_id, users.username, users.identity_type, users.college, users.grade
		FROM users
		WHERE users.delete_time IS NULL
		  AND NOT EXISTS (SELECT 1 FROM user_roles WHERE user_roles.user_id = users.id)
		ORDER BY users.id`).Scan(&report.NoRoleUsers).Error; err != nil {
		return nil, err
	}

	if err := DB.Raw(`
		SELECT id AS user_id, username, identity_type, college, grade
		FROM users
		WHERE delete_time IS NULL AND (identity_type IS NULL OR TRIM(identity_type) = '')
		ORDER BY id`).Scan(&report.MissingIdentityUsers).Error; err != nil {
		return nil, err
	}

	if err := DB.Raw(`
		SELECT users.id AS user_id, users.username, users.identity_type, users.college, users.grade
		FROM users
		JOIN user_roles ON user_roles.user_id = users.id
		JOIN roles ON roles.id = user_roles.role_id AND roles.role_code = 'college_admin'
		WHERE users.delete_time IS NULL AND users.managed_college_id IS NULL
		ORDER BY users.id`).Scan(&report.CollegeAdminsWithoutScope).Error; err != nil {
		return nil, err
	}

	exists, err := userRoleUniqueIndexExists()
	if err != nil {
		return nil, err
	}
	report.UniqueConstraintAlreadyExist = exists
	return report, nil
}

func ApplyUserRoleMigration(backupTable string) (*UserRoleMigrationReport, error) {
	if backupTable == "" {
		backupTable = "user_roles_backup_" + time.Now().Format("20060102150405")
	}
	if !safeSQLIdentifier.MatchString(backupTable) {
		return nil, errors.New("备份表名只能包含字母、数字和下划线")
	}
	usersBackupTable := backupTable + "_users"
	hadUniqueIndex, err := userRoleUniqueIndexExists()
	if err != nil {
		return nil, err
	}

	if err := createUserRoleMigrationBackup(backupTable, usersBackupTable, hadUniqueIndex); err != nil {
		return nil, err
	}
	if err := backfillIdentityTypes(); err != nil {
		return nil, err
	}
	if err := normalizeMultipleRoles(); err != nil {
		return nil, err
	}
	if err := assignMissingRoles(); err != nil {
		return nil, err
	}
	if err := backfillCollegeAdminScopes(); err != nil {
		return nil, err
	}
	if err := ensureUserRoleUniqueIndex(); err != nil {
		return nil, err
	}

	report, err := BuildUserRoleMigrationReport()
	if report != nil {
		report.BackupTable = backupTable
	}
	if err != nil {
		return report, err
	}
	if len(report.MultiRoleUsers) > 0 || len(report.NoRoleUsers) > 0 ||
		len(report.MissingIdentityUsers) > 0 || len(report.CollegeAdminsWithoutScope) > 0 {
		return report, errors.New("迁移后一致性检查失败")
	}
	return report, nil
}

func VerifyUserRoleMigration() (*UserRoleMigrationReport, error) {
	report, err := BuildUserRoleMigrationReport()
	if err != nil {
		return nil, err
	}
	if len(report.MultiRoleUsers) > 0 || len(report.NoRoleUsers) > 0 || len(report.MissingIdentityUsers) > 0 {
		return report, errors.New("用户角色或人员身份仍存在异常")
	}
	if !report.UniqueConstraintAlreadyExist {
		return report, errors.New("user_roles.user_id 唯一约束尚未创建")
	}
	if len(report.CollegeAdminsWithoutScope) > 0 {
		return report, errors.New("仍有院管理员未绑定管理学院")
	}
	return report, nil
}

func RollbackUserRoleMigration(backupTable string) error {
	if backupTable == "" || !safeSQLIdentifier.MatchString(backupTable) {
		return errors.New("必须提供有效备份表名")
	}
	usersBackupTable := backupTable + "_users"
	metaTable := backupTable + "_meta"
	var hadUniqueIndex bool
	if err := DB.Raw(fmt.Sprintf("SELECT had_unique_index FROM %s LIMIT 1", metaTable)).Scan(&hadUniqueIndex).Error; err != nil {
		return fmt.Errorf("读取迁移备份元数据失败: %w", err)
	}

	currentHasUniqueIndex, err := userRoleUniqueIndexExists()
	if err != nil {
		return err
	}
	// MySQL 的 ALTER TABLE 会隐式提交，因此索引 DDL 必须放在数据恢复事务之外。
	if currentHasUniqueIndex && !hadUniqueIndex {
		if err := DB.Exec(fmt.Sprintf("ALTER TABLE user_roles DROP INDEX %s", userRoleUniqueIndex)).Error; err != nil {
			return err
		}
		currentHasUniqueIndex = false
	}

	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM user_roles").Error; err != nil {
			return err
		}
		if err := tx.Exec(fmt.Sprintf("INSERT INTO user_roles SELECT * FROM %s", backupTable)).Error; err != nil {
			return err
		}
		if err := tx.Exec(fmt.Sprintf(`
			UPDATE users
			JOIN %s backup_users ON backup_users.id = users.id
			SET users.identity_type = backup_users.identity_type,
			    users.managed_college_id = backup_users.managed_college_id`, usersBackupTable)).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	if hadUniqueIndex && !currentHasUniqueIndex {
		if err := DB.Exec(fmt.Sprintf("ALTER TABLE user_roles ADD UNIQUE INDEX %s (user_id)", userRoleUniqueIndex)).Error; err != nil {
			return fmt.Errorf("恢复迁移前唯一索引失败: %w", err)
		}
	}
	return nil
}

func createUserRoleMigrationBackup(roleTable, usersTable string, hadUniqueIndex bool) error {
	metaTable := roleTable + "_meta"
	if err := DB.Exec(fmt.Sprintf("CREATE TABLE %s LIKE user_roles", roleTable)).Error; err != nil {
		return fmt.Errorf("创建角色关联备份表失败: %w", err)
	}
	if err := DB.Exec(fmt.Sprintf("INSERT INTO %s SELECT * FROM user_roles", roleTable)).Error; err != nil {
		return fmt.Errorf("备份角色关联失败: %w", err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		CREATE TABLE %s AS
		SELECT id, identity_type, managed_college_id FROM users`, usersTable)).Error; err != nil {
		return fmt.Errorf("备份用户授权字段失败: %w", err)
	}
	if err := DB.Exec(fmt.Sprintf(`CREATE TABLE %s (
		id TINYINT PRIMARY KEY,
		had_unique_index BOOLEAN NOT NULL
	)`, metaTable)).Error; err != nil {
		return fmt.Errorf("创建迁移元数据备份表失败: %w", err)
	}
	if err := DB.Exec(fmt.Sprintf("INSERT INTO %s (id, had_unique_index) VALUES (1, ?)", metaTable), hadUniqueIndex).Error; err != nil {
		return fmt.Errorf("备份迁移元数据失败: %w", err)
	}
	return nil
}

func backfillIdentityTypes() error {
	var users []models.User
	if err := DB.Where("identity_type IS NULL OR TRIM(identity_type) = ''").Find(&users).Error; err != nil {
		return err
	}
	for _, user := range users {
		identityType := inferMigrationIdentityType(user)
		if err := DB.Model(&models.User{}).Where("id = ?", user.ID).
			Update("identity_type", identityType).Error; err != nil {
			return err
		}
	}
	return nil
}

func inferMigrationIdentityType(user models.User) string {
	username := strings.ToUpper(strings.TrimSpace(user.Username))
	grade := strings.TrimSpace(user.Grade)
	if strings.Contains(grade, "教职") || strings.HasPrefix(username, "T") {
		return "staff"
	}
	if strings.Contains(grade, "级") || strings.HasPrefix(username, "S") {
		return "student"
	}
	return "external"
}

func normalizeMultipleRoles() error {
	var anomalies []UserRoleAnomaly
	if err := DB.Raw(`
		SELECT users.id AS user_id, users.username,
		       GROUP_CONCAT(roles.role_code ORDER BY roles.role_code SEPARATOR ',') AS roles,
		       COUNT(*) AS count
		FROM users
		JOIN user_roles ON user_roles.user_id = users.id
		JOIN roles ON roles.id = user_roles.role_id AND roles.delete_time IS NULL
		WHERE users.delete_time IS NULL
		GROUP BY users.id, users.username
		HAVING COUNT(*) > 1`).Scan(&anomalies).Error; err != nil {
		return err
	}
	if len(anomalies) == 0 {
		return nil
	}
	decisions, err := buildMigrationRoleDecisions(anomalies)
	if err != nil {
		return err
	}

	for _, anomaly := range anomalies {
		decision, exists := decisions[anomaly.UserID]
		if !exists || decision.RoleID == 0 {
			return fmt.Errorf("无法确定用户 %d 的迁移后角色", anomaly.UserID)
		}
		if err := DB.Transaction(func(tx *gorm.DB) error {
			return SetUserRole(tx, anomaly.UserID, decision.RoleID)
		}); err != nil {
			return err
		}
	}
	return nil
}

func buildMigrationRoleDecisions(anomalies []UserRoleAnomaly) (map[uint]migrationRoleDecision, error) {
	decisions := make(map[uint]migrationRoleDecision, len(anomalies))
	if len(anomalies) == 0 {
		return decisions, nil
	}

	userIDs := make([]uint, 0, len(anomalies))
	for _, anomaly := range anomalies {
		userIDs = append(userIDs, anomaly.UserID)
	}

	var users []models.User
	if err := DB.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	usersByID := make(map[uint]models.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}

	var roles []models.Role
	if err := DB.Find(&roles).Error; err != nil {
		return nil, err
	}
	rolesByID := make(map[uint]models.Role, len(roles))
	for _, role := range roles {
		rolesByID[role.ID] = role
	}

	var links []models.UserRole
	if err := DB.Where("user_id IN ?", userIDs).Order("user_id ASC, role_id ASC").Find(&links).Error; err != nil {
		return nil, err
	}
	linksByUserID := make(map[uint][]models.UserRole, len(anomalies))
	for _, link := range links {
		linksByUserID[link.UserID] = append(linksByUserID[link.UserID], link)
	}

	managerUserIDs := make(map[uint]bool)
	var compManagerIDs []uint
	if err := DB.Model(&models.CompDirectory{}).Distinct().Where("manager_id > 0").Pluck("manager_id", &compManagerIDs).Error; err != nil {
		return nil, err
	}
	for _, userID := range compManagerIDs {
		managerUserIDs[userID] = true
	}
	var declareManagerIDs []uint
	if err := DB.Model(&models.CompDeclaration{}).Distinct().Where("manager_id > 0").Pluck("manager_id", &declareManagerIDs).Error; err != nil {
		return nil, err
	}
	for _, userID := range declareManagerIDs {
		managerUserIDs[userID] = true
	}

	for _, anomaly := range anomalies {
		user := usersByID[anomaly.UserID]
		if user.IdentityType == "" {
			user.IdentityType = inferMigrationIdentityType(user)
		}
		decision := chooseMigrationRole(
			linksByUserID[anomaly.UserID],
			rolesByID,
			user,
			managerUserIDs[anomaly.UserID],
		)
		if decision.RoleID == 0 {
			return nil, fmt.Errorf("用户 %d 没有可保留的有效角色", anomaly.UserID)
		}
		decisions[anomaly.UserID] = decision
	}
	return decisions, nil
}

func chooseMigrationRole(links []models.UserRole, rolesByID map[uint]models.Role, user models.User, managesCompetition bool) migrationRoleDecision {
	bestScore := 1000
	best := migrationRoleDecision{}
	hasCompetitionManager := false
	for _, link := range links {
		if rolesByID[link.RoleID].RoleCode == "competition_manager" {
			hasCompetitionManager = true
			break
		}
	}

	for _, link := range links {
		role := rolesByID[link.RoleID]
		score := migrationRoleScore(role.RoleCode, user.IdentityType, managesCompetition)
		if score < bestScore {
			bestScore = score
			best = migrationRoleDecision{RoleID: link.RoleID, RoleCode: role.RoleCode}
		}
	}

	switch best.RoleCode {
	case "school_admin":
		best.Reason = "保留校级管理员角色"
	case "college_admin":
		best.Reason = "保留院级管理员角色"
	case "competition_manager":
		if managesCompetition {
			best.Reason = "用户仍是赛事或申报负责人，保留赛事负责人角色"
		} else {
			best.Reason = "没有更匹配的自然身份角色，保留赛事负责人角色"
		}
	case "teacher":
		if hasCompetitionManager && !managesCompetition {
			best.Reason = "未发现负责赛事或申报，按教职工身份保留教师角色"
		} else {
			best.Reason = "按教职工身份保留教师角色"
		}
	case "student":
		best.Reason = "按学生身份保留学生角色"
	case "expert":
		best.Reason = "保留专家角色"
	case "guest":
		best.Reason = "保留访客角色"
	default:
		best.Reason = "按确定性角色顺序保留角色"
	}
	return best
}

func migrationRoleScore(roleCode, identityType string, managesCompetition bool) int {
	switch roleCode {
	case "school_admin":
		return 10
	case "college_admin":
		return 20
	case "competition_manager":
		if managesCompetition {
			return 30
		}
		return 80
	case "expert":
		return 40
	case "teacher":
		if identityType == "staff" {
			return 50
		}
		return 70
	case "student":
		if identityType == "student" || identityType == "postgraduate" {
			return 50
		}
		return 70
	case "guest":
		return 90
	default:
		return 100
	}
}

func assignMissingRoles() error {
	var users []models.User
	if err := DB.Raw(`
		SELECT users.* FROM users
		WHERE users.delete_time IS NULL
		  AND NOT EXISTS (SELECT 1 FROM user_roles WHERE user_roles.user_id = users.id)`).Scan(&users).Error; err != nil {
		return err
	}
	if len(users) == 0 {
		return nil
	}

	var roles []models.Role
	// P1-3：列表必须包含 teacher，否则 roleIDs["teacher"] 为 0 会报「默认角色不存在」。
	if err := DB.Where("role_code IN ?",
		[]string{"teacher", "competition_manager", "student", "guest"}).Find(&roles).Error; err != nil {
		return err
	}
	roleIDs := make(map[string]uint, len(roles))
	for _, role := range roles {
		roleIDs[role.RoleCode] = role.ID
	}
	for _, user := range users {
		roleCode := "guest"
		switch user.IdentityType {
		case "staff":
			// P1-3：教职工默认角色为 teacher，与同步/CAS 保持一致。
			roleCode = "teacher"
		case "student", "postgraduate":
			roleCode = "student"
		}
		roleID := roleIDs[roleCode]
		if roleID == 0 {
			return fmt.Errorf("默认角色不存在: %s", roleCode)
		}
		if err := DB.Transaction(func(tx *gorm.DB) error {
			return SetUserRole(tx, user.ID, roleID)
		}); err != nil {
			return err
		}
	}
	return nil
}

func backfillCollegeAdminScopes() error {
	return DB.Exec(`
		UPDATE users
		JOIN user_roles ON user_roles.user_id = users.id
		JOIN roles ON roles.id = user_roles.role_id AND roles.role_code = 'college_admin'
		JOIN colleges ON TRIM(colleges.name) = TRIM(users.college)
		SET users.managed_college_id = colleges.id
		WHERE users.managed_college_id IS NULL`).Error
}

func ensureUserRoleUniqueIndex() error {
	exists, err := userRoleUniqueIndexExists()
	if err != nil || exists {
		return err
	}
	return DB.Exec(fmt.Sprintf("ALTER TABLE user_roles ADD UNIQUE INDEX %s (user_id)", userRoleUniqueIndex)).Error
}

func userRoleUniqueIndexExists() (bool, error) {
	return userRoleUniqueIndexExistsWithDB(DB)
}

func userRoleUniqueIndexExistsWithDB(db *gorm.DB) (bool, error) {
	var count int64
	err := db.Raw(`
		SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = 'user_roles'
		  AND index_name = ?`, userRoleUniqueIndex).Scan(&count).Error
	return count > 0, err
}
