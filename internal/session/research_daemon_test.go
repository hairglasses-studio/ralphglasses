package session

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockGateway implements ResearchGateway for testing.
type mockGateway struct {
	mu              sync.Mutex
	entries         []*ResearchEntry
	dequeued        []string
	completed       []string
	abandoned       []string
	written         []string
	commits         int
	dedupConfidence float64
	dedupRecommend  string
	expireCount     int
	failWrite       bool
}

func newMockGateway(entries ...*ResearchEntry) *mockGateway {
	return &mockGateway{
		entries:        entries,
		dedupRecommend: "proceed",
	}
}

func (m *mockGateway) ExpireStale(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := m.expireCount
	m.expireCount = 0
	return n, nil
}

func (m *mockGateway) DequeueNext(_ context.Context, agent string, claimTTL int) (*ResearchEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.entries) == 0 {
		return nil, nil
	}
	e := m.entries[0]
	m.entries = m.entries[1:]
	m.dequeued = append(m.dequeued, e.Topic)
	return e, nil
}

func (m *mockGateway) DedupCheck(_ context.Context, topic, domain string) (float64, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dedupConfidence, m.dedupRecommend, nil
}

func (m *mockGateway) Complete(_ context.Context, topic, domain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completed = append(m.completed, topic)
	return nil
}

func (m *mockGateway) Abandon(_ context.Context, topic, domain, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.abandoned = append(m.abandoned, topic+":"+reason)
	return nil
}

func (m *mockGateway) WriteResearch(_ context.Context, domain, title, content string, urls []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWrite {
		return fmt.Errorf("write failed")
	}
	m.written = append(m.written, title)
	return nil
}

func (m *mockGateway) CommitAndPush(_ context.Context, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commits++
	return nil
}

func TestResearchDaemonTickInterval(t *testing.T) {
	gw := newMockGateway(&ResearchEntry{
		Topic: "test-topic", Domain: "mcp", Source: "freshness",
		PriorityScore: 0.5, ModelTier: "sonnet", BudgetUSD: 3.0,
	})
	gw.dedupConfidence = 0.5 // partial → novelty 2 → complexity 2

	cfg := DefaultResearchDaemonConfig()
	cfg.TickInterval = 3
	rd := NewResearchDaemon(gw, cfg)

	ctx := context.Background()

	// Ticks 1, 2 should not trigger processing.
	rd.Tick(ctx)
	rd.Tick(ctx)
	gw.mu.Lock()
	dequeued := len(gw.dequeued)
	gw.mu.Unlock()
	if dequeued != 0 {
		t.Errorf("expected 0 dequeues after 2 ticks, got %d", dequeued)
	}

	// Tick 3 should trigger.
	rd.Tick(ctx)
	gw.mu.Lock()
	dequeued = len(gw.dequeued)
	gw.mu.Unlock()
	if dequeued != 1 {
		t.Errorf("expected 1 dequeue after 3 ticks, got %d", dequeued)
	}
}

func TestResearchDaemonEmptyQueue(t *testing.T) {
	gw := newMockGateway() // no entries
	cfg := DefaultResearchDaemonConfig()
	cfg.TickInterval = 1
	rd := NewResearchDaemon(gw, cfg)

	ctx := context.Background()
	rd.Tick(ctx) // should not panic

	stats := rd.Stats()
	if stats.TopicsProcessed != 0 {
		t.Errorf("expected 0 processed, got %d", stats.TopicsProcessed)
	}
}

func TestResearchDaemonDedupSkip(t *testing.T) {
	gw := newMockGateway(&ResearchEntry{
		Topic: "well-known", Domain: "mcp", Source: "manual",
		PriorityScore: 0.5, ModelTier: "sonnet", BudgetUSD: 3.0,
	})
	gw.dedupConfidence = 0.8
	gw.dedupRecommend = "exists"

	cfg := DefaultResearchDaemonConfig()
	cfg.TickInterval = 1
	rd := NewResearchDaemon(gw, cfg)

	ctx := context.Background()
	rd.Tick(ctx)

	gw.mu.Lock()
	defer gw.mu.Unlock()
	if len(gw.completed) != 1 || gw.completed[0] != "well-known" {
		t.Errorf("expected dedup-skipped topic to be completed, got %v", gw.completed)
	}
	if len(gw.written) != 0 {
		t.Error("expected no writes for dedup-skipped topic")
	}
}

