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
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/parity"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// RegisterResources registers MCP resource templates for browsing .ralph state
// files. This enables clients to read repo state without tool calls — reducing
// latency and token cost.
func RegisterResources(srv *server.MCPServer, appSrv *Server) {
	templateHandlers := map[string]server.ResourceTemplateHandlerFunc{
		"ralph:///{repo}/triage":   makeTriageHandler(appSrv),
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
			instrumentResourceTemplateHandler(appSrv, def.URI, handler),
		)
	}

	staticHandlers := map[string]server.ResourceHandlerFunc{
		"ralph:///catalog/server":              makeCatalogServerHandler(appSrv),
		"ralph:///catalog/tool-groups":         makeCatalogToolGroupsHandler(appSrv),
		"ralph:///catalog/workflows":           makeCatalogWorkflowsHandler(),
		"ralph:///catalog/skills":              makeCatalogSkillsHandler(),
		"ralph:///catalog/cli-parity":          makeCLIParityHandler(appSrv),
		"ralph:///catalog/discovery-adoption":  makeDiscoveryAdoptionHandler(appSrv),
		"ralph:///catalog/adoption-priorities": makeAdoptionPrioritiesHandler(appSrv),
		"ralph:///bootstrap/checklist":         makeBootstrapChecklistHandler(),
		"ralph:///runtime/recovery":            makeRuntimeRecoveryHandler(appSrv),
		"ralph:///runtime/sessions":            makeRuntimeSessionsHandler(appSrv),
		"ralph:///runtime/operator":            makeRuntimeOperatorHandler(appSrv),
		"ralph:///runtime/health":              makeRuntimeHealthHandler(appSrv),
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
			Handler: instrumentResourceHandler(appSrv, def.URI, handler),
		})
	}
}

// extractRepoName parses the repo name from a ralph:/// URI.
// Expected formats: ralph:///{repo}/triage, ralph:///{repo}/status, ralph:///{repo}/progress, ralph:///{repo}/logs
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

func makeTriageHandler(appSrv *Server) server.ResourceTemplateHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		repoName := extractRepoName(req.Params.URI)
		if repoName == "" {
			return nil, fmt.Errorf("invalid URI: missing repo name")
		}

		repo, err := resolveRepo(appSrv, repoName)
		if err != nil {
			return nil, err
		}

		return jsonResourceContents(req.Params.URI, buildRepoTriageDoc(appSrv, repo))
	}
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

func makeDiscoveryAdoptionHandler(appSrv *Server) server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, appSrv.discoveryAdoptionSummary())
	}
}

func makeAdoptionPrioritiesHandler(appSrv *Server) server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, appSrv.adoptionPrioritySummary())
	}
}

func makeBootstrapChecklistHandler() server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, buildBootstrapChecklistDoc())
	}
}

func makeRuntimeRecoveryHandler(appSrv *Server) server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, appSrv.buildRuntimeRecoveryDoc(ctx))
	}
}

func makeRuntimeSessionsHandler(appSrv *Server) server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, appSrv.buildRuntimeSessionsDoc(ctx))
	}
}

func makeRuntimeOperatorHandler(appSrv *Server) server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, appSrv.buildRuntimeOperatorDoc(ctx))
	}
}

func makeRuntimeHealthHandler(appSrv *Server) server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return jsonResourceContents(req.Params.URI, appSrv.runtimeHealthDoc())
	}
}

func buildCatalogServerDoc(appSrv *Server) map[string]any {
	usageSummary := parity.CLIParityUsage(parity.DefaultCLIParityUsageOptions(appSrv.ScanPath))
	discoverySummary := appSrv.discoveryAdoptionSummary()
	return map[string]any{
		"server_name":                "ralphglasses",
		"instructions":               ServerInstructions(),
		"tool_group_count":           len(ToolGroupNames),
		"group_tool_count":           GeneratedTotalTools,
		"management_tool_count":      len(managementToolNames()),
		"tool_count":                 GeneratedTotalTools + len(managementToolNames()),
		"resource_count":             len(staticResourceCatalog()),
		"resource_template_count":    len(resourceTemplateCatalog()),
		"skill_count":                len(skillCatalog()),
		"prompt_count":               len(promptCatalog()),
		"deferred_mode_default":      true,
		"management_tools":           managementToolNames(),
		"resources":                  resourceURIs(staticResourceCatalog()),
		"resource_templates":         resourceTemplateURIs(resourceTemplateCatalog()),
		"skills":                     skillNames(),
		"cli_parity_summary":         parity.CLIParityCoverage(),
		"cli_parity_usage":           usageSummary,
		"discovery_adoption_summary": discoverySummary,
		"adoption_priority_summary":  appSrv.adoptionPrioritySummary(),
		"prompts":                    promptNames(),
		"tool_groups":                buildCatalogToolGroupsDoc(appSrv),
	}
}

