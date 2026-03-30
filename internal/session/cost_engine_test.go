package session

import (
	"testing"
	"time"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func advancingClock(start time.Time, step time.Duration) func() time.Time {
	current := start
	return func() time.Time {
		now := current
		current = current.Add(step)
		return now
	}
}

func TestCostEngine_TrackAndRecord(t *testing.T) {
	ce := DefaultCostEngine()
	ce.TrackSession("s1", ProviderClaude)

	if got := ce.TotalSpent("s1"); got != 0 {
		t.Fatalf("expected 0 total, got %f", got)
	}

	ce.RecordCost("s1", 0.10)
	ce.RecordCost("s1", 0.25)

	if got := ce.TotalSpent("s1"); got != 0.35 {
		t.Fatalf("expected 0.35, got %f", got)
	}
}

func TestCostEngine_RecordCost_ZeroOrNegative(t *testing.T) {
	ce := DefaultCostEngine()
	ce.TrackSession("s1", ProviderClaude)

	if rec := ce.RecordCost("s1", 0); rec != nil {
		t.Fatal("expected nil for zero cost")
	}
	if rec := ce.RecordCost("s1", -1.0); rec != nil {
		t.Fatal("expected nil for negative cost")
	}
	if got := ce.TotalSpent("s1"); got != 0 {
		t.Fatalf("expected 0, got %f", got)
	}
}

func TestCostEngine_RecordCost_UntrackedSession(t *testing.T) {
	ce := DefaultCostEngine()
	if rec := ce.RecordCost("unknown", 1.0); rec != nil {
		t.Fatal("expected nil for untracked session")
	}
}

func TestCostEngine_SpendRate(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	step := 30 * time.Second
	clock := advancingClock(start, step)

	ce := NewCostEngine(nil, BatchConfig{})
	ce.now = clock

	ce.TrackSession("s1", ProviderGemini)

	// First record at t=0 (clock advances to t+30s after call to now in TrackSession)
	ce.RecordCost("s1", 0.50) // recorded at t+30s, clock moves to t+60s
	ce.RecordCost("s1", 0.50) // recorded at t+60s, clock moves to t+90s

	rate := ce.SpendRate("s1") // computed at t+90s
	if rate <= 0 {
		t.Fatalf("expected positive rate, got %f", rate)
	}
}

func TestCostEngine_SpendRate_InsufficientData(t *testing.T) {
	ce := DefaultCostEngine()
	ce.TrackSession("s1", ProviderClaude)

	// No entries yet
	if rate := ce.SpendRate("s1"); rate != 0 {
		t.Fatalf("expected 0 rate with no entries, got %f", rate)
	}

	// One entry still insufficient (need >= 2)
	ce.RecordCost("s1", 0.10)
	if rate := ce.SpendRate("s1"); rate != 0 {
		t.Fatalf("expected 0 rate with one entry, got %f", rate)
	}
}

func TestCostEngine_SpendRate_UntrackedSession(t *testing.T) {
	ce := DefaultCostEngine()
	if rate := ce.SpendRate("nope"); rate != 0 {
		t.Fatalf("expected 0 for untracked, got %f", rate)
	}
}

func TestCostEngine_ThresholdDetection_Rate(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	callCount := 0
	// Each call advances 1 minute.
	clock := func() time.Time {
		now := start.Add(time.Duration(callCount) * time.Minute)
		callCount++
		return now
	}

	thresholds := []SpendThreshold{
		{
			MaxRatePerHour: 1.0, // $1/hr threshold
			FromProvider:   ProviderClaude,
			ToProvider:     ProviderGemini,
			Label:          "test-rate",
		},
	}

	ce := NewCostEngine(thresholds, BatchConfig{})
	ce.now = clock

	ce.TrackSession("s1", ProviderClaude) // t=0min

	// Spend $0.50 over ~2 minutes → $15/hr, well above $1/hr threshold.
	ce.RecordCost("s1", 0.25) // t=1min
	rec := ce.RecordCost("s1", 0.25) // t=2min

	if rec == nil {
		t.Fatal("expected switch recommendation for high rate")
	}
	if rec.ToProvider != ProviderGemini {
		t.Fatalf("expected gemini, got %s", rec.ToProvider)
	}
	if rec.Reason != "spend rate threshold breached" && rec.Reason != "rate and total spend thresholds breached" {
		t.Fatalf("unexpected reason: %s", rec.Reason)
	}
}

func TestCostEngine_ThresholdDetection_TotalSpend(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	callCount := 0
	clock := func() time.Time {
		now := start.Add(time.Duration(callCount) * 30 * time.Minute)
		callCount++
		return now
	}

	thresholds := []SpendThreshold{
		{
			MaxTotalSpend: 0.50,
			FromProvider:  ProviderCodex,
			ToProvider:    ProviderGemini,
			Label:         "test-total",
		},
	}

	ce := NewCostEngine(thresholds, BatchConfig{})
	ce.now = clock

	ce.TrackSession("s1", ProviderCodex)

	// Below threshold
	rec := ce.RecordCost("s1", 0.20)
	if rec != nil {
		t.Fatal("should not trigger below threshold")
	}

	rec = ce.RecordCost("s1", 0.15)
	if rec != nil {
		t.Fatal("should not trigger at $0.35")
	}

	// Push over $0.50
	rec = ce.RecordCost("s1", 0.20)
	if rec == nil {
		t.Fatal("expected switch recommendation for total spend")
	}
	if rec.TotalSpent < 0.50 {
		t.Fatalf("expected total >= 0.50, got %f", rec.TotalSpent)
	}
	if rec.Reason != "total spend threshold breached" && rec.Reason != "rate and total spend thresholds breached" {
		t.Fatalf("unexpected reason: %s", rec.Reason)
	}
}

func TestCostEngine_ThresholdDetection_ProviderMismatch(t *testing.T) {
	thresholds := []SpendThreshold{
		{
			MaxTotalSpend: 0.10,
			FromProvider:  ProviderClaude,
			ToProvider:    ProviderGemini,
		},
	}

	ce := NewCostEngine(thresholds, BatchConfig{})
	ce.TrackSession("s1", ProviderGemini) // different provider

	rec := ce.RecordCost("s1", 1.00)
	if rec != nil {
		t.Fatal("threshold should not apply to mismatched provider")
	}
}

func TestCostEngine_CheckThresholds(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	callCount := 0
	clock := func() time.Time {
		now := start.Add(time.Duration(callCount) * time.Minute)
		callCount++
		return now
	}

	thresholds := []SpendThreshold{
		{MaxTotalSpend: 0.50, ToProvider: ProviderGemini},
	}

	ce := NewCostEngine(thresholds, BatchConfig{})
	ce.now = clock

	ce.TrackSession("s1", ProviderClaude)
	ce.RecordCost("s1", 0.60)

	rec := ce.CheckThresholds("s1")
	if rec == nil {
		t.Fatal("expected recommendation from CheckThresholds")
	}

	// Untracked session
	if ce.CheckThresholds("nope") != nil {
		t.Fatal("expected nil for untracked session")
	}
}

func TestCostEngine_BatchQueue_FlushOnSize(t *testing.T) {
	cfg := BatchConfig{
		MaxBatchSize: 3,
		MaxWait:      time.Minute,
		MinTokens:    100000, // very high so size triggers first
	}

	ce := NewCostEngine(nil, cfg)
	ce.TrackSession("s1", ProviderGemini)

	if res := ce.QueueBatch("s1", "p1", 100); res != nil {
		t.Fatal("should not flush after 1 entry")
	}
	if res := ce.QueueBatch("s1", "p2", 200); res != nil {
		t.Fatal("should not flush after 2 entries")
	}

	res := ce.QueueBatch("s1", "p3", 150)
	if res == nil {
		t.Fatal("expected flush at batch size 3")
	}
	if len(res.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(res.Entries))
	}
	if res.TotalTokens != 450 {
		t.Fatalf("expected 450 tokens, got %d", res.TotalTokens)
	}
}

