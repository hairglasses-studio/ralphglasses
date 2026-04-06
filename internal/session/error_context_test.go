package session

import (
	"strings"
	"sync"
	"testing"
)

func TestErrorContext_RecordError(t *testing.T) {
	t.Parallel()
	ec := NewErrorContext(3)

	ec.RecordError("build failed: missing import", ErrCatBuild, 1)
	ec.RecordError("test failed: expected 42", ErrCatTest, 2)

	if got := ec.TotalErrors(); got != 2 {
		t.Errorf("TotalErrors() = %d, want 2", got)
	}

	errors := ec.Errors()
	if errors[0].Message != "build failed: missing import" {
		t.Errorf("first error message = %q, want %q", errors[0].Message, "build failed: missing import")
	}
	if errors[0].Category != ErrCatBuild {
		t.Errorf("first error category = %q, want %q", errors[0].Category, ErrCatBuild)
	}
	if errors[0].Iteration != 1 {
		t.Errorf("first error iteration = %d, want 1", errors[0].Iteration)
	}
	if errors[1].Message != "test failed: expected 42" {
		t.Errorf("second error message = %q, want %q", errors[1].Message, "test failed: expected 42")
	}
}

func TestErrorContext_ConsecutiveCount(t *testing.T) {
	t.Parallel()
	ec := NewErrorContext(3)

	ec.RecordError("err1", ErrCatBuild, 1)
	if got := ec.ConsecutiveErrors(); got != 1 {
		t.Errorf("ConsecutiveErrors() = %d, want 1", got)
	}

	ec.RecordError("err2", ErrCatBuild, 2)
	if got := ec.ConsecutiveErrors(); got != 2 {
		t.Errorf("ConsecutiveErrors() = %d, want 2", got)
	}

	ec.RecordError("err3", ErrCatTest, 3)
	if got := ec.ConsecutiveErrors(); got != 3 {
		t.Errorf("ConsecutiveErrors() = %d, want 3", got)
	}
}

func TestErrorContext_EscalationThreshold(t *testing.T) {
	t.Parallel()
	ec := NewErrorContext(3)

	ec.RecordError("err1", ErrCatBuild, 1)
	ec.RecordError("err2", ErrCatBuild, 2)
	if ec.ShouldEscalate() {
		t.Error("ShouldEscalate() = true before threshold, want false")
	}

	ec.RecordError("err3", ErrCatBuild, 3)
	if !ec.ShouldEscalate() {
		t.Error("ShouldEscalate() = false at threshold, want true")
	}

	ec.RecordError("err4", ErrCatBuild, 4)
	if !ec.ShouldEscalate() {
		t.Error("ShouldEscalate() = false above threshold, want true")
	}
}

func TestErrorContext_SuccessResetsConsecutive(t *testing.T) {
	t.Parallel()
	ec := NewErrorContext(3)

	ec.RecordError("err1", ErrCatBuild, 1)
	ec.RecordError("err2", ErrCatBuild, 2)
	if got := ec.ConsecutiveErrors(); got != 2 {
		t.Errorf("ConsecutiveErrors() = %d before success, want 2", got)
	}

	ec.RecordSuccess()
	if got := ec.ConsecutiveErrors(); got != 0 {
		t.Errorf("ConsecutiveErrors() = %d after success, want 0", got)
	}
	if ec.ShouldEscalate() {
		t.Error("ShouldEscalate() = true after success, want false")
	}

	// Errors still in buffer after success.
	if got := ec.TotalErrors(); got != 2 {
		t.Errorf("TotalErrors() = %d after success, want 2 (errors should persist)", got)
	}
}

func TestErrorContext_FormatForContext(t *testing.T) {
	t.Parallel()
	ec := NewErrorContext(3)

	ec.RecordError("Build failed: missing import \"fmt\"", ErrCatBuild, 5)
	ec.RecordError("TestFoo: expected 42, got 0", ErrCatTest, 6)

	result := ec.FormatForContext()

	if !strings.HasPrefix(result, "<recent_errors") {
		t.Error("FormatForContext should start with <recent_errors")
	}
	if !strings.HasSuffix(result, "</recent_errors>") {
		t.Error("FormatForContext should end with </recent_errors>")
	}
	if !strings.Contains(result, `count="2"`) {
		t.Error("FormatForContext should contain count=\"2\"")
	}
	if !strings.Contains(result, `consecutive="2"`) {
		t.Error("FormatForContext should contain consecutive=\"2\"")
	}
	if !strings.Contains(result, `category="build"`) {
		t.Error("FormatForContext should contain category=\"build\"")
	}
	if !strings.Contains(result, `category="test"`) {
		t.Error("FormatForContext should contain category=\"test\"")
	}
	if !strings.Contains(result, `iteration="5"`) {
		t.Error("FormatForContext should contain iteration=\"5\"")
	}
	if !strings.Contains(result, `iteration="6"`) {
		t.Error("FormatForContext should contain iteration=\"6\"")
	}
	if !strings.Contains(result, "Build failed: missing import") {
		t.Error("FormatForContext should contain the build error message")
	}
	if !strings.Contains(result, "TestFoo: expected 42, got 0") {
		t.Error("FormatForContext should contain the test error message")
	}
}

