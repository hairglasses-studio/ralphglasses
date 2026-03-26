package mcpserver

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
)

// RegisterResources registers MCP resource templates for browsing .ralph state
// files. This enables clients to read repo state without tool calls — reducing
// latency and token cost.
func RegisterResources(srv *server.MCPServer, appSrv *Server) {
	// Resource template: ralph:///{repo}/status
	srv.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"ralph:///{repo}/status",
			"Repo status",
			mcp.WithTemplateDescription("Read .ralph/status.json for a repository"),
			mcp.WithTemplateMIMEType("application/json"),
		),
		makeStatusHandler(appSrv),
	)

	// Resource template: ralph:///{repo}/progress
	srv.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"ralph:///{repo}/progress",
			"Repo progress",
			mcp.WithTemplateDescription("Read .ralph/progress.json for a repository"),
			mcp.WithTemplateMIMEType("application/json"),
		),
		makeProgressHandler(appSrv),
	)

	// Resource template: ralph:///{repo}/logs
	srv.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"ralph:///{repo}/logs",
			"Repo logs",
			mcp.WithTemplateDescription("Read last 100 lines of .ralph/logs/ralph.log for a repository"),
			mcp.WithTemplateMIMEType("text/plain"),
		),
		makeLogsHandler(appSrv),
	)
}

// extractRepoName parses the repo name from a ralph:/// URI.
// Expected formats: ralph:///{repo}/status, ralph:///{repo}/progress, ralph:///{repo}/logs
func extractRepoName(uri string) string {
	// Strip the scheme prefix.
	const prefix = "ralph:///"
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	rest := uri[len(prefix):]
	// The repo name is everything before the first slash.
	if idx := strings.Index(rest, "/"); idx >= 0 {
		return rest[:idx]
	}
	return rest
}

func makeStatusHandler(appSrv *Server) server.ResourceTemplateHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		repoName := extractRepoName(req.Params.URI)
		if repoName == "" {
			return nil, fmt.Errorf("invalid URI: missing repo name")
		}

		repo, err := resolveRepo(appSrv, repoName)
		if err != nil {
			return nil, err
		}

		data, err := os.ReadFile(filepath.Join(repo.Path, ".ralph", "status.json"))
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("status.json not found for repo %s", repoName)
			}
			return nil, fmt.Errorf("reading status.json: %w", err)
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}

func makeProgressHandler(appSrv *Server) server.ResourceTemplateHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		repoName := extractRepoName(req.Params.URI)
		if repoName == "" {
			return nil, fmt.Errorf("invalid URI: missing repo name")
		}

		repo, err := resolveRepo(appSrv, repoName)
		if err != nil {
			return nil, err
		}

		data, err := os.ReadFile(filepath.Join(repo.Path, ".ralph", "progress.json"))
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("progress.json not found for repo %s", repoName)
			}
			return nil, fmt.Errorf("reading progress.json: %w", err)
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}

func makeLogsHandler(appSrv *Server) server.ResourceTemplateHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		repoName := extractRepoName(req.Params.URI)
		if repoName == "" {
			return nil, fmt.Errorf("invalid URI: missing repo name")
		}

		repo, err := resolveRepo(appSrv, repoName)
		if err != nil {
			return nil, err
		}

		logPath := process.LogFilePath(repo.Path)
		text, err := tailFile(logPath, 100)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("ralph.log not found for repo %s", repoName)
			}
			return nil, fmt.Errorf("reading ralph.log: %w", err)
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      req.Params.URI,
				MIMEType: "text/plain",
				Text:     text,
			},
		}, nil
	}
}

// resolveRepo ensures repos are scanned and finds the named repo.
func resolveRepo(appSrv *Server, name string) (*model.Repo, error) {
	if appSrv.reposNil() {
		if err := appSrv.scan(); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
	}
	r := appSrv.findRepo(name)
	if r == nil {
		return nil, fmt.Errorf("repo not found: %s", name)
	}
	return r, nil
}

// tailFile reads the last n lines from a file.
func tailFile(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	// Increase buffer size for potentially long log lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.Join(lines, "\n"), nil
}
