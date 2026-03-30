package fontkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UnicodeRange represents a named block of Unicode code points that the TUI
// needs for correct rendering.
type UnicodeRange struct {
	Name  string // Human-readable name (e.g. "Box Drawing")
	Start rune   // First code point (inclusive)
	End   rune   // Last code point (inclusive)
}

// Size returns the number of code points in the range.
func (r UnicodeRange) Size() int {
	return int(r.End-r.Start) + 1
}

// RequiredRanges lists the Unicode ranges the ralphglasses TUI depends on.
var RequiredRanges = []UnicodeRange{
	{Name: "Box Drawing", Start: 0x2500, End: 0x257F},
	{Name: "Block Elements", Start: 0x2580, End: 0x259F},
	{Name: "Powerline", Start: 0xE0A0, End: 0xE0D4},
	{Name: "Powerline Extra", Start: 0xE0B0, End: 0xE0BF},
	{Name: "Devicons", Start: 0xE700, End: 0xE7C5},
	{Name: "Seti-UI + Custom", Start: 0xE5FA, End: 0xE6AC},
	{Name: "Font Awesome", Start: 0xF000, End: 0xF2E0},
	{Name: "Octicons", Start: 0xF400, End: 0xF532},
	{Name: "Material Design", Start: 0xF0001, End: 0xF1AF0},
	{Name: "Weather Icons", Start: 0xE300, End: 0xE3E3},
	{Name: "Braille Patterns", Start: 0x2800, End: 0x28FF},
	{Name: "Geometric Shapes", Start: 0x25A0, End: 0x25FF},
}

// RangeCoverage holds the analysis result for a single Unicode range.
type RangeCoverage struct {
	Range      UnicodeRange
	Covered    int     // Number of glyphs present
	Total      int     // Total code points in the range
	Percentage float64 // 0.0–100.0
}

// CoverageReport is the top-level result of a glyph coverage analysis.
type CoverageReport struct {
	FontName      string          // Font family that was analyzed
	IsNerdFont    bool            // Whether the font is a Nerd Font variant
	Ranges        []RangeCoverage // Per-range results
	TotalCovered  int             // Sum of all covered glyphs
	TotalRequired int             // Sum of all required glyphs
	OverallPct    float64         // Weighted overall percentage
	Missing       []UnicodeRange  // Ranges with 0% coverage
	Suggestions   []string        // Actionable recommendations
}

// GlyphChecker abstracts the mechanism for probing whether a font covers a
// specific code point. Production code can supply an OS-level checker;
// tests inject a mock.
type GlyphChecker interface {
	// HasGlyph reports whether the font contains a glyph for r.
	HasGlyph(r rune) bool
}

// nerdFontChecker is a GlyphChecker that uses the Nerd Font known-coverage
// ranges. It assumes full coverage for standard Unicode blocks (box drawing,
// block elements, braille, geometric shapes) that are present in virtually
// every monospace font, and full Nerd Font private-use-area ranges when the
// font is a patched Nerd Font.
type nerdFontChecker struct {
	isNerdFont bool
}

func (c *nerdFontChecker) HasGlyph(r rune) bool {
	// Standard Unicode blocks — present in all monospace fonts.
	if (r >= 0x2500 && r <= 0x259F) || // box drawing + block elements
		(r >= 0x2800 && r <= 0x28FF) || // braille
		(r >= 0x25A0 && r <= 0x25FF) { // geometric shapes
		return true
	}
	// Nerd Font private-use-area ranges — only if the font is patched.
	if c.isNerdFont {
		return true
	}
	return false
}

// dirChecker probes font files on disk by looking for OTF/TTF files whose
// names match the family pattern. It cannot do per-glyph cmap parsing (no
// font library dependency), so it delegates to nerdFontChecker for the
// actual glyph-level decision.
type dirChecker struct {
	inner *nerdFontChecker
}

func (c *dirChecker) HasGlyph(r rune) bool {
	return c.inner.HasGlyph(r)
}

