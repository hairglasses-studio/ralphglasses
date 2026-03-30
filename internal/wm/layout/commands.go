package layout

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// WindowPlacement describes the position and size of a single window
// within a layout.
type WindowPlacement struct {
	Workspace string `json:"workspace"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SessionID string `json:"session_id"`
}

// Layout is a named, serializable window arrangement.
type Layout struct {
	Name         string            `json:"name"`
	Windows      []WindowPlacement `json:"windows"`
	CreatedAt    time.Time         `json:"created_at"`
	MonitorCount int               `json:"monitor_count"`
}

// Command is a named, executable layout operation.
type Command struct {
	Name        string
	Description string
	Execute     func(args []string) error
}

// Registry holds the set of available layout commands and the state
// needed to execute them (storage directory, current layout snapshot).
type Registry struct {
	mu       sync.RWMutex
	commands map[string]*Command
	storeDir string

	// CurrentLayout is a callback the caller sets so the registry can
	// snapshot the live layout for "save". If nil, save produces an
	// empty layout.
	CurrentLayout func() *Layout

	// OnLoad is called after "load" successfully reads a layout from
	// disk. The caller uses it to apply the layout to the window
	// manager. If nil, load is a no-op beyond reading.
	OnLoad func(*Layout) error
}

// NewRegistry creates a Registry with the built-in layout commands.
// storeDir overrides the default storage path; pass "" to use
// ~/.config/ralphglasses/layouts/.
func NewRegistry(storeDir string) *Registry {
	if storeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		storeDir = filepath.Join(home, ".config", "ralphglasses", "layouts")
	}

	r := &Registry{
		commands:  make(map[string]*Command),
		storeDir:  storeDir,
	}

	r.register(&Command{
		Name:        "save",
		Description: "Save current window layout to JSON",
		Execute: func(args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: layout save <name>")
			}
			return r.saveLayout(args[0])
		},
	})

	r.register(&Command{
		Name:        "load",
		Description: "Restore a saved layout",
		Execute: func(args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: layout load <name>")
			}
			return r.loadLayout(args[0])
		},
	})

	r.register(&Command{
		Name:        "list",
		Description: "List saved layouts",
		Execute: func(_ []string) error {
			return r.listLayouts()
		},
	})

	r.register(&Command{
		Name:        "delete",
		Description: "Delete a saved layout",
		Execute: func(args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: layout delete <name>")
			}
			return r.deleteLayout(args[0])
		},
	})

	r.register(&Command{
		Name:        "auto",
		Description: "Auto-arrange windows for current monitor count",
		Execute: func(args []string) error {
			monitorCount := 1
			if len(args) > 0 {
				n, err := parseInt(args[0])
				if err != nil {
					return fmt.Errorf("invalid monitor count: %v", err)
				}
				monitorCount = n
			}
			return r.autoArrange(monitorCount)
		},
	})

	r.register(&Command{
		Name:        "reset",
		Description: "Reset to default layout",
		Execute: func(_ []string) error {
			return r.resetLayout()
		},
	})

	return r
}

// Commands returns all registered commands sorted by name.
func (r *Registry) Commands() []*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmds := make([]*Command, 0, len(r.commands))
	for _, c := range r.commands {
		cmds = append(cmds, c)
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

// Get returns the command with the given name, or nil.
func (r *Registry) Get(name string) *Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.commands[name]
}

// Run parses a command line like "save my-layout" and executes it.
func (r *Registry) Run(line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}
	cmd := r.Get(parts[0])
	if cmd == nil {
		return fmt.Errorf("unknown layout command: %q", parts[0])
	}
	return cmd.Execute(parts[1:])
}

// StoreDir returns the directory where layouts are persisted.
func (r *Registry) StoreDir() string {
	return r.storeDir
}

// --- internal helpers ---

func (r *Registry) register(cmd *Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[cmd.Name] = cmd
}

func (r *Registry) layoutPath(name string) string {
	return filepath.Join(r.storeDir, name+".json")
}

func (r *Registry) ensureStoreDir() error {
	return os.MkdirAll(r.storeDir, 0o755)
}

func (r *Registry) saveLayout(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if err := r.ensureStoreDir(); err != nil {
		return fmt.Errorf("create layout dir: %w", err)
	}

	path := r.layoutPath(name)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("layout %q already exists; delete it first", name)
	}

	var lay Layout
	if r.CurrentLayout != nil {
		if snap := r.CurrentLayout(); snap != nil {
			lay = *snap
		}
	}
	lay.Name = name
	lay.CreatedAt = time.Now().UTC()

	data, err := json.MarshalIndent(lay, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal layout: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func (r *Registry) loadLayout(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := r.layoutPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("layout %q not found", name)
		}
		return fmt.Errorf("read layout: %w", err)
	}

	var lay Layout
	if err := json.Unmarshal(data, &lay); err != nil {
		return fmt.Errorf("parse layout %q: %w", name, err)
	}

	if r.OnLoad != nil {
		return r.OnLoad(&lay)
	}
	return nil
}

// ListResult holds metadata for one saved layout returned by ListLayouts.
type ListResult struct {
	Name         string
	MonitorCount int
	WindowCount  int
	CreatedAt    time.Time
}

// ListLayouts returns metadata for all saved layouts, sorted by name.
func (r *Registry) ListLayouts() ([]ListResult, error) {
	entries, err := os.ReadDir(r.storeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var results []ListResult
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(r.storeDir, e.Name()))
		if err != nil {
			continue
		}
		var lay Layout
		if err := json.Unmarshal(data, &lay); err != nil {
			continue
		}
		results = append(results, ListResult{
			Name:         lay.Name,
			MonitorCount: lay.MonitorCount,
			WindowCount:  len(lay.Windows),
			CreatedAt:    lay.CreatedAt,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results, nil
}

func (r *Registry) listLayouts() error {
	_, err := r.ListLayouts()
	return err
}

func (r *Registry) deleteLayout(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := r.layoutPath(name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("layout %q not found", name)
	}
	return os.Remove(path)
}

// AutoArrange computes a tiled layout for the given number of monitors,
// distributing windows evenly. It returns the generated Layout without
// persisting it.
func AutoArrange(monitorCount, windowCount int) *Layout {
	if monitorCount < 1 {
		monitorCount = 1
	}
	if windowCount < 1 {
		return &Layout{
			Name:         "auto",
			MonitorCount: monitorCount,
			CreatedAt:    time.Now().UTC(),
		}
	}

	const screenW, screenH = 1920, 1080

	var placements []WindowPlacement

	// Distribute windows across monitors round-robin, then tile each
	// monitor's windows in a grid.
	perMonitor := make([]int, monitorCount)
	for i := 0; i < windowCount; i++ {
		perMonitor[i%monitorCount]++
	}

	windowIdx := 0
	for mon := 0; mon < monitorCount; mon++ {
		n := perMonitor[mon]
		if n == 0 {
			continue
		}
		cols := int(math.Ceil(math.Sqrt(float64(n))))
		rows := int(math.Ceil(float64(n) / float64(cols)))
		cellW := screenW / cols
		cellH := screenH / rows

		for i := 0; i < n; i++ {
			col := i % cols
			row := i / cols
			placements = append(placements, WindowPlacement{
				Workspace: fmt.Sprintf("ws-%d", mon+1),
				X:         col * cellW,
				Y:         row * cellH,
				Width:     cellW,
				Height:    cellH,
				SessionID: fmt.Sprintf("session-%d", windowIdx),
			})
			windowIdx++
		}
	}

	return &Layout{
		Name:         "auto",
		Windows:      placements,
		MonitorCount: monitorCount,
		CreatedAt:    time.Now().UTC(),
	}
}

func (r *Registry) autoArrange(monitorCount int) error {
	windowCount := 0
	if r.CurrentLayout != nil {
		if snap := r.CurrentLayout(); snap != nil {
			windowCount = len(snap.Windows)
		}
	}
	if windowCount == 0 {
		windowCount = monitorCount // at least one per monitor
	}

	lay := AutoArrange(monitorCount, windowCount)
	if r.OnLoad != nil {
		return r.OnLoad(lay)
	}
	return nil
}

func (r *Registry) resetLayout() error {
	lay := &Layout{
		Name:         "default",
		MonitorCount: 1,
		Windows: []WindowPlacement{
			{Workspace: "ws-1", X: 0, Y: 0, Width: 1920, Height: 1080, SessionID: ""},
		},
		CreatedAt: time.Now().UTC(),
	}
	if r.OnLoad != nil {
		return r.OnLoad(lay)
	}
	return nil
}

// validateName rejects empty or path-separator-containing names.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("layout name must not be empty")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("layout name must not contain path separators")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("layout name %q is reserved", name)
	}
	return nil
}

// parseInt is a minimal integer parser to avoid importing strconv in
// the main module path (keeps dependency surface small).
func parseInt(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	neg := false
	start := 0
	if s[0] == '-' {
		neg = true
		start = 1
	} else if s[0] == '+' {
		start = 1
	}
	if start >= len(s) {
		return 0, fmt.Errorf("invalid number: %q", s)
	}
	n := 0
	for i := start; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid number: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}
