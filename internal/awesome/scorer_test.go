package awesome

import (
	"math"
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
}

func TestNewProjectScorer_DefaultWeights(t *testing.T) {
	t.Parallel()
	s := NewProjectScorer(nil)
	if s.Weights != DefaultWeights() {
		t.Errorf("expected default weights, got %+v", s.Weights)
	}
}

func TestNewProjectScorer_CustomWeights(t *testing.T) {
	t.Parallel()
	w := ScoreWeights{Stars: 1.0, Recency: 0, Documentation: 0, License: 0, Dependencies: 0, TestCoverage: 0}
	s := NewProjectScorer(&w)
	if s.Weights.Stars != 1.0 {
		t.Errorf("stars weight = %f, want 1.0", s.Weights.Stars)
	}
}

func TestScoreWeights_Normalize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		w    ScoreWeights
	}{
		{"default", DefaultWeights()},
		{"all ones", ScoreWeights{1, 1, 1, 1, 1, 1}},
		{"uneven", ScoreWeights{5, 3, 1, 0.5, 0.3, 0.2}},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n := tc.w.normalize()
			sum := n.Stars + n.Recency + n.Documentation + n.License + n.Dependencies + n.TestCoverage
			if math.Abs(sum-1.0) > 0.001 {
				t.Errorf("normalized sum = %f, want ~1.0", sum)
			}
		})
	}
}

func TestScoreWeights_NormalizeZero(t *testing.T) {
	t.Parallel()
	n := ScoreWeights{}.normalize()
	sum := n.Stars + n.Recency + n.Documentation + n.License + n.Dependencies + n.TestCoverage
	if math.Abs(sum-1.0) > 0.001 {
		t.Errorf("zero weights normalized sum = %f, want ~1.0", sum)
	}
	// Each should be 1/6
	if math.Abs(n.Stars-1.0/6) > 0.001 {
		t.Errorf("stars = %f, want ~0.1667", n.Stars)
	}
}

func TestScoreStars(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		stars int
		min   float64
		max   float64
	}{
		{"zero", 0, 0, 0},
		{"one", 1, 0, 1},
		{"ten", 10, 24, 26},
		{"hundred", 100, 49, 51},
		{"thousand", 1000, 74, 76},
		{"ten_thousand", 10000, 99, 100},
		{"hundred_thousand", 100000, 100, 100},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scoreStars(tc.stars)
			if got < tc.min || got > tc.max {
				t.Errorf("scoreStars(%d) = %f, want [%f, %f]", tc.stars, got, tc.min, tc.max)
			}
		})
	}
}

func TestScoreRecency(t *testing.T) {
	t.Parallel()
	now := fixedNow()

	tests := []struct {
		name       string
		lastCommit time.Time
		min        float64
		max        float64
	}{
		{"today", now, 99, 100},
		{"yesterday", now.Add(-24 * time.Hour), 99, 100},
		{"30_days", now.Add(-30 * 24 * time.Hour), 95, 96.5},
		{"180_days", now.Add(-180 * 24 * time.Hour), 74, 76},
		{"365_days", now.Add(-365 * 24 * time.Hour), 49, 51},
		{"730_days", now.Add(-730 * 24 * time.Hour), 0, 0.5},
		{"old", now.Add(-1000 * 24 * time.Hour), 0, 0},
		{"zero", time.Time{}, 0, 0},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scoreRecency(tc.lastCommit, now)
			if got < tc.min || got > tc.max {
				t.Errorf("scoreRecency(%v) = %f, want [%f, %f]", tc.lastCommit, got, tc.min, tc.max)
			}
		})
	}
}

func TestScoreRecency_FutureCommit(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	future := now.Add(48 * time.Hour)
	got := scoreRecency(future, now)
	if got != 100 {
		t.Errorf("scoreRecency(future) = %f, want 100", got)
	}
}

func TestScoreDocumentation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		p    ProjectData
		min  float64
		max  float64
	}{
		{"nothing", ProjectData{}, 0, 0},
		{"readme_only", ProjectData{HasReadme: true, ReadmeLength: 50}, 30, 30},
		{"readme_100", ProjectData{HasReadme: true, ReadmeLength: 100}, 40, 40},
		{"readme_500", ProjectData{HasReadme: true, ReadmeLength: 500}, 50, 50},
		{"readme_2000", ProjectData{HasReadme: true, ReadmeLength: 2000}, 60, 60},
		{"readme_5000", ProjectData{HasReadme: true, ReadmeLength: 5000}, 70, 70},
		{"full", ProjectData{HasReadme: true, ReadmeLength: 5000, HasChangelog: true, HasExamples: true}, 100, 100},
		{"changelog_only", ProjectData{HasChangelog: true}, 15, 15},
		{"examples_only", ProjectData{HasExamples: true}, 15, 15},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scoreDocumentation(tc.p)
			if got < tc.min || got > tc.max {
				t.Errorf("scoreDocumentation() = %f, want [%f, %f]", got, tc.min, tc.max)
			}
		})
	}
}

