package session

import (
	"encoding/json"
	"os/exec"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// Provider identifies which LLM CLI backend to use.
type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderGemini Provider = "gemini"
	ProviderCodex  Provider = "codex"
)

// SessionStatus represents the lifecycle state of a Claude Code session.
type SessionStatus string

const (
	StatusLaunching SessionStatus = "launching"
	StatusRunning   SessionStatus = "running"
	StatusCompleted SessionStatus = "completed"
	StatusStopped   SessionStatus = "stopped"
	StatusErrored   SessionStatus = "errored"
)

// IsTerminal returns true if the status represents a finished session.
func (s SessionStatus) IsTerminal() bool {
	return s == StatusCompleted || s == StatusErrored || s == StatusStopped
}

// Session represents a managed headless LLM CLI session.
type Session struct {
	ID                string        `json:"id"`
	Provider          Provider      `json:"provider"`
	ProviderSessionID string        `json:"provider_session_id,omitempty"`
	RepoPath          string        `json:"repo_path"`
	RepoName          string        `json:"repo_name"`
	Status            SessionStatus `json:"status"`
	Prompt            string        `json:"prompt"`
	Model               string        `json:"model,omitempty"`
	EnhancementSource   string        `json:"enhancement_source,omitempty"`    // "local", "llm", "none"
	EnhancementPreScore int           `json:"enhancement_pre_score,omitempty"` // 0-100 quality score before enhancement
	AgentName           string        `json:"agent,omitempty"`
	TeamName          string        `json:"team_name,omitempty"`
	BudgetUSD         float64       `json:"max_budget_usd,omitempty"`
	SpentUSD          float64       `json:"spent_usd"`
	TurnCount         int           `json:"turn_count"`
	MaxTurns          int           `json:"max_turns,omitempty"`
	LaunchedAt        time.Time     `json:"launched_at"`
	LastActivity      time.Time     `json:"last_activity"`
	EndedAt           *time.Time    `json:"ended_at,omitempty"`
	ExitReason        string        `json:"exit_reason,omitempty"`
	LastOutput        string        `json:"last_output,omitempty"`
	Error             string        `json:"error,omitempty"`
	LastEventType     string        `json:"last_event_type,omitempty"`
	StreamParseErrors int           `json:"stream_parse_errors,omitempty"`
	CostSource        string        `json:"cost_source,omitempty"` // "structured", "stderr", or "estimated"
	CostHistory       []float64     `json:"cost_history,omitempty"`
	OutputHistory    []string `json:"output_history,omitempty"`    // last N output lines
	TotalOutputCount int      `json:"total_output_count,omitempty"` // monotonic counter for cursor-based tailing

	Pid       int   `json:"pid,omitempty"`        // process PID captured at launch
	ChildPids []int `json:"child_pids,omitempty"` // child PIDs collected at launch (best-effort)

	cmd        *exec.Cmd
	cancel     func()
	mu         sync.Mutex
	doneCh     chan struct{}   // closed when cmd.Wait() returns in the runner goroutine
	OutputCh   chan string    `json:"-"` // real-time output channel
	bus        *events.Bus    `json:"-"` // event bus for publishing lifecycle events
	onComplete func(*Session) `json:"-"` // called when session ends (for persistence)
}

// Lock locks the session mutex for external callers.
func (s *Session) Lock() { s.mu.Lock() }

// Unlock unlocks the session mutex.
func (s *Session) Unlock() { s.mu.Unlock() }

// StreamEvent represents a parsed line from claude -p --output-format stream-json.
type StreamEvent struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id,omitempty"`
	Model     string          `json:"model,omitempty"`
	CostUSD   float64         `json:"cost_usd,omitempty"`
	Content   string          `json:"content,omitempty"`
	Text      string          `json:"text,omitempty"`
	Error     string          `json:"error,omitempty"`
	NumTurns  int             `json:"num_turns,omitempty"`
	Duration  float64         `json:"duration_seconds,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Result    string          `json:"result,omitempty"`
	CostSource string          `json:"-"`              // "structured" or "estimated" — set by normalizer
	Raw        json.RawMessage `json:"-"`
}

// BatchOptions configures batch/async session execution.
// When Enabled is true, the session is treated as part of a batch and results
// are collected via polling or webhook callback.
type BatchOptions struct {
	Enabled     bool   `json:"enabled"`
	CallbackURL string `json:"callback_url,omitempty"`
	BatchID     string `json:"batch_id,omitempty"`
	Priority    int    `json:"priority,omitempty"` // 0 = default, higher = more urgent
}

// LaunchOptions configures a session launch.
type LaunchOptions struct {
	Provider     Provider
	RepoPath     string
	Prompt       string
	Model        string   // --model
	MaxBudgetUSD float64  // --max-budget-usd
	MaxTurns     int      // --max-turns
	Agent        string   // --agent <name>
	AllowedTools []string // --allowedTools
	SystemPrompt string   // --append-system-prompt
	Resume       string   // --resume <session_id>
	Continue     bool     // --continue
	Worktree     string   // --worktree (branch name or "true" for auto)
	SessionName  string   // --name
	TeamName     string   // team membership (internal tracking)
	Sandbox      bool     // run session in Docker container
	SandboxImage string   // Docker image override (default: ubuntu:24.04)

	Bare          bool              // --bare (skip hooks/plugins for faster scripted calls)
	Effort        string            // --effort low|medium|high|max
	Betas         []string          // --betas (beta feature headers)
	FallbackModel string            // --fallback-model (auto-fallback on overload)
	OutputSchema  json.RawMessage   // --json-schema (Claude) / --output-schema (Codex)

	Batch *BatchOptions // nil means non-batch (normal) mode
}

// TeamConfig holds agent team configuration.
type TeamConfig struct {
	Name           string   `json:"name"`
	Provider       Provider `json:"provider,omitempty"`        // lead session provider
	WorkerProvider Provider `json:"worker_provider,omitempty"` // default provider for worker tasks
	RepoPath       string   `json:"repo_path"`
	LeadAgent      string   `json:"lead_agent,omitempty"`
	Tasks          []string `json:"tasks"`
	Model          string   `json:"model,omitempty"`
	MaxBudgetUSD   float64  `json:"max_budget_usd,omitempty"`
}

// TeamStatus holds team state information.
type TeamStatus struct {
	Name      string        `json:"name"`
	RepoPath  string        `json:"repo_path"`
	LeadID    string        `json:"lead_session_id"`
	Status    SessionStatus `json:"status"`
	Tasks     []TeamTask    `json:"tasks"`
	CreatedAt time.Time     `json:"created_at"`
}

// TeamTask represents a task assigned to a team.
type TeamTask struct {
	Description string   `json:"description"`
	Provider    Provider `json:"provider,omitempty"` // override team default for this task
	Status      string   `json:"status"`             // pending, in-progress, completed
}

// AgentDef represents an agent definition file.
// Claude: .claude/agents/*.md, Gemini: .gemini/agents/*.md, Codex: AGENTS.md sections.
type AgentDef struct {
	Name        string   `json:"name"`
	Provider    Provider `json:"provider,omitempty"` // which provider this agent targets
	Description string   `json:"description,omitempty"`
	Model       string   `json:"model,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	MaxTurns    int      `json:"max_turns,omitempty"`
	Prompt      string   `json:"prompt"` // markdown body after frontmatter
}
