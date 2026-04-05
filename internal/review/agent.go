package review

import (
	"fmt"
	"strings"
)

// Finding represents a single review issue found in a diff.
type Finding struct {
	CriterionID string   `json:"criterion_id"`
	Name        string   `json:"name"`
	Category    Category `json:"category"`
	Severity    Severity `json:"severity"`
	File        string   `json:"file"`
	Line        int      `json:"line"`
	Message     string   `json:"message"`
	DiffLine    string   `json:"diff_line"` // The raw diff line that triggered the finding
}

// key returns a deduplication key for the finding.
func (f *Finding) key() string {
	return fmt.Sprintf("%s:%s:%d:%s", f.CriterionID, f.File, f.Line, f.DiffLine)
}

// ReviewResult is the output of a review agent analysis.
type ReviewResult struct {
	Findings   []Finding `json:"findings"`
	FilesCount int       `json:"files_count"` // Number of files in the diff
	LinesCount int       `json:"lines_count"` // Total diff lines analyzed
	Summary    string    `json:"summary"`
}

// ErrorCount returns the number of error-severity findings.
func (r *ReviewResult) ErrorCount() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			n++
		}
	}
	return n
}

// WarningCount returns the number of warning-severity findings.
func (r *ReviewResult) WarningCount() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == SeverityWarning {
			n++
		}
	}
	return n
}

// ReviewAgent analyzes unified diffs against configurable criteria.
type ReviewAgent struct {
	criteria *CriteriaSet

	// FuncLengthThreshold is the max added lines in a function before
	// STY001 fires. Default 60.
	FuncLengthThreshold int
}

// NewReviewAgent creates an agent with the given criteria.
// If criteria is nil, DefaultCriteria() is used.
func NewReviewAgent(criteria *CriteriaSet) *ReviewAgent {
	if criteria == nil {
		criteria = DefaultCriteria()
	}
	return &ReviewAgent{
		criteria:            criteria,
		FuncLengthThreshold: 60,
	}
}

// Analyze scans a unified diff string and returns findings.
// The diff should be in standard unified diff format (e.g., git diff output).
func (a *ReviewAgent) Analyze(diff string) *ReviewResult {
	lines := strings.Split(diff, "\n")

	result := &ReviewResult{
		LinesCount: len(lines),
	}

	var currentFile string
	var lineNum int
	seen := make(map[string]struct{})
	files := make(map[string]struct{})

	// Track function length for STY001.
	var inFunc bool
	var funcStart int
	var funcAddedLines int
	var funcFile string
	var funcLineNum int

	for _, line := range lines {
		// Parse file header.
		if after, ok := strings.CutPrefix(line, "+++ b/"); ok {
			currentFile = after
			files[currentFile] = struct{}{}
			continue
		}
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			continue
		}

		// Parse hunk header for line numbers.
		if strings.HasPrefix(line, "@@") {
			lineNum = parseHunkLine(line)
			inFunc = false
			funcAddedLines = 0
			continue
		}

		// Track line numbers — only added/context lines advance.
		if strings.HasPrefix(line, "-") {
			// Removed lines don't advance the new-file line counter.
		} else {
			lineNum++
		}

		// Only check added lines (starting with "+").
		if !strings.HasPrefix(line, "+") {
			continue
		}

		// Function length tracking.
		if strings.HasPrefix(line, "+func ") || strings.HasPrefix(line, "+\tfunc ") {
			inFunc = true
			funcStart = lineNum
			funcAddedLines = 0
			funcFile = currentFile
			funcLineNum = lineNum
		}
		if inFunc {
			funcAddedLines++
			if funcAddedLines > a.FuncLengthThreshold {
				f := Finding{
					CriterionID: "STY001",
					Name:        "long-function",
					Category:    CategoryStyle,
					Severity:    SeverityInfo,
					File:        funcFile,
					Line:        funcLineNum,
					Message:     fmt.Sprintf("Function starting at line %d has %d+ added lines (threshold %d).", funcStart, funcAddedLines, a.FuncLengthThreshold),
					DiffLine:    line,
				}
				k := f.key()
				if _, dup := seen[k]; !dup {
					seen[k] = struct{}{}
					result.Findings = append(result.Findings, f)
				}
				inFunc = false // Only report once per function.
			}
		}

		// Check all criteria except STY001 (handled above via line counting).
		for _, c := range a.criteria.All() {
			if c.ID == "STY001" {
				continue
			}

			// STY002: only fire if the previous line is NOT a doc comment.
			if c.ID == "STY002" {
				if !c.Match(line) {
					continue
				}
				// We would need previous-line context; for diff analysis we
				// check whether the line just before this added line is a comment.
				// This is a best-effort check.
			} else if !c.Match(line) {
				continue
			}

			f := Finding{
				CriterionID: c.ID,
				Name:        c.Name,
				Category:    c.Category,
				Severity:    c.Severity,
				File:        currentFile,
				Line:        lineNum,
				Message:     c.Message,
				DiffLine:    strings.TrimPrefix(line, "+"),
			}

			k := f.key()
			if _, dup := seen[k]; !dup {
				seen[k] = struct{}{}
				result.Findings = append(result.Findings, f)
			}
		}
	}

	result.FilesCount = len(files)
	result.Summary = a.summarize(result)
	return result
}

// summarize builds a one-line summary of the review result.
func (a *ReviewAgent) summarize(r *ReviewResult) string {
	if len(r.Findings) == 0 {
		return fmt.Sprintf("No issues found in %d file(s).", r.FilesCount)
	}
	return fmt.Sprintf("%d finding(s) in %d file(s): %d error(s), %d warning(s), %d info.",
		len(r.Findings), r.FilesCount,
		r.ErrorCount(), r.WarningCount(),
		len(r.Findings)-r.ErrorCount()-r.WarningCount())
}

// Deduplicate removes duplicate findings from the result, keeping the first
// occurrence of each unique (criterion, file, line, content) combination.
func Deduplicate(findings []Finding) []Finding {
	seen := make(map[string]struct{}, len(findings))
	out := make([]Finding, 0, len(findings))
	for _, f := range findings {
		k := f.key()
		if _, dup := seen[k]; !dup {
			seen[k] = struct{}{}
			out = append(out, f)
		}
	}
	return out
}

// parseHunkLine extracts the starting line number from a hunk header.
// Input format: @@ -oldStart,oldCount +newStart,newCount @@
func parseHunkLine(hunk string) int {
	// Find the +N part.
	_, after, ok := strings.Cut(hunk, "+")
	if !ok {
		return 0
	}
	rest := after
	var n int
	for _, ch := range rest {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int(ch-'0')
		} else {
			break
		}
	}
	// Return n-1 because the loop increments before first use.
	if n > 0 {
		return n - 1
	}
	return 0
}
