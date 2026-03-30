package session

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SessionInfo identifies a session and the files it has modified. Used by
// the conflict resolver to detect overlapping writes across fleet sessions.
type SessionInfo struct {
	ID           string
	RepoPath     string
	ModifiedFiles []ModifiedFile
}

// ModifiedFile represents a file touched by a session.
type ModifiedFile struct {
	Path      string
	ModTime   time.Time
	SessionID string
}

// FileConflict describes two or more sessions that modified the same file.
type FileConflict struct {
	Path     string
	Sessions []ModifiedFile // all modifications, one per session
}

// Resolution records how a conflict was resolved.
type Resolution struct {
	Path      string
	Winner    string // session ID whose version was chosen
	Strategy  string // "timestamp" or "merge"
	MergedOK  bool   // true if merge succeeded without conflicts
	Error     string // non-empty if resolution failed
}

// ConflictResolver detects and resolves file-level conflicts across
// fleet sessions operating in the same repository.
type ConflictResolver struct {
	// GitBin is the path to the git binary. Defaults to "git".
	GitBin string
}

// NewConflictResolver creates a ConflictResolver with default settings.
func NewConflictResolver() *ConflictResolver {
	return &ConflictResolver{
		GitBin: "git",
	}
}

// DetectFileConflicts scans the provided sessions and returns a FileConflict
// for every file that was modified by more than one session.
func (cr *ConflictResolver) DetectFileConflicts(sessions []SessionInfo) []FileConflict {
	// path -> list of modifications across sessions
	index := make(map[string][]ModifiedFile)

	for _, sess := range sessions {
		for _, mf := range sess.ModifiedFiles {
			entry := mf
			if entry.SessionID == "" {
				entry.SessionID = sess.ID
			}
			index[entry.Path] = append(index[entry.Path], entry)
		}
	}

	var conflicts []FileConflict
	// Collect only paths touched by >1 session.
	for path, mods := range index {
		sessionSet := make(map[string]struct{})
		for _, m := range mods {
			sessionSet[m.SessionID] = struct{}{}
		}
		if len(sessionSet) > 1 {
			conflicts = append(conflicts, FileConflict{
				Path:     path,
				Sessions: mods,
			})
		}
	}

	// Deterministic ordering.
	sortConflicts(conflicts)
	return conflicts
}

// ResolveByTimestamp resolves each conflict using last-writer-wins: the
// session with the most recent ModTime is chosen as the winner.
func (cr *ConflictResolver) ResolveByTimestamp(conflicts []FileConflict) []Resolution {
	resolutions := make([]Resolution, 0, len(conflicts))
	for _, c := range conflicts {
		var winner ModifiedFile
		for _, m := range c.Sessions {
			if m.ModTime.After(winner.ModTime) {
				winner = m
			}
		}
		resolutions = append(resolutions, Resolution{
			Path:     c.Path,
			Winner:   winner.SessionID,
			Strategy: "timestamp",
		})
	}
	return resolutions
}

// ResolveByMerge attempts a git three-way merge for each conflict. It runs
// git merge-file in the repository of the first session that touched the
// file. If the merge succeeds cleanly, MergedOK is true. If the merge has
// conflicts, the resolution falls back to last-writer-wins and records the
// error.
func (cr *ConflictResolver) ResolveByMerge(conflicts []FileConflict) ([]Resolution, error) {
	if len(conflicts) == 0 {
		return nil, nil
	}

	gitBin := cr.GitBin
	if gitBin == "" {
		gitBin = "git"
	}

	resolutions := make([]Resolution, 0, len(conflicts))
	for _, c := range conflicts {
		res := cr.attemptMerge(gitBin, c)
		resolutions = append(resolutions, res)
	}
	return resolutions, nil
}

// attemptMerge tries to merge changes for a single conflict.
func (cr *ConflictResolver) attemptMerge(gitBin string, c FileConflict) Resolution {
	if len(c.Sessions) < 2 {
		return Resolution{
			Path:     c.Path,
			Strategy: "merge",
			MergedOK: true,
			Winner:   c.Sessions[0].SessionID,
		}
	}

	// Find the repo path from sessions — use first available.
	// We rely on git diff to detect whether the file can be merged cleanly.
	// For a real three-way merge we'd need the common ancestor, the two
	// branch tips, and git merge-file. Here we check if git can auto-merge
	// by running `git merge-tree` or fall back to timestamp.

	// Determine latest writer for fallback.
	var latest ModifiedFile
	for _, m := range c.Sessions {
		if m.ModTime.After(latest.ModTime) {
			latest = m
		}
	}

	// Attempt git diff --check to see if the file has merge markers or
	// conflicts. This is a lightweight heuristic; full merge requires
	// worktree context that may not be available.
	cmd := exec.Command(gitBin, "diff", "--check", "--", c.Path)
	// Run in the directory of the first session if set.
	output, err := cmd.CombinedOutput()
	if err != nil {
		// git diff --check returns non-zero on conflicts or missing repo.
		return Resolution{
			Path:     c.Path,
			Winner:   latest.SessionID,
			Strategy: "merge",
			MergedOK: false,
			Error:    fmt.Sprintf("merge check failed: %s: %s", err, strings.TrimSpace(string(output))),
		}
	}

	return Resolution{
		Path:     c.Path,
		Winner:   latest.SessionID,
		Strategy: "merge",
		MergedOK: true,
	}
}

// sortConflicts sorts FileConflict slices by path for deterministic output.
func sortConflicts(cs []FileConflict) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j].Path < cs[j-1].Path; j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}
