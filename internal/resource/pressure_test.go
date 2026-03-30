package resource

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeMetrics is a test double for metricsSource.
type fakeMetrics struct {
	mu   sync.Mutex
	cpu  float64
	mem  float64
	disk float64
}

func (f *fakeMetrics) cpuLoadPct() float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cpu
}

func (f *fakeMetrics) memoryUsedPct() float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mem
}

func (f *fakeMetrics) diskUsedPct() float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.disk
}

func (f *fakeMetrics) set(cpu, mem, disk float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cpu = cpu
	f.mem = mem
	f.disk = disk
}

func TestClassifyPressure(t *testing.T) {
	tests := []struct {
		pct  float64
		want PressureLevel
	}{
		{0, PressureLow},
		{50, PressureLow},
		{69.9, PressureLow},
		{70, PressureMedium},
		{75, PressureMedium},
		{84.9, PressureMedium},
		{85, PressureHigh},
		{90, PressureHigh},
		{94.9, PressureHigh},
		{95, PressureCritical},
		{99, PressureCritical},
		{100, PressureCritical},
	}
	for _, tt := range tests {
		got := classifyPressure(tt.pct)
		if got != tt.want {
			t.Errorf("classifyPressure(%.1f) = %s, want %s", tt.pct, got, tt.want)
		}
	}
}

func TestRecommendation(t *testing.T) {
	tests := []struct {
		level PressureLevel
		want  ThrottleAction
	}{
		{PressureLow, ThrottleNone},
		{PressureMedium, ThrottleReduceWorkers},
		{PressureHigh, ThrottlePauseLaunches},
		{PressureCritical, ThrottleEmergencyStop},
	}
	for _, tt := range tests {
		got := recommend(tt.level)
		if got != tt.want {
			t.Errorf("recommend(%s) = %s, want %s", tt.level, got, tt.want)
		}
	}
}

func TestMaxLevel(t *testing.T) {
	tests := []struct {
		name   string
		levels []PressureLevel
		want   PressureLevel
	}{
		{"all low", []PressureLevel{PressureLow, PressureLow, PressureLow}, PressureLow},
		{"one high", []PressureLevel{PressureLow, PressureHigh, PressureMedium}, PressureHigh},
		{"critical wins", []PressureLevel{PressureMedium, PressureCritical, PressureLow}, PressureCritical},
		{"single", []PressureLevel{PressureMedium}, PressureMedium},
		{"empty", []PressureLevel{}, PressureLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maxLevel(tt.levels...)
			if got != tt.want {
				t.Errorf("maxLevel(%v) = %s, want %s", tt.levels, got, tt.want)
			}
		})
	}
}

func TestPressureReportFromMetrics(t *testing.T) {
	tests := []struct {
		name      string
		cpu       float64
		mem       float64
		disk      float64
		wantLevel PressureLevel
		wantRec   ThrottleAction
	}{
		{"all low", 30, 40, 20, PressureLow, ThrottleNone},
		{"cpu medium", 75, 40, 20, PressureMedium, ThrottleReduceWorkers},
		{"mem high", 30, 90, 20, PressureHigh, ThrottlePauseLaunches},
		{"disk critical", 30, 40, 97, PressureCritical, ThrottleEmergencyStop},
		{"cpu critical overrides mem medium", 96, 72, 20, PressureCritical, ThrottleEmergencyStop},
		{"all high takes highest", 90, 88, 91, PressureHigh, ThrottlePauseLaunches},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := &fakeMetrics{cpu: tt.cpu, mem: tt.mem, disk: tt.disk}
			pm := newTestMonitor(time.Hour, fm) // long interval, we'll poll manually

			pm.poll()
			report := pm.CurrentPressure()

			if report.OverallLevel != tt.wantLevel {
				t.Errorf("overall level = %s, want %s", report.OverallLevel, tt.wantLevel)
			}
			if report.Recommendation != tt.wantRec {
				t.Errorf("recommendation = %s, want %s", report.Recommendation, tt.wantRec)
			}
			if report.CPULoadPct != tt.cpu {
				t.Errorf("cpu pct = %.1f, want %.1f", report.CPULoadPct, tt.cpu)
			}
			if report.MemoryUsedPct != tt.mem {
				t.Errorf("mem pct = %.1f, want %.1f", report.MemoryUsedPct, tt.mem)
			}
			if report.DiskUsedPct != tt.disk {
				t.Errorf("disk pct = %.1f, want %.1f", report.DiskUsedPct, tt.disk)
			}
			if report.Timestamp.IsZero() {
				t.Error("timestamp should not be zero")
			}
		})
	}
}