func TestResearchDaemonComplexityGate(t *testing.T) {
	gw := newMockGateway(&ResearchEntry{
		Topic: "complex-topic", Domain: "agents", Source: "roadmap",
		PriorityScore: 0.9, ModelTier: "opus", BudgetUSD: 10.0,
	})
	gw.dedupConfidence = 0.0 // very new → novelty 4

	cfg := DefaultResearchDaemonConfig()
	cfg.TickInterval = 1
	cfg.MaxComplexity = 2 // low threshold
	rd := NewResearchDaemon(gw, cfg)

	ctx := context.Background()
	rd.Tick(ctx)

	gw.mu.Lock()
	defer gw.mu.Unlock()
	if len(gw.abandoned) != 1 {
		t.Fatalf("expected 1 abandoned, got %d", len(gw.abandoned))
	}
	if gw.abandoned[0] != "complex-topic:complexity_exceeds_max" {
		t.Errorf("unexpected abandon reason: %s", gw.abandoned[0])
	}
}

func TestResearchDaemonSuccessfulProcessing(t *testing.T) {
	gw := newMockGateway(&ResearchEntry{
		Topic: "go-mcp-patterns", Domain: "mcp", Source: "manual",
		PriorityScore: 0.6, ModelTier: "sonnet", BudgetUSD: 3.0,
	})
	gw.dedupConfidence = 0.3

	cfg := DefaultResearchDaemonConfig()
	cfg.TickInterval = 1
	rd := NewResearchDaemon(gw, cfg)

	ctx := context.Background()
	rd.Tick(ctx)

	gw.mu.Lock()
	defer gw.mu.Unlock()

	if len(gw.written) != 1 {
		t.Fatalf("expected 1 write, got %d", len(gw.written))
	}
	if gw.written[0] != "go-mcp-patterns" {
		t.Errorf("unexpected write title: %s", gw.written[0])
	}
	if len(gw.completed) != 1 {
		t.Errorf("expected 1 completion, got %d", len(gw.completed))
	}

	stats := rd.Stats()
	if stats.TopicsProcessed != 1 {
		t.Errorf("expected 1 processed, got %d", stats.TopicsProcessed)
	}
	if stats.DailySpentUSD == 0 {
		t.Error("expected non-zero daily spend")
	}
}

func TestResearchDaemonWriteFailure(t *testing.T) {
	gw := newMockGateway(&ResearchEntry{
		Topic: "failing-topic", Domain: "mcp", Source: "freshness",
		PriorityScore: 0.5, ModelTier: "sonnet", BudgetUSD: 3.0,
	})
	gw.dedupConfidence = 0.5 // novelty 2, scope 1 (freshness) → complexity 2
	gw.failWrite = true

	cfg := DefaultResearchDaemonConfig()
	cfg.TickInterval = 1
	rd := NewResearchDaemon(gw, cfg)

	ctx := context.Background()
	rd.Tick(ctx)

	gw.mu.Lock()
	defer gw.mu.Unlock()
	if len(gw.abandoned) != 1 {
		t.Fatalf("expected 1 abandoned on write failure, got %d", len(gw.abandoned))
	}

	stats := rd.Stats()
	if stats.TopicsFailed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.TopicsFailed)
	}
}

func TestResearchDaemonDailyBudgetExhausted(t *testing.T) {
	gw := newMockGateway(&ResearchEntry{
		Topic: "budget-topic", Domain: "mcp", Source: "manual",
		PriorityScore: 0.5, ModelTier: "sonnet", BudgetUSD: 3.0,
	})

	cfg := DefaultResearchDaemonConfig()
	cfg.TickInterval = 1
	cfg.BudgetDailyUSD = 0.01 // very low
	rd := NewResearchDaemon(gw, cfg)
	// Pre-exhaust the budget.
	rd.mu.Lock()
	rd.dailySpentUSD = 0.02
	rd.mu.Unlock()

	ctx := context.Background()
	rd.Tick(ctx)

	gw.mu.Lock()
	defer gw.mu.Unlock()
	if len(gw.dequeued) != 0 {
		t.Errorf("expected no dequeues when budget exhausted, got %d", len(gw.dequeued))
	}
}

func TestResearchDaemonDisabled(t *testing.T) {
	gw := newMockGateway(&ResearchEntry{
		Topic: "should-not-run", Domain: "mcp", Source: "manual",
	})

	cfg := DefaultResearchDaemonConfig()
	cfg.Enabled = false
	cfg.TickInterval = 1
	rd := NewResearchDaemon(gw, cfg)

	ctx := context.Background()
	rd.Tick(ctx)

	gw.mu.Lock()
	defer gw.mu.Unlock()
	if len(gw.dequeued) != 0 {
		t.Error("expected no processing when disabled")
	}
}

