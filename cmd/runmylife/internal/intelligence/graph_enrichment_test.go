package intelligence

import (
	"context"
	"testing"

	"github.com/hairglasses-studio/runmylife/internal/knowledge"
	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

func TestEnrichFromGraph_NoLinks(t *testing.T) {
	db := testutil.TestDB(t)
	knowledge.EnsureTable(db)
	ctx := context.Background()

	suggestions := []Suggestion{
		{Category: "personal", Title: "Reply debt: 5 pending", Description: "Multiple unreplied messages."},
	}

	enriched := EnrichFromGraph(ctx, db, suggestions)
	if len(enriched) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(enriched))
	}
	if enriched[0].Description != suggestions[0].Description {
		t.Error("description should be unchanged with no graph links")
	}
}

func TestEnrichFromGraph_WithLinks(t *testing.T) {
	db := testutil.TestDB(t)
	knowledge.EnsureTable(db)
	ctx := context.Background()

	// Create a person entity with a relationship
	knowledge.UpsertLink(ctx, db, knowledge.EntityLink{
		SourceType: "person", SourceID: "alice-1", SourceLabel: "Alice",
		TargetType: "email_thread", TargetID: "alice@example.com", TargetLabel: "15 emails",
		Relationship: "communicates", Confidence: 0.8,
	})

	suggestions := []Suggestion{
		{Category: "social", Title: "Follow up with Alice", Description: "Alice hasn't replied."},
	}

	enriched := EnrichFromGraph(ctx, db, suggestions)
	if len(enriched) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(enriched))
	}
	if enriched[0].Description == suggestions[0].Description {
		t.Error("expected enriched description to include graph context")
	}
	if !contains(enriched[0].Description, "Related:") {
		t.Errorf("expected 'Related:' in description, got %q", enriched[0].Description)
	}
}

func TestGraphBasedSuggestions_StaleContact(t *testing.T) {
	db := testutil.TestDB(t)
	knowledge.EnsureTable(db)
	ctx := context.Background()

	// Create a calendar event with an attendee
	knowledge.UpsertLink(ctx, db, knowledge.EntityLink{
		SourceType: "event", SourceID: "evt-1", SourceLabel: "Meeting",
		TargetType: "person", TargetID: "bob@example.com", TargetLabel: "Bob",
		Relationship: "attends", Confidence: 1.0,
	})
	// No "communicates" link for Bob → he should show up as stale

	suggestions := GraphBasedSuggestions(ctx, db)

	found := false
	for _, s := range suggestions {
		if contains(s.Title, "Bob") {
			found = true
			if s.Category != "social" {
				t.Errorf("stale contact category = %q, want social", s.Category)
			}
		}
	}
	if !found {
		t.Error("expected suggestion for stale calendar contact Bob")
	}
}

func TestGraphBasedSuggestions_BatchableTasks(t *testing.T) {
	db := testutil.TestDB(t)
	knowledge.EnsureTable(db)
	ctx := context.Background()

	// Create 3 open tasks linked to the same project
	for i := 1; i <= 3; i++ {
		taskID := "task-" + string(rune('0'+i))
		db.ExecContext(ctx,
			`INSERT INTO tasks (id, title, priority, completed) VALUES (?, ?, 2, 0)`,
			taskID, "Task "+string(rune('0'+i)))
		knowledge.UpsertLink(ctx, db, knowledge.EntityLink{
			SourceType: "task", SourceID: taskID, SourceLabel: "Task",
			TargetType: "topic", TargetID: "project:alpha", TargetLabel: "alpha",
			Relationship: "about", Confidence: 1.0,
		})
	}

	suggestions := GraphBasedSuggestions(ctx, db)

	found := false
	for _, s := range suggestions {
		if contains(s.Title, "alpha") {
			found = true
			if s.Category != "personal" {
				t.Errorf("batch category = %q, want personal", s.Category)
			}
		}
	}
	if !found {
		t.Error("expected batch suggestion for project alpha with 3 tasks")
	}
}

func TestEnrichFromGraph_NoMutation(t *testing.T) {
	db := testutil.TestDB(t)
	knowledge.EnsureTable(db)
	ctx := context.Background()

	original := []Suggestion{
		{Category: "personal", Title: "Test", Description: "Original desc"},
	}

	enriched := EnrichFromGraph(ctx, db, original)
	// Ensure original slice was not mutated
	if original[0].Description != "Original desc" {
		t.Error("EnrichFromGraph mutated the original slice")
	}
	_ = enriched
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