// AnalyzeCoverage checks which required Unicode ranges a font family covers.
// It accepts an optional GlyphChecker; if nil, a default heuristic checker
// is used based on the family metadata (HasNerdGlyphs).
func AnalyzeCoverage(family FontFamily, checker GlyphChecker) *CoverageReport {
	if checker == nil {
		checker = &nerdFontChecker{isNerdFont: family.HasNerdGlyphs}
	}

	report := &CoverageReport{
		FontName:   family.Name,
		IsNerdFont: family.HasNerdGlyphs,
	}

	for _, ur := range RequiredRanges {
		covered := 0
		total := ur.Size()
		for r := ur.Start; r <= ur.End; r++ {
			if checker.HasGlyph(r) {
				covered++
			}
		}
		pct := 0.0
		if total > 0 {
			pct = float64(covered) / float64(total) * 100.0
		}
		rc := RangeCoverage{
			Range:      ur,
			Covered:    covered,
			Total:      total,
			Percentage: pct,
		}
		report.Ranges = append(report.Ranges, rc)
		report.TotalCovered += covered
		report.TotalRequired += total
		if covered == 0 {
			report.Missing = append(report.Missing, ur)
		}
	}

	if report.TotalRequired > 0 {
		report.OverallPct = float64(report.TotalCovered) / float64(report.TotalRequired) * 100.0
	}

	report.Suggestions = buildSuggestions(family, report)
	return report
}

// DetectNerdFonts scans system font directories for installed Nerd Font
// families and returns them.
func DetectNerdFonts(ctx context.Context) ([]FontFamily, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	fontDirs := systemFontDirs(home)
	var found []FontFamily

	for _, fam := range AllMonaspice() {
		if fontOnDisk(fontDirs, fam) {
			found = append(found, fam)
		}
	}

	// Also look for other well-known Nerd Font families by file pattern.
	otherNF := knownNerdFontFamilies()
	for _, fam := range otherNF {
		if fontOnDisk(fontDirs, fam) {
			found = append(found, fam)
		}
	}

	return found, nil
}

// CheckRequiredRanges is a convenience function that detects Nerd Fonts,
// picks the best one, and runs coverage analysis.
func CheckRequiredRanges(ctx context.Context) (*CoverageReport, error) {
	nfFonts, err := DetectNerdFonts(ctx)
	if err != nil {
		return nil, fmt.Errorf("detecting nerd fonts: %w", err)
	}

	if len(nfFonts) == 0 {
		// No Nerd Fonts — analyze the default font with nerd=false.
		report := AnalyzeCoverage(FontFamily{
			Name:          "System Monospace",
			HasNerdGlyphs: false,
		}, nil)
		report.Suggestions = append([]string{
			"No Nerd Fonts detected. Install one: brew install --cask font-monaspice-nerd-font",
		}, report.Suggestions...)
		return report, nil
	}

	// Prefer MonaspiceNe (default), otherwise first found.
	best := nfFonts[0]
	for _, f := range nfFonts {
		if f.PostScriptBase == DefaultFont.PostScriptBase {
			best = f
			break
		}
	}

	return AnalyzeCoverage(best, nil), nil
}

// SuggestMissingFonts returns install suggestions for any Nerd Font families
// that are not yet installed.
func SuggestMissingFonts(ctx context.Context) ([]string, error) {
	installed, err := DetectNerdFonts(ctx)
	if err != nil {
		return nil, err
	}

	installedSet := make(map[string]bool, len(installed))
	for _, f := range installed {
		installedSet[f.PostScriptBase] = true
	}

	var suggestions []string

	// Check Monaspice families.
	anyMonaspice := false
	for _, f := range AllMonaspice() {
		if installedSet[f.PostScriptBase] {
			anyMonaspice = true
			break
		}
	}
	if !anyMonaspice {
		suggestions = append(suggestions,
			"Install Monaspice (Nerd Font patched Monaspace): brew install --cask font-monaspice-nerd-font")
	}

	// Check well-known alternatives.
	alternatives := knownNerdFontFamilies()
	anyAlt := false
	for _, f := range alternatives {
		if installedSet[f.PostScriptBase] {
			anyAlt = true
			break
		}
	}
	if !anyAlt && !anyMonaspice {
		suggestions = append(suggestions,
			"Or install another Nerd Font: brew install --cask font-fira-code-nerd-font",
			"Browse all: https://www.nerdfonts.com/font-downloads",
		)
	}

	if len(suggestions) == 0 {
		suggestions = append(suggestions, "Nerd Font coverage looks good. No action needed.")
	}

	return suggestions, nil
}

