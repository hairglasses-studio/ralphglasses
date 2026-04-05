package telemetry

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSpanTree_StartAndEnd(t *testing.T) {
	tree := NewSpanTree()

	id := tree.StartSpan("", "root-op")
	if id == "" {
		t.Fatal("StartSpan returned empty ID")
	}

	s := tree.Span(id)
	if s == nil {
		t.Fatal("Span returned nil for valid ID")
	}
	if s.Name != "root-op" {
		t.Errorf("Name = %q, want %q", s.Name, "root-op")
	}
	if s.Status != SpanStatusRunning {
		t.Errorf("Status = %q, want %q", s.Status, SpanStatusRunning)
	}
	if s.ParentID != "" {
		t.Errorf("ParentID = %q, want empty", s.ParentID)
	}

	ok := tree.EndSpan(id, SpanStatusOK)
	if !ok {
		t.Fatal("EndSpan returned false for valid ID")
	}

	s = tree.Span(id)
	if s.Status != SpanStatusOK {
		t.Errorf("Status after end = %q, want %q", s.Status, SpanStatusOK)
	}
	if s.Duration <= 0 {
		t.Error("Duration should be positive after EndSpan")
	}
	if s.EndTime.IsZero() {
		t.Error("EndTime should be set after EndSpan")
	}
}

func TestSpanTree_EndSpanDefaultStatus(t *testing.T) {
	tree := NewSpanTree()
	id := tree.StartSpan("", "op")
	ok := tree.EndSpan(id, "")
	if !ok {
		t.Fatal("EndSpan returned false")
	}
	s := tree.Span(id)
	if s.Status != SpanStatusOK {
		t.Errorf("Status = %q, want %q (default)", s.Status, SpanStatusOK)
	}
}

func TestSpanTree_EndSpanNotFound(t *testing.T) {
	tree := NewSpanTree()
	ok := tree.EndSpan("nonexistent", SpanStatusOK)
	if ok {
		t.Error("EndSpan should return false for unknown ID")
	}
}

func TestSpanTree_SpanNotFound(t *testing.T) {
	tree := NewSpanTree()
	if s := tree.Span("no-such-id"); s != nil {
		t.Error("Span should return nil for unknown ID")
	}
}

func TestSpanTree_SetAttribute(t *testing.T) {
	tree := NewSpanTree()
	id := tree.StartSpan("", "op")

	ok := tree.SetAttribute(id, "key", "value")
	if !ok {
		t.Fatal("SetAttribute returned false")
	}

	s := tree.Span(id)
	if s.Attributes["key"] != "value" {
		t.Errorf("Attributes[key] = %q, want %q", s.Attributes["key"], "value")
	}

	ok = tree.SetAttribute("missing", "k", "v")
	if ok {
		t.Error("SetAttribute should return false for unknown ID")
	}
}

func TestSpanTree_NestedSpans(t *testing.T) {
	tree := NewSpanTree()

	root := tree.StartSpan("", "request")
	child1 := tree.StartSpan(root, "auth")
	child2 := tree.StartSpan(root, "query")
	grandchild := tree.StartSpan(child2, "fetch-rows")

	if tree.Len() != 4 {
		t.Errorf("Len() = %d, want 4", tree.Len())
	}

	// verify parent IDs
	if s := tree.Span(child1); s.ParentID != root {
		t.Errorf("child1 ParentID = %q, want %q", s.ParentID, root)
	}
	if s := tree.Span(grandchild); s.ParentID != child2 {
		t.Errorf("grandchild ParentID = %q, want %q", s.ParentID, child2)
	}

	tree.EndSpan(grandchild, SpanStatusOK)
	tree.EndSpan(child1, SpanStatusOK)
	tree.EndSpan(child2, SpanStatusError)
	tree.EndSpan(root, SpanStatusOK)

	// Render tree should include all four spans nested
	rendered := tree.RenderTree()
	if !strings.Contains(rendered, "request") {
		t.Error("RenderTree missing root span 'request'")
	}
	if !strings.Contains(rendered, "auth") {
		t.Error("RenderTree missing child span 'auth'")
	}
	if !strings.Contains(rendered, "query") {
		t.Error("RenderTree missing child span 'query'")
	}
	if !strings.Contains(rendered, "fetch-rows") {
		t.Error("RenderTree missing grandchild span 'fetch-rows'")
	}
	if !strings.Contains(rendered, "[error]") {
		t.Error("RenderTree missing [error] status for 'query'")
	}
}