func TestScoreLicense(t *testing.T) {
	t.Parallel()
	tests := []struct {
		license string
		want    float64
	}{
		{"MIT", 100},
		{"Apache-2.0", 100},
		{"BSD-3-Clause", 100},
		{"BSD-2-Clause", 100},
		{"ISC", 100},
		{"Unlicense", 100},
		{"0BSD", 100},
		{"MPL-2.0", 80},
		{"LGPL-2.1", 60},
		{"LGPL-3.0", 60},
		{"GPL-2.0", 40},
		{"GPL-3.0", 40},
		{"AGPL-3.0", 20},
		{"", 0},
		{"CustomLicense", 20},
		{"PROPRIETARY", 20},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.license, func(t *testing.T) {
			t.Parallel()
			got := scoreLicense(tc.license)
			if got != tc.want {
				t.Errorf("scoreLicense(%q) = %f, want %f", tc.license, got, tc.want)
			}
		})
	}
}

func TestScoreDependencies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		count int
		want  float64
	}{
		{"zero", 0, 100},
		{"negative", -1, 100},
		{"one", 1, 99},
		{"ten", 10, 90},
		{"fifty", 50, 50},
		{"ninety_nine", 99, 1},
		{"hundred", 100, 0},
		{"two_hundred", 200, 0},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scoreDependencies(tc.count)
			if got != tc.want {
				t.Errorf("scoreDependencies(%d) = %f, want %f", tc.count, got, tc.want)
			}
		})
	}
}

func TestScoreTestCoverage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		p    ProjectData
		want float64
	}{
		{"no_tests", ProjectData{HasTests: false}, 0},
		{"tests_no_coverage", ProjectData{HasTests: true, TestCoverage: 0}, 25},
		{"50_pct", ProjectData{HasTests: true, TestCoverage: 0.5}, 50},
		{"80_pct", ProjectData{HasTests: true, TestCoverage: 0.8}, 80},
		{"100_pct", ProjectData{HasTests: true, TestCoverage: 1.0}, 100},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scoreTestCoverage(tc.p)
			if got != tc.want {
				t.Errorf("scoreTestCoverage() = %f, want %f", got, tc.want)
			}
		})
	}
}

func TestLetterGrade(t *testing.T) {
	t.Parallel()
	tests := []struct {
		score float64
		want  string
	}{
		{100, "A"},
		{95, "A"},
		{90, "A"},
		{89.9, "B"},
		{85, "B"},
		{80, "B"},
		{79.9, "C"},
		{75, "C"},
		{70, "C"},
		{69.9, "D"},
		{65, "D"},
		{60, "D"},
		{59.9, "F"},
		{50, "F"},
		{0, "F"},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			got := letterGrade(tc.score)
			if got != tc.want {
				t.Errorf("letterGrade(%f) = %q, want %q", tc.score, got, tc.want)
			}
		})
	}
}

func TestClamp(t *testing.T) {
	t.Parallel()
	if clamp(-5) != 0 {
		t.Error("clamp(-5) should be 0")
	}
	if clamp(50) != 50 {
		t.Error("clamp(50) should be 50")
	}
	if clamp(150) != 100 {
		t.Error("clamp(150) should be 100")
	}
}

func TestScore_ExcellentProject(t *testing.T) {
	t.Parallel()
	s := NewProjectScorer(nil)
	s.Now = fixedNow

	p := ProjectData{
		Stars:        5000,
		LastCommit:   fixedNow().Add(-7 * 24 * time.Hour),
		HasReadme:    true,
		ReadmeLength: 8000,
		HasChangelog: true,
		HasExamples:  true,
		License:      "MIT",
		Dependencies: 5,
		TestCoverage: 0.85,
		HasTests:     true,
	}

	result := s.Score(p)
	if result.Composite < 85 {
		t.Errorf("excellent project composite = %f, want >= 85", result.Composite)
	}
	if result.Grade != "A" {
		t.Errorf("excellent project grade = %q, want A", result.Grade)
	}
	if len(result.Dimensions) != 6 {
		t.Errorf("dimensions = %d, want 6", len(result.Dimensions))
	}
}

func TestScore_PoorProject(t *testing.T) {
	t.Parallel()
	s := NewProjectScorer(nil)
	s.Now = fixedNow

	p := ProjectData{
		Stars:        2,
		LastCommit:   fixedNow().Add(-900 * 24 * time.Hour),
		HasReadme:    false,
		License:      "",
		Dependencies: 150,
		HasTests:     false,
	}

	result := s.Score(p)
	if result.Composite > 20 {
		t.Errorf("poor project composite = %f, want <= 20", result.Composite)
	}
	if result.Grade != "F" {
		t.Errorf("poor project grade = %q, want F", result.Grade)
	}
}

