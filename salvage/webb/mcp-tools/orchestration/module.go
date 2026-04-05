// Package orchestration provides multi-agent workflow orchestration tools (v16.5)
package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/clients"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// Module implements the ToolModule interface for orchestration tools
type Module struct{}

// Name returns the module name
func (m *Module) Name() string {
	return "orchestration"
}

// Description returns a brief description of the module
func (m *Module) Description() string {
	return "Multi-agent workflow orchestration, smart routing, and tool chaining"
}

// Tools returns all tool definitions in this module
func (m *Module) Tools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		// Workflow Define
		{
			Tool: mcp.NewTool("webb_workflow_define",
				mcp.WithDescription("Define a multi-step workflow with tool sequences, conditions, and handoffs. Workflows can be saved and reused."),
				mcp.WithString("name",
					mcp.Required(),
					mcp.Description("Workflow name"),
				),
				mcp.WithString("description",
					mcp.Description("Workflow description"),
				),
				mcp.WithString("steps_json",
					mcp.Required(),
					mcp.Description("JSON array of workflow steps: [{\"id\": \"step1\", \"name\": \"Check Health\", \"tool\": \"webb_cluster_health_full\", \"params\": {\"context\": \"headspace\"}}]"),
				),
				mcp.WithString("on_error",
					mcp.Description("Error handling: abort, continue, retry (default: continue)"),
				),
				mcp.WithBoolean("save",
					mcp.Description("Save workflow to vault (default: false)"),
				),
			),
			Handler:     handleWorkflowDefine,
			Category:    "orchestration",
			Subcategory: "workflows",
			Tags:        []string{"workflow", "orchestration", "automation", "multi-step"},
			UseCases:    []string{"define workflow", "create automation", "multi-step task"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     true,
		},
		// Workflow Execute
		{
			Tool: mcp.NewTool("webb_workflow_execute",
				mcp.WithDescription("Execute a defined workflow or template with automatic step execution and handoffs."),
				mcp.WithString("workflow_id",
					mcp.Required(),
					mcp.Description("Workflow ID or template name (e.g., 'investigation', 'deployment-preflight', 'standup')"),
				),
				mcp.WithString("inputs_json",
					mcp.Description("JSON object of input variables: {\"cluster\": \"headspace\", \"customer\": \"acme\"}"),
				),
				mcp.WithBoolean("async",
					mcp.Description("Run asynchronously and return run ID (default: false)"),
				),
			),
			Handler:     handleWorkflowExecute,
			Category:    "orchestration",
			Subcategory: "workflows",
			Tags:        []string{"workflow", "execute", "orchestration", "automation"},
			UseCases:    []string{"run workflow", "execute automation", "multi-step execution"},
			Complexity:  tools.ComplexityComplex,
			IsWrite:     true,
		},
		// Workflow List
		{
			Tool: mcp.NewTool("webb_workflow_list",
				mcp.WithDescription("List available workflow templates and custom workflows with their steps and variables."),
				mcp.WithString("category",
					mcp.Description("Filter by category: investigation, deployment, remediation, operations"),
				),
			),
			Handler:     handleWorkflowList,
			Category:    "orchestration",
			Subcategory: "workflows",
			Tags:        []string{"workflow", "templates", "list"},
			UseCases:    []string{"list workflows", "view templates", "available automations"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Workflow Status
		{
			Tool: mcp.NewTool("webb_workflow_status",
				mcp.WithDescription("Get the status of a running or completed workflow execution."),
				mcp.WithString("run_id",
					mcp.Required(),
					mcp.Description("Workflow run ID"),
				),
			),
			Handler:     handleWorkflowStatus,
			Category:    "orchestration",
			Subcategory: "workflows",
			Tags:        []string{"workflow", "status", "execution"},
			UseCases:    []string{"check workflow status", "get execution result"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Router Suggest
		{
			Tool: mcp.NewTool("webb_router_suggest",
				mcp.WithDescription("Suggest optimal tool based on query intent using AI. Analyzes query and recommends the best tool with extracted parameters."),
				mcp.WithString("query",
					mcp.Required(),
					mcp.Description("Natural language query (e.g., 'what's the health of headspace cluster?')"),
				),
				mcp.WithBoolean("include_workflow",
					mcp.Description("Include workflow suggestions if applicable (default: true)"),
				),
			),
			Handler:     handleRouterSuggest,
			Category:    "orchestration",
			Subcategory: "routing",
			Tags:        []string{"router", "suggest", "intent", "ai"},
			UseCases:    []string{"find right tool", "query routing", "tool suggestion"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Chain Builder
		{
			Tool: mcp.NewTool("webb_chain_builder",
				mcp.WithDescription("Build workflow chains interactively. Start with a goal and get suggested tool sequence."),
				mcp.WithString("goal",
					mcp.Required(),
					mcp.Description("What you want to accomplish (e.g., 'investigate slow API responses for headspace')"),
				),
				mcp.WithString("cluster",
					mcp.Description("Target cluster context"),
				),
				mcp.WithString("customer",
					mcp.Description("Customer name if applicable"),
				),
			),
			Handler:     handleChainBuilder,
			Category:    "orchestration",
			Subcategory: "workflows",
			Tags:        []string{"chain", "builder", "workflow", "suggest"},
			UseCases:    []string{"build workflow", "suggest chain", "create automation"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     false,
		},
	}
}

// Register module on init
func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// =============================================================================
// Handler Implementations
// =============================================================================

func handleWorkflowDefine(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("name is required")), nil
	}

	stepsJSON, err := req.RequireString("steps_json")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("steps_json is required")), nil
	}

	description := req.GetString("description", "")
	onError := req.GetString("on_error", "continue")
	save := req.GetBool("save", false)

	// Parse steps
	var steps []clients.OrchStep
	if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
		return tools.ErrorResult(fmt.Errorf("invalid steps_json: %v", err)), nil
	}

	// Create workflow
	workflow := &clients.OrchWorkflow{
		Name:        name,
		Description: description,
		Steps:       steps,
		OnError:     onError,
	}

	orchestrator := clients.GetOrchestratorClient()
	if err := orchestrator.DefineWorkflow(ctx, workflow); err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to define workflow: %v", err)), nil
	}

	// Save if requested
	if save {
		if err := orchestrator.SaveWorkflow(workflow); err != nil {
			tools.LogWriteOperation("webb_workflow_define", map[string]string{
				"workflow_id": workflow.ID, "name": name,
			}, false, err.Error())
		} else {
			tools.LogWriteOperation("webb_workflow_define", map[string]string{
				"workflow_id": workflow.ID, "name": name, "saved": "true",
			}, true, "")
		}
	}

	var sb strings.Builder
	sb.WriteString("# Workflow Defined\n\n")
	sb.WriteString(fmt.Sprintf("**ID:** `%s`\n", workflow.ID))
	sb.WriteString(fmt.Sprintf("**Name:** %s\n", workflow.Name))
	sb.WriteString(fmt.Sprintf("**Steps:** %d\n", len(workflow.Steps)))
	sb.WriteString(fmt.Sprintf("**On Error:** %s\n", workflow.OnError))

	if save {
		sb.WriteString("\n_Workflow saved to vault_\n")
	}

	sb.WriteString("\n## Steps\n\n")
	for i, step := range workflow.Steps {
		parallel := ""
		if step.Parallel {
			parallel = " [parallel]"
		}
		sb.WriteString(fmt.Sprintf("%d. **%s** → `%s`%s\n", i+1, step.Name, step.Tool, parallel))
	}

	return tools.TextResult(sb.String()), nil
}

