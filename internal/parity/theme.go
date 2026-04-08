package parity

import (
	"fmt"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

type ThemeExport struct {
	Format  string `json:"format"`
	Theme   string `json:"theme"`
	Content string `json:"content"`
}

func ExportTheme(format, name string) (*ThemeExport, error) {
	t := styles.ResolveTheme(name)
	if t == nil {
		return nil, fmt.Errorf("theme %q not found", name)
	}

	var b strings.Builder
	switch format {
	case "ghostty":
		fmt.Fprintf(&b, "# Ghostty palette from ralphglasses theme %q\n", t.Name)
		fmt.Fprintf(&b, "palette = 0=%s\n", t.DarkBg)
		fmt.Fprintf(&b, "palette = 1=%s\n", t.Red)
		fmt.Fprintf(&b, "palette = 2=%s\n", t.Green)
		fmt.Fprintf(&b, "palette = 3=%s\n", t.Yellow)
		fmt.Fprintf(&b, "palette = 4=%s\n", t.Primary)
		fmt.Fprintf(&b, "palette = 5=%s\n", t.Accent)
		fmt.Fprintf(&b, "palette = 6=%s\n", t.Primary)
		fmt.Fprintf(&b, "palette = 7=%s\n", t.BrightFg)
		fmt.Fprintf(&b, "background = %s\n", t.DarkBg)
		fmt.Fprintf(&b, "foreground = %s\n", t.BrightFg)
	case "starship":
		fmt.Fprintf(&b, "# Starship color overrides from ralphglasses theme %q\n", t.Name)
		fmt.Fprintf(&b, "[palette.%s]\n", t.Name)
		fmt.Fprintf(&b, "primary = \"%s\"\n", t.Primary)
		fmt.Fprintf(&b, "accent = \"%s\"\n", t.Accent)
		fmt.Fprintf(&b, "green = \"%s\"\n", t.Green)
		fmt.Fprintf(&b, "yellow = \"%s\"\n", t.Yellow)
		fmt.Fprintf(&b, "red = \"%s\"\n", t.Red)
	case "k9s":
		fmt.Fprintf(&b, "# k9s skin.yml from ralphglasses theme %q\n", t.Name)
		fmt.Fprintf(&b, "k9s:\n")
		fmt.Fprintf(&b, "  body:\n")
		fmt.Fprintf(&b, "    fgColor: \"%s\"\n", t.BrightFg)
		fmt.Fprintf(&b, "    bgColor: \"%s\"\n", t.DarkBg)
		fmt.Fprintf(&b, "    logoColor: \"%s\"\n", t.Primary)
		fmt.Fprintf(&b, "  info:\n")
		fmt.Fprintf(&b, "    fgColor: \"%s\"\n", t.Accent)
		fmt.Fprintf(&b, "    sectionColor: \"%s\"\n", t.Primary)
		fmt.Fprintf(&b, "  frame:\n")
		fmt.Fprintf(&b, "    border:\n")
		fmt.Fprintf(&b, "      fgColor: \"%s\"\n", t.Primary)
		fmt.Fprintf(&b, "      focusColor: \"%s\"\n", t.Accent)
	default:
		return nil, fmt.Errorf("unsupported format %q (try: ghostty, starship, k9s)", format)
	}

	return &ThemeExport{
		Format:  format,
		Theme:   t.Name,
		Content: b.String(),
	}, nil
}
