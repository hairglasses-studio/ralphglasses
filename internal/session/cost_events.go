package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
)

// CostEvent is a single cost record emitted for external consumption.
type CostEvent struct {
	SessionID    string    `json:"session_id"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	Repo         string    `json:"repo"`
	Timestamp    time.Time `json:"timestamp"`
}

// CostEventWriter appends cost events to a JSONL file.
type CostEventWriter struct {
	mu   sync.Mutex
	path string
	file *os.File
}

// NewCostEventWriter creates a writer that appends to the given path.
func NewCostEventWriter(path string) (*CostEventWriter, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cost events: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("cost events: open: %w", err)
	}
	return &CostEventWriter{path: path, file: f}, nil
}

// Write appends a cost event as a JSONL line.
func (w *CostEventWriter) Write(evt CostEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w.file, "%s\n", data)
	return err
}

// Close closes the underlying file.
func (w *CostEventWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

// DefaultCostEventPath returns the default path for cost events.
func DefaultCostEventPath() string {
	return ralphpath.CostEventsPath()
}
