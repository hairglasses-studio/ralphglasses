package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/plugin"
)

// fakePlugin implements plugin.Plugin for testing.
type fakePlugin struct {
	name    string
	version string
}

func (p *fakePlugin) Name() string    { return p.name }
func (p *fakePlugin) Version() string { return p.version }
func (p *fakePlugin) Init(_ context.Context, _ plugin.PluginHost) error {
	return nil
}
func (p *fakePlugin) Shutdown() error { return nil }

func setupPluginServer(t *testing.T) *Server {
	t.Helper()
	srv, _ := setupTestServer(t)
	reg := plugin.NewRegistry()

	p1 := &fakePlugin{name: "metrics", version: "1.0.0"}
	p2 := &fakePlugin{name: "logger", version: "2.1.0"}
	if err := reg.Register(p1); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(p2); err != nil {
		t.Fatal(err)
	}
	// Simulate init to set status to active.
	if err := reg.InitAll(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	srv.PluginRegistry = reg
	return srv
}

func TestHandlePluginList(t *testing.T) {
	t.Parallel()

	t.Run("with plugins", func(t *testing.T) {
		t.Parallel()
		srv := setupPluginServer(t)
		result, err := srv.handlePluginList(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", getResultText(result))
		}
		text := getResultText(result)

		var data map[string]any
		if err := json.Unmarshal([]byte(text), &data); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		count := int(data["count"].(float64))
		if count != 2 {
			t.Errorf("count = %d, want 2", count)
		}
		plugins := data["plugins"].([]any)
		if len(plugins) != 2 {
			t.Errorf("plugins length = %d, want 2", len(plugins))
		}
		// Check first plugin has required fields.
		p0 := plugins[0].(map[string]any)
		for _, key := range []string{"name", "version", "status", "type"} {
			if _, ok := p0[key]; !ok {
				t.Errorf("plugin missing key %q", key)
			}
		}
	})

	t.Run("no registry", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)
		result, err := srv.handlePluginList(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := getResultText(result)
		if !strings.Contains(text, "not initialized") {
			t.Errorf("expected 'not initialized' message, got: %s", text)
		}
	})
}

func TestHandlePluginInfo(t *testing.T) {
	t.Parallel()

	t.Run("existing plugin", func(t *testing.T) {
		t.Parallel()
		srv := setupPluginServer(t)
		result, err := srv.handlePluginInfo(context.Background(), makeRequest(map[string]any{
			"name": "metrics",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", getResultText(result))
		}
		text := getResultText(result)
		assertJSON(t, text, "name", "version", "status", "type")

		var data map[string]any
		if err := json.Unmarshal([]byte(text), &data); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if data["name"] != "metrics" {
			t.Errorf("name = %v, want metrics", data["name"])
		}
		if data["version"] != "1.0.0" {
			t.Errorf("version = %v, want 1.0.0", data["version"])
		}
	})

	t.Run("missing name", func(t *testing.T) {
		t.Parallel()
		srv := setupPluginServer(t)
		result, err := srv.handlePluginInfo(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing name")
		}
		text := getResultText(result)
		if !strings.Contains(text, "INVALID_PARAMS") {
			t.Errorf("expected INVALID_PARAMS, got: %s", text)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		srv := setupPluginServer(t)
		result, err := srv.handlePluginInfo(context.Background(), makeRequest(map[string]any{
			"name": "nonexistent",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for nonexistent plugin")
		}
		text := getResultText(result)
		if !strings.Contains(text, "not found") {
			t.Errorf("expected 'not found', got: %s", text)
		}
	})
}

func TestHandlePluginEnable(t *testing.T) {
	t.Parallel()

	t.Run("enable disabled plugin", func(t *testing.T) {
		t.Parallel()
		srv := setupPluginServer(t)
		// First disable the plugin.
		if err := srv.PluginRegistry.Disable("metrics"); err != nil {
			t.Fatal(err)
		}
		// Then enable via handler.
		result, err := srv.handlePluginEnable(context.Background(), makeRequest(map[string]any{
			"name": "metrics",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, "enabled") {
			t.Errorf("expected 'enabled' in response, got: %s", text)
		}
		// Verify status.
		status, _ := srv.PluginRegistry.GetStatus("metrics")
		if status != plugin.StatusActive {
			t.Errorf("status = %s, want active", status)
		}
	})

	t.Run("enable already active", func(t *testing.T) {
		t.Parallel()
		srv := setupPluginServer(t)
		result, err := srv.handlePluginEnable(context.Background(), makeRequest(map[string]any{
			"name": "metrics",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for already active plugin")
		}
		text := getResultText(result)
		if !strings.Contains(text, "not disabled") {
			t.Errorf("expected 'not disabled', got: %s", text)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		t.Parallel()
		srv := setupPluginServer(t)
		result, err := srv.handlePluginEnable(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing name")
		}
	})
}

func TestHandlePluginDisable(t *testing.T) {
	t.Parallel()

	t.Run("disable active plugin", func(t *testing.T) {
		t.Parallel()
		srv := setupPluginServer(t)
		result, err := srv.handlePluginDisable(context.Background(), makeRequest(map[string]any{
			"name": "logger",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, "disabled") {
			t.Errorf("expected 'disabled' in response, got: %s", text)
		}
		// Verify status.
		status, _ := srv.PluginRegistry.GetStatus("logger")
		if status != plugin.StatusDisabled {
			t.Errorf("status = %s, want disabled", status)
		}
	})

	t.Run("disable already disabled", func(t *testing.T) {
		t.Parallel()
		srv := setupPluginServer(t)
		// Disable first.
		if err := srv.PluginRegistry.Disable("logger"); err != nil {
			t.Fatal(err)
		}
		result, err := srv.handlePluginDisable(context.Background(), makeRequest(map[string]any{
			"name": "logger",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for already disabled plugin")
		}
		text := getResultText(result)
		if !strings.Contains(text, "not active") {
			t.Errorf("expected 'not active', got: %s", text)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		srv := setupPluginServer(t)
		result, err := srv.handlePluginDisable(context.Background(), makeRequest(map[string]any{
			"name": "nonexistent",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for nonexistent plugin")
		}
	})
}

func TestHandlePluginList_StatusReflectsChanges(t *testing.T) {
	t.Parallel()
	srv := setupPluginServer(t)

	// Disable a plugin.
	if err := srv.PluginRegistry.Disable("metrics"); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handlePluginList(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)

	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	plugins := data["plugins"].([]any)
	for _, p := range plugins {
		pm := p.(map[string]any)
		if pm["name"] == "metrics" {
			if pm["status"] != "disabled" {
				t.Errorf("metrics status = %v, want disabled", pm["status"])
			}
		}
	}
}
