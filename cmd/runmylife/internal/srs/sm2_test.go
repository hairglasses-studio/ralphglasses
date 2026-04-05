package srs

import (
	"testing"
	"time"
)

func TestNewCard(t *testing.T) {
	c := NewCard()
	if c.EasinessFactor != 2.5 {
		t.Errorf("EasinessFactor = %v, want 2.5", c.EasinessFactor)
	}
	if c.IntervalDays != 0 {
		t.Errorf("IntervalDays = %d, want 0", c.IntervalDays)
	}
	if c.Repetitions != 0 {
		t.Errorf("Repetitions = %d, want 0", c.Repetitions)
	}
	if time.Until(c.NextReview) > time.Second {
		t.Errorf("NextReview should be approximately now")
	}
}

func applyReview(c Card, quality int) Card {
	r := Review(c, quality)
	c.EasinessFactor = r.NewEF
	c.IntervalDays = r.NewInterval
	c.Repetitions = r.NewRepetitions
	c.NextReview = r.NextReview
	return c
}

func TestReview_PerfectRecall(t *testing.T) {
	c := applyReview(NewCard(), 5)
	if c.Repetitions != 1 {
		t.Errorf("Repetitions = %d, want 1", c.Repetitions)
	}
	if c.IntervalDays != 1 {
		t.Errorf("IntervalDays = %d, want 1", c.IntervalDays)
	}
	// EF: 2.5 + (0.1 - (5-5)*(0.08+(5-5)*0.02)) = 2.6
	if c.EasinessFactor < 2.59 || c.EasinessFactor > 2.61 {
		t.Errorf("EasinessFactor = %v, want ~2.6", c.EasinessFactor)
	}
}

func TestReview_FailedRecall(t *testing.T) {
	c := NewCard()
	c = applyReview(c, 5)
	c = applyReview(c, 5)
	c = applyReview(c, 5)
	if c.Repetitions != 3 {
		t.Fatalf("setup: Repetitions = %d, want 3", c.Repetitions)
	}

	// Fail: quality < 3 resets repetitions
	c = applyReview(c, 2)
	if c.Repetitions != 0 {
		t.Errorf("Repetitions = %d, want 0 after fail", c.Repetitions)
	}
	if c.IntervalDays != 1 {
		t.Errorf("IntervalDays = %d, want 1 after fail", c.IntervalDays)
	}
}

func TestReview_EFFloor(t *testing.T) {
	c := NewCard()
	for i := 0; i < 20; i++ {
		c = applyReview(c, 3)
	}
	if c.EasinessFactor < 1.3 {
		t.Errorf("EasinessFactor = %v, should not drop below 1.3", c.EasinessFactor)
	}
}

func TestReview_IntervalProgression(t *testing.T) {
	c := NewCard()
	c = applyReview(c, 4)
	if c.IntervalDays != 1 {
		t.Errorf("after rep 1: IntervalDays = %d, want 1", c.IntervalDays)
	}
	c = applyReview(c, 4)
	if c.IntervalDays != 6 {
		t.Errorf("after rep 2: IntervalDays = %d, want 6", c.IntervalDays)
	}
	c = applyReview(c, 4)
	// rep 3: interval = round(6 * EF)
	if c.IntervalDays < 12 {
		t.Errorf("after rep 3: IntervalDays = %d, want >= 12", c.IntervalDays)
	}
}

func TestReview_WasCorrect(t *testing.T) {
	r := Review(NewCard(), 5)
	if !r.WasCorrect {
		t.Error("quality 5 should be correct")
	}
	r = Review(NewCard(), 3)
	if !r.WasCorrect {
		t.Error("quality 3 should be correct")
	}
	r = Review(NewCard(), 2)
	if r.WasCorrect {
		t.Error("quality 2 should not be correct")
	}
}

func TestReview_QualityClamping(t *testing.T) {
	// Should not panic with out-of-range quality
	r := Review(NewCard(), -1)
	if r.WasCorrect {
		t.Error("quality -1 should not be correct")
	}
	r = Review(NewCard(), 10)
	if !r.WasCorrect {
		t.Error("quality 10 (clamped to 5) should be correct")
	}
}

func TestIsDue(t *testing.T) {
	c := NewCard()
	if !IsDue(c) {
		t.Error("new card should be due immediately")
	}

	c.NextReview = time.Now().Add(24 * time.Hour)
	if IsDue(c) {
		t.Error("card due tomorrow should not be due now")
	}

	c.NextReview = time.Now().Add(-time.Hour)
	if !IsDue(c) {
		t.Error("card due an hour ago should be due")
	}
}

func TestDaysUntilReview(t *testing.T) {
	c := NewCard()
	c.NextReview = time.Now().Add(48 * time.Hour)
	days := DaysUntilReview(c)
	if days < 1 || days > 2 {
		t.Errorf("DaysUntilReview = %d, want 1-2", days)
	}

	c.NextReview = time.Now().Add(-24 * time.Hour)
	days = DaysUntilReview(c)
	if days > 0 {
		t.Errorf("overdue card DaysUntilReview = %d, want <= 0", days)
	}
}

func TestDecayAlert(t *testing.T) {
	c := NewCard()
	if DecayAlert(c) {
		t.Error("new card should not trigger decay alert")
	}

	c.EasinessFactor = 1.5
	c.Repetitions = 3
	if DecayAlert(c) {
		t.Error("EF 1.5 with 3 reps should not trigger (need EF < 1.5)")
	}

	c.EasinessFactor = 1.3
	c.Repetitions = 3
	if !DecayAlert(c) {
		t.Error("EF 1.3 with 3 reps should trigger decay alert")
	}

	c.EasinessFactor = 1.3
	c.Repetitions = 2
	if DecayAlert(c) {
		t.Error("EF 1.3 with only 2 reps should not trigger (need > 2)")
	}
}
