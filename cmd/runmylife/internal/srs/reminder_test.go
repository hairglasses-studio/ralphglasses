package srs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

func TestCountDueCards_None(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	count := CountDueCards(ctx, db)
	if count != 0 {
		t.Errorf("CountDueCards = %d, want 0", count)
	}
}

func TestCountDueCards_Mixed(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	// 2 due cards (next_review_at in the past)
	pastReview := time.Now().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
	futureReview := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")

	db.ExecContext(ctx, `INSERT INTO srs_cards (front, back, next_review_at) VALUES ('Q1', 'A1', ?)`, pastReview)
	db.ExecContext(ctx, `INSERT INTO srs_cards (front, back, next_review_at) VALUES ('Q2', 'A2', ?)`, pastReview)
	db.ExecContext(ctx, `INSERT INTO srs_cards (front, back, next_review_at) VALUES ('Q3', 'A3', ?)`, futureReview)

	count := CountDueCards(ctx, db)
	if count != 2 {
		t.Errorf("CountDueCards = %d, want 2", count)
	}
}

func TestGetDueCardsSummary(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	pastReview := time.Now().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")

	db.ExecContext(ctx,
		`INSERT INTO srs_cards (front, back, topic, next_review_at) VALUES ('What is Go?', 'A language', 'programming', ?)`,
		pastReview)
	db.ExecContext(ctx,
		`INSERT INTO srs_cards (front, back, topic, next_review_at) VALUES ('What is SQL?', 'A query language', 'databases', ?)`,
		pastReview)

	cards := GetDueCardsSummary(ctx, db, 5)
	if len(cards) != 2 {
		t.Fatalf("GetDueCardsSummary = %d cards, want 2", len(cards))
	}

	if cards[0].Front != "What is Go?" && cards[1].Front != "What is Go?" {
		t.Error("expected 'What is Go?' in results")
	}
}

func TestGetDueCardsSummary_Limit(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	pastReview := time.Now().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
	for i := 0; i < 5; i++ {
		db.ExecContext(ctx, `INSERT INTO srs_cards (front, back, next_review_at) VALUES (?, 'A', ?)`,
			"Q"+string(rune('A'+i)), pastReview)
	}

	cards := GetDueCardsSummary(ctx, db, 2)
	if len(cards) != 2 {
		t.Errorf("GetDueCardsSummary with limit 2 = %d cards, want 2", len(cards))
	}
}

func TestFormatReminder_NoDue(t *testing.T) {
	result := FormatReminder(0, nil)
	if result != "" {
		t.Errorf("FormatReminder(0) = %q, want empty", result)
	}
}

func TestFormatReminder_WithCards(t *testing.T) {
	cards := []CardSummary{
		{ID: 1, Front: "What is Go?", Topic: "programming"},
		{ID: 2, Front: "What is SQL?", Topic: ""},
	}

	result := FormatReminder(5, cards)
	if !strings.Contains(result, "5 SRS cards due") {
		t.Errorf("expected '5 SRS cards due' in %q", result)
	}
	if !strings.Contains(result, "[programming]") {
		t.Errorf("expected topic tag in %q", result)
	}
	if !strings.Contains(result, "What is SQL?") {
		t.Errorf("expected card without topic in %q", result)
	}
}

func TestFormatReminder_Truncation(t *testing.T) {
	longFront := strings.Repeat("x", 100)
	cards := []CardSummary{
		{ID: 1, Front: longFront, Topic: "test"},
	}

	result := FormatReminder(1, cards)
	if strings.Contains(result, longFront) {
		t.Error("long front text should be truncated")
	}
	if !strings.Contains(result, "...") {
		t.Error("truncated text should end with ...")
	}
}
