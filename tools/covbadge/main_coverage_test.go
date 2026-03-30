package main

import (
	"os"
	"strings"
	"testing"
)

func TestBadgeColor(t *testing.T) {
	tests := []struct {
		pct   float64
		color string
	}{
		{100.0, "#4c1"},
		{80.0, "#4c1"},
		{79.9, "#dfb317"},
		{60.0, "#dfb317"},
		{59.9, "#e05d44"},
		{0.0, "#e05d44"},
	}
	for _, tt := range tests {
		got := badgeColor(tt.pct)
		if got != tt.color {
			t.Errorf("badgeColor(%.1f) = %q, want %q", tt.pct, got, tt.color)
		}
	}
}

func TestRenderBadge_ContainsValue(t *testing.T) {
	tests := []float64{0.0, 50.0, 82.3, 100.0}
	for _, pct := range tests {
		svg := renderBadge(pct)
		if !strings.Contains(svg, "<svg") {
			t.Errorf("renderBadge(%.1f) missing <svg tag", pct)
		}
		// Should contain the percentage string.
		pctStr := strings.TrimSuffix(strings.TrimSuffix(
			strings.Replace(strings.Replace(strings.Replace(strings.Replace(
				svg, "\n", " ", -1), "\t", " ", -1), "  ", " ", -1), "  ", " ", -1),
			""), "")
		_ = pctStr
		if !strings.Contains(svg, "coverage") {
			t.Errorf("renderBadge(%.1f) missing label 'coverage'", pct)
		}
	}
}

func TestRenderBadge_ColorsForThresholds(t *testing.T) {
	// 85% should be green.
	svg85 := renderBadge(85.0)
	if !strings.Contains(svg85, "#4c1") {
		t.Error("85% badge should be green (#4c1)")
	}
	// 70% should be yellow.
	svg70 := renderBadge(70.0)
	if !strings.Contains(svg70, "#dfb317") {
		t.Error("70% badge should be yellow (#dfb317)")
	}
	// 40% should be red.
	svg40 := renderBadge(40.0)
	if !strings.Contains(svg40, "#e05d44") {
		t.Error("40% badge should be red (#e05d44)")
	}
}

func TestParseCoverageProfile_Basic(t *testing.T) {
	// Write a minimal coverage profile to a temp file.
	content := "mode: set\ngithub.com/foo/bar/file.go:1.1,5.2 3 1\ngithub.com/foo/bar/file.go:6.1,10.2 2 0\n"
	f, err := os.CreateTemp("", "cov*.out")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	pct, err := parseCoverageProfile(f.Name())
	if err != nil {
		t.Fatalf("parseCoverageProfile error: %v", err)
	}
	// 3 covered out of 5 total = 60%.
	if pct < 59.9 || pct > 60.1 {
		t.Errorf("parseCoverageProfile = %.2f%%, want ~60%%", pct)
	}
}

func TestParseCoverageProfile_AllCovered(t *testing.T) {
	content := "mode: set\nfoo/bar.go:1.1,5.2 4 1\nfoo/bar.go:6.1,8.2 2 3\n"
	f, err := os.CreateTemp("", "cov*.out")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	pct, err := parseCoverageProfile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pct < 99.9 {
		t.Errorf("all covered should be 100%%, got %.2f%%", pct)
	}
}

func TestParseCoverageProfile_Empty(t *testing.T) {
	content := "mode: set\n"
	f, err := os.CreateTemp("", "cov*.out")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	pct, err := parseCoverageProfile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pct != 0.0 {
		t.Errorf("empty profile = %.2f%%, want 0", pct)
	}
}

func TestParseCoverageProfile_NotFound(t *testing.T) {
	_, err := parseCoverageProfile("/tmp/nonexistent-coverage-file-xyz.out")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