func handleWorkflowExecute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workflowID, err := req.RequireString("workflow_id")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("workflow_id is required")), nil
	}

	inputsJSON := req.GetString("inputs_json", "{}")
	async := req.GetBool("async", false)

	// Parse inputs
	var inputs map[string]any
	if err := json.Unmarshal([]byte(inputsJSON), &inputs); err != nil {
		return tools.ErrorResult(fmt.Errorf("invalid inputs_json: %v", err)), nil
	}

	orchestrator := clients.GetOrchestratorClient()

	// Execute workflow
	run, err := orchestrator.ExecuteWorkflow(ctx, workflowID, inputs)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to execute workflow: %v", err)), nil
	}

	tools.LogWriteOperation("webb_workflow_execute", map[string]string{
		"workflow_id": workflowID, "run_id": run.ID,
	}, true, "")

	if async {
		return tools.TextResult(fmt.Sprintf("# Workflow Started\n\n**Run ID:** `%s`\n\nUse `webb_workflow_status` to check progress.", run.ID)), nil
	}

	// Wait for completion (synchronous mode)
	// In real implementation, this would poll until complete
	// For now, return the run status
	return tools.TextResult(clients.FormatWorkflowRun(run)), nil
}

func handleWorkflowList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	category := req.GetString("category", "")

	orchestrator := clients.GetOrchestratorClient()
	templates := orchestrator.ListTemplates()

	// Filter by category if specified
	if category != "" {
		var filtered []*clients.OrchTemplate
		for _, t := range templates {
			if t.Category == category {
				filtered = append(filtered, t)
			}
		}
		templates = filtered
	}

	return tools.TextResult(clients.FormatWorkflowTemplates(templates)), nil
}

func handleWorkflowStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	runID, err := req.RequireString("run_id")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("run_id is required")), nil
	}

	orchestrator := clients.GetOrchestratorClient()
	run, ok := orchestrator.GetWorkflowRun(runID)
	if !ok {
		return tools.ErrorResult(fmt.Errorf("run not found: %s", runID)), nil
	}

	return tools.TextResult(clients.FormatWorkflowRun(run)), nil
}

func handleRouterSuggest(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("query is required")), nil
	}

	includeWorkflow := req.GetBool("include_workflow", true)

	// Use existing SmartRouter for routing
	router := clients.NewSmartRouter()
	result, err := router.Route(ctx, clients.RouteParams{
		Question: query,
	})

	if err != nil {
		return tools.ErrorResult(fmt.Errorf("routing failed: %v", err)), nil
	}

	var sb strings.Builder
	sb.WriteString("# Routing Suggestion\n\n")
	sb.WriteString(fmt.Sprintf("**Query:** %s\n", query))
	sb.WriteString(fmt.Sprintf("**Routed To:** %s\n", result.RoutedTo))
	sb.WriteString(fmt.Sprintf("**Tool:** `%s`\n\n", result.ToolName))

	if result.ExtractedContext != "" {
		sb.WriteString(fmt.Sprintf("**Extracted Context:** %s\n", result.ExtractedContext))
	}
	if result.ExtractedCustomer != "" {
		sb.WriteString(fmt.Sprintf("**Extracted Customer:** %s\n", result.ExtractedCustomer))
	}

	// Suggest workflow if applicable
	if includeWorkflow {
		orchestrator := clients.GetOrchestratorClient()
		queryLower := strings.ToLower(query)

		var suggestedWorkflow string
		if strings.Contains(queryLower, "investigate") || strings.Contains(queryLower, "problem") {
			suggestedWorkflow = "incident-response"
		} else if strings.Contains(queryLower, "deploy") || strings.Contains(queryLower, "release") {
			suggestedWorkflow = "deployment-preflight"
		} else if strings.Contains(queryLower, "morning") || strings.Contains(queryLower, "standup") {
			suggestedWorkflow = "standup"
		} else if strings.Contains(queryLower, "health") || strings.Contains(queryLower, "cluster") {
			suggestedWorkflow = "investigation"
		}

		if suggestedWorkflow != "" {
			if tmpl, ok := orchestrator.GetTemplate(suggestedWorkflow); ok {
				sb.WriteString(fmt.Sprintf("\n## Suggested Workflow\n\n"))
				sb.WriteString(fmt.Sprintf("**Template:** `%s` - %s\n\n", tmpl.ID, tmpl.Name))
				sb.WriteString(fmt.Sprintf("_Run with:_ `webb_workflow_execute(workflow_id=\"%s\")`\n", tmpl.ID))
			}
		}
	}

	return tools.TextResult(sb.String()), nil
}

