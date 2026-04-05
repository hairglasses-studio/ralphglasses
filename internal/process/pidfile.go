package process

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PIDInfo describes a running session tracked by a JSON PID file.
type PIDInfo struct {
	PID       int       `json:"pid"`
	StartTime time.Time `json:"start_time"`
	RepoPath  string    `json:"repo_path"`
	Provider  string    `json:"provider,omitempty"`
}

// SessionDir is the subdirectory under .ralph where session PID files are stored.
const SessionDir = "sessions"

// WritePIDFile writes a JSON PID file into dir/<pid>.json.
func WritePIDFile(dir string, info PIDInfo) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal PID info: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%d.json", info.PID))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}
	return nil
}

// ReadPIDFile reads and parses a JSON PID file at the given path.
func ReadPIDFile(path string) (*PIDInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read PID file: %w", err)
	}
	var info PIDInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parse PID file: %w", err)
	}
	if info.PID <= 0 {
		return nil, fmt.Errorf("invalid PID in file: %d", info.PID)
	}
	return &info, nil
}

// RemovePIDFile removes the JSON PID file for the given PID from dir.
func RemovePIDFile(dir string, pid int) error {
	path := filepath.Join(dir, fmt.Sprintf("%d.json", pid))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove PID file: %w", err)
	}
	return nil
}

// ListPIDFiles reads all JSON PID files from dir and returns their info.
// Non-JSON files and files that fail to parse are silently skipped.
func ListPIDFiles(dir string) ([]PIDInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session dir: %w", err)
	}
	var infos []PIDInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := ReadPIDFile(path)
		if err != nil {
			slog.Debug("skipping invalid PID file", "path", path, "error", err)
			continue
		}
		infos = append(infos, *info)
	}
	return infos, nil
}

// ScanOrphans reads PID files from sessionDir and returns entries whose
// processes are no longer alive.
func ScanOrphans(sessionDir string) ([]PIDInfo, error) {
	infos, err := ListPIDFiles(sessionDir)
	if err != nil {
		return nil, err
	}
	var orphans []PIDInfo
	for _, info := range infos {
		if !aliveFn(info.PID) {
			orphans = append(orphans, info)
		}
	}
	return orphans, nil
}

// CleanupOrphans removes PID files for dead processes in sessionDir.
// Returns the number of PID files removed.
func CleanupOrphans(sessionDir string) (int, error) {
	orphans, err := ScanOrphans(sessionDir)
	if err != nil {
		return 0, err
	}
	cleaned := 0
	for _, info := range orphans {
		if err := RemovePIDFile(sessionDir, info.PID); err != nil {
			slog.Warn("failed to remove orphan PID file", "pid", info.PID, "error", err)
			continue
		}
		cleaned++
	}
	return cleaned, nil
}

// RestartPolicy configures automatic restart behavior for managed processes.
type RestartPolicy struct {
	MaxRestarts  int `json:"max_restarts"`  // maximum restart attempts before giving up
	BackoffSecs  int `json:"backoff_secs"`  // initial backoff duration in seconds
	CooldownSecs int `json:"cooldown_secs"` // cooldown period in seconds after max restarts exhausted
}

// DefaultRestartPolicy returns sensible defaults for restart behavior.
func DefaultRestartPolicy() RestartPolicy {
	return RestartPolicy{
		MaxRestarts:  5,
		BackoffSecs:  30,
		CooldownSecs: 300,
	}
}

// ShouldRestart returns true if the restart count has not exceeded the max.
func (rp RestartPolicy) ShouldRestart(restartCount int) bool {
	return restartCount < rp.MaxRestarts
}

// BackoffDuration returns the backoff duration for the given restart attempt,
// using exponential backoff starting from BackoffSecs.
func (rp RestartPolicy) BackoffDuration(restartCount int) time.Duration {
	base := rp.BackoffSecs
	if base <= 0 {
		base = 1
	}
	// Exponential backoff: base * 2^restartCount, capped at cooldown.
	backoff := time.Duration(base) * time.Second
	for range restartCount {
		backoff *= 2
		if rp.CooldownSecs > 0 && backoff > time.Duration(rp.CooldownSecs)*time.Second {
			backoff = time.Duration(rp.CooldownSecs) * time.Second
			break
		}
	}
	return backoff
}

// CooldownDuration returns the cooldown period as a time.Duration.
func (rp RestartPolicy) CooldownDuration() time.Duration {
	return time.Duration(rp.CooldownSecs) * time.Second
}

// healthCheckState tracks the state of a running health check loop.
type healthCheckState struct {
	mu               sync.Mutex
	consecutiveFails int
	totalChecks      int
	totalFailures    int
	stopped          bool
}

