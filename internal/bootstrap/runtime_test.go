package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func requireSessionError(t *testing.T, bus *events.Bus, component string) events.Event {
	t.Helper()

	for _, event := range bus.History(events.SessionError, 20) {
		if event.Data["component"] == component {
			return event
		}
	}

	t.Fatalf("expected SessionError event for component %q", component)
	return events.Event{}
}

func TestInitManagerWithStore_PublishesFallbackEvent(t *testing.T) {
	home := t.TempDir()
	blockedPath := filepath.Join(home, ".ralphglasses")
	if err := os.WriteFile(blockedPath, []byte("not-a-directory"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("HOME", home)

	bus := events.NewBus(10)
	mgr := InitManagerWithStore(bus)
	t.Cleanup(func() {
		_ = mgr.Store().Close()
	})

	if _, ok := mgr.Store().(*session.MemoryStore); !ok {
		t.Fatalf("store = %T, want *session.MemoryStore", mgr.Store())
	}

	history := bus.History(events.SessionError, 10)
	if len(history) != 1 {
		t.Fatalf("session error history len = %d, want 1", len(history))
	}

	event := history[0]
	if got := event.Data["component"]; got != "bootstrap.store" {
		t.Fatalf("event component = %v, want bootstrap.store", got)
	}
	if got := event.Data["backend"]; got != "sqlite" {
		t.Fatalf("event backend = %v, want sqlite", got)
	}
	if got := event.Data["fallback_backend"]; got != "memory" {
		t.Fatalf("event fallback_backend = %v, want memory", got)
	}
	if got := event.Data["path"]; got != filepath.Join(home, ".ralphglasses", "state.db") {
		t.Fatalf("event path = %v, want %s", got, filepath.Join(home, ".ralphglasses", "state.db"))
	}
	if got := event.Data["error"]; got == "" {
		t.Fatal("event error should not be empty")
	}
}

func TestInitManagerWithStore_NoFallbackEventOnSQLiteSuccess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	bus := events.NewBus(10)
	mgr := InitManagerWithStore(bus)
	t.Cleanup(func() {
		_ = mgr.Store().Close()
	})

	if _, ok := mgr.Store().(*session.SQLiteStore); !ok {
		t.Fatalf("store = %T, want *session.SQLiteStore", mgr.Store())
	}

	history := bus.History(events.SessionError, 10)
	if len(history) != 0 {
		t.Fatalf("session error history len = %d, want 0", len(history))
	}
}

func TestInitManagerRuntime_PublishesConfigLoadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	scanRoot := t.TempDir()
	configPath := filepath.Join(scanRoot, ".ralphrc")
	if err := os.WriteFile(configPath, []byte("AUTONOMY_LEVEL=1\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(configPath, 0); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	bus := events.NewBus(10)
	mgr := InitManagerRuntime(scanRoot, bus)
	t.Cleanup(func() {
		_ = mgr.Store().Close()
	})

	event := requireSessionError(t, bus, "bootstrap.config")
	if got := event.Data["path"]; got != configPath {
		t.Fatalf("event path = %v, want %s", got, configPath)
	}
	if got := event.Data["error"]; got == "" {
		t.Fatal("event error should not be empty")
	}
}

func TestConfigureMCPRuntime_PublishesResearchGatewayError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := t.TempDir()
	scanRoot := filepath.Join(root, "ralphglasses")
	if err := os.MkdirAll(scanRoot, 0755); err != nil {
		t.Fatalf("MkdirAll(scanRoot): %v", err)
	}

	docsRoot := DefaultDocsRoot(scanRoot)
	if err := os.MkdirAll(filepath.Join(docsRoot, ".docs.sqlite"), 0755); err != nil {
		t.Fatalf("MkdirAll(.docs.sqlite dir): %v", err)
	}

	bus := events.NewBus(10)
	srv := mcpserver.NewServerWithBus(scanRoot, bus)
	cleanup := ConfigureMCPRuntime(scanRoot, bus, srv)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = srv.SessMgr.Store().Close()
	})

	event := requireSessionError(t, bus, "bootstrap.research_gateway")
	if got := event.Data["docs_root"]; got != docsRoot {
		t.Fatalf("event docs_root = %v, want %s", got, docsRoot)
	}
	if got := event.Data["error"]; got == "" {
		t.Fatal("event error should not be empty")
	}
}
