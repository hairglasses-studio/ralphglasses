package intelligence

import (
	"context"
	"database/sql"
	"time"
)

// PersistSuggestions saves suggestions to the database with daily deduplication by title.
func PersistSuggestions(ctx context.Context, db *sql.DB, suggestions []Suggestion) {
	today := time.Now().Format("2006-01-02")

	for _, s := range suggestions {
		// Skip if same title already recorded today
		var exists int
		_ = db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM intelligence_suggestions
			 WHERE title = ? AND date(created_at) = ?`,
			s.Title, today,
		).Scan(&exists)
		if exists > 0 {
			continue
		}

		_, _ = db.ExecContext(ctx,
			`INSERT INTO intelligence_suggestions (category, priority, title, description, action_hint, source)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			s.Category, s.Priority, s.Title, s.Description, s.ActionHint, "engine",
		)
	}
}

// QuerySuggestions returns the most recent suggestions up to the given limit.
func QuerySuggestions(ctx context.Context, db *sql.DB, limit int) []Suggestion {
	rows, err := db.QueryContext(ctx,
		`SELECT category, priority, title, description, action_hint
		 FROM intelligence_suggestions
		 ORDER BY created_at DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var suggestions []Suggestion
	for rows.Next() {
		var s Suggestion
		if rows.Scan(&s.Category, &s.Priority, &s.Title, &s.Description, &s.ActionHint) == nil {
			suggestions = append(suggestions, s)
		}
	}
	return suggestions
}

// PruneSuggestions deletes suggestions older than the given time.
func PruneSuggestions(ctx context.Context, db *sql.DB, olderThan time.Time) int64 {
	result, err := db.ExecContext(ctx,
		`DELETE FROM intelligence_suggestions WHERE created_at < ?`,
		olderThan.Format(time.RFC3339))
	if err != nil {
		return 0
	}
	n, _ := result.RowsAffected()
	return n
}
