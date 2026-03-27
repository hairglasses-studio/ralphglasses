package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Basic sanity checks (kept from original)
// ---------------------------------------------------------------------------

func TestOutputSchemasNotEmpty(t *testing.T) {
	if len(OutputSchemas) == 0 {
		t.Fatal("OutputSchemas should have entries")
	}
}

func TestOutputSchemasValidJSON(t *testing.T) {
	for name, schema := range OutputSchemas {
		data, err := json.Marshal(schema)
		if err != nil {
			t.Errorf("schema %q failed to marshal: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("schema %q marshalled to empty bytes", name)
		}
	}
}

func TestOutputSchemaKeys(t *testing.T) {
	for name := range OutputSchemas {
		if !strings.HasPrefix(name, "ralphglasses_") {
			t.Errorf("schema key %q does not start with 'ralphglasses_'", name)
		}
	}
}

func TestOutputSchemaHasType(t *testing.T) {
	for name, schema := range OutputSchemas {
		typ, ok := schema["type"]
		if !ok {
			t.Errorf("schema %q missing root 'type' field", name)
			continue
		}
		if typ != "object" {
			t.Errorf("schema %q root type = %v, want 'object'", name, typ)
		}
	}
}

// ---------------------------------------------------------------------------
// SchemaForTool — hit and miss paths
// ---------------------------------------------------------------------------

func TestSchemaForToolExists(t *testing.T) {
	// Exhaustively test every key in OutputSchemas.
	for name := range OutputSchemas {
		schema := SchemaForTool(name)
		if schema == nil {
			t.Errorf("SchemaForTool(%q) returned nil, want non-nil", name)
		}
	}
}

func TestSchemaForToolMissing(t *testing.T) {
	missing := []string{
		"nonexistent_tool",
		"",
		"ralphglasses_",
		"ralphglasses_does_not_exist",
	}
	for _, name := range missing {
		schema := SchemaForTool(name)
		if schema != nil {
			t.Errorf("SchemaForTool(%q) = non-nil, want nil", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Schema count — ensure we do not silently lose entries
// ---------------------------------------------------------------------------

func TestOutputSchemasExpectedCount(t *testing.T) {
	// There are 20 schemas defined in schemas.go as of this writing.
	// If someone adds/removes one, this test will flag it for review.
	const minExpected = 20
	if got := len(OutputSchemas); got < minExpected {
		t.Errorf("OutputSchemas has %d entries, want at least %d", got, minExpected)
	}
}

// ---------------------------------------------------------------------------
// Every schema must have a "properties" key with at least one property
// ---------------------------------------------------------------------------

func TestOutputSchemasHaveProperties(t *testing.T) {
	for name, schema := range OutputSchemas {
		props, ok := schema["properties"]
		if !ok {
			t.Errorf("schema %q missing 'properties' field", name)
			continue
		}
		pm, ok := props.(map[string]any)
		if !ok {
			t.Errorf("schema %q 'properties' is not a map[string]any", name)
			continue
		}
		if len(pm) == 0 {
			t.Errorf("schema %q has empty 'properties'", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Property type validation helpers
// ---------------------------------------------------------------------------

// validJSONSchemaTypes lists the types allowed in JSON Schema.
var validJSONSchemaTypes = map[string]bool{
	"string":  true,
	"integer": true,
	"number":  true,
	"boolean": true,
	"object":  true,
	"array":   true,
}

// walkProperties recursively walks all properties in a schema and calls fn
// with the full dot-path and the property definition.
func walkProperties(path string, schema map[string]any, fn func(path string, prop map[string]any)) {
	propsRaw, ok := schema["properties"]
	if !ok {
		return
	}
	props, ok := propsRaw.(map[string]any)
	if !ok {
		return
	}
	for key, valRaw := range props {
		prop, ok := valRaw.(map[string]any)
		if !ok {
			continue
		}
		fullPath := path + "." + key
		fn(fullPath, prop)

		// Recurse into nested objects.
		if prop["type"] == "object" {
			walkProperties(fullPath, prop, fn)
		}
		// Recurse into array items that are objects.
		if prop["type"] == "array" {
			if items, ok := prop["items"].(map[string]any); ok {
				if items["type"] == "object" {
					walkProperties(fullPath+"[]", items, fn)
				}
			}
		}
	}
}

func TestAllPropertyTypesAreValid(t *testing.T) {
	for name, schema := range OutputSchemas {
		walkProperties(name, schema, func(path string, prop map[string]any) {
			typVal, ok := prop["type"]
			if !ok {
				// Some properties may use additionalProperties instead of type at leaf.
				if _, hasAP := prop["additionalProperties"]; hasAP {
					return
				}
				t.Errorf("%s: missing 'type' field", path)
				return
			}
			typStr, ok := typVal.(string)
			if !ok {
				t.Errorf("%s: 'type' is not a string: %T", path, typVal)
				return
			}
			if !validJSONSchemaTypes[typStr] {
				t.Errorf("%s: invalid type %q", path, typStr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Enum validation: every enum must be a non-empty []any of strings
// ---------------------------------------------------------------------------

func TestEnumValuesAreStrings(t *testing.T) {
	for name, schema := range OutputSchemas {
		walkProperties(name, schema, func(path string, prop map[string]any) {
			enumRaw, ok := prop["enum"]
			if !ok {
				return
			}
			enumSlice, ok := enumRaw.([]any)
			if !ok {
				t.Errorf("%s: 'enum' is not []any", path)
				return
			}
			if len(enumSlice) == 0 {
				t.Errorf("%s: 'enum' is empty", path)
				return
			}
			for i, v := range enumSlice {
				if _, ok := v.(string); !ok {
					t.Errorf("%s: enum[%d] is %T, want string", path, i, v)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Array items must define "type"
// ---------------------------------------------------------------------------

func TestArrayItemsHaveType(t *testing.T) {
	for name, schema := range OutputSchemas {
		walkProperties(name, schema, func(path string, prop map[string]any) {
			if prop["type"] != "array" {
				return
			}
			items, ok := prop["items"].(map[string]any)
			if !ok {
				t.Errorf("%s: array property missing 'items'", path)
				return
			}
			if _, ok := items["type"]; !ok {
				t.Errorf("%s: array items missing 'type'", path)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Nested object properties must also have "properties" or "additionalProperties"
// ---------------------------------------------------------------------------

func TestNestedObjectsHavePropertiesOrAdditional(t *testing.T) {
	for name, schema := range OutputSchemas {
		walkProperties(name, schema, func(path string, prop map[string]any) {
			if prop["type"] != "object" {
				return
			}
			_, hasProps := prop["properties"]
			_, hasAdditional := prop["additionalProperties"]
			if !hasProps && !hasAdditional {
				t.Errorf("%s: object property has neither 'properties' nor 'additionalProperties'", path)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Table-driven tests for individual schema structures
// ---------------------------------------------------------------------------

// schemaPropertyCheck describes expected properties for a single schema.
type schemaPropertyCheck struct {
	name           string
	wantProperties []string // top-level property names that must exist
	wantEnums      map[string][]string // path -> expected enum values (relative to root)
}

func TestIndividualSchemaProperties(t *testing.T) {
	checks := []schemaPropertyCheck{
		{
			name:           "ralphglasses_observation_query",
			wantProperties: []string{"observations", "total", "filtered"},
		},
		{
			name:           "ralphglasses_observation_summary",
			wantProperties: []string{"window_hours", "total_iterations", "completion_rate", "avg_cost_per_iter", "cost_trend", "efficiency_score", "velocity", "cost_by_provider", "total_stalls", "total_files_changed", "acceptance_counts"},
			wantEnums: map[string][]string{
				"cost_trend": {"decreasing", "stable", "increasing"},
			},
		},
		{
			name:           "ralphglasses_loop_benchmark",
			wantProperties: []string{"session_id", "loop_count", "timing", "tokens", "cost_estimate", "cost_per_task", "tasks_completed", "tasks_total", "spin_events", "model"},
		},
		{
			name:           "ralphglasses_fleet_status",
			wantProperties: []string{"workers", "total_sessions", "active_loops", "cost_summary", "health"},
			wantEnums: map[string][]string{
				"health": {"healthy", "degraded", "critical"},
			},
		},
		{
			name:           "ralphglasses_cost_estimate",
			wantProperties: []string{"estimate", "breakdown", "burn_rate_per_hour", "trend_direction", "exhaustion_eta", "historical_avg_usd", "sample_count"},
			wantEnums: map[string][]string{
				"trend_direction": {"increasing", "decreasing", "stable"},
			},
		},
		{
			name:           "ralphglasses_coverage_report",
			wantProperties: []string{"overall_coverage", "threshold", "meets_threshold", "packages", "gate_verdict"},
			wantEnums: map[string][]string{
				"gate_verdict": {"pass", "warn", "fail", "skip"},
			},
		},
		{
			name:           "ralphglasses_session_status",
			wantProperties: []string{"id", "provider", "provider_session_id", "repo", "repo_path", "status", "prompt", "model", "agent", "team", "budget_usd", "spent_usd", "turns", "max_turns", "launched_at", "last_activity", "ended_at", "exit_reason", "last_output", "error"},
		},
		{
			name:           "ralphglasses_session_list",
			wantProperties: []string{"sessions"},
		},
		{
			name:           "ralphglasses_session_errors",
			wantProperties: []string{"total_errors", "by_type", "by_severity", "errors", "sessions_with_errors", "healthy_sessions"},
		},
		{
			name:           "ralphglasses_session_diff",
			wantProperties: []string{"session_id", "repo", "window", "commits", "stat", "diff", "truncated"},
		},
		{
			name:           "ralphglasses_loop_status",
			wantProperties: []string{"id", "repo", "repo_path", "status", "last_error", "profile", "iterations", "created_at", "updated_at"},
		},
		{
			name:           "ralphglasses_loop_gates",
			wantProperties: []string{"ts", "sample_count", "overall", "results"},
			wantEnums: map[string][]string{
				"overall": {"pass", "warn", "fail", "skip"},
			},
		},
		{
			name:           "ralphglasses_fleet_submit",
			wantProperties: []string{"work_item_id", "status", "queue"},
			wantEnums: map[string][]string{
				"status": {"pending"},
				"queue":  {"local_coordinator", "remote_coordinator"},
			},
		},
		{
			name:           "ralphglasses_fleet_workers",
			wantProperties: []string{"workers", "total", "active_work"},
		},
		{
			name:           "ralphglasses_fleet_analytics",
			wantProperties: []string{"providers", "repos", "total_sessions"},
		},
		{
			name:           "ralphglasses_merge_verify",
			wantProperties: []string{"repo", "overall", "steps", "failed_at", "total_elapsed_seconds"},
			wantEnums: map[string][]string{
				"overall": {"pass", "fail"},
			},
		},
		{
			name:           "ralphglasses_repo_health",
			wantProperties: []string{"repo", "health_score", "circuit_breaker", "active_sessions", "errored_sessions", "total_spend_usd", "loop_running", "issues", "claudemd_findings"},
		},
		{
			name:           "ralphglasses_anomaly_detect",
			wantProperties: []string{"repo", "metric", "hours", "observations", "anomalies", "count", "message"},
		},
		{
			name:           "ralphglasses_session_tail",
			wantProperties: []string{"session_id", "status", "output", "lines_returned", "next_cursor", "is_active", "idle_seconds"},
		},
		{
			name:           "ralphglasses_session_output",
			wantProperties: []string{"session_id", "lines", "output"},
		},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			schema, ok := OutputSchemas[tc.name]
			if !ok {
				t.Fatalf("OutputSchemas missing %q", tc.name)
			}

			props, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema %q: properties is not map[string]any", tc.name)
			}

			// Check all expected properties exist.
			for _, wantProp := range tc.wantProperties {
				if _, exists := props[wantProp]; !exists {
					t.Errorf("schema %q missing property %q", tc.name, wantProp)
				}
			}

			// Check enum values at root level.
			for propName, wantEnum := range tc.wantEnums {
				propDef, ok := props[propName].(map[string]any)
				if !ok {
					t.Errorf("schema %q: property %q is not a map", tc.name, propName)
					continue
				}
				enumRaw, ok := propDef["enum"].([]any)
				if !ok {
					t.Errorf("schema %q: property %q has no enum", tc.name, propName)
					continue
				}
				if len(enumRaw) != len(wantEnum) {
					t.Errorf("schema %q: property %q enum length = %d, want %d", tc.name, propName, len(enumRaw), len(wantEnum))
					continue
				}
				for i, want := range wantEnum {
					got, _ := enumRaw[i].(string)
					if got != want {
						t.Errorf("schema %q: property %q enum[%d] = %q, want %q", tc.name, propName, i, got, want)
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Verify nested structures for complex schemas
// ---------------------------------------------------------------------------

func TestObservationQueryNestedItems(t *testing.T) {
	schema := OutputSchemas["ralphglasses_observation_query"]
	props := schema["properties"].(map[string]any)
	obs := props["observations"].(map[string]any)

	if obs["type"] != "array" {
		t.Fatalf("observations type = %v, want array", obs["type"])
	}
	items := obs["items"].(map[string]any)
	if items["type"] != "object" {
		t.Fatalf("observations items type = %v, want object", items["type"])
	}
	itemProps := items["properties"].(map[string]any)
	wantFields := []string{
		"loop_id", "iteration", "status", "planner_provider", "worker_provider",
		"cost_usd", "duration_seconds", "files_changed", "stall_count",
		"verify_passed", "task_type", "task_title", "confidence", "acceptance_path",
	}
	for _, f := range wantFields {
		if _, ok := itemProps[f]; !ok {
			t.Errorf("observation item missing field %q", f)
		}
	}
}

func TestLoopBenchmarkNestedTiming(t *testing.T) {
	schema := OutputSchemas["ralphglasses_loop_benchmark"]
	props := schema["properties"].(map[string]any)

	timing := props["timing"].(map[string]any)
	if timing["type"] != "object" {
		t.Fatalf("timing type = %v, want object", timing["type"])
	}
	timingProps := timing["properties"].(map[string]any)
	for _, f := range []string{"mean_seconds", "p50_seconds", "p95_seconds", "min_seconds", "max_seconds"} {
		prop, ok := timingProps[f].(map[string]any)
		if !ok {
			t.Errorf("timing missing %q", f)
			continue
		}
		if prop["type"] != "number" {
			t.Errorf("timing.%s type = %v, want number", f, prop["type"])
		}
	}

	tokens := props["tokens"].(map[string]any)
	if tokens["type"] != "object" {
		t.Fatalf("tokens type = %v, want object", tokens["type"])
	}
	tokenProps := tokens["properties"].(map[string]any)
	for _, f := range []string{"total_input", "total_output", "total"} {
		prop, ok := tokenProps[f].(map[string]any)
		if !ok {
			t.Errorf("tokens missing %q", f)
			continue
		}
		if prop["type"] != "integer" {
			t.Errorf("tokens.%s type = %v, want integer", f, prop["type"])
		}
	}
}

func TestFleetStatusNestedWorkers(t *testing.T) {
	schema := OutputSchemas["ralphglasses_fleet_status"]
	props := schema["properties"].(map[string]any)

	workers := props["workers"].(map[string]any)
	if workers["type"] != "array" {
		t.Fatalf("workers type = %v, want array", workers["type"])
	}
	items := workers["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)
	for _, f := range []string{"worker_id", "status", "provider", "current_task", "sessions_active", "capacity", "cost_usd", "uptime_seconds"} {
		if _, ok := itemProps[f]; !ok {
			t.Errorf("fleet worker item missing field %q", f)
		}
	}

	costSummary := props["cost_summary"].(map[string]any)
	costProps := costSummary["properties"].(map[string]any)
	for _, f := range []string{"total_spend_usd", "budget_usd", "burn_rate_per_hour"} {
		prop := costProps[f].(map[string]any)
		if prop["type"] != "number" {
			t.Errorf("cost_summary.%s type = %v, want number", f, prop["type"])
		}
	}
}

func TestCostEstimateNestedBreakdown(t *testing.T) {
	schema := OutputSchemas["ralphglasses_cost_estimate"]
	props := schema["properties"].(map[string]any)

	est := props["estimate"].(map[string]any)
	estProps := est["properties"].(map[string]any)
	for _, f := range []string{"low_usd", "mid_usd", "high_usd"} {
		prop := estProps[f].(map[string]any)
		if prop["type"] != "number" {
			t.Errorf("estimate.%s type = %v, want number", f, prop["type"])
		}
	}

	breakdown := props["breakdown"].(map[string]any)
	if breakdown["type"] != "array" {
		t.Fatalf("breakdown type = %v, want array", breakdown["type"])
	}
	bdItems := breakdown["items"].(map[string]any)
	bdProps := bdItems["properties"].(map[string]any)
	for _, f := range []string{"provider", "model", "cost_usd", "tokens"} {
		if _, ok := bdProps[f]; !ok {
			t.Errorf("breakdown item missing %q", f)
		}
	}
}

func TestSessionErrorsNestedSeverityEnum(t *testing.T) {
	schema := OutputSchemas["ralphglasses_session_errors"]
	props := schema["properties"].(map[string]any)
	errors := props["errors"].(map[string]any)
	items := errors["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)

	severity := itemProps["severity"].(map[string]any)
	enumRaw := severity["enum"].([]any)
	want := []string{"critical", "warning", "info"}
	if len(enumRaw) != len(want) {
		t.Fatalf("severity enum length = %d, want %d", len(enumRaw), len(want))
	}
	for i, w := range want {
		if enumRaw[i] != w {
			t.Errorf("severity enum[%d] = %v, want %q", i, enumRaw[i], w)
		}
	}
}

func TestAnomalyDetectNestedDirectionEnum(t *testing.T) {
	schema := OutputSchemas["ralphglasses_anomaly_detect"]
	props := schema["properties"].(map[string]any)
	anomalies := props["anomalies"].(map[string]any)
	items := anomalies["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)

	direction := itemProps["direction"].(map[string]any)
	enumRaw := direction["enum"].([]any)
	want := []string{"high", "low"}
	if len(enumRaw) != len(want) {
		t.Fatalf("direction enum length = %d, want %d", len(enumRaw), len(want))
	}
	for i, w := range want {
		if enumRaw[i] != w {
			t.Errorf("direction enum[%d] = %v, want %q", i, enumRaw[i], w)
		}
	}
}

func TestLoopGatesNestedVerdictEnum(t *testing.T) {
	schema := OutputSchemas["ralphglasses_loop_gates"]
	props := schema["properties"].(map[string]any)
	results := props["results"].(map[string]any)
	items := results["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)

	verdict := itemProps["verdict"].(map[string]any)
	enumRaw := verdict["enum"].([]any)
	want := []string{"pass", "warn", "fail", "skip"}
	if len(enumRaw) != len(want) {
		t.Fatalf("verdict enum length = %d, want %d", len(enumRaw), len(want))
	}
	for i, w := range want {
		if enumRaw[i] != w {
			t.Errorf("verdict enum[%d] = %v, want %q", i, enumRaw[i], w)
		}
	}
}

func TestMergeVerifyNestedStepStatusEnum(t *testing.T) {
	schema := OutputSchemas["ralphglasses_merge_verify"]
	props := schema["properties"].(map[string]any)
	steps := props["steps"].(map[string]any)
	items := steps["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)

	status := itemProps["status"].(map[string]any)
	enumRaw := status["enum"].([]any)
	want := []string{"pass", "fail"}
	if len(enumRaw) != len(want) {
		t.Fatalf("step status enum length = %d, want %d", len(enumRaw), len(want))
	}
	for i, w := range want {
		if enumRaw[i] != w {
			t.Errorf("step status enum[%d] = %v, want %q", i, enumRaw[i], w)
		}
	}
}

// ---------------------------------------------------------------------------
// Date-time format fields
// ---------------------------------------------------------------------------

func TestDateTimeFormatFields(t *testing.T) {
	// Collect all properties across all schemas that have format: "date-time".
	dateTimeFields := map[string][]string{
		"ralphglasses_cost_estimate":    {"exhaustion_eta"},
		"ralphglasses_session_status":   {"launched_at", "last_activity", "ended_at"},
		"ralphglasses_session_errors":   {}, // nested, tested separately below
		"ralphglasses_loop_status":      {"created_at", "updated_at"},
		"ralphglasses_loop_gates":       {"ts"},
		"ralphglasses_anomaly_detect":   {}, // nested
		"ralphglasses_session_diff":     {}, // nested in window
	}

	for schemaName, fields := range dateTimeFields {
		schema, ok := OutputSchemas[schemaName]
		if !ok {
			t.Errorf("missing schema %q", schemaName)
			continue
		}
		props := schema["properties"].(map[string]any)
		for _, f := range fields {
			prop, ok := props[f].(map[string]any)
			if !ok {
				t.Errorf("%s.%s missing", schemaName, f)
				continue
			}
			if prop["format"] != "date-time" {
				t.Errorf("%s.%s format = %v, want date-time", schemaName, f, prop["format"])
			}
		}
	}
}

func TestSessionDiffWindowDateTimeFields(t *testing.T) {
	schema := OutputSchemas["ralphglasses_session_diff"]
	props := schema["properties"].(map[string]any)
	window := props["window"].(map[string]any)
	windowProps := window["properties"].(map[string]any)

	for _, f := range []string{"started", "ended"} {
		prop := windowProps[f].(map[string]any)
		if prop["format"] != "date-time" {
			t.Errorf("session_diff.window.%s format = %v, want date-time", f, prop["format"])
		}
	}
}

func TestAnomalyDetectTimestampFormat(t *testing.T) {
	schema := OutputSchemas["ralphglasses_anomaly_detect"]
	props := schema["properties"].(map[string]any)
	anomalies := props["anomalies"].(map[string]any)
	items := anomalies["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)
	ts := itemProps["timestamp"].(map[string]any)
	if ts["format"] != "date-time" {
		t.Errorf("anomaly_detect.anomalies[].timestamp format = %v, want date-time", ts["format"])
	}
}

// ---------------------------------------------------------------------------
// additionalProperties schemas
// ---------------------------------------------------------------------------

func TestAdditionalPropertiesSchemas(t *testing.T) {
	cases := []struct {
		schema   string
		property string
		wantType string
	}{
		{"ralphglasses_observation_summary", "cost_by_provider", "number"},
		{"ralphglasses_observation_summary", "acceptance_counts", "integer"},
		{"ralphglasses_fleet_analytics", "repos", "number"},
		{"ralphglasses_session_errors", "by_type", "integer"},
		{"ralphglasses_session_errors", "by_severity", "integer"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%s", tc.schema, tc.property), func(t *testing.T) {
			schema := OutputSchemas[tc.schema]
			props := schema["properties"].(map[string]any)
			prop := props[tc.property].(map[string]any)

			if prop["type"] != "object" {
				t.Fatalf("type = %v, want object", prop["type"])
			}
			ap, ok := prop["additionalProperties"].(map[string]any)
			if !ok {
				t.Fatal("missing additionalProperties")
			}
			if ap["type"] != tc.wantType {
				t.Errorf("additionalProperties type = %v, want %q", ap["type"], tc.wantType)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// JSON round-trip: marshal then unmarshal produces identical structure
// ---------------------------------------------------------------------------

func TestSchemasRoundTrip(t *testing.T) {
	for name, schema := range OutputSchemas {
		data, err := json.Marshal(schema)
		if err != nil {
			t.Errorf("%s: marshal failed: %v", name, err)
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Errorf("%s: unmarshal failed: %v", name, err)
			continue
		}
		// Re-marshal to compare (canonical form).
		data2, _ := json.Marshal(decoded)
		if string(data) != string(data2) {
			t.Errorf("%s: round-trip mismatch", name)
		}
	}
}

// ---------------------------------------------------------------------------
// SchemaForTool returns the same pointer as OutputSchemas
// ---------------------------------------------------------------------------

func TestSchemaForToolReturnsSameReference(t *testing.T) {
	for name, schema := range OutputSchemas {
		got := SchemaForTool(name)
		if fmt.Sprintf("%p", got) != fmt.Sprintf("%p", schema) {
			t.Errorf("SchemaForTool(%q) returned different map pointer", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Verify specific property types for session_output (simple schema)
// ---------------------------------------------------------------------------

func TestSessionOutputPropertyTypes(t *testing.T) {
	schema := OutputSchemas["ralphglasses_session_output"]
	props := schema["properties"].(map[string]any)

	wantTypes := map[string]string{
		"session_id": "string",
		"lines":      "integer",
		"output":     "array",
	}
	for field, wantType := range wantTypes {
		prop := props[field].(map[string]any)
		if prop["type"] != wantType {
			t.Errorf("session_output.%s type = %v, want %q", field, prop["type"], wantType)
		}
	}

	// output items should be strings
	output := props["output"].(map[string]any)
	items := output["items"].(map[string]any)
	if items["type"] != "string" {
		t.Errorf("session_output.output items type = %v, want string", items["type"])
	}
}

// ---------------------------------------------------------------------------
// Verify session_tail output array and boolean fields
// ---------------------------------------------------------------------------

func TestSessionTailPropertyTypes(t *testing.T) {
	schema := OutputSchemas["ralphglasses_session_tail"]
	props := schema["properties"].(map[string]any)

	wantTypes := map[string]string{
		"session_id":     "string",
		"status":         "string",
		"output":         "array",
		"lines_returned": "integer",
		"next_cursor":    "string",
		"is_active":      "boolean",
		"idle_seconds":   "integer",
	}
	for field, wantType := range wantTypes {
		prop := props[field].(map[string]any)
		if prop["type"] != wantType {
			t.Errorf("session_tail.%s type = %v, want %q", field, prop["type"], wantType)
		}
	}
}

// ---------------------------------------------------------------------------
// Verify repo_health issues array items are strings
// ---------------------------------------------------------------------------

func TestRepoHealthIssuesArrayItems(t *testing.T) {
	schema := OutputSchemas["ralphglasses_repo_health"]
	props := schema["properties"].(map[string]any)
	issues := props["issues"].(map[string]any)
	if issues["type"] != "array" {
		t.Fatalf("issues type = %v, want array", issues["type"])
	}
	items := issues["items"].(map[string]any)
	if items["type"] != "string" {
		t.Errorf("issues items type = %v, want string", items["type"])
	}
}

// ---------------------------------------------------------------------------
// Verify coverage_report packages nested structure
// ---------------------------------------------------------------------------

func TestCoverageReportPackagesNested(t *testing.T) {
	schema := OutputSchemas["ralphglasses_coverage_report"]
	props := schema["properties"].(map[string]any)
	packages := props["packages"].(map[string]any)
	if packages["type"] != "array" {
		t.Fatalf("packages type = %v, want array", packages["type"])
	}
	items := packages["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)

	wantFields := map[string]string{
		"package":         "string",
		"coverage":        "number",
		"threshold":       "number",
		"meets_threshold": "boolean",
		"functions_hit":   "integer",
		"functions_total": "integer",
	}
	for field, wantType := range wantFields {
		prop, ok := itemProps[field].(map[string]any)
		if !ok {
			t.Errorf("coverage_report.packages[].%s missing", field)
			continue
		}
		if prop["type"] != wantType {
			t.Errorf("coverage_report.packages[].%s type = %v, want %q", field, prop["type"], wantType)
		}
	}
}

// ---------------------------------------------------------------------------
// Fleet analytics nested provider structure
// ---------------------------------------------------------------------------

func TestFleetAnalyticsProviderNested(t *testing.T) {
	schema := OutputSchemas["ralphglasses_fleet_analytics"]
	props := schema["properties"].(map[string]any)
	providers := props["providers"].(map[string]any)
	if providers["type"] != "object" {
		t.Fatalf("providers type = %v, want object", providers["type"])
	}
	ap := providers["additionalProperties"].(map[string]any)
	if ap["type"] != "object" {
		t.Fatalf("providers additionalProperties type = %v, want object", ap["type"])
	}
	apProps := ap["properties"].(map[string]any)
	wantFields := []string{"sessions", "running", "total_spend_usd", "avg_cost_per_turn", "total_turns"}
	for _, f := range wantFields {
		if _, ok := apProps[f]; !ok {
			t.Errorf("fleet_analytics.providers.*.%s missing", f)
		}
	}
}

// ---------------------------------------------------------------------------
// Loop status profile nested structure
// ---------------------------------------------------------------------------

func TestLoopStatusProfileNested(t *testing.T) {
	schema := OutputSchemas["ralphglasses_loop_status"]
	props := schema["properties"].(map[string]any)
	profile := props["profile"].(map[string]any)
	profileProps := profile["properties"].(map[string]any)

	wantFields := map[string]string{
		"planner_model":   "string",
		"worker_model":    "string",
		"verifier_model":  "string",
		"worktree_policy": "string",
		"retry_limit":     "integer",
	}
	for field, wantType := range wantFields {
		prop, ok := profileProps[field].(map[string]any)
		if !ok {
			t.Errorf("loop_status.profile.%s missing", field)
			continue
		}
		if prop["type"] != wantType {
			t.Errorf("loop_status.profile.%s type = %v, want %q", field, prop["type"], wantType)
		}
	}
}
