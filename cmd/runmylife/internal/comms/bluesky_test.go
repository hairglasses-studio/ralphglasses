package comms

import (
	"context"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/db"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestScanBluesky_NoData(t *testing.T) {
	database := testDB(t)
	msgs, err := ScanBluesky(context.Background(), database.DB, "did:plc:myuser123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestScanBluesky_EmptyDID(t *testing.T) {
	database := testDB(t)
	msgs, err := ScanBluesky(context.Background(), database.DB, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgs != nil {
		t.Errorf("expected nil for empty DID, got %v", msgs)
	}
}

func TestScanBluesky_UnrepliedPosts(t *testing.T) {
	database := testDB(t)
	myDID := "did:plc:myuser123"
	otherDID := "did:plc:otheruser456"

	now := time.Now().UTC()

	// Insert incoming post from another user
	_, err := database.DB.Exec(
		`INSERT INTO bluesky_posts (uri, cid, author, text, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"at://did:plc:otheruser456/app.bsky.feed.post/abc123",
		"cid1",
		otherDID,
		"Hey, what do you think about this?",
		now.Add(-2*time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert incoming: %v", err)
	}

	// Insert another incoming post
	_, err = database.DB.Exec(
		`INSERT INTO bluesky_posts (uri, cid, author, text, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"at://did:plc:otheruser456/app.bsky.feed.post/def456",
		"cid2",
		otherDID,
		"Also check this out",
		now.Add(-1*time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert incoming 2: %v", err)
	}

	msgs, err := ScanBluesky(context.Background(), database.DB, myDID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Verify message fields
	msg := msgs[0] // most recent first (ORDER BY created_at DESC)
	if msg.Channel != ChannelBluesky {
		t.Errorf("channel = %v, want bluesky", msg.Channel)
	}
	if msg.ContactID != otherDID {
		t.Errorf("contact = %v, want %s", msg.ContactID, otherDID)
	}
	if msg.Direction != DirectionIncoming {
		t.Errorf("direction = %v, want incoming", msg.Direction)
	}
	if !msg.NeedsReply {
		t.Error("expected NeedsReply = true")
	}
}

func TestScanBluesky_RepliedPostsExcluded(t *testing.T) {
	database := testDB(t)
	myDID := "did:plc:myuser123"
	otherDID := "did:plc:otheruser456"

	now := time.Now().UTC()

	// Incoming post
	_, err := database.DB.Exec(
		`INSERT INTO bluesky_posts (uri, cid, author, text, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"at://other/post1", "cid1", otherDID, "Question?",
		now.Add(-2*time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// My reply (after their post)
	_, err = database.DB.Exec(
		`INSERT INTO bluesky_posts (uri, cid, author, text, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"at://me/reply1", "cid2", myDID, "Here's my answer",
		now.Add(-1*time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert reply: %v", err)
	}

	msgs, err := ScanBluesky(context.Background(), database.DB, myDID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 (replied), got %d", len(msgs))
	}
}