// systemFontDirs returns the font directories for the current platform.
func systemFontDirs(home string) []string {
	return []string{
		filepath.Join(home, "Library", "Fonts"),
		"/Library/Fonts",
		filepath.Join(home, ".local", "share", "fonts"),
		"/usr/share/fonts",
		"/usr/local/share/fonts",
	}
}

// knownNerdFontFamilies returns non-Monaspice Nerd Font families we can
// detect by file pattern.
func knownNerdFontFamilies() []FontFamily {
	return []FontFamily{
		{
			Name:           "FiraCode Nerd Font",
			BrewCask:       "font-fira-code-nerd-font",
			HasNerdGlyphs:  true,
			PostScriptBase: "FiraCodeNFM",
			FilePattern:    "FiraCodeNFM-*.ttf",
		},
		{
			Name:           "JetBrainsMono Nerd Font",
			BrewCask:       "font-jetbrains-mono-nerd-font",
			HasNerdGlyphs:  true,
			PostScriptBase: "JetBrainsMonoNFM",
			FilePattern:    "JetBrainsMonoNFM-*.ttf",
		},
		{
			Name:           "Hack Nerd Font",
			BrewCask:       "font-hack-nerd-font",
			HasNerdGlyphs:  true,
			PostScriptBase: "HackNFM",
			FilePattern:    "HackNFM-*.ttf",
		},
		{
			Name:           "CaskaydiaCove Nerd Font",
			BrewCask:       "font-caskaydia-cove-nerd-font",
			HasNerdGlyphs:  true,
			PostScriptBase: "CaskaydiaCoveNFM",
			FilePattern:    "CaskaydiaCoveNFM-*.ttf",
		},
	}
}

// buildSuggestions produces actionable recommendations based on coverage gaps.
func buildSuggestions(family FontFamily, report *CoverageReport) []string {
	var sug []string

	if !family.HasNerdGlyphs {
		sug = append(sug,
			fmt.Sprintf("%s is not a Nerd Font — Powerline and icon glyphs will be missing", family.Name),
			"Install a Nerd Font variant: brew install --cask font-monaspice-nerd-font",
		)
	}

	// Flag ranges that are partially covered.
	for _, rc := range report.Ranges {
		if rc.Percentage > 0 && rc.Percentage < 50 {
			sug = append(sug, fmt.Sprintf("Low coverage for %s (%.0f%%) — consider a font with better %s support",
				rc.Range.Name, rc.Percentage, rc.Range.Name))
		}
	}

	// Specific missing-range advice.
	for _, m := range report.Missing {
		switch m.Name {
		case "Box Drawing", "Block Elements":
			sug = append(sug, fmt.Sprintf("Missing %s glyphs — the TUI will render incorrectly without these", m.Name))
		case "Powerline", "Powerline Extra":
			sug = append(sug, fmt.Sprintf("Missing %s glyphs — status bar separators will be blank", m.Name))
		case "Devicons", "Seti-UI + Custom", "Font Awesome", "Octicons":
			sug = append(sug, fmt.Sprintf("Missing %s glyphs — file-type icons will not render", m.Name))
		}
	}

	return sug
}

// FormatReport produces a human-readable summary string for a CoverageReport.
func FormatReport(r *CoverageReport) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Glyph Coverage Report: %s\n", r.FontName)
	if r.IsNerdFont {
		b.WriteString("  Type: Nerd Font (patched)\n")
	} else {
		b.WriteString("  Type: Standard (not Nerd Font patched)\n")
	}
	fmt.Fprintf(&b, "  Overall: %d / %d (%.1f%%)\n\n", r.TotalCovered, r.TotalRequired, r.OverallPct)

	// Sort ranges by percentage ascending so gaps are visible first.
	sorted := make([]RangeCoverage, len(r.Ranges))
	copy(sorted, r.Ranges)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Percentage < sorted[j].Percentage
	})

	for _, rc := range sorted {
		bar := coverageBar(rc.Percentage, 20)
		fmt.Fprintf(&b, "  %-20s %s %5.1f%% (%d/%d)\n",
			rc.Range.Name, bar, rc.Percentage, rc.Covered, rc.Total)
	}

	if len(r.Suggestions) > 0 {
		b.WriteString("\nSuggestions:\n")
		for _, s := range r.Suggestions {
			fmt.Fprintf(&b, "  - %s\n", s)
		}
	}

	return b.String()
}

// coverageBar renders a simple ASCII progress bar.
func coverageBar(pct float64, width int) string {
	filled := int(pct / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}
