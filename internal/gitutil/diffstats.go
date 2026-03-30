package gitutil

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// FileStat represents diff statistics for a single file.
type FileStat struct {
	Path       string // file path (new path for renames)
	OldPath    string // original path if renamed, empty otherwise
	Insertions int
	Deletions  int
	Binary     bool // true when git reports "Bin X -> Y bytes"
	Renamed    bool
}

// DiffStats holds the aggregated result of parsing git diff --stat output.
type DiffStats struct {
	Files           []FileStat
	TotalFiles      int
	TotalInsertions int
	TotalDeletions  int
}

// BinaryFiles returns only the files marked as binary changes.
func (d *DiffStats) BinaryFiles() []FileStat {
	var out []FileStat
	for _, f := range d.Files {
		if f.Binary {
			out = append(out, f)
		}
	}
	return out
}

// RenamedFiles returns only the files that were renamed.
func (d *DiffStats) RenamedFiles() []FileStat {
	var out []FileStat
	for _, f := range d.Files {
		if f.Renamed {
			out = append(out, f)
		}
	}
	return out
}

// DiffBetween runs git diff --stat --find-renames between two refs (base and
// head) in the given repo directory and returns structured DiffStats.
func DiffBetween(repo, base, head string) (*DiffStats, error) {
	args := []string{"diff", "--stat", "--find-renames", base + "..." + head}
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --stat %s...%s: %w", base, head, err)
	}
	return ParseDiffStat(string(out))
}

// reRename matches rename lines: "old/path => new/path" or "{old => new}/rest"
var reRename = regexp.MustCompile(`^(.+?)\{(.+?) => (.+?)\}(.*)$|^(.+?) => (.+?)$`)

// reBinary matches binary file change lines like "Bin 0 -> 1234 bytes" or "Bin 1234 -> 0 bytes"
var reBinary = regexp.MustCompile(`Bin \d+ -> \d+ bytes`)

// ParseDiffStat parses the full text output of git diff --stat into a DiffStats
// struct. It handles normal file lines, renamed files (both {old => new} and
// full-path rename syntax), and binary file changes.
func ParseDiffStat(output string) (*DiffStats, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return &DiffStats{}, nil
	}

	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return &DiffStats{}, nil
	}

	ds := &DiffStats{}

	// The last line is the summary. Process per-file lines first.
	var fileLines []string
	var summaryLine string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i == len(lines)-1 && isSummaryLine(trimmed) {
			summaryLine = trimmed
		} else {
			fileLines = append(fileLines, line)
		}
	}

	for _, line := range fileLines {
		fs, err := parseFileLine(line)
		if err != nil {
			continue // skip unparseable lines
		}
		ds.Files = append(ds.Files, fs)
	}

	// Parse the summary line for totals.
	if summaryLine != "" {
		ds.TotalFiles, ds.TotalInsertions, ds.TotalDeletions = parseSummaryLine(summaryLine)
	}

	// If no summary line was found, derive totals from per-file data.
	if summaryLine == "" {
		ds.TotalFiles = len(ds.Files)
		for _, f := range ds.Files {
			ds.TotalInsertions += f.Insertions
			ds.TotalDeletions += f.Deletions
		}
	}

	return ds, nil
}

// isSummaryLine returns true if the line looks like the trailing summary
// produced by git diff --stat, e.g.:
//
//	" 3 files changed, 10 insertions(+), 5 deletions(-)"
func isSummaryLine(line string) bool {
	return strings.Contains(line, "file") && strings.Contains(line, "changed")
}

// parseFileLine parses a single per-file line from git diff --stat output.
//
// Examples of lines we handle:
//
//	" main.go              | 12 +++++-------"
//	" old.go => new.go     |  4 ++--"
//	" path/{old => new}.go |  2 +-"
//	" img.png              | Bin 0 -> 1234 bytes"
func parseFileLine(line string) (FileStat, error) {
	// Split on the pipe character that separates file name from stats.
	parts := strings.SplitN(line, "|", 2)
	if len(parts) != 2 {
		return FileStat{}, fmt.Errorf("no pipe separator found")
	}

	namePart := strings.TrimSpace(parts[0])
	statPart := strings.TrimSpace(parts[1])

	fs := FileStat{}

	// Detect renames.
	fs.Path, fs.OldPath, fs.Renamed = parseRenamePath(namePart)

	// Detect binary changes.
	if reBinary.MatchString(statPart) {
		fs.Binary = true
		return fs, nil
	}

	// Parse insertions/deletions from the stat part.
	// The stat part looks like "12 +++++-------" or just "0".
	fs.Insertions, fs.Deletions = parseStatCounts(statPart)

	return fs, nil
}

// parseRenamePath extracts old and new paths from a rename expression.
// Returns (newPath, oldPath, isRenamed).
func parseRenamePath(name string) (string, string, bool) {
	// Try brace rename: "prefix{old => new}suffix"
	if idx := strings.Index(name, "{"); idx >= 0 {
		if endIdx := strings.Index(name, "}"); endIdx > idx {
			prefix := name[:idx]
			suffix := name[endIdx+1:]
			inner := name[idx+1 : endIdx]
			arrowParts := strings.SplitN(inner, " => ", 2)
			if len(arrowParts) == 2 {
				oldPath := cleanPath(prefix + arrowParts[0] + suffix)
				newPath := cleanPath(prefix + arrowParts[1] + suffix)
				return newPath, oldPath, true
			}
		}
	}

	// Try full-path rename: "old/path => new/path"
	if arrowParts := strings.SplitN(name, " => ", 2); len(arrowParts) == 2 {
		return strings.TrimSpace(arrowParts[1]), strings.TrimSpace(arrowParts[0]), true
	}

	return name, "", false
}

// cleanPath removes empty segments caused by brace expansion, e.g.
// "src//file.go" => "src/file.go", and trims whitespace and leading slashes.
func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	p = strings.TrimPrefix(p, "/")
	return p
}

// parseStatCounts extracts insertions and deletions from the stat column.
// Input examples: "12 +++++-------", "3 +++", "0".
func parseStatCounts(stat string) (insertions, deletions int) {
	fields := strings.Fields(stat)
	if len(fields) == 0 {
		return 0, 0
	}

	// If there is a bar graph (plus/minus chars), count them.
	if len(fields) >= 2 {
		graph := fields[1]
		for _, ch := range graph {
			switch ch {
			case '+':
				insertions++
			case '-':
				deletions++
			}
		}
		return insertions, deletions
	}

	// Single number with no graph means zero changes shown (e.g. "0").
	if n, err := strconv.Atoi(fields[0]); err == nil {
		_ = n // no graph means we cannot distinguish insertions from deletions
		return 0, 0
	}

	return 0, 0
}

// parseSummaryLine parses the summary line from git diff --stat, e.g.:
// " 3 files changed, 10 insertions(+), 5 deletions(-)"
func parseSummaryLine(summary string) (files, insertions, deletions int) {
	for _, part := range strings.Split(summary, ",") {
		part = strings.TrimSpace(part)
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		switch {
		case strings.Contains(part, "file"):
			files = n
		case strings.Contains(part, "insertion"):
			insertions = n
		case strings.Contains(part, "deletion"):
			deletions = n
		}
	}
	return files, insertions, deletions
}
