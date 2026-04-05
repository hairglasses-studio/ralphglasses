package envkit

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEnvAuditorGetSet(t *testing.T) {
	t.Setenv("TEST_AUDIT_VAR", "hello")
	a := NewEnvAuditor()

	val := a.Get("TEST_AUDIT_VAR", "test.component")
	if val != "hello" {
		t.Errorf("Get: got %q, want %q", val, "hello")
	}

	entries := a.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].VarName != "TEST_AUDIT_VAR" {
		t.Errorf("VarName: got %q, want %q", entries[0].VarName, "TEST_AUDIT_VAR")
	}
	if entries[0].Component != "test.component" {
		t.Errorf("Component: got %q, want %q", entries[0].Component, "test.component")
	}
	if !entries[0].WasSet {
		t.Error("WasSet should be true for a set variable")
	}
}

func TestEnvAuditorGetMissing(t *testing.T) {
	t.Setenv("TEST_AUDIT_MISSING", "")
	// Unset to ensure it's truly missing
	t.Setenv("TEST_AUDIT_MISSING_REAL", "")

	a := NewEnvAuditor()
	val := a.Get("TEST_AUDIT_NONEXISTENT_VAR_12345", "provider.claude")
	if val != "" {
		t.Errorf("Get: got %q, want empty string", val)
	}

	entries := a.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].WasSet {
		t.Error("WasSet should be false for an unset variable")
	}
}

func TestEnvAuditorLookupSet(t *testing.T) {
	t.Setenv("TEST_LOOKUP_VAR", "world")
	a := NewEnvAuditor()

	val, ok := a.Lookup("TEST_LOOKUP_VAR", "session.runner")
	if !ok {
		t.Error("Lookup: expected ok=true for set variable")
	}
	if val != "world" {
		t.Errorf("Lookup: got %q, want %q", val, "world")
	}

	entries := a.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !entries[0].WasSet {
		t.Error("WasSet should be true")
	}
}

func TestEnvAuditorLookupMissing(t *testing.T) {
	a := NewEnvAuditor()

	val, ok := a.Lookup("TEST_LOOKUP_NONEXISTENT_98765", "session.runner")
	if ok {
		t.Error("Lookup: expected ok=false for unset variable")
	}
	if val != "" {
		t.Errorf("Lookup: got %q, want empty", val)
	}

	entries := a.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].WasSet {
		t.Error("WasSet should be false")
	}
}

func TestEnvAuditorLookupEmptyValue(t *testing.T) {
	// A variable set to empty string is still "set"
	t.Setenv("TEST_LOOKUP_EMPTY", "")
	a := NewEnvAuditor()

	val, ok := a.Lookup("TEST_LOOKUP_EMPTY", "provider.gemini")
	if !ok {
		t.Error("Lookup: expected ok=true for variable set to empty string")
	}
	if val != "" {
		t.Errorf("Lookup: got %q, want empty", val)
	}

	entries := a.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !entries[0].WasSet {
		t.Error("WasSet should be true — variable is set, just empty")
	}
}

func TestAuditLogFormat(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	callCount := 0
	a := newEnvAuditorWithClock(func() time.Time {
		callCount++
		return ts.Add(time.Duration(callCount) * time.Second)
	})

	t.Setenv("API_KEY", "secret")
	a.Get("API_KEY", "provider.claude")
	a.Get("MISSING_VAR_FOR_TEST_99999", "session.runner")

	log := a.AuditLog()

	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d: %s", len(lines), log)
	}

	// First line: set variable
	if !strings.Contains(lines[0], "2025-06-15T10:30:01Z") {
		t.Errorf("line 0 missing timestamp: %s", lines[0])
	}
	if !strings.Contains(lines[0], "[provider.claude]") {
		t.Errorf("line 0 missing component: %s", lines[0])
	}
	if !strings.Contains(lines[0], "API_KEY") {
		t.Errorf("line 0 missing var name: %s", lines[0])
	}
	if !strings.Contains(lines[0], "(set)") {
		t.Errorf("line 0 missing status: %s", lines[0])
	}

	// Second line: missing variable
	if !strings.Contains(lines[1], "(missing)") {
		t.Errorf("line 1 missing status: %s", lines[1])
	}

	// Value should never appear in log
	if strings.Contains(log, "secret") {
		t.Error("audit log must not contain env var values")
	}
}

func TestAuditLogEmpty(t *testing.T) {
	a := NewEnvAuditor()
	log := a.AuditLog()
	if log != "" {
		t.Errorf("empty auditor should return empty log, got %q", log)
	}
}

func TestMissingVars(t *testing.T) {
	a := NewEnvAuditor()

	t.Setenv("PRESENT_VAR", "ok")
	a.Get("PRESENT_VAR", "comp.a")
	a.Get("ABSENT_VAR_AAA_12345", "comp.a")
	a.Get("ABSENT_VAR_BBB_12345", "comp.b")
	// Access same missing var again — should be deduplicated
	a.Get("ABSENT_VAR_AAA_12345", "comp.c")

	missing := a.MissingVars()
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing vars, got %d: %v", len(missing), missing)
	}
	// Should be sorted
	if missing[0] != "ABSENT_VAR_AAA_12345" {
		t.Errorf("missing[0]: got %q, want %q", missing[0], "ABSENT_VAR_AAA_12345")
	}
	if missing[1] != "ABSENT_VAR_BBB_12345" {
		t.Errorf("missing[1]: got %q, want %q", missing[1], "ABSENT_VAR_BBB_12345")
	}
}

