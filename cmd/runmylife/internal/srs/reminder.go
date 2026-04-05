package srs

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// CardSummary holds display info for a due card.
type CardSummary struct {
	ID    int64
	Front string
	Topic string
}

// CountDueCards returns the number of SRS cards currently due for review.
func CountDueCards(ctx context.Context, db *sql.DB) int {
	var count int
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM srs_cards WHERE next_review_at <= datetime('now')").Scan(&count)
	return count
}

// GetDueCardsSummary returns up to `limit` due cards with their front text and topic.
func GetDueCardsSummary(ctx context.Context, db *sql.DB, limit int) []CardSummary {
	if limit <= 0 {
		limit = 10
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, front, topic FROM srs_cards
		 WHERE next_review_at <= datetime('now')
		 ORDER BY next_review_at ASC
		 LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var cards []CardSummary
	for rows.Next() {
		var c CardSummary
		if rows.Scan(&c.ID, &c.Front, &c.Topic) == nil {
			cards = append(cards, c)
		}
	}
	return cards
}

// FormatReminder produces a human-readable reminder string for due SRS cards.
func FormatReminder(count int, cards []CardSummary) string {
	if count == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d SRS cards due for review", count)

	if len(cards) > 0 {
		b.WriteString(":\n")
		for i, c := range cards {
			if c.Topic != "" {
				fmt.Fprintf(&b, "  %d. [%s] %s\n", i+1, c.Topic, truncate(c.Front, 60))
			} else {
				fmt.Fprintf(&b, "  %d. %s\n", i+1, truncate(c.Front, 60))
			}
		}
	}

	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
