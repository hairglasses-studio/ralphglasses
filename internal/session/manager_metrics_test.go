package session

import (
	"sync"
	"testing"
	"time"
)

func TestManagerMetrics_BasicCounters(t *testing.T) {
	m := NewManagerMetrics()

	m.RecordLaunch(ProviderClaude)
	m.RecordLaunch(ProviderGemini)
	m.RecordLaunch(ProviderClaude)

	s := m.Snapshot()
	if s.ActiveSessions != 3 {
		t.Errorf("active sessions = %d, want 3", s.ActiveSessions)
	}
	if s.TotalLaunched != 3 {
		t.Errorf("total launched = %d, want 3", s.TotalLaunched)
	}
	if s.PeakSessions != 3 {
		t.Errorf("peak sessions = %d, want 3", s.PeakSessions)
	}

	m.RecordComplete(ProviderClaude)
	s = m.Snapshot()
	if s.ActiveSessions != 2 {
		t.Errorf("active sessions after complete = %d, want 2", s.ActiveSessions)
	}
	if s.TotalCompleted != 1 {
		t.Errorf("total completed = %d, want 1", s.TotalCompleted)
	}
	// Peak should remain at 3.
	if s.PeakSessions != 3 {
		t.Errorf("peak should remain 3, got %d", s.PeakSessions)
	}

	m.RecordError(ProviderGemini)
	s = m.Snapshot()
	if s.ActiveSessions != 1 {
		t.Errorf("active sessions after error = %d, want 1", s.ActiveSessions)
	}
	if s.TotalErrored != 1 {
		t.Errorf("total errored = %d, want 1", s.TotalErrored)
	}
}

func TestManagerMetrics_ProviderCounts(t *testing.T) {
	m := NewManagerMetrics()

	m.RecordLaunch(ProviderClaude)
	m.RecordLaunch(ProviderClaude)
	m.RecordLaunch(ProviderGemini)
	m.RecordLaunch(ProviderCodex)

	s := m.Snapshot()
	if s.ProviderCounts[ProviderClaude] != 2 {
		t.Errorf("claude count = %d, want 2", s.ProviderCounts[ProviderClaude])
	}
	if s.ProviderCounts[ProviderGemini] != 1 {
		t.Errorf("gemini count = %d, want 1", s.ProviderCounts[ProviderGemini])
	}
	if s.ProviderCounts[ProviderCodex] != 1 {
		t.Errorf("codex count = %d, want 1", s.ProviderCounts[ProviderCodex])
	}

	m.RecordComplete(ProviderClaude)
	s = m.Snapshot()
	if s.ProviderCounts[ProviderClaude] != 1 {
		t.Errorf("claude count after complete = %d, want 1", s.ProviderCounts[ProviderClaude])
	}
}

func TestManagerMetrics_ContentionTracking(t *testing.T) {
	m := NewManagerMetrics()

	m.RecordContention(10 * time.Millisecond)
	m.RecordContention(30 * time.Millisecond)

	s := m.Snapshot()
	if s.ContentionCount != 2 {
		t.Errorf("contention count = %d, want 2", s.ContentionCount)
	}
	if s.ContentionTotal != 40*time.Millisecond {
		t.Errorf("contention total = %v, want 40ms", s.ContentionTotal)
	}
	if s.ContentionAvg != 20*time.Millisecond {
		t.Errorf("contention avg = %v, want 20ms", s.ContentionAvg)
	}
}

func TestManagerMetrics_LaunchLatency(t *testing.T) {
	m := NewManagerMetrics()

	m.RecordLaunchLatency(100 * time.Millisecond)
	m.RecordLaunchLatency(200 * time.Millisecond)
	m.RecordLaunchLatency(300 * time.Millisecond)

	s := m.Snapshot()
	if s.LaunchCount != 3 {
		t.Errorf("launch count = %d, want 3", s.LaunchCount)
	}
	if s.LaunchLatencyTotal != 600*time.Millisecond {
		t.Errorf("launch latency total = %v, want 600ms", s.LaunchLatencyTotal)
	}
	if s.LaunchLatencyAvg != 200*time.Millisecond {
		t.Errorf("launch latency avg = %v, want 200ms", s.LaunchLatencyAvg)
	}
}

