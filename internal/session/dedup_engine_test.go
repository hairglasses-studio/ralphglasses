package session

import (
	"sync"
	"testing"
)

func TestDedupEngine_BasicFlow(t *testing.T) {
	t.Parallel()
	e := NewDedupEngine()

	fp := TaskFingerprint("fix bug in parser")

	// Not yet recorded.
	if e.IsDuplicate(fp) {
		t.Fatal("expected fingerprint to not be duplicate before recording")
	}

	// Record it.
	e.Record(fp, "session-1", "success")

	if !e.IsDuplicate(fp) {
		t.Fatal("expected fingerprint to be duplicate after recording")
	}

	// History should have one entry.
	h := e.History(fp)
	if len(h) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(h))
	}
	if h[0].SessionID != "session-1" || h[0].Result != "success" {
		t.Fatalf("unexpected history entry: %+v", h[0])
	}
}

func TestDedupEngine_MultipleRecords(t *testing.T) {
	t.Parallel()
	e := NewDedupEngine()

	fp := TaskFingerprint("run linter")
	e.Record(fp, "s1", "fail")
	e.Record(fp, "s2", "pass")

	h := e.History(fp)
	if len(h) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(h))
	}
	if h[0].Result != "fail" || h[1].Result != "pass" {
		t.Fatalf("unexpected history order: %+v, %+v", h[0], h[1])
	}
}

func TestDedupEngine_HistoryIsolation(t *testing.T) {
	t.Parallel()
	e := NewDedupEngine()

	fp := TaskFingerprint("task-x")
	e.Record(fp, "s1", "ok")

	// Mutating returned slice should not affect engine internals.
	h := e.History(fp)
	h[0].Result = "mutated"

	h2 := e.History(fp)
	if h2[0].Result != "ok" {
		t.Fatal("History should return a copy, not a reference to internal state")
	}
}

func TestDedupEngine_DifferentFingerprints(t *testing.T) {
	t.Parallel()
	e := NewDedupEngine()

	fp1 := TaskFingerprint("task one")
	fp2 := TaskFingerprint("task two")

	if fp1 == fp2 {
		t.Fatal("different content should produce different fingerprints")
	}

	e.Record(fp1, "s1", "done")

	if !e.IsDuplicate(fp1) {
		t.Fatal("fp1 should be duplicate")
	}
	if e.IsDuplicate(fp2) {
		t.Fatal("fp2 should not be duplicate")
	}
}

func TestDedupEngine_HistoryNil(t *testing.T) {
	t.Parallel()
	e := NewDedupEngine()

	h := e.History("nonexistent")
	if h != nil {
		t.Fatalf("expected nil history for unknown fingerprint, got %v", h)
	}
}

func TestDedupEngine_SizeAndClear(t *testing.T) {
	t.Parallel()
	e := NewDedupEngine()

	e.Record(TaskFingerprint("a"), "s1", "ok")
	e.Record(TaskFingerprint("b"), "s1", "ok")

	if e.Size() != 2 {
		t.Fatalf("expected size 2, got %d", e.Size())
	}

	e.Clear()
	if e.Size() != 0 {
		t.Fatalf("expected size 0 after clear, got %d", e.Size())
	}
}

func TestDedupEngine_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	e := NewDedupEngine()

	const goroutines = 100
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fp := TaskFingerprint("shared-task")
			_ = e.IsDuplicate(fp)
			e.Record(fp, "session", "ok")
			_ = e.History(fp)
		}(i)
	}
	wg.Wait()

	fp := TaskFingerprint("shared-task")
	h := e.History(fp)
	if len(h) != goroutines {
		t.Fatalf("expected %d history entries, got %d", goroutines, len(h))
	}
}

func TestTaskFingerprint_Deterministic(t *testing.T) {
	t.Parallel()

	fp1 := TaskFingerprint("hello world")
	fp2 := TaskFingerprint("hello world")
	if fp1 != fp2 {
		t.Fatal("same content should produce same fingerprint")
	}

	fp3 := TaskFingerprint("hello  world")
	if fp1 == fp3 {
		t.Fatal("different content should produce different fingerprint")
	}
}
