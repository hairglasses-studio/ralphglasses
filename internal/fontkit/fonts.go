// Package fontkit provides font detection, installation, and terminal configuration
// for Monaspace and Monaspice font families.
package fontkit

// Style represents a font weight/style variant.
type Style string

const (
	StyleRegular    Style = "Regular"
	StyleBold       Style = "Bold"
	StyleItalic     Style = "Italic"
	StyleBoldItalic Style = "BoldItalic"
)

// FontFamily describes a font family with its metadata and installation info.
type FontFamily struct {
	Name           string   // Human-readable name (e.g. "Monaspace Neon")
	BrewCask       string   // Homebrew cask name (empty if not available)
	Subfamily      string   // Variant identifier (e.g. "Neon", "Ne")
	HasNerdGlyphs  bool     // Whether this family includes Nerd Font icons
	PostScriptBase string   // Base PostScript name for font matching
	FilePattern    string   // Glob pattern for font files on disk
	Styles         []Style  // Available styles
	Aliases        []string // Alternative names
}

var allStyles = []Style{StyleRegular, StyleBold, StyleItalic, StyleBoldItalic}

// Monaspace subfamilies — the upstream GitHub font.
var (
	MonaspaceArgon = FontFamily{
		Name:           "Monaspace Argon",
		BrewCask:       "font-monaspace",
		Subfamily:      "Argon",
		PostScriptBase: "MonaspaceArgon",
		FilePattern:    "MonaspaceArgon-*.otf",
		Styles:         allStyles,
	}
	MonaspaceNeon = FontFamily{
		Name:           "Monaspace Neon",
		BrewCask:       "font-monaspace",
		Subfamily:      "Neon",
		PostScriptBase: "MonaspaceNeon",
		FilePattern:    "MonaspaceNeon-*.otf",
		Styles:         allStyles,
	}
	MonaspaceXenon = FontFamily{
		Name:           "Monaspace Xenon",
		BrewCask:       "font-monaspace",
		Subfamily:      "Xenon",
		PostScriptBase: "MonaspaceXenon",
		FilePattern:    "MonaspaceXenon-*.otf",
		Styles:         allStyles,
	}
	MonaspaceRadon = FontFamily{
		Name:           "Monaspace Radon",
		BrewCask:       "font-monaspace",
		Subfamily:      "Radon",
		PostScriptBase: "MonaspaceRadon",
		FilePattern:    "MonaspaceRadon-*.otf",
		Styles:         allStyles,
	}
	MonaspaceKrypton = FontFamily{
		Name:           "Monaspace Krypton",
		BrewCask:       "font-monaspace",
		Subfamily:      "Krypton",
		PostScriptBase: "MonaspaceKrypton",
		FilePattern:    "MonaspaceKrypton-*.otf",
		Styles:         allStyles,
	}
)

// Monaspice subfamilies — Nerd Font patched variants.
var (
	MonaspiceAr = FontFamily{
		Name:           "Monaspice Ar Nerd Font",
		BrewCask:       "font-monaspice-nerd-font",
		Subfamily:      "Ar",
		HasNerdGlyphs:  true,
		PostScriptBase: "MonaspiceArNFM",
		FilePattern:    "MonaspiceArNFM-*.otf",
		Styles:         allStyles,
		Aliases:        []string{"MonaspiceArNFM"},
	}
	MonaspiceNe = FontFamily{
		Name:           "Monaspice Ne Nerd Font",
		BrewCask:       "font-monaspice-nerd-font",
		Subfamily:      "Ne",
		HasNerdGlyphs:  true,
		PostScriptBase: "MonaspiceNeNFM",
		FilePattern:    "MonaspiceNeNFM-*.otf",
		Styles:         allStyles,
		Aliases:        []string{"MonaspiceNeNFM"},
	}
	MonaspiceXe = FontFamily{
		Name:           "Monaspice Xe Nerd Font",
		BrewCask:       "font-monaspice-nerd-font",
		Subfamily:      "Xe",
		HasNerdGlyphs:  true,
		PostScriptBase: "MonaspiceXeNFM",
		FilePattern:    "MonaspiceXeNFM-*.otf",
		Styles:         allStyles,
		Aliases:        []string{"MonaspiceXeNFM"},
	}
	MonaspiceRn = FontFamily{
		Name:           "Monaspice Rn Nerd Font",
		BrewCask:       "font-monaspice-nerd-font",
		Subfamily:      "Rn",
		HasNerdGlyphs:  true,
		PostScriptBase: "MonaspiceRnNFM",
		FilePattern:    "MonaspiceRnNFM-*.otf",
		Styles:         allStyles,
		Aliases:        []string{"MonaspiceRnNFM"},
	}
	MonaspiceKr = FontFamily{
		Name:           "Monaspice Kr Nerd Font",
		BrewCask:       "font-monaspice-nerd-font",
		Subfamily:      "Kr",
		HasNerdGlyphs:  true,
		PostScriptBase: "MonaspiceKrNFM",
		FilePattern:    "MonaspiceKrNFM-*.otf",
		Styles:         allStyles,
		Aliases:        []string{"MonaspiceKrNFM"},
	}
)

// AllMonaspace returns all upstream Monaspace font families.
func AllMonaspace() []FontFamily {
	return []FontFamily{MonaspaceArgon, MonaspaceNeon, MonaspaceXenon, MonaspaceRadon, MonaspaceKrypton}
}

// AllMonaspice returns all Nerd Font patched Monaspice families.
func AllMonaspice() []FontFamily {
	return []FontFamily{MonaspiceAr, MonaspiceNe, MonaspiceXe, MonaspiceRn, MonaspiceKr}
}

// AllFamilies returns every known font family.
func AllFamilies() []FontFamily {
	return append(AllMonaspace(), AllMonaspice()...)
}

// DefaultFont is the recommended font for Claude Code terminals.
var DefaultFont = MonaspiceNe

// FallbackChain returns the ordered font preference for terminal configuration.
// First available font wins.
func FallbackChain() []string {
	return []string{
		"MonaspiceNeNFM-Regular", // Monaspice Neon (Nerd Font)
		"MonaspaceNeon-Regular",  // Upstream Monaspace Neon
		"Menlo",                  // macOS system monospace
	}
}
