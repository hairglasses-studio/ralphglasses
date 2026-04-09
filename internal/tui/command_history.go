package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
)

// CommandHistory stores and replays TUI command history.
type CommandHistory struct {
	mu      sync.Mutex
	entries []string
	cursor  int
	maxLen  int
	path    string
}

// NewCommandHistory creates a persistent command history.
func NewCommandHistory(maxLen int) *CommandHistory {
	h := &CommandHistory{
		maxLen: maxLen,
		cursor: -1,
	}
	h.path = ralphpath.CommandHistoryPath()
	h.load()
	return h
}

// Add records a command.
func (h *CommandHistory) Add(cmd string) {
	if cmd == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	// Skip consecutive duplicates
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == cmd {
		return
	}

	h.entries = append(h.entries, cmd)
	if len(h.entries) > h.maxLen {
		h.entries = h.entries[len(h.entries)-h.maxLen:]
	}
	h.cursor = len(h.entries) // Reset cursor past end
	h.save()
}

// Previous returns the previous command in history (up arrow behavior).
func (h *CommandHistory) Previous() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) == 0 {
		return ""
	}
	if h.cursor > 0 {
		h.cursor--
	}
	return h.entries[h.cursor]
}

// Next returns the next command in history (down arrow behavior).
func (h *CommandHistory) Next() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) == 0 || h.cursor >= len(h.entries)-1 {
		h.cursor = len(h.entries)
		return ""
	}
	h.cursor++
	return h.entries[h.cursor]
}

// Reset puts the cursor past the end of history.
func (h *CommandHistory) Reset() {
	h.mu.Lock()
	h.cursor = len(h.entries)
	h.mu.Unlock()
}

// List returns all history entries.
func (h *CommandHistory) List() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]string, len(h.entries))
	copy(result, h.entries)
	return result
}

func (h *CommandHistory) load() {
	if h.path == "" {
		return
	}
	data, err := os.ReadFile(h.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &h.entries)
	h.cursor = len(h.entries)
}

func (h *CommandHistory) save() {
	if h.path == "" {
		return
	}
	dir := filepath.Dir(h.path)
	_ = os.MkdirAll(dir, 0755)
	data, _ := json.Marshal(h.entries)
	_ = os.WriteFile(h.path, data, 0644)
}
