package model

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AgentIdentity represents the unique identity of a parallel agent.
// Written to .ralph/agent_identity.json before each agent launch.
type AgentIdentity struct {
	AgentIndex        int      `json:"agent_index"`
	AgentCount        int      `json:"agent_count"`
	RunID             string   `json:"run_id"`
	SeedHash          string   `json:"seed_hash"`
	AssignedRole      string   `json:"assigned_role"`
	Persona           string   `json:"persona"`
	ApproachDirective string   `json:"approach_directive"`
	FileOwnership     []string `json:"file_ownership"`
	ForbiddenOverlap  []string `json:"forbidden_overlap"`
}

// Personas are pre-defined complementary roles for parallel agents.
var Personas = []struct {
	Name      string
	Directive string
}{
	{"implementer", "Build the happy path first, get it working."},
	{"hardener", "Focus on error handling, edge cases, validation."},
	{"tester", "Write tests first. TDD approach. Find coverage gaps."},
	{"refactorer", "Clean up existing code, reduce duplication."},
	{"documentor", "Add docs, comments, examples, update README."},
}

// GenerateIdentity creates a deterministic agent identity from run parameters.
func GenerateIdentity(runID string, agentIndex, agentCount int, taskHash string) *AgentIdentity {
	seedInput := fmt.Sprintf("%s:%d:%s", runID, agentIndex, taskHash)
	hash := sha256.Sum256([]byte(seedInput))
	seedHash := fmt.Sprintf("%x", hash[:4])

	persona := Personas[agentIndex%len(Personas)]

	return &AgentIdentity{
		AgentIndex:        agentIndex,
		AgentCount:        agentCount,
		RunID:             runID,
		SeedHash:          seedHash,
		AssignedRole:      "worker",
		Persona:           persona.Name,
		ApproachDirective: persona.Directive,
		FileOwnership:     []string{},
		ForbiddenOverlap:  []string{},
	}
}

// GenerateRunID creates a unique run ID from the current time.
func GenerateRunID() string {
	return time.Now().Format("2006-01-02-150405")
}

// SaveIdentity writes the agent identity to .ralph/agent_identity.json.
func SaveIdentity(repoPath string, id *AgentIdentity) error {
	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "agent_identity.json"), data, 0644)
}

// LoadIdentity reads the agent identity from .ralph/agent_identity.json.
func LoadIdentity(repoPath string) (*AgentIdentity, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, ".ralph", "agent_identity.json"))
	if err != nil {
		return nil, err
	}
	var id AgentIdentity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, err
	}
	return &id, nil
}

// TaskClaim represents a claimed task in .ralph/claimed_tasks/.
type TaskClaim struct {
	TaskID    string    `json:"task_id"`
	AgentIdx  int       `json:"agent_index"`
	ClaimedAt time.Time `json:"claimed_at"`
	Status    string    `json:"status"` // in_progress, completed, failed
}

// ClaimTask creates a lock file for a task in .ralph/claimed_tasks/.
func ClaimTask(repoPath string, taskID string, agentIndex int) error {
	dir := filepath.Join(repoPath, ".ralph", "claimed_tasks")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	lockPath := filepath.Join(dir, taskID+".lock")

	// Check if already claimed
	if _, err := os.Stat(lockPath); err == nil {
		return fmt.Errorf("task %s already claimed", taskID)
	}

	claim := TaskClaim{
		TaskID:    taskID,
		AgentIdx:  agentIndex,
		ClaimedAt: time.Now(),
		Status:    "in_progress",
	}
	data, err := json.MarshalIndent(claim, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(lockPath, data, 0644)
}

// ListClaims reads all task claims from .ralph/claimed_tasks/.
func ListClaims(repoPath string) ([]TaskClaim, error) {
	dir := filepath.Join(repoPath, ".ralph", "claimed_tasks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var claims []TaskClaim
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".lock" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var c TaskClaim
		if err := json.Unmarshal(data, &c); err != nil {
			continue
		}
		claims = append(claims, c)
	}
	return claims, nil
}
