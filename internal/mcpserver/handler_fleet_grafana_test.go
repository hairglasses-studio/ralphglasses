package mcpserver

import (
	"context"
	"strings"
	"testing"
)

func TestHandleFleetGrafana_DefaultTitle(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleFleetGrafana(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "Ralphglasses Fleet Metrics") {
		t.Errorf("expected default title in dashboard, got: %s", text[:min(200, len(text))])
	}
}

func TestHandleFleetGrafana_CustomTitle(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleFleetGrafana(context.Background(), makeRequest(map[string]any{
		"title": "My Custom Dashboard",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "My Custom Dashboard") {
		t.Errorf("expected custom title in dashboard, got: %s", text[:min(200, len(text))])
	}
}

func TestHandleFleetGrafana_WithMetrics(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleFleetGrafana(context.Background(), makeRequest(map[string]any{
		"metrics": "cost_usd,session_count",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
}

func TestHandleFleetGrafana_WithDatasource(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleFleetGrafana(context.Background(), makeRequest(map[string]any{
		"datasource": "prometheus",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
}
