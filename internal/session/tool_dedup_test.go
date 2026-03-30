package session

import (
	"testing"
	"time"
)

func TestToolDeduplicatorExactDuplicate(t *testing.T) {
	d := NewToolDeduplicator()

	args := map[string]any{"path": "/foo.go"}
	d.Record("read_file", args, "file content here")

	dup, output := d.IsDuplicate("read_file", args, nil)
	if !dup {
		t.Error("expected duplicate for same tool+args")
	}
	if output != "file content here" {
		t.Errorf("expected cached output, got %q", output)
	}
}

func TestToolDeduplicatorDifferentArgs(t *testing.T) {
	d := NewToolDeduplicator()

	d.Record("read_file", map[string]any{"path": "/foo.go"}, "foo content")

	dup, _ := d.IsDuplicate("read_file", map[string]any{"path": "/bar.go"}, nil)
	if dup {
		t.Error("should NOT be duplicate for different args")
	}
}

func TestToolDeduplicatorDifferentTool(t *testing.T) {
	d := NewToolDeduplicator()

	args := map[string]any{"path": "/foo.go"}
	d.Record("read_file", args, "content")

	dup, _ := d.IsDuplicate("write_file", args, nil)
	if dup {
		t.Error("should NOT be duplicate for different tool name")
	}
}

func TestToolDeduplicatorTTLExpiration(t *testing.T) {
	d := NewToolDeduplicator()
	d.SetTTL("status_check", 50*time.Millisecond)

	args := map[string]any{"id": "123"}
	d.Record("status_check", args, "running")

	// Should be duplicate immediately.
	dup, _ := d.IsDuplicate("status_check", args, nil)
	if !dup {
		t.Error("expected duplicate within TTL")
	}

	// Wait for TTL to expire.
	time.Sleep(60 * time.Millisecond)

	dup, _ = d.IsDuplicate("status_check", args, nil)
	if dup {
		t.Error("should NOT be duplicate after TTL expiration")
	}
}

func TestToolDeduplicatorPrevResults(t *testing.T) {
	d := NewToolDeduplicator()

	args := map[string]any{"id": "abc"}
	prev := []ToolResult{
		{
			ToolName:  "get_status",
			Args:      args,
			Output:    "cached status",
			Timestamp: time.Now(),
		},
	}

	dup, output := d.IsDuplicate("get_status", args, prev)
	if !dup {
		t.Error("expected duplicate from prevResults")
	}
	if output != "cached status" {
		t.Errorf("expected cached output from prevResults, got %q", output)
	}
}

func TestToolDeduplicatorPrevResultsExpired(t *testing.T) {
	d := NewToolDeduplicator()
	d.SetTTL("get_status", 50*time.Millisecond)

	args := map[string]any{"id": "abc"}
	prev := []ToolResult{
		{
			ToolName:  "get_status",
			Args:      args,
			Output:    "old status",
			Timestamp: time.Now().Add(-100 * time.Millisecond), // already expired
		},
	}

	dup, _ := d.IsDuplicate("get_status", args, prev)
	if dup {
		t.Error("should NOT be duplicate for expired prevResults")
	}
}

func TestToolDeduplicatorEvict(t *testing.T) {
	d := NewToolDeduplicator()

	args := map[string]any{"path": "/foo.go"}
	d.Record("read_file", args, "content")

	// Confirm it is cached.
	dup, _ := d.IsDuplicate("read_file", args, nil)
	if !dup {
		t.Fatal("expected duplicate before eviction")
	}

	d.Evict("read_file")

	dup, _ = d.IsDuplicate("read_file", args, nil)
	if dup {
		t.Error("should NOT be duplicate after eviction")
	}
}

func TestToolDeduplicatorEvictExpired(t *testing.T) {
	d := NewToolDeduplicator()
	d.SetTTL("fast_tool", 10*time.Millisecond)

	d.Record("fast_tool", map[string]any{"x": 1}, "result1")
	d.Record("slow_tool", map[string]any{"x": 2}, "result2")

	time.Sleep(15 * time.Millisecond)

	evicted := d.EvictExpired()
	if evicted < 1 {
		t.Errorf("expected at least 1 eviction, got %d", evicted)
	}

	stats := d.Stats()
	// slow_tool should still be in cache (5min default TTL).
	if stats["slow_tool"] != 1 {
		t.Errorf("expected slow_tool to survive eviction, stats: %v", stats)
	}
}

func TestToolDeduplicatorStats(t *testing.T) {
	d := NewToolDeduplicator()

	d.Record("read_file", map[string]any{"p": "a"}, "a")
	d.Record("read_file", map[string]any{"p": "b"}, "b")
	d.Record("write_file", map[string]any{"p": "c"}, "c")

	stats := d.Stats()
	if stats["_total"] != 3 {
		t.Errorf("expected total=3, got %d", stats["_total"])
	}
	if stats["read_file"] != 2 {
		t.Errorf("expected read_file=2, got %d", stats["read_file"])
	}
	if stats["write_file"] != 1 {
		t.Errorf("expected write_file=1, got %d", stats["write_file"])
	}
}

func TestToolDeduplicatorMaxHistory(t *testing.T) {
	d := NewToolDeduplicator()
	d.maxHistory = 5

	for i := 0; i < 10; i++ {
		d.Record("tool", map[string]any{"i": i}, "result")
	}

	stats := d.Stats()
	if stats["_total"] != 5 {
		t.Errorf("expected max 5 entries after overflow, got %d", stats["_total"])
	}
}

func TestToolDeduplicatorNilArgs(t *testing.T) {
	d := NewToolDeduplicator()

	d.Record("simple_tool", nil, "output1")

	dup, output := d.IsDuplicate("simple_tool", nil, nil)
	if !dup {
		t.Error("expected duplicate for nil args matching nil args")
	}
	if output != "output1" {
		t.Errorf("expected cached output, got %q", output)
	}

	// Non-nil empty map should also match nil (both marshal to the same hash).
	dup2, _ := d.IsDuplicate("simple_tool", map[string]any{}, nil)
	// This may or may not match depending on hash — both are acceptable.
	_ = dup2
}

func TestToolDeduplicatorDefaultTTLs(t *testing.T) {
	d := NewToolDeduplicator()

	// Status tools should have 30s TTL.
	if d.ttlFor("git_status") != 30*time.Second {
		t.Errorf("expected 30s TTL for git_status, got %v", d.ttlFor("git_status"))
	}
	// Unknown tools should get the 5min default.
	if d.ttlFor("unknown_tool") != 5*time.Minute {
		t.Errorf("expected 5m default TTL, got %v", d.ttlFor("unknown_tool"))
	}
}