func buildRepoTriageDoc(appSrv *Server, repo *model.Repo) map[string]any {
	triage := map[string]any{
		"repo":               repo.Name,
		"repo_path":          repo.Path,
		"has_ralph":          repo.HasRalph,
		"recommended_prompt": "repo-triage-brief",
		"recommended_skills": []string{
			"ralphglasses-repo-admin",
			"ralphglasses-recovery-observability",
			"ralphglasses-self-dev",
		},
		"supporting_resources": map[string]string{
			"triage":           fmt.Sprintf("ralph:///%s/triage", repo.Name),
			"status":           fmt.Sprintf("ralph:///%s/status", repo.Name),
			"progress":         fmt.Sprintf("ralph:///%s/progress", repo.Name),
			"logs":             fmt.Sprintf("ralph:///%s/logs", repo.Name),
			"runtime_recovery": "ralph:///runtime/recovery",
			"runtime_health":   "ralph:///runtime/health",
		},
	}

	var notes []string
	if status, err := readJSONFile(filepath.Join(repo.Path, ".ralph", "status.json")); err == nil {
		triage["status"] = status
	} else if !os.IsNotExist(err) {
		notes = append(notes, fmt.Sprintf("status.json: %v", err))
	} else {
		notes = append(notes, "status.json not found")
	}

	if progress, err := readJSONFile(filepath.Join(repo.Path, ".ralph", "progress.json")); err == nil {
		triage["progress"] = progress
	} else if !os.IsNotExist(err) {
		notes = append(notes, fmt.Sprintf("progress.json: %v", err))
	} else {
		notes = append(notes, "progress.json not found")
	}

	if logs, err := tailFile(process.LogFilePath(repo.Path), 40); err == nil && strings.TrimSpace(logs) != "" {
		triage["recent_logs"] = logs
	} else if err != nil && !os.IsNotExist(err) {
		notes = append(notes, fmt.Sprintf("ralph.log: %v", err))
	} else if err != nil {
		notes = append(notes, "ralph.log not found")
	}

	runtime := appSrv.runtimeHealthDoc()
	triage["runtime_health"] = map[string]any{
		"status":                    runtime["status"],
		"deferred_mode":             runtime["deferred_mode"],
		"loaded_groups":             runtime["loaded_groups"],
		"tool_group_count":          runtime["tool_group_count"],
		"resource_template_count":   runtime["resource_template_count"],
		"prompt_count":              runtime["prompt_count"],
		"highest_priority_workflow": nestedString(runtime["adoption_priority_summary"], "highest_priority_workflow"),
	}

	if len(notes) > 0 {
		triage["notes"] = notes
	}
	return triage
}

