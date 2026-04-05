package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

// Layout identifies a preset pane arrangement for monitoring sessions.
type Layout int

const (
	// LayoutSingle displays one session full-screen (no splits).
	LayoutSingle Layout = iota
	// LayoutDual splits horizontally: session on the left, logs on the right.
	LayoutDual
	// LayoutQuad splits into a 2x2 grid for four sessions.
	LayoutQuad
	// LayoutFleet arranges N sessions in an auto-calculated grid (rows x cols).
	LayoutFleet
)

// String returns the human-readable name of a layout preset.
func (l Layout) String() string {
	switch l {
	case LayoutSingle:
		return "single"
	case LayoutDual:
		return "dual"
	case LayoutQuad:
		return "quad"
	case LayoutFleet:
		return "fleet"
	default:
		return fmt.Sprintf("layout(%d)", int(l))
	}
}

// ParseLayout converts a string name to a Layout constant.
// Returns an error for unrecognised names.
func ParseLayout(s string) (Layout, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "single":
		return LayoutSingle, nil
	case "dual":
		return LayoutDual, nil
	case "quad":
		return LayoutQuad, nil
	case "fleet":
		return LayoutFleet, nil
	default:
		return 0, fmt.Errorf("unknown layout %q", s)
	}
}

// Pane describes a single pane inside a layout, expressed in absolute
// character coordinates within the terminal window.
type Pane struct {
	// Label is a human-readable identifier (e.g. "session-0", "logs").
	Label string `json:"label"`
	// X and Y are the top-left origin in characters.
	X int `json:"x"`
	Y int `json:"y"`
	// W and H are the width and height in characters.
	W int `json:"w"`
	H int `json:"h"`
}

// Right returns the x-coordinate just past the right edge.
func (p Pane) Right() int { return p.X + p.W }

// Bottom returns the y-coordinate just past the bottom edge.
func (p Pane) Bottom() int { return p.Y + p.H }

