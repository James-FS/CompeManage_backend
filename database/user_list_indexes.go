package database

import (
	"CompeManage_backend/models"
	"context"
	"database/sql/driver"
	"errors"
	"log"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func prepareUserListIndexes(db *gorm.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := ensureUserListIndexes(db.WithContext(ctx)); err != nil {
		log.Printf("创建用户管理查询索引失败，可在下次启动时重试: %v", err)
	}
}

func ensureUserListIndexes(db *gorm.DB) error {
	indexes := []struct{ name, columns string }{
		{name: "idx_users_admin_order", columns: "delete_time, create_time, id"},
		{name: "idx_users_admin_search", columns: "delete_time, college, identity_type, username, realname"},
	}
	clauses := make([]string, 0, len(indexes))
	for _, index := range indexes {
		if !db.Migrator().HasIndex(&models.User{}, index.name) {
			clauses = append(clauses, "ADD INDEX "+index.name+" ("+index.columns+")")
		}
	}
	if len(clauses) == 0 {
		return nil
	}

	err := createUserListIndexes(db, "ALTER TABLE users "+strings.Join(clauses, ", ")+", ALGORITHM=INPLACE, LOCK=NONE")
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1061 {
		// Another instance may have created the same indexes during startup.
		for _, index := range indexes {
			if !db.Migrator().HasIndex(&models.User{}, index.name) {
				return err
			}
		}
		return nil
	}
	return err
}

func createUserListIndexes(db *gorm.DB, statement string) error {
	ctx := db.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	var previousTimeout int
	if err := conn.QueryRowContext(ctx, "SELECT @@SESSION.lock_wait_timeout").Scan(&previousTimeout); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "SET SESSION lock_wait_timeout = 5"); err != nil {
		return err
	}
	defer func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(restoreCtx, "SET SESSION lock_wait_timeout = ?", previousTimeout); err != nil {
			// A changed session setting must not leak into another request.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	_, err = conn.ExecContext(ctx, statement)
	return err
}