func (s *Server) buildRuntimeRecoveryDoc(ctx context.Context) map[string]any {
	until := time.Now().UTC()
	since := until.Add(-24 * time.Hour)
	triageStatuses := []string{string(session.StatusInterrupted), string(session.StatusErrored)}
	triage := s.buildSessionTriageSummary(ctx, "", triageStatuses, since, until)

	stalled := make([]string, 0)
	if s.SessMgr != nil {
		stalled = s.SessMgr.DetectStalls(session.DefaultStallThreshold)
		sort.Strings(stalled)
	}

	candidates := make([]map[string]any, 0)
	for _, raw := range s.collectTriagedSessions(ctx, "", triageStatuses, since, until) {
		candidates = append(candidates, map[string]any{
			"id":                  raw.ID,
			"repo":                raw.RepoName,
			"provider":            raw.Provider,
			"model":               raw.Model,
			"status":              raw.Status,
			"priority_score":      scorePriority(raw),
			"assessment":          classifySalvage(raw),
			"estimated_retry_usd": estimateRetryCost(raw),
			"kill_reason":         classifySessionKillReason(raw),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		pi, _ := candidates[i]["priority_score"].(float64)
		pj, _ := candidates[j]["priority_score"].(float64)
		if pi == pj {
			ii, _ := candidates[i]["id"].(string)
			jj, _ := candidates[j]["id"].(string)
			return ii < jj
		}
		return pi > pj
	})
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}

	runtime := s.runtimeHealthDoc()
	return map[string]any{
		"title":             "Ralphglasses runtime recovery front door",
		"description":       "Read the current recovery posture before resuming sessions, marathon work, or repo-specific incident response.",
		"recommended_skill": "ralphglasses-recovery-observability",
		"recommended_skills": []string{
			"ralphglasses-recovery-observability",
			"ralphglasses-session-ops",
			"ralphglasses-bootstrap",
		},
		"supporting_resources": map[string]string{
			"runtime_recovery": "ralph:///runtime/recovery",
			"runtime_sessions": "ralph:///runtime/sessions",
			"runtime_health":   "ralph:///runtime/health",
			"skills":           "ralph:///catalog/skills",
			"priorities":       "ralph:///catalog/adoption-priorities",
			"repo_logs":        "ralph:///{repo}/logs",
		},
		"supporting_tools": []string{
			"ralphglasses_server_health",
			"ralphglasses_session_triage",
			"ralphglasses_recovery_plan",
			"ralphglasses_logs",
		},
		"recovery_window":         triage["incident_window"],
		"stalled_session_ids":     stalled,
		"stalled_session_count":   len(stalled),
		"session_triage":          triage,
		"top_recovery_candidates": candidates,
		"runtime_health": map[string]any{
			"status":                    runtime["status"],
			"deferred_mode":             runtime["deferred_mode"],
			"loaded_groups":             runtime["loaded_groups"],
			"tool_group_count":          runtime["tool_group_count"],
			"resource_count":            runtime["resource_count"],
			"prompt_count":              runtime["prompt_count"],
			"highest_priority_workflow": nestedString(runtime["adoption_priority_summary"], "highest_priority_workflow"),
		},
	}
}

func (s *Server) buildRuntimeSessionsDoc(_ context.Context) map[string]any {
	sessions := []*session.Session(nil)
	if s.SessMgr != nil {
		sessions = s.SessMgr.List("")
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].LastActivity.Equal(sessions[j].LastActivity) {
			return sessions[i].ID < sessions[j].ID
		}
		return sessions[i].LastActivity.After(sessions[j].LastActivity)
	})

	statusCounts := make(map[string]int)
	providerCounts := make(map[string]int)
	overBudget := make([]string, 0)
	recent := make([]map[string]any, 0, len(sessions))
	activeCount := 0
	for _, sess := range sessions {
		statusCounts[string(sess.Status)]++
		providerCounts[string(sess.Provider)]++
		if sess.Status == session.StatusRunning || sess.Status == session.StatusLaunching {
			activeCount++
		}
		if sess.BudgetUSD > 0 && sess.SpentUSD >= sess.BudgetUSD {
			overBudget = append(overBudget, sess.ID)
		}
		recent = append(recent, map[string]any{
			"id":            sess.ID,
			"repo":          sess.RepoName,
			"provider":      sess.Provider,
			"status":        sess.Status,
			"model":         sess.Model,
			"spent_usd":     sess.SpentUSD,
			"budget_usd":    sess.BudgetUSD,
			"turn_count":    sess.TurnCount,
			"team_name":     sess.TeamName,
			"last_activity": sess.LastActivity,
			"resumed":       sess.Resumed,
		})
	}
	if len(recent) > 8 {
		recent = recent[:8]
	}

	stalled := make([]string, 0)
	if s.SessMgr != nil {
		stalled = s.SessMgr.DetectStalls(session.DefaultStallThreshold)
		sort.Strings(stalled)
	}
	sort.Strings(overBudget)

	runtime := s.runtimeHealthDoc()
	return map[string]any{
		"title":                     "Ralphglasses session execution front door",
		"description":               "Read the live session posture before launching, resuming, handing off, or budget-tuning provider work.",
		"recommended_skill":         "ralphglasses-session-ops",
		"recommended_skills":        []string{"ralphglasses-session-ops", "ralphglasses-recovery-observability"},
		"highest_priority_workflow": "session-execution",
		"supporting_resources": map[string]string{
			"runtime_sessions": "ralph:///runtime/sessions",
			"runtime_health":   "ralph:///runtime/health",
			"runtime_recovery": "ralph:///runtime/recovery",
			"skills":           "ralph:///catalog/skills",
			"priorities":       "ralph:///catalog/adoption-priorities",
		},
		"supporting_tools": []string{
			"ralphglasses_session_launch",
			"ralphglasses_session_list",
			"ralphglasses_session_status",
			"ralphglasses_session_budget",
			"ralphglasses_session_handoff",
		},
		"session_count":           len(sessions),
		"active_session_count":    activeCount,
		"stalled_session_ids":     stalled,
		"stalled_session_count":   len(stalled),
		"over_budget_session_ids": overBudget,
		"status_breakdown":        statusCounts,
		"provider_breakdown":      providerCounts,
		"recent_sessions":         recent,
		"runtime_health": map[string]any{
			"status":                    runtime["status"],
			"loaded_groups":             runtime["loaded_groups"],
			"tool_group_count":          runtime["tool_group_count"],
			"resource_count":            runtime["resource_count"],
			"prompt_count":              runtime["prompt_count"],
			"highest_priority_workflow": nestedString(runtime["adoption_priority_summary"], "highest_priority_workflow"),
		},
	}
}

