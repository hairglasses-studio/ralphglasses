package fleet

import (
	"encoding/json"
	"fmt"
	"time"
)

// DefaultDatasource is the Prometheus data source name used when none is specified.
const DefaultDatasource = "prometheus"

// GrafanaDashboard mirrors the Grafana dashboard JSON model (schemaVersion 39).
type GrafanaDashboard struct {
	// Meta fields (not part of Grafana model but useful for callers).
	GeneratedAt time.Time `json:"-"`

	// Dashboard model fields.
	ID                   *int64             `json:"id"`
	UID                  string             `json:"uid,omitempty"`
	Title                string             `json:"title"`
	Description          string             `json:"description,omitempty"`
	Tags                 []string           `json:"tags"`
	Timezone             string             `json:"timezone"`
	SchemaVersion        int                `json:"schemaVersion"`
	Version              int                `json:"version"`
	Editable             bool               `json:"editable"`
	Refresh              string             `json:"refresh"`
	Time                 GrafanaTimeRange   `json:"time"`
	Templating           GrafanaTemplating  `json:"templating"`
	Panels               []GrafanaPanel     `json:"panels"`
	Annotations          GrafanaAnnotations `json:"annotations"`
	FiscalYearStartMonth int                `json:"fiscalYearStartMonth"`
	LiveNow              bool               `json:"liveNow"`
}

// GrafanaTimeRange defines the dashboard time window.
type GrafanaTimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// GrafanaTemplating holds dashboard template variables.
type GrafanaTemplating struct {
	List []GrafanaTemplateVar `json:"list"`
}

// GrafanaTemplateVar is a single template variable.
type GrafanaTemplateVar struct {
	Name       string             `json:"name"`
	Label      string             `json:"label,omitempty"`
	Type       string             `json:"type"`
	Datasource *GrafanaDatasource `json:"datasource,omitempty"`
	Query      string             `json:"query,omitempty"`
	Current    map[string]any     `json:"current,omitempty"`
	Options    []map[string]any   `json:"options,omitempty"`
	Multi      bool               `json:"multi"`
	IncludeAll bool               `json:"includeAll"`
	Refresh    int                `json:"refresh"`
	Sort       int                `json:"sort"`
}

// GrafanaAnnotations holds annotation list for the dashboard.
type GrafanaAnnotations struct {
	List []GrafanaAnnotation `json:"list"`
}

// GrafanaAnnotation is a single annotation definition.
type GrafanaAnnotation struct {
	BuiltIn    int               `json:"builtIn"`
	Datasource GrafanaDatasource `json:"datasource"`
	Enable     bool              `json:"enable"`
	Hide       bool              `json:"hide"`
	IconColor  string            `json:"iconColor"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
}

// GrafanaDatasource identifies a Grafana data source.
type GrafanaDatasource struct {
	Type string `json:"type"`
	UID  string `json:"uid"`
}

// GrafanaPanel represents a single panel in a Grafana dashboard.
type GrafanaPanel struct {
	ID          int                `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description,omitempty"`
	Type        string             `json:"type"`
	Datasource  *GrafanaDatasource `json:"datasource,omitempty"`
	GridPos     GrafanaGridPos     `json:"gridPos"`
	Targets     []GrafanaTarget    `json:"targets"`
	FieldConfig GrafanaFieldConfig `json:"fieldConfig"`
	Options     map[string]any     `json:"options"`
}

// GrafanaGridPos defines the panel position and size in the grid.
type GrafanaGridPos struct {
	H int `json:"h"`
	W int `json:"w"`
	X int `json:"x"`
	Y int `json:"y"`
}

// GrafanaTarget is a single query target (PromQL).
type GrafanaTarget struct {
	Expr         string `json:"expr"`
	LegendFormat string `json:"legendFormat,omitempty"`
	RefID        string `json:"refId"`
	Interval     string `json:"interval,omitempty"`
}

// GrafanaFieldConfig holds panel field configuration.
type GrafanaFieldConfig struct {
	Defaults  GrafanaFieldDefaults `json:"defaults"`
	Overrides []any                `json:"overrides"`
}

// GrafanaFieldDefaults holds default field settings.
type GrafanaFieldDefaults struct {
	Color      map[string]any          `json:"color,omitempty"`
	Custom     map[string]any          `json:"custom,omitempty"`
	Thresholds *GrafanaThresholdConfig `json:"thresholds,omitempty"`
	Unit       string                  `json:"unit,omitempty"`
}

