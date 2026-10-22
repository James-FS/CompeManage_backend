package controllers

import (
	"CompeManage_backend/models"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type awardImportSQLCounter struct {
	logger.Interface
	count int
}

func (c *awardImportSQLCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	c.count++
	c.Interface.Trace(ctx, begin, fc, err)
}

// Real MySQL acceptance uses an isolated database, exactly 100 and 1000 rows.
// Besides timing, enforce SQL bounds so a return to per-row I/O fails the test.
func TestAwardImportBatchMySQL(t *testing.T) {
	db := newAwardMySQLTestDB(t)
	for _, size := range []int{100, 1000} {
		t.Run(fmt.Sprintf("rows_%d", size), func(t *testing.T) {
			teamSize := 5
			if size == 1000 {
				teamSize = 1
			} // Exercise multiple batches of generated award IDs.
			teacher := models.User{Username: fmt.Sprintf("TB%d", size), Realname: "教师", College: "学院甲", IdentityType: "staff"}
			require.NoError(t, db.Create(&teacher).Error)
			comp := models.CompDirectory{CompCode: fmt.Sprintf("BATCH%d", size), CompName: "批量测试", CompLevel: "校级", ManagerID: teacher.ID}
			require.NoError(t, db.Create(&comp).Error)
			users := make([]models.User, size)
			for i := range users {
				users[i] = models.User{Username: fmt.Sprintf("B%d_%04d", size, i), Realname: fmt.Sprintf("学生%d", i), College: "学院甲", Major: "专业甲", IdentityType: "student"}
			}
			require.NoError(t, db.CreateInBatches(&users, awardImportBatchSize).Error)
			regs := make([]models.Register, size/teamSize)
			for i := range regs {
				regs[i] = models.Register{CompID: comp.ID, LeaderID: users[i*teamSize].ID, Status: 1, TeamName: fmt.Sprintf("项目%d", i), AdvisorInfo: "[]"}
			}
			require.NoError(t, db.CreateInBatches(&regs, awardImportBatchSize).Error)
			regMembers := make([]models.RegMember, size)
			rows := make([]AwardImportRow, size)
			for i, user := range users {
				regMembers[i] = models.RegMember{RegID: regs[i/teamSize].ID, StudentID: user.Username, Name: user.Realname}
				rows[i] = AwardImportRow{StudentNumber: user.Username, Name: user.Realname, College: user.College, Major: user.Major, ProjectName: fmt.Sprintf("项目%d", i/teamSize), AwardMonth: "2026.10", Category: "国际级", AwardName: "金奖", AdvisorUsernames: teacher.Username, RowNumber: i + 2}
			}
			require.NoError(t, db.CreateInBatches(&regMembers, awardImportBatchSize).Error)
			scope := &UserAccessScope{RoleCode: "school_admin", UserID: teacher.ID}
			counter := &awardImportSQLCounter{Interface: logger.Default.LogMode(logger.Silent)}
			measuredDB := db.Session(&gorm.Session{Logger: counter})
			var validated []AwardImportRow
			var validationTime, writeTime time.Duration
			var validationSQL, writeSQL int
			require.NoError(t, measuredDB.Transaction(func(tx *gorm.DB) error {
				var compLock models.CompDirectory
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&compLock, comp.ID).Error; err != nil {
					return err
				}
				counter.count = 0
				start := time.Now()
				result, bad, err := validateAwardRows(tx, comp.ID, rows, true)
				validationTime, validationSQL = time.Since(start), counter.count
				require.NoError(t, err)
				require.Zero(t, bad)
				validated = result
				for i, row := range result {
					require.Equal(t, users[i].ID, row.UserID)
					require.NotNil(t, row.RegID)
					require.Equal(t, regs[i/teamSize].ID, *row.RegID)
					require.Equal(t, "一等奖", row.AwardName)
					require.Equal(t, "2026-10", row.AwardMonth)
				}
				counter.count = 0
				start = time.Now()
				created, existing, teams, err := writeAwardImportRows(tx, scope, comp.ID, result)
				writeTime, writeSQL = time.Since(start), counter.count
				require.NoError(t, err)
				require.Equal(t, size, created)
				require.Zero(t, existing)
				require.Equal(t, len(regs), teams)
				return nil
			}))
			t.Logf("rows=%d validation=%s (%d SQL), write=%s (%d SQL)", size, validationTime, validationSQL, writeTime, writeSQL)
			require.LessOrEqual(t, validationSQL, 11)
			require.LessOrEqual(t, writeSQL, 13)
			var members []models.AwardMember
			require.NoError(t, db.Joins("JOIN awards ON awards.id = award_members.award_id").Where("awards.comp_id = ?", comp.ID).Order("award_members.student_id").Find(&members).Error)
			require.Len(t, members, size)
			var awards []models.Award
			require.NoError(t, db.Where("comp_id = ?", comp.ID).Find(&awards).Error)
			nameByID := map[uint]string{}
			for _, award := range awards {
				nameByID[award.ID] = award.ProjectName
			}
			for i, member := range members {
				require.Equal(t, users[i].ID, member.StudentID)
				require.Equal(t, rows[i].ProjectName, nameByID[member.AwardID])
				require.Equal(t, scope.UserID, *member.SubmittedBy)
			}
			var advisorCount, completed int64
			require.NoError(t, db.Model(&models.AwardAdvisor{}).Joins("JOIN awards ON awards.id = award_advisors.award_id").Where("awards.comp_id = ?", comp.ID).Count(&advisorCount).Error)
			require.EqualValues(t, len(regs), advisorCount)
			require.NoError(t, db.Model(&models.Register{}).Where("comp_id = ? AND status = 4", comp.ID).Count(&completed).Error)
			require.EqualValues(t, len(regs), completed)
			// Idempotence must preserve the first submitter and write no additional members.
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				result, bad, err := validateAwardRows(tx, comp.ID, rows, true)
				require.NoError(t, err)
				require.Zero(t, bad)
				for _, row := range result {
					require.True(t, row.Existing)
				}
				created, existing, teams, err := writeAwardImportRows(tx, &UserAccessScope{RoleCode: "school_admin", UserID: 9999}, comp.ID, result)
				require.NoError(t, err)
				require.Zero(t, created)
				require.Equal(t, size, existing)
				require.Equal(t, len(regs), teams)
				return nil
			}))
			var first models.AwardMember
			require.NoError(t, db.First(&first, members[0].ID).Error)
			require.Equal(t, scope.UserID, *first.SubmittedBy)
			// A duplicate in a later insert batch must roll back earlier batches and newly created awards.
			rollbackComp := models.CompDirectory{CompCode: fmt.Sprintf("ROLLBACK%d", size), CompName: "回滚测试", ManagerID: teacher.ID}
			require.NoError(t, db.Create(&rollbackComp).Error)
			toWrite := append([]AwardImportRow(nil), validated...)
			toWrite = append(toWrite, toWrite[0])
			err := db.Transaction(func(tx *gorm.DB) error {
				_, _, _, err := writeAwardImportRows(tx, scope, rollbackComp.ID, toWrite)
				return err
			})
			require.Error(t, err)
			var leftovers int64
			require.NoError(t, db.Model(&models.Award{}).Where("comp_id = ?", rollbackComp.ID).Count(&leftovers).Error)
			require.Zero(t, leftovers)
			// A student already in this event cannot be assigned to a different project.
			conflict := rows[0]
			conflict.ProjectName = "其他项目"
			result, bad, err := validateAwardRows(db, comp.ID, []AwardImportRow{conflict}, true)
			require.NoError(t, err)
			require.Equal(t, 1, bad)
			require.Contains(t, result[0].Error, "其他获奖记录")
			// A deleted membership stops blocking subsequent imports.
			require.NoError(t, db.Delete(&first).Error)
			result, bad, err = validateAwardRows(db, comp.ID, []AwardImportRow{conflict}, true)
			require.NoError(t, err)
			require.Zero(t, bad)
			require.False(t, result[0].Existing)
			if size == 100 {
				t.Run("collation, registrations and legacy conflicts", func(t *testing.T) {
					// The database uses a case-insensitive username collation.
					lookupUsers, err := loadAwardImportUsers(db, []string{"b100_0001", "B100_0001"}, false)
					require.NoError(t, err)
					require.Equal(t, users[1].ID, lookupUsers["b100_0001"].ID)
					require.Equal(t, users[1].ID, lookupUsers["B100_0001"].ID)
					lookupTeachers, err := loadAwardImportUsers(db, []string{"tb100"}, true)
					require.NoError(t, err)
					require.Equal(t, teacher.ID, lookupTeachers["tb100"].ID)
					secondReg := models.Register{CompID: comp.ID, LeaderID: teacher.ID, Status: 1, AdvisorInfo: "[]"}
					require.NoError(t, db.Create(&secondReg).Error)
					require.NoError(t, db.Create(&models.RegMember{RegID: secondReg.ID, StudentID: users[1].Username}).Error)
					result, bad, err := validateAwardRows(db, comp.ID, rows[1:2], true)
					require.NoError(t, err)
					require.Zero(t, bad)
					require.Nil(t, result[0].RegID, "multiple approved registrations must remain ambiguous")
					freshComp := models.CompDirectory{CompCode: "LEGACY_BATCH", CompName: "历史测试", ManagerID: teacher.ID}
					require.NoError(t, db.Create(&freshComp).Error)
					result, bad, err = validateAwardRows(db, freshComp.ID, rows[1:2], true)
					require.NoError(t, err)
					require.Equal(t, 1, bad)
					require.Contains(t, result[0].Error, "未找到")
					month, _ := parseAwardImportDate("2026-10")
					legacy := models.Award{CompID: freshComp.ID, ProjectName: "TeamCase", AwardCategory: "国家级", AwardName: "一等奖", AwardTime: &month, Status: "approved", AdvisorInfo: "[]"}
					require.NoError(t, db.Create(&legacy).Error)
					legacyRow := rows[1]
					legacyRow.ProjectName = "teamcase"
					result, bad, err = validateAwardRows(db, freshComp.ID, []AwardImportRow{legacyRow}, false)
					require.NoError(t, err)
					require.Zero(t, bad)
					require.Zero(t, result[0].AwardID, "legacy project matching must remain binary")
					legacyRow.ProjectName = "TeamCase"
					result, bad, err = validateAwardRows(db, freshComp.ID, []AwardImportRow{legacyRow}, false)
					require.NoError(t, err)
					require.Zero(t, bad)
					require.Equal(t, legacy.ID, result[0].AwardID)
					require.True(t, result[0].NeedsProjectKey)
					require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
						_, _, _, err := writeAwardImportRows(tx, scope, freshComp.ID, result)
						return err
					}))
					require.NoError(t, db.First(&legacy, legacy.ID).Error)
					require.NotNil(t, legacy.ProjectKey)
					// Rejected and pending awards cannot be overwritten by the import.
					require.NoError(t, db.Model(&legacy).Update("status", "rejected").Error)
					result, bad, err = validateAwardRows(db, freshComp.ID, []AwardImportRow{legacyRow}, false)
					require.NoError(t, err)
					require.Equal(t, 1, bad)
					require.Contains(t, result[0].Error, "不能通过导入覆盖")
				})
			}
		})
	}
}
