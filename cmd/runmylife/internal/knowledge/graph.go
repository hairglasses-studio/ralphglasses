// Package knowledge provides an entity knowledge graph built from cross-module data.
package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// EntityLink represents a relationship between two entities.
type EntityLink struct {
	ID           int64
	SourceType   string  // person, event, task, place, topic, file
	SourceID     string
	SourceLabel  string
	TargetType   string
	TargetID     string
	TargetLabel  string
	Relationship string  // mentions, attends, assigned, located_at, about, related
	Confidence   float64 // 0.0-1.0
	Metadata     string  // JSON
	CreatedAt    string
}

// EntitySummary holds aggregate info about an entity.
type EntitySummary struct {
	Type       string
	ID         string
	Label      string
	LinkCount  int
	FirstSeen  string
	LastSeen   string
}

// GraphStats holds knowledge graph statistics.
type GraphStats struct {
	TotalLinks   int
	TotalPersons int
	TotalEvents  int
	TotalTasks   int
	TotalPlaces  int
	TotalTopics  int
}

// EnsureTable creates the entity_links table if it doesn't exist.
func EnsureTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS entity_links (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_label TEXT,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			target_label TEXT,
			relationship TEXT NOT NULL,
			confidence REAL DEFAULT 1.0,
			metadata TEXT,
			created_at TEXT DEFAULT (datetime('now')),
			UNIQUE(source_type, source_id, target_type, target_id, relationship)
		);
		CREATE INDEX IF NOT EXISTS idx_entity_links_source ON entity_links(source_type, source_id);
		CREATE INDEX IF NOT EXISTS idx_entity_links_target ON entity_links(target_type, target_id);
	`)
	return err
}

// UpsertLink inserts or updates an entity link.
func UpsertLink(ctx context.Context, db *sql.DB, link EntityLink) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO entity_links (source_type, source_id, source_label, target_type, target_id, target_label, relationship, confidence, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(source_type, source_id, target_type, target_id, relationship)
		 DO UPDATE SET source_label=excluded.source_label, target_label=excluded.target_label, confidence=excluded.confidence, metadata=excluded.metadata, created_at=datetime('now')`,
		link.SourceType, link.SourceID, link.SourceLabel,
		link.TargetType, link.TargetID, link.TargetLabel,
		link.Relationship, link.Confidence, link.Metadata,
	)
	return err
}

// FindRelated finds entities related to the given entity.
func FindRelated(ctx context.Context, db *sql.DB, entityType, entityID string, limit int) ([]EntityLink, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, source_type, source_id, source_label, target_type, target_id, target_label, relationship, confidence, metadata, created_at
		 FROM entity_links
		 WHERE (source_type = ? AND source_id = ?) OR (target_type = ? AND target_id = ?)
		 ORDER BY confidence DESC, created_at DESC
		 LIMIT ?`,
		entityType, entityID, entityType, entityID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLinks(rows)
}

// FindByRelationship finds links of a specific relationship type.
func FindByRelationship(ctx context.Context, db *sql.DB, relationship string, limit int) ([]EntityLink, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, source_type, source_id, source_label, target_type, target_id, target_label, relationship, confidence, metadata, created_at
		 FROM entity_links WHERE relationship = ? ORDER BY created_at DESC LIMIT ?`,
		relationship, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLinks(rows)
}

// GetStats returns knowledge graph statistics.
func GetStats(ctx context.Context, db *sql.DB) (*GraphStats, error) {
	stats := &GraphStats{}
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM entity_links").Scan(&stats.TotalLinks)
	db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT source_id) FROM entity_links WHERE source_type='person'").Scan(&stats.TotalPersons)
	db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT source_id) FROM entity_links WHERE source_type='event'").Scan(&stats.TotalEvents)
	db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT source_id) FROM entity_links WHERE source_type='task'").Scan(&stats.TotalTasks)
	db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT source_id) FROM entity_links WHERE source_type='place'").Scan(&stats.TotalPlaces)
	db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT source_id) FROM entity_links WHERE source_type='topic'").Scan(&stats.TotalTopics)
	return stats, nil
}

