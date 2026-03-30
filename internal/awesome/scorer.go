package awesome

import (
	"math"
	"strings"
	"time"
)

// ScoreWeights controls the relative importance of each scoring dimension.
// All weights are normalized to sum to 1.0 before use, so absolute values
// only matter relative to each other.
type ScoreWeights struct {
	Stars         float64 `json:"stars"`
	Recency       float64 `json:"recency"`
	Documentation float64 `json:"documentation"`
	License       float64 `json:"license"`
	Dependencies  float64 `json:"dependencies"`
	TestCoverage  float64 `json:"test_coverage"`
}

// DefaultWeights returns the default scoring weights.
func DefaultWeights() ScoreWeights {
	return ScoreWeights{
		Stars:         0.25,
		Recency:       0.20,
		Documentation: 0.20,
		License:       0.10,
		Dependencies:  0.10,
		TestCoverage:  0.15,
	}
}

// normalize returns weights that sum to 1.0. If all weights are zero,
// it returns equal weights across all dimensions.
func (w ScoreWeights) normalize() ScoreWeights {
	sum := w.Stars + w.Recency + w.Documentation + w.License + w.Dependencies + w.TestCoverage
	if sum == 0 {
		return ScoreWeights{
			Stars:         1.0 / 6,
			Recency:       1.0 / 6,
			Documentation: 1.0 / 6,
			License:       1.0 / 6,
			Dependencies:  1.0 / 6,
			TestCoverage:  1.0 / 6,
		}
	}
	return ScoreWeights{
		Stars:         w.Stars / sum,
		Recency:       w.Recency / sum,
		Documentation: w.Documentation / sum,
		License:       w.License / sum,
		Dependencies:  w.Dependencies / sum,
		TestCoverage:  w.TestCoverage / sum,
	}
}

// ProjectData holds the raw signals used to score a project.
type ProjectData struct {
	Stars         int       `json:"stars"`
	LastCommit    time.Time `json:"last_commit"`
	HasReadme     bool      `json:"has_readme"`
	HasChangelog  bool      `json:"has_changelog"`
	HasExamples   bool      `json:"has_examples"`
	ReadmeLength  int       `json:"readme_length"`  // bytes
	License       string    `json:"license"`         // SPDX identifier, e.g. "MIT", "Apache-2.0"
	Dependencies  int       `json:"dependencies"`    // direct dependency count
	TestCoverage  float64   `json:"test_coverage"`   // 0.0–1.0
	HasTests      bool      `json:"has_tests"`
}

// DimensionScore holds the sub-score and letter grade for one dimension.
type DimensionScore struct {
	Name   string  `json:"name"`
	Score  float64 `json:"score"`  // 0–100
	Grade  string  `json:"grade"`  // A, B, C, D, F
	Weight float64 `json:"weight"` // normalized weight used
}

// ProjectScore is the complete scoring result for one project.
type ProjectScore struct {
	Composite  float64          `json:"composite"`  // 0–100 weighted score
	Grade      string           `json:"grade"`      // overall letter grade
	Dimensions []DimensionScore `json:"dimensions"`
}

// ProjectScorer rates awesome-list projects on multiple dimensions.
type ProjectScorer struct {
	Weights ScoreWeights
	Now     func() time.Time // injectable clock for testing
}

// NewProjectScorer creates a scorer with the given weights. If weights is nil,
// DefaultWeights are used.
func NewProjectScorer(weights *ScoreWeights) *ProjectScorer {
	w := DefaultWeights()
	if weights != nil {
		w = *weights
	}
	return &ProjectScorer{
		Weights: w,
		Now:     time.Now,
	}
}

