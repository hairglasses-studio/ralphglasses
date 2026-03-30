package session

import (
	"testing"
	"time"
)

func TestConflictResolver_DetectFileConflicts_NoConflict(t *testing.T) {
	t.Parallel()
	cr := NewConflictResolver()

	sessions := []SessionInfo{
		{ID: "s1", ModifiedFiles: []ModifiedFile{
			{Path: "a.go", ModTime: time.Now()},
		}},
		{ID: "s2", ModifiedFiles: []ModifiedFile{
			{Path: "b.go", ModTime: time.Now()},
		}},
	}

	conflicts := cr.DetectFileConflicts(sessions)
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %d", len(conflicts))
	}
}

func TestConflictResolver_DetectFileConflicts_OneConflict(t *testing.T) {
	t.Parallel()
	cr := NewConflictResolver()

	now := time.Now()
	sessions := []SessionInfo{
		{ID: "s1", ModifiedFiles: []ModifiedFile{
			{Path: "shared.go", ModTime: now},
		}},
		{ID: "s2", ModifiedFiles: []ModifiedFile{
			{Path: "shared.go", ModTime: now.Add(time.Second)},
		}},
	}

	conflicts := cr.DetectFileConflicts(sessions)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Path != "shared.go" {
		t.Errorf("conflict path = %q, want shared.go", conflicts[0].Path)
	}
	if len(conflicts[0].Sessions) != 2 {
		t.Errorf("expected 2 sessions in conflict, got %d", len(conflicts[0].Sessions))
	}
}

func TestConflictResolver_DetectFileConflicts_MultipleConflicts(t *testing.T) {
	t.Parallel()
	cr := NewConflictResolver()

	now := time.Now()
	sessions := []SessionInfo{
		{ID: "s1", ModifiedFiles: []ModifiedFile{
			{Path: "b.go", ModTime: now},
			{Path: "a.go", ModTime: now},
		}},
		{ID: "s2", ModifiedFiles: []ModifiedFile{
			{Path: "a.go", ModTime: now.Add(time.Second)},
			{Path: "b.go", ModTime: now.Add(time.Second)},
		}},
	}

	conflicts := cr.DetectFileConflicts(sessions)
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(conflicts))
	}
	// Should be sorted by path.
	if conflicts[0].Path != "a.go" {
		t.Errorf("first conflict path = %q, want a.go", conflicts[0].Path)
	}
	if conflicts[1].Path != "b.go" {
		t.Errorf("second conflict path = %q, want b.go", conflicts[1].Path)
	}
}

func TestConflictResolver_DetectFileConflicts_SameSessionNoDuplicate(t *testing.T) {
	t.Parallel()
	cr := NewConflictResolver()

	now := time.Now()
	sessions := []SessionInfo{
		{ID: "s1", ModifiedFiles: []ModifiedFile{
			{Path: "a.go", ModTime: now},
			{Path: "a.go", ModTime: now.Add(time.Second)},
		}},
	}

	conflicts := cr.DetectFileConflicts(sessions)
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts for single session, got %d", len(conflicts))
	}
}

func TestConflictResolver_DetectFileConflicts_SessionIDFromParent(t *testing.T) {
	t.Parallel()
	cr := NewConflictResolver()

	now := time.Now()
	sessions := []SessionInfo{
		{ID: "s1", ModifiedFiles: []ModifiedFile{
			{Path: "x.go", ModTime: now},
		}},
		{ID: "s2", ModifiedFiles: []ModifiedFile{
			{Path: "x.go", ModTime: now.Add(time.Second)},
		}},
	}

	conflicts := cr.DetectFileConflicts(sessions)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	// Verify session IDs were propagated from parent.
	ids := make(map[string]bool)
	for _, m := range conflicts[0].Sessions {
		ids[m.SessionID] = true
	}
	if !ids["s1"] || !ids["s2"] {
		t.Errorf("expected sessions s1 and s2, got %v", ids)
	}
}

func TestConflictResolver_DetectFileConflicts_Empty(t *testing.T) {
	t.Parallel()
	cr := NewConflictResolver()

	conflicts := cr.DetectFileConflicts(nil)
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts for nil input, got %d", len(conflicts))
	}
}

