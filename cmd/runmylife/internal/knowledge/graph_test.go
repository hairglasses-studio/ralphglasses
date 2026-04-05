package knowledge

import (
	"context"
	"testing"

	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

func TestEnsureTable_Idempotent(t *testing.T) {
	db := testutil.TestDB(t)
	// entity_links already created by migrations, but EnsureTable should be safe to call again
	if err := EnsureTable(db); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	if err := EnsureTable(db); err != nil {
		t.Fatalf("EnsureTable second call: %v", err)
	}
}

func TestUpsertLink_New(t *testing.T) {
	db := testutil.TestDB(t)
	EnsureTable(db)
	ctx := context.Background()

	err := UpsertLink(ctx, db, EntityLink{
		SourceType: "person", SourceID: "alice-1", SourceLabel: "Alice",
		TargetType: "email_thread", TargetID: "alice@example.com", TargetLabel: "10 emails",
		Relationship: "communicates", Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("UpsertLink: %v", err)
	}

	links, err := FindRelated(ctx, db, "person", "alice-1", 10)
	if err != nil {
		t.Fatalf("FindRelated: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].SourceLabel != "Alice" {
		t.Errorf("SourceLabel = %q, want Alice", links[0].SourceLabel)
	}
	if links[0].Confidence != 0.9 {
		t.Errorf("Confidence = %v, want 0.9", links[0].Confidence)
	}
}

func TestUpsertLink_Update(t *testing.T) {
	db := testutil.TestDB(t)
	EnsureTable(db)
	ctx := context.Background()

	link := EntityLink{
		SourceType: "person", SourceID: "bob-1", SourceLabel: "Bob",
		TargetType: "email_thread", TargetID: "bob@example.com", TargetLabel: "5 emails",
		Relationship: "communicates", Confidence: 0.5,
	}
	UpsertLink(ctx, db, link)

	// Update with higher confidence
	link.Confidence = 0.95
	link.TargetLabel = "20 emails"
	UpsertLink(ctx, db, link)

	links, _ := FindRelated(ctx, db, "person", "bob-1", 10)
	if len(links) != 1 {
		t.Fatalf("expected 1 link after upsert, got %d", len(links))
	}
	if links[0].Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95 after update", links[0].Confidence)
	}
	if links[0].TargetLabel != "20 emails" {
		t.Errorf("TargetLabel = %q, want '20 emails' after update", links[0].TargetLabel)
	}
}

func TestFindRelated(t *testing.T) {
	db := testutil.TestDB(t)
	EnsureTable(db)
	ctx := context.Background()

	// Create multiple links for a person
	UpsertLink(ctx, db, EntityLink{
		SourceType: "person", SourceID: "carol-1", SourceLabel: "Carol",
		TargetType: "email_thread", TargetID: "carol@ex.com", TargetLabel: "emails",
		Relationship: "communicates", Confidence: 0.8,
	})
	UpsertLink(ctx, db, EntityLink{
		SourceType: "event", SourceID: "evt-1", SourceLabel: "Meeting",
		TargetType: "person", TargetID: "carol-1", TargetLabel: "Carol",
		Relationship: "attends", Confidence: 1.0,
	})

	links, err := FindRelated(ctx, db, "person", "carol-1", 10)
	if err != nil {
		t.Fatalf("FindRelated: %v", err)
	}
	if len(links) != 2 {
		t.Errorf("expected 2 links for carol-1, got %d", len(links))
	}
}

func TestFindByRelationship(t *testing.T) {
	db := testutil.TestDB(t)
	EnsureTable(db)
	ctx := context.Background()

	UpsertLink(ctx, db, EntityLink{
		SourceType: "event", SourceID: "evt-1", SourceLabel: "Standup",
		TargetType: "person", TargetID: "dave@ex.com", TargetLabel: "Dave",
		Relationship: "attends", Confidence: 1.0,
	})
	UpsertLink(ctx, db, EntityLink{
		SourceType: "person", SourceID: "dave-1", SourceLabel: "Dave",
		TargetType: "email_thread", TargetID: "dave@ex.com", TargetLabel: "3 emails",
		Relationship: "communicates", Confidence: 0.5,
	})

	attends, err := FindByRelationship(ctx, db, "attends", 10)
	if err != nil {
		t.Fatalf("FindByRelationship: %v", err)
	}
	if len(attends) != 1 {
		t.Errorf("expected 1 'attends' link, got %d", len(attends))
	}
	if attends[0].SourceLabel != "Standup" {
		t.Errorf("SourceLabel = %q, want Standup", attends[0].SourceLabel)
	}
}

func TestBuildFromDB_EmptyTables(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	count, err := BuildFromDB(ctx, db)
	if err != nil {
		t.Fatalf("BuildFromDB: %v", err)
	}
	if count != 0 {
		t.Errorf("BuildFromDB on empty DB = %d, want 0", count)
	}
}

func TestBuildFromDB_Contacts(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	// Seed a contact with matching email in gmail_messages
	db.ExecContext(ctx,
		`INSERT INTO contacts (id, name, email) VALUES ('c1', 'Eve', 'eve@example.com')`)
	db.ExecContext(ctx,
		`INSERT INTO gmail_messages (id, thread_id, from_addr, subject, snippet, body, timestamp, labels, triaged)
		 VALUES ('m1', 't1', 'eve@example.com', 'Hello', '', '', datetime('now'), '', 0)`)

	count, err := BuildFromDB(ctx, db)
	if err != nil {
		t.Fatalf("BuildFromDB: %v", err)
	}
	if count < 1 {
		t.Errorf("BuildFromDB with contact+email = %d, want >= 1", count)
	}

	// Verify the link was created
	links, _ := FindRelated(ctx, db, "person", "c1", 10)
	if len(links) == 0 {
		t.Error("expected link from contact to email thread")
	}
}

func TestGetStats(t *testing.T) {
	db := testutil.TestDB(t)
	EnsureTable(db)
	ctx := context.Background()

	UpsertLink(ctx, db, EntityLink{
		SourceType: "person", SourceID: "p1", SourceLabel: "Alice",
		TargetType: "topic", TargetID: "t1", TargetLabel: "Go",
		Relationship: "about", Confidence: 1.0,
	})

	stats, err := GetStats(ctx, db)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalLinks != 1 {
		t.Errorf("TotalLinks = %d, want 1", stats.TotalLinks)
	}
	if stats.TotalPersons != 1 {
		t.Errorf("TotalPersons = %d, want 1", stats.TotalPersons)
	}
}