func TestResearchDaemonBatchCommit(t *testing.T) {
	entries := make([]*ResearchEntry, 4)
	for i := range entries {
		entries[i] = &ResearchEntry{
			Topic: fmt.Sprintf("topic-%d", i), Domain: "mcp", Source: "freshness",
			PriorityScore: 0.5, ModelTier: "sonnet", BudgetUSD: 3.0,
		}
	}
	gw := newMockGateway(entries...)
	gw.dedupConfidence = 0.5 // novelty 2, scope 1 (freshness) → complexity 2

	cfg := DefaultResearchDaemonConfig()
	cfg.TickInterval = 1
	cfg.MaxTopicsPerRun = 4
	rd := NewResearchDaemon(gw, cfg)

	ctx := context.Background()
	rd.Tick(ctx)

	gw.mu.Lock()
	defer gw.mu.Unlock()
	// Should have triggered a commit after 3+ writes.
	if gw.commits != 1 {
		t.Errorf("expected 1 batch commit, got %d", gw.commits)
	}
}

func TestClassifyResearchComplexity(t *testing.T) {
	tests := []struct {
		name       string
		entry      *ResearchEntry
		confidence float64
		want       int
	}{
		{
			name:       "freshness refresh, well-known",
			entry:      &ResearchEntry{Source: "freshness"},
			confidence: 0.8,
			want:       1, // max(scope=1, novelty=1, impact=1) = 1
		},
		{
			name:       "gap research, partially known",
			entry:      &ResearchEntry{Source: "gap"},
			confidence: 0.5,
			want:       3, // max(scope=2, novelty=2, impact=3) = 3
		},
		{
			name:       "roadmap item, brand new",
			entry:      &ResearchEntry{Source: "roadmap"},
			confidence: 0.1,
			want:       4, // max(scope=3, novelty=4, impact=3) = 4
		},
		{
			name:       "manual research, no existing coverage",
			entry:      &ResearchEntry{Source: "manual"},
			confidence: 0.0,
			want:       4, // max(scope=2, novelty=4, impact=2) = 4
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyResearchComplexity(tt.entry, tt.confidence)
			if got != tt.want {
				t.Errorf("classifyResearchComplexity() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBudgetForComplexity(t *testing.T) {
	tests := []struct {
		complexity int
		want       float64
	}{
		{1, 0.10},
		{2, 0.50},
		{3, 2.00},
		{4, 5.00},
		{0, 0.50},
	}
	for _, tt := range tests {
		if got := budgetForComplexity(tt.complexity); got != tt.want {
			t.Errorf("budgetForComplexity(%d) = %v, want %v", tt.complexity, got, tt.want)
		}
	}
}

func TestBuildResearchPrompt(t *testing.T) {
	entry := &ResearchEntry{Topic: "MCP caching", Domain: "mcp", Source: "manual"}

	prompt := buildResearchPrompt(entry, "new", 2)
	if !researchContainsAll(prompt, "MCP caching", "mcp", "new", "2/4") {
		t.Errorf("prompt missing expected content: %s", prompt[:100])
	}

	prompt = buildResearchPrompt(entry, "expand", 3)
	if !researchContainsAll(prompt, "Build on and expand", "3/4") {
		t.Errorf("expand prompt missing expected content: %s", prompt[:100])
	}
}

func TestNextMidnight(t *testing.T) {
	mid := nextMidnight()
	if mid.Before(time.Now()) {
		t.Error("nextMidnight returned a time in the past")
	}
	if mid.Hour() != 0 || mid.Minute() != 0 || mid.Second() != 0 {
		t.Errorf("nextMidnight should be midnight, got %s", mid.Format(time.RFC3339))
	}
}

func TestResearchDaemonStats(t *testing.T) {
	gw := newMockGateway()
	cfg := DefaultResearchDaemonConfig()
	rd := NewResearchDaemon(gw, cfg)

	stats := rd.Stats()
	if !stats.Enabled {
		t.Error("expected enabled=true")
	}
	if stats.DailyBudgetUSD != 25.0 {
		t.Errorf("expected daily budget 25.0, got %v", stats.DailyBudgetUSD)
	}
}

func researchContainsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
