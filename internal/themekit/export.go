package themekit

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExportFormat identifies a theme serialization format.
type ExportFormat string

const (
	FormatYAML ExportFormat = "yaml"
	FormatJSON ExportFormat = "json"
	FormatTOML ExportFormat = "toml"
	FormatCSS  ExportFormat = "css"
)

// themeDTO is the serializable representation of a Palette, used for
// YAML/JSON/TOML round-trips. Colors are stored as an ordered slice
// so the output is deterministic.
type themeDTO struct {
	Name   string     `json:"name" yaml:"name"`
	Colors []colorDTO `json:"colors" yaml:"colors"`
}

type colorDTO struct {
	Name string `json:"name" yaml:"name"`
	Hex  string `json:"hex" yaml:"hex"`
	R    uint8  `json:"r" yaml:"r"`
	G    uint8  `json:"g" yaml:"g"`
	B    uint8  `json:"b" yaml:"b"`
}

// ExportTheme serialises a Palette to the requested format and returns the
// formatted bytes. Supported formats: yaml, json, toml, css.
func ExportTheme(p Palette, format ExportFormat) ([]byte, error) {
	switch format {
	case FormatYAML:
		return exportYAML(p)
	case FormatJSON:
		return exportJSON(p)
	case FormatTOML:
		return exportTOML(p)
	case FormatCSS:
		return exportCSS(p)
	default:
		return nil, fmt.Errorf("unsupported export format: %q", format)
	}
}

// sortedColorDTOs converts a Palette's color map to a deterministically
// ordered slice of colorDTO.
func sortedColorDTOs(p Palette) []colorDTO {
	names := make([]string, 0, len(p.Colors))
	for name := range p.Colors {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]colorDTO, len(names))
	for i, name := range names {
		col := p.Colors[name]
		out[i] = colorDTO{Name: col.Name, Hex: col.Hex, R: col.R, G: col.G, B: col.B}
	}
	return out
}

// ---------------------------------------------------------------------------
// YAML
// ---------------------------------------------------------------------------

func exportYAML(p Palette) ([]byte, error) {
	dto := themeDTO{Name: p.Name, Colors: sortedColorDTOs(p)}
	return yaml.Marshal(dto)
}

// ParseYAML deserialises YAML bytes back into a Palette.
func ParseYAML(data []byte) (Palette, error) {
	var dto themeDTO
	if err := yaml.Unmarshal(data, &dto); err != nil {
		return Palette{}, fmt.Errorf("parse yaml theme: %w", err)
	}
	return dtoToPalette(dto), nil
}

// ---------------------------------------------------------------------------
// JSON
// ---------------------------------------------------------------------------

func exportJSON(p Palette) ([]byte, error) {
	dto := themeDTO{Name: p.Name, Colors: sortedColorDTOs(p)}
	return json.MarshalIndent(dto, "", "  ")
}

// ParseJSON deserialises JSON bytes back into a Palette.
func ParseJSON(data []byte) (Palette, error) {
	var dto themeDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return Palette{}, fmt.Errorf("parse json theme: %w", err)
	}
	return dtoToPalette(dto), nil
}

// ---------------------------------------------------------------------------
// TOML
// ---------------------------------------------------------------------------

func exportTOML(p Palette) ([]byte, error) {
	colors := sortedColorDTOs(p)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("name = %q\n\n", p.Name))

	for _, col := range colors {
		b.WriteString(fmt.Sprintf("[[colors]]\n"))
		b.WriteString(fmt.Sprintf("name = %q\n", col.Name))
		b.WriteString(fmt.Sprintf("hex = %q\n", col.Hex))
		b.WriteString(fmt.Sprintf("r = %d\n", col.R))
		b.WriteString(fmt.Sprintf("g = %d\n", col.G))
		b.WriteString(fmt.Sprintf("b = %d\n", col.B))
		b.WriteString("\n")
	}

	return []byte(b.String()), nil
}

