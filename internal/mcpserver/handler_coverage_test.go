package mcpserver

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleCoverageReportNoRepo(t *testing.T) {
	srv := &Server{}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleCoverageReport(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo param")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !containsSubstring(text, "INVALID_PARAMS") {
		t.Errorf("expected INVALID_PARAMS in error, got: %s", text)
	}
}

func TestParseCoverFuncOutput(t *testing.T) {
	input := `github.com/foo/bar/pkg.go:10:	FuncA		100.0%
github.com/foo/bar/pkg.go:20:	FuncB		50.0%
github.com/foo/bar/baz/other.go:5:	FuncC		80.0%
total:					(statements)	75.0%
`
	pkgs, overall := parseCoverOutput(input)

	if overall != 75.0 {
		t.Errorf("expected overall 75.0, got %v", overall)
	}

	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}

	// First package: github.com/foo/bar — average of 100.0 and 50.0 = 75.0
	if pkgs[0].Name != "github.com/foo/bar" {
		t.Errorf("expected package github.com/foo/bar, got %s", pkgs[0].Name)
	}
	if pkgs[0].Coverage != 75.0 {
		t.Errorf("expected coverage 75.0 for first package, got %v", pkgs[0].Coverage)
	}

	// Second package: github.com/foo/bar/baz — 80.0
	if pkgs[1].Name != "github.com/foo/bar/baz" {
		t.Errorf("expected package github.com/foo/bar/baz, got %s", pkgs[1].Name)
	}
	if pkgs[1].Coverage != 80.0 {
		t.Errorf("expected coverage 80.0 for second package, got %v", pkgs[1].Coverage)
	}
}

func TestParseCoverFuncOutputEmpty(t *testing.T) {
	pkgs, overall := parseCoverOutput("")
	if overall != 0 {
		t.Errorf("expected overall 0, got %v", overall)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages, got %d", len(pkgs))
	}
}

func TestCoverageThresholdComparison(t *testing.T) {
	tests := []struct {
		name      string
		coverage  float64
		threshold float64
		wantPass  bool
	}{
		{"above threshold", 80.0, 70.0, true},
		{"at threshold", 70.0, 70.0, true},
		{"below threshold", 65.0, 70.0, false},
		{"zero coverage", 0.0, 70.0, false},
		{"zero threshold", 50.0, 0.0, true},
		{"high threshold", 99.0, 100.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pass := tt.coverage >= tt.threshold
			if pass != tt.wantPass {
				t.Errorf("coverage %.1f >= threshold %.1f: got %v, want %v",
					tt.coverage, tt.threshold, pass, tt.wantPass)
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
