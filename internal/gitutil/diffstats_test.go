package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// --- Fixture-based tests for ParseDiffStat ---

func TestParseDiffStat_Empty(t *testing.T) {
	ds, err := ParseDiffStat("")
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(ds.Files))
	}
	if ds.TotalFiles != 0 || ds.TotalInsertions != 0 || ds.TotalDeletions != 0 {
		t.Errorf("expected all zeros, got files=%d ins=%d del=%d",
			ds.TotalFiles, ds.TotalInsertions, ds.TotalDeletions)
	}
}

func TestParseDiffStat_SingleFile(t *testing.T) {
	input := ` main.go | 5 +++--
 1 file changed, 3 insertions(+), 2 deletions(-)`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(ds.Files))
	}
	f := ds.Files[0]
	if f.Path != "main.go" {
		t.Errorf("path = %q, want %q", f.Path, "main.go")
	}
	if f.Insertions != 3 {
		t.Errorf("insertions = %d, want 3", f.Insertions)
	}
	if f.Deletions != 2 {
		t.Errorf("deletions = %d, want 2", f.Deletions)
	}
	if f.Renamed || f.Binary {
		t.Errorf("expected not renamed and not binary")
	}

	// Summary totals.
	if ds.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1", ds.TotalFiles)
	}
	if ds.TotalInsertions != 3 {
		t.Errorf("TotalInsertions = %d, want 3", ds.TotalInsertions)
	}
	if ds.TotalDeletions != 2 {
		t.Errorf("TotalDeletions = %d, want 2", ds.TotalDeletions)
	}
}

func TestParseDiffStat_MultipleFiles(t *testing.T) {
	input := ` cmd/main.go             | 10 +++++++---
 internal/util/helper.go |  3 ++-
 README.md               |  2 ++
 3 files changed, 12 insertions(+), 4 deletions(-)`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(ds.Files))
	}

	// Verify file paths.
	wantPaths := []string{"cmd/main.go", "internal/util/helper.go", "README.md"}
	for i, want := range wantPaths {
		if ds.Files[i].Path != want {
			t.Errorf("file[%d].Path = %q, want %q", i, ds.Files[i].Path, want)
		}
	}

	if ds.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3", ds.TotalFiles)
	}
	if ds.TotalInsertions != 12 {
		t.Errorf("TotalInsertions = %d, want 12", ds.TotalInsertions)
	}
	if ds.TotalDeletions != 4 {
		t.Errorf("TotalDeletions = %d, want 4", ds.TotalDeletions)
	}
}

func TestParseDiffStat_RenamedBraces(t *testing.T) {
	// Brace-style rename: internal/{old => new}/file.go
	input := ` internal/{old => new}/file.go | 4 ++--
 1 file changed, 2 insertions(+), 2 deletions(-)`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(ds.Files))
	}
	f := ds.Files[0]
	if !f.Renamed {
		t.Error("expected Renamed = true")
	}
	if f.Path != "internal/new/file.go" {
		t.Errorf("Path = %q, want %q", f.Path, "internal/new/file.go")
	}
	if f.OldPath != "internal/old/file.go" {
		t.Errorf("OldPath = %q, want %q", f.OldPath, "internal/old/file.go")
	}
}

func TestParseDiffStat_RenamedFullPath(t *testing.T) {
	// Full-path rename: old.go => new.go
	input := ` old.go => new.go | 0
 1 file changed, 0 insertions(+), 0 deletions(-)`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(ds.Files))
	}
	f := ds.Files[0]
	if !f.Renamed {
		t.Error("expected Renamed = true")
	}
	if f.Path != "new.go" {
		t.Errorf("Path = %q, want %q", f.Path, "new.go")
	}
	if f.OldPath != "old.go" {
		t.Errorf("OldPath = %q, want %q", f.OldPath, "old.go")
	}
}

