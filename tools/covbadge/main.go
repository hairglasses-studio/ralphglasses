// Command covbadge reads a Go coverage profile and generates an SVG badge.
//
// Usage:
//
//	go run ./tools/covbadge -i coverage.out -o .ralph/coverage-badge.svg
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	var inPath, outPath string
	flag.StringVar(&inPath, "i", "coverage.out", "path to coverage profile")
	flag.StringVar(&outPath, "o", ".ralph/coverage-badge.svg", "output SVG path")
	flag.Parse()

	pct, err := parseCoverageProfile(inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "covbadge: %v\n", err)
		os.Exit(1)
	}

	svg := renderBadge(pct)
	if err := os.WriteFile(outPath, []byte(svg), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "covbadge: write %s: %v\n", outPath, err)
		os.Exit(1)
	}
	fmt.Printf("covbadge: %.1f%% → %s\n", pct, outPath)
}

// parseCoverageProfile computes total coverage from a Go coverprofile.
// It sums statement counts and covered counts across all blocks.
func parseCoverageProfile(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var totalStmts, coveredStmts int64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip the mode line.
		if strings.HasPrefix(line, "mode:") {
			continue
		}
		// Format: name:startLine.startCol,endLine.endCol numStatements count
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		numStmts, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		count, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			continue
		}
		totalStmts += numStmts
		if count > 0 {
			coveredStmts += numStmts
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if totalStmts == 0 {
		return 0, nil
	}
	return float64(coveredStmts) / float64(totalStmts) * 100, nil
}

// badgeColor returns a hex color based on coverage thresholds.
func badgeColor(pct float64) string {
	switch {
	case pct >= 80:
		return "#4c1"  // green
	case pct >= 60:
		return "#dfb317" // yellow
	default:
		return "#e05d44" // red
	}
}

// renderBadge generates an SVG coverage badge similar to shields.io style.
func renderBadge(pct float64) string {
	color := badgeColor(pct)
	label := "coverage"
	value := fmt.Sprintf("%.1f%%", pct)

	// Calculate widths (approximate: 6.5px per char + padding).
	labelWidth := len(label)*7 + 10
	valueWidth := len(value)*7 + 10
	totalWidth := labelWidth + valueWidth

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">
  <title>%s: %s</title>
  <linearGradient id="s" x2="0" y2="100%%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="r">
    <rect width="%d" height="20" rx="3" fill="#fff"/>
  </clipPath>
  <g clip-path="url(#r)">
    <rect width="%d" height="20" fill="#555"/>
    <rect x="%d" width="%d" height="20" fill="%s"/>
    <rect width="%d" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" text-rendering="geometricPrecision" font-size="110">
    <text aria-hidden="true" x="%d" y="150" fill="#010101" fill-opacity=".3" transform="scale(.1)">%s</text>
    <text x="%d" y="140" transform="scale(.1)">%s</text>
    <text aria-hidden="true" x="%d" y="150" fill="#010101" fill-opacity=".3" transform="scale(.1)">%s</text>
    <text x="%d" y="140" transform="scale(.1)">%s</text>
  </g>
</svg>`,
		totalWidth, label, value,
		label, value,
		totalWidth,
		labelWidth,
		labelWidth, valueWidth, color,
		totalWidth,
		labelWidth*10/2, label,
		labelWidth*10/2, label,
		(labelWidth+totalWidth)*10/2, value,
		(labelWidth+totalWidth)*10/2, value,
	)
}