// BuildFromDB scans existing database tables and builds entity links.
func BuildFromDB(ctx context.Context, db *sql.DB) (int, error) {
	if err := EnsureTable(db); err != nil {
		return 0, fmt.Errorf("ensure entity_links table: %w", err)
	}

	var count int

	// Link contacts to emails.
	n, _ := buildContactEmailLinks(ctx, db)
	count += n

	// Link calendar events to attendees.
	n, _ = buildCalendarAttendeeLinks(ctx, db)
	count += n

	// Link tasks to projects.
	n, _ = buildTaskProjectLinks(ctx, db)
	count += n

	return count, nil
}

func buildContactEmailLinks(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT id, name, email FROM contacts WHERE email IS NOT NULL AND email != ''")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var id, name, email string
		if err := rows.Scan(&id, &name, &email); err != nil {
			continue
		}
		// Count emails from this contact.
		var emailCount int
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages WHERE from_addr LIKE ?", "%"+email+"%").Scan(&emailCount)
		if emailCount > 0 {
			confidence := float64(emailCount) / 100.0
			if confidence > 1.0 {
				confidence = 1.0
			}
			UpsertLink(ctx, db, EntityLink{
				SourceType: "person", SourceID: id, SourceLabel: name,
				TargetType: "email_thread", TargetID: email, TargetLabel: fmt.Sprintf("%d emails", emailCount),
				Relationship: "communicates", Confidence: confidence,
				Metadata: fmt.Sprintf(`{"email_count":%d}`, emailCount),
			})
			count++
		}
	}
	return count, nil
}

func buildCalendarAttendeeLinks(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT id, summary, attendees, start_time FROM calendar_events WHERE attendees IS NOT NULL AND attendees != '' AND attendees != '[]'")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var id, summary, attendees, startTime string
		if err := rows.Scan(&id, &summary, &attendees, &startTime); err != nil {
			continue
		}
		// Parse attendee emails from JSON array.
		emails := parseJSONStringArray(attendees)
		for _, email := range emails {
			UpsertLink(ctx, db, EntityLink{
				SourceType: "event", SourceID: id, SourceLabel: summary,
				TargetType: "person", TargetID: email, TargetLabel: email,
				Relationship: "attends", Confidence: 1.0,
				Metadata: fmt.Sprintf(`{"event_time":"%s"}`, startTime),
			})
			count++
		}
	}
	return count, nil
}

func buildTaskProjectLinks(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT id, title, project FROM tasks WHERE project IS NOT NULL AND project != ''")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var id, title, project string
		if err := rows.Scan(&id, &title, &project); err != nil {
			continue
		}
		UpsertLink(ctx, db, EntityLink{
			SourceType: "task", SourceID: id, SourceLabel: title,
			TargetType: "topic", TargetID: "project:" + project, TargetLabel: project,
			Relationship: "about", Confidence: 1.0,
		})
		count++
	}
	return count, nil
}

func parseJSONStringArray(s string) []string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") {
		return nil
	}
	s = strings.Trim(s, "[]")
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, `"`)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func scanLinks(rows *sql.Rows) ([]EntityLink, error) {
	var links []EntityLink
	for rows.Next() {
		var l EntityLink
		var metadata sql.NullString
		if err := rows.Scan(&l.ID, &l.SourceType, &l.SourceID, &l.SourceLabel, &l.TargetType, &l.TargetID, &l.TargetLabel, &l.Relationship, &l.Confidence, &metadata, &l.CreatedAt); err != nil {
			continue
		}
		l.Metadata = metadata.String
		links = append(links, l)
	}
	return links, nil
}

// PruneOlderThan removes links older than the given duration.
func PruneOlderThan(ctx context.Context, db *sql.DB, age time.Duration) (int64, error) {
	cutoff := time.Now().Add(-age).Format("2006-01-02T15:04:05")
	result, err := db.ExecContext(ctx, "DELETE FROM entity_links WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
