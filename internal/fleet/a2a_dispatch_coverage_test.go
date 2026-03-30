package fleet

import (
	"testing"
)

func TestA2ADispatcher_NewWithNilConfig(t *testing.T) {
	d := NewA2ADispatcher(nil)
	if d == nil {
		t.Fatal("NewA2ADispatcher(nil) returned nil")
	}
	if d.client == nil {
		t.Error("default client should not be nil")
	}
}

func TestA2ADispatcher_NewWithConfig(t *testing.T) {
	cfg := &A2ADispatcherConfig{
		Strategy: StrategyBestFit,
	}
	d := NewA2ADispatcher(cfg)
	if d.strategy != StrategyBestFit {
		t.Errorf("strategy = %v, want %v", d.strategy, StrategyBestFit)
	}
}

func TestA2ADispatcher_AddAndAgents(t *testing.T) {
	d := NewA2ADispatcher(nil)
	d.AddAgent("http://agent1.example.com")
	d.AddAgent("http://agent2.example.com")
	agents := d.Agents()
	if len(agents) != 2 {
		t.Errorf("Agents() len = %d, want 2", len(agents))
	}
}

func TestA2ADispatcher_AddAgent_Dedup(t *testing.T) {
	d := NewA2ADispatcher(nil)
	d.AddAgent("http://agent1.example.com")
	d.AddAgent("http://agent1.example.com") // duplicate
	agents := d.Agents()
	if len(agents) != 1 {
		t.Errorf("Agents() after dedup = %d, want 1", len(agents))
	}
}

func TestA2ADispatcher_AddAgent_StripsTrailingSlash(t *testing.T) {
	d := NewA2ADispatcher(nil)
	d.AddAgent("http://agent1.example.com/")
	d.AddAgent("http://agent1.example.com") // should be treated as same
	agents := d.Agents()
	if len(agents) != 1 {
		t.Errorf("Agents() after slash-strip dedup = %d, want 1", len(agents))
	}
}

func TestA2ADispatcher_RemoveAgent(t *testing.T) {
	d := NewA2ADispatcher(nil)
	d.AddAgent("http://agent1.example.com")
	d.AddAgent("http://agent2.example.com")
	d.RemoveAgent("http://agent1.example.com")
	agents := d.Agents()
	if len(agents) != 1 {
		t.Errorf("Agents() after remove = %d, want 1", len(agents))
	}
	if agents[0] != "http://agent2.example.com" {
		t.Errorf("remaining agent = %q, want http://agent2.example.com", agents[0])
	}
}

func TestA2ADispatcher_RemoveAgent_NonExistent(t *testing.T) {
	d := NewA2ADispatcher(nil)
	d.AddAgent("http://agent1.example.com")
	// Remove non-existent agent should not panic.
	d.RemoveAgent("http://nonexistent.example.com")
	agents := d.Agents()
	if len(agents) != 1 {
		t.Errorf("Agents() after non-existent remove = %d, want 1", len(agents))
	}
}

func TestA2ADispatcher_InvalidateAllCaches(t *testing.T) {
	d := NewA2ADispatcher(nil)
	d.AddAgent("http://agent1.example.com")
	// Just verify it doesn't panic.
	d.InvalidateAllCaches()
}

func TestA2ADispatcher_InvalidateCache(t *testing.T) {
	d := NewA2ADispatcher(nil)
	d.AddAgent("http://agent1.example.com")
	// Invalidate non-existent cache should not panic.
	d.InvalidateCache("http://agent1.example.com")
}

func TestA2ADispatcher_GetCachedCard_Empty(t *testing.T) {
	d := NewA2ADispatcher(nil)
	// No cached card, should return nil.
	card := d.GetCachedCard("http://agent1.example.com")
	if card != nil {
		t.Error("GetCachedCard on empty cache should return nil")
	}
}
