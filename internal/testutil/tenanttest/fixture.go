package tenanttest

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// Fixture provides a shared temp HOME plus scan root/state layout for tenant tests.
type Fixture struct {
	Home      string
	ScanRoot  string
	StateDir  string
	StorePath string
}

// NewFixture creates an isolated HOME, scan root, session state dir, and store path.
func NewFixture(t testing.TB) Fixture {
	t.Helper()

	home := t.TempDir()
	scanRoot := filepath.Join(home, "scan-root")
	stateDir := filepath.Join(scanRoot, ".session-state")
	storePath := filepath.Join(home, ".ralphglasses", "state.db")

	mustMkdirAll(t, filepath.Join(home, ".ralphglasses"))
	mustMkdirAll(t, stateDir)

	return Fixture{
		Home:      home,
		ScanRoot:  scanRoot,
		StateDir:  stateDir,
		StorePath: storePath,
	}
}

// ApplyHome sets HOME so bootstrap-backed command/runtime helpers resolve the fixture store.
func (f Fixture) ApplyHome(t testing.TB) {
	t.Helper()
	t.Setenv("HOME", f.Home)
}

// WriteExternalSession writes a session-shaped JSON payload to the tenant state dir.
func (f Fixture) WriteExternalSession(t testing.TB, tenantID, sessionID string, payload any) string {
	t.Helper()

	dir := filepath.Join(f.StateDir, tenantDir(tenantID))
	mustMkdirAll(t, dir)

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal external session %s: %v", sessionID, err)
	}

	path := filepath.Join(dir, sessionID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write external session %s: %v", sessionID, err)
	}
	return path
}

// SeedLegacySQLiteSchema creates the pre-tenant schema used by migration regression tests.
func SeedLegacySQLiteSchema(t testing.TB, dbPath string) {
	t.Helper()

	mustMkdirAll(t, filepath.Dir(dbPath))

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open(%s): %v", dbPath, err)
	}
	defer db.Close()

	const ddl = `
CREATE TABLE sessions (
	id TEXT PRIMARY KEY,
	provider TEXT NOT NULL DEFAULT 'codex',
	provider_session TEXT NOT NULL DEFAULT '',
	repo_path TEXT NOT NULL DEFAULT '',
	repo_name TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'launching',
	prompt TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	agent_name TEXT NOT NULL DEFAULT '',
	team_name TEXT NOT NULL DEFAULT '',
	budget_usd REAL NOT NULL DEFAULT 0,
	spend_usd REAL NOT NULL DEFAULT 0,
	turn_count INTEGER NOT NULL DEFAULT 0,
	max_turns INTEGER NOT NULL DEFAULT 0,
	error_msg TEXT NOT NULL DEFAULT '',
	exit_reason TEXT NOT NULL DEFAULT '',
	last_output TEXT NOT NULL DEFAULT '',
	last_event_type TEXT NOT NULL DEFAULT '',
	pid INTEGER NOT NULL DEFAULT 0,
	enhancement_source TEXT NOT NULL DEFAULT '',
	enhancement_pre_score INTEGER NOT NULL DEFAULT 0,
	cost_history TEXT NOT NULL DEFAULT '[]',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	ended_at DATETIME
);
CREATE TABLE loop_runs (
	id TEXT PRIMARY KEY,
	repo_path TEXT NOT NULL DEFAULT '',
	repo_name TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'pending',
	profile TEXT NOT NULL DEFAULT '{}',
	iterations TEXT NOT NULL DEFAULT '[]',
	last_error TEXT NOT NULL DEFAULT '',
	paused INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deadline DATETIME
);
CREATE TABLE cost_ledger (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT,
	loop_id TEXT,
	provider TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	spend_usd REAL NOT NULL DEFAULT 0,
	turn_count INTEGER NOT NULL DEFAULT 0,
	elapsed_sec REAL NOT NULL DEFAULT 0,
	recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE recovery_ops (
	id TEXT PRIMARY KEY,
	severity TEXT NOT NULL DEFAULT 'none',
	status TEXT NOT NULL DEFAULT 'detected',
	total_sessions INTEGER NOT NULL DEFAULT 0,
	alive_count INTEGER NOT NULL DEFAULT 0,
	dead_count INTEGER NOT NULL DEFAULT 0,
	resumed_count INTEGER NOT NULL DEFAULT 0,
	failed_count INTEGER NOT NULL DEFAULT 0,
	total_cost_usd REAL NOT NULL DEFAULT 0,
	budget_cap_usd REAL NOT NULL DEFAULT 0,
	trigger_source TEXT NOT NULL DEFAULT '',
	decision_id TEXT NOT NULL DEFAULT '',
	error_msg TEXT NOT NULL DEFAULT '',
	detected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME,
	completed_at DATETIME
);
CREATE TABLE recovery_actions (
	id TEXT PRIMARY KEY,
	recovery_op_id TEXT NOT NULL,
	claude_session_id TEXT NOT NULL DEFAULT '',
	ralph_session_id TEXT NOT NULL DEFAULT '',
	repo_path TEXT NOT NULL DEFAULT '',
	repo_name TEXT NOT NULL DEFAULT '',
	priority INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'pending',
	cost_usd REAL NOT NULL DEFAULT 0,
	error_msg TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME,
	completed_at DATETIME
);
`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("seed legacy sqlite schema: %v", err)
	}
}

func tenantDir(tenantID string) string {
	if tenantID == "" {
		return "_default"
	}
	return tenantID
}

func mustMkdirAll(t testing.TB, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
