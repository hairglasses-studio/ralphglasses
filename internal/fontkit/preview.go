package fontkit

import (
	"fmt"
	"strings"
)

// PreviewOpts controls what the font preview shows.
type PreviewOpts struct {
	ShowLigatures  bool
	ShowNerdGlyphs bool
	ShowAll        bool
}

// Preview generates a terminal-printable font preview showing ligatures and glyphs.
func Preview(opts PreviewOpts) string {
	var b strings.Builder

	if opts.ShowAll || opts.ShowLigatures {
		b.WriteString("=== Monaspace Ligatures ===\n\n")
		b.WriteString("  Arrows:     -> => <- =< >>= <<= |> <| ->> <<-\n")
		b.WriteString("  Comparison: == != === !== >= <= <> <=> =~\n")
		b.WriteString("  Logic:      && || !! :: ;;\n")
		b.WriteString("  Assignment: := += -= *= /= |= &= ^= >>= <<=\n")
		b.WriteString("  Comments:   // /* */ /// /** -- ---\n")
		b.WriteString("  Brackets:   </ /> </> {| |}\n")
		b.WriteString("  Other:      .. ... :: ## ### ||| <~> ~~> <~~ ~>\n")
		b.WriteString("\n")
	}

	if opts.ShowAll || opts.ShowNerdGlyphs {
		b.WriteString("=== Nerd Font Glyphs ===\n\n")
		// Common development icons
		glyphs := []struct {
			icon string
			name string
		}{
			{"\uf121", "code"},
			{"\uf09b", "github"},
			{"\ue725", "git-branch"},
			{"\uf07b", "folder"},
			{"\uf15b", "file"},
			{"\uf10b", "terminal"},
			{"\uf155", "dollar"},
			{"\uf017", "clock"},
			{"\uf0e7", "bolt"},
			{"\uf00c", "check"},
			{"\uf00d", "x-mark"},
			{"\uf071", "warning"},
			{"\uf05a", "info"},
			{"\uf188", "bug"},
			{"\uf1e0", "share"},
			{"\uf013", "gear"},
			{"\uf023", "lock"},
			{"\uf0c1", "link"},
			{"\uf0e8", "sitemap"},
			{"\uf1c0", "database"},
		}

		for _, g := range glyphs {
			b.WriteString(fmt.Sprintf("  %s  %s\n", g.icon, g.name))
		}
		b.WriteString("\n")

		// Powerline symbols
		b.WriteString("  Powerline:  \ue0b0 \ue0b1 \ue0b2 \ue0b3 \ue0b4 \ue0b5 \ue0b6 \ue0b7\n")
		b.WriteString("\n")
	}

	// Font fallback tiers
	b.WriteString("=== Font Fallback Tiers ===\n\n")
	b.WriteString("  Tier 1 (Nerd Font):  \uf10b Opus │ \uf07b ~/project │ ▓▓▓░░░ │ \uf155 0.42\n")
	b.WriteString("  Tier 2 (Unicode):    ⬡ Opus │ ● ~/project │ ▓▓▓░░░ │ $ 0.42\n")
	b.WriteString("  Tier 3 (ASCII):      [M] Opus | > ~/project | ###--- | $ 0.42\n")

	return b.String()
}