func TestCostEngine_BatchQueue_FlushOnTokens(t *testing.T) {
	cfg := BatchConfig{
		MaxBatchSize: 100, // very high
		MinTokens:    500,
	}

	ce := NewCostEngine(nil, cfg)
	ce.TrackSession("s1", ProviderClaude)

	ce.QueueBatch("s1", "p1", 200)
	res := ce.QueueBatch("s1", "p2", 350) // total = 550 >= 500

	if res == nil {
		t.Fatal("expected flush at token threshold")
	}
	if res.TotalTokens != 550 {
		t.Fatalf("expected 550 tokens, got %d", res.TotalTokens)
	}
}

func TestCostEngine_BatchQueue_FlushOnMaxWait(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	callCount := 0
	clock := func() time.Time {
		now := start.Add(time.Duration(callCount) * 10 * time.Second)
		callCount++
		return now
	}

	cfg := BatchConfig{
		MaxBatchSize: 100,
		MaxWait:      15 * time.Second,
		MinTokens:    100000,
	}

	ce := NewCostEngine(nil, cfg)
	ce.now = clock

	ce.TrackSession("s1", ProviderGemini)

	// Queue at t=0s (TrackSession), then t=10s
	ce.QueueBatch("s1", "p1", 10) // queued at t=10s, clock now t=20s

	// Next call at t=20s; oldest entry is 10s old → not yet
	res := ce.QueueBatch("s1", "p2", 10) // queued at t=20s, check at t=30s
	// After QueueBatch, shouldFlush checks at t=30s. oldest=10s, elapsed=20s > 15s → flush
	if res == nil {
		t.Fatal("expected flush due to MaxWait")
	}
}

func TestCostEngine_ForceFlush(t *testing.T) {
	cfg := BatchConfig{MaxBatchSize: 100}

	ce := NewCostEngine(nil, cfg)
	ce.TrackSession("s1", ProviderClaude)

	ce.QueueBatch("s1", "p1", 100)

	res := ce.FlushBatch("s1")
	if res == nil {
		t.Fatal("expected forced flush")
	}
	if len(res.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(res.Entries))
	}

	// Flush again on empty queue
	if ce.FlushBatch("s1") != nil {
		t.Fatal("expected nil on empty queue")
	}
}

