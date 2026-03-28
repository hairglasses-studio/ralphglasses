package fontkit

import "testing"

func TestAllFamilies(t *testing.T) {
	families := AllFamilies()
	if got := len(families); got != 10 {
		t.Errorf("AllFamilies() = %d families, want 10", got)
	}
}

func TestAllMonaspace(t *testing.T) {
	for _, f := range AllMonaspace() {
		if f.HasNerdGlyphs {
			t.Errorf("Monaspace %s should not have Nerd glyphs", f.Name)
		}
		if f.BrewCask != "font-monaspace" {
			t.Errorf("Monaspace %s cask = %q, want font-monaspace", f.Name, f.BrewCask)
		}
	}
}

func TestAllMonaspice(t *testing.T) {
	for _, f := range AllMonaspice() {
		if !f.HasNerdGlyphs {
			t.Errorf("Monaspice %s should have Nerd glyphs", f.Name)
		}
		if f.BrewCask != "font-monaspice-nerd-font" {
			t.Errorf("Monaspice %s cask = %q, want font-monaspice-nerd-font", f.Name, f.BrewCask)
		}
	}
}

func TestFallbackChain(t *testing.T) {
	chain := FallbackChain()
	if len(chain) < 3 {
		t.Fatal("FallbackChain should have at least 3 entries")
	}
	if chain[len(chain)-1] != "Menlo" {
		t.Errorf("last fallback = %q, want Menlo", chain[len(chain)-1])
	}
}

func TestDefaultFont(t *testing.T) {
	if DefaultFont.Name != "Monaspice Ne Nerd Font" {
		t.Errorf("DefaultFont = %q, want Monaspice Ne Nerd Font", DefaultFont.Name)
	}
}
