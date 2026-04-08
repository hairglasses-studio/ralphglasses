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
						"duration_seconds": map[string]any{"type": "number"},
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
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "number"},
			},
			"total_stalls":        map[string]any{"type": "integer"},
			"total_files_changed": map[string]any{"type": "integer"},
			"acceptance_counts": map[string]any{
				"type":                 "object",
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
			"cost_estimate":   map[string]any{"type": "number"},
			"cost_per_task":   map[string]any{"type": "number"},
			"tasks_completed": map[string]any{"type": "integer"},
			"tasks_total":     map[string]any{"type": "integer"},
			"spin_events":     map[string]any{"type": "integer"},
			"model":           map[string]any{"type": "string"},
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
						"provider": map[string]any{"type": "string"},
						"model":    map[string]any{"type": "string"},
						"cost_usd": map[string]any{"type": "number"},
						"tokens":   map[string]any{"type": "integer"},
					},
				},
			},
			"burn_rate_per_hour": map[string]any{"type": "number"},
			"trend_direction": map[string]any{
				"type": "string",
				"enum": []any{"increasing", "decreasing", "stable"},
			},
			"exhaustion_eta":     map[string]any{"type": "string", "format": "date-time"},
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

	"ralphglasses_session_status": {
		"type": "object",
		"properties": map[string]any{
			"id":                     map[string]any{"type": "string"},
			"provider":               map[string]any{"type": "string"},
			"provider_session_id":    map[string]any{"type": "string"},
			"repo":                   map[string]any{"type": "string"},
			"repo_path":              map[string]any{"type": "string"},
			"status":                 map[string]any{"type": "string"},
			"prompt":                 map[string]any{"type": "string"},
			"model":                  map[string]any{"type": "string"},
			"agent":                  map[string]any{"type": "string"},
			"team":                   map[string]any{"type": "string"},
			"budget_usd":             map[string]any{"type": "number"},
			"spent_usd":              map[string]any{"type": "number"},
			"turns":                  map[string]any{"type": "integer"},
			"max_turns":              map[string]any{"type": "integer"},
			"launched_at":            map[string]any{"type": "string", "format": "date-time"},
			"last_activity":          map[string]any{"type": "string", "format": "date-time"},
			"ended_at":               map[string]any{"type": "string", "format": "date-time"},
			"exit_reason":            map[string]any{"type": "string"},
			"last_output":            map[string]any{"type": "string"},
			"error":                  map[string]any{"type": "string"},
			"cache_read_tokens":      map[string]any{"type": "integer"},
			"cache_write_tokens":     map[string]any{"type": "integer"},
			"cache_read_write_ratio": map[string]any{"type": "number"},
			"cache_anomaly":          map[string]any{"type": "string"},
		},
	},

	"ralphglasses_session_list": {
		"type": "object",
		"properties": map[string]any{
			"sessions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":        map[string]any{"type": "string"},
						"provider":  map[string]any{"type": "string"},
						"repo":      map[string]any{"type": "string"},
						"status":    map[string]any{"type": "string"},
						"model":     map[string]any{"type": "string"},
						"spent_usd": map[string]any{"type": "number"},
						"turns":     map[string]any{"type": "integer"},
						"agent":     map[string]any{"type": "string"},
						"team":      map[string]any{"type": "string"},
					},
				},
			},
		},
	},

	"ralphglasses_session_errors": {
		"type": "object",
		"properties": map[string]any{
			"total_errors": map[string]any{"type": "integer"},
			"by_type": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "integer"},
			},
			"by_severity": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "integer"},
			},
			"errors": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"session_id": map[string]any{"type": "string"},
						"repo":       map[string]any{"type": "string"},
						"provider":   map[string]any{"type": "string"},
						"severity": map[string]any{
							"type": "string",
							"enum": []any{"critical", "warning", "info"},
						},
						"type":      map[string]any{"type": "string"},
						"message":   map[string]any{"type": "string"},
						"timestamp": map[string]any{"type": "string", "format": "date-time"},
					},
				},
			},
			"sessions_with_errors": map[string]any{"type": "integer"},
			"healthy_sessions":     map[string]any{"type": "integer"},
		},
	},

	"ralphglasses_session_diff": {
		"type": "object",
		"properties": map[string]any{
			"session_id": map[string]any{"type": "string"},
			"repo":       map[string]any{"type": "string"},
			"window": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"started":  map[string]any{"type": "string", "format": "date-time"},
					"ended":    map[string]any{"type": "string", "format": "date-time"},
					"duration": map[string]any{"type": "string"},
				},
			},
			"commits": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"hash":    map[string]any{"type": "string"},
						"message": map[string]any{"type": "string"},
						"author":  map[string]any{"type": "string"},
						"date":    map[string]any{"type": "string"},
					},
				},
			},
			"stat":      map[string]any{"type": "string"},
			"diff":      map[string]any{"type": "string"},
			"truncated": map[string]any{"type": "boolean"},
		},
	},

	"ralphglasses_loop_status": {
		"type": "object",
		"properties": map[string]any{
			"id":         map[string]any{"type": "string"},
			"repo":       map[string]any{"type": "string"},
			"repo_path":  map[string]any{"type": "string"},
			"status":     map[string]any{"type": "string"},
			"last_error": map[string]any{"type": "string"},
			"profile": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"planner_model":   map[string]any{"type": "string"},
					"worker_model":    map[string]any{"type": "string"},
					"verifier_model":  map[string]any{"type": "string"},
					"worktree_policy": map[string]any{"type": "string"},
					"retry_limit":     map[string]any{"type": "integer"},
				},
			},
			"iterations": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"number":     map[string]any{"type": "integer"},
						"status":     map[string]any{"type": "string"},
						"task":       map[string]any{"type": "object", "additionalProperties": true},
						"error":      map[string]any{"type": "string"},
						"started_at": map[string]any{"type": "string", "format": "date-time"},
						"ended_at":   map[string]any{"type": "string", "format": "date-time"},
					},
				},
			},
			"created_at": map[string]any{"type": "string", "format": "date-time"},
			"updated_at": map[string]any{"type": "string", "format": "date-time"},
		},
	},

	"ralphglasses_loop_gates": {
		"type": "object",
		"properties": map[string]any{
			"ts":           map[string]any{"type": "string", "format": "date-time"},
			"sample_count": map[string]any{"type": "integer"},
			"overall": map[string]any{
				"type": "string",
				"enum": []any{"pass", "warn", "fail", "skip"},
			},
			"results": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"metric": map[string]any{"type": "string"},
						"verdict": map[string]any{
							"type": "string",
							"enum": []any{"pass", "warn", "fail", "skip"},
						},
						"baseline":  map[string]any{"type": "number"},
						"current":   map[string]any{"type": "number"},
						"delta_pct": map[string]any{"type": "number"},
					},
				},
			},
		},
	},

	"ralphglasses_fleet_submit": {
		"type": "object",
		"properties": map[string]any{
			"work_item_id": map[string]any{"type": "string"},
			"status": map[string]any{
				"type": "string",
				"enum": []any{"pending"},
			},
			"queue": map[string]any{
				"type": "string",
				"enum": []any{"local_coordinator", "remote_coordinator"},
			},
		},
	},

	"ralphglasses_fleet_workers": {
		"type": "object",
		"properties": map[string]any{
			"workers": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":       map[string]any{"type": "string"},
						"status":   map[string]any{"type": "string"},
						"provider": map[string]any{"type": "string"},
						"capacity": map[string]any{"type": "integer"},
					},
				},
			},
			"total":       map[string]any{"type": "integer"},
			"active_work": map[string]any{"type": "integer"},
		},
	},

	"ralphglasses_fleet_analytics": {
		"type": "object",
		"properties": map[string]any{
			"providers": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"sessions":          map[string]any{"type": "integer"},
						"running":           map[string]any{"type": "integer"},
						"total_spend_usd":   map[string]any{"type": "number"},
						"avg_cost_per_turn": map[string]any{"type": "number"},
						"total_turns":       map[string]any{"type": "integer"},
					},
				},
			},
			"repos": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "number"},
			},
			"total_sessions": map[string]any{"type": "integer"},
		},
	},

	"ralphglasses_merge_verify": {
		"type": "object",
		"properties": map[string]any{
			"repo": map[string]any{"type": "string"},
			"overall": map[string]any{
				"type": "string",
				"enum": []any{"pass", "fail"},
			},
			"steps": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
						"status": map[string]any{
							"type": "string",
							"enum": []any{"pass", "fail"},
						},
						"elapsed_seconds": map[string]any{"type": "number"},
						"output":          map[string]any{"type": "string"},
						"coverage":        map[string]any{"type": "number"},
					},
				},
			},
			"failed_at":             map[string]any{"type": "string"},
			"total_elapsed_seconds": map[string]any{"type": "number"},
		},
	},

	"ralphglasses_repo_health": {
		"type": "object",
		"properties": map[string]any{
			"repo":             map[string]any{"type": "string"},
			"health_score":     map[string]any{"type": "integer"},
			"circuit_breaker":  map[string]any{"type": "string"},
			"active_sessions":  map[string]any{"type": "integer"},
			"errored_sessions": map[string]any{"type": "integer"},
			"total_spend_usd":  map[string]any{"type": "number"},
			"loop_running":     map[string]any{"type": "boolean"},
			"issues": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"claudemd_findings": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"severity": map[string]any{"type": "string"},
						"message":  map[string]any{"type": "string"},
					},
				},
			},
		},
	},

	"ralphglasses_anomaly_detect": {
		"type": "object",
		"properties": map[string]any{
			"repo":         map[string]any{"type": "string"},
			"metric":       map[string]any{"type": "string"},
			"hours":        map[string]any{"type": "number"},
			"observations": map[string]any{"type": "integer"},
			"anomalies": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"index":     map[string]any{"type": "integer"},
						"timestamp": map[string]any{"type": "string", "format": "date-time"},
						"value":     map[string]any{"type": "number"},
						"expected":  map[string]any{"type": "number"},
						"z_score":   map[string]any{"type": "number"},
						"direction": map[string]any{
							"type": "string",
							"enum": []any{"high", "low"},
						},
					},
				},
			},
			"count":   map[string]any{"type": "integer"},
			"message": map[string]any{"type": "string"},
		},
	},

	"ralphglasses_session_tail": {
		"type": "object",
		"properties": map[string]any{
			"session_id": map[string]any{"type": "string"},
			"status":     map[string]any{"type": "string"},
			"output": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"lines_returned": map[string]any{"type": "integer"},
			"next_cursor":    map[string]any{"type": "string"},
			"is_active":      map[string]any{"type": "boolean"},
			"idle_seconds":   map[string]any{"type": "integer"},
		},
	},

	"ralphglasses_session_output": {
		"type": "object",
		"properties": map[string]any{
			"session_id": map[string]any{"type": "string"},
			"lines":      map[string]any{"type": "integer"},
			"output": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
	},
}

// SchemaForTool returns the output schema for a tool, or nil if not defined.
func SchemaForTool(toolName string) map[string]any {
	return OutputSchemas[toolName]
}
