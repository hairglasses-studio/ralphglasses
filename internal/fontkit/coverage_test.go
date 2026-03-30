package fontkit

import (
	"context"
	"os"
	"strings"
	"testing"
)

// mockChecker is a GlyphChecker that reports coverage for a configurable
// set of rune ranges. Tests build it to simulate various fonts.
type mockChecker struct {
	covered map[rune]bool
}

func newMockChecker(ranges ...UnicodeRange) *mockChecker {
	m := &mockChecker{covered: make(map[rune]bool)}
	for _, r := range ranges {
		for c := r.Start; c <= r.End; c++ {
			m.covered[c] = true
		}
	}
	return m
}

func (m *mockChecker) HasGlyph(r rune) bool {
	return m.covered[r]
}

// --- UnicodeRange ---

func TestUnicodeRange_Size(t *testing.T) {
	tests := []struct {
		name string
		r    UnicodeRange
		want int
	}{
		{"single codepoint", UnicodeRange{Start: 0x2500, End: 0x2500}, 1},
		{"box drawing", UnicodeRange{Name: "Box Drawing", Start: 0x2500, End: 0x257F}, 128},
		{"block elements", UnicodeRange{Name: "Block Elements", Start: 0x2580, End: 0x259F}, 32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.Size(); got != tt.want {
				t.Errorf("Size() = %d, want %d", got, tt.want)
			}
		})
	}
}

// --- RequiredRanges ---

func TestRequiredRanges_NonEmpty(t *testing.T) {
	if len(RequiredRanges) == 0 {
		t.Fatal("RequiredRanges should not be empty")
	}
	for _, r := range RequiredRanges {
		if r.Name == "" {
			t.Error("range has empty name")
		}
		if r.Start > r.End {
			t.Errorf("range %s: Start (0x%X) > End (0x%X)", r.Name, r.Start, r.End)
		}
	}
}

func TestRequiredRanges_ContainsExpectedBlocks(t *testing.T) {
	names := make(map[string]bool)
	for _, r := range RequiredRanges {
		names[r.Name] = true
	}
	for _, want := range []string{
		"Box Drawing",
		"Block Elements",
		"Powerline",
		"Devicons",
		"Font Awesome",
	} {
		if !names[want] {
			t.Errorf("RequiredRanges missing expected block %q", want)
		}
	}
}

// --- AnalyzeCoverage ---

func TestAnalyzeCoverage_NerdFont_FullCoverage(t *testing.T) {
	// Nerd Font should cover everything via the default heuristic checker.
	report := AnalyzeCoverage(MonaspiceNe, nil)

	if report.FontName != MonaspiceNe.Name {
		t.Errorf("FontName = %q, want %q", report.FontName, MonaspiceNe.Name)
	}
	if !report.IsNerdFont {
		t.Error("IsNerdFont should be true for MonaspiceNe")
	}
	if report.OverallPct < 99.9 {
		t.Errorf("OverallPct = %.1f, want ~100 for Nerd Font", report.OverallPct)
	}
	if report.TotalCovered != report.TotalRequired {
		t.Errorf("TotalCovered (%d) != TotalRequired (%d)", report.TotalCovered, report.TotalRequired)
	}
	if len(report.Missing) != 0 {
		t.Errorf("Missing ranges = %d, want 0 for Nerd Font", len(report.Missing))
	}
}

func TestAnalyzeCoverage_StandardFont_PartialCoverage(t *testing.T) {
	// A standard (non-Nerd) Monaspace covers box drawing etc. but not
	// Powerline/icon ranges.
	report := AnalyzeCoverage(MonaspaceNeon, nil)

	if report.IsNerdFont {
		t.Error("IsNerdFont should be false for MonaspaceNeon")
	}
	if report.OverallPct >= 100.0 {
		t.Errorf("OverallPct = %.1f, should be < 100 for non-Nerd font", report.OverallPct)
	}
	if len(report.Missing) == 0 {
		t.Error("non-Nerd font should have missing ranges")
	}

	// Box drawing and block elements should be covered.
	for _, rc := range report.Ranges {
		if rc.Range.Name == "Box Drawing" && rc.Percentage < 99.9 {
			t.Errorf("Box Drawing coverage = %.1f%%, should be ~100%%", rc.Percentage)
		}
		if rc.Range.Name == "Block Elements" && rc.Percentage < 99.9 {
			t.Errorf("Block Elements coverage = %.1f%%, should be ~100%%", rc.Percentage)
		}
	}

	// Powerline should be missing.
	foundPowerlineMissing := false
	for _, m := range report.Missing {
		if m.Name == "Powerline" {
			foundPowerlineMissing = true
		}
	}
	if !foundPowerlineMissing {
		t.Error("Powerline should be in Missing for non-Nerd font")
	}
}

