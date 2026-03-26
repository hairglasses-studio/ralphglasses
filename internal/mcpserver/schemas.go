package mcpserver

// OutputSchemas maps tool names to their JSON Schema output definitions.
// These schemas describe the structured output format for high-value tools.
// When the MCP SDK supports outputSchema, these can be wired into tool definitions.
var OutputSchemas = map[string]map[string]any{
	"ralphglasses_observation_query": {
		"type": "object",
		"properties": map[string]any{
			"observations": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"loop_id":          map[string]any{"type": "string"},
						"iteration":        map[string]any{"type": "integer"},
						"status":           map[string]any{"type": "string"},
						"planner_provider": map[string]any{"type": "string"},
						"worker_provider":  map[string]any{"type": "string"},
						"cost_usd":         map[string]any{"type": "number"},
						"duration_seconds":  map[string]any{"type": "number"},
						"files_changed":    map[string]any{"type": "integer"},
						"stall_count":      map[string]any{"type": "integer"},
						"verify_passed":    map[string]any{"type": "boolean"},
						"task_type":        map[string]any{"type": "string"},
						"task_title":       map[string]any{"type": "string"},
						"confidence":       map[string]any{"type": "number"},
						"acceptance_path":  map[string]any{"type": "string"},
					},
				},
			},
			"total":    map[string]any{"type": "integer"},
			"filtered": map[string]any{"type": "integer"},
		},
	},

	"ralphglasses_observation_summary": {
		"type": "object",
		"properties": map[string]any{
			"window_hours":      map[string]any{"type": "number"},
			"total_iterations":  map[string]any{"type": "integer"},
			"completion_rate":   map[string]any{"type": "number"},
			"avg_cost_per_iter": map[string]any{"type": "number"},
			"cost_trend": map[string]any{
				"type": "string",
				"enum": []any{"decreasing", "stable", "increasing"},
			},
			"efficiency_score": map[string]any{"type": "number"},
			"velocity":         map[string]any{"type": "number"},
			"cost_by_provider": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{"type": "number"},
			},
			"total_stalls":       map[string]any{"type": "integer"},
			"total_files_changed": map[string]any{"type": "integer"},
			"acceptance_counts": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{"type": "integer"},
			},
		},
	},

	"ralphglasses_loop_benchmark": {
		"type": "object",
		"properties": map[string]any{
			"session_id": map[string]any{"type": "string"},
			"loop_count": map[string]any{"type": "integer"},
			"timing": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mean_seconds": map[string]any{"type": "number"},
					"p50_seconds":  map[string]any{"type": "number"},
					"p95_seconds":  map[string]any{"type": "number"},
					"min_seconds":  map[string]any{"type": "number"},
					"max_seconds":  map[string]any{"type": "number"},
				},
			},
			"tokens": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"total_input":  map[string]any{"type": "integer"},
					"total_output": map[string]any{"type": "integer"},
					"total":        map[string]any{"type": "integer"},
				},
			},
			"cost_estimate":    map[string]any{"type": "number"},
			"cost_per_task":    map[string]any{"type": "number"},
			"tasks_completed":  map[string]any{"type": "integer"},
			"tasks_total":      map[string]any{"type": "integer"},
			"spin_events":      map[string]any{"type": "integer"},
			"model":            map[string]any{"type": "string"},
		},
	},

	"ralphglasses_fleet_status": {
		"type": "object",
		"properties": map[string]any{
			"workers": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"worker_id":       map[string]any{"type": "string"},
						"status":          map[string]any{"type": "string"},
						"provider":        map[string]any{"type": "string"},
						"current_task":    map[string]any{"type": "string"},
						"sessions_active": map[string]any{"type": "integer"},
						"capacity":        map[string]any{"type": "integer"},
						"cost_usd":        map[string]any{"type": "number"},
						"uptime_seconds":  map[string]any{"type": "number"},
					},
				},
			},
			"total_sessions": map[string]any{"type": "integer"},
			"active_loops":   map[string]any{"type": "integer"},
			"cost_summary": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"total_spend_usd":    map[string]any{"type": "number"},
					"budget_usd":         map[string]any{"type": "number"},
					"burn_rate_per_hour": map[string]any{"type": "number"},
				},
			},
			"health": map[string]any{
				"type": "string",
				"enum": []any{"healthy", "degraded", "critical"},
			},
		},
	},

	"ralphglasses_cost_estimate": {
		"type": "object",
		"properties": map[string]any{
			"estimate": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"low_usd":  map[string]any{"type": "number"},
					"mid_usd":  map[string]any{"type": "number"},
					"high_usd": map[string]any{"type": "number"},
				},
			},
			"breakdown": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"provider":  map[string]any{"type": "string"},
						"model":     map[string]any{"type": "string"},
						"cost_usd":  map[string]any{"type": "number"},
						"tokens":    map[string]any{"type": "integer"},
					},
				},
			},
			"burn_rate_per_hour": map[string]any{"type": "number"},
			"trend_direction": map[string]any{
				"type": "string",
				"enum": []any{"increasing", "decreasing", "stable"},
			},
			"exhaustion_eta":      map[string]any{"type": "string", "format": "date-time"},
			"historical_avg_usd": map[string]any{"type": "number"},
			"sample_count":       map[string]any{"type": "integer"},
		},
	},

	"ralphglasses_coverage_report": {
		"type": "object",
		"properties": map[string]any{
			"overall_coverage": map[string]any{"type": "number"},
			"threshold":        map[string]any{"type": "number"},
			"meets_threshold":  map[string]any{"type": "boolean"},
			"packages": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"package":         map[string]any{"type": "string"},
						"coverage":        map[string]any{"type": "number"},
						"threshold":       map[string]any{"type": "number"},
						"meets_threshold": map[string]any{"type": "boolean"},
						"functions_hit":   map[string]any{"type": "integer"},
						"functions_total": map[string]any{"type": "integer"},
					},
				},
			},
			"gate_verdict": map[string]any{
				"type": "string",
				"enum": []any{"pass", "warn", "fail", "skip"},
			},
		},
	},
}

// SchemaForTool returns the output schema for a tool, or nil if not defined.
func SchemaForTool(toolName string) map[string]any {
	return OutputSchemas[toolName]
}
