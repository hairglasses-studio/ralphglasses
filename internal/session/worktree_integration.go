package session

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/worktree"
)

// WorktreeIntegrationInfo describes an active session worktree managed by WorktreeManager.
type WorktreeIntegrationInfo struct {
	SessionID    string    `json:"session_id"`
	WorktreePath string    `json:"worktree_path"`
	Branch       string    `json:"branch"`
	RepoPath     string    `json:"repo_path"`
	CreatedAt    time.Time `json:"created_at"`
}

// WorktreeManager is the integration layer between session launches and the
// internal/worktree package. It creates, tracks, and cleans up git worktrees
// on a per-session basis.
type WorktreeManager struct {
	mu       sync.Mutex
	active   map[string]*WorktreeIntegrationInfo // keyed by sessionID
	baseDir  string                               // override for worktree parent (empty = use repo default)
}

// NewWorktreeManager creates a WorktreeManager. If baseDir is non-empty, worktrees
// are created under baseDir/<sessionID>; otherwise they go under <repoPath>/.ralph/worktrees/sessions/<sessionID>.
func NewWorktreeManager(baseDir string) *WorktreeManager {
	return &WorktreeManager{
		active:  make(map[string]*WorktreeIntegrationInfo),
		baseDir: baseDir,
	}
}

// CreateForSession creates a git worktree for the given session, branching from
// the current HEAD of repoPath. The worktree path and any error are returned.
func (wm *WorktreeManager) CreateForSession(sessionID, repoPath string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("worktree manager: session ID is empty")
	}
	if strings.TrimSpace(repoPath) == "" {
		return "", fmt.Errorf("worktree manager: repo path is empty")
	}

	wm.mu.Lock()
	if _, exists := wm.active[sessionID]; exists {
		wm.mu.Unlock()
		return "", fmt.Errorf("worktree manager: session %q already has an active worktree", sessionID)
	}
	wm.mu.Unlock()

	// Determine the worktree path.
	var wtPath string
	if wm.baseDir != "" {
		wtPath = filepath.Join(wm.baseDir, sessionID)
	} else {
		wtPath = filepath.Join(repoPath, ".ralph", "worktrees", "sessions", sessionID)
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
		return "", fmt.Errorf("worktree manager: create parent dir: %w", err)
	}

	// Create the worktree via the worktree package.
	branch := fmt.Sprintf("ralph/session/%s", sessionID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := worktree.Create(ctx, repoPath, wtPath, branch); err != nil {
		return "", fmt.Errorf("worktree manager: create: %w", err)
	}

	info := &WorktreeIntegrationInfo{
		SessionID:    sessionID,
		WorktreePath: wtPath,
		Branch:       branch,
		RepoPath:     repoPath,
		CreatedAt:    time.Now(),
	}

	wm.mu.Lock()
	wm.active[sessionID] = info
	wm.mu.Unlock()

	slog.Info("created session worktree", "session", sessionID, "path", wtPath, "branch", branch)
	return wtPath, nil
}

// CleanupSession removes the worktree associated with the given session.
// It removes the git worktree (force=true to handle dirty state) and prunes
// stale references. Returns nil if the session has no tracked worktree.
func (wm *WorktreeManager) CleanupSession(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("worktree manager: session ID is empty")
	}

	wm.mu.Lock()
	info, exists := wm.active[sessionID]
	if !exists {
		wm.mu.Unlock()
		return nil // nothing to clean up
	}
	delete(wm.active, sessionID)
	wm.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Remove the git worktree (force to handle uncommitted changes).
	if err := worktree.Remove(ctx, info.RepoPath, info.WorktreePath, true); err != nil {
		// If the directory is already gone, that is fine.
		if _, statErr := os.Stat(info.WorktreePath); !os.IsNotExist(statErr) {
			slog.Warn("worktree remove failed, cleaning up directory",
				"session", sessionID, "path", info.WorktreePath, "error", err)
			_ = os.RemoveAll(info.WorktreePath)
		}
	}

	// Prune stale worktree references (best-effort).
	if err := worktree.Prune(ctx, info.RepoPath); err != nil {
		slog.Debug("worktree prune after cleanup failed", "session", sessionID, "error", err)
	}

	slog.Info("cleaned up session worktree", "session", sessionID, "path", info.WorktreePath)
	return nil
}

// ListActive returns information about all currently tracked session worktrees.
func (wm *WorktreeManager) ListActive() []WorktreeIntegrationInfo {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	result := make([]WorktreeIntegrationInfo, 0, len(wm.active))
	for _, info := range wm.active {
		result = append(result, *info)
	}
	return result
}
