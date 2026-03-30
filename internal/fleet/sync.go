package fleet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// SyncStatus represents the state of a worktree sync operation.
type SyncStatus string

const (
	SyncIdle       SyncStatus = "idle"
	SyncPending    SyncStatus = "pending"
	SyncInProgress SyncStatus = "in_progress"
	SyncComplete   SyncStatus = "complete"
	SyncConflict   SyncStatus = "conflict"
	SyncFailed     SyncStatus = "failed"
)

// NodeSyncState tracks synchronization state for a single remote node.
type NodeSyncState struct {
	NodeID      string     `json:"node_id"`
	RepoName    string     `json:"repo_name"`
	Status      SyncStatus `json:"status"`
	LastSyncRef string     `json:"last_sync_ref"` // last successfully synced commit
	BundleHash  string     `json:"bundle_hash,omitempty"`
	Error       string     `json:"error,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// BundleInfo describes a git bundle created for transfer.
type BundleInfo struct {
	Path      string    `json:"path"`
	RepoPath  string    `json:"repo_path"`
	BaseRef   string    `json:"base_ref"`   // basis commit (empty for full bundle)
	TipRef    string    `json:"tip_ref"`    // newest commit in bundle
	Hash      string    `json:"hash"`       // SHA-256 of bundle file
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// WorktreeSync manages git worktree synchronization between fleet nodes
// using git bundles for efficient transfer.
type WorktreeSync struct {
	mu        sync.RWMutex
	bundleDir string // directory to store generated bundles
	states    map[string]*NodeSyncState // key: "nodeID:repoName"
}

// NewWorktreeSync creates a WorktreeSync that stores bundles in the given directory.
func NewWorktreeSync(bundleDir string) *WorktreeSync {
	return &WorktreeSync{
		bundleDir: bundleDir,
		states:    make(map[string]*NodeSyncState),
	}
}

// CreateBundle generates a git bundle from repoPath. If baseRef is non-empty,
// the bundle is incremental (baseRef..HEAD). Otherwise it bundles all history.
func (ws *WorktreeSync) CreateBundle(ctx context.Context, repoPath, baseRef string) (*BundleInfo, error) {
	if err := os.MkdirAll(ws.bundleDir, 0o755); err != nil {
		return nil, fmt.Errorf("create bundle dir: %w", err)
	}

	// Resolve HEAD for the tip ref
	tipRef, err := ws.gitRevParse(ctx, repoPath, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}

	repoName := filepath.Base(repoPath)
	bundleName := fmt.Sprintf("%s-%s.bundle", repoName, tipRef[:12])
	bundlePath := filepath.Join(ws.bundleDir, bundleName)

	// Build bundle command
	args := []string{"bundle", "create", bundlePath}
	if baseRef != "" {
		// Incremental: only commits after baseRef
		args = append(args, fmt.Sprintf("%s..HEAD", baseRef))
	} else {
		// Full bundle: all reachable from HEAD
		args = append(args, "HEAD")
	}

	if err := ws.gitCmd(ctx, repoPath, args...); err != nil {
		return nil, fmt.Errorf("git bundle create: %w", err)
	}

	// Compute hash and size
	hash, size, err := hashFile(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("hash bundle: %w", err)
	}

	return &BundleInfo{
		Path:      bundlePath,
		RepoPath:  repoPath,
		BaseRef:   baseRef,
		TipRef:    tipRef,
		Hash:      hash,
		SizeBytes: size,
		CreatedAt: time.Now(),
	}, nil
}

// VerifyBundle checks that a bundle file is valid for the given repo.
func (ws *WorktreeSync) VerifyBundle(ctx context.Context, repoPath, bundlePath string) error {
	return ws.gitCmd(ctx, repoPath, "bundle", "verify", bundlePath)
}

// ApplyBundle fetches refs from a bundle into the repo.
func (ws *WorktreeSync) ApplyBundle(ctx context.Context, repoPath, bundlePath string) error {
	if err := ws.VerifyBundle(ctx, repoPath, bundlePath); err != nil {
		return fmt.Errorf("bundle verification failed: %w", err)
	}
	return ws.gitCmd(ctx, repoPath, "fetch", bundlePath)
}

// SetSyncState updates the tracked sync state for a node+repo pair.
func (ws *WorktreeSync) SetSyncState(nodeID, repoName string, status SyncStatus, ref string, syncErr error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	key := syncKey(nodeID, repoName)
	state, ok := ws.states[key]
	if !ok {
		state = &NodeSyncState{
			NodeID:   nodeID,
			RepoName: repoName,
		}
		ws.states[key] = state
	}

	state.Status = status
	state.UpdatedAt = time.Now()

	if ref != "" {
		state.LastSyncRef = ref
	}
	if syncErr != nil {
		state.Error = syncErr.Error()
	} else {
		state.Error = ""
	}
}

// GetSyncState returns the current sync state for a node+repo pair.
func (ws *WorktreeSync) GetSyncState(nodeID, repoName string) *NodeSyncState {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	state, ok := ws.states[syncKey(nodeID, repoName)]
	if !ok {
		return nil
	}
	// Return a copy
	cp := *state
	return &cp
}

// AllSyncStates returns a snapshot of all tracked sync states.
func (ws *WorktreeSync) AllSyncStates() []NodeSyncState {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	out := make([]NodeSyncState, 0, len(ws.states))
	for _, s := range ws.states {
		out = append(out, *s)
	}
	return out
}

// DetectConflict checks whether a node's last synced ref is still an ancestor
// of the repo's current HEAD. If not, the histories have diverged.
func (ws *WorktreeSync) DetectConflict(ctx context.Context, repoPath, lastSyncRef string) (bool, error) {
	if lastSyncRef == "" {
		// No prior sync, no conflict possible
		return false, nil
	}

	err := ws.gitCmd(ctx, repoPath, "merge-base", "--is-ancestor", lastSyncRef, "HEAD")
	if err != nil {
		// Exit code 1 means not an ancestor => diverged
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("merge-base check: %w", err)
	}
	return false, nil
}

// SyncNode performs a full sync cycle for a single node+repo: detect conflicts,
// create an incremental bundle, and update sync state.
func (ws *WorktreeSync) SyncNode(ctx context.Context, nodeID, repoPath string) (*BundleInfo, error) {
	repoName := filepath.Base(repoPath)

	// Get last sync ref for incremental bundle
	state := ws.GetSyncState(nodeID, repoName)
	var baseRef string
	if state != nil {
		baseRef = state.LastSyncRef
	}

	ws.SetSyncState(nodeID, repoName, SyncInProgress, "", nil)

	// Check for divergence
	if baseRef != "" {
		diverged, err := ws.DetectConflict(ctx, repoPath, baseRef)
		if err != nil {
			ws.SetSyncState(nodeID, repoName, SyncFailed, "", err)
			return nil, err
		}
		if diverged {
			ws.SetSyncState(nodeID, repoName, SyncConflict, "", fmt.Errorf("histories diverged from %s", baseRef))
			return nil, fmt.Errorf("conflict: node %s repo %s diverged from %s", nodeID, repoName, baseRef)
		}
	}

	// Create bundle
	bundle, err := ws.CreateBundle(ctx, repoPath, baseRef)
	if err != nil {
		ws.SetSyncState(nodeID, repoName, SyncFailed, "", err)
		return nil, err
	}

	ws.SetSyncState(nodeID, repoName, SyncComplete, bundle.TipRef, nil)
	return bundle, nil
}

// gitCmd runs a git command in the given working directory.
func (ws *WorktreeSync) gitCmd(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, stderr.String())
		}
		return err
	}
	return nil
}

// gitRevParse runs git rev-parse and returns the trimmed output.
func (ws *WorktreeSync) gitRevParse(ctx context.Context, dir, ref string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", ref)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(out)), nil
}

// hashFile computes the SHA-256 hash and size of a file.
func hashFile(path string) (string, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), int64(len(data)), nil
}

// syncKey returns the map key for a node+repo pair.
func syncKey(nodeID, repoName string) string {
	return nodeID + ":" + repoName
}