// GrafanaThresholdConfig defines thresholds for a panel.
type GrafanaThresholdConfig struct {
	Mode  string             `json:"mode"`
	Steps []GrafanaThreshold `json:"steps"`
}

// GrafanaThreshold is a single threshold step.
type GrafanaThreshold struct {
	Color string   `json:"color"`
	Value *float64 `json:"value"`
}

// allMetrics lists every supported metric key for panel generation.
var allMetrics = []string{
	"session_throughput",
	"cost_burn_rate",
	"provider_comparison",
	"error_rate",
	"active_sessions",
	"budget_utilization",
}

// ExportDashboard generates a Grafana dashboard with panels for the requested
// metrics. If metrics is nil or empty, all available panels are included.
// The datasource parameter controls the Prometheus data source name; pass ""
// for the default.
func ExportDashboard(title string, metrics []string, datasource string) *GrafanaDashboard {
	if datasource == "" {
		datasource = DefaultDatasource
	}
	if len(metrics) == 0 {
		metrics = allMetrics
	}

	ds := &GrafanaDatasource{
		Type: "prometheus",
		UID:  datasource,
	}

	selected := make(map[string]bool, len(metrics))
	for _, m := range metrics {
		selected[m] = true
	}

	var panels []GrafanaPanel
	id := 1
	y := 0

	// Build panels in a deterministic order; only include selected metrics.
	type panelSpec struct {
		key   string
		build func(id int, y int, ds *GrafanaDatasource) GrafanaPanel
	}
	specs := []panelSpec{
		{"session_throughput", buildSessionThroughputPanel},
		{"cost_burn_rate", buildCostBurnRatePanel},
		{"provider_comparison", buildProviderComparisonPanel},
		{"error_rate", buildErrorRatePanel},
		{"active_sessions", buildActiveSessionsPanel},
		{"budget_utilization", buildBudgetUtilizationPanel},
	}

	for _, spec := range specs {
		if !selected[spec.key] {
			continue
		}
		p := spec.build(id, y, ds)
		panels = append(panels, p)
		y += p.GridPos.H
		id++
	}

	return &GrafanaDashboard{
		GeneratedAt:          time.Now(),
		ID:                   nil,
		Title:                title,
		Description:          fmt.Sprintf("Ralphglasses fleet metrics dashboard — generated %s", time.Now().UTC().Format(time.RFC3339)),
		Tags:                 []string{"ralphglasses", "fleet", "llm"},
		Timezone:             "browser",
		SchemaVersion:        39,
		Version:              1,
		Editable:             true,
		Refresh:              "30s",
		FiscalYearStartMonth: 0,
		LiveNow:              false,
		Time: GrafanaTimeRange{
			From: "now-1h",
			To:   "now",
		},
		Templating: GrafanaTemplating{
			List: []GrafanaTemplateVar{
				{
					Name:  "datasource",
					Label: "Data Source",
					Type:  "datasource",
					Query: "prometheus",
					Current: map[string]any{
						"selected": false,
						"text":     datasource,
						"value":    datasource,
					},
					Multi:      false,
					IncludeAll: false,
					Refresh:    1,
					Sort:       0,
				},
				{
					Name:  "provider",
					Label: "Provider",
					Type:  "query",
					Datasource: &GrafanaDatasource{
						Type: "prometheus",
						UID:  datasource,
					},
					Query:      "label_values(ralphglasses_session_total, provider)",
					Multi:      true,
					IncludeAll: true,
					Refresh:    2,
					Sort:       1,
				},
			},
		},
		Annotations: GrafanaAnnotations{
			List: []GrafanaAnnotation{
				{
					BuiltIn: 1,
					Datasource: GrafanaDatasource{
						Type: "grafana",
						UID:  "-- Grafana --",
					},
					Enable:    true,
					Hide:      true,
					IconColor: "rgba(0, 211, 255, 1)",
					Name:      "Annotations & Alerts",
					Type:      "dashboard",
				},
			},
		},
		Panels: panels,
	}
}

// ToJSON serializes a dashboard to Grafana-importable JSON.
func ToJSON(dash *GrafanaDashboard) ([]byte, error) {
	return json.MarshalIndent(dash, "", "  ")
}

