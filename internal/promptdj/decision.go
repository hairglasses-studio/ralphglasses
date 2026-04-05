package promptdj

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DecisionRecord is the persistent form of a routing decision with outcome data.
type DecisionRecord struct {
	// Routing decision fields
	ID              string  `json:"id"`
	Timestamp       string  `json:"ts"`
	Repo            string  `json:"repo"`
	PromptHash      string  `json:"prompt_hash"`
	PromptPreview   string  `json:"prompt_preview"`
	TaskType        string  `json:"task_type"`
	Complexity      int     `json:"complexity"`
	Provider        string  `json:"provider"`
	Model           string  `json:"model"`
	Agent           string  `json:"agent,omitempty"`
	TierLabel       string  `json:"tier_label"`
	Confidence      float64 `json:"confidence"`
	PromptScore     int     `json:"prompt_score"`
	Enhanced        bool    `json:"enhanced"`
	EnhancementMode string  `json:"enhancement_mode,omitempty"`
	FinalScore      int     `json:"final_score"`
	FewShotCount    int     `json:"few_shot_count"`
	CascadeEnabled  bool    `json:"cascade_enabled"`
	EstimatedCost   float64 `json:"estimated_cost_usd"`
	Reasoning       string  `json:"reasoning"`

	// Status tracking
	Status    string `json:"status"` // routed, dispatched, succeeded, failed
	SessionID string `json:"session_id,omitempty"`

	// Outcome (populated by feedback)
	ActualCost      float64    `json:"actual_cost_usd,omitempty"`
	ActualTurns     int        `json:"actual_turns,omitempty"`
	Quality         float64    `json:"quality,omitempty"` // 0.0-1.0
	Escalated       bool       `json:"escalated,omitempty"`
	CorrectProvider string     `json:"correct_provider,omitempty"`
	Notes           string     `json:"notes,omitempty"`
	FeedbackAt      *time.Time `json:"feedback_at,omitempty"`
}

// DecisionFilter specifies query criteria for decision log queries.
type DecisionFilter struct {
	Repo     string
	Provider string
	TaskType string
	Status   string
	Since    time.Time
	Limit    int
}

// DecisionLog persists routing decisions as append-only JSONL.
type DecisionLog struct {
	mu       sync.RWMutex
	path     string
	records  map[string]*DecisionRecord // id -> record
	byTime   []string                   // ids sorted by time (newest first)
	loaded   bool
}

// NewDecisionLog creates a decision log backed by the given directory.
func NewDecisionLog(stateDir string) *DecisionLog {
	path := filepath.Join(stateDir, "promptdj", "decisions.jsonl")
	return &DecisionLog{
		path:    path,
		records: make(map[string]*DecisionRecord),
	}
}

// load reads the JSONL file into memory. Idempotent.
func (dl *DecisionLog) load() error {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	if dl.loaded {
		return nil
	}

	dir := filepath.Dir(dl.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := os.ReadFile(dl.path)
	if err != nil {
		if os.IsNotExist(err) {
			dl.loaded = true
			return nil
		}
		return err
	}

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec DecisionRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		dl.records[rec.ID] = &rec
		dl.byTime = append(dl.byTime, rec.ID)
	}
	dl.loaded = true
	return nil
}

