package common

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/db"
)

var (
	dbInstance *db.DB
	dbOnce     sync.Once
	dbErr      error
	dbMu       sync.Mutex
)

// OpenDB returns a shared database singleton.
func OpenDB() (*db.DB, error) {
	dbMu.Lock()
	defer dbMu.Unlock()
	dbOnce.Do(func() {
		cfg, err := config.Load()
		if err != nil {
			dbErr = fmt.Errorf("load config: %w", err)
			return
		}
		dbInstance, dbErr = db.Open(cfg.DBPath)
		if dbErr != nil {
			return
		}
		dbInstance.MarkShared()
	})
	return dbInstance, dbErr
}

// SqlDB returns the raw *sql.DB from the shared singleton.
func SqlDB() (*sql.DB, error) {
	database, err := OpenDB()
	if err != nil {
		return nil, err
	}
	return database.SqlDB(), nil
}

// ResetDB closes the shared singleton and resets state.
func ResetDB() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if dbInstance != nil {
		if err := dbInstance.ForceClose(); err != nil {
			return err
		}
	}
	dbInstance = nil
	dbErr = nil
	dbOnce = sync.Once{}
	return nil
}

// SetTestDB injects a pre-configured DB instance for testing.
func SetTestDB(database *db.DB) {
	dbMu.Lock()
	defer dbMu.Unlock()
	dbOnce.Do(func() {})
	dbInstance = database
	dbErr = nil
}