func TestMissingVarsEmpty(t *testing.T) {
	a := NewEnvAuditor()
	missing := a.MissingVars()
	if len(missing) != 0 {
		t.Errorf("expected 0 missing vars, got %d", len(missing))
	}
}

func TestMissingVarsAllPresent(t *testing.T) {
	a := NewEnvAuditor()
	t.Setenv("ALL_PRESENT_A", "x")
	t.Setenv("ALL_PRESENT_B", "y")
	a.Get("ALL_PRESENT_A", "comp")
	a.Get("ALL_PRESENT_B", "comp")

	missing := a.MissingVars()
	if len(missing) != 0 {
		t.Errorf("expected 0 missing vars when all present, got %v", missing)
	}
}

func TestComponents(t *testing.T) {
	a := NewEnvAuditor()

	t.Setenv("COMP_TEST_VAR", "v")
	a.Get("COMP_TEST_VAR", "provider.claude")
	a.Get("COMP_TEST_VAR", "session.runner")
	a.Get("COMP_TEST_VAR", "provider.claude") // duplicate

	comps := a.Components()
	if len(comps) != 2 {
		t.Fatalf("expected 2 components, got %d: %v", len(comps), comps)
	}
	if comps[0] != "provider.claude" {
		t.Errorf("comps[0]: got %q, want %q", comps[0], "provider.claude")
	}
	if comps[1] != "session.runner" {
		t.Errorf("comps[1]: got %q, want %q", comps[1], "session.runner")
	}
}

func TestComponentsEmpty(t *testing.T) {
	a := NewEnvAuditor()
	comps := a.Components()
	if len(comps) != 0 {
		t.Errorf("expected 0 components, got %d", len(comps))
	}
}

func TestReset(t *testing.T) {
	a := NewEnvAuditor()
	t.Setenv("RESET_VAR", "v")
	a.Get("RESET_VAR", "comp")
	a.Get("RESET_VAR", "comp")

	if len(a.Entries()) != 2 {
		t.Fatal("expected 2 entries before reset")
	}

	a.Reset()

	if len(a.Entries()) != 0 {
		t.Errorf("expected 0 entries after reset, got %d", len(a.Entries()))
	}
	if a.AuditLog() != "" {
		t.Error("audit log should be empty after reset")
	}
	if len(a.MissingVars()) != 0 {
		t.Error("missing vars should be empty after reset")
	}
	if len(a.Components()) != 0 {
		t.Error("components should be empty after reset")
	}
}

func TestConcurrentAccess(t *testing.T) {
	a := NewEnvAuditor()
	t.Setenv("CONCURRENT_VAR", "v")

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			a.Get("CONCURRENT_VAR", "goroutine")
			a.Get("CONCURRENT_MISSING_FOR_TEST_77777", "goroutine")
			_ = a.AuditLog()
			_ = a.Entries()
			_ = a.MissingVars()
			_ = a.Components()
		}(i)
	}
	wg.Wait()

	entries := a.Entries()
	if len(entries) != 100 {
		t.Errorf("expected 100 entries from 50 goroutines x 2 calls, got %d", len(entries))
	}
}

func TestEntriesReturnsCopy(t *testing.T) {
	a := NewEnvAuditor()
	t.Setenv("COPY_VAR", "v")
	a.Get("COPY_VAR", "comp")

	entries := a.Entries()
	entries[0].VarName = "MUTATED"

	// Original should be unaffected
	original := a.Entries()
	if original[0].VarName != "COPY_VAR" {
		t.Error("Entries should return a defensive copy")
	}
}

func TestMultipleVarsMultipleComponents(t *testing.T) {
	a := NewEnvAuditor()

	t.Setenv("CLAUDE_API_KEY", "sk-xxx")
	t.Setenv("GEMINI_API_KEY", "gm-xxx")

	a.Get("CLAUDE_API_KEY", "provider.claude")
	a.Get("GEMINI_API_KEY", "provider.gemini")
	a.Get("OPENAI_API_KEY_NONEXISTENT_TEST", "provider.openai")

	entries := a.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Verify chronological order
	for i := 1; i < len(entries); i++ {
		if entries[i].Timestamp.Before(entries[i-1].Timestamp) {
			t.Errorf("entries not in chronological order at index %d", i)
		}
	}

	missing := a.MissingVars()
	if len(missing) != 1 || missing[0] != "OPENAI_API_KEY_NONEXISTENT_TEST" {
		t.Errorf("expected [OPENAI_API_KEY_NONEXISTENT_TEST], got %v", missing)
	}

	comps := a.Components()
	if len(comps) != 3 {
		t.Errorf("expected 3 components, got %d: %v", len(comps), comps)
	}
}

func TestAuditLogChronologicalOrder(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	callCount := 0
	a := newEnvAuditorWithClock(func() time.Time {
		callCount++
		return ts.Add(time.Duration(callCount) * time.Minute)
	})

	t.Setenv("ORDER_A", "a")
	t.Setenv("ORDER_B", "b")
	t.Setenv("ORDER_C", "c")

	a.Get("ORDER_A", "first")
	a.Get("ORDER_B", "second")
	a.Get("ORDER_C", "third")

	log := a.AuditLog()
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "ORDER_A") {
		t.Errorf("first line should contain ORDER_A: %s", lines[0])
	}
	if !strings.Contains(lines[1], "ORDER_B") {
		t.Errorf("second line should contain ORDER_B: %s", lines[1])
	}
	if !strings.Contains(lines[2], "ORDER_C") {
		t.Errorf("third line should contain ORDER_C: %s", lines[2])
	}
}
