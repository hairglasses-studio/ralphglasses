package parity

import (
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

type WorktreeInfo struct {
	session.WorktreeInfo
	Stale bool `json:"stale"`
}

func ListWorktrees(repoPath string, dirtyOnly bool, staleAfter time.Duration) ([]WorktreeInfo, error) {
	wts, err := session.ListWorktrees(repoPath)
	if err != nil {
		return nil, err
	}
	result := make([]WorktreeInfo, 0, len(wts))
	cutoff := time.Now().Add(-staleAfter)
	for _, wt := range wts {
		if dirtyOnly && !wt.Dirty {
			continue
		}
		item := WorktreeInfo{WorktreeInfo: wt}
		if staleAfter > 0 {
			if mod, err := time.Parse(time.RFC3339, wt.ModTime); err == nil && mod.Before(cutoff) {
				item.Stale = true
			}
		}
		result = append(result, item)
	}
	return result, nil
}

func PreviewWorktreeCleanup(repoPath string, olderThan time.Duration) ([]WorktreeInfo, error) {
	wts, err := ListWorktrees(repoPath, false, olderThan)
	if err != nil {
		return nil, err
	}
	result := make([]WorktreeInfo, 0, len(wts))
	for _, wt := range wts {
		if wt.Stale && !wt.Dirty {
			result = append(result, wt)
		}
	}
	return result, nil
}
