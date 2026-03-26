package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

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

func TestSchemaForToolExists(t *testing.T) {
	known := []string{
		"ralphglasses_observation_query",
		"ralphglasses_observation_summary",
		"ralphglasses_loop_benchmark",
		"ralphglasses_fleet_status",
		"ralphglasses_cost_estimate",
		"ralphglasses_coverage_report",
	}
	for _, name := range known {
		schema := SchemaForTool(name)
		if schema == nil {
			t.Errorf("SchemaForTool(%q) returned nil, want non-nil", name)
		}
	}
}

func TestSchemaForToolMissing(t *testing.T) {
	schema := SchemaForTool("nonexistent_tool")
	if schema != nil {
		t.Errorf("SchemaForTool(nonexistent) = %v, want nil", schema)
	}
}
