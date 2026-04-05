package session

import (
	"sort"
	"sync"
	"time"
)

// CostLedgerEntry is a single cost record in the ledger.
type CostLedgerEntry struct {
	SessionID string    `json:"session_id"`
	Amount    float64   `json:"amount"`
	Provider  string    `json:"provider"`
	Timestamp time.Time `json:"timestamp"`
}

// CostLedger tracks per-session cost entries with timestamps. It is safe for
// concurrent use.
type CostLedger struct {
	mu          sync.RWMutex
	entries     []CostLedgerEntry
	byID        map[string][]int // sessionID → indices into entries
	eventWriter *CostEventWriter // optional external event emitter
}

// NewCostLedger creates an empty CostLedger ready for use.
func NewCostLedger() *CostLedger {
	return &CostLedger{
		byID: make(map[string][]int),
	}
}

// Record adds a cost entry for the given session. The timestamp is set to
// time.Now() automatically.
func (cl *CostLedger) Record(sessionID string, amount float64, provider string) {
	cl.RecordAt(sessionID, amount, provider, time.Now())
}

// SetEventWriter attaches an optional CostEventWriter. When set, every
// RecordAt call also emits a CostEvent to the writer for external consumption
// (e.g., docs-mcp ingestion). Pass nil to disable.
func (cl *CostLedger) SetEventWriter(w *CostEventWriter) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.eventWriter = w
}

// RecordAt adds a cost entry with an explicit timestamp. This is useful for
// tests and back-filling historical data.
func (cl *CostLedger) RecordAt(sessionID string, amount float64, provider string, ts time.Time) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	idx := len(cl.entries)
	entry := CostLedgerEntry{
		SessionID: sessionID,
		Amount:    amount,
		Provider:  provider,
		Timestamp: ts,
	}
	cl.entries = append(cl.entries, entry)
	cl.byID[sessionID] = append(cl.byID[sessionID], idx)

	// Emit cost event if a writer is attached (best-effort, errors are silent).
	if cl.eventWriter != nil {
		_ = cl.eventWriter.Write(CostEvent{
			SessionID: sessionID,
			Provider:  provider,
			CostUSD:   amount,
			Timestamp: ts,
		})
	}
}

// Total returns the sum of all recorded costs across every session.
func (cl *CostLedger) Total() float64 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	var sum float64
	for _, e := range cl.entries {
		sum += e.Amount
	}
	return sum
}

// TotalForSession returns the sum of all costs for a specific session.
func (cl *CostLedger) TotalForSession(sessionID string) float64 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	indices := cl.byID[sessionID]
	var sum float64
	for _, i := range indices {
		sum += cl.entries[i].Amount
	}
	return sum
}

// Entries returns a copy of all cost entries for a given session, sorted by
// timestamp ascending.
func (cl *CostLedger) Entries(sessionID string) []CostLedgerEntry {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	indices := cl.byID[sessionID]
	if len(indices) == 0 {
		return nil
	}
	out := make([]CostLedgerEntry, len(indices))
	for i, idx := range indices {
		out[i] = cl.entries[idx]
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}

// AllEntries returns a copy of every cost entry in the ledger, sorted by
// timestamp ascending.
func (cl *CostLedger) AllEntries() []CostLedgerEntry {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	if len(cl.entries) == 0 {
		return nil
	}
	out := make([]CostLedgerEntry, len(cl.entries))
	copy(out, cl.entries)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}

// EntriesSince returns all entries with timestamps at or after the given time.
func (cl *CostLedger) EntriesSince(since time.Time) []CostLedgerEntry {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	var out []CostLedgerEntry
	for _, e := range cl.entries {
		if !e.Timestamp.Before(since) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}
