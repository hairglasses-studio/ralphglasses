package comms

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ScanBluesky scans bluesky_posts for unreplied incoming posts/mentions.
// myDID is the user's Bluesky DID (Decentralized Identifier).
func ScanBluesky(ctx context.Context, db *sql.DB, myDID string) ([]UnifiedMessage, error) {
	if myDID == "" {
		return nil, nil
	}

	// Find posts from others that mention or reply to the user
	// and where the user hasn't replied after them
	rows, err := db.QueryContext(ctx, `
		SELECT p.uri, p.author, p.text, p.created_at
		FROM bluesky_posts p
		WHERE p.author != ?
		  AND p.text != ''
		  AND p.created_at IS NOT NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM bluesky_posts p2
		    WHERE p2.author = ?
		      AND p2.created_at > p.created_at
		  )
		ORDER BY p.created_at DESC
		LIMIT 20
	`, myDID, myDID)
	if err != nil {
		return nil, fmt.Errorf("scan Bluesky: %w", err)
	}
	defer rows.Close()

	var msgs []UnifiedMessage
	for rows.Next() {
		var uri, author, text, createdAt string
		if err := rows.Scan(&uri, &author, &text, &createdAt); err != nil {
			continue
		}
		parsed, _ := time.Parse(time.RFC3339, createdAt)
		if parsed.IsZero() {
			parsed, _ = time.Parse("2006-01-02T15:04:05", createdAt)
		}
		if parsed.IsZero() {
			parsed, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		msgs = append(msgs, UnifiedMessage{
			ID:               fmt.Sprintf("bluesky-%s", uri),
			Channel:          ChannelBluesky,
			ChannelMessageID: uri,
			ContactID:        author,
			ContactName:      author,
			Preview:          truncate(text, 120),
			Direction:        DirectionIncoming,
			ReceivedAt:       parsed,
			NeedsReply:       true,
			ConversationID:   uri,
		})
	}
	return msgs, nil
}
