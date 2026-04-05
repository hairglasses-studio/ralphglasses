package tools

import (
	"database/sql"
	"log"
	"sync"
	"time"
)

// UsageTracker records tool invocations.
type UsageTracker struct {
	mu   sync.Mutex
	db   *sql.DB
	open func() (*sql.DB, error)
}

var (
	globalTracker     *UsageTracker
	globalTrackerOnce sync.Once
)

// SetTrackerDB sets the database opener for the global usage tracker.
func SetTrackerDB(open func() (*sql.DB, error)) {
	globalTrackerOnce.Do(func() {
		globalTracker = &UsageTracker{open: open}
	})
}

// RecordUsage records a tool invocation asynchronously.
func RecordUsage(toolName string) {
	if globalTracker == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[runmylife] usage tracker panic: %v", r)
			}
		}()
		globalTracker.record(toolName)
	}()
}

func (t *UsageTracker) ensureDB() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.db != nil {
		return nil
	}
	db, err := t.open()
	if err != nil {
		return err
	}
	t.db = db
	return nil
}

func (t *UsageTracker) record(toolName string) {
	if err := t.ensureDB(); err != nil {
		return
	}
	_, err := t.db.Exec(`
		INSERT INTO tool_usage (tool_name, invocation_count, last_used_at)
		VALUES (?, 1, ?)
		ON CONFLICT(tool_name) DO UPDATE SET
			invocation_count = invocation_count + 1,
			last_used_at = ?`,
		toolName, time.Now(), time.Now(),
	)
	if err != nil {
		log.Printf("[runmylife] record tool usage: %v", err)
	}
}

// ToolUsageEntry represents a row from the tool_usage table.
type ToolUsageEntry struct {
	ToolName        string
	InvocationCount int
	LastUsedAt      time.Time
}

// TopTools returns the most frequently used tools.
func TopTools(db *sql.DB, limit int) ([]ToolUsageEntry, error) {
	rows, err := db.Query(`
		SELECT tool_name, invocation_count, last_used_at
		FROM tool_usage ORDER BY invocation_count DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ToolUsageEntry
	for rows.Next() {
		var e ToolUsageEntry
		if err := rows.Scan(&e.ToolName, &e.InvocationCount, &e.LastUsedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
