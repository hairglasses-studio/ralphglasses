package review

import (
	"fmt"
	"strings"
	"testing"
)

func TestFinding_Key(t *testing.T) {
	tests := []struct {
		name    string
		finding Finding
		want    string
	}{
		{
			name: "basic key",
			finding: Finding{
				CriterionID: "SEC001",
				File:        "main.go",
				Line:        10,
				DiffLine:    `password = "x"`,
			},
			want: `SEC001:main.go:10:password = "x"`,
		},
		{
			name: "empty fields",
			finding: Finding{
				CriterionID: "",
				File:        "",
				Line:        0,
				DiffLine:    "",
			},
			want: "::0:",
		},
		{
			name: "special characters in diff line",
			finding: Finding{
				CriterionID: "COR003",
				File:        "pkg/util.go",
				Line:        99,
				DiffLine:    `panic("oh:no")`,
			},
			want: `COR003:pkg/util.go:99:panic("oh:no")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.finding.key()
			if got != tt.want {
				t.Errorf("key() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReviewResult_ErrorCount(t *testing.T) {
	tests := []struct {
		name     string
		findings []Finding
		want     int
	}{
		{"no findings", nil, 0},
		{"no errors", []Finding{{Severity: SeverityWarning}, {Severity: SeverityInfo}}, 0},
		{"all errors", []Finding{{Severity: SeverityError}, {Severity: SeverityError}}, 2},
		{"mixed", []Finding{{Severity: SeverityError}, {Severity: SeverityWarning}, {Severity: SeverityError}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReviewResult{Findings: tt.findings}
			if got := r.ErrorCount(); got != tt.want {
				t.Errorf("ErrorCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestReviewResult_WarningCount(t *testing.T) {
	tests := []struct {
		name     string
		findings []Finding
		want     int
	}{
		{"no findings", nil, 0},
		{"no warnings", []Finding{{Severity: SeverityError}, {Severity: SeverityInfo}}, 0},
		{"all warnings", []Finding{{Severity: SeverityWarning}, {Severity: SeverityWarning}}, 2},
		{"mixed", []Finding{{Severity: SeverityError}, {Severity: SeverityWarning}, {Severity: SeverityInfo}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReviewResult{Findings: tt.findings}
			if got := r.WarningCount(); got != tt.want {
				t.Errorf("WarningCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNewReviewAgent_NilCriteria(t *testing.T) {
	agent := NewReviewAgent(nil)
	if agent == nil {
		t.Fatal("NewReviewAgent returned nil")
	}
	if agent.FuncLengthThreshold != 60 {
		t.Errorf("FuncLengthThreshold = %d, want 60", agent.FuncLengthThreshold)
	}
	if agent.criteria == nil {
		t.Error("criteria should default to DefaultCriteria()")
	}
	if agent.criteria.Len() != DefaultCriteria().Len() {
		t.Errorf("criteria len = %d, want %d", agent.criteria.Len(), DefaultCriteria().Len())
	}
}

func TestNewReviewAgent_CustomCriteria(t *testing.T) {
	cs := SecurityCriteria()
	agent := NewReviewAgent(cs)
	if agent.criteria.Len() != cs.Len() {
		t.Errorf("criteria len = %d, want %d", agent.criteria.Len(), cs.Len())
	}
}

func TestReviewAgent_Analyze_EmptyDiff(t *testing.T) {
	agent := NewReviewAgent(nil)
	result := agent.Analyze("")
	if result.FilesCount != 0 {
		t.Errorf("FilesCount = %d, want 0", result.FilesCount)
	}
	if len(result.Findings) != 0 {
		t.Errorf("Findings = %d, want 0", len(result.Findings))
	}
	if !strings.Contains(result.Summary, "No issues found") {
		t.Errorf("Summary = %q, want 'No issues found' prefix", result.Summary)
	}
}

func TestReviewAgent_Analyze_MultipleFiles(t *testing.T) {
	diff := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,4 @@
 package a
+// added line
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,3 +1,4 @@
 package b
+// another line
`
	agent := NewReviewAgent(nil)
	result := agent.Analyze(diff)
	if result.FilesCount != 2 {
		t.Errorf("FilesCount = %d, want 2", result.FilesCount)
	}
}

func TestReviewAgent_Analyze_LongFunction(t *testing.T) {
	// Build a diff with a function exceeding the threshold.
	var b strings.Builder
	b.WriteString("diff --git a/big.go b/big.go\n")
	b.WriteString("--- a/big.go\n")
	b.WriteString("+++ b/big.go\n")
	b.WriteString("@@ -1,0 +1,100 @@\n")
	b.WriteString("+func BigFunc() {\n")
	for i := 0; i < 65; i++ {
		b.WriteString(fmt.Sprintf("+\tx := %d\n", i))
	}
	b.WriteString("+}\n")

	agent := NewReviewAgent(nil)
	agent.FuncLengthThreshold = 10 // Low threshold for test.
	result := agent.Analyze(b.String())

	var foundSTY001 bool
	for _, f := range result.Findings {
		if f.CriterionID == "STY001" {
			foundSTY001 = true
			if f.File != "big.go" {
				t.Errorf("STY001 file = %q, want big.go", f.File)
			}
			break
		}
	}
	if !foundSTY001 {
		t.Error("expected STY001 finding for long function")
	}
}

func TestReviewAgent_Analyze_LongFunctionBelowThreshold(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/small.go b/small.go\n")
	b.WriteString("--- a/small.go\n")
	b.WriteString("+++ b/small.go\n")
	b.WriteString("@@ -1,0 +1,10 @@\n")
	b.WriteString("+func SmallFunc() {\n")
	for i := 0; i < 5; i++ {
		b.WriteString(fmt.Sprintf("+\tx := %d\n", i))
	}
	b.WriteString("+}\n")

	agent := NewReviewAgent(nil)
	agent.FuncLengthThreshold = 60
	result := agent.Analyze(b.String())

	for _, f := range result.Findings {
		if f.CriterionID == "STY001" {
			t.Error("did not expect STY001 for a short function")
		}
	}
}

func TestReviewAgent_Analyze_RemovedLinesDoNotAdvanceLineNum(t *testing.T) {
	diff := `diff --git a/r.go b/r.go
--- a/r.go
+++ b/r.go
@@ -10,5 +10,5 @@
-old line
-old line 2
+panic("bad")
`
	agent := NewReviewAgent(nil)
	result := agent.Analyze(diff)

	for _, f := range result.Findings {
		if f.CriterionID == "COR003" {
			// The panic is at +10 (hunk starts at 10-1=9, then incremented to 10).
			if f.Line != 10 {
				t.Errorf("COR003 line = %d, want 10", f.Line)
			}
			return
		}
	}
	t.Error("expected COR003 finding for panic")
}

func TestReviewAgent_Analyze_DeduplicatesFindings(t *testing.T) {
	// Same line appearing twice should only produce one finding.
	diff := `diff --git a/d.go b/d.go
--- a/d.go
+++ b/d.go
@@ -1,0 +1,2 @@
+panic("a")
`
	agent := NewReviewAgent(nil)
	result := agent.Analyze(diff)

	panicCount := 0
	for _, f := range result.Findings {
		if f.CriterionID == "COR003" {
			panicCount++
		}
	}
	if panicCount > 1 {
		t.Errorf("COR003 appeared %d times, expected at most 1", panicCount)
	}
}

func TestReviewAgent_Summarize_NoFindings(t *testing.T) {
	agent := NewReviewAgent(nil)
	r := &ReviewResult{FilesCount: 3}
	got := agent.summarize(r)
	if got != "No issues found in 3 file(s)." {
		t.Errorf("summarize = %q", got)
	}
}

func TestReviewAgent_Summarize_WithFindings(t *testing.T) {
	agent := NewReviewAgent(nil)
	r := &ReviewResult{
		FilesCount: 2,
		Findings: []Finding{
			{Severity: SeverityError},
			{Severity: SeverityWarning},
			{Severity: SeverityInfo},
		},
	}
	got := agent.summarize(r)
	if !strings.Contains(got, "3 finding(s)") {
		t.Errorf("summarize missing finding count: %q", got)
	}
	if !strings.Contains(got, "1 error(s)") {
		t.Errorf("summarize missing error count: %q", got)
	}
	if !strings.Contains(got, "1 warning(s)") {
		t.Errorf("summarize missing warning count: %q", got)
	}
}

func TestDeduplicate_Empty(t *testing.T) {
	out := Deduplicate(nil)
	if len(out) != 0 {
		t.Errorf("Deduplicate(nil) = %d, want 0", len(out))
	}
}

func TestDeduplicate_NoDuplicates(t *testing.T) {
	findings := []Finding{
		{CriterionID: "A", File: "a.go", Line: 1, DiffLine: "x"},
		{CriterionID: "B", File: "b.go", Line: 2, DiffLine: "y"},
	}
	out := Deduplicate(findings)
	if len(out) != 2 {
		t.Errorf("Deduplicate = %d, want 2", len(out))
	}
}

func TestDeduplicate_AllDuplicates(t *testing.T) {
	f := Finding{CriterionID: "A", File: "a.go", Line: 1, DiffLine: "x"}
	out := Deduplicate([]Finding{f, f, f})
	if len(out) != 1 {
		t.Errorf("Deduplicate = %d, want 1", len(out))
	}
}

func TestParseHunkLine_TableDriven(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"@@ -10,6 +10,12 @@ func init() {", 9},
		{"@@ -0,0 +1,5 @@", 0},
		{"@@ -100,3 +200,7 @@", 199},
		{"@@ -1 +1 @@", 0},
		{"no plus sign here", 0},
		{"@@ -5,3 +0,3 @@", 0}, // +0 returns 0
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseHunkLine(tt.input)
			if got != tt.want {
				t.Errorf("parseHunkLine(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestReviewAgent_Analyze_HunkHeaderResetsFunc(t *testing.T) {
	// A new hunk header should reset function tracking.
	var b strings.Builder
	b.WriteString("diff --git a/f.go b/f.go\n")
	b.WriteString("--- a/f.go\n")
	b.WriteString("+++ b/f.go\n")
	b.WriteString("@@ -1,0 +1,5 @@\n")
	b.WriteString("+func Foo() {\n")
	for i := 0; i < 3; i++ {
		b.WriteString("+\tx := 1\n")
	}
	// New hunk resets func tracking.
	b.WriteString("@@ -50,0 +50,5 @@\n")
	for i := 0; i < 3; i++ {
		b.WriteString("+\ty := 2\n")
	}

	agent := NewReviewAgent(nil)
	agent.FuncLengthThreshold = 5 // Would trigger if not reset.
	result := agent.Analyze(b.String())

	for _, f := range result.Findings {
		if f.CriterionID == "STY001" {
			t.Error("STY001 should not fire when hunk resets func tracking")
		}
	}
}