// ---------- panel builders ----------

func buildSessionThroughputPanel(id, y int, ds *GrafanaDatasource) GrafanaPanel {
	return GrafanaPanel{
		ID:         id,
		Title:      "Session Throughput",
		Type:       "timeseries",
		Datasource: ds,
		GridPos:    GrafanaGridPos{H: 8, W: 12, X: 0, Y: y},
		Targets: []GrafanaTarget{
			{
				Expr:         `sum(rate(ralphglasses_session_completions_total{provider=~"$provider"}[5m])) * 3600`,
				LegendFormat: "sessions/hour",
				RefID:        "A",
			},
		},
		FieldConfig: GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Color: map[string]any{"mode": "palette-classic"},
				Custom: map[string]any{
					"axisCenteredZero": false,
					"drawStyle":        "line",
					"fillOpacity":      10,
					"lineWidth":        2,
					"pointSize":        5,
					"showPoints":       "auto",
				},
				Unit: "short",
			},
			Overrides: []any{},
		},
		Options: map[string]any{
			"tooltip": map[string]any{"mode": "single", "sort": "none"},
			"legend":  map[string]any{"displayMode": "list", "placement": "bottom"},
		},
	}
}

func buildCostBurnRatePanel(id, y int, ds *GrafanaDatasource) GrafanaPanel {
	return GrafanaPanel{
		ID:         id,
		Title:      "Cost Burn Rate",
		Type:       "timeseries",
		Datasource: ds,
		GridPos:    GrafanaGridPos{H: 8, W: 12, X: 12, Y: y},
		Targets: []GrafanaTarget{
			{
				Expr:         `sum(rate(ralphglasses_cost_usd_total{provider=~"$provider"}[5m])) * 3600`,
				LegendFormat: "$/hour",
				RefID:        "A",
			},
		},
		FieldConfig: GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Color: map[string]any{"mode": "palette-classic"},
				Custom: map[string]any{
					"axisCenteredZero": false,
					"drawStyle":        "line",
					"fillOpacity":      20,
					"lineWidth":        2,
					"pointSize":        5,
					"showPoints":       "auto",
				},
				Unit: "currencyUSD",
				Thresholds: &GrafanaThresholdConfig{
					Mode: "absolute",
					Steps: []GrafanaThreshold{
						{Color: "green", Value: nil},
						{Color: "yellow", Value: float64Ptr(5)},
						{Color: "red", Value: float64Ptr(20)},
					},
				},
			},
			Overrides: []any{},
		},
		Options: map[string]any{
			"tooltip": map[string]any{"mode": "single", "sort": "none"},
			"legend":  map[string]any{"displayMode": "list", "placement": "bottom"},
		},
	}
}

func buildProviderComparisonPanel(id, y int, ds *GrafanaDatasource) GrafanaPanel {
	return GrafanaPanel{
		ID:         id,
		Title:      "Cost by Provider",
		Type:       "barchart",
		Datasource: ds,
		GridPos:    GrafanaGridPos{H: 8, W: 12, X: 0, Y: y},
		Targets: []GrafanaTarget{
			{
				Expr:         `sum by (provider) (increase(ralphglasses_cost_usd_total[1h]))`,
				LegendFormat: "{{provider}}",
				RefID:        "A",
			},
		},
		FieldConfig: GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Color: map[string]any{"mode": "palette-classic"},
				Unit:  "currencyUSD",
			},
			Overrides: []any{},
		},
		Options: map[string]any{
			"orientation":        "horizontal",
			"xTickLabelRotation": 0,
			"showValue":          "auto",
			"groupWidth":         0.7,
			"barWidth":           0.97,
			"stacking":           map[string]any{"mode": "none"},
			"tooltip":            map[string]any{"mode": "single", "sort": "none"},
			"legend":             map[string]any{"displayMode": "list", "placement": "bottom"},
		},
	}
}

