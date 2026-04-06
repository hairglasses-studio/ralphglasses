package safety

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// GuardrailLevel maps to session autonomy levels (L0-L3).
type GuardrailLevel int

const (
	LevelObserve      GuardrailLevel = 0 // read-only, no changes
	LevelAutoRecover  GuardrailLevel = 1 // auto-restart on transient errors
	LevelAutoOptimize GuardrailLevel = 2 // auto-adjust config from feedback
	LevelFullAutonomy GuardrailLevel = 3 // auto-launch tasks, scale teams
)

// OperationType classifies what kind of action an agent wants to perform.
type OperationType string

const (
	OpRead      OperationType = "read"
	OpWrite     OperationType = "write"
	OpDelete    OperationType = "delete"
	OpExecute   OperationType = "execute"
	OpGitPush   OperationType = "git_push"
	OpGitForce  OperationType = "git_force"
	OpNetAccess OperationType = "net_access"
	OpSpend     OperationType = "spend"
	OpConfig    OperationType = "config"
	OpLaunch    OperationType = "launch"
	OpStop      OperationType = "stop"
)

// defaultAllowlists defines which operations are permitted at each autonomy level.
var defaultAllowlists = map[GuardrailLevel]map[OperationType]bool{
	LevelObserve: {
		OpRead: true,
	},
	LevelAutoRecover: {
		OpRead:    true,
		OpExecute: true,
		OpLaunch:  true,
		OpStop:    true,
	},
	LevelAutoOptimize: {
		OpRead:      true,
		OpWrite:     true,
		OpExecute:   true,
		OpGitPush:   true,
		OpNetAccess: true,
		OpSpend:     true,
		OpConfig:    true,
		OpLaunch:    true,
		OpStop:      true,
	},
	LevelFullAutonomy: {
		OpRead:      true,
		OpWrite:     true,
		OpDelete:    true,
		OpExecute:   true,
		OpGitPush:   true,
		OpNetAccess: true,
		OpSpend:     true,
		OpConfig:    true,
		OpLaunch:    true,
		OpStop:      true,
		// OpGitForce is NEVER allowed at any level.
	},
}

// sensitivePatterns lists file patterns that agents must never read or write.
var sensitivePatterns = []string{
	".env",
	".env.*",
	"*.pem",
	"*.key",
	"*.p12",
	"credentials.json",
	"credentials.yaml",
	"credentials.yml",
	"*-secret.json",
	"*-secret.yaml",
	"*.secret",
	"id_rsa",
	"id_ed25519",
	".ssh/config",
}

// Guardrails enforces safety boundaries for autonomous agent operations.
// It checks operation allowlists, sensitive file access, and blast radius.
type Guardrails struct {
	mu         sync.RWMutex
	level      GuardrailLevel
	allowlists map[GuardrailLevel]map[OperationType]bool

	// Blast radius limits.
	maxFilesPerSession int
	maxLinesPerSession int

	// Tracking.
	filesModified int
	linesModified int
}

// NewGuardrails creates a guardrails instance at the given autonomy level.
func NewGuardrails(level GuardrailLevel) *Guardrails {
	return &Guardrails{
		level:              level,
		allowlists:         defaultAllowlists,
		maxFilesPerSession: 100,
		maxLinesPerSession: 10000,
	}
}

// SetLevel changes the autonomy level.
func (g *Guardrails) SetLevel(level GuardrailLevel) {
	g.mu.Lock()
	g.level = level
	g.mu.Unlock()
}

// Level returns the current autonomy level.
func (g *Guardrails) Level() GuardrailLevel {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.level
}

// CheckOperation returns nil if the operation is allowed at the current level,
// or an error describing why it's blocked.
func (g *Guardrails) CheckOperation(op OperationType) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Git force push is NEVER allowed.
	if op == OpGitForce {
		return fmt.Errorf("guardrail: force push is never permitted at any autonomy level")
	}

	allowed, ok := g.allowlists[g.level]
	if !ok {
		return fmt.Errorf("guardrail: unknown autonomy level %d", g.level)
	}

	if !allowed[op] {
		return fmt.Errorf("guardrail: operation %q not permitted at autonomy level %d", op, g.level)
	}

	return nil
}

// CheckFileAccess returns nil if the file path is safe to access,
// or an error if the file matches a sensitive pattern.
func (g *Guardrails) CheckFileAccess(path string) error {
	base := filepath.Base(path)
	for _, pattern := range sensitivePatterns {
		matched, err := filepath.Match(pattern, base)
		if err != nil {
			continue
		}
		if matched {
			return fmt.Errorf("guardrail: access to sensitive file %q is blocked (matches %q)", path, pattern)
		}
	}
	return nil
}

// CheckGitSafety validates git operations against safety rules.
func (g *Guardrails) CheckGitSafety(args []string) error {
	if len(args) == 0 {
		return nil
	}

	// Block force push.
	for _, arg := range args {
		if arg == "--force" || arg == "-f" || strings.HasPrefix(arg, "--force-") {
			if len(args) > 0 && args[0] == "push" {
				return fmt.Errorf("guardrail: git force push is blocked")
			}
		}
	}

	// Block push to main/master without explicit approval at L3.
	if args[0] == "push" {
		for _, arg := range args[1:] {
			if arg == "main" || arg == "master" {
				g.mu.RLock()
				level := g.level
				g.mu.RUnlock()
				if level < LevelFullAutonomy {
					return fmt.Errorf("guardrail: push to %s requires L3 autonomy (current: L%d)", arg, level)
				}
			}
		}
	}

	return nil
}

// RecordModification tracks file modifications for blast radius enforcement.
// Returns an error if the blast radius limit is exceeded.
func (g *Guardrails) RecordModification(files int, lines int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.filesModified += files
	g.linesModified += lines

	if g.filesModified > g.maxFilesPerSession {
		return fmt.Errorf("guardrail: blast radius exceeded — %d files modified (max %d)",
			g.filesModified, g.maxFilesPerSession)
	}
	if g.linesModified > g.maxLinesPerSession {
		return fmt.Errorf("guardrail: blast radius exceeded — %d lines modified (max %d)",
			g.linesModified, g.maxLinesPerSession)
	}

	return nil
}

// BlastRadius returns the current modification counts.
func (g *Guardrails) BlastRadius() (files int, lines int) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.filesModified, g.linesModified
}

// SetBlastRadiusLimits sets custom blast radius limits.
func (g *Guardrails) SetBlastRadiusLimits(maxFiles, maxLines int) {
	g.mu.Lock()
	g.maxFilesPerSession = maxFiles
	g.maxLinesPerSession = maxLines
	g.mu.Unlock()
}
