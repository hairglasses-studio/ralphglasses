package session

import (
	"context"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/fleet/pool"
)

func TestFleetPoolRefresh(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	// Inject sessions directly into the manager.
	m.mu.Lock()
	m.sessions["s1"] = &Session{
		ID:         "s1",
		Provider:   ProviderClaude,
		Status:     StatusRunning,
		SpentUSD:   1.50,
		BudgetUSD:  5.00,
		RepoPath:   "/tmp/repo1",
		LaunchedAt: time.Now(),
	}
	m.sessions["s2"] = &Session{
		ID:         "s2",
		Provider:   ProviderGemini,
		Status:     StatusCompleted,
		SpentUSD:   0.30,
		BudgetUSD:  2.00,
		RepoPath:   "/tmp/repo2",
		LaunchedAt: time.Now().Add(-5 * time.Minute),
	}
	m.mu.Unlock()

	// Refresh fleet state.
	m.RefreshFleetState()

	sum := m.FleetPool.GetSummary()

	if sum.TotalSpentUSD != 1.80 {
		t.Errorf("TotalSpentUSD = %f, want 1.80", sum.TotalSpentUSD)
	}
	if sum.TotalBudgetUSD != 7.00 {
		t.Errorf("TotalBudgetUSD = %f, want 7.00", sum.TotalBudgetUSD)
	}
	if sum.ActiveSessions != 1 {
		t.Errorf("ActiveSessions = %d, want 1", sum.ActiveSessions)
	}
	if sum.ProviderCounts["claude"] != 1 {
		t.Errorf("ProviderCounts[claude] = %d, want 1", sum.ProviderCounts["claude"])
	}
	if sum.ProviderCounts["gemini"] != 1 {
		t.Errorf("ProviderCounts[gemini] = %d, want 1", sum.ProviderCounts["gemini"])
	}
}

func TestCanSpendGate(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	// Set a tight budget cap.
	m.FleetPool = pool.NewState(2.00)

	// Inject a session that already spent most of the budget.
	m.mu.Lock()
	m.sessions["s1"] = &Session{
		ID:         "s1",
		Provider:   ProviderClaude,
		Status:     StatusRunning,
		SpentUSD:   1.80,
		BudgetUSD:  5.00,
		RepoPath:   "/tmp/repo",
		LaunchedAt: time.Now(),
	}
	m.mu.Unlock()

	// Refresh so pool knows about existing spend.
	m.RefreshFleetState()

	// Use a fake launcher so we don't exec anything.
	m.launchSession = func(_ context.Context, opts LaunchOptions) (*Session, error) {
		return &Session{
			ID:       "new",
			Provider: opts.Provider,
			Status:   StatusRunning,
			RepoPath: opts.RepoPath,
		}, nil
	}

	// Launch with default estimated cost ($0.50) should fail: 1.80 + 0.50 > 2.00.
	_, err := m.Launch(context.Background(), LaunchOptions{
		Provider: ProviderClaude,
		RepoPath: "/tmp/repo",
		Prompt:   "test",
	})
	if err == nil {
		t.Fatal("expected fleet budget cap error, got nil")
	}
	if got := err.Error(); got == "" {
		t.Fatal("expected non-empty error message")
	}

	// Raise the cap so launch succeeds.
	m.FleetPool.SetBudgetCap(10.00)
	sess, err := m.Launch(context.Background(), LaunchOptions{
		Provider: ProviderClaude,
		RepoPath: "/tmp/repo",
		Prompt:   "test",
	})
	if err != nil {
		t.Fatalf("expected launch to succeed after raising cap, got: %v", err)
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
}

func TestCascadeEnabledByDefault(t *testing.T) {
	tests := []struct {
		name    string
		factory func() *Manager
	}{
		{"NewManager", func() *Manager { return NewManager() }},
		{"NewManagerWithBus", func() *Manager { return NewManagerWithBus(nil) }},
		{"NewManagerWithStore", func() *Manager { return NewManagerWithStore(nil, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.factory()
			if !m.HasCascadeRouter() {
				t.Errorf("%s: cascade router not enabled by default", tt.name)
			}
		})
	}
}

func TestFleetPoolInitializedByDefault(t *testing.T) {
	tests := []struct {
		name    string
		factory func() *Manager
	}{
		{"NewManager", func() *Manager { return NewManager() }},
		{"NewManagerWithBus", func() *Manager { return NewManagerWithBus(nil) }},
		{"NewManagerWithStore", func() *Manager { return NewManagerWithStore(nil, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.factory()
			if m.FleetPool == nil {
				t.Errorf("%s: FleetPool not initialized by default", tt.name)
			}
		})
	}
}

func TestRefreshFleetStateNilPool(t *testing.T) {
	m := NewManager()
	m.FleetPool = nil
	// Should not panic.
	m.RefreshFleetState()
}
