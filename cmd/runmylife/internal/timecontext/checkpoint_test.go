package timecontext

import (
	"context"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

func TestSaveCheckpoint(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	err := SaveCheckpoint(ctx, db, "focus_start", "session-1", time.Now().Add(30*time.Minute), 15)
	if err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM time_checkpoints").Scan(&count)
	if count != 1 {
		t.Errorf("time_checkpoints rows = %d, want 1", count)
	}
}

func TestSaveCheckpoint_DefaultAlertMinutes(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	SaveCheckpoint(ctx, db, "meeting", "cal-1", time.Now().Add(time.Hour), 0) // 0 → default 15

	var alert int
	db.QueryRow("SELECT alert_minutes_before FROM time_checkpoints LIMIT 1").Scan(&alert)
	if alert != 15 {
		t.Errorf("alert_minutes_before = %d, want 15 (default)", alert)
	}
}

func TestLoadCheckpoints_Empty(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	cps, err := LoadCheckpoints(ctx, db, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("LoadCheckpoints: %v", err)
	}
	if len(cps) != 0 {
		t.Errorf("expected 0 checkpoints, got %d", len(cps))
	}
}

func TestLoadCheckpoints_WithData(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	future := time.Now().Add(time.Hour)
	SaveCheckpoint(ctx, db, "focus_end", "s-1", future, 10)
	SaveCheckpoint(ctx, db, "meeting", "cal-2", future.Add(time.Hour), 15)

	cps, err := LoadCheckpoints(ctx, db, time.Now())
	if err != nil {
		t.Fatalf("LoadCheckpoints: %v", err)
	}
	if len(cps) != 2 {
		t.Errorf("expected 2 checkpoints, got %d", len(cps))
	}
	if cps[0].EventType != "focus_end" {
		t.Errorf("first checkpoint type = %q, want focus_end", cps[0].EventType)
	}
}

func TestLoadCheckpoints_ExcludesAcknowledged(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	future := time.Now().Add(time.Hour)
	SaveCheckpoint(ctx, db, "focus_end", "s-1", future, 10)
	SaveCheckpoint(ctx, db, "meeting", "cal-2", future.Add(time.Hour), 15)

	// Acknowledge the first one
	AcknowledgeCheckpoint(ctx, db, 1)

	cps, err := LoadCheckpoints(ctx, db, time.Now())
	if err != nil {
		t.Fatalf("LoadCheckpoints: %v", err)
	}
	if len(cps) != 1 {
		t.Errorf("expected 1 unacknowledged checkpoint, got %d", len(cps))
	}
}

func TestAcknowledgeCheckpoint(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	SaveCheckpoint(ctx, db, "test", "ref-1", time.Now().Add(time.Hour), 15)

	err := AcknowledgeCheckpoint(ctx, db, 1)
	if err != nil {
		t.Fatalf("AcknowledgeCheckpoint: %v", err)
	}

	var ack int
	db.QueryRow("SELECT acknowledged FROM time_checkpoints WHERE id = 1").Scan(&ack)
	if ack != 1 {
		t.Errorf("acknowledged = %d, want 1", ack)
	}
}

func TestLoadCheckpoints_FiltersBySince(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	past := time.Now().Add(-2 * time.Hour)
	future := time.Now().Add(time.Hour)

	SaveCheckpoint(ctx, db, "old", "ref-old", past, 10)
	SaveCheckpoint(ctx, db, "new", "ref-new", future, 10)

	cps, err := LoadCheckpoints(ctx, db, time.Now())
	if err != nil {
		t.Fatalf("LoadCheckpoints: %v", err)
	}
	if len(cps) != 1 {
		t.Errorf("expected 1 future checkpoint, got %d", len(cps))
	}
	if len(cps) > 0 && cps[0].EventType != "new" {
		t.Errorf("checkpoint type = %q, want new", cps[0].EventType)
	}
}
