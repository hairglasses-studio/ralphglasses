package themekit

import "testing"

func TestCatppuccinMocha(t *testing.T) {
	p := Catppuccin(Mocha)
	if p.Name != "Catppuccin Mocha" {
		t.Errorf("Name = %q", p.Name)
	}
	if len(p.Colors) != 26 {
		t.Errorf("expected 26 colors, got %d", len(p.Colors))
	}

	base := p.Get("base")
	if base.Hex != "1e1e2e" {
		t.Errorf("base hex = %q, want 1e1e2e", base.Hex)
	}
	if base.R != 30 || base.G != 30 || base.B != 46 {
		t.Errorf("base RGB = (%d,%d,%d), want (30,30,46)", base.R, base.G, base.B)
	}
}

func TestAllFlavors(t *testing.T) {
	flavors := AllFlavors()
	if len(flavors) != 4 {
		t.Fatalf("expected 4 flavors, got %d", len(flavors))
	}
	for _, f := range flavors {
		p := Catppuccin(f)
		if len(p.Colors) != 26 {
			t.Errorf("%s: expected 26 colors, got %d", f, len(p.Colors))
		}
	}
}

func TestColorANSI(t *testing.T) {
	col := Color{Name: "test", Hex: "ff0000", R: 255, G: 0, B: 0}
	ansi := col.ANSI()
	want := "\033[38;2;255;0;0m"
	if ansi != want {
		t.Errorf("ANSI() = %q, want %q", ansi, want)
	}
}

func TestColorANSIBg(t *testing.T) {
	col := Color{Name: "test", Hex: "00ff00", R: 0, G: 255, B: 0}
	ansi := col.ANSIBg()
	want := "\033[48;2;0;255;0m"
	if ansi != want {
		t.Errorf("ANSIBg() = %q, want %q", ansi, want)
	}
}

func TestGetMissing(t *testing.T) {
	p := Catppuccin(Mocha)
	col := p.Get("nonexistent")
	if col.Name != "" {
		t.Errorf("Get(nonexistent) should return zero Color, got %+v", col)
	}
}