func (s *Server) buildRuntimeOperatorDoc(ctx context.Context) map[string]any {
	sessions := s.buildRuntimeSessionsDoc(ctx)
	runtime := s.runtimeHealthDoc()
	return map[string]any{
		"title":                     "Ralphglasses operator control-plane front door",
		"description":               "Read this before using the interactive TUI, tmux control loops, fleet runtime, or marathon execution paths.",
		"recommended_skill":         "ralphglasses-operator",
		"recommended_skills":        []string{"ralphglasses-operator", "ralphglasses-bootstrap", "ralphglasses-session-ops"},
		"highest_priority_workflow": "operator-control-plane",
		"supporting_resources": map[string]string{
			"runtime_operator": "ralph:///runtime/operator",
			"runtime_sessions": "ralph:///runtime/sessions",
			"bootstrap":        "ralph:///bootstrap/checklist",
			"runtime_recovery": "ralph:///runtime/recovery",
			"runtime_health":   "ralph:///runtime/health",
			"skills":           "ralph:///catalog/skills",
		},
		"supporting_tools": []string{
			"ralphglasses_server_health",
			"ralphglasses_fleet_runtime",
			"ralphglasses_marathon",
			"ralphglasses_session_launch",
		},
		"fleet_runtime":         s.fleetRuntimeStatus(ctx),
		"marathon_runtime":      s.marathonStatus(),
		"active_session_count":  sessions["active_session_count"],
		"stalled_session_count": sessions["stalled_session_count"],
		"runtime_health": map[string]any{
			"status":                    runtime["status"],
			"loaded_groups":             runtime["loaded_groups"],
			"tool_group_count":          runtime["tool_group_count"],
			"resource_count":            runtime["resource_count"],
			"prompt_count":              runtime["prompt_count"],
			"highest_priority_workflow": nestedString(runtime["adoption_priority_summary"], "highest_priority_workflow"),
		},
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
			"ralph:///runtime/operator",
			"ralph:///runtime/recovery",
			"ralph:///runtime/sessions",
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

func readJSONFile(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return value, nil
}

func nestedString(value any, key string) string {
	m, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	raw, ok := m[key]
	if !ok {
		return ""
	}
	str, _ := raw.(string)
	return str
}

func instrumentResourceHandler(appSrv *Server, name string, next server.ResourceHandlerFunc) server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		result, err := next(ctx, req)
		if err == nil && len(result) > 0 && appSrv != nil && appSrv.DiscoveryRecorder != nil {
			appSrv.DiscoveryRecorder.RecordResource(name, "")
		}
		return result, err
	}
}

func instrumentResourceTemplateHandler(appSrv *Server, name string, next server.ResourceTemplateHandlerFunc) server.ResourceTemplateHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		result, err := next(ctx, req)
		if err == nil && len(result) > 0 && appSrv != nil && appSrv.DiscoveryRecorder != nil {
			appSrv.DiscoveryRecorder.RecordResource(name, req.Params.URI)
		}
		return result, err
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