func TestConflictResolver_ResolveByTimestamp(t *testing.T) {
	t.Parallel()
	cr := NewConflictResolver()

	early := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2025, 1, 1, 0, 0, 1, 0, time.UTC)

	conflicts := []FileConflict{
		{
			Path: "main.go",
			Sessions: []ModifiedFile{
				{Path: "main.go", SessionID: "s1", ModTime: early},
				{Path: "main.go", SessionID: "s2", ModTime: late},
			},
		},
	}

	resolutions := cr.ResolveByTimestamp(conflicts)
	if len(resolutions) != 1 {
		t.Fatalf("expected 1 resolution, got %d", len(resolutions))
	}
	if resolutions[0].Winner != "s2" {
		t.Errorf("winner = %q, want s2 (later timestamp)", resolutions[0].Winner)
	}
	if resolutions[0].Strategy != "timestamp" {
		t.Errorf("strategy = %q, want timestamp", resolutions[0].Strategy)
	}
	if resolutions[0].Path != "main.go" {
		t.Errorf("path = %q, want main.go", resolutions[0].Path)
	}
}

func TestConflictResolver_ResolveByTimestamp_ThreeSessions(t *testing.T) {
	t.Parallel()
	cr := NewConflictResolver()

	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 0, 0, 5, 0, time.UTC)
	t3 := time.Date(2025, 1, 1, 0, 0, 3, 0, time.UTC)

	conflicts := []FileConflict{
		{
			Path: "lib.go",
			Sessions: []ModifiedFile{
				{Path: "lib.go", SessionID: "s1", ModTime: t1},
				{Path: "lib.go", SessionID: "s2", ModTime: t2},
				{Path: "lib.go", SessionID: "s3", ModTime: t3},
			},
		},
	}

	resolutions := cr.ResolveByTimestamp(conflicts)
	if len(resolutions) != 1 {
		t.Fatalf("expected 1 resolution, got %d", len(resolutions))
	}
	if resolutions[0].Winner != "s2" {
		t.Errorf("winner = %q, want s2 (latest at t2)", resolutions[0].Winner)
	}
}

func TestConflictResolver_ResolveByTimestamp_Empty(t *testing.T) {
	t.Parallel()
	cr := NewConflictResolver()

	resolutions := cr.ResolveByTimestamp(nil)
	if len(resolutions) != 0 {
		t.Fatalf("expected no resolutions, got %d", len(resolutions))
	}
}

func TestConflictResolver_ResolveByMerge_Empty(t *testing.T) {
	t.Parallel()
	cr := NewConflictResolver()

	resolutions, err := cr.ResolveByMerge(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolutions) != 0 {
		t.Fatalf("expected no resolutions, got %d", len(resolutions))
	}
}

func TestConflictResolver_ResolveByMerge_FallsBackToTimestamp(t *testing.T) {
	t.Parallel()
	// Use a non-existent git binary to force merge failure.
	cr := &ConflictResolver{GitBin: "/nonexistent/git"}

	early := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2025, 1, 1, 0, 0, 1, 0, time.UTC)

	conflicts := []FileConflict{
		{
			Path: "x.go",
			Sessions: []ModifiedFile{
				{Path: "x.go", SessionID: "s1", ModTime: early},
				{Path: "x.go", SessionID: "s2", ModTime: late},
			},
		},
	}

	resolutions, err := cr.ResolveByMerge(conflicts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolutions) != 1 {
		t.Fatalf("expected 1 resolution, got %d", len(resolutions))
	}
	r := resolutions[0]
	if r.Strategy != "merge" {
		t.Errorf("strategy = %q, want merge", r.Strategy)
	}
	// Should fall back to latest writer.
	if r.Winner != "s2" {
		t.Errorf("winner = %q, want s2", r.Winner)
	}
	if r.MergedOK {
		t.Error("expected MergedOK=false when git is unavailable")
	}
	if r.Error == "" {
		t.Error("expected non-empty error when merge fails")
	}
}

func TestConflictResolver_ResolveByMerge_SingleSession(t *testing.T) {
	t.Parallel()
	cr := NewConflictResolver()

	conflicts := []FileConflict{
		{
			Path: "solo.go",
			Sessions: []ModifiedFile{
				{Path: "solo.go", SessionID: "s1", ModTime: time.Now()},
			},
		},
	}

	resolutions, err := cr.ResolveByMerge(conflicts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolutions) != 1 {
		t.Fatalf("expected 1 resolution, got %d", len(resolutions))
	}
	if !resolutions[0].MergedOK {
		t.Error("expected MergedOK=true for single-session conflict")
	}
	if resolutions[0].Winner != "s1" {
		t.Errorf("winner = %q, want s1", resolutions[0].Winner)
	}
}

func TestSortConflicts(t *testing.T) {
	t.Parallel()
	cs := []FileConflict{
		{Path: "c.go"},
		{Path: "a.go"},
		{Path: "b.go"},
	}
	sortConflicts(cs)
	expected := []string{"a.go", "b.go", "c.go"}
	for i, e := range expected {
		if cs[i].Path != e {
			t.Errorf("cs[%d].Path = %q, want %q", i, cs[i].Path, e)
		}
	}
}
