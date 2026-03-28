package fontkit

import (
	"strings"
	"testing"
)

func TestPreviewAll(t *testing.T) {
	output := Preview(PreviewOpts{ShowAll: true})
	if !strings.Contains(output, "Ligatures") {
		t.Error("ShowAll should include ligatures section")
	}
	if !strings.Contains(output, "Nerd Font Glyphs") {
		t.Error("ShowAll should include glyphs section")
	}
	if !strings.Contains(output, "Fallback Tiers") {
		t.Error("ShowAll should include fallback tiers")
	}
}

func TestPreviewLigaturesOnly(t *testing.T) {
	output := Preview(PreviewOpts{ShowLigatures: true})
	if !strings.Contains(output, "Ligatures") {
		t.Error("should include ligatures")
	}
	if strings.Contains(output, "Nerd Font Glyphs") {
		t.Error("should not include glyphs when only ligatures requested")
	}
}

func TestPreviewGlyphsOnly(t *testing.T) {
	output := Preview(PreviewOpts{ShowNerdGlyphs: true})
	if strings.Contains(output, "Ligatures") {
		t.Error("should not include ligatures when only glyphs requested")
	}
	if !strings.Contains(output, "Nerd Font Glyphs") {
		t.Error("should include glyphs")
	}
}
