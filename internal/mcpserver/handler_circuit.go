package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// handleCircuitReset resets circuit breaker state for a named service.
// Supported services:
//   - "enhancer" — resets the in-memory LLM prompt enhancer circuit breaker
//   - "<repo-name>" — resets the file-based .ralph/.circuit_breaker_state for a repo
func (s *Server) handleCircuitReset(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	service := getStringArg(req, "service")
	if service == "" {
		return codedError(ErrInvalidParams, "service parameter is required"), nil
	}

	// Special case: reset the in-memory enhancer circuit breaker.
	if service == "enhancer" {
		engine := s.getEngine()
		if engine == nil {
			return codedError(ErrProviderUnavailable, "enhancer engine not available (no API key or LLM disabled)"), nil
		}
		prevState := engine.CB.State()
		engine.CB.Reset()
		return jsonResult(map[string]any{
			"status":         "reset",
			"service":        "enhancer",
			"previous_state": prevState,
		}), nil
	}

	// Otherwise treat service as a repo name and reset its file-based circuit breaker.
	repo := s.findRepo(service)
	if repo == nil {
		return codedError(ErrServiceNotFound, fmt.Sprintf("no repo or service named %q — use 'enhancer' for LLM circuit breaker or a repo name", service)), nil
	}

	cbPath := filepath.Join(repo.Path, ".ralph", ".circuit_breaker_state")

	// Read current state for the response.
	prevState := "unknown"
	if existing, err := model.LoadCircuitBreaker(repo.Path); err == nil {
		prevState = existing.State
	}

	// Write a fresh closed state.
	now := time.Now()
	fresh := model.CircuitBreakerState{
		State:      "CLOSED",
		LastChange: now,
	}
	data, err := json.MarshalIndent(fresh, "", "  ")
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("marshal: %v", err)), nil
	}

	// Ensure .ralph directory exists.
	if err := os.MkdirAll(filepath.Dir(cbPath), 0755); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("mkdir: %v", err)), nil
	}

	if err := os.WriteFile(cbPath, data, 0644); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("write circuit breaker state: %v", err)), nil
	}

	// Update the in-memory repo state under lock.
	s.mu.Lock()
	for _, r := range s.Repos {
		if r.Name == service {
			r.Circuit = &fresh
			break
		}
	}
	s.mu.Unlock()

	return jsonResult(map[string]any{
		"status":         "reset",
		"service":        service,
		"previous_state": prevState,
	}), nil
}
