package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleSessionFork(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParams(req)

	parentID, errResult := pp.RequireString("id")
	if errResult != nil {
		return errResult, nil
	}

	if s.SessMgr == nil {
		return codedError(ErrInternal, "session manager not initialized"), nil
	}

	opts := session.ForkOptions{
		Prompt:              getStringArg(req, "prompt"),
		Model:               getStringArg(req, "model"),
		InjectParentContext: pp.OptionalBool("inject_context", true),
	}

	if p := getStringArg(req, "provider"); p != "" {
		opts.Provider = session.Provider(p)
	}
	if b := getNumberArg(req, "budget_usd", 0); b > 0 {
		opts.MaxBudgetUSD = b
	}

	child, err := s.SessMgr.Fork(ctx, parentID, opts)
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}

	return jsonResult(map[string]any{
		"id":         child.ID,
		"parent_id":  parentID,
		"fork_point": child.ForkPoint,
		"status":     child.Status,
		"provider":   child.Provider,
		"model":      child.Model,
		"repo_path":  child.RepoPath,
	}), nil
}
