package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func newTestAnalyticsStore(t *testing.T) (*Store, *AnalyticsStore) {
	t.Helper()
	s := newTestStore(t)
	a, err := NewAnalyticsStore(s)
	if err != nil {
		t.Fatalf("new analytics store: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	return s, a
}

// ---------- NewAnalyticsStore ----------

func TestNewAnalyticsStoreNilStore(t *testing.T) {
	_, err := NewAnalyticsStore(nil)
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

// ---------- Record ----------

func TestRecordAndQuery(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	ev := AnalyticsEvent{
		Timestamp: now,
		EventType: "session_complete",
		SessionID: "sess-001",
		Provider:  "claude",
		Cost:      0.05,
		Duration:  3 * time.Second,
		Metadata:  json.RawMessage(`{"tokens":1200}`),
	}

	if err := a.Record(ctx, ev); err != nil {
		t.Fatalf("record: %v", err)
	}

	results, err := a.Query(ctx, "session_complete", TimeRange{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	got := results[0]
	if got.EventType != "session_complete" {
		t.Errorf("EventType = %q, want session_complete", got.EventType)
	}
	if got.SessionID != "sess-001" {
		t.Errorf("SessionID = %q, want sess-001", got.SessionID)
	}
	if got.Provider != "claude" {
		t.Errorf("Provider = %q, want claude", got.Provider)
	}
	if got.Cost != 0.05 {
		t.Errorf("Cost = %f, want 0.05", got.Cost)
	}
	if got.Duration != 3*time.Second {
		t.Errorf("Duration = %v, want 3s", got.Duration)
	}
	if string(got.Metadata) != `{"tokens":1200}` {
		t.Errorf("Metadata = %s, want {\"tokens\":1200}", got.Metadata)
	}
}

func TestRecordAutoTimestamp(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	ctx := context.Background()

	before := time.Now().UTC().Add(-time.Second)

	ev := AnalyticsEvent{
		EventType: "step",
		SessionID: "sess-auto",
		Provider:  "gemini",
		Cost:      0.01,
		Duration:  100 * time.Millisecond,
	}
	if err := a.Record(ctx, ev); err != nil {
		t.Fatalf("record: %v", err)
	}

	results, err := a.Query(ctx, "step", TimeRange{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].Timestamp.Before(before) {
		t.Errorf("auto timestamp %v is before test start %v", results[0].Timestamp, before)
	}
}

// ---------- Query with time range ----------

func TestQueryTimeRange(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	ctx := context.Background()

	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	// Insert events across several days.
	for i := 0; i < 5; i++ {
		ev := AnalyticsEvent{
			Timestamp: base.Add(time.Duration(i) * 24 * time.Hour),
			EventType: "step",
			SessionID: "sess-tr",
			Provider:  "claude",
			Cost:      float64(i) * 0.01,
			Duration:  time.Second,
		}
		if err := a.Record(ctx, ev); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	// Query a sub-range: days 1-3 (inclusive).
	tr := TimeRange{
		Start: base.Add(1 * 24 * time.Hour),
		End:   base.Add(3 * 24 * time.Hour),
	}
	results, err := a.Query(ctx, "step", tr)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results in range, got %d", len(results))
	}
}

func TestQueryAllTypes(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	ctx := context.Background()

	for _, et := range []string{"step", "error", "cost"} {
		ev := AnalyticsEvent{
			Timestamp: time.Now().UTC(),
			EventType: et,
			SessionID: "sess-all",
			Provider:  "claude",
			Cost:      0.01,
			Duration:  time.Second,
		}
		if err := a.Record(ctx, ev); err != nil {
			t.Fatalf("record %s: %v", et, err)
		}
	}

	// Empty eventType returns all.
	results, err := a.Query(ctx, "", TimeRange{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

// ---------- Aggregate ----------

func TestAggregateByProvider(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	ctx := context.Background()

	events := []AnalyticsEvent{
		{Timestamp: time.Now().UTC(), EventType: "step", SessionID: "s1", Provider: "claude", Cost: 0.10, Duration: time.Second},
		{Timestamp: time.Now().UTC(), EventType: "step", SessionID: "s2", Provider: "claude", Cost: 0.20, Duration: 2 * time.Second},
		{Timestamp: time.Now().UTC(), EventType: "step", SessionID: "s3", Provider: "gemini", Cost: 0.05, Duration: 500 * time.Millisecond},
	}
	for _, ev := range events {
		if err := a.Record(ctx, ev); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	results, err := a.Aggregate(ctx, "step", TimeRange{}, "provider")
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(results))
	}

	// Ordered by total cost DESC, so claude first.
	if results[0].GroupKey != "claude" {
		t.Errorf("first group = %q, want claude", results[0].GroupKey)
	}
	if results[0].Count != 2 {
		t.Errorf("claude count = %d, want 2", results[0].Count)
	}
	// Total cost for claude: 0.10 + 0.20 = 0.30
	if results[0].TotalCost < 0.29 || results[0].TotalCost > 0.31 {
		t.Errorf("claude total cost = %f, want ~0.30", results[0].TotalCost)
	}

	if results[1].GroupKey != "gemini" {
		t.Errorf("second group = %q, want gemini", results[1].GroupKey)
	}
	if results[1].Count != 1 {
		t.Errorf("gemini count = %d, want 1", results[1].Count)
	}
}

func TestAggregateBySessionID(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		ev := AnalyticsEvent{
			Timestamp: time.Now().UTC(),
			EventType: "step",
			SessionID: "sess-agg",
			Provider:  "claude",
			Cost:      0.10,
			Duration:  time.Second,
		}
		if err := a.Record(ctx, ev); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	results, err := a.Aggregate(ctx, "", TimeRange{}, "session_id")
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 group, got %d", len(results))
	}
	if results[0].Count != 3 {
		t.Errorf("count = %d, want 3", results[0].Count)
	}
}

func TestAggregateInvalidGroupBy(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	ctx := context.Background()

	_, err := a.Aggregate(ctx, "", TimeRange{}, "invalid_column")
	if err == nil {
		t.Fatal("expected error for invalid groupBy")
	}
}

// ---------- TopSessions ----------

func TestTopSessions(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	ctx := context.Background()

	// Session A: 3 events, total cost 0.30
	for i := 0; i < 3; i++ {
		ev := AnalyticsEvent{
			Timestamp: time.Now().UTC(),
			EventType: "step",
			SessionID: "sess-a",
			Provider:  "claude",
			Cost:      0.10,
			Duration:  time.Second,
		}
		if err := a.Record(ctx, ev); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	// Session B: 1 event, total cost 0.50
	ev := AnalyticsEvent{
		Timestamp: time.Now().UTC(),
		EventType: "step",
		SessionID: "sess-b",
		Provider:  "gemini",
		Cost:      0.50,
		Duration:  5 * time.Second,
	}
	if err := a.Record(ctx, ev); err != nil {
		t.Fatalf("record: %v", err)
	}

	// Session C: 2 events, total cost 0.02
	for i := 0; i < 2; i++ {
		ev := AnalyticsEvent{
			Timestamp: time.Now().UTC(),
			EventType: "step",
			SessionID: "sess-c",
			Provider:  "codex",
			Cost:      0.01,
			Duration:  100 * time.Millisecond,
		}
		if err := a.Record(ctx, ev); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	results, err := a.TopSessions(ctx, TimeRange{}, 2)
	if err != nil {
		t.Fatalf("top sessions: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Ordered by cost DESC: sess-b (0.50), sess-a (0.30).
	if results[0].SessionID != "sess-b" {
		t.Errorf("top session = %q, want sess-b", results[0].SessionID)
	}
	if results[0].TotalCost < 0.49 || results[0].TotalCost > 0.51 {
		t.Errorf("top cost = %f, want ~0.50", results[0].TotalCost)
	}
	if results[1].SessionID != "sess-a" {
		t.Errorf("second session = %q, want sess-a", results[1].SessionID)
	}
}

func TestTopSessionsDefaultLimit(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	ctx := context.Background()

	ev := AnalyticsEvent{
		Timestamp: time.Now().UTC(),
		EventType: "step",
		SessionID: "sess-default",
		Provider:  "claude",
		Cost:      0.01,
		Duration:  time.Second,
	}
	if err := a.Record(ctx, ev); err != nil {
		t.Fatalf("record: %v", err)
	}

	// limit=0 should default to 10, still return our one session.
	results, err := a.TopSessions(ctx, TimeRange{}, 0)
	if err != nil {
		t.Fatalf("top sessions: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result with default limit, got %d", len(results))
	}
}

func TestTopSessionsTimeRange(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	ctx := context.Background()

	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	evOld := AnalyticsEvent{
		Timestamp: old,
		EventType: "step",
		SessionID: "sess-old",
		Provider:  "claude",
		Cost:      1.00,
		Duration:  time.Second,
	}
	evRecent := AnalyticsEvent{
		Timestamp: recent,
		EventType: "step",
		SessionID: "sess-recent",
		Provider:  "claude",
		Cost:      0.01,
		Duration:  time.Second,
	}
	if err := a.Record(ctx, evOld); err != nil {
		t.Fatalf("record old: %v", err)
	}
	if err := a.Record(ctx, evRecent); err != nil {
		t.Fatalf("record recent: %v", err)
	}

	tr := TimeRange{Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	results, err := a.TopSessions(ctx, tr, 10)
	if err != nil {
		t.Fatalf("top sessions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result in range, got %d", len(results))
	}
	if results[0].SessionID != "sess-recent" {
		t.Errorf("session = %q, want sess-recent", results[0].SessionID)
	}
}

// ---------- Metadata handling ----------

func TestRecordNilMetadata(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	ctx := context.Background()

	ev := AnalyticsEvent{
		Timestamp: time.Now().UTC(),
		EventType: "step",
		SessionID: "sess-nil-meta",
		Provider:  "claude",
		Cost:      0.01,
		Duration:  time.Second,
	}
	if err := a.Record(ctx, ev); err != nil {
		t.Fatalf("record: %v", err)
	}

	results, err := a.Query(ctx, "step", TimeRange{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	// Metadata should be nil when stored as "null".
	if results[0].Metadata != nil {
		t.Errorf("Metadata = %s, want nil", results[0].Metadata)
	}
}

// ---------- Close ----------

func TestAnalyticsStoreClose(t *testing.T) {
	_, a := newTestAnalyticsStore(t)
	// Close should not panic or error.
	if err := a.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
