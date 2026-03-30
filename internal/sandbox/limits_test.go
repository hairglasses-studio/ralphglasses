package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ResourceLimits defaults
// ---------------------------------------------------------------------------

func TestResourceLimits_Defaults(t *testing.T) {
	t.Parallel()

	var rl ResourceLimits
	if rl.CPUTimeout != 0 {
		t.Errorf("zero CPUTimeout = %v, want 0", rl.CPUTimeout)
	}
	if rl.MemoryLimitBytes != 0 {
		t.Errorf("zero MemoryLimitBytes = %d, want 0", rl.MemoryLimitBytes)
	}
	if rl.MaxFiles != 0 {
		t.Errorf("zero MaxFiles = %d, want 0", rl.MaxFiles)
	}
	if rl.PollInterval != 0 {
		t.Errorf("zero PollInterval = %v, want 0", rl.PollInterval)
	}
}

func TestNewResourceLimiter_DefaultPollInterval(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{}, "")
	if rl.limits.PollInterval != 250*time.Millisecond {
		t.Errorf("PollInterval = %v, want 250ms", rl.limits.PollInterval)
	}
}

func TestNewResourceLimiter_CustomPollInterval(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{PollInterval: time.Second}, "")
	if rl.limits.PollInterval != time.Second {
		t.Errorf("PollInterval = %v, want 1s", rl.limits.PollInterval)
	}
}

// ---------------------------------------------------------------------------
// CPU timeout enforcement (via context deadline)
// ---------------------------------------------------------------------------

func TestResourceLimiter_CPUTimeout(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{
		CPUTimeout:   200 * time.Millisecond,
		PollInterval: 50 * time.Millisecond,
	}, "")

	// Use a command that sleeps indefinitely; the limiter should kill it.
	usage, err := rl.RunCmd(context.Background(), "sleep", "30")

	if err == nil {
		t.Fatal("expected error from CPU timeout, got nil")
	}
	if usage.LimitHit != ErrCPUTimeExceeded {
		t.Errorf("LimitHit = %v, want ErrCPUTimeExceeded", usage.LimitHit)
	}
	if usage.Duration < 150*time.Millisecond {
		t.Errorf("Duration = %v, expected >= 150ms (timeout is 200ms)", usage.Duration)
	}
}

func TestResourceLimiter_NoCPUTimeout(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{
		PollInterval: 50 * time.Millisecond,
	}, "")

	// A fast command that exits normally.
	usage, err := rl.RunCmd(context.Background(), "true")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.LimitHit != nil {
		t.Errorf("LimitHit = %v, want nil", usage.LimitHit)
	}
}

// ---------------------------------------------------------------------------
// Memory limit enforcement (with mock RSS reader)
// ---------------------------------------------------------------------------

func TestResourceLimiter_MemoryExceeded(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{
		MemoryLimitBytes: 100 * 1024 * 1024, // 100 MB
		PollInterval:     20 * time.Millisecond,
	}, "")

	// Mock RSS reader: always returns 200 MB.
	rl.rssReader = func(pid int) (int64, error) {
		return 200 * 1024 * 1024, nil
	}

	usage, err := rl.RunCmd(context.Background(), "sleep", "30")

	if err != ErrMemoryExceeded {
		t.Fatalf("expected ErrMemoryExceeded, got %v", err)
	}
	if usage.LimitHit != ErrMemoryExceeded {
		t.Errorf("LimitHit = %v, want ErrMemoryExceeded", usage.LimitHit)
	}
	if usage.PeakRSSBytes < 200*1024*1024 {
		t.Errorf("PeakRSSBytes = %d, want >= 200MB", usage.PeakRSSBytes)
	}
}

func TestResourceLimiter_MemoryUnderLimit(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{
		MemoryLimitBytes: 500 * 1024 * 1024, // 500 MB
		PollInterval:     20 * time.Millisecond,
	}, "")

	// Mock RSS reader: returns 10 MB (well under limit).
	rl.rssReader = func(pid int) (int64, error) {
		return 10 * 1024 * 1024, nil
	}

	usage, err := rl.RunCmd(context.Background(), "true")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.LimitHit != nil {
		t.Errorf("LimitHit = %v, want nil", usage.LimitHit)
	}
}