func TestErrorContext_FormatForContext_Empty(t *testing.T) {
	t.Parallel()
	ec := NewErrorContext(3)

	result := ec.FormatForContext()
	if result != "" {
		t.Errorf("FormatForContext() on empty context = %q, want empty string", result)
	}
}

func TestErrorContext_RingBufferOverflow(t *testing.T) {
	t.Parallel()
	ec := NewErrorContext(3)

	// Record 15 errors (buffer holds max 10).
	for i := 1; i <= 15; i++ {
		ec.RecordError("error "+strings.Repeat("x", i), ErrCatRuntime, i)
	}

	if got := ec.TotalErrors(); got != DefaultMaxErrors {
		t.Errorf("TotalErrors() = %d, want %d (max ring buffer size)", got, DefaultMaxErrors)
	}

	// Verify the oldest entries were evicted: first entry should be iteration 6.
	errors := ec.Errors()
	if errors[0].Iteration != 6 {
		t.Errorf("oldest error iteration = %d, want 6 (first 5 should be evicted)", errors[0].Iteration)
	}
	if errors[len(errors)-1].Iteration != 15 {
		t.Errorf("newest error iteration = %d, want 15", errors[len(errors)-1].Iteration)
	}
}

func TestErrorContext_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	ec := NewErrorContext(5)

	var wg sync.WaitGroup
	// Spawn writers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ec.RecordError("concurrent error", ErrCatUnknown, n)
		}(i)
	}
	// Spawn readers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ec.ConsecutiveErrors()
			_ = ec.ShouldEscalate()
			_ = ec.FormatForContext()
			_ = ec.TotalErrors()
			_ = ec.Errors()
		}()
	}
	// Spawn success recorders.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ec.RecordSuccess()
		}()
	}

	wg.Wait()

	// No assertions on final state since ordering is non-deterministic.
	// The test passes if there are no races (run with -race).
	if ec.TotalErrors() > DefaultMaxErrors {
		t.Errorf("TotalErrors() = %d, exceeded max %d", ec.TotalErrors(), DefaultMaxErrors)
	}
}

func TestErrorContext_Reset(t *testing.T) {
	t.Parallel()
	ec := NewErrorContext(3)

	ec.RecordError("err1", ErrCatBuild, 1)
	ec.RecordError("err2", ErrCatTest, 2)
	ec.RecordError("err3", ErrCatLint, 3)

	if ec.TotalErrors() != 3 {
		t.Fatalf("TotalErrors() = %d before reset, want 3", ec.TotalErrors())
	}
	if ec.ConsecutiveErrors() != 3 {
		t.Fatalf("ConsecutiveErrors() = %d before reset, want 3", ec.ConsecutiveErrors())
	}

	ec.Reset()

	if got := ec.TotalErrors(); got != 0 {
		t.Errorf("TotalErrors() = %d after reset, want 0", got)
	}
	if got := ec.ConsecutiveErrors(); got != 0 {
		t.Errorf("ConsecutiveErrors() = %d after reset, want 0", got)
	}
	if got := ec.FormatForContext(); got != "" {
		t.Errorf("FormatForContext() = %q after reset, want empty string", got)
	}
	if ec.ShouldEscalate() {
		t.Error("ShouldEscalate() = true after reset, want false")
	}
}

func TestErrorContext_DefaultMaxConsecutive(t *testing.T) {
	t.Parallel()
	ec := NewErrorContext(0)

	// Should use DefaultMaxConsecutive (3).
	ec.RecordError("err1", ErrCatBuild, 1)
	ec.RecordError("err2", ErrCatBuild, 2)
	if ec.ShouldEscalate() {
		t.Error("ShouldEscalate() = true at 2 errors with default threshold, want false")
	}

	ec.RecordError("err3", ErrCatBuild, 3)
	if !ec.ShouldEscalate() {
		t.Error("ShouldEscalate() = false at 3 errors with default threshold, want true")
	}
}

func TestErrorContext_AllCategories(t *testing.T) {
	t.Parallel()
	ec := NewErrorContext(10)

	categories := []ErrorCategory{
		ErrCatBuild, ErrCatTest, ErrCatLint, ErrCatRuntime,
		ErrCatTimeout, ErrCatPermission, ErrCatNetwork, ErrCatUnknown,
	}

	for i, cat := range categories {
		ec.RecordError("error for "+string(cat), cat, i+1)
	}

	if got := ec.TotalErrors(); got != len(categories) {
		t.Errorf("TotalErrors() = %d, want %d", got, len(categories))
	}

	result := ec.FormatForContext()
	for _, cat := range categories {
		if !strings.Contains(result, string(cat)) {
			t.Errorf("FormatForContext() missing category %q", cat)
		}
	}
}

func TestErrorContext_ManagerGetErrorContext(t *testing.T) {
	t.Parallel()
	m := NewManager()

	ec1 := m.GetErrorContext("session-1")
	if ec1 == nil {
		t.Fatal("GetErrorContext returned nil")
	}

	// Same ID returns same instance.
	ec2 := m.GetErrorContext("session-1")
	if ec1 != ec2 {
		t.Error("GetErrorContext returned different instances for the same session ID")
	}

	// Different ID returns different instance.
	ec3 := m.GetErrorContext("session-2")
	if ec1 == ec3 {
		t.Error("GetErrorContext returned same instance for different session IDs")
	}
}
