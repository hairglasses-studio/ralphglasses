// Package themekit provides terminal color theme management with Catppuccin support.
package themekit

// Color represents an RGB color with an ANSI escape sequence.
type Color struct {
	Name string // Semantic name (e.g. "base", "text", "red")
	Hex  string // Hex value without # (e.g. "1e1e2e")
	R, G, B uint8
}

// ANSI returns the 24-bit ANSI foreground escape for this color.
func (c Color) ANSI() string {
	return "\033[38;2;" + itoa(c.R) + ";" + itoa(c.G) + ";" + itoa(c.B) + "m"
}

// ANSIBg returns the 24-bit ANSI background escape for this color.
func (c Color) ANSIBg() string {
	return "\033[48;2;" + itoa(c.R) + ";" + itoa(c.G) + ";" + itoa(c.B) + "m"
}

func itoa(n uint8) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	if n < 100 {
		return string(rune('0'+n/10)) + string(rune('0'+n%10))
	}
	return string(rune('0'+n/100)) + string(rune('0'+(n/10)%10)) + string(rune('0'+n%10))
}

// Palette is a named collection of colors forming a complete theme.
type Palette struct {
	Name   string
	Colors map[string]Color
}

// Get returns a color by semantic name, or a zero Color if not found.
func (p Palette) Get(name string) Color {
	return p.Colors[name]
}

// Flavor is a Catppuccin flavor identifier.
type Flavor string

const (
	Latte     Flavor = "latte"
	Frappe    Flavor = "frappe"
	Macchiato Flavor = "macchiato"
	Mocha     Flavor = "mocha"
)

func c(name, hex string, r, g, b uint8) Color {
	return Color{Name: name, Hex: hex, R: r, G: g, B: b}
}

// Catppuccin returns the palette for the given Catppuccin flavor.
func Catppuccin(flavor Flavor) Palette {
	switch flavor {
	case Latte:
		return catppuccinLatte
	case Frappe:
		return catppuccinFrappe
	case Macchiato:
		return catppuccinMacchiato
	default:
		return catppuccinMocha
	}
}

// AllFlavors returns all Catppuccin flavor identifiers.
func AllFlavors() []Flavor {
	return []Flavor{Latte, Frappe, Macchiato, Mocha}
}

var catppuccinMocha = Palette{
	Name: "Catppuccin Mocha",
	Colors: map[string]Color{
		"rosewater": c("rosewater", "f5e0dc", 245, 224, 220),
		"flamingo":  c("flamingo", "f2cdcd", 242, 205, 205),
		"pink":      c("pink", "f5c2e7", 245, 194, 231),
		"mauve":     c("mauve", "cba6f7", 203, 166, 247),
		"red":       c("red", "f38ba8", 243, 139, 168),
		"maroon":    c("maroon", "eba0ac", 235, 160, 172),
		"peach":     c("peach", "fab387", 250, 179, 135),
		"yellow":    c("yellow", "f9e2af", 249, 226, 175),
		"green":     c("green", "a6e3a1", 166, 227, 161),
		"teal":      c("teal", "94e2d5", 148, 226, 213),
		"sky":       c("sky", "89dceb", 137, 220, 235),
		"sapphire":  c("sapphire", "74c7ec", 116, 199, 236),
		"blue":      c("blue", "89b4fa", 137, 180, 250),
		"lavender":  c("lavender", "b4befe", 180, 190, 254),
		"text":      c("text", "cdd6f4", 205, 214, 244),
		"subtext1":  c("subtext1", "bac2de", 186, 194, 222),
		"subtext0":  c("subtext0", "a6adc8", 166, 173, 200),
		"overlay2":  c("overlay2", "9399b2", 147, 153, 178),
		"overlay1":  c("overlay1", "7f849c", 127, 132, 156),
		"overlay0":  c("overlay0", "6c7086", 108, 112, 134),
		"surface2":  c("surface2", "585b70", 88, 91, 112),
		"surface1":  c("surface1", "45475a", 69, 71, 90),
		"surface0":  c("surface0", "313244", 49, 50, 68),
		"base":      c("base", "1e1e2e", 30, 30, 46),
		"mantle":    c("mantle", "181825", 24, 24, 37),
		"crust":     c("crust", "11111b", 17, 17, 27),
	},
}