func TestAnalyzeCoverage_MockChecker_PartialRanges(t *testing.T) {
	// Mock: only cover box drawing and braille.
	mock := newMockChecker(
		UnicodeRange{Start: 0x2500, End: 0x257F}, // box drawing
		UnicodeRange{Start: 0x2800, End: 0x28FF}, // braille
	)

	report := AnalyzeCoverage(FontFamily{Name: "TestFont"}, mock)

	if report.TotalCovered == 0 {
		t.Fatal("TotalCovered should be > 0 with mock ranges")
	}
	if report.TotalCovered == report.TotalRequired {
		t.Error("TotalCovered should be < TotalRequired for partial mock")
	}

	// Check that box drawing shows 100%.
	for _, rc := range report.Ranges {
		if rc.Range.Name == "Box Drawing" {
			if rc.Percentage < 99.9 {
				t.Errorf("Box Drawing = %.1f%%, want 100%%", rc.Percentage)
			}
		}
		if rc.Range.Name == "Powerline" {
			if rc.Percentage != 0 {
				t.Errorf("Powerline = %.1f%%, want 0%%", rc.Percentage)
			}
		}
	}
}

func TestAnalyzeCoverage_MockChecker_ZeroCoverage(t *testing.T) {
	// Mock with no coverage at all.
	mock := &mockChecker{covered: make(map[rune]bool)}
	report := AnalyzeCoverage(FontFamily{Name: "EmptyFont"}, mock)

	if report.TotalCovered != 0 {
		t.Errorf("TotalCovered = %d, want 0", report.TotalCovered)
	}
	if report.OverallPct != 0 {
		t.Errorf("OverallPct = %.1f, want 0", report.OverallPct)
	}
	if len(report.Missing) != len(RequiredRanges) {
		t.Errorf("Missing = %d ranges, want %d (all)", len(report.Missing), len(RequiredRanges))
	}
}

func TestAnalyzeCoverage_MockChecker_HalfCoverage(t *testing.T) {
	// Cover only the first half of each required range.
	mock := &mockChecker{covered: make(map[rune]bool)}
	for _, ur := range RequiredRanges {
		mid := ur.Start + rune(ur.Size()/2)
		for r := ur.Start; r < mid; r++ {
			mock.covered[r] = true
		}
	}

	report := AnalyzeCoverage(FontFamily{Name: "HalfFont"}, mock)

	// Overall should be roughly 50%.
	if report.OverallPct < 40 || report.OverallPct > 60 {
		t.Errorf("OverallPct = %.1f, want ~50%%", report.OverallPct)
	}
	// No range should be in Missing (all have some coverage).
	if len(report.Missing) != 0 {
		t.Errorf("Missing = %d, want 0 (all partially covered)", len(report.Missing))
	}
}

// --- CoverageReport fields ---

func TestCoverageReport_RangesMatchRequired(t *testing.T) {
	report := AnalyzeCoverage(MonaspiceNe, nil)
	if len(report.Ranges) != len(RequiredRanges) {
		t.Errorf("Ranges count = %d, want %d", len(report.Ranges), len(RequiredRanges))
	}
}

func TestCoverageReport_TotalRequired_Positive(t *testing.T) {
	report := AnalyzeCoverage(MonaspiceNe, nil)
	if report.TotalRequired <= 0 {
		t.Errorf("TotalRequired = %d, should be positive", report.TotalRequired)
	}
}

// --- Suggestions ---

func TestSuggestions_NonNerdFont(t *testing.T) {
	report := AnalyzeCoverage(MonaspaceNeon, nil)
	if len(report.Suggestions) == 0 {
		t.Error("non-Nerd font should produce suggestions")
	}

	hasInstallSuggestion := false
	for _, s := range report.Suggestions {
		if strings.Contains(s, "Nerd Font") {
			hasInstallSuggestion = true
		}
	}
	if !hasInstallSuggestion {
		t.Error("suggestions should recommend installing a Nerd Font")
	}
}

func TestSuggestions_NerdFont_NoGaps(t *testing.T) {
	report := AnalyzeCoverage(MonaspiceNe, nil)
	// A Nerd Font with full coverage should have no suggestions about
	// missing ranges.
	for _, s := range report.Suggestions {
		if strings.Contains(s, "Missing") {
			t.Errorf("unexpected missing-range suggestion for Nerd Font: %s", s)
		}
	}
}

