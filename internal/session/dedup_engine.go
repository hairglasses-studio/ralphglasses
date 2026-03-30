package session

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// DedupRecord captures when a task fingerprint was seen, which session
// processed it, and what the result was.
type DedupRecord struct {
	Fingerprint string    `json:"fingerprint"`
	SessionID   string    `json:"session_id"`
	Result      string    `json:"result"`
	RecordedAt  time.Time `json:"recorded_at"`
}

// DedupEngine prevents duplicate work across sessions by tracking content-
// hashed task fingerprints. It is safe for concurrent use.
type DedupEngine struct {
	mu      sync.Mutex
	records map[string][]DedupRecord // fingerprint -> history
}

// NewDedupEngine creates a ready-to-use DedupEngine.
func NewDedupEngine() *DedupEngine {
	return &DedupEngine{
		records: make(map[string][]DedupRecord),
	}
}

// TaskFingerprint computes a SHA-256 content hash suitable for use as a
// dedup fingerprint. Callers should pass a canonical representation of
// the task (e.g., normalised prompt text).
func TaskFingerprint(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:])
}

// IsDuplicate returns true if the given fingerprint has already been
// recorded by any session.
func (d *DedupEngine) IsDuplicate(taskFingerprint string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.records[taskFingerprint]) > 0
}

// Record stores a completed task result under the given fingerprint. The
// same fingerprint may be recorded multiple times (e.g., retries), and
// each entry is preserved in the history.
func (d *DedupEngine) Record(taskFingerprint string, sessionID string, result string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.records[taskFingerprint] = append(d.records[taskFingerprint], DedupRecord{
		Fingerprint: taskFingerprint,
		SessionID:   sessionID,
		Result:      result,
		RecordedAt:  time.Now(),
	})
}

// History returns all records for a given fingerprint, ordered
// chronologically. Returns nil if the fingerprint has never been seen.
func (d *DedupEngine) History(taskFingerprint string) []DedupRecord {
	d.mu.Lock()
	defer d.mu.Unlock()

	recs := d.records[taskFingerprint]
	if len(recs) == 0 {
		return nil
	}
	// Return a copy to avoid callers mutating internal state.
	out := make([]DedupRecord, len(recs))
	copy(out, recs)
	return out
}

// Size returns the total number of unique fingerprints tracked.
func (d *DedupEngine) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.records)
}

// Clear removes all recorded fingerprints.
func (d *DedupEngine) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.records = make(map[string][]DedupRecord)
}