func TestManagerMetrics_QueryLatency(t *testing.T) {
	m := NewManagerMetrics()

	m.RecordQueryLatency(5 * time.Millisecond)
	m.RecordQueryLatency(15 * time.Millisecond)

	s := m.Snapshot()
	if s.QueryCount != 2 {
		t.Errorf("query count = %d, want 2", s.QueryCount)
	}
	if s.QueryLatencyAvg != 10*time.Millisecond {
		t.Errorf("query latency avg = %v, want 10ms", s.QueryLatencyAvg)
	}
}

func TestManagerMetrics_SnapshotZeroAverages(t *testing.T) {
	m := NewManagerMetrics()
	s := m.Snapshot()

	if s.ContentionAvg != 0 {
		t.Errorf("zero-state contention avg = %v, want 0", s.ContentionAvg)
	}
	if s.LaunchLatencyAvg != 0 {
		t.Errorf("zero-state launch avg = %v, want 0", s.LaunchLatencyAvg)
	}
	if s.QueryLatencyAvg != 0 {
		t.Errorf("zero-state query avg = %v, want 0", s.QueryLatencyAvg)
	}
	if len(s.ProviderCounts) != 0 {
		t.Errorf("zero-state provider counts should be empty, got %v", s.ProviderCounts)
	}
}

func TestManagerMetrics_ConcurrentUpdates(t *testing.T) {
	m := NewManagerMetrics()

	const goroutines = 100
	const opsPerGoroutine = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			p := []Provider{ProviderClaude, ProviderGemini, ProviderCodex}[id%3]
			for j := 0; j < opsPerGoroutine; j++ {
				m.RecordLaunch(p)
				m.RecordContention(time.Microsecond)
				m.RecordLaunchLatency(time.Microsecond)
				m.RecordQueryLatency(time.Microsecond)
				// Take snapshots mid-flight to stress concurrent reads.
				if j%100 == 0 {
					_ = m.Snapshot()
				}
				m.RecordComplete(p)
			}
		}(i)
	}

	wg.Wait()

	s := m.Snapshot()

	wantTotal := int64(goroutines * opsPerGoroutine)
	if s.TotalLaunched != wantTotal {
		t.Errorf("total launched = %d, want %d", s.TotalLaunched, wantTotal)
	}
	if s.TotalCompleted != wantTotal {
		t.Errorf("total completed = %d, want %d", s.TotalCompleted, wantTotal)
	}
	if s.ActiveSessions != 0 {
		t.Errorf("active sessions = %d, want 0 (all completed)", s.ActiveSessions)
	}
	if s.ContentionCount != wantTotal {
		t.Errorf("contention count = %d, want %d", s.ContentionCount, wantTotal)
	}
	if s.LaunchCount != wantTotal {
		t.Errorf("launch count = %d, want %d", s.LaunchCount, wantTotal)
	}
	if s.QueryCount != wantTotal {
		t.Errorf("query count = %d, want %d", s.QueryCount, wantTotal)
	}
	if s.PeakSessions < 1 {
		t.Error("peak sessions should be at least 1")
	}

	// All providers should sum to zero active.
	var provSum int64
	for _, c := range s.ProviderCounts {
		provSum += c
	}
	if provSum != 0 {
		t.Errorf("provider count sum = %d, want 0", provSum)
	}
}

func TestManagerMetrics_PeakWatermark(t *testing.T) {
	m := NewManagerMetrics()

	// Launch 5, complete 3, launch 2 more — peak should be 5.
	for i := 0; i < 5; i++ {
		m.RecordLaunch(ProviderClaude)
	}
	for i := 0; i < 3; i++ {
		m.RecordComplete(ProviderClaude)
	}
	for i := 0; i < 2; i++ {
		m.RecordLaunch(ProviderGemini)
	}

	s := m.Snapshot()
	if s.PeakSessions != 5 {
		t.Errorf("peak = %d, want 5", s.PeakSessions)
	}
	if s.ActiveSessions != 4 {
		t.Errorf("active = %d, want 4", s.ActiveSessions)
	}
}

func TestManagerMetrics_SnapshotIsIsolated(t *testing.T) {
	m := NewManagerMetrics()
	m.RecordLaunch(ProviderClaude)

	s1 := m.Snapshot()

	// Mutate the provider counts map in the snapshot.
	s1.ProviderCounts[ProviderGemini] = 999

	s2 := m.Snapshot()
	if s2.ProviderCounts[ProviderGemini] != 0 {
		t.Errorf("snapshot mutation leaked: gemini = %d, want 0", s2.ProviderCounts[ProviderGemini])
	}
}
