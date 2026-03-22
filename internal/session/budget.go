package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BudgetEnforcer provides secondary budget enforcement and ledger writes.
// Primary enforcement is via claude --max-budget-usd.
type BudgetEnforcer struct {
	Headroom float64 // fraction of budget at which to stop (default 0.90)
}

// NewBudgetEnforcer creates a budget enforcer with default headroom.
func NewBudgetEnforcer() *BudgetEnforcer {
	return &BudgetEnforcer{Headroom: 0.90}
}

// Check returns true if the session has exceeded its budget headroom.
func (b *BudgetEnforcer) Check(s *Session) (exceeded bool, reason string) {
	if s.BudgetUSD <= 0 {
		return false, ""
	}

	s.mu.Lock()
	spent := s.SpentUSD
	s.mu.Unlock()

	threshold := s.BudgetUSD * b.Headroom
	if spent >= threshold {
		return true, fmt.Sprintf("spent $%.2f of $%.2f budget (%.0f%% headroom)",
			spent, s.BudgetUSD, b.Headroom*100)
	}
	return false, ""
}

// LedgerEntry records a cost snapshot for the cost ledger.
type LedgerEntry struct {
	Timestamp  time.Time `json:"ts"`
	SessionID  string    `json:"session_id"`
	Provider   string    `json:"provider"`
	SpendUSD   float64   `json:"spend_usd"`
	TurnCount  int       `json:"turn_count"`
	ElapsedSec float64   `json:"elapsed_s"`
	Model      string    `json:"model"`
	Status     string    `json:"status"`
}

// WriteLedgerEntry appends a cost entry to the repo's cost ledger.
func (b *BudgetEnforcer) WriteLedgerEntry(s *Session, repoPath string) error {
	logDir := filepath.Join(repoPath, ".ralph", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	s.mu.Lock()
	entry := LedgerEntry{
		Timestamp:  time.Now(),
		SessionID:  s.ID,
		Provider:   string(s.Provider),
		SpendUSD:   s.SpentUSD,
		TurnCount:  s.TurnCount,
		ElapsedSec: time.Since(s.LaunchedAt).Seconds(),
		Model:      s.Model,
		Status:     string(s.Status),
	}
	s.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	ledgerPath := filepath.Join(logDir, "cost_ledger.jsonl")
	f, err := os.OpenFile(ledgerPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// CostSummary holds aggregated cost data for a session.
type CostSummary struct {
	SessionID   string    `json:"session_id"`
	Provider    string    `json:"provider"`
	RepoName    string    `json:"repo_name"`
	TotalSpend  float64   `json:"total_spend_usd"`
	BudgetUSD   float64   `json:"budget_usd"`
	TurnCount   int       `json:"turn_count"`
	DurationSec float64   `json:"duration_seconds"`
	Model       string    `json:"model"`
	Status      string    `json:"status"`
	GeneratedAt time.Time `json:"generated_at"`
}

// WriteCostSummary writes a cost summary JSON file to .ralph/.
func (b *BudgetEnforcer) WriteCostSummary(s *Session, repoPath string) error {
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		return err
	}

	s.mu.Lock()
	summary := CostSummary{
		SessionID:   s.ID,
		Provider:    string(s.Provider),
		RepoName:    s.RepoName,
		TotalSpend:  s.SpentUSD,
		BudgetUSD:   s.BudgetUSD,
		TurnCount:   s.TurnCount,
		DurationSec: time.Since(s.LaunchedAt).Seconds(),
		Model:       s.Model,
		Status:      string(s.Status),
		GeneratedAt: time.Now(),
	}
	s.mu.Unlock()

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(ralphDir, "cost_summary.json"), data, 0644)
}
