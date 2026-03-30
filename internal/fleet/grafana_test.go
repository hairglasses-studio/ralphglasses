package fleet

import (
	"encoding/json"
	"testing"
)

func TestExportDashboard_Default(t *testing.T) {
	dash := ExportDashboard("Fleet Metrics", nil, "")
	if dash.Title != "Fleet Metrics" {
		t.Fatalf("expected title 'Fleet Metrics', got %q", dash.Title)
	}
	if dash.SchemaVersion != 39 {
		t.Fatalf("expected schemaVersion 39, got %d", dash.SchemaVersion)
	}
	if len(dash.Panels) != 6 {
		t.Fatalf("expected 6 panels, got %d", len(dash.Panels))
	}

	// Verify all expected panel types present.
	titles := map[string]bool{}
	for _, p := range dash.Panels {
		titles[p.Title] = true
	}
	expected := []string{
		"Session Throughput",
		"Cost Burn Rate",
		"Cost by Provider",
		"Error Rate",
		"Active Sessions",
		"Budget Utilization",
	}
	for _, e := range expected {
		if !titles[e] {
			t.Errorf("missing expected panel %q", e)
		}
	}
}

func TestExportDashboard_CustomMetrics(t *testing.T) {
	dash := ExportDashboard("Custom", []string{"error_rate", "active_sessions"}, "")
	if len(dash.Panels) != 2 {
		t.Fatalf("expected 2 panels, got %d", len(dash.Panels))
	}
	if dash.Panels[0].Title != "Error Rate" {
		t.Errorf("expected first panel 'Error Rate', got %q", dash.Panels[0].Title)
	}
	if dash.Panels[1].Title != "Active Sessions" {
		t.Errorf("expected second panel 'Active Sessions', got %q", dash.Panels[1].Title)
	}
}

func TestExportDashboard_CustomDatasource(t *testing.T) {
	dash := ExportDashboard("DS Test", nil, "my-prometheus")
	for _, p := range dash.Panels {
		if p.Datasource == nil {
			t.Fatalf("panel %q has nil datasource", p.Title)
		}
		if p.Datasource.UID != "my-prometheus" {
			t.Errorf("panel %q: expected datasource UID 'my-prometheus', got %q", p.Title, p.Datasource.UID)
		}
	}

	// Template variable should also reference the custom datasource.
	if len(dash.Templating.List) < 2 {
		t.Fatalf("expected at least 2 template vars, got %d", len(dash.Templating.List))
	}
	dsVar := dash.Templating.List[0]
	if dsVar.Current["value"] != "my-prometheus" {
		t.Errorf("expected datasource template current value 'my-prometheus', got %v", dsVar.Current["value"])
	}
}

func TestExportDashboard_NonOverlappingGrid(t *testing.T) {
	dash := ExportDashboard("Grid Test", nil, "")

	type rect struct {
		x, y, w, h int
	}
	rects := make([]rect, len(dash.Panels))
	for i, p := range dash.Panels {
		rects[i] = rect{p.GridPos.X, p.GridPos.Y, p.GridPos.W, p.GridPos.H}
	}

	// Check no two panels overlap.
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			a, b := rects[i], rects[j]
			xOverlap := a.x < b.x+b.w && a.x+a.w > b.x
			yOverlap := a.y < b.y+b.h && a.y+a.h > b.y
			if xOverlap && yOverlap {
				t.Errorf("panels %d (%q) and %d (%q) overlap: a=%+v b=%+v",
					i, dash.Panels[i].Title, j, dash.Panels[j].Title, a, b)
			}
		}
	}

	// Verify all panels fit within Grafana's 24-column grid.
	for i, r := range rects {
		if r.x+r.w > 24 {
			t.Errorf("panel %d (%q) exceeds 24-col grid: x=%d w=%d", i, dash.Panels[i].Title, r.x, r.w)
		}
	}
}

func TestToJSON_ValidStructure(t *testing.T) {
	dash := ExportDashboard("JSON Test", nil, "")
	data, err := ToJSON(dash)
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	// Must be valid JSON.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Must have required Grafana top-level fields.
	requiredKeys := []string{"title", "schemaVersion", "panels", "time", "templating", "tags"}
	for _, key := range requiredKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing required Grafana key %q", key)
		}
	}

	// SchemaVersion should be numeric and >= 39.
	sv, ok := raw["schemaVersion"].(float64)
	if !ok || sv < 39 {
		t.Errorf("expected schemaVersion >= 39, got %v", raw["schemaVersion"])
	}

	// Panels should be an array.
	panels, ok := raw["panels"].([]any)
	if !ok {
		t.Fatalf("panels is not an array")
	}
	if len(panels) == 0 {
		t.Error("panels array is empty")
	}

	// Each panel should have required fields.
	for i, p := range panels {
		pm, ok := p.(map[string]any)
		if !ok {
			t.Errorf("panel %d is not an object", i)
			continue
		}
		for _, key := range []string{"id", "title", "type", "gridPos", "targets", "fieldConfig"} {
			if _, ok := pm[key]; !ok {
				t.Errorf("panel %d missing key %q", i, key)
			}
		}
	}
}

func TestToJSON_Reimport(t *testing.T) {
	dash := ExportDashboard("Reimport", nil, "")
	data, err := ToJSON(dash)
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	// Round-trip through our struct.
	var reimported GrafanaDashboard
	if err := json.Unmarshal(data, &reimported); err != nil {
		t.Fatalf("failed to unmarshal back into GrafanaDashboard: %v", err)
	}
	if reimported.Title != dash.Title {
		t.Errorf("title mismatch after reimport: %q vs %q", reimported.Title, dash.Title)
	}
	if len(reimported.Panels) != len(dash.Panels) {
		t.Errorf("panel count mismatch after reimport: %d vs %d", len(reimported.Panels), len(dash.Panels))
	}
}

func TestExportDashboard_PanelIDs(t *testing.T) {
	dash := ExportDashboard("IDs", nil, "")
	ids := make(map[int]bool)
	for _, p := range dash.Panels {
		if ids[p.ID] {
			t.Errorf("duplicate panel ID %d", p.ID)
		}
		ids[p.ID] = true
	}
}

func TestExportDashboard_SingleMetric(t *testing.T) {
	dash := ExportDashboard("Single", []string{"cost_burn_rate"}, "")
	if len(dash.Panels) != 1 {
		t.Fatalf("expected 1 panel, got %d", len(dash.Panels))
	}
	if dash.Panels[0].Title != "Cost Burn Rate" {
		t.Errorf("expected 'Cost Burn Rate', got %q", dash.Panels[0].Title)
	}
}

func TestExportDashboard_UnknownMetricIgnored(t *testing.T) {
	dash := ExportDashboard("Unknown", []string{"nonexistent_metric"}, "")
	if len(dash.Panels) != 0 {
		t.Errorf("expected 0 panels for unknown metric, got %d", len(dash.Panels))
	}
}
