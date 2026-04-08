package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/parity"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
)

// RegisterResources registers MCP resource templates for browsing .ralph state
// files. This enables clients to read repo state without tool calls — reducing
// latency and token cost.
func RegisterResources(srv *server.MCPServer, appSrv *Server) {
	templateHandlers := map[string]server.ResourceTemplateHandlerFunc{
		"ralph:///{repo}/status":   makeStatusHandler(appSrv),
		"ralph:///{repo}/progress": makeProgressHandler(appSrv),
		"ralph:///{repo}/logs":     makeLogsHandler(appSrv),
	}

	for _, def := range resourceTemplateCatalog() {
		handler, ok := templateHandlers[def.URI]
		if !ok {
			panic("missing resource template handler for " + def.URI)
		}
		srv.AddResourceTemplate(
			mcp.NewResourceTemplate(
				def.URI,
				def.Name,
				mcp.WithTemplateDescription(def.Description),
				mcp.WithTemplateMIMEType(def.MIMEType),
			),
			handler,
		)
	}

	staticHandlers := map[string]server.ResourceHandlerFunc{
		"ralph:///catalog/server":      makeCatalogServerHandler(appSrv),
		"ralph:///catalog/tool-groups": makeCatalogToolGroupsHandler(appSrv),
		"ralph:///catalog/workflows":   makeCatalogWorkflowsHandler(),
		"ralph:///catalog/skills":      makeCatalogSkillsHandler(),
		"ralph:///catalog/cli-parity":  makeCLIParityHandler(appSrv),
		"ralph:///bootstrap/checklist": makeBootstrapChecklistHandler(),
		"ralph:///runtime/health":      makeRuntimeHealthHandler(appSrv),
	}

	for _, def := range staticResourceCatalog() {
		handler, ok := staticHandlers[def.URI]
		if !ok {
			panic("missing static resource handler for " + def.URI)
		}
		srv.AddResources(server.ServerResource{
			Resource: mcp.NewResource(
				def.URI,
				def.Name,
				mcp.WithResourceDescription(def.Description),
				mcp.WithMIMEType(def.MIMEType),
			),
			Handler: handler,
		})
	}
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

func makeCatalogServerHandler(appSrv *Server) server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, buildCatalogServerDoc(appSrv))
	}
}

func makeCatalogToolGroupsHandler(appSrv *Server) server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, buildCatalogToolGroupsDoc(appSrv))
	}
}

func makeCatalogWorkflowsHandler() server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, workflowCatalog())
	}
}

func makeCatalogSkillsHandler() server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, skillCatalog())
	}
}

func makeCLIParityHandler(appSrv *Server) server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, parity.CLIParityDocumentWithUsage(parity.DefaultCLIParityUsageOptions(appSrv.ScanPath)))
	}
}

func makeBootstrapChecklistHandler() server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, buildBootstrapChecklistDoc())
	}
}

func makeRuntimeHealthHandler(appSrv *Server) server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, appSrv.runtimeHealthDoc())
	}
}

func buildCatalogServerDoc(appSrv *Server) map[string]any {
	usageSummary := parity.CLIParityUsage(parity.DefaultCLIParityUsageOptions(appSrv.ScanPath))
	return map[string]any{
		"server_name":             "ralphglasses",
		"instructions":            ServerInstructions(),
		"tool_group_count":        len(ToolGroupNames),
		"group_tool_count":        GeneratedTotalTools,
		"management_tool_count":   len(managementToolNames()),
		"tool_count":              GeneratedTotalTools + len(managementToolNames()),
		"resource_count":          len(staticResourceCatalog()),
		"resource_template_count": len(resourceTemplateCatalog()),
		"skill_count":             len(skillCatalog()),
		"prompt_count":            len(promptCatalog()),
		"deferred_mode_default":   true,
		"management_tools":        managementToolNames(),
		"resources":               resourceURIs(staticResourceCatalog()),
		"resource_templates":      resourceTemplateURIs(resourceTemplateCatalog()),
		"skills":                  skillNames(),
		"cli_parity_summary":      parity.CLIParityCoverage(),
		"cli_parity_usage":        usageSummary,
		"prompts":                 promptNames(),
		"tool_groups":             buildCatalogToolGroupsDoc(appSrv),
	}
}

func buildBootstrapChecklistDoc() map[string]any {
	return map[string]any{
		"title":       "Ralphglasses MCP-first bootstrap checklist",
		"description": "Use this checklist to validate provider readiness, config health, and the first safe MCP interactions before launching work.",
		"resources": []string{
			"ralph:///catalog/server",
			"ralph:///catalog/skills",
			"ralph:///catalog/workflows",
			"ralph:///runtime/health",
		},
		"prompts": []string{
			"bootstrap-firstboot",
		},
		"skills": []string{
			"ralphglasses-bootstrap",
			"ralphglasses-operator",
		},
		"key_tools": []string{
			"ralphglasses_doctor",
			"ralphglasses_validate",
			"ralphglasses_firstboot_profile",
			"ralphglasses_repo_scaffold",
			"ralphglasses_server_health",
		},
		"steps": []map[string]any{
			{
				"name":       "provider-readiness",
				"goal":       "Confirm provider CLIs and authentication are healthy before any repo mutation.",
				"tools":      []string{"ralphglasses_doctor"},
				"skill":      "ralphglasses-bootstrap",
				"validation": "Doctor reports the required providers as ready.",
			},
			{
				"name":       "config-validation",
				"goal":       "Inspect and validate repo-local config before applying profiles or scaffold changes.",
				"tools":      []string{"ralphglasses_validate", "ralphglasses_config_schema"},
				"validation": "Validation returns no blocking errors for the target repo or scan path.",
			},
			{
				"name":       "profile-application",
				"goal":       "Apply or inspect the best available firstboot profile before using the interactive wizard.",
				"tools":      []string{"ralphglasses_firstboot_profile"},
				"skill":      "ralphglasses-bootstrap",
				"validation": "The selected profile matches the intended provider/runtime posture.",
			},
			{
				"name":       "interactive-bridge",
				"goal":       "Use the operator-first path only when the remaining setup step is inherently interactive.",
				"skill":      "ralphglasses-operator",
				"validation": "Any terminal-native firstboot step is followed by MCP health verification.",
			},
		},
	}
}

func buildCatalogToolGroupsDoc(appSrv *Server) []map[string]any {
	groups := appSrv.buildToolGroups()
	out := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		toolNames := make([]string, 0, len(group.Tools))
		for _, entry := range group.Tools {
			toolNames = append(toolNames, entry.Tool.Name)
		}
		sort.Strings(toolNames)
		out = append(out, map[string]any{
			"name":        group.Name,
			"description": group.Description,
			"tool_count":  len(group.Tools),
			"tools":       toolNames,
		})
	}
	return out
}

func resourceURIs(resources []ResourceDef) []string {
	out := make([]string, 0, len(resources))
	for _, resource := range resources {
		out = append(out, resource.URI)
	}
	sort.Strings(out)
	return out
}

func resourceTemplateURIs(resources []ResourceTemplateDef) []string {
	out := make([]string, 0, len(resources))
	for _, resource := range resources {
		out = append(out, resource.URI)
	}
	sort.Strings(out)
	return out
}

func promptNames() []string {
	defs := promptCatalog()
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		out = append(out, def.Name)
	}
	sort.Strings(out)
	return out
}

func skillNames() []string {
	defs := skillCatalog()
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		out = append(out, def.Name)
	}
	sort.Strings(out)
	return out
}

func jsonResourceContents(uri string, value any) ([]mcp.ResourceContents, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal resource %s: %w", uri, err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
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