func buildErrorRatePanel(id, y int, ds *GrafanaDatasource) GrafanaPanel {
	return GrafanaPanel{
		ID:         id,
		Title:      "Error Rate",
		Type:       "timeseries",
		Datasource: ds,
		GridPos:    GrafanaGridPos{H: 8, W: 12, X: 12, Y: y},
		Targets: []GrafanaTarget{
			{
				Expr:         `sum(rate(ralphglasses_session_failures_total{provider=~"$provider"}[5m])) / (sum(rate(ralphglasses_session_completions_total{provider=~"$provider"}[5m])) + sum(rate(ralphglasses_session_failures_total{provider=~"$provider"}[5m])))`,
				LegendFormat: "error rate",
				RefID:        "A",
			},
		},
		FieldConfig: GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Color: map[string]any{"mode": "palette-classic"},
				Custom: map[string]any{
					"axisCenteredZero": false,
					"drawStyle":        "line",
					"fillOpacity":      10,
					"lineWidth":        2,
					"pointSize":        5,
					"showPoints":       "auto",
				},
				Unit: "percentunit",
				Thresholds: &GrafanaThresholdConfig{
					Mode: "absolute",
					Steps: []GrafanaThreshold{
						{Color: "green", Value: nil},
						{Color: "yellow", Value: new(0.05)},
						{Color: "red", Value: new(0.2)},
					},
				},
			},
			Overrides: []any{},
		},
		Options: map[string]any{
			"tooltip": map[string]any{"mode": "single", "sort": "none"},
			"legend":  map[string]any{"displayMode": "list", "placement": "bottom"},
		},
	}
}

func buildActiveSessionsPanel(id, y int, ds *GrafanaDatasource) GrafanaPanel {
	return GrafanaPanel{
		ID:         id,
		Title:      "Active Sessions",
		Type:       "gauge",
		Datasource: ds,
		GridPos:    GrafanaGridPos{H: 8, W: 8, X: 0, Y: y},
		Targets: []GrafanaTarget{
			{
				Expr:         `sum(ralphglasses_sessions_active{provider=~"$provider"})`,
				LegendFormat: "active",
				RefID:        "A",
			},
		},
		FieldConfig: GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Color: map[string]any{"mode": "thresholds"},
				Unit:  "short",
				Thresholds: &GrafanaThresholdConfig{
					Mode: "absolute",
					Steps: []GrafanaThreshold{
						{Color: "green", Value: nil},
						{Color: "yellow", Value: float64Ptr(10)},
						{Color: "red", Value: float64Ptr(25)},
					},
				},
			},
			Overrides: []any{},
		},
		Options: map[string]any{
			"reduceOptions": map[string]any{
				"calcs":  []string{"lastNotNull"},
				"fields": "",
				"values": false,
			},
			"showThresholdLabels":  false,
			"showThresholdMarkers": true,
			"orientation":          "auto",
		},
	}
}

func buildBudgetUtilizationPanel(id, y int, ds *GrafanaDatasource) GrafanaPanel {
	return GrafanaPanel{
		ID:         id,
		Title:      "Budget Utilization",
		Type:       "stat",
		Datasource: ds,
		GridPos:    GrafanaGridPos{H: 8, W: 8, X: 8, Y: y},
		Targets: []GrafanaTarget{
			{
				Expr:         `ralphglasses_budget_spent_usd / ralphglasses_budget_limit_usd`,
				LegendFormat: "utilization",
				RefID:        "A",
			},
			{
				Expr:         `ralphglasses_budget_spent_usd`,
				LegendFormat: "spent",
				RefID:        "B",
			},
			{
				Expr:         `ralphglasses_budget_limit_usd - ralphglasses_budget_spent_usd`,
				LegendFormat: "remaining",
				RefID:        "C",
			},
		},
		FieldConfig: GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Color: map[string]any{"mode": "thresholds"},
				Unit:  "percentunit",
				Thresholds: &GrafanaThresholdConfig{
					Mode: "absolute",
					Steps: []GrafanaThreshold{
						{Color: "green", Value: nil},
						{Color: "yellow", Value: new(0.7)},
						{Color: "red", Value: new(0.9)},
					},
				},
			},
			Overrides: []any{},
		},
		Options: map[string]any{
			"reduceOptions": map[string]any{
				"calcs":  []string{"lastNotNull"},
				"fields": "",
				"values": false,
			},
			"colorMode":   "background",
			"graphMode":   "area",
			"justifyMode": "auto",
			"textMode":    "auto",
			"orientation": "horizontal",
		},
	}
}

//go:fix inline
func float64Ptr(v float64) *float64 {
	return new(v)
}