// LayoutGeometry holds the computed pane positions for a given layout and
// terminal size.  It is a pure data structure that can be inspected in tests
// without touching tmux.
type LayoutGeometry struct {
	Layout Layout `json:"layout"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Panes  []Pane `json:"panes"`
}

// separatorWidth is the character width consumed by a tmux pane border.
const separatorWidth = 1

// ComputeGeometry calculates pane positions for the given layout, terminal
// dimensions, and (for fleet layout) the number of sessions.  The function is
// pure — it performs no I/O and is safe to call from tests.
//
// For LayoutFleet, n must be >= 1.  For the fixed layouts (Single, Dual, Quad)
// n is ignored.
func ComputeGeometry(layout Layout, width, height, n int) (*LayoutGeometry, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("terminal dimensions must be positive, got %dx%d", width, height)
	}

	geo := &LayoutGeometry{Layout: layout, Width: width, Height: height}

	switch layout {
	case LayoutSingle:
		geo.Panes = computeSingle(width, height)
	case LayoutDual:
		geo.Panes = computeDual(width, height)
	case LayoutQuad:
		geo.Panes = computeQuad(width, height)
	case LayoutFleet:
		if n < 1 {
			return nil, fmt.Errorf("fleet layout requires at least 1 session, got %d", n)
		}
		geo.Panes = computeFleet(width, height, n)
	default:
		return nil, fmt.Errorf("unsupported layout: %v", layout)
	}

	return geo, nil
}

// computeSingle: full-screen, one pane.
func computeSingle(w, h int) []Pane {
	return []Pane{
		{Label: "session-0", X: 0, Y: 0, W: w, H: h},
	}
}

// computeDual: vertical split — session (60 %) | logs (40 %).
func computeDual(w, h int) []Pane {
	leftW := (w - separatorWidth) * 60 / 100
	rightW := w - separatorWidth - leftW
	return []Pane{
		{Label: "session-0", X: 0, Y: 0, W: leftW, H: h},
		{Label: "logs", X: leftW + separatorWidth, Y: 0, W: rightW, H: h},
	}
}

// computeQuad: 2x2 grid.
func computeQuad(w, h int) []Pane {
	leftW := (w - separatorWidth) / 2
	rightW := w - separatorWidth - leftW
	topH := (h - separatorWidth) / 2
	botH := h - separatorWidth - topH

	return []Pane{
		{Label: "session-0", X: 0, Y: 0, W: leftW, H: topH},
		{Label: "session-1", X: leftW + separatorWidth, Y: 0, W: rightW, H: topH},
		{Label: "session-2", X: 0, Y: topH + separatorWidth, W: leftW, H: botH},
		{Label: "session-3", X: leftW + separatorWidth, Y: topH + separatorWidth, W: rightW, H: botH},
	}
}

// gridDimensions picks rows and cols for n items, preferring wider-than-tall
// arrangements (cols >= rows) to suit widescreen terminals.
func gridDimensions(n int) (rows, cols int) {
	if n <= 0 {
		return 0, 0
	}
	// Start with the integer square root, rounding up for cols.
	cols = 1
	for cols*cols < n {
		cols++
	}
	rows = (n + cols - 1) / cols
	return rows, cols
}

// computeFleet: auto-grid for N sessions.
func computeFleet(w, h, n int) []Pane {
	if n == 1 {
		return computeSingle(w, h)
	}

	rows, cols := gridDimensions(n)

	// Available space after subtracting separators between panes.
	usableW := w - separatorWidth*(cols-1)
	usableH := h - separatorWidth*(rows-1)

	// Base cell sizes and remainders for even distribution.
	baseW := usableW / cols
	extraW := usableW % cols
	baseH := usableH / rows
	extraH := usableH % rows

	panes := make([]Pane, 0, n)
	idx := 0
	yOff := 0
	for r := range rows {
		cellH := baseH
		if r < extraH {
			cellH++
		}
		xOff := 0
		for c := range cols {
			if idx >= n {
				break
			}
			cellW := baseW
			if c < extraW {
				cellW++
			}
			panes = append(panes, Pane{
				Label: fmt.Sprintf("session-%d", idx),
				X:     xOff,
				Y:     yOff,
				W:     cellW,
				H:     cellH,
			})
			xOff += cellW + separatorWidth
			idx++
		}
		yOff += cellH + separatorWidth
	}
	return panes
}

// Commander abstracts tmux command execution so that ApplyLayout can be
// tested without a real tmux server.
type Commander interface {
	// Run executes a tmux command with the given arguments and returns
	// combined stdout, or an error.
	Run(args ...string) (string, error)
}

// execCommander implements Commander by calling the real tmux binary.
type execCommander struct{}

func (execCommander) Run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// DefaultCommander returns a Commander that shells out to the tmux binary.
func DefaultCommander() Commander { return execCommander{} }

// ApplyLayout creates panes inside an existing tmux window to match the
// computed geometry.  It expects the target window already has one pane
// (pane index 0); additional panes are created via split-window.
//
// The session parameter is a tmux target in "session:window" form.
// The layout and terminal dimensions determine the split sequence.
//
// Returns the list of tmux pane IDs that were created (index 0 is the
// pre-existing pane).
func ApplyLayout(cmd Commander, session string, geo *LayoutGeometry) ([]string, error) {
	if len(geo.Panes) == 0 {
		return nil, fmt.Errorf("layout geometry has no panes")
	}

	// Collect pane IDs — the first pane already exists.
	firstID, err := cmd.Run("display-message", "-t", session, "-p", "#{pane_id}")
	if err != nil {
		return nil, fmt.Errorf("get initial pane ID: %w", err)
	}
	paneIDs := []string{firstID}

	// Create remaining panes via split-window with calculated percentages.
	for i := 1; i < len(geo.Panes); i++ {
		p := geo.Panes[i]
		prev := geo.Panes[i-1]

		var direction string
		var sizePercent int

		if p.Y == prev.Y {
			// Same row — horizontal split from the previous pane.
			direction = "-h"
			total := p.W + prev.W + separatorWidth
			if total > 0 {
				sizePercent = p.W * 100 / total
			}
		} else {
			// New row — vertical split from the first pane of the previous row.
			direction = "-v"
			total := p.H + prev.H + separatorWidth
			if total > 0 {
				sizePercent = p.H * 100 / total
			}
		}

		if sizePercent < 1 {
			sizePercent = 50
		}

		target := fmt.Sprintf("%s.%d", session, i-1)
		out, err := cmd.Run(
			"split-window", direction,
			"-t", target,
			"-p", fmt.Sprintf("%d", sizePercent),
			"-P", "-F", "#{pane_id}",
		)
		if err != nil {
			return paneIDs, fmt.Errorf("split pane %d: %w", i, err)
		}
		paneIDs = append(paneIDs, strings.TrimSpace(out))
	}

	return paneIDs, nil
}
