package database

import (
	"CompeManage_backend/models"
	"context"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestEnsureUserListIndexesMySQL(t *testing.T) {
	db := newSeedMySQLTestDB(t)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	var original, restored int
	require.NoError(t, db.Raw("SELECT @@SESSION.lock_wait_timeout").Scan(&original).Error)
	require.NoError(t, ensureUserListIndexes(db))
	require.NoError(t, ensureUserListIndexes(db))
	require.NoError(t, db.Raw("SELECT @@SESSION.lock_wait_timeout").Scan(&restored).Error)
	require.Equal(t, original, restored)
	for _, name := range []string{"idx_users_admin_order", "idx_users_admin_search"} {
		require.True(t, db.Migrator().HasIndex(&models.User{}, name))
	}
}

func TestUserListIndexesMySQL_BoundsMetadataLockWait(t *testing.T) {
	db := newSeedMySQLTestDB(t)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(2)
	blocker := db.Begin()
	require.NoError(t, blocker.Error)
	defer blocker.Rollback()
	var readRows int64
	require.NoError(t, blocker.Model(&models.User{}).Count(&readRows).Error)

	ctx, cancel := context.WithTimeout(t.Context(), 9*time.Second)
	defer cancel()
	var mysqlErr *mysqlDriver.MySQLError
	require.ErrorAs(t, ensureUserListIndexes(db.WithContext(ctx)), &mysqlErr)
	require.Equal(t, uint16(1205), mysqlErr.Number)
	require.NoError(t, blocker.Rollback().Error)
	require.NoError(t, ensureUserListIndexes(db.WithContext(ctx)))
}
