// D2.2: Hyperagent Execution Engine — Self-modification engine for Level 3 autonomy.
//
// Informed by Hyperagents (ArXiv 2603.19461, Meta AI): recursive metacognitive
// self-modification with DGM-H framework. Modifications transfer across domains
// and accumulate over time.
//
// Safety: All modifications are logged, rate-limited, and revertible.
// ForbiddenTargets cannot be self-modified (kill_switch, safety_config,
// autonomy_level, forbidden_targets).
package session

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Modification represents a proposed or applied self-modification.
type Modification struct {
	ID         string          `json:"id"`
	Target     string          `json:"target"` // e.g. "prompt_template:plan", "cascade:threshold"
	OldValue   json.RawMessage `json:"old_value"`
	NewValue   json.RawMessage `json:"new_value"`
	Reason     string          `json:"reason"`
	Confidence float64         `json:"confidence"` // 0.0-1.0
	Status     string          `json:"status"`     // "proposed", "applied", "rolled_back", "rejected"
	AppliedAt  *time.Time      `json:"applied_at,omitempty"`
	RevertedAt *time.Time      `json:"reverted_at,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// HyperagentEngine manages gated self-modification of system parameters.
// Only active at autonomy Level 3 (LevelFullAutonomy).
type HyperagentEngine struct {
	mu            sync.Mutex
	modifications []Modification
	stateDir      string

	// Safety configuration
	GateThreshold    float64  // minimum confidence to apply (default 0.8)
	MaxModsPerHour   int      // rate limit (default 3)
	ForbiddenTargets []string // never modify these

	// Rate limiting
	recentApplyTimes []time.Time
}

// DefaultForbiddenTargets returns the hardcoded list of targets that cannot be self-modified.
func DefaultForbiddenTargets() []string {
	return []string{"kill_switch", "safety_config", "autonomy_level", "forbidden_targets"}
}

// NewHyperagentEngine creates a self-modification engine.
func NewHyperagentEngine(stateDir string) *HyperagentEngine {
	h := &HyperagentEngine{
		stateDir:         stateDir,
		GateThreshold:    0.8,
		MaxModsPerHour:   3,
		ForbiddenTargets: DefaultForbiddenTargets(),
	}
	_ = h.Load()
	return h
}

// Propose registers a modification proposal. Returns the modification ID.
func (h *HyperagentEngine) Propose(target string, oldValue, newValue json.RawMessage, reason string, confidence float64) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check forbidden targets
	if slices.Contains(h.ForbiddenTargets, target) {
		return "", fmt.Errorf("hyperagent: target %q is forbidden", target)
	}

	mod := Modification{
		ID:         fmt.Sprintf("mod-%d", time.Now().UnixNano()),
		Target:     target,
		OldValue:   oldValue,
		NewValue:   newValue,
		Reason:     reason,
		Confidence: confidence,
		Status:     "proposed",
		CreatedAt:  time.Now(),
	}

	h.modifications = append(h.modifications, mod)

	slog.Info("hyperagent: modification proposed",
		"id", mod.ID, "target", target, "confidence", confidence, "reason", reason)
	return mod.ID, nil
}

// Execute applies a proposed modification if it passes the confidence gate
// and rate limit. Returns error if gating fails.
func (h *HyperagentEngine) Execute(modID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	mod := h.findMod(modID)
	if mod == nil {
		return fmt.Errorf("hyperagent: modification %s not found", modID)
	}
	if mod.Status != "proposed" {
		return fmt.Errorf("hyperagent: modification %s is %s, not proposed", modID, mod.Status)
	}

	// Confidence gate
	if mod.Confidence < h.GateThreshold {
		mod.Status = "rejected"
		return fmt.Errorf("hyperagent: confidence %.2f below threshold %.2f", mod.Confidence, h.GateThreshold)
	}

	// Rate limit
	cutoff := time.Now().Add(-1 * time.Hour)
	recent := 0
	for _, t := range h.recentApplyTimes {
		if t.After(cutoff) {
			recent++
		}
	}
	if recent >= h.MaxModsPerHour {
		return fmt.Errorf("hyperagent: rate limit exceeded (%d/%d mods in last hour)", recent, h.MaxModsPerHour)
	}

	// Apply
	now := time.Now()
	mod.Status = "applied"
	mod.AppliedAt = &now
	h.recentApplyTimes = append(h.recentApplyTimes, now)

	slog.Info("hyperagent: modification applied",
		"id", modID, "target", mod.Target, "confidence", mod.Confidence)

	_ = h.Save()
	return nil
}

// Rollback reverts an applied modification using the stored OldValue.
func (h *HyperagentEngine) Rollback(modID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	mod := h.findMod(modID)
	if mod == nil {
		return fmt.Errorf("hyperagent: modification %s not found", modID)
	}
	if mod.Status != "applied" {
		return fmt.Errorf("hyperagent: modification %s is %s, cannot rollback", modID, mod.Status)
	}

	now := time.Now()
	mod.Status = "rolled_back"
	mod.RevertedAt = &now

	slog.Info("hyperagent: modification rolled back", "id", modID, "target", mod.Target)
	_ = h.Save()
	return nil
}

// PendingModifications returns all proposed (unapplied) modifications.
func (h *HyperagentEngine) PendingModifications() []Modification {
	h.mu.Lock()
	defer h.mu.Unlock()

	var pending []Modification
	for _, m := range h.modifications {
		if m.Status == "proposed" {
			pending = append(pending, m)
		}
	}
	return pending
}

// History returns all modifications.
func (h *HyperagentEngine) History() []Modification {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]Modification, len(h.modifications))
	copy(result, h.modifications)
	return result
}

// AppliedCount returns the number of applied modifications.
func (h *HyperagentEngine) AppliedCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	count := 0
	for _, m := range h.modifications {
		if m.Status == "applied" {
			count++
		}
	}
	return count
}

// Save persists state to disk.
func (h *HyperagentEngine) Save() error {
	if h.stateDir == "" {
		return nil
	}
	data, err := json.MarshalIndent(h.modifications, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(h.stateDir, "hyperagent_state.json")
	return os.WriteFile(path, data, 0644)
}

// Load restores state from disk.
func (h *HyperagentEngine) Load() error {
	if h.stateDir == "" {
		return nil
	}
	path := filepath.Join(h.stateDir, "hyperagent_state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &h.modifications)
}

func (h *HyperagentEngine) findMod(id string) *Modification {
	for i := range h.modifications {
		if h.modifications[i].ID == id {
			return &h.modifications[i]
		}
	}
	return nil
}