func TestSuggestions_LowCoverage(t *testing.T) {
	// Mock: 10% of powerline only.
	mock := &mockChecker{covered: make(map[rune]bool)}
	powerline := RequiredRanges[2] // Powerline
	count := powerline.Size() / 10
	for r := powerline.Start; r < powerline.Start+rune(count); r++ {
		mock.covered[r] = true
	}

	report := AnalyzeCoverage(FontFamily{Name: "WeakPL"}, mock)

	hasLowCovWarn := false
	for _, s := range report.Suggestions {
		if strings.Contains(s, "Low coverage") {
			hasLowCovWarn = true
		}
	}
	if !hasLowCovWarn {
		t.Error("should warn about low coverage for partially-covered range")
	}
}

// --- DetectNerdFonts ---

func TestDetectNerdFonts_NoFontsInEmptyDir(t *testing.T) {
	// Override HOME to an empty dir so no fonts are found.
	tmpDir := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", orig)

	found, err := DetectNerdFonts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Errorf("found %d fonts in empty HOME, want 0", len(found))
	}
}

func TestDetectNerdFonts_FindsMockFont(t *testing.T) {
	tmpDir := t.TempDir()
	// Create font dir structure.
	fontDir := tmpDir + "/.local/share/fonts"
	os.MkdirAll(fontDir, 0755)
	// Plant a fake Monaspice font file.
	os.WriteFile(fontDir+"/MonaspiceNeNFM-Regular.otf", []byte("fake"), 0644)

	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", orig)

	found, err := DetectNerdFonts(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	foundNe := false
	for _, f := range found {
		if f.PostScriptBase == "MonaspiceNeNFM" {
			foundNe = true
		}
	}
	if !foundNe {
		t.Error("DetectNerdFonts should find MonaspiceNe from planted file")
	}
}

func TestDetectNerdFonts_FindsOtherNerdFonts(t *testing.T) {
	tmpDir := t.TempDir()
	fontDir := tmpDir + "/.local/share/fonts"
	os.MkdirAll(fontDir, 0755)
	// Plant a FiraCode Nerd Font file.
	os.WriteFile(fontDir+"/FiraCodeNFM-Regular.ttf", []byte("fake"), 0644)

	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", orig)

	found, err := DetectNerdFonts(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	foundFira := false
	for _, f := range found {
		if f.PostScriptBase == "FiraCodeNFM" {
			foundFira = true
		}
	}
	if !foundFira {
		t.Error("DetectNerdFonts should find FiraCode Nerd Font from planted file")
	}
}

// --- SuggestMissingFonts ---

func TestSuggestMissingFonts_NoFontsInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", orig)

	suggestions, err := SuggestMissingFonts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) == 0 {
		t.Error("should suggest fonts when none installed")
	}

	hasBrew := false
	for _, s := range suggestions {
		if strings.Contains(s, "brew install") {
			hasBrew = true
		}
	}
	if !hasBrew {
		t.Error("suggestions should include brew install command")
	}
}

func TestSuggestMissingFonts_MonaspiceInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	fontDir := tmpDir + "/.local/share/fonts"
	os.MkdirAll(fontDir, 0755)
	os.WriteFile(fontDir+"/MonaspiceNeNFM-Regular.otf", []byte("fake"), 0644)

	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", orig)

	suggestions, err := SuggestMissingFonts(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	hasNoAction := false
	for _, s := range suggestions {
		if strings.Contains(s, "No action needed") {
			hasNoAction = true
		}
	}
	if !hasNoAction {
		t.Errorf("should say no action needed, got: %v", suggestions)
	}
}

// --- FormatReport ---

func TestFormatReport_ContainsKey(t *testing.T) {
	report := AnalyzeCoverage(MonaspiceNe, nil)
	out := FormatReport(report)

	checks := []string{
		"Glyph Coverage Report:",
		MonaspiceNe.Name,
		"Nerd Font",
		"Overall:",
		"Box Drawing",
		"Powerline",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("FormatReport missing %q", want)
		}
	}
}

func TestFormatReport_NonNerdFont_HasSuggestions(t *testing.T) {
	report := AnalyzeCoverage(MonaspaceNeon, nil)
	out := FormatReport(report)

	if !strings.Contains(out, "Suggestions:") {
		t.Error("non-Nerd font report should have Suggestions section")
	}
}