func handleChainBuilder(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	goal, err := req.RequireString("goal")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("goal is required")), nil
	}

	cluster := req.GetString("cluster", "")
	customer := req.GetString("customer", "")

	goalLower := strings.ToLower(goal)

	var sb strings.Builder
	sb.WriteString("# Suggested Tool Chain\n\n")
	sb.WriteString(fmt.Sprintf("**Goal:** %s\n\n", goal))

	// Analyze goal and suggest chain
	var chain []struct {
		Step int
		Tool string
		Desc string
	}

	// Investigation chain
	if strings.Contains(goalLower, "investigate") || strings.Contains(goalLower, "problem") ||
		strings.Contains(goalLower, "issue") || strings.Contains(goalLower, "slow") ||
		strings.Contains(goalLower, "error") || strings.Contains(goalLower, "failing") {

		chain = append(chain,
			struct{ Step int; Tool, Desc string }{1, "webb_cluster_health_full", "Check cluster health for issues"},
			struct{ Step int; Tool, Desc string }{2, "webb_grafana_alerts", "Check for active alerts"},
			struct{ Step int; Tool, Desc string }{3, "webb_k8s_events", "Review recent Kubernetes events"},
			struct{ Step int; Tool, Desc string }{4, "webb_k8s_logs", "Check pod logs for errors"},
			struct{ Step int; Tool, Desc string }{5, "webb_similar_incidents", "Find similar past incidents"},
		)
	} else if strings.Contains(goalLower, "deploy") || strings.Contains(goalLower, "release") {
		chain = append(chain,
			struct{ Step int; Tool, Desc string }{1, "webb_preflight_full", "Run deployment preflight checks"},
			struct{ Step int; Tool, Desc string }{2, "webb_version_audit", "Audit current versions"},
			struct{ Step int; Tool, Desc string }{3, "webb_queue_health_full", "Check queue health before deploy"},
			struct{ Step int; Tool, Desc string }{4, "webb_k8s_rollout_status", "Monitor rollout progress"},
		)
	} else if strings.Contains(goalLower, "customer") || strings.Contains(goalLower, "account") {
		chain = append(chain,
			struct{ Step int; Tool, Desc string }{1, "webb_customer_snapshot", "Get customer overview"},
			struct{ Step int; Tool, Desc string }{2, "webb_pylon_search", "Search customer tickets"},
			struct{ Step int; Tool, Desc string }{3, "webb_cluster_health_full", "Check customer cluster health"},
		)
	} else if strings.Contains(goalLower, "alert") || strings.Contains(goalLower, "oncall") {
		chain = append(chain,
			struct{ Step int; Tool, Desc string }{1, "webb_grafana_alerts", "Get active alerts"},
			struct{ Step int; Tool, Desc string }{2, "webb_oncall_triage", "Triage and prioritize"},
			struct{ Step int; Tool, Desc string }{3, "webb_escalation_preflight", "Check escalation readiness"},
		)
	} else {
		// Generic investigation
		chain = append(chain,
			struct{ Step int; Tool, Desc string }{1, "webb_ask", "Ask about the goal"},
			struct{ Step int; Tool, Desc string }{2, "webb_ticket_summary", "Check related tickets"},
			struct{ Step int; Tool, Desc string }{3, "webb_recent_activity", "Review recent activity"},
		)
	}

	sb.WriteString("## Recommended Chain\n\n")
	for _, step := range chain {
		sb.WriteString(fmt.Sprintf("%d. `%s` - %s\n", step.Step, step.Tool, step.Desc))
	}

	// Generate workflow JSON
	sb.WriteString("\n## Workflow Definition\n\n")
	sb.WriteString("```json\n")

	steps := make([]map[string]any, len(chain))
	for i, step := range chain {
		s := map[string]any{
			"id":   fmt.Sprintf("step%d", step.Step),
			"name": step.Desc,
			"tool": step.Tool,
		}
		if i > 0 {
			s["depends_on"] = []string{fmt.Sprintf("step%d", step.Step-1)}
		}
		if cluster != "" {
			s["params"] = map[string]any{"context": cluster}
		} else if customer != "" {
			s["params"] = map[string]any{"customer": customer}
		}
		steps[i] = s
	}

	stepsJSON, _ := json.MarshalIndent(steps, "", "  ")
	sb.WriteString(string(stepsJSON))
	sb.WriteString("\n```\n")

	sb.WriteString("\n## Quick Start\n\n")
	sb.WriteString("To create and run this workflow:\n")
	sb.WriteString("```\nwebb_workflow_define(name=\"custom-chain\", steps_json='...')\nwebb_workflow_execute(workflow_id=\"custom-chain\")\n```\n")

	return tools.TextResult(sb.String()), nil
}