// StartHealthCheck runs periodic health checks at the given interval.
// The check function is called each interval. After 3 consecutive failures,
// onFailure is called (if non-nil). Returns a stop function that terminates
// the health check loop.
func StartHealthCheck(interval time.Duration, check func() error, onFailure func()) func() {
	if interval <= 0 {
		interval = 5 * time.Second
	}

	state := &healthCheckState{}
	stopCh := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				state.mu.Lock()
				if state.stopped {
					state.mu.Unlock()
					return
				}
				state.totalChecks++
				state.mu.Unlock()

				err := check()

				state.mu.Lock()
				if err != nil {
					state.consecutiveFails++
					state.totalFailures++
					slog.Warn("health check failed",
						"consecutive_failures", state.consecutiveFails,
						"error", err,
					)
					if state.consecutiveFails >= 3 && onFailure != nil {
						state.mu.Unlock()
						onFailure()
						state.mu.Lock()
						state.consecutiveFails = 0 // reset after triggering
					}
				} else {
					state.consecutiveFails = 0
				}
				state.mu.Unlock()
			}
		}
	}()

	return func() {
		state.mu.Lock()
		state.stopped = true
		state.mu.Unlock()
		close(stopCh)
	}
}

// HealthCheckStats returns the total checks and failures for a health check state.
// This is exposed for testing via the HealthCheckResult type.
type HealthCheckStats struct {
	TotalChecks      int
	TotalFailures    int
	ConsecutiveFails int
}

// StartHealthCheckWithStats is like StartHealthCheck but also returns a function
// to query health check statistics. Used primarily for testing.
func StartHealthCheckWithStats(interval time.Duration, check func() error, onFailure func()) (stop func(), stats func() HealthCheckStats) {
	if interval <= 0 {
		interval = 5 * time.Second
	}

	state := &healthCheckState{}
	stopCh := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				state.mu.Lock()
				if state.stopped {
					state.mu.Unlock()
					return
				}
				state.totalChecks++
				state.mu.Unlock()

				err := check()

				state.mu.Lock()
				if err != nil {
					state.consecutiveFails++
					state.totalFailures++
					slog.Warn("health check failed",
						"consecutive_failures", state.consecutiveFails,
						"error", err,
					)
					if state.consecutiveFails >= 3 && onFailure != nil {
						state.mu.Unlock()
						onFailure()
						state.mu.Lock()
						state.consecutiveFails = 0
					}
				} else {
					state.consecutiveFails = 0
				}
				state.mu.Unlock()
			}
		}
	}()

	stopFn := func() {
		state.mu.Lock()
		state.stopped = true
		state.mu.Unlock()
		close(stopCh)
	}

	statsFn := func() HealthCheckStats {
		state.mu.Lock()
		defer state.mu.Unlock()
		return HealthCheckStats{
			TotalChecks:      state.totalChecks,
			TotalFailures:    state.totalFailures,
			ConsecutiveFails: state.consecutiveFails,
		}
	}

	return stopFn, statsFn
}

// sessionPIDFilePath returns the path for a session PID file within a repo.
func sessionPIDFilePath(repoPath string) string {
	return filepath.Join(repoPath, ".ralph", SessionDir)
}

// WriteSessionPIDFile writes a session PID file for the given repo, creating
// the .ralph/sessions/ directory as needed.
func WriteSessionPIDFile(repoPath string, pid int, provider string) error {
	dir := sessionPIDFilePath(repoPath)
	info := PIDInfo{
		PID:       pid,
		StartTime: time.Now(),
		RepoPath:  repoPath,
		Provider:  provider,
	}
	return WritePIDFile(dir, info)
}

// RemoveSessionPIDFile removes the session PID file for a given PID under a repo.
func RemoveSessionPIDFile(repoPath string, pid int) error {
	dir := sessionPIDFilePath(repoPath)
	return RemovePIDFile(dir, pid)
}

// ListSessionPIDFiles lists all session PID files for a repo.
func ListSessionPIDFiles(repoPath string) ([]PIDInfo, error) {
	dir := sessionPIDFilePath(repoPath)
	return ListPIDFiles(dir)
}

// ScanSessionOrphans scans for orphaned sessions in a repo.
func ScanSessionOrphans(repoPath string) ([]PIDInfo, error) {
	dir := sessionPIDFilePath(repoPath)
	return ScanOrphans(dir)
}

// CleanupSessionOrphans removes PID files for dead sessions in a repo.
func CleanupSessionOrphans(repoPath string) (int, error) {
	dir := sessionPIDFilePath(repoPath)
	return CleanupOrphans(dir)
}

// pidInfoFromLegacy creates a PIDInfo from the legacy single-integer PID file format.
// Used to bridge between the old writePIDFile/readPIDFile and the new JSON format.
func pidInfoFromLegacy(repoPath string) *PIDInfo {
	pid := readPIDFile(repoPath)
	if pid == 0 {
		return nil
	}
	return &PIDInfo{
		PID:      pid,
		RepoPath: repoPath,
	}
}

// migrateToJSON converts a legacy integer PID file to the new JSON format.
// Returns nil if no legacy PID file exists or the process is dead.
func migrateToJSON(repoPath string) *PIDInfo {
	info := pidInfoFromLegacy(repoPath)
	if info == nil {
		return nil
	}
	if !aliveFn(info.PID) {
		removePIDFile(repoPath)
		return nil
	}
	info.StartTime = time.Now() // best-effort, actual start time unknown
	dir := sessionPIDFilePath(repoPath)
	if err := WritePIDFile(dir, *info); err != nil {
		slog.Warn("failed to migrate PID file to JSON", "repo", repoPath, "error", err)
		return nil
	}
	return info
}

// formatPIDFileName generates the PID file name as "<pid>.json".
func formatPIDFileName(pid int) string {
	return strconv.Itoa(pid) + ".json"
}