func TestCostEngine_FlushBatch_UntrackedSession(t *testing.T) {
	ce := DefaultCostEngine()
	if ce.FlushBatch("nope") != nil {
		t.Fatal("expected nil for untracked session")
	}
}

func TestCostEngine_QueueBatch_UntrackedSession(t *testing.T) {
	ce := DefaultCostEngine()
	if ce.QueueBatch("nope", "prompt", 100) != nil {
		t.Fatal("expected nil for untracked session")
	}
}

func TestCostEngine_BatchSavingsCalculation(t *testing.T) {
	entries := []BatchEntry{
		{TokenCount: 100},
		{TokenCount: 200},
		{TokenCount: 300},
	}
	totalTokens := 600

	result := computeBatchSavings(entries, totalTokens, ProviderGemini)

	if result.TotalTokens != 600 {
		t.Fatalf("expected 600 tokens, got %d", result.TotalTokens)
	}

	// Individual: 600 + 3*50 = 750 tokens
	// Batched: 600 + 50 = 650 tokens
	// Savings: 100 tokens worth
	if result.Savings <= 0 {
		t.Fatalf("expected positive savings, got %f", result.Savings)
	}
	if result.IndividualCost <= result.BatchedCost {
		t.Fatal("individual cost should exceed batched cost")
	}
	if result.SavingsPct <= 0 || result.SavingsPct > 100 {
		t.Fatalf("savings pct should be between 0-100, got %f", result.SavingsPct)
	}
}

func TestCostEngine_BatchSavings_SingleEntry(t *testing.T) {
	entries := []BatchEntry{{TokenCount: 100}}
	result := computeBatchSavings(entries, 100, ProviderClaude)

	// With 1 entry: individual = 100+50=150, batch = 100+50=150 → no savings
	if result.Savings != 0 {
		t.Fatalf("expected 0 savings for single entry, got %f", result.Savings)
	}
}

func TestCostEngine_RemoveSession(t *testing.T) {
	ce := DefaultCostEngine()
	ce.TrackSession("s1", ProviderClaude)
	ce.RecordCost("s1", 1.00)

	ce.RemoveSession("s1")

	if got := ce.TotalSpent("s1"); got != 0 {
		t.Fatalf("expected 0 after removal, got %f", got)
	}
	if ce.SessionCount() != 0 {
		t.Fatalf("expected 0 sessions, got %d", ce.SessionCount())
	}
}

func TestCostEngine_SessionCount(t *testing.T) {
	ce := DefaultCostEngine()
	if ce.SessionCount() != 0 {
		t.Fatal("expected 0 initially")
	}

	ce.TrackSession("s1", ProviderClaude)
	ce.TrackSession("s2", ProviderGemini)

	if ce.SessionCount() != 2 {
		t.Fatalf("expected 2, got %d", ce.SessionCount())
	}
}

func TestCostEngine_DefaultEngine(t *testing.T) {
	ce := DefaultCostEngine()
	if ce == nil {
		t.Fatal("DefaultCostEngine returned nil")
	}
	if len(ce.thresholds) == 0 {
		t.Fatal("expected default thresholds")
	}
	if ce.batchCfg.MaxBatchSize == 0 {
		t.Fatal("expected default batch config")
	}
}

func TestCostEngine_MultipleSessionsIndependent(t *testing.T) {
	ce := NewCostEngine(
		[]SpendThreshold{{MaxTotalSpend: 1.0, ToProvider: ProviderGemini}},
		BatchConfig{},
	)

	ce.TrackSession("s1", ProviderClaude)
	ce.TrackSession("s2", ProviderClaude)

	ce.RecordCost("s1", 0.50)
	ce.RecordCost("s2", 0.30)

	if ce.TotalSpent("s1") != 0.50 {
		t.Fatalf("s1 total wrong: %f", ce.TotalSpent("s1"))
	}
	if ce.TotalSpent("s2") != 0.30 {
		t.Fatalf("s2 total wrong: %f", ce.TotalSpent("s2"))
	}
}

func TestSwitchRecommendation_Fields(t *testing.T) {
	rec := SwitchRecommendation{
		SessionID:    "test",
		FromProvider: ProviderClaude,
		ToProvider:   ProviderGemini,
		Reason:       "spend rate threshold breached",
		CurrentRate:  5.5,
		TotalSpent:   2.0,
	}

	if rec.SessionID != "test" {
		t.Fatal("session ID mismatch")
	}
	if rec.FromProvider != ProviderClaude {
		t.Fatal("from provider mismatch")
	}
}

func TestRoundCost(t *testing.T) {
	tests := []struct {
		in, want float64
	}{
		{0.123456789, 0.12345679},
		{0.0, 0.0},
		{1.0, 1.0},
		{0.000000001, 0.0},
	}

	for _, tt := range tests {
		got := roundCost(tt.in)
		if got != tt.want {
			t.Errorf("roundCost(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