// RecordDecision persists a new routing decision.
func (dl *DecisionLog) RecordDecision(d *RoutingDecision) error {
	if err := dl.load(); err != nil {
		return err
	}

	preview := d.EnhancedPrompt
	if preview == "" {
		// Use original prompt from the request (caller should set this)
		preview = d.Rationale
	}
	if len(preview) > 200 {
		preview = preview[:200]
	}

	rec := &DecisionRecord{
		ID:            d.DecisionID,
		Timestamp:     d.Timestamp.UTC().Format(time.RFC3339),
		TaskType:      string(d.TaskType),
		Complexity:    d.Complexity,
		Provider:      string(d.Provider),
		Model:         d.Model,
		Agent:         d.AgentProfile,
		TierLabel:     d.CostTier,
		Confidence:    d.Confidence,
		PromptScore:   d.OriginalScore,
		Enhanced:      d.WasEnhanced,
		FinalScore:    d.EnhancedScore,
		EstimatedCost: d.EstimatedCostUSD,
		Reasoning:     d.Rationale,
		Status:        "routed",
		PromptPreview: preview,
	}
	if d.EnhancedScore == 0 {
		rec.FinalScore = d.OriginalScore
	}

	dl.mu.Lock()
	dl.records[rec.ID] = rec
	dl.byTime = append([]string{rec.ID}, dl.byTime...)
	dl.mu.Unlock()

	return dl.appendRecord(rec)
}

// GetDecision retrieves a decision by ID.
func (dl *DecisionLog) GetDecision(id string) (*DecisionRecord, bool) {
	if err := dl.load(); err != nil {
		return nil, false
	}
	dl.mu.RLock()
	defer dl.mu.RUnlock()
	rec, ok := dl.records[id]
	return rec, ok
}

// RecordOutcome updates a decision with feedback data.
func (dl *DecisionLog) RecordOutcome(id string, success bool, costUSD float64, turns int, notes string) error {
	if err := dl.load(); err != nil {
		return err
	}

	dl.mu.Lock()
	rec, ok := dl.records[id]
	if !ok {
		dl.mu.Unlock()
		return fmt.Errorf("decision not found: %s", id)
	}

	now := time.Now().UTC()
	rec.FeedbackAt = &now
	rec.ActualCost = costUSD
	rec.ActualTurns = turns
	rec.Notes = notes
	if success {
		rec.Status = "succeeded"
		rec.Quality = 1.0
	} else {
		rec.Status = "failed"
		rec.Quality = 0.0
	}
	dl.mu.Unlock()

	return dl.rewriteAll()
}

// QueryDecisions returns decisions matching the filter.
func (dl *DecisionLog) QueryDecisions(f DecisionFilter) []DecisionRecord {
	if err := dl.load(); err != nil {
		return nil
	}

	dl.mu.RLock()
	defer dl.mu.RUnlock()

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	var results []DecisionRecord
	for _, id := range dl.byTime {
		if len(results) >= limit {
			break
		}
		rec := dl.records[id]
		if f.Repo != "" && rec.Repo != f.Repo {
			continue
		}
		if f.Provider != "" && rec.Provider != f.Provider {
			continue
		}
		if f.TaskType != "" && rec.TaskType != f.TaskType {
			continue
		}
		if f.Status != "" && f.Status != "all" && rec.Status != f.Status {
			continue
		}
		if !f.Since.IsZero() {
			ts, _ := time.Parse(time.RFC3339, rec.Timestamp)
			if ts.Before(f.Since) {
				continue
			}
		}
		results = append(results, *rec)
	}
	return results
}

// Len returns the number of decisions in the log.
func (dl *DecisionLog) Len() int {
	dl.mu.RLock()
	defer dl.mu.RUnlock()
	return len(dl.records)
}

// appendRecord appends a single record to the JSONL file with flock.
func (dl *DecisionLog) appendRecord(rec *DecisionRecord) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	dir := filepath.Dir(dl.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(dl.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("flock: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	_, err = fmt.Fprintf(f, "%s\n", line)
	return err
}

// rewriteAll atomically rewrites the entire JSONL file.
func (dl *DecisionLog) rewriteAll() error {
	dl.mu.RLock()
	defer dl.mu.RUnlock()

	tmp := dl.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	for _, id := range dl.byTime {
		rec := dl.records[id]
		line, err := json.Marshal(rec)
		if err != nil {
			continue
		}
		fmt.Fprintf(f, "%s\n", line)
	}
	f.Close()
	return os.Rename(tmp, dl.path)
}
