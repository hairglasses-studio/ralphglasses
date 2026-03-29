package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MaxChainDepth is the maximum number of chained cycles before requiring manual review.
const MaxChainDepth = 10

// CycleLineage tracks parent-child relationships between chained cycles.
type CycleLineage struct {
	ParentID  string    `json:"parent_id"`
	ChildID   string    `json:"child_id"`
	ChainedAt time.Time `json:"chained_at"`
}

// CycleChainer watches completed cycles and proposes next cycles from their synthesis.
type CycleChainer struct {
	mu         sync.Mutex
	lineage    []CycleLineage
	lastCheck  time.Time
	seenCycles map[string]bool
}

// NewCycleChainer creates a CycleChainer with an initialized seenCycles map.
func NewCycleChainer() *CycleChainer {
	return &CycleChainer{
		seenCycles: make(map[string]bool),
	}
}

// CheckAndChain scans for newly completed cycles and returns chain parameters
// for the first one found. The caller (supervisor) decides whether to launch.
func (cc *CycleChainer) CheckAndChain(ctx context.Context, repoPath string) (*CycleRun, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	cycles, err := ListCycles(repoPath)
	if err != nil {
		return nil, fmt.Errorf("list cycles: %w", err)
	}

	cc.lastCheck = time.Now()

	for _, cycle := range cycles {
		if cycle.Phase != CycleComplete {
			continue
		}
		if cc.seenCycles[cycle.ID] {
			continue
		}

		cc.seenCycles[cycle.ID] = true

		depth := cc.chainDepthLocked(cycle.ID)
		if depth >= MaxChainDepth {
			return nil, nil
		}

		name, objective, criteria := ChainFromSynthesis(cycle)
		if objective == "" {
			return nil, nil
		}

		return NewCycleRun(name, repoPath, objective, criteria), nil
	}

	return nil, nil
}

// ChainFromSynthesis builds next cycle parameters from a completed cycle's synthesis.
// Returns empty objective if there is nothing meaningful to chain.
func ChainFromSynthesis(completed *CycleRun) (name, objective string, criteria []string) {
	if completed.Synthesis == nil {
		return "", "", nil
	}

	idPrefix := completed.ID
	if len(idPrefix) > 8 {
		idPrefix = idPrefix[:8]
	}

	// Check for regression/failure findings first.
	for _, f := range completed.Findings {
		if f.Category == "regression" || f.Category == "failure" {
			name = fmt.Sprintf("chain-%s-fix", idPrefix)
			objective = fmt.Sprintf("Fix regressions found in cycle %s", idPrefix)
			for _, ff := range completed.Findings {
				if ff.Category == "regression" || ff.Category == "failure" {
					criteria = append(criteria, ff.Description)
				}
			}
			return name, objective, criteria
		}
	}

	// Use remaining items if present.
	if len(completed.Synthesis.Remaining) > 0 {
		name = fmt.Sprintf("chain-%s-cont", idPrefix)
		objective = fmt.Sprintf("Continue: address remaining items from cycle %s", idPrefix)
		criteria = completed.Synthesis.Remaining
		return name, objective, criteria
	}

	// Explore patterns if present.
	if len(completed.Synthesis.Patterns) > 0 {
		name = fmt.Sprintf("chain-%s-pat", idPrefix)
		objective = fmt.Sprintf("Explore patterns: %s", completed.Synthesis.Patterns[0])
		criteria = completed.Synthesis.Patterns
		return name, objective, criteria
	}

	return "", "", nil
}

// RecordLineage adds a parent-child link and persists to disk. Thread-safe.
func (cc *CycleChainer) RecordLineage(repoPath, parentID, childID string) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.lineage = append(cc.lineage, CycleLineage{
		ParentID:  parentID,
		ChildID:   childID,
		ChainedAt: time.Now(),
	})
	return cc.saveLineageLocked(repoPath)
}

// ChainDepth walks lineage backwards from cycleID to find the root. Returns 0 for unchained cycles.
func (cc *CycleChainer) ChainDepth(cycleID string) int {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.chainDepthLocked(cycleID)
}

func (cc *CycleChainer) chainDepthLocked(cycleID string) int {
	// Build child→parent map.
	parentOf := make(map[string]string, len(cc.lineage))
	for _, l := range cc.lineage {
		parentOf[l.ChildID] = l.ParentID
	}

	depth := 0
	current := cycleID
	for depth < MaxChainDepth {
		parent, ok := parentOf[current]
		if !ok {
			break
		}
		depth++
		current = parent
	}
	return depth
}

// LoadLineage reads cycle lineage from .ralph/cycle_lineage.json if it exists.
func (cc *CycleChainer) LoadLineage(repoPath string) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	path := filepath.Join(repoPath, ".ralph", "cycle_lineage.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read lineage: %w", err)
	}

	var lineage []CycleLineage
	if err := json.Unmarshal(data, &lineage); err != nil {
		return fmt.Errorf("unmarshal lineage: %w", err)
	}
	cc.lineage = lineage
	return nil
}

// SaveLineage writes lineage to .ralph/cycle_lineage.json.
func (cc *CycleChainer) SaveLineage(repoPath string) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.saveLineageLocked(repoPath)
}

func (cc *CycleChainer) saveLineageLocked(repoPath string) error {
	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .ralph dir: %w", err)
	}

	data, err := json.MarshalIndent(cc.lineage, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lineage: %w", err)
	}

	path := filepath.Join(dir, "cycle_lineage.json")
	return os.WriteFile(path, data, 0o644)
}