// ParseTOML deserialises the TOML bytes produced by exportTOML back into a
// Palette. It handles the subset of TOML that ExportTheme generates: bare
// key = value pairs and [[colors]] array-of-tables.
func ParseTOML(data []byte) (Palette, error) {
	dto := themeDTO{}
	var cur *colorDTO

	flush := func() {
		if cur != nil {
			dto.Colors = append(dto.Colors, *cur)
			cur = nil
		}
	}

	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if line == "[[colors]]" {
			flush()
			cur = &colorDTO{}
			continue
		}

		eqIdx := strings.Index(line, " = ")
		if eqIdx < 0 {
			continue
		}
		key := line[:eqIdx]
		val := line[eqIdx+3:]

		// Unquote strings
		val = strings.TrimPrefix(val, "\"")
		val = strings.TrimSuffix(val, "\"")

		if cur != nil {
			switch key {
			case "name":
				cur.Name = val
			case "hex":
				cur.Hex = val
			case "r":
				cur.R = parseTOMLUint8(val)
			case "g":
				cur.G = parseTOMLUint8(val)
			case "b":
				cur.B = parseTOMLUint8(val)
			}
		} else {
			switch key {
			case "name":
				dto.Name = val
			}
		}
	}
	flush()

	return dtoToPalette(dto), nil
}

func parseTOMLUint8(s string) uint8 {
	var n uint8
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			n = n*10 + uint8(ch-'0')
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// CSS
// ---------------------------------------------------------------------------

func exportCSS(p Palette) ([]byte, error) {
	colors := sortedColorDTOs(p)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("/* %s — generated by ralphglasses themekit */\n", p.Name))
	b.WriteString(":root {\n")

	for _, col := range colors {
		b.WriteString(fmt.Sprintf("  --theme-%s: #%s;\n", col.Name, col.Hex))
		b.WriteString(fmt.Sprintf("  --theme-%s-rgb: %d, %d, %d;\n", col.Name, col.R, col.G, col.B))
	}

	b.WriteString("}\n")
	return []byte(b.String()), nil
}

// ParseCSS extracts theme variables from CSS `:root` output produced by
// exportCSS and reconstructs a Palette. It recognises `--theme-<name>: #hex`
// and `--theme-<name>-rgb: r, g, b` custom properties.
func ParseCSS(data []byte) (Palette, error) {
	p := Palette{Colors: make(map[string]Color)}

	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)

		// Extract palette name from the comment header.
		if strings.HasPrefix(line, "/*") && strings.Contains(line, "generated by") {
			name := strings.TrimPrefix(line, "/* ")
			if idx := strings.Index(name, " —"); idx > 0 {
				p.Name = name[:idx]
			} else if idx := strings.Index(name, " -"); idx > 0 {
				p.Name = name[:idx]
			}
			continue
		}

		if !strings.HasPrefix(line, "--theme-") {
			continue
		}

		// Strip trailing semicolon
		line = strings.TrimSuffix(line, ";")

		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		prop := line[:colonIdx]
		val := strings.TrimSpace(line[colonIdx+1:])

		// --theme-<name>-rgb: r, g, b
		if strings.HasSuffix(prop, "-rgb") {
			colorName := strings.TrimPrefix(prop, "--theme-")
			colorName = strings.TrimSuffix(colorName, "-rgb")
			parts := strings.Split(val, ",")
			if len(parts) != 3 {
				continue
			}
			col := p.Colors[colorName]
			col.Name = colorName
			col.R = parseTOMLUint8(strings.TrimSpace(parts[0]))
			col.G = parseTOMLUint8(strings.TrimSpace(parts[1]))
			col.B = parseTOMLUint8(strings.TrimSpace(parts[2]))
			p.Colors[colorName] = col
			continue
		}

		// --theme-<name>: #hex
		colorName := strings.TrimPrefix(prop, "--theme-")
		hex := strings.TrimPrefix(val, "#")
		col := p.Colors[colorName]
		col.Name = colorName
		col.Hex = hex
		p.Colors[colorName] = col
	}

	return p, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func dtoToPalette(dto themeDTO) Palette {
	p := Palette{
		Name:   dto.Name,
		Colors: make(map[string]Color, len(dto.Colors)),
	}
	for _, col := range dto.Colors {
		p.Colors[col.Name] = Color{
			Name: col.Name,
			Hex:  col.Hex,
			R:    col.R,
			G:    col.G,
			B:    col.B,
		}
	}
	return p
}
