package comms

import (
	"testing"
	"time"
)

func TestScoreUrgency_VIPOverdue(t *testing.T) {
	msg := UnifiedMessage{
		Preview:    "Can you review this urgent document?",
		ReceivedAt: time.Now().Add(-48 * time.Hour),
	}
	u := ScoreUrgency(msg, TierVIP, 24, 1, 1.0)

	if u.ContactTierScore != 0.3 {
		t.Errorf("VIP tier score = %.2f, want 0.3", u.ContactTierScore)
	}
	if u.TimeDecayScore < 0.15 {
		t.Errorf("48h overdue TimeDecay = %.2f, want >= 0.15", u.TimeDecayScore)
	}
	if u.ContentScore == 0 {
		t.Error("expected content score for 'urgent' + question mark")
	}
	if u.Total < 0.5 {
		t.Errorf("VIP overdue total = %.2f, want >= 0.5", u.Total)
	}
}

func TestScoreUrgency_LowTierFresh(t *testing.T) {
	msg := UnifiedMessage{
		Preview:    "hey what's up",
		ReceivedAt: time.Now().Add(-30 * time.Minute),
	}
	u := ScoreUrgency(msg, TierLow, 24, 1, 1.0)

	if u.ContactTierScore != 0.05 {
		t.Errorf("Low tier score = %.2f, want 0.05", u.ContactTierScore)
	}
	if u.Total > 0.3 {
		t.Errorf("fresh low-tier total = %.2f, want <= 0.3", u.Total)
	}
}

func TestScoreUrgency_QuestionContent(t *testing.T) {
	msg := UnifiedMessage{
		Preview:    "Are you available for a meeting tomorrow?",
		ReceivedAt: time.Now(),
	}
	u := ScoreUrgency(msg, TierNormal, 24, 1, 1.0)

	if u.ContentScore < 0.08 {
		t.Errorf("question content score = %.2f, want >= 0.08", u.ContentScore)
	}
}

func TestScoreUrgency_TimeSensitive(t *testing.T) {
	msg := UnifiedMessage{
		Preview:    "Need this asap, deadline is today",
		ReceivedAt: time.Now(),
	}
	u := ScoreUrgency(msg, TierNormal, 24, 1, 1.0)

	if u.ContentScore < 0.07 {
		t.Errorf("time-sensitive content score = %.2f, want >= 0.07", u.ContentScore)
	}
}

func TestScoreUrgency_Momentum(t *testing.T) {
	msg := UnifiedMessage{
		Preview:    "hello",
		ReceivedAt: time.Now(),
	}
	u := ScoreUrgency(msg, TierNormal, 24, 5, 1.0)

	if u.MomentumScore == 0 {
		t.Error("expected momentum score with 5 unreplied messages")
	}
}

func TestScoreUrgency_Reciprocity(t *testing.T) {
	msg := UnifiedMessage{
		Preview:    "hello",
		ReceivedAt: time.Now(),
	}
	u := ScoreUrgency(msg, TierNormal, 24, 1, 5.0)

	if u.ReciprocityScore == 0 {
		t.Error("expected reciprocity score with 5.0 ratio")
	}
}

func TestScoreUrgency_Cap(t *testing.T) {
	msg := UnifiedMessage{
		Preview:    "urgent help please asap emergency?",
		ReceivedAt: time.Now().Add(-100 * time.Hour),
	}
	u := ScoreUrgency(msg, TierVIP, 24, 10, 10.0)

	if u.Total > 1.0 {
		t.Errorf("total = %.2f, should be capped at 1.0", u.Total)
	}
}

func TestGhostProbability_NoData(t *testing.T) {
	p := GhostProbability(0, 24)
	if p != 0.5 {
		t.Errorf("no data ghost prob = %.2f, want 0.5", p)
	}
}

func TestGhostProbability_Fresh(t *testing.T) {
	// avgReply = 60min, hoursSince = 0.5h → well within normal
	p := GhostProbability(60, 0.5)
	if p > 0.3 {
		t.Errorf("fresh message ghost prob = %.2f, want <= 0.3", p)
	}
}

func TestGhostProbability_Overdue(t *testing.T) {
	// avgReply = 60min (1h), hoursSince = 5h → 5x avg
	p := GhostProbability(60, 5)
	if p < 0.8 {
		t.Errorf("5x overdue ghost prob = %.2f, want >= 0.8", p)
	}
	if p > 0.99 {
		t.Errorf("ghost prob = %.2f, should cap at 0.99", p)
	}
}

func TestRelationshipWeather(t *testing.T) {
	tests := []struct {
		days  float64
		ratio float64
		want  string
	}{
		{3, 1.0, "sunny"},
		{8, 1.0, "cloudy"},
		{8, 3.0, "cloudy"},
		{15, 1.0, "stormy"},
		{5, 4.5, "stormy"},
		{31, 1.0, "drought"},
	}
	for _, tt := range tests {
		got := RelationshipWeather(tt.days, tt.ratio)
		if got != tt.want {
			t.Errorf("RelationshipWeather(%.0fd, %.1f) = %q, want %q", tt.days, tt.ratio, got, tt.want)
		}
	}
}