func TestScore_MedianProject(t *testing.T) {
	t.Parallel()
	s := NewProjectScorer(nil)
	s.Now = fixedNow

	p := ProjectData{
		Stars:        100,
		LastCommit:   fixedNow().Add(-90 * 24 * time.Hour),
		HasReadme:    true,
		ReadmeLength: 1500,
		License:      "Apache-2.0",
		Dependencies: 20,
		HasTests:     true,
		TestCoverage: 0.5,
	}

	result := s.Score(p)
	if result.Composite < 50 || result.Composite > 80 {
		t.Errorf("median project composite = %f, want 50–80", result.Composite)
	}
}

func TestScore_StarsOnlyWeight(t *testing.T) {
	t.Parallel()
	w := ScoreWeights{Stars: 1.0}
	s := NewProjectScorer(&w)
	s.Now = fixedNow

	highStars := ProjectData{Stars: 10000}
	lowStars := ProjectData{Stars: 1}

	high := s.Score(highStars)
	low := s.Score(lowStars)

	if high.Composite <= low.Composite {
		t.Errorf("high stars (%f) should beat low stars (%f)", high.Composite, low.Composite)
	}
	if high.Composite < 95 {
		t.Errorf("10k stars with stars-only weight should score ~100, got %f", high.Composite)
	}
}

func TestScore_RecencyOnlyWeight(t *testing.T) {
	t.Parallel()
	w := ScoreWeights{Recency: 1.0}
	s := NewProjectScorer(&w)
	s.Now = fixedNow

	recent := ProjectData{LastCommit: fixedNow().Add(-1 * 24 * time.Hour)}
	stale := ProjectData{LastCommit: fixedNow().Add(-700 * 24 * time.Hour)}

	recentScore := s.Score(recent)
	staleScore := s.Score(stale)

	if recentScore.Composite <= staleScore.Composite {
		t.Errorf("recent (%f) should beat stale (%f)", recentScore.Composite, staleScore.Composite)
	}
}

func TestScore_DimensionCount(t *testing.T) {
	t.Parallel()
	s := NewProjectScorer(nil)
	s.Now = fixedNow

	result := s.Score(ProjectData{})
	if len(result.Dimensions) != 6 {
		t.Fatalf("dimensions = %d, want 6", len(result.Dimensions))
	}

	names := map[string]bool{}
	for _, d := range result.Dimensions {
		names[d.Name] = true
	}
	for _, expected := range []string{"stars", "recency", "documentation", "license", "dependencies", "test_coverage"} {
		if !names[expected] {
			t.Errorf("missing dimension %q", expected)
		}
	}
}

func TestScore_CompositeRange(t *testing.T) {
	t.Parallel()
	s := NewProjectScorer(nil)
	s.Now = fixedNow

	projects := []ProjectData{
		{},
		{Stars: 50000, LastCommit: fixedNow(), HasReadme: true, ReadmeLength: 10000, HasChangelog: true, HasExamples: true, License: "MIT", Dependencies: 0, HasTests: true, TestCoverage: 1.0},
		{Stars: 1, LastCommit: fixedNow().Add(-800 * 24 * time.Hour)},
	}

	for i, p := range projects {
		result := s.Score(p)
		if result.Composite < 0 || result.Composite > 100 {
			t.Errorf("project %d: composite %f out of [0, 100] range", i, result.Composite)
		}
	}
}

func TestScore_DimensionWeightsMatch(t *testing.T) {
	t.Parallel()
	s := NewProjectScorer(nil)
	s.Now = fixedNow

	result := s.Score(ProjectData{Stars: 100, HasReadme: true, ReadmeLength: 500, License: "MIT", HasTests: true, TestCoverage: 0.5, LastCommit: fixedNow(), Dependencies: 10})

	var weightSum float64
	for _, d := range result.Dimensions {
		weightSum += d.Weight
	}
	if math.Abs(weightSum-1.0) > 0.001 {
		t.Errorf("dimension weights sum = %f, want ~1.0", weightSum)
	}
}

func TestScore_AllDimensionsHaveGrades(t *testing.T) {
	t.Parallel()
	s := NewProjectScorer(nil)
	s.Now = fixedNow

	result := s.Score(ProjectData{Stars: 100, License: "MIT", HasTests: true, TestCoverage: 0.75, LastCommit: fixedNow()})
	for _, d := range result.Dimensions {
		if d.Grade == "" {
			t.Errorf("dimension %q has empty grade", d.Name)
		}
		validGrades := map[string]bool{"A": true, "B": true, "C": true, "D": true, "F": true}
		if !validGrades[d.Grade] {
			t.Errorf("dimension %q has invalid grade %q", d.Name, d.Grade)
		}
	}
}

func TestScore_ZeroProject(t *testing.T) {
	t.Parallel()
	s := NewProjectScorer(nil)
	s.Now = fixedNow

	result := s.Score(ProjectData{})

	// Zero dependencies scores 100, so composite cannot be literally 0 with default weights.
	// But all other dimensions should be 0.
	for _, d := range result.Dimensions {
		if d.Name == "dependencies" {
			if d.Score != 100 {
				t.Errorf("zero-deps score = %f, want 100", d.Score)
			}
			continue
		}
		if d.Score != 0 {
			t.Errorf("dimension %q score = %f, want 0 for zero project", d.Name, d.Score)
		}
	}
}