func TestParseDiffStat_RenamedBracesFilename(t *testing.T) {
	// Rename at the filename level: path/{old.go => new.go}
	input := ` internal/{handler.go => controller.go} | 8 ++++----
 1 file changed, 4 insertions(+), 4 deletions(-)`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(ds.Files))
	}
	f := ds.Files[0]
	if !f.Renamed {
		t.Error("expected Renamed = true")
	}
	if f.Path != "internal/controller.go" {
		t.Errorf("Path = %q, want %q", f.Path, "internal/controller.go")
	}
	if f.OldPath != "internal/handler.go" {
		t.Errorf("OldPath = %q, want %q", f.OldPath, "internal/handler.go")
	}
}

func TestParseDiffStat_BinaryFile(t *testing.T) {
	input := ` logo.png | Bin 0 -> 12345 bytes
 1 file changed, 0 insertions(+), 0 deletions(-)`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(ds.Files))
	}
	f := ds.Files[0]
	if !f.Binary {
		t.Error("expected Binary = true")
	}
	if f.Path != "logo.png" {
		t.Errorf("Path = %q, want %q", f.Path, "logo.png")
	}
	if f.Insertions != 0 || f.Deletions != 0 {
		t.Errorf("binary file should have 0 insertions/deletions, got %d/%d",
			f.Insertions, f.Deletions)
	}
}

func TestParseDiffStat_BinaryFileRemoved(t *testing.T) {
	input := ` old.bin | Bin 5678 -> 0 bytes
 1 file changed, 0 insertions(+), 0 deletions(-)`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(ds.Files))
	}
	if !ds.Files[0].Binary {
		t.Error("expected Binary = true for removed binary")
	}
}

func TestParseDiffStat_MixedChanges(t *testing.T) {
	// Mix of normal, renamed, and binary files.
	input := ` cmd/main.go                            | 15 +++++++++------
 internal/{handler.go => controller.go} |  8 ++++----
 assets/logo.png                        | Bin 0 -> 4567 bytes
 docs/README.md                         |  3 +++
 4 files changed, 14 insertions(+), 10 deletions(-)`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Files) != 4 {
		t.Fatalf("expected 4 files, got %d", len(ds.Files))
	}

	// Check file 0: normal edit.
	if ds.Files[0].Path != "cmd/main.go" {
		t.Errorf("file[0].Path = %q", ds.Files[0].Path)
	}
	if ds.Files[0].Renamed || ds.Files[0].Binary {
		t.Errorf("file[0] should not be renamed or binary")
	}

	// Check file 1: rename.
	if !ds.Files[1].Renamed {
		t.Error("file[1] should be renamed")
	}
	if ds.Files[1].Path != "internal/controller.go" {
		t.Errorf("file[1].Path = %q", ds.Files[1].Path)
	}

	// Check file 2: binary.
	if !ds.Files[2].Binary {
		t.Error("file[2] should be binary")
	}

	// Check file 3: additions only.
	if ds.Files[3].Path != "docs/README.md" {
		t.Errorf("file[3].Path = %q", ds.Files[3].Path)
	}
	if ds.Files[3].Insertions != 3 || ds.Files[3].Deletions != 0 {
		t.Errorf("file[3] ins=%d del=%d, want ins=3 del=0",
			ds.Files[3].Insertions, ds.Files[3].Deletions)
	}

	// Summary.
	if ds.TotalFiles != 4 {
		t.Errorf("TotalFiles = %d, want 4", ds.TotalFiles)
	}
	if ds.TotalInsertions != 14 {
		t.Errorf("TotalInsertions = %d, want 14", ds.TotalInsertions)
	}
	if ds.TotalDeletions != 10 {
		t.Errorf("TotalDeletions = %d, want 10", ds.TotalDeletions)
	}
}

func TestParseDiffStat_InsertionsOnly(t *testing.T) {
	input := ` newfile.go | 20 ++++++++++++++++++++
 1 file changed, 20 insertions(+)`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}
	if ds.TotalInsertions != 20 {
		t.Errorf("TotalInsertions = %d, want 20", ds.TotalInsertions)
	}
	if ds.TotalDeletions != 0 {
		t.Errorf("TotalDeletions = %d, want 0", ds.TotalDeletions)
	}
}

