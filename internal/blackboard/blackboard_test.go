package blackboard

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPutAndGet(t *testing.T) {
	bb := NewBlackboard("")

	err := bb.Put(Entry{
		Key:       "task-1",
		Namespace: "tasks",
		Value:     map[string]any{"status": "running"},
		WriterID:  "worker-a",
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	e, ok := bb.Get("tasks", "task-1")
	if !ok {
		t.Fatal("expected entry to exist")
	}
	if e.Version != 1 {
		t.Fatalf("expected version 1, got %d", e.Version)
	}
	if e.Value["status"] != "running" {
		t.Fatalf("expected status=running, got %v", e.Value["status"])
	}
	if e.WriterID != "worker-a" {
		t.Fatalf("expected writer-id=worker-a, got %s", e.WriterID)
	}
	if bb.Len() != 1 {
		t.Fatalf("expected len 1, got %d", bb.Len())
	}
}

func TestVersionConflict(t *testing.T) {
	bb := NewBlackboard("")

	// Initial write (Version=0 → unconditional).
	_ = bb.Put(Entry{Key: "k", Namespace: "ns", Value: map[string]any{"v": 1}, WriterID: "w1"})

	// CAS with correct version succeeds.
	err := bb.Put(Entry{Key: "k", Namespace: "ns", Value: map[string]any{"v": 2}, WriterID: "w1", Version: 1})
	if err != nil {
		t.Fatalf("expected CAS success, got: %v", err)
	}

	// CAS with stale version fails.
	err = bb.Put(Entry{Key: "k", Namespace: "ns", Value: map[string]any{"v": 3}, WriterID: "w2", Version: 1})
	if err != ErrVersionConflict {
		t.Fatalf("expected ErrVersionConflict, got: %v", err)
	}

	// CAS against non-existent key also fails.
	err = bb.Put(Entry{Key: "missing", Namespace: "ns", Value: map[string]any{}, WriterID: "w1", Version: 1})
	if err != ErrVersionConflict {
		t.Fatalf("expected ErrVersionConflict for missing key, got: %v", err)
	}
}

func TestQuery(t *testing.T) {
	bb := NewBlackboard("")

	_ = bb.Put(Entry{Key: "a", Namespace: "ns1", Value: map[string]any{"x": 1}})
	_ = bb.Put(Entry{Key: "b", Namespace: "ns1", Value: map[string]any{"x": 2}})
	_ = bb.Put(Entry{Key: "c", Namespace: "ns2", Value: map[string]any{"x": 3}})

	results := bb.Query("ns1")
	if len(results) != 2 {
		t.Fatalf("expected 2 entries in ns1, got %d", len(results))
	}

	results = bb.Query("ns2")
	if len(results) != 1 {
		t.Fatalf("expected 1 entry in ns2, got %d", len(results))
	}

	results = bb.Query("ns3")
	if len(results) != 0 {
		t.Fatalf("expected 0 entries in ns3, got %d", len(results))
	}
}

func TestWatch(t *testing.T) {
	bb := NewBlackboard("")

	var mu sync.Mutex
	var received []Entry
	bb.Watch(func(e Entry) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	_ = bb.Put(Entry{Key: "k1", Namespace: "ns", Value: map[string]any{"a": 1}})
	_ = bb.Put(Entry{Key: "k2", Namespace: "ns", Value: map[string]any{"b": 2}})

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(received))
	}
	if received[0].Key != "k1" || received[1].Key != "k2" {
		t.Fatalf("unexpected notification order: %s, %s", received[0].Key, received[1].Key)
	}
}

func TestGC(t *testing.T) {
	bb := NewBlackboard("")

	// Entry with very short TTL.
	_ = bb.Put(Entry{
		Key: "ephemeral", Namespace: "ns",
		Value: map[string]any{}, TTL: time.Millisecond,
	})
	// Entry with no TTL (permanent).
	_ = bb.Put(Entry{
		Key: "permanent", Namespace: "ns",
		Value: map[string]any{},
	})

	if bb.Len() != 2 {
		t.Fatalf("expected 2 entries before GC, got %d", bb.Len())
	}

	// Wait for TTL to expire.
	time.Sleep(5 * time.Millisecond)

	bb.GC()

	if bb.Len() != 1 {
		t.Fatalf("expected 1 entry after GC, got %d", bb.Len())
	}
	if _, ok := bb.Get("ns", "permanent"); !ok {
		t.Fatal("permanent entry should survive GC")
	}
	if _, ok := bb.Get("ns", "ephemeral"); ok {
		t.Fatal("ephemeral entry should be GC'd")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()

	// Write entries with first blackboard.
	bb1 := NewBlackboard(dir)
	_ = bb1.Put(Entry{Key: "k1", Namespace: "ns", Value: map[string]any{"v": 1}, WriterID: "w1"})
	_ = bb1.Put(Entry{Key: "k2", Namespace: "ns", Value: map[string]any{"v": 2}, WriterID: "w2"})

	// Reload into a fresh blackboard.
	bb2 := NewBlackboard(dir)
	if bb2.Len() != 2 {
		t.Fatalf("expected 2 entries after reload, got %d", bb2.Len())
	}

	e, ok := bb2.Get("ns", "k1")
	if !ok {
		t.Fatal("expected k1 to exist after reload")
	}
	if e.Version != 1 {
		t.Fatalf("expected version 1, got %d", e.Version)
	}
	if e.Value["v"] != float64(1) { // JSON numbers unmarshal as float64
		t.Fatalf("expected value 1, got %v", e.Value["v"])
	}
}

func TestSnapshot(t *testing.T) {
	dir := t.TempDir()

	bb1 := NewBlackboard(dir)
	// Write multiple versions of the same key (appends multiple lines).
	_ = bb1.Put(Entry{Key: "k", Namespace: "ns", Value: map[string]any{"v": 1}})
	_ = bb1.Put(Entry{Key: "k", Namespace: "ns", Value: map[string]any{"v": 2}})
	_ = bb1.Put(Entry{Key: "k", Namespace: "ns", Value: map[string]any{"v": 3}})

	// File should have 3 lines before snapshot.
	data, _ := os.ReadFile(filepath.Join(dir, "blackboard.jsonl"))
	linesBefore := countLines(data)
	if linesBefore != 3 {
		t.Fatalf("expected 3 lines before snapshot, got %d", linesBefore)
	}

	// Compact.
	if err := bb1.Snapshot(); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// File should have 1 line after snapshot (only latest state).
	data, _ = os.ReadFile(filepath.Join(dir, "blackboard.jsonl"))
	linesAfter := countLines(data)
	if linesAfter != 1 {
		t.Fatalf("expected 1 line after snapshot, got %d", linesAfter)
	}

	// Reload and verify latest value survived.
	bb2 := NewBlackboard(dir)
	e, ok := bb2.Get("ns", "k")
	if !ok {
		t.Fatal("expected entry after snapshot reload")
	}
	if e.Value["v"] != float64(3) {
		t.Fatalf("expected value 3 after snapshot reload, got %v", e.Value["v"])
	}
	if e.Version != 3 {
		t.Fatalf("expected version 3, got %d", e.Version)
	}
}

func TestBlackboardCorruptedJSONL(t *testing.T) {
	dir := t.TempDir()

	// Write a mix of valid and corrupted lines to the JSONL file.
	path := filepath.Join(dir, "blackboard.jsonl")
	content := `{"key":"good1","namespace":"ns","value":{"x":1},"writer_id":"w1","version":1,"ttl":0,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}
this is not json at all
{"key":"good2","namespace":"ns","value":{"x":2},"writer_id":"w2","version":1,"ttl":0,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}
{truncated json
`
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Should load without crashing, skipping bad lines.
	bb := NewBlackboard(dir)

	if bb.Len() != 2 {
		t.Fatalf("expected 2 valid entries from corrupted file, got %d", bb.Len())
	}

	e1, ok := bb.Get("ns", "good1")
	if !ok {
		t.Fatal("expected good1 to be loaded")
	}
	if e1.Value["x"] != float64(1) {
		t.Fatalf("expected good1 value x=1, got %v", e1.Value["x"])
	}

	e2, ok := bb.Get("ns", "good2")
	if !ok {
		t.Fatal("expected good2 to be loaded")
	}
	if e2.Value["x"] != float64(2) {
		t.Fatalf("expected good2 value x=2, got %v", e2.Value["x"])
	}
}

func TestBlackboardPutNilValue(t *testing.T) {
	bb := NewBlackboard("")

	// Put with nil Value map should not panic.
	err := bb.Put(Entry{
		Key:       "nil-val",
		Namespace: "ns",
		Value:     nil,
		WriterID:  "w1",
	})
	if err != nil {
		t.Fatalf("Put with nil Value: %v", err)
	}

	e, ok := bb.Get("ns", "nil-val")
	if !ok {
		t.Fatal("expected entry to exist")
	}
	if e.Value != nil {
		t.Fatalf("expected nil Value, got %v", e.Value)
	}
	if e.Version != 1 {
		t.Fatalf("expected version 1, got %d", e.Version)
	}
}

func countLines(data []byte) int {
	n := 0
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 {
			n++
		}
	}
	return n
}
