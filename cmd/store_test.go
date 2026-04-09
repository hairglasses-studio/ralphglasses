package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/testutil/tenanttest"
)

func TestLoadManagerExternalSessions_LoadsSharedAndLegacyScanRootState(t *testing.T) {
	fx := tenanttest.NewFixture(t)
	fx.ApplyHome(t)

	sharedSession := &session.Session{
		ID:           "shared-session",
		TenantID:     "tenant-a",
		Provider:     session.ProviderCodex,
		RepoPath:     fx.ScanRoot,
		RepoName:     "scan-root",
		Status:       session.StatusRunning,
		Model:        "gpt-5.4",
		AgentName:    "shared",
		LaunchedAt:   time.Now().UTC(),
		LastActivity: time.Now().UTC(),
	}
	legacySession := &session.Session{
		ID:           "legacy-session",
		TenantID:     "tenant-a",
		Provider:     session.ProviderCodex,
		RepoPath:     fx.ScanRoot,
		RepoName:     "scan-root",
		Status:       session.StatusRunning,
		Model:        "gpt-5.4",
		AgentName:    "legacy",
		LaunchedAt:   time.Now().UTC(),
		LastActivity: time.Now().UTC(),
	}

	writeSessionStateFile(t, filepath.Join(ralphpath.SessionsDir(), "tenant-a", "shared-session.json"), sharedSession)
	fx.WriteExternalSession(t, legacySession.TenantID, legacySession.ID, legacySession)

	mgr := session.NewManager()
	loadManagerExternalSessions(mgr, fx.ScanRoot)

	if got := mgr.ListByTenant("", "tenant-a"); len(got) != 2 {
		t.Fatalf("loaded sessions = %d, want 2", len(got))
	}
	if _, ok := mgr.GetForTenant(sharedSession.ID, sharedSession.TenantID); !ok {
		t.Fatalf("shared session %q not loaded", sharedSession.ID)
	}
	if _, ok := mgr.GetForTenant(legacySession.ID, legacySession.TenantID); !ok {
		t.Fatalf("legacy session %q not loaded", legacySession.ID)
	}
}

func writeSessionStateFile(t *testing.T, path string, s *session.Session) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal session %s: %v", s.ID, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write session %s: %v", s.ID, err)
	}
}