func TestFormatReport_EmptyFont(t *testing.T) {
	mock := &mockChecker{covered: make(map[rune]bool)}
	report := AnalyzeCoverage(FontFamily{Name: "NoGlyphs"}, mock)
	out := FormatReport(report)

	if !strings.Contains(out, "0.0%") {
		t.Error("empty font report should show 0.0%")
	}
}

// --- coverageBar ---

func TestCoverageBar(t *testing.T) {
	tests := []struct {
		pct  float64
		want string
	}{
		{0, "[--------------------]"},
		{50, "[##########----------]"},
		{100, "[####################]"},
		{25, "[#####---------------]"},
	}
	for _, tt := range tests {
		got := coverageBar(tt.pct, 20)
		if got != tt.want {
			t.Errorf("coverageBar(%.0f, 20) = %q, want %q", tt.pct, got, tt.want)
		}
	}
}

// --- nerdFontChecker ---

func TestNerdFontChecker_BoxDrawing(t *testing.T) {
	c := &nerdFontChecker{isNerdFont: false}
	// Box drawing should be covered even without Nerd Font.
	if !c.HasGlyph(0x2500) {
		t.Error("box drawing U+2500 should be covered")
	}
	if !c.HasGlyph(0x257F) {
		t.Error("box drawing U+257F should be covered")
	}
}

func TestNerdFontChecker_Powerline_NerdFont(t *testing.T) {
	c := &nerdFontChecker{isNerdFont: true}
	if !c.HasGlyph(0xE0B0) {
		t.Error("Nerd Font should cover Powerline U+E0B0")
	}
}

func TestNerdFontChecker_Powerline_StandardFont(t *testing.T) {
	c := &nerdFontChecker{isNerdFont: false}
	if c.HasGlyph(0xE0B0) {
		t.Error("standard font should NOT cover Powerline U+E0B0")
	}
}

func TestNerdFontChecker_Braille(t *testing.T) {
	c := &nerdFontChecker{isNerdFont: false}
	if !c.HasGlyph(0x2800) {
		t.Error("braille U+2800 should be covered by standard font")
	}
}

// --- knownNerdFontFamilies ---

func TestKnownNerdFontFamilies(t *testing.T) {
	families := knownNerdFontFamilies()
	if len(families) == 0 {
		t.Fatal("knownNerdFontFamilies should not be empty")
	}
	for _, f := range families {
		if !f.HasNerdGlyphs {
			t.Errorf("known NF family %q should have HasNerdGlyphs=true", f.Name)
		}
		if f.BrewCask == "" {
			t.Errorf("known NF family %q should have a BrewCask", f.Name)
		}
		if f.FilePattern == "" {
			t.Errorf("known NF family %q should have a FilePattern", f.Name)
		}
	}
}

// --- systemFontDirs ---

func TestSystemFontDirs(t *testing.T) {
	dirs := systemFontDirs("/home/test")
	if len(dirs) < 3 {
		t.Errorf("systemFontDirs returned %d dirs, want >= 3", len(dirs))
	}
	hasLocal := false
	for _, d := range dirs {
		if strings.Contains(d, ".local/share/fonts") {
			hasLocal = true
		}
	}
	if !hasLocal {
		t.Error("systemFontDirs should include ~/.local/share/fonts")
	}
}

// --- CheckRequiredRanges ---

func TestCheckRequiredRanges_NoNerdFonts(t *testing.T) {
	tmpDir := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", orig)

	report, err := CheckRequiredRanges(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.FontName != "System Monospace" {
		t.Errorf("FontName = %q, want System Monospace", report.FontName)
	}
	hasInstall := false
	for _, s := range report.Suggestions {
		if strings.Contains(s, "No Nerd Fonts detected") {
			hasInstall = true
		}
	}
	if !hasInstall {
		t.Error("should suggest installing Nerd Font when none found")
	}
}

func TestCheckRequiredRanges_WithNerdFont(t *testing.T) {
	tmpDir := t.TempDir()
	fontDir := tmpDir + "/.local/share/fonts"
	os.MkdirAll(fontDir, 0755)
	os.WriteFile(fontDir+"/MonaspiceNeNFM-Regular.otf", []byte("fake"), 0644)

	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", orig)

	report, err := CheckRequiredRanges(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.FontName != MonaspiceNe.Name {
		t.Errorf("FontName = %q, want %q", report.FontName, MonaspiceNe.Name)
	}
	if !report.IsNerdFont {
		t.Error("should be identified as Nerd Font")
	}
	if report.OverallPct < 99.9 {
		t.Errorf("OverallPct = %.1f, want ~100", report.OverallPct)
	}
}