func TestParseDiffStat_DeletionsOnly(t *testing.T) {
	input := ` old_code.go | 15 ---------------
 1 file changed, 15 deletions(-)`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}
	if ds.TotalInsertions != 0 {
		t.Errorf("TotalInsertions = %d, want 0", ds.TotalInsertions)
	}
	if ds.TotalDeletions != 15 {
		t.Errorf("TotalDeletions = %d, want 15", ds.TotalDeletions)
	}
	if len(ds.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(ds.Files))
	}
	if ds.Files[0].Deletions != 15 {
		t.Errorf("file deletions = %d, want 15", ds.Files[0].Deletions)
	}
}

func TestParseDiffStat_NoSummaryLine(t *testing.T) {
	// If only file lines are present without a summary, totals should be derived.
	input := ` main.go | 5 +++--`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(ds.Files))
	}
	// Totals derived from per-file data.
	if ds.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1", ds.TotalFiles)
	}
	if ds.TotalInsertions != 3 {
		t.Errorf("TotalInsertions = %d, want 3", ds.TotalInsertions)
	}
	if ds.TotalDeletions != 2 {
		t.Errorf("TotalDeletions = %d, want 2", ds.TotalDeletions)
	}
}

// --- Helper method tests ---

func TestDiffStats_BinaryFiles(t *testing.T) {
	ds := &DiffStats{
		Files: []FileStat{
			{Path: "main.go", Insertions: 5},
			{Path: "logo.png", Binary: true},
			{Path: "icon.ico", Binary: true},
			{Path: "util.go", Insertions: 3},
		},
	}
	bins := ds.BinaryFiles()
	if len(bins) != 2 {
		t.Fatalf("expected 2 binary files, got %d", len(bins))
	}
	if bins[0].Path != "logo.png" || bins[1].Path != "icon.ico" {
		t.Errorf("unexpected binary paths: %v, %v", bins[0].Path, bins[1].Path)
	}
}

func TestDiffStats_RenamedFiles(t *testing.T) {
	ds := &DiffStats{
		Files: []FileStat{
			{Path: "new.go", OldPath: "old.go", Renamed: true},
			{Path: "main.go"},
			{Path: "b.go", OldPath: "a.go", Renamed: true},
		},
	}
	renames := ds.RenamedFiles()
	if len(renames) != 2 {
		t.Fatalf("expected 2 renamed files, got %d", len(renames))
	}
	if renames[0].OldPath != "old.go" || renames[1].OldPath != "a.go" {
		t.Errorf("unexpected old paths: %q, %q", renames[0].OldPath, renames[1].OldPath)
	}
}

func TestDiffStats_EmptyFilters(t *testing.T) {
	ds := &DiffStats{
		Files: []FileStat{
			{Path: "main.go", Insertions: 5},
		},
	}
	if len(ds.BinaryFiles()) != 0 {
		t.Error("expected 0 binary files")
	}
	if len(ds.RenamedFiles()) != 0 {
		t.Error("expected 0 renamed files")
	}
}

// --- Internal parser tests ---

func TestParseRenamePath_BraceStyle(t *testing.T) {
	tests := []struct {
		input   string
		path    string
		oldPath string
		renamed bool
	}{
		{
			input:   "internal/{old => new}/file.go",
			path:    "internal/new/file.go",
			oldPath: "internal/old/file.go",
			renamed: true,
		},
		{
			input:   "pkg/{handler.go => controller.go}",
			path:    "pkg/controller.go",
			oldPath: "pkg/handler.go",
			renamed: true,
		},
		{
			input:   "{ => src}/main.go",
			path:    "src/main.go",
			oldPath: "main.go",
			renamed: true,
		},
		{
			input:   "main.go",
			path:    "main.go",
			oldPath: "",
			renamed: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			path, oldPath, renamed := parseRenamePath(tt.input)
			if path != tt.path {
				t.Errorf("path = %q, want %q", path, tt.path)
			}
			if oldPath != tt.oldPath {
				t.Errorf("oldPath = %q, want %q", oldPath, tt.oldPath)
			}
			if renamed != tt.renamed {
				t.Errorf("renamed = %v, want %v", renamed, tt.renamed)
			}
		})
	}
}