func TestSpanTree_FlatList(t *testing.T) {
	tree := NewSpanTree()

	// Start spans with small time gaps so ordering is deterministic.
	id1 := tree.StartSpan("", "first")
	time.Sleep(time.Millisecond)
	id2 := tree.StartSpan(id1, "second")
	time.Sleep(time.Millisecond)
	id3 := tree.StartSpan(id1, "third")

	tree.EndSpan(id2, SpanStatusOK)
	tree.EndSpan(id3, SpanStatusOK)
	tree.EndSpan(id1, SpanStatusOK)

	flat := tree.FlatList()
	if len(flat) != 3 {
		t.Fatalf("FlatList len = %d, want 3", len(flat))
	}
	if flat[0].Name != "first" {
		t.Errorf("flat[0].Name = %q, want %q", flat[0].Name, "first")
	}
	if flat[1].Name != "second" {
		t.Errorf("flat[1].Name = %q, want %q", flat[1].Name, "second")
	}
	if flat[2].Name != "third" {
		t.Errorf("flat[2].Name = %q, want %q", flat[2].Name, "third")
	}
}

func TestSpanTree_RenderTreeEmpty(t *testing.T) {
	tree := NewSpanTree()
	out := tree.RenderTree()
	if out != "(empty)" {
		t.Errorf("RenderTree on empty tree = %q, want %q", out, "(empty)")
	}
}

func TestSpanTree_RenderTreeASCII(t *testing.T) {
	tree := NewSpanTree()

	root := tree.StartSpan("", "deploy")
	c1 := tree.StartSpan(root, "build")
	tree.StartSpan(root, "test")

	tree.EndSpan(c1, SpanStatusOK)

	out := tree.RenderTree()
	lines := strings.Split(out, "\n")

	// Root should be the first line and not indented with a connector.
	if !strings.HasPrefix(lines[0], " deploy") {
		t.Errorf("first line = %q, expected root span without connector prefix", lines[0])
	}

	// At least 3 lines for 3 spans.
	if len(lines) < 3 {
		t.Errorf("expected at least 3 lines, got %d:\n%s", len(lines), out)
	}

	// Completed span should show duration.
	found := false
	for _, line := range lines {
		if strings.Contains(line, "build") && strings.Contains(line, "[ok]") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'build' span with [ok] status in output:\n%s", out)
	}
}

func TestSpanTree_RenderJSON(t *testing.T) {
	tree := NewSpanTree()

	root := tree.StartSpan("", "api-call")
	child := tree.StartSpan(root, "db-query")
	tree.SetAttribute(child, "table", "users")
	tree.EndSpan(child, SpanStatusOK)
	tree.EndSpan(root, SpanStatusOK)

	data, err := tree.RenderJSON()
	if err != nil {
		t.Fatalf("RenderJSON error: %v", err)
	}

	// Parse back and verify structure.
	var parsed []json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 root in JSON, got %d", len(parsed))
	}

	// Verify the root has a children array.
	var rootObj map[string]json.RawMessage
	if err := json.Unmarshal(parsed[0], &rootObj); err != nil {
		t.Fatalf("unmarshal root: %v", err)
	}
	if _, ok := rootObj["children"]; !ok {
		t.Error("root JSON missing 'children' key")
	}

	var children []map[string]any
	if err := json.Unmarshal(rootObj["children"], &children); err != nil {
		t.Fatalf("unmarshal children: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0]["name"] != "db-query" {
		t.Errorf("child name = %v, want %q", children[0]["name"], "db-query")
	}
	attrs, ok := children[0]["attributes"].(map[string]any)
	if !ok {
		t.Fatal("child attributes not a map")
	}
	if attrs["table"] != "users" {
		t.Errorf("child attribute 'table' = %v, want %q", attrs["table"], "users")
	}
}