func TestCallbackFiresOnLevelTransition(t *testing.T) {
	fm := &fakeMetrics{cpu: 30, mem: 30, disk: 30}
	pm := newTestMonitor(10*time.Millisecond, fm)

	var mu sync.Mutex
	var reports []PressureReport
	pm.OnPressureChange(func(r PressureReport) {
		mu.Lock()
		reports = append(reports, r)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	go pm.Start(ctx)

	// Wait for initial poll to settle.
	time.Sleep(50 * time.Millisecond)

	// Transition to high.
	fm.set(90, 30, 30)
	time.Sleep(50 * time.Millisecond)

	// Transition to critical.
	fm.set(96, 30, 30)
	time.Sleep(50 * time.Millisecond)

	cancel()
	<-pm.done

	mu.Lock()
	defer mu.Unlock()

	if len(reports) < 2 {
		t.Fatalf("expected at least 2 callbacks, got %d", len(reports))
	}

	// First transition should be to high.
	if reports[0].OverallLevel != PressureHigh {
		t.Errorf("first transition = %s, want high", reports[0].OverallLevel)
	}

	// Second transition should be to critical.
	foundCritical := false
	for _, r := range reports[1:] {
		if r.OverallLevel == PressureCritical {
			foundCritical = true
			break
		}
	}
	if !foundCritical {
		t.Error("expected a transition to critical level")
	}
}

func TestCallbackDoesNotFireWhenLevelSame(t *testing.T) {
	fm := &fakeMetrics{cpu: 30, mem: 30, disk: 30}
	pm := newTestMonitor(10*time.Millisecond, fm)

	callCount := 0
	var mu sync.Mutex
	pm.OnPressureChange(func(_ PressureReport) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	go pm.Start(ctx)

	// Let it poll several times at the same level.
	time.Sleep(100 * time.Millisecond)

	cancel()
	<-pm.done

	mu.Lock()
	defer mu.Unlock()

	// Should not have fired: level stays at PressureLow the entire time.
	if callCount != 0 {
		t.Errorf("expected 0 callbacks when level stays same, got %d", callCount)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	fm := &fakeMetrics{cpu: 30, mem: 30, disk: 30}
	pm := newTestMonitor(10*time.Millisecond, fm)

	ctx, cancel := context.WithCancel(context.Background())
	go pm.Start(ctx)
	time.Sleep(30 * time.Millisecond)

	cancel()
	// Calling Stop after context cancel should not panic.
	pm.Stop()
	pm.Stop() // second call should be safe too
}

func TestPressureLevelString(t *testing.T) {
	tests := []struct {
		level PressureLevel
		want  string
	}{
		{PressureLow, "low"},
		{PressureMedium, "medium"},
		{PressureHigh, "high"},
		{PressureCritical, "critical"},
		{PressureLevel(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("PressureLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestThrottleActionString(t *testing.T) {
	tests := []struct {
		action ThrottleAction
		want   string
	}{
		{ThrottleNone, "none"},
		{ThrottleReduceWorkers, "reduce-workers"},
		{ThrottlePauseLaunches, "pause-launches"},
		{ThrottleEmergencyStop, "emergency-stop"},
		{ThrottleAction(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.action.String(); got != tt.want {
			t.Errorf("ThrottleAction(%d).String() = %q, want %q", tt.action, got, tt.want)
		}
	}
}

func TestPressureReportSummary(t *testing.T) {
	r := PressureReport{
		CPU:            PressureHigh,
		Memory:         PressureMedium,
		Disk:           PressureLow,
		OverallLevel:   PressureHigh,
		Recommendation: ThrottlePauseLaunches,
		CPULoadPct:     90,
		MemoryUsedPct:  75,
		DiskUsedPct:    40,
	}
	s := r.Summary()
	if s == "" {
		t.Error("summary should not be empty")
	}
	// Verify key substrings are present.
	for _, want := range []string{"pressure=high", "cpu=high", "mem=medium", "disk=low", "action=pause-launches"} {
		if !contains(s, want) {
			t.Errorf("summary %q missing %q", s, want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestParseMeminfo(t *testing.T) {
	data := []byte(`MemTotal:       16384000 kB
MemFree:          512000 kB
MemAvailable:    8192000 kB
Buffers:          256000 kB
Cached:          4096000 kB
`)
	total, avail := parseMeminfo(data)
	wantTotal := uint64(16384000) * 1024
	wantAvail := uint64(8192000) * 1024
	if total != wantTotal {
		t.Errorf("total = %d, want %d", total, wantTotal)
	}
	if avail != wantAvail {
		t.Errorf("available = %d, want %d", avail, wantAvail)
	}
}

func TestParseMeminfoEmpty(t *testing.T) {
	total, avail := parseMeminfo([]byte(""))
	if total != 0 || avail != 0 {
		t.Errorf("expected zeros for empty meminfo, got total=%d avail=%d", total, avail)
	}
}

func TestHighCPURecommendsReduceWorkers(t *testing.T) {
	// When only CPU is medium (70-85%), recommendation should be reduce-workers.
	fm := &fakeMetrics{cpu: 75, mem: 30, disk: 30}
	pm := newTestMonitor(time.Hour, fm)
	pm.poll()
	r := pm.CurrentPressure()
	if r.CPU != PressureMedium {
		t.Errorf("expected CPU=medium, got %s", r.CPU)
	}
	if r.Recommendation != ThrottleReduceWorkers {
		t.Errorf("expected reduce-workers, got %s", r.Recommendation)
	}
}

func TestCriticalRecommendsEmergencyStop(t *testing.T) {
	fm := &fakeMetrics{cpu: 97, mem: 30, disk: 30}
	pm := newTestMonitor(time.Hour, fm)
	pm.poll()
	r := pm.CurrentPressure()
	if r.Recommendation != ThrottleEmergencyStop {
		t.Errorf("expected emergency-stop, got %s", r.Recommendation)
	}
}

func TestMultipleCallbacks(t *testing.T) {
	fm := &fakeMetrics{cpu: 30, mem: 30, disk: 30}
	pm := newTestMonitor(time.Hour, fm)

	var mu sync.Mutex
	count1, count2 := 0, 0
	pm.OnPressureChange(func(_ PressureReport) {
		mu.Lock()
		count1++
		mu.Unlock()
	})
	pm.OnPressureChange(func(_ PressureReport) {
		mu.Lock()
		count2++
		mu.Unlock()
	})

	// Initial poll at low.
	pm.poll()

	// Transition to high — both callbacks should fire.
	fm.set(90, 30, 30)
	pm.poll()

	mu.Lock()
	defer mu.Unlock()
	if count1 != 1 || count2 != 1 {
		t.Errorf("expected both callbacks to fire once, got count1=%d count2=%d", count1, count2)
	}
}