func TestParseRenamePath_FullPath(t *testing.T) {
	path, oldPath, renamed := parseRenamePath("old/file.go => new/file.go")
	if !renamed {
		t.Error("expected renamed = true")
	}
	if path != "new/file.go" {
		t.Errorf("path = %q, want %q", path, "new/file.go")
	}
	if oldPath != "old/file.go" {
		t.Errorf("oldPath = %q, want %q", oldPath, "old/file.go")
	}
}

func TestParseStatCounts(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantIns int
		wantDel int
	}{
		{"plus and minus", "5 +++--", 3, 2},
		{"plus only", "10 ++++++++++", 10, 0},
		{"minus only", "3 ---", 0, 3},
		{"zero", "0", 0, 0},
		{"empty", "", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ins, del := parseStatCounts(tt.input)
			if ins != tt.wantIns {
				t.Errorf("insertions = %d, want %d", ins, tt.wantIns)
			}
			if del != tt.wantDel {
				t.Errorf("deletions = %d, want %d", del, tt.wantDel)
			}
		})
	}
}

func TestIsSummaryLine(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"3 files changed, 10 insertions(+), 5 deletions(-)", true},
		{"1 file changed, 1 insertion(+)", true},
		{"1 file changed, 1 deletion(-)", true},
		{"main.go | 5 +++--", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isSummaryLine(tt.input); got != tt.want {
				t.Errorf("isSummaryLine(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCleanPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"src//file.go", "src/file.go"},
		{"  main.go  ", "main.go"},
		{"a///b//c", "a/b/c"},
		{"normal/path.go", "normal/path.go"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := cleanPath(tt.input); got != tt.want {
				t.Errorf("cleanPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Integration test: DiffBetween with a real git repo ---

func TestDiffBetween_Integration(t *testing.T) {
	dir := initTestRepo(t)

	// Create a branch, make changes, and verify DiffBetween parses correctly.
	gitRun(t, dir, "checkout", "-b", "feature")

	// Add a new file.
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\n\nfunc New() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Modify existing file.
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\nline two\nline three\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "feature changes")

	ds, err := DiffBetween(dir, "main", "feature")
	if err != nil {
		t.Fatalf("DiffBetween: %v", err)
	}

	if ds.TotalFiles < 1 {
		t.Errorf("expected at least 1 file changed, got %d", ds.TotalFiles)
	}
	if len(ds.Files) < 1 {
		t.Errorf("expected at least 1 file entry, got %d", len(ds.Files))
	}
	if ds.TotalInsertions == 0 {
		t.Error("expected non-zero insertions")
	}
}

func TestDiffBetween_Rename(t *testing.T) {
	dir := initTestRepo(t)

	// Add a file with enough content that git recognizes the rename.
	content := "package main\n\n// This file has enough content that git\n// will detect it as a rename rather than\n// a delete+add pair when we move it.\nfunc Original() {\n\treturn\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "original.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "original.go")
	gitRun(t, dir, "commit", "-m", "add original")

	// Create branch, rename the file.
	gitRun(t, dir, "checkout", "-b", "rename-branch")
	gitRun(t, dir, "mv", "original.go", "renamed.go")
	gitRun(t, dir, "commit", "-m", "rename file")

	ds, err := DiffBetween(dir, "main", "rename-branch")
	if err != nil {
		t.Fatalf("DiffBetween: %v", err)
	}

	if ds.TotalFiles < 1 {
		t.Errorf("expected at least 1 file changed, got %d", ds.TotalFiles)
	}

	renames := ds.RenamedFiles()
	if len(renames) != 1 {
		t.Fatalf("expected 1 renamed file, got %d", len(renames))
	}
	if renames[0].Path != "renamed.go" {
		t.Errorf("renamed path = %q, want %q", renames[0].Path, "renamed.go")
	}
	if renames[0].OldPath != "original.go" {
		t.Errorf("old path = %q, want %q", renames[0].OldPath, "original.go")
	}
}

func TestDiffBetween_BinaryFile(t *testing.T) {
	dir := initTestRepo(t)

	gitRun(t, dir, "checkout", "-b", "binary-branch")

	// Write a binary file (non-UTF-8 bytes).
	binData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00}
	if err := os.WriteFile(filepath.Join(dir, "image.png"), binData, 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "image.png")
	gitRun(t, dir, "commit", "-m", "add binary")

	ds, err := DiffBetween(dir, "main", "binary-branch")
	if err != nil {
		t.Fatalf("DiffBetween: %v", err)
	}

	bins := ds.BinaryFiles()
	if len(bins) != 1 {
		t.Fatalf("expected 1 binary file, got %d", len(bins))
	}
	if bins[0].Path != "image.png" {
		t.Errorf("binary path = %q, want %q", bins[0].Path, "image.png")
	}
}

func TestDiffBetween_NoDiff(t *testing.T) {
	dir := initTestRepo(t)
	gitRun(t, dir, "checkout", "-b", "same")

	ds, err := DiffBetween(dir, "main", "same")
	if err != nil {
		t.Fatalf("DiffBetween: %v", err)
	}
	if ds.TotalFiles != 0 {
		t.Errorf("expected 0 files changed, got %d", ds.TotalFiles)
	}
	if len(ds.Files) != 0 {
		t.Errorf("expected 0 file entries, got %d", len(ds.Files))
	}
}

func TestDiffBetween_InvalidRef(t *testing.T) {
	dir := initTestRepo(t)
	_, err := DiffBetween(dir, "nonexistent-base", "nonexistent-head")
	if err == nil {
		t.Error("expected error for invalid refs, got nil")
	}
}

// --- Fixture: real git diff --stat output from a large change ---

func TestParseDiffStat_LargeFixture(t *testing.T) {
	input := ` Makefile                                      |   8 +++---
 cmd/root.go                                   |  42 +++++++++++++++++-----------
 internal/config/config.go                     |  15 ++++++----
 internal/config/config_test.go                |  23 +++++++++++++++
 internal/session/manager.go                   | 102 +++++++++++++++++++++++++++++++++++++++++-------------------
 internal/session/manager_test.go              |  87 +++++++++++++++++++++++++++++++++++++++++++++++
 internal/tui/views/dashboard.go               |  31 ++++++++++--------
 internal/{util/helpers.go => helpers/util.go} |   4 +--
 assets/logo.png                               | Bin 0 -> 15432 bytes
 docs/arch.svg                                 | Bin 8901 -> 9123 bytes
 10 files changed, 252 insertions(+), 80 deletions(-)`

	ds, err := ParseDiffStat(input)
	if err != nil {
		t.Fatal(err)
	}

	if len(ds.Files) != 10 {
		t.Fatalf("expected 10 files, got %d", len(ds.Files))
	}

	if ds.TotalFiles != 10 {
		t.Errorf("TotalFiles = %d, want 10", ds.TotalFiles)
	}
	if ds.TotalInsertions != 252 {
		t.Errorf("TotalInsertions = %d, want 252", ds.TotalInsertions)
	}
	if ds.TotalDeletions != 80 {
		t.Errorf("TotalDeletions = %d, want 80", ds.TotalDeletions)
	}

	// Check the renamed file.
	renames := ds.RenamedFiles()
	if len(renames) != 1 {
		t.Fatalf("expected 1 rename, got %d", len(renames))
	}
	if renames[0].Path != "internal/helpers/util.go" {
		t.Errorf("renamed path = %q, want %q", renames[0].Path, "internal/helpers/util.go")
	}
	if renames[0].OldPath != "internal/util/helpers.go" {
		t.Errorf("renamed old path = %q, want %q", renames[0].OldPath, "internal/util/helpers.go")
	}

	// Check binary files.
	bins := ds.BinaryFiles()
	if len(bins) != 2 {
		t.Fatalf("expected 2 binary files, got %d", len(bins))
	}
}

// gitRun is a test helper that runs a git command in the given directory.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