// Score computes a composite 0–100 score for the given project data.
func (s *ProjectScorer) Score(p ProjectData) ProjectScore {
	nw := s.Weights.normalize()
	now := s.Now()

	dims := []DimensionScore{
		{Name: "stars", Score: scoreStars(p.Stars), Weight: nw.Stars},
		{Name: "recency", Score: scoreRecency(p.LastCommit, now), Weight: nw.Recency},
		{Name: "documentation", Score: scoreDocumentation(p), Weight: nw.Documentation},
		{Name: "license", Score: scoreLicense(p.License), Weight: nw.License},
		{Name: "dependencies", Score: scoreDependencies(p.Dependencies), Weight: nw.Dependencies},
		{Name: "test_coverage", Score: scoreTestCoverage(p), Weight: nw.TestCoverage},
	}

	for i := range dims {
		dims[i].Grade = letterGrade(dims[i].Score)
	}

	var composite float64
	for _, d := range dims {
		composite += d.Score * d.Weight
	}
	composite = math.Round(composite*100) / 100

	return ProjectScore{
		Composite:  composite,
		Grade:      letterGrade(composite),
		Dimensions: dims,
	}
}

// scoreStars maps star count to 0–100 using a logarithmic curve.
// 0 stars → 0, 10 → ~33, 100 → ~67, 1000 → ~90, 10000+ → 100.
func scoreStars(stars int) float64 {
	if stars <= 0 {
		return 0
	}
	// log10(stars) / log10(10000) * 100, capped at 100
	score := math.Log10(float64(stars)) / math.Log10(10000) * 100
	return clamp(score)
}

// scoreRecency maps time since last commit to 0–100.
// Same day → 100, 30 days → ~80, 180 days → ~50, 365 days → ~25, 730+ days → 0.
func scoreRecency(lastCommit time.Time, now time.Time) float64 {
	if lastCommit.IsZero() {
		return 0
	}
	days := now.Sub(lastCommit).Hours() / 24
	if days < 0 {
		days = 0
	}
	if days >= 730 {
		return 0
	}
	// Linear decay over 730 days
	return clamp((1 - days/730) * 100)
}

// scoreDocumentation scores based on presence and quality of docs.
func scoreDocumentation(p ProjectData) float64 {
	var score float64

	// README presence and length
	if p.HasReadme {
		score += 30
		// Bonus for substantive README (up to 40 more points)
		switch {
		case p.ReadmeLength >= 5000:
			score += 40
		case p.ReadmeLength >= 2000:
			score += 30
		case p.ReadmeLength >= 500:
			score += 20
		case p.ReadmeLength >= 100:
			score += 10
		}
	}

	// Changelog
	if p.HasChangelog {
		score += 15
	}

	// Examples
	if p.HasExamples {
		score += 15
	}

	return clamp(score)
}

// compatibleLicenses lists SPDX identifiers considered compatible for
// open-source Go projects.
var compatibleLicenses = map[string]float64{
	"MIT":          100,
	"Apache-2.0":   100,
	"BSD-2-Clause": 100,
	"BSD-3-Clause": 100,
	"ISC":          100,
	"MPL-2.0":       80,
	"LGPL-2.1":      60,
	"LGPL-3.0":      60,
	"GPL-2.0":       40,
	"GPL-3.0":       40,
	"AGPL-3.0":      20,
	"Unlicense":    100,
	"0BSD":         100,
}

// scoreLicense returns 0–100 based on license compatibility.
func scoreLicense(license string) float64 {
	if license == "" {
		return 0
	}
	normalized := strings.TrimSpace(license)
	if score, ok := compatibleLicenses[normalized]; ok {
		return score
	}
	// Unknown license gets a baseline score
	return 20
}

// scoreDependencies rewards fewer direct dependencies. Zero deps is ideal.
func scoreDependencies(count int) float64 {
	if count <= 0 {
		return 100
	}
	if count >= 100 {
		return 0
	}
	// Linear decay: 0 deps → 100, 100 deps → 0
	return clamp(float64(100-count) / 100 * 100)
}

// scoreTestCoverage combines test presence with coverage percentage.
func scoreTestCoverage(p ProjectData) float64 {
	if !p.HasTests {
		return 0
	}
	// HasTests without coverage data gets a baseline
	if p.TestCoverage <= 0 {
		return 25
	}
	// Coverage percentage (0–1) maps directly to 0–100
	return clamp(p.TestCoverage * 100)
}

// letterGrade converts a 0–100 score to A/B/C/D/F.
func letterGrade(score float64) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

// clamp restricts a value to [0, 100].
func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
