package mcpserver

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleSessionExport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getStringArg(req, "session_id")
	if sessionID == "" {
		return codedError(ErrInvalidParams, "session_id is required"), nil
	}
	format := getStringArg(req, "format")
	if format == "" {
		format = "markdown"
	}
	if format != "markdown" && format != "json" {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid format %q: must be 'markdown' or 'json'", format)), nil
	}

	// Resolve the replay file path. First try to find the session to get its
	// repo path; fall back to searching all scanned repos.
	var replayPath string

	if sess, ok := s.SessMgr.Get(sessionID); ok {
		sess.Lock()
		repoPath := sess.RepoPath
		sess.Unlock()
		candidate := filepath.Join(repoPath, ".ralph", "replays", sessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			replayPath = candidate
		}
	}

	// If not found via live session, search all repos.
	if replayPath == "" {
		if s.reposNil() {
			_ = s.scan()
		}
		repos := s.reposCopy()
		for _, r := range repos {
			candidate := filepath.Join(r.Path, ".ralph", "replays", sessionID+".jsonl")
			if _, err := os.Stat(candidate); err == nil {
				replayPath = candidate
				break
			}
		}
	}

	// Also try explicit repo param.
	if replayPath == "" {
		repoName := getStringArg(req, "repo")
		if repoName != "" {
			repoPath, errRes := s.resolveRepoPath(repoName)
			if errRes != nil {
				return errRes, nil
			}
			candidate := filepath.Join(repoPath, ".ralph", "replays", sessionID+".jsonl")
			if _, err := os.Stat(candidate); err == nil {
				replayPath = candidate
			}
		}
	}

	if replayPath == "" {
		return codedError(ErrFilesystem, fmt.Sprintf("replay file not found for session %s — expected at .ralph/replays/%s.jsonl", sessionID, sessionID)), nil
	}

	player, err := session.NewPlayer(replayPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load replay: %v", err)), nil
	}

	// Build filter from optional params.
	var filter *session.ExportFilter
	eventTypesStr := getStringArg(req, "event_types")
	afterStr := getStringArg(req, "after")
	beforeStr := getStringArg(req, "before")

	if eventTypesStr != "" || afterStr != "" || beforeStr != "" {
		filter = &session.ExportFilter{}

		if eventTypesStr != "" {
			for _, t := range strings.Split(eventTypesStr, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					filter.EventTypes = append(filter.EventTypes, session.ReplayEventType(t))
				}
			}
		}
		if afterStr != "" {
			ts, err := time.Parse(time.RFC3339, afterStr)
			if err != nil {
				return codedError(ErrInvalidParams, fmt.Sprintf("invalid 'after' timestamp: %v", err)), nil
			}
			filter.After = ts
		}
		if beforeStr != "" {
			ts, err := time.Parse(time.RFC3339, beforeStr)
			if err != nil {
				return codedError(ErrInvalidParams, fmt.Sprintf("invalid 'before' timestamp: %v", err)), nil
			}
			filter.Before = ts
		}
	}

	var buf bytes.Buffer
	switch format {
	case "markdown":
		if err := session.ExportMarkdown(player, &buf, filter); err != nil {
			return codedError(ErrInternal, fmt.Sprintf("export markdown: %v", err)), nil
		}
		return textResult(buf.String()), nil
	case "json":
		if err := session.ExportJSON(player, &buf, filter); err != nil {
			return codedError(ErrInternal, fmt.Sprintf("export json: %v", err)), nil
		}
		return textResult(buf.String()), nil
	}

	return codedError(ErrInternal, "unreachable"), nil
}
