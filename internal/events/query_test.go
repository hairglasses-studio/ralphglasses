package events

import (
	"testing"
	"time"
)

// helper to create a bus pre-loaded with test events.
func seedBus() *Bus {
	b := NewBus(100)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	events := []Event{
		{Type: SessionStarted, Timestamp: base, SessionID: "s1", RepoName: "repoA", Provider: "claude"},
		{Type: CostUpdate, Timestamp: base.Add(1 * time.Minute), SessionID: "s1", RepoName: "repoA", Provider: "claude"},
		{Type: SessionStarted, Timestamp: base.Add(2 * time.Minute), SessionID: "s2", RepoName: "repoB", Provider: "gemini"},
		{Type: LoopStarted, Timestamp: base.Add(3 * time.Minute), SessionID: "s1", RepoName: "repoA", Provider: "claude"},
		{Type: CostUpdate, Timestamp: base.Add(4 * time.Minute), SessionID: "s2", RepoName: "repoB", Provider: "gemini"},
		{Type: SessionEnded, Timestamp: base.Add(5 * time.Minute), SessionID: "s1", RepoName: "repoA", Provider: "claude"},
		{Type: LoopStopped, Timestamp: base.Add(6 * time.Minute), SessionID: "s2", RepoName: "repoB", Provider: "gemini"},
		{Type: SessionEnded, Timestamp: base.Add(7 * time.Minute), SessionID: "s2", RepoName: "repoB", Provider: "gemini"},
	}
	for _, e := range events {
		b.Publish(e)
	}
	return b
}

func TestQuery_AllEvents(t *testing.T) {
	b := seedBus()
	result := b.Query(EventQuery{})

	if result.TotalCount != 8 {
		t.Errorf("expected TotalCount=8, got %d", result.TotalCount)
	}
	if len(result.Events) != 8 {
		t.Errorf("expected 8 events, got %d", len(result.Events))
	}
	if result.HasMore {
		t.Error("expected HasMore=false")
	}
}

func TestQuery_ByType(t *testing.T) {
	b := seedBus()
	result := b.Query(EventQuery{Types: []EventType{CostUpdate}})

	if result.TotalCount != 2 {
		t.Errorf("expected TotalCount=2, got %d", result.TotalCount)
	}
	for _, e := range result.Events {
		if e.Type != CostUpdate {
			t.Errorf("expected type CostUpdate, got %s", e.Type)
		}
	}
}

func TestQuery_ByMultipleTypes(t *testing.T) {
	b := seedBus()
	result := b.Query(EventQuery{Types: []EventType{SessionStarted, SessionEnded}})

	if result.TotalCount != 4 {
		t.Errorf("expected TotalCount=4, got %d", result.TotalCount)
	}
	for _, e := range result.Events {
		if e.Type != SessionStarted && e.Type != SessionEnded {
			t.Errorf("unexpected type %s", e.Type)
		}
	}
}

func TestQuery_BySince(t *testing.T) {
	b := seedBus()
	since := time.Date(2026, 1, 1, 0, 4, 0, 0, time.UTC)
	result := b.Query(EventQuery{Since: &since})

	// Events at minutes 4, 5, 6, 7 should match
	if result.TotalCount != 4 {
		t.Errorf("expected TotalCount=4, got %d", result.TotalCount)
	}
	for _, e := range result.Events {
		if e.Timestamp.Before(since) {
			t.Errorf("event timestamp %v is before since %v", e.Timestamp, since)
		}
	}
}

func TestQuery_BySessionID(t *testing.T) {
	b := seedBus()
	result := b.Query(EventQuery{SessionID: "s2"})

	if result.TotalCount != 4 {
		t.Errorf("expected TotalCount=4, got %d", result.TotalCount)
	}
	for _, e := range result.Events {
		if e.SessionID != "s2" {
			t.Errorf("expected session_id=s2, got %s", e.SessionID)
		}
	}
}

func TestQuery_Combined(t *testing.T) {
	b := seedBus()
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := b.Query(EventQuery{
		Types:     []EventType{CostUpdate, LoopStarted},
		SessionID: "s1",
		Since:     &since,
	})

	// s1 has: CostUpdate at min 1, LoopStarted at min 3
	if result.TotalCount != 2 {
		t.Errorf("expected TotalCount=2, got %d", result.TotalCount)
	}
}

func TestQuery_Pagination(t *testing.T) {
	b := seedBus()

	// Page 1: offset=0, limit=3
	r1 := b.Query(EventQuery{Limit: 3, Offset: 0})
	if len(r1.Events) != 3 {
		t.Errorf("page 1: expected 3 events, got %d", len(r1.Events))
	}
	if r1.TotalCount != 8 {
		t.Errorf("page 1: expected TotalCount=8, got %d", r1.TotalCount)
	}
	if !r1.HasMore {
		t.Error("page 1: expected HasMore=true")
	}

	// Page 2: offset=3, limit=3
	r2 := b.Query(EventQuery{Limit: 3, Offset: 3})
	if len(r2.Events) != 3 {
		t.Errorf("page 2: expected 3 events, got %d", len(r2.Events))
	}
	if !r2.HasMore {
		t.Error("page 2: expected HasMore=true")
	}

	// Page 3: offset=6, limit=3
	r3 := b.Query(EventQuery{Limit: 3, Offset: 6})
	if len(r3.Events) != 2 {
		t.Errorf("page 3: expected 2 events, got %d", len(r3.Events))
	}
	if r3.HasMore {
		t.Error("page 3: expected HasMore=false")
	}

	// Offset beyond total
	r4 := b.Query(EventQuery{Limit: 3, Offset: 100})
	if len(r4.Events) != 0 {
		t.Errorf("beyond: expected 0 events, got %d", len(r4.Events))
	}
}

func TestAggregate_ByType(t *testing.T) {
	b := seedBus()
	result := b.Aggregate(EventAggregation{GroupBy: "type"})

	if result.Total != 8 {
		t.Errorf("expected Total=8, got %d", result.Total)
	}
	if result.Groups[string(SessionStarted)] != 2 {
		t.Errorf("expected 2 SessionStarted, got %d", result.Groups[string(SessionStarted)])
	}
	if result.Groups[string(CostUpdate)] != 2 {
		t.Errorf("expected 2 CostUpdate, got %d", result.Groups[string(CostUpdate)])
	}
}

func TestAggregate_ByProvider(t *testing.T) {
	b := seedBus()
	result := b.Aggregate(EventAggregation{GroupBy: "provider"})

	if result.Total != 8 {
		t.Errorf("expected Total=8, got %d", result.Total)
	}
	if result.Groups["claude"] != 4 {
		t.Errorf("expected 4 claude events, got %d", result.Groups["claude"])
	}
	if result.Groups["gemini"] != 4 {
		t.Errorf("expected 4 gemini events, got %d", result.Groups["gemini"])
	}
}