func TestSpanTree_OrphanSpan(t *testing.T) {
	tree := NewSpanTree()

	// Span with a parent ID that does not exist should be treated as root.
	id := tree.StartSpan("nonexistent-parent", "orphan")
	tree.EndSpan(id, SpanStatusOK)

	out := tree.RenderTree()
	if !strings.Contains(out, "orphan") {
		t.Errorf("orphan span not found in RenderTree output:\n%s", out)
	}
}

func TestSpanTree_MultipleRoots(t *testing.T) {
	tree := NewSpanTree()

	r1 := tree.StartSpan("", "root-a")
	r2 := tree.StartSpan("", "root-b")
	tree.EndSpan(r1, SpanStatusOK)
	tree.EndSpan(r2, SpanStatusError)

	out := tree.RenderTree()
	if !strings.Contains(out, "root-a") || !strings.Contains(out, "root-b") {
		t.Errorf("expected both roots in output:\n%s", out)
	}
}

func TestSpanTree_ConcurrentUsage(t *testing.T) {
	tree := NewSpanTree()
	const goroutines = 50
	const spansPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			root := tree.StartSpan("", "concurrent-root")
			for range spansPerGoroutine {
				child := tree.StartSpan(root, "child")
				tree.SetAttribute(child, "idx", "val")
				tree.EndSpan(child, SpanStatusOK)
			}
			tree.EndSpan(root, SpanStatusOK)
		}()
	}

	wg.Wait()

	expected := goroutines * (1 + spansPerGoroutine) // roots + children
	if tree.Len() != expected {
		t.Errorf("Len() = %d, want %d", tree.Len(), expected)
	}

	// All render methods should succeed without panic.
	_ = tree.RenderTree()
	if _, err := tree.RenderJSON(); err != nil {
		t.Errorf("RenderJSON after concurrent writes: %v", err)
	}
	flat := tree.FlatList()
	if len(flat) != expected {
		t.Errorf("FlatList len = %d, want %d", len(flat), expected)
	}
}

func TestSpanTree_ConcurrentReadWrite(t *testing.T) {
	tree := NewSpanTree()
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(3)

	// Writer goroutine
	go func() {
		defer wg.Done()
		for range iterations {
			id := tree.StartSpan("", "writer-span")
			tree.SetAttribute(id, "i", "v")
			tree.EndSpan(id, SpanStatusOK)
		}
	}()

	// Reader goroutine: RenderTree
	go func() {
		defer wg.Done()
		for range iterations {
			_ = tree.RenderTree()
		}
	}()

	// Reader goroutine: FlatList + RenderJSON
	go func() {
		defer wg.Done()
		for range iterations {
			_ = tree.FlatList()
			_, _ = tree.RenderJSON()
		}
	}()

	wg.Wait()

	// No panics or data races is the success condition.
	if tree.Len() != iterations {
		t.Errorf("Len() = %d, want %d", tree.Len(), iterations)
	}
}

func TestSpanTree_UniqueIDs(t *testing.T) {
	tree := NewSpanTree()
	ids := make(map[string]bool)

	for range 100 {
		id := tree.StartSpan("", "span")
		if ids[id] {
			t.Fatalf("duplicate span ID: %s", id)
		}
		ids[id] = true
	}
}

func TestSpanTree_SpanCopyIsolation(t *testing.T) {
	tree := NewSpanTree()
	id := tree.StartSpan("", "op")
	tree.SetAttribute(id, "original", "yes")

	// Get a copy and mutate it.
	cp := tree.Span(id)
	cp.Attributes["mutated"] = "true"
	cp.Name = "changed"

	// Original should be unaffected.
	s := tree.Span(id)
	if s.Name != "op" {
		t.Errorf("original Name mutated to %q", s.Name)
	}
	if _, ok := s.Attributes["mutated"]; ok {
		t.Error("original Attributes leaked mutation from copy")
	}
}