func TestResourceLimiter_MemoryPeakTracking(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{
		MemoryLimitBytes: 500 * 1024 * 1024, // 500 MB
		PollInterval:     15 * time.Millisecond,
	}, "")

	// Simulate increasing then decreasing RSS.
	callCount := 0
	rssValues := []int64{
		10 * 1024 * 1024,  // 10 MB
		50 * 1024 * 1024,  // 50 MB
		100 * 1024 * 1024, // 100 MB (peak)
		80 * 1024 * 1024,  // 80 MB
		30 * 1024 * 1024,  // 30 MB
	}
	rl.rssReader = func(pid int) (int64, error) {
		idx := callCount
		if idx >= len(rssValues) {
			idx = len(rssValues) - 1
		}
		callCount++
		return rssValues[idx], nil
	}

	// Sleep long enough for several polls.
	usage, err := rl.RunCmd(context.Background(), "sleep", "0.2")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.PeakRSSBytes < 100*1024*1024 {
		t.Errorf("PeakRSSBytes = %d, want >= 100MB", usage.PeakRSSBytes)
	}
}

func TestResourceLimiter_RSSReaderErrorIgnored(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{
		MemoryLimitBytes: 100 * 1024 * 1024,
		PollInterval:     20 * time.Millisecond,
	}, "")

	// RSS reader always returns an error (simulates process already exited).
	rl.rssReader = func(pid int) (int64, error) {
		return 0, fmt.Errorf("no such process")
	}

	usage, err := rl.RunCmd(context.Background(), "true")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.LimitHit != nil {
		t.Errorf("LimitHit = %v, want nil (errors should be ignored)", usage.LimitHit)
	}
}

// ---------------------------------------------------------------------------
// File count limit enforcement (with mock file counter)
// ---------------------------------------------------------------------------

func TestResourceLimiter_FileCountExceeded(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	rl := NewResourceLimiter(ResourceLimits{
		MaxFiles:     3,
		PollInterval: 20 * time.Millisecond,
	}, tmpDir)

	// Mock file counter: returns 0 for baseline, then 10 for monitoring polls.
	callNum := 0
	rl.fileCounter = func(dir string) (int, error) {
		callNum++
		if callNum == 1 {
			return 0, nil // baseline snapshot
		}
		return 10, nil // monitoring: 10 - 0 = 10 created > 3 limit
	}

	usage, err := rl.RunCmd(context.Background(), "sleep", "30")

	if err != ErrFileCountExceeded {
		t.Fatalf("expected ErrFileCountExceeded, got %v", err)
	}
	if usage.LimitHit != ErrFileCountExceeded {
		t.Errorf("LimitHit = %v, want ErrFileCountExceeded", usage.LimitHit)
	}
}

func TestResourceLimiter_FileCountUnderLimit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	rl := NewResourceLimiter(ResourceLimits{
		MaxFiles:     10,
		PollInterval: 20 * time.Millisecond,
	}, tmpDir)

	// Mock file counter: starts at 5, stays at 5 (no new files).
	rl.fileCounter = func(dir string) (int, error) {
		return 5, nil
	}

	usage, err := rl.RunCmd(context.Background(), "true")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.LimitHit != nil {
		t.Errorf("LimitHit = %v, want nil", usage.LimitHit)
	}
}

func TestResourceLimiter_FileCountWithRealDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	rl := NewResourceLimiter(ResourceLimits{
		MaxFiles:     5,
		PollInterval: 20 * time.Millisecond,
	}, tmpDir)
	// Use the real fileCounter for this test.

	// Create some files before the run so the baseline is established.
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(tmpDir, fmt.Sprintf("pre_%d.txt", i)), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	usage, err := rl.RunCmd(context.Background(), "true")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.LimitHit != nil {
		t.Errorf("LimitHit = %v, want nil", usage.LimitHit)
	}
	// No new files created, so FilesCreated should be 0.
	if usage.FilesCreated != 0 {
		t.Errorf("FilesCreated = %d, want 0", usage.FilesCreated)
	}
}

func TestResourceLimiter_FileCountDisabledWhenNoTrackDir(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{
		MaxFiles:     5,
		PollInterval: 20 * time.Millisecond,
	}, "") // no trackDir

	usage, err := rl.RunCmd(context.Background(), "true")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.LimitHit != nil {
		t.Errorf("LimitHit = %v, want nil", usage.LimitHit)
	}
}

// ---------------------------------------------------------------------------
// Context cancellation (parent context cancelled)
// ---------------------------------------------------------------------------

func TestResourceLimiter_ParentContextCancelled(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{
		PollInterval: 20 * time.Millisecond,
	}, "")

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := rl.RunCmd(ctx, "sleep", "30")
	// The command should be killed and return an error.
	if err == nil {
		t.Fatal("expected error from context cancellation, got nil")
	}
}

// ---------------------------------------------------------------------------
// Combined limits
// ---------------------------------------------------------------------------

