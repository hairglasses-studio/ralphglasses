package session

import (
	"sync"
	"testing"
	"time"
)

func TestCostLedger_NewIsEmpty(t *testing.T) {
	cl := NewCostLedger()
	if got := cl.Total(); got != 0 {
		t.Fatalf("Total() = %f, want 0", got)
	}
	if got := cl.AllEntries(); got != nil {
		t.Fatalf("AllEntries() = %v, want nil", got)
	}
}

func TestCostLedger_RecordAndTotal(t *testing.T) {
	cl := NewCostLedger()
	cl.Record("s1", 1.50, "claude")
	cl.Record("s1", 0.50, "claude")
	cl.Record("s2", 2.00, "gemini")

	if got := cl.Total(); got != 4.00 {
		t.Fatalf("Total() = %f, want 4.00", got)
	}
}

func TestCostLedger_TotalForSession(t *testing.T) {
	cl := NewCostLedger()
	cl.Record("s1", 1.00, "claude")
	cl.Record("s2", 3.00, "gemini")
	cl.Record("s1", 2.00, "claude")

	if got := cl.TotalForSession("s1"); got != 3.00 {
		t.Fatalf("TotalForSession(s1) = %f, want 3.00", got)
	}
	if got := cl.TotalForSession("s2"); got != 3.00 {
		t.Fatalf("TotalForSession(s2) = %f, want 3.00", got)
	}
	if got := cl.TotalForSession("s3"); got != 0 {
		t.Fatalf("TotalForSession(s3) = %f, want 0", got)
	}
}

func TestCostLedger_Entries(t *testing.T) {
	cl := NewCostLedger()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cl.RecordAt("s1", 1.00, "claude", t0.Add(2*time.Minute))
	cl.RecordAt("s1", 0.50, "claude", t0)
	cl.RecordAt("s2", 9.00, "gemini", t0.Add(time.Minute))

	entries := cl.Entries("s1")
	if len(entries) != 2 {
		t.Fatalf("len(Entries) = %d, want 2", len(entries))
	}
	// Should be sorted by timestamp ascending.
	if !entries[0].Timestamp.Before(entries[1].Timestamp) {
		t.Fatal("entries not sorted ascending by timestamp")
	}
	if entries[0].Amount != 0.50 {
		t.Fatalf("first entry amount = %f, want 0.50", entries[0].Amount)
	}
}

func TestCostLedger_EntriesEmpty(t *testing.T) {
	cl := NewCostLedger()
	if got := cl.Entries("nope"); got != nil {
		t.Fatalf("Entries(nope) = %v, want nil", got)
	}
}

func TestCostLedger_AllEntries(t *testing.T) {
	cl := NewCostLedger()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cl.RecordAt("s2", 2.00, "gemini", t0.Add(time.Hour))
	cl.RecordAt("s1", 1.00, "claude", t0)

	all := cl.AllEntries()
	if len(all) != 2 {
		t.Fatalf("len(AllEntries) = %d, want 2", len(all))
	}
	if all[0].SessionID != "s1" {
		t.Fatalf("first entry session = %s, want s1", all[0].SessionID)
	}
}

func TestCostLedger_EntriesSince(t *testing.T) {
	cl := NewCostLedger()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cl.RecordAt("s1", 1.00, "claude", t0)
	cl.RecordAt("s1", 2.00, "claude", t0.Add(time.Hour))
	cl.RecordAt("s1", 3.00, "claude", t0.Add(2*time.Hour))

	got := cl.EntriesSince(t0.Add(time.Hour))
	if len(got) != 2 {
		t.Fatalf("len(EntriesSince) = %d, want 2", len(got))
	}
}

func TestCostLedger_ReturnedSlicesAreIsolated(t *testing.T) {
	cl := NewCostLedger()
	cl.Record("s1", 1.00, "claude")

	entries := cl.Entries("s1")
	entries[0].Amount = 999
	if cl.TotalForSession("s1") != 1.00 {
		t.Fatal("modifying returned slice should not affect ledger")
	}
}

func TestCostLedger_ConcurrentAccess(t *testing.T) {
	cl := NewCostLedger()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cl.Record("s1", 0.01, "claude")
			_ = cl.Total()
			_ = cl.TotalForSession("s1")
			_ = cl.Entries("s1")
			_ = cl.AllEntries()
		}(i)
	}
	wg.Wait()

	if got := cl.Total(); got < 0.99 {
		t.Fatalf("Total() = %f, want >= 1.00 (100 x 0.01)", got)
	}
}