var catppuccinLatte = Palette{
	Name: "Catppuccin Latte",
	Colors: map[string]Color{
		"rosewater": c("rosewater", "dc8a78", 220, 138, 120),
		"flamingo":  c("flamingo", "dd7878", 221, 120, 120),
		"pink":      c("pink", "ea76cb", 234, 118, 203),
		"mauve":     c("mauve", "8839ef", 136, 57, 239),
		"red":       c("red", "d20f39", 210, 15, 57),
		"maroon":    c("maroon", "e64553", 230, 69, 83),
		"peach":     c("peach", "fe640b", 254, 100, 11),
		"yellow":    c("yellow", "df8e1d", 223, 142, 29),
		"green":     c("green", "40a02b", 64, 160, 43),
		"teal":      c("teal", "179299", 23, 146, 153),
		"sky":       c("sky", "04a5e5", 4, 165, 229),
		"sapphire":  c("sapphire", "209fb5", 32, 159, 181),
		"blue":      c("blue", "1e66f5", 30, 102, 245),
		"lavender":  c("lavender", "7287fd", 114, 135, 253),
		"text":      c("text", "4c4f69", 76, 79, 105),
		"subtext1":  c("subtext1", "5c5f77", 92, 95, 119),
		"subtext0":  c("subtext0", "6c6f85", 108, 111, 133),
		"overlay2":  c("overlay2", "7c7f93", 124, 127, 147),
		"overlay1":  c("overlay1", "8c8fa1", 140, 143, 161),
		"overlay0":  c("overlay0", "9ca0b0", 156, 160, 176),
		"surface2":  c("surface2", "acb0be", 172, 176, 190),
		"surface1":  c("surface1", "bcc0cc", 188, 192, 204),
		"surface0":  c("surface0", "ccd0da", 204, 208, 218),
		"base":      c("base", "eff1f5", 239, 241, 245),
		"mantle":    c("mantle", "e6e9ef", 230, 233, 239),
		"crust":     c("crust", "dce0e8", 220, 224, 232),
	},
}

var catppuccinFrappe = Palette{
	Name: "Catppuccin Frappé",
	Colors: map[string]Color{
		"rosewater": c("rosewater", "f2d5cf", 242, 213, 207),
		"flamingo":  c("flamingo", "eebebe", 238, 190, 190),
		"pink":      c("pink", "f4b8e4", 244, 184, 228),
		"mauve":     c("mauve", "ca9ee6", 202, 158, 230),
		"red":       c("red", "e78284", 231, 130, 132),
		"maroon":    c("maroon", "ea999c", 234, 153, 156),
		"peach":     c("peach", "ef9f76", 239, 159, 118),
		"yellow":    c("yellow", "e5c890", 229, 200, 144),
		"green":     c("green", "a6d189", 166, 209, 137),
		"teal":      c("teal", "81c8be", 129, 200, 190),
		"sky":       c("sky", "99d1db", 153, 209, 219),
		"sapphire":  c("sapphire", "85c1dc", 133, 193, 220),
		"blue":      c("blue", "8caaee", 140, 170, 238),
		"lavender":  c("lavender", "babbf1", 186, 187, 241),
		"text":      c("text", "c6d0f5", 198, 208, 245),
		"subtext1":  c("subtext1", "b5bfe2", 181, 191, 226),
		"subtext0":  c("subtext0", "a5adce", 165, 173, 206),
		"overlay2":  c("overlay2", "949cbb", 148, 156, 187),
		"overlay1":  c("overlay1", "838ba7", 131, 139, 167),
		"overlay0":  c("overlay0", "737994", 115, 121, 148),
		"surface2":  c("surface2", "626880", 98, 104, 128),
		"surface1":  c("surface1", "51576d", 81, 87, 109),
		"surface0":  c("surface0", "414559", 65, 69, 89),
		"base":      c("base", "303446", 48, 52, 70),
		"mantle":    c("mantle", "292c3c", 41, 44, 60),
		"crust":     c("crust", "232634", 35, 38, 52),
	},
}

var catppuccinMacchiato = Palette{
	Name: "Catppuccin Macchiato",
	Colors: map[string]Color{
		"rosewater": c("rosewater", "f4dbd6", 244, 219, 214),
		"flamingo":  c("flamingo", "f0c6c6", 240, 198, 198),
		"pink":      c("pink", "f5bde6", 245, 189, 230),
		"mauve":     c("mauve", "c6a0f6", 198, 160, 246),
		"red":       c("red", "ed8796", 237, 135, 150),
		"maroon":    c("maroon", "ee99a0", 238, 153, 160),
		"peach":     c("peach", "f5a97f", 245, 169, 127),
		"yellow":    c("yellow", "eed49f", 238, 212, 159),
		"green":     c("green", "a6da95", 166, 218, 149),
		"teal":      c("teal", "8bd5ca", 139, 213, 202),
		"sky":       c("sky", "91d7e3", 145, 215, 227),
		"sapphire":  c("sapphire", "7dc4e4", 125, 196, 228),
		"blue":      c("blue", "8aadf4", 138, 173, 244),
		"lavender":  c("lavender", "b7bdf8", 183, 189, 248),
		"text":      c("text", "cad3f5", 202, 211, 245),
		"subtext1":  c("subtext1", "b8c0e0", 184, 192, 224),
		"subtext0":  c("subtext0", "a5adcb", 165, 173, 203),
		"overlay2":  c("overlay2", "939ab7", 147, 154, 183),
		"overlay1":  c("overlay1", "8087a2", 128, 135, 162),
		"overlay0":  c("overlay0", "6e738d", 110, 115, 141),
		"surface2":  c("surface2", "5b6078", 91, 96, 120),
		"surface1":  c("surface1", "494d64", 73, 77, 100),
		"surface0":  c("surface0", "363a4f", 54, 58, 79),
		"base":      c("base", "24273a", 36, 39, 58),
		"mantle":    c("mantle", "1e2030", 30, 32, 48),
		"crust":     c("crust", "181926", 24, 25, 38),
	},
}
