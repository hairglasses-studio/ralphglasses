package db

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

var memDBCounter atomic.Int64

// OpenMemory creates an in-memory SQLite database for testing.
// Uses shared cache so multiple connections see the same data (matching
// production's MaxOpenConns=5 behavior).
func OpenMemory() (*DB, error) {
	name := fmt.Sprintf("testdb%d", memDBCounter.Add(1))
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys%%3d1&_pragma=busy_timeout%%3d5000", name)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	db := &DB{DB: sqlDB}
	if err := db.Migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}