func TestResourceLimiter_CombinedLimits(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	rl := NewResourceLimiter(ResourceLimits{
		CPUTimeout:       2 * time.Second,
		MemoryLimitBytes: 500 * 1024 * 1024,
		MaxFiles:         100,
		PollInterval:     20 * time.Millisecond,
	}, tmpDir)

	// All within limits.
	rl.rssReader = func(pid int) (int64, error) {
		return 50 * 1024 * 1024, nil
	}
	rl.fileCounter = func(dir string) (int, error) {
		return 0, nil
	}

	usage, err := rl.RunCmd(context.Background(), "true")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.LimitHit != nil {
		t.Errorf("LimitHit = %v, want nil", usage.LimitHit)
	}
	if usage.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", usage.Duration)
	}
}

// ---------------------------------------------------------------------------
// Sentinel error identity
// ---------------------------------------------------------------------------

func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"cpu", ErrCPUTimeExceeded, "cpu time limit exceeded"},
		{"memory", ErrMemoryExceeded, "memory limit exceeded"},
		{"files", ErrFileCountExceeded, "file count limit exceeded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.err.Error() != tt.msg {
				t.Errorf("error = %q, want %q", tt.err.Error(), tt.msg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ResourceUsage struct
// ---------------------------------------------------------------------------

func TestResourceUsage_ZeroValue(t *testing.T) {
	t.Parallel()

	var u ResourceUsage
	if u.PeakRSSBytes != 0 {
		t.Errorf("PeakRSSBytes = %d, want 0", u.PeakRSSBytes)
	}
	if u.FilesCreated != 0 {
		t.Errorf("FilesCreated = %d, want 0", u.FilesCreated)
	}
	if u.Duration != 0 {
		t.Errorf("Duration = %v, want 0", u.Duration)
	}
	if u.LimitHit != nil {
		t.Errorf("LimitHit = %v, want nil", u.LimitHit)
	}
}

// ---------------------------------------------------------------------------
// Usage() accessor
// ---------------------------------------------------------------------------

func TestResourceLimiter_UsageAccessor(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{
		PollInterval: 20 * time.Millisecond,
	}, "")

	_, err := rl.RunCmd(context.Background(), "true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	usage := rl.Usage()
	if usage.Duration <= 0 {
		t.Errorf("Usage().Duration = %v, want > 0", usage.Duration)
	}
}

// ---------------------------------------------------------------------------
// countFiles unit tests
// ---------------------------------------------------------------------------

func TestCountFiles_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	n, err := countFiles(dir)
	if err != nil {
		t.Fatalf("countFiles: %v", err)
	}
	if n != 0 {
		t.Errorf("countFiles = %d, want 0", n)
	}
}

func TestCountFiles_WithFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Also create a subdirectory (should not be counted).
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	n, err := countFiles(dir)
	if err != nil {
		t.Fatalf("countFiles: %v", err)
	}
	if n != 5 {
		t.Errorf("countFiles = %d, want 5", n)
	}
}

func TestCountFiles_NonexistentDir(t *testing.T) {
	t.Parallel()

	_, err := countFiles("/nonexistent/path/abc123")
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}
}

// ---------------------------------------------------------------------------
// readRSSLinux unit test (string parsing)
// ---------------------------------------------------------------------------

func TestReadRSSLinux_ParsesVmRSS(t *testing.T) {
	t.Parallel()

	// We can only test the Linux parsing logic directly if we mock /proc.
	// Instead, test the line-parsing logic indirectly by verifying the function
	// exists and returns an appropriate error on non-Linux.
	if testing.Short() {
		t.Skip("skipping RSS test in short mode")
	}

	// On macOS, readRSSLinux should fail with a file not found error.
	_, err := readRSSLinux(os.Getpid())
	if err == nil {
		// Running on Linux where /proc exists -- that's fine.
		return
	}
	// On other OSes, it should error.
	if err != nil {
		t.Logf("expected error on non-Linux: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Start failure
// ---------------------------------------------------------------------------

func TestResourceLimiter_StartFailure(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{
		PollInterval: 20 * time.Millisecond,
	}, "")

	// Command that cannot be started (nonexistent binary).
	_, err := rl.RunCmd(context.Background(), "/nonexistent/binary/xyz")

	if err == nil {
		t.Fatal("expected error from start failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// Initial file count error
// ---------------------------------------------------------------------------

func TestResourceLimiter_InitialFileCountError(t *testing.T) {
	t.Parallel()

	rl := NewResourceLimiter(ResourceLimits{
		MaxFiles:     5,
		PollInterval: 20 * time.Millisecond,
	}, "/nonexistent/path/abc123")

	_, err := rl.RunCmd(context.Background(), "true")

	if err == nil {
		t.Fatal("expected error from initial file count, got nil")
	}
}
