package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store backed by a SQLite database.
// Uses modernc.org/sqlite (pure Go, no CGO required).
type SQLiteStore struct {
	db   *sql.DB
	path string
}

// NewSQLiteStore opens (or creates) a SQLite database at the given path
// and runs auto-migration to ensure the schema exists.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("sqlite store: mkdir %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite store: open %s: %w", dbPath, err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite store: enable WAL: %w", err)
	}

	store := &SQLiteStore{db: db, path: dbPath}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite store: migrate: %w", err)
	}
	return store, nil
}

// migrate creates tables if they don't exist.
func (s *SQLiteStore) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS tenants (
	id TEXT PRIMARY KEY,
	display_name TEXT NOT NULL DEFAULT '',
	allowed_repo_roots TEXT NOT NULL DEFAULT '[]',
	budget_cap_usd REAL NOT NULL DEFAULT 0,
	trigger_token_hash TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
	id               TEXT PRIMARY KEY,
	tenant_id        TEXT NOT NULL DEFAULT '_default',
	provider         TEXT NOT NULL DEFAULT 'codex',
	provider_session TEXT NOT NULL DEFAULT '',
	repo_path        TEXT NOT NULL DEFAULT '',
	repo_name        TEXT NOT NULL DEFAULT '',
	status           TEXT NOT NULL DEFAULT 'launching',
	prompt           TEXT NOT NULL DEFAULT '',
	model            TEXT NOT NULL DEFAULT '',
	agent_name       TEXT NOT NULL DEFAULT '',
	team_name        TEXT NOT NULL DEFAULT '',
	budget_usd       REAL NOT NULL DEFAULT 0,
	spend_usd        REAL NOT NULL DEFAULT 0,
	turn_count       INTEGER NOT NULL DEFAULT 0,
	max_turns        INTEGER NOT NULL DEFAULT 0,
	error_msg        TEXT NOT NULL DEFAULT '',
	exit_reason      TEXT NOT NULL DEFAULT '',
	last_output      TEXT NOT NULL DEFAULT '',
	last_event_type  TEXT NOT NULL DEFAULT '',
	pid              INTEGER NOT NULL DEFAULT 0,
	enhancement_source    TEXT NOT NULL DEFAULT '',
	enhancement_pre_score INTEGER NOT NULL DEFAULT 0,
	cost_history     TEXT NOT NULL DEFAULT '[]',
	created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	ended_at         DATETIME
);

CREATE INDEX IF NOT EXISTS idx_sessions_repo ON sessions(repo_path);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_repo_name ON sessions(repo_name);

CREATE TABLE IF NOT EXISTS loop_runs (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL DEFAULT '_default',
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

CREATE INDEX IF NOT EXISTS idx_loop_runs_repo ON loop_runs(repo_path);
CREATE INDEX IF NOT EXISTS idx_loop_runs_status ON loop_runs(status);

CREATE TABLE IF NOT EXISTS cost_ledger (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id TEXT NOT NULL DEFAULT '_default',
	session_id TEXT,
	loop_id TEXT,
	provider TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	spend_usd REAL NOT NULL DEFAULT 0,
	turn_count INTEGER NOT NULL DEFAULT 0,
	elapsed_sec REAL NOT NULL DEFAULT 0,
	recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_cost_ledger_session ON cost_ledger(session_id);
CREATE INDEX IF NOT EXISTS idx_cost_ledger_loop ON cost_ledger(loop_id);
CREATE INDEX IF NOT EXISTS idx_cost_ledger_provider ON cost_ledger(provider);

CREATE TABLE IF NOT EXISTS recovery_ops (
	id             TEXT PRIMARY KEY,
	tenant_id      TEXT NOT NULL DEFAULT '_default',
	severity       TEXT NOT NULL DEFAULT 'none',
	status         TEXT NOT NULL DEFAULT 'detected',
	total_sessions INTEGER NOT NULL DEFAULT 0,
	alive_count    INTEGER NOT NULL DEFAULT 0,
	dead_count     INTEGER NOT NULL DEFAULT 0,
	resumed_count  INTEGER NOT NULL DEFAULT 0,
	failed_count   INTEGER NOT NULL DEFAULT 0,
	total_cost_usd REAL NOT NULL DEFAULT 0,
	budget_cap_usd REAL NOT NULL DEFAULT 0,
	trigger_source TEXT NOT NULL DEFAULT '',
	decision_id    TEXT NOT NULL DEFAULT '',
	error_msg      TEXT NOT NULL DEFAULT '',
	detected_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at     DATETIME,
	completed_at   DATETIME
);

CREATE INDEX IF NOT EXISTS idx_recovery_ops_status ON recovery_ops(status);
CREATE INDEX IF NOT EXISTS idx_recovery_ops_detected ON recovery_ops(detected_at);

CREATE TABLE IF NOT EXISTS recovery_actions (
	id                TEXT PRIMARY KEY,
	tenant_id         TEXT NOT NULL DEFAULT '_default',
	recovery_op_id    TEXT NOT NULL,
	claude_session_id TEXT NOT NULL DEFAULT '',
	ralph_session_id  TEXT NOT NULL DEFAULT '',
	repo_path         TEXT NOT NULL DEFAULT '',
	repo_name         TEXT NOT NULL DEFAULT '',
	priority          INTEGER NOT NULL DEFAULT 0,
	status            TEXT NOT NULL DEFAULT 'pending',
	cost_usd          REAL NOT NULL DEFAULT 0,
	error_msg         TEXT NOT NULL DEFAULT '',
	created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at        DATETIME,
	completed_at      DATETIME,
	FOREIGN KEY (recovery_op_id) REFERENCES recovery_ops(id)
);

CREATE INDEX IF NOT EXISTS idx_recovery_actions_op ON recovery_actions(recovery_op_id);
CREATE INDEX IF NOT EXISTS idx_recovery_actions_status ON recovery_actions(status);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return err
	}
	for _, change := range []struct {
		table      string
		columnName string
		definition string
	}{
		{"sessions", "tenant_id", "tenant_id TEXT NOT NULL DEFAULT '_default'"},
		{"loop_runs", "tenant_id", "tenant_id TEXT NOT NULL DEFAULT '_default'"},
		{"cost_ledger", "tenant_id", "tenant_id TEXT NOT NULL DEFAULT '_default'"},
		{"recovery_ops", "tenant_id", "tenant_id TEXT NOT NULL DEFAULT '_default'"},
		{"recovery_actions", "tenant_id", "tenant_id TEXT NOT NULL DEFAULT '_default'"},
	} {
		if err := s.ensureColumn(change.table, change.columnName, change.definition); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`
CREATE INDEX IF NOT EXISTS idx_sessions_tenant_repo ON sessions(tenant_id, repo_path);
CREATE INDEX IF NOT EXISTS idx_sessions_tenant_status ON sessions(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_loop_runs_tenant_repo ON loop_runs(tenant_id, repo_path);
CREATE INDEX IF NOT EXISTS idx_loop_runs_tenant_status ON loop_runs(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_cost_ledger_tenant_recorded ON cost_ledger(tenant_id, recorded_at);
CREATE INDEX IF NOT EXISTS idx_recovery_ops_tenant_status ON recovery_ops(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_recovery_actions_tenant_status ON recovery_actions(tenant_id, status);
`); err != nil {
		return err
	}
	return s.seedDefaultTenant()
}

func (s *SQLiteStore) ensureColumn(table, columnName, definition string) error {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notNull   int
			defaultV  sql.NullString
			primaryPK int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultV, &primaryPK); err != nil {
			return fmt.Errorf("scan pragma table_info(%s): %w", table, err)
		}
		if name == columnName {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate pragma table_info(%s): %w", table, err)
	}

	if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, definition)); err != nil {
		return fmt.Errorf("alter table %s add column %s: %w", table, columnName, err)
	}
	return nil
}

func (s *SQLiteStore) seedDefaultTenant() error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
INSERT INTO tenants (id, display_name, allowed_repo_roots, budget_cap_usd, trigger_token_hash, created_at, updated_at)
VALUES (?, ?, '[]', 0, '', ?, ?)
ON CONFLICT(id) DO UPDATE SET
	display_name = excluded.display_name,
	updated_at = excluded.updated_at
`, DefaultTenantID, "Default", now, now)
	if err != nil {
		return fmt.Errorf("seed default tenant: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveSession(ctx context.Context, sess *Session) error {
	if sess == nil || sess.ID == "" {
		return fmt.Errorf("save session: nil session or empty ID")
	}
	snap := cloneSession(sess)
	if snap == nil {
		return fmt.Errorf("save session: nil session")
	}
	snap.TenantID = NormalizeTenantID(snap.TenantID)

	costJSON, _ := json.Marshal(snap.CostHistory)
	if costJSON == nil {
		costJSON = []byte("[]")
	}

	var endedAt *string
	if snap.EndedAt != nil {
		t := snap.EndedAt.Format(time.RFC3339)
		endedAt = &t
	}

	const query = `
INSERT INTO sessions (
	id, tenant_id, provider, provider_session, repo_path, repo_name, status, prompt, model,
	agent_name, team_name, budget_usd, spend_usd, turn_count, max_turns,
	error_msg, exit_reason, last_output, last_event_type, pid,
	enhancement_source, enhancement_pre_score, cost_history,
	created_at, updated_at, ended_at
) VALUES (
	?, ?, ?, ?, ?, ?, ?, ?, ?,
	?, ?, ?, ?, ?, ?,
	?, ?, ?, ?, ?,
	?, ?, ?,
	?, ?, ?
)
ON CONFLICT(id) DO UPDATE SET
	tenant_id=excluded.tenant_id,
	provider=excluded.provider, provider_session=excluded.provider_session,
	repo_path=excluded.repo_path, repo_name=excluded.repo_name,
	status=excluded.status, prompt=excluded.prompt, model=excluded.model,
	agent_name=excluded.agent_name, team_name=excluded.team_name,
	budget_usd=excluded.budget_usd, spend_usd=excluded.spend_usd,
	turn_count=excluded.turn_count, max_turns=excluded.max_turns,
	error_msg=excluded.error_msg, exit_reason=excluded.exit_reason,
	last_output=excluded.last_output, last_event_type=excluded.last_event_type,
	pid=excluded.pid,
	enhancement_source=excluded.enhancement_source,
	enhancement_pre_score=excluded.enhancement_pre_score,
	cost_history=excluded.cost_history,
	updated_at=excluded.updated_at, ended_at=excluded.ended_at
`
	_, err := s.db.ExecContext(ctx, query,
		snap.ID, snap.TenantID, string(snap.Provider), snap.ProviderSessionID,
		snap.RepoPath, snap.RepoName, string(snap.Status),
		snap.Prompt, snap.Model,
		snap.AgentName, snap.TeamName,
		snap.BudgetUSD, snap.SpentUSD, snap.TurnCount, snap.MaxTurns,
		snap.Error, snap.ExitReason, snap.LastOutput, snap.LastEventType,
		snap.Pid,
		snap.EnhancementSource, snap.EnhancementPreScore,
		string(costJSON),
		snap.LaunchedAt.Format(time.RFC3339),
		snap.LastActivity.Format(time.RFC3339),
		endedAt,
	)
	if err != nil {
		return fmt.Errorf("save session %s: %w", snap.ID, err)
	}
	return nil
}

func (s *SQLiteStore) GetSession(ctx context.Context, id string) (*Session, error) {
	const query = `
SELECT id, tenant_id, provider, provider_session, repo_path, repo_name, status, prompt, model,
	agent_name, team_name, budget_usd, spend_usd, turn_count, max_turns,
	error_msg, exit_reason, last_output, last_event_type, pid,
	enhancement_source, enhancement_pre_score, cost_history,
	created_at, updated_at, ended_at
FROM sessions WHERE id = ?
`
	row := s.db.QueryRowContext(ctx, query, id)
	sess, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session %s: %w", id, err)
	}
	return sess, nil
}

func (s *SQLiteStore) ListSessions(ctx context.Context, opts ListOpts) ([]*Session, error) {
	query := "SELECT id, tenant_id, provider, provider_session, repo_path, repo_name, status, prompt, model, agent_name, team_name, budget_usd, spend_usd, turn_count, max_turns, error_msg, exit_reason, last_output, last_event_type, pid, enhancement_source, enhancement_pre_score, cost_history, created_at, updated_at, ended_at FROM sessions WHERE 1=1"
	var args []any

	if opts.TenantID != "" {
		query += " AND tenant_id = ?"
		args = append(args, NormalizeTenantID(opts.TenantID))
	}
	if opts.RepoPath != "" {
		query += " AND repo_path = ?"
		args = append(args, opts.RepoPath)
	}
	if opts.RepoName != "" {
		query += " AND repo_name = ?"
		args = append(args, opts.RepoName)
	}
	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, string(opts.Status))
	}
	if !opts.Since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, opts.Since.Format("2006-01-02 15:04:05"))
	}
	if !opts.Until.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, opts.Until.Format("2006-01-02 15:04:05"))
	}

	query += " ORDER BY created_at DESC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var result []*Session
	for rows.Next() {
		sess, err := scanSessionRows(rows)
		if err != nil {
			return nil, fmt.Errorf("list sessions: scan: %w", err)
		}
		result = append(result, sess)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete session %s: %w", id, err)
	}
	return nil
}

func (s *SQLiteStore) UpdateSessionStatus(ctx context.Context, id string, status SessionStatus) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?",
		string(status), time.Now().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("update session status %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (s *SQLiteStore) AggregateSpend(ctx context.Context, tenantID, repo string) (float64, error) {
	query := "SELECT COALESCE(SUM(spend_usd), 0) FROM sessions"
	var args []any
	var filters []string
	if tenantID != "" {
		filters = append(filters, "tenant_id = ?")
		args = append(args, NormalizeTenantID(tenantID))
	}
	if repo != "" {
		filters = append(filters, "repo_path = ?")
		args = append(args, repo)
	}
	if len(filters) > 0 {
		query += " WHERE " + strings.Join(filters, " AND ")
	}

	var total float64
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("aggregate spend: %w", err)
	}
	return total, nil
}

func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// scanner is implemented by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanSessionFromScanner(sc scanner) (*Session, error) {
	var (
		sess            Session
		tenantID        string
		provider        string
		status          string
		createdAt       string
		updatedAt       string
		endedAt         *string
		costHistoryJSON string
	)

	err := sc.Scan(
		&sess.ID, &tenantID, &provider, &sess.ProviderSessionID,
		&sess.RepoPath, &sess.RepoName, &status,
		&sess.Prompt, &sess.Model,
		&sess.AgentName, &sess.TeamName,
		&sess.BudgetUSD, &sess.SpentUSD, &sess.TurnCount, &sess.MaxTurns,
		&sess.Error, &sess.ExitReason, &sess.LastOutput, &sess.LastEventType,
		&sess.Pid,
		&sess.EnhancementSource, &sess.EnhancementPreScore,
		&costHistoryJSON,
		&createdAt, &updatedAt, &endedAt,
	)
	if err != nil {
		return nil, err
	}

	sess.TenantID = NormalizeTenantID(tenantID)
	sess.Provider = Provider(provider)
	sess.Status = SessionStatus(status)

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		sess.LaunchedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		sess.LastActivity = t
	}
	if endedAt != nil {
		if t, err := time.Parse(time.RFC3339, *endedAt); err == nil {
			sess.EndedAt = &t
		}
	}

	if costHistoryJSON != "" && costHistoryJSON != "[]" {
		_ = json.Unmarshal([]byte(costHistoryJSON), &sess.CostHistory)
	}

	return &sess, nil
}

func scanSession(row *sql.Row) (*Session, error) {
	return scanSessionFromScanner(row)
}

func scanSessionRows(rows *sql.Rows) (*Session, error) {
	return scanSessionFromScanner(rows)
}

// ---------- LoopRun persistence ----------

func (s *SQLiteStore) SaveLoopRun(ctx context.Context, run *LoopRun) error {
	if run == nil || run.ID == "" {
		return fmt.Errorf("save loop run: nil run or empty ID")
	}
	run.TenantID = NormalizeTenantID(run.TenantID)

	profileJSON, err := json.Marshal(run.Profile)
	if err != nil {
		return fmt.Errorf("save loop run %s: marshal profile: %w", run.ID, err)
	}

	iterJSON, err := json.Marshal(run.Iterations)
	if err != nil {
		return fmt.Errorf("save loop run %s: marshal iterations: %w", run.ID, err)
	}

	var deadline *string
	if run.Deadline != nil {
		t := run.Deadline.Format(time.RFC3339)
		deadline = &t
	}

	paused := 0
	if run.Paused {
		paused = 1
	}

	const query = `
INSERT INTO loop_runs (
	id, tenant_id, repo_path, repo_name, status, profile, iterations,
	last_error, paused, created_at, updated_at, deadline
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	tenant_id=excluded.tenant_id,
	repo_path=excluded.repo_path, repo_name=excluded.repo_name,
	status=excluded.status, profile=excluded.profile, iterations=excluded.iterations,
	last_error=excluded.last_error, paused=excluded.paused,
	updated_at=excluded.updated_at, deadline=excluded.deadline
`
	_, err = s.db.ExecContext(ctx, query,
		run.ID, run.TenantID, run.RepoPath, run.RepoName, run.Status,
		string(profileJSON), string(iterJSON),
		run.LastError, paused,
		run.CreatedAt.Format(time.RFC3339),
		run.UpdatedAt.Format(time.RFC3339),
		deadline,
	)
	if err != nil {
		return fmt.Errorf("save loop run %s: %w", run.ID, err)
	}
	return nil
}

func (s *SQLiteStore) GetLoopRun(ctx context.Context, id string) (*LoopRun, error) {
	const query = `
SELECT id, tenant_id, repo_path, repo_name, status, profile, iterations,
	last_error, paused, created_at, updated_at, deadline
FROM loop_runs WHERE id = ?
`
	row := s.db.QueryRowContext(ctx, query, id)
	run, err := scanLoopRun(row)
	if err == sql.ErrNoRows {
		return nil, ErrLoopNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get loop run %s: %w", id, err)
	}
	return run, nil
}

func (s *SQLiteStore) ListLoopRuns(ctx context.Context, filter LoopRunFilter) ([]*LoopRun, error) {
	query := `SELECT id, tenant_id, repo_path, repo_name, status, profile, iterations,
		last_error, paused, created_at, updated_at, deadline
		FROM loop_runs WHERE 1=1`
	var args []any

	if filter.TenantID != "" {
		query += " AND tenant_id = ?"
		args = append(args, NormalizeTenantID(filter.TenantID))
	}
	if filter.RepoPath != "" {
		query += " AND repo_path = ?"
		args = append(args, filter.RepoPath)
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list loop runs: %w", err)
	}
	defer rows.Close()

	var result []*LoopRun
	for rows.Next() {
		run, err := scanLoopRunRows(rows)
		if err != nil {
			return nil, fmt.Errorf("list loop runs: scan: %w", err)
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) UpdateLoopRunStatus(ctx context.Context, id string, status string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE loop_runs SET status = ?, updated_at = ? WHERE id = ?",
		status, time.Now().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("update loop run status %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrLoopNotFound
	}
	return nil
}

func scanLoopRunFromScanner(sc scanner) (*LoopRun, error) {
	var (
		run         LoopRun
		tenantID    string
		profileJSON string
		iterJSON    string
		paused      int
		createdAt   string
		updatedAt   string
		deadline    *string
	)

	err := sc.Scan(
		&run.ID, &tenantID, &run.RepoPath, &run.RepoName, &run.Status,
		&profileJSON, &iterJSON,
		&run.LastError, &paused,
		&createdAt, &updatedAt, &deadline,
	)
	if err != nil {
		return nil, err
	}

	run.TenantID = NormalizeTenantID(tenantID)
	run.Paused = paused != 0

	if profileJSON != "" && profileJSON != "{}" {
		_ = json.Unmarshal([]byte(profileJSON), &run.Profile)
	}
	if iterJSON != "" && iterJSON != "[]" {
		_ = json.Unmarshal([]byte(iterJSON), &run.Iterations)
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		run.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		run.UpdatedAt = t
	}
	if deadline != nil {
		if t, err := time.Parse(time.RFC3339, *deadline); err == nil {
			run.Deadline = &t
		}
	}

	return &run, nil
}

func scanLoopRun(row *sql.Row) (*LoopRun, error) {
	return scanLoopRunFromScanner(row)
}

func scanLoopRunRows(rows *sql.Rows) (*LoopRun, error) {
	return scanLoopRunFromScanner(rows)
}

// ---------- Cost ledger persistence ----------

func (s *SQLiteStore) RecordCost(ctx context.Context, entry *CostEntry) error {
	if entry == nil {
		return fmt.Errorf("record cost: nil entry")
	}
	entry.TenantID = NormalizeTenantID(entry.TenantID)

	const query = `
INSERT INTO cost_ledger (tenant_id, session_id, loop_id, provider, model, spend_usd, turn_count, elapsed_sec, recorded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	recordedAt := entry.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now()
	}

	res, err := s.db.ExecContext(ctx, query,
		entry.TenantID, entry.SessionID, entry.LoopID, entry.Provider, entry.Model,
		entry.SpendUSD, entry.TurnCount, entry.ElapsedSec,
		recordedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("record cost: %w", err)
	}
	entry.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLiteStore) AggregateCostByProvider(ctx context.Context, tenantID string, since time.Time) (map[string]float64, error) {
	query := `SELECT provider, COALESCE(SUM(spend_usd), 0) FROM cost_ledger WHERE recorded_at >= ?`
	args := []any{since.Format(time.RFC3339)}
	if tenantID != "" {
		query += ` AND tenant_id = ?`
		args = append(args, NormalizeTenantID(tenantID))
	}
	query += ` GROUP BY provider`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("aggregate cost by provider: %w", err)
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var provider string
		var total float64
		if err := rows.Scan(&provider, &total); err != nil {
			return nil, fmt.Errorf("aggregate cost by provider: scan: %w", err)
		}
		result[provider] = total
	}
	return result, rows.Err()
}

// ---------- Recovery persistence ----------

func (s *SQLiteStore) SaveRecoveryOp(ctx context.Context, op *RecoveryOp) error {
	if op == nil || op.ID == "" {
		return fmt.Errorf("save recovery op: nil op or empty ID")
	}
	op.TenantID = NormalizeTenantID(op.TenantID)

	var startedAt, completedAt *string
	if op.StartedAt != nil {
		t := op.StartedAt.Format(time.RFC3339)
		startedAt = &t
	}
	if op.CompletedAt != nil {
		t := op.CompletedAt.Format(time.RFC3339)
		completedAt = &t
	}

	const query = `
INSERT INTO recovery_ops (
	id, tenant_id, severity, status, total_sessions, alive_count, dead_count,
	resumed_count, failed_count, total_cost_usd, budget_cap_usd,
	trigger_source, decision_id, error_msg, detected_at, started_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	tenant_id=excluded.tenant_id,
	severity=excluded.severity, status=excluded.status,
	total_sessions=excluded.total_sessions, alive_count=excluded.alive_count,
	dead_count=excluded.dead_count, resumed_count=excluded.resumed_count,
	failed_count=excluded.failed_count, total_cost_usd=excluded.total_cost_usd,
	budget_cap_usd=excluded.budget_cap_usd, trigger_source=excluded.trigger_source,
	decision_id=excluded.decision_id, error_msg=excluded.error_msg,
	started_at=excluded.started_at, completed_at=excluded.completed_at
`
	_, err := s.db.ExecContext(ctx, query,
		op.ID, op.TenantID, op.Severity, string(op.Status),
		op.TotalSessions, op.AliveCount, op.DeadCount,
		op.ResumedCount, op.FailedCount, op.TotalCostUSD, op.BudgetCapUSD,
		op.TriggerSource, op.DecisionID, op.ErrorMsg,
		op.DetectedAt.Format(time.RFC3339), startedAt, completedAt,
	)
	if err != nil {
		return fmt.Errorf("save recovery op %s: %w", op.ID, err)
	}
	return nil
}

func (s *SQLiteStore) GetRecoveryOp(ctx context.Context, id string) (*RecoveryOp, error) {
	const query = `
SELECT id, tenant_id, severity, status, total_sessions, alive_count, dead_count,
	resumed_count, failed_count, total_cost_usd, budget_cap_usd,
	trigger_source, decision_id, error_msg, detected_at, started_at, completed_at
FROM recovery_ops WHERE id = ?
`
	row := s.db.QueryRowContext(ctx, query, id)

	var op RecoveryOp
	var tenantID string
	var status string
	var detectedAt string
	var startedAt, completedAt sql.NullString

	err := row.Scan(
		&op.ID, &tenantID, &op.Severity, &status,
		&op.TotalSessions, &op.AliveCount, &op.DeadCount,
		&op.ResumedCount, &op.FailedCount, &op.TotalCostUSD, &op.BudgetCapUSD,
		&op.TriggerSource, &op.DecisionID, &op.ErrorMsg,
		&detectedAt, &startedAt, &completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrRecoveryOpNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get recovery op %s: %w", id, err)
	}

	op.TenantID = NormalizeTenantID(tenantID)
	op.Status = RecoveryOpStatus(status)
	op.DetectedAt, _ = time.Parse(time.RFC3339, detectedAt)
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		op.StartedAt = &t
	}
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		op.CompletedAt = &t
	}

	return &op, nil
}

func (s *SQLiteStore) ListRecoveryOps(ctx context.Context, filter RecoveryOpFilter) ([]*RecoveryOp, error) {
	query := `SELECT id, tenant_id, severity, status, total_sessions, alive_count, dead_count,
		resumed_count, failed_count, total_cost_usd, budget_cap_usd,
		trigger_source, decision_id, error_msg, detected_at, started_at, completed_at
	FROM recovery_ops WHERE 1=1`
	var args []any

	if filter.TenantID != "" {
		query += " AND tenant_id = ?"
		args = append(args, NormalizeTenantID(filter.TenantID))
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, string(filter.Status))
	}
	if !filter.Since.IsZero() {
		query += " AND detected_at >= ?"
		args = append(args, filter.Since.Format(time.RFC3339))
	}
	query += " ORDER BY detected_at DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list recovery ops: %w", err)
	}
	defer rows.Close()

	var result []*RecoveryOp
	for rows.Next() {
		var op RecoveryOp
		var tenantID, status, detectedAt string
		var startedAt, completedAt sql.NullString

		if err := rows.Scan(
			&op.ID, &tenantID, &op.Severity, &status,
			&op.TotalSessions, &op.AliveCount, &op.DeadCount,
			&op.ResumedCount, &op.FailedCount, &op.TotalCostUSD, &op.BudgetCapUSD,
			&op.TriggerSource, &op.DecisionID, &op.ErrorMsg,
			&detectedAt, &startedAt, &completedAt,
		); err != nil {
			return nil, fmt.Errorf("list recovery ops: scan: %w", err)
		}

		op.TenantID = NormalizeTenantID(tenantID)
		op.Status = RecoveryOpStatus(status)
		op.DetectedAt, _ = time.Parse(time.RFC3339, detectedAt)
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339, startedAt.String)
			op.StartedAt = &t
		}
		if completedAt.Valid {
			t, _ := time.Parse(time.RFC3339, completedAt.String)
			op.CompletedAt = &t
		}

		result = append(result, &op)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) SaveRecoveryAction(ctx context.Context, action *RecoveryAction) error {
	if action == nil || action.ID == "" {
		return fmt.Errorf("save recovery action: nil action or empty ID")
	}
	action.TenantID = NormalizeTenantID(action.TenantID)

	var startedAt, completedAt *string
	if action.StartedAt != nil {
		t := action.StartedAt.Format(time.RFC3339)
		startedAt = &t
	}
	if action.CompletedAt != nil {
		t := action.CompletedAt.Format(time.RFC3339)
		completedAt = &t
	}

	const query = `
INSERT INTO recovery_actions (
	id, tenant_id, recovery_op_id, claude_session_id, ralph_session_id,
	repo_path, repo_name, priority, status, cost_usd, error_msg,
	created_at, started_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	tenant_id=excluded.tenant_id,
	ralph_session_id=excluded.ralph_session_id,
	status=excluded.status, cost_usd=excluded.cost_usd,
	error_msg=excluded.error_msg,
	started_at=excluded.started_at, completed_at=excluded.completed_at
`
	_, err := s.db.ExecContext(ctx, query,
		action.ID, action.TenantID, action.RecoveryOpID, action.ClaudeSessionID, action.RalphSessionID,
		action.RepoPath, action.RepoName, action.Priority,
		string(action.Status), action.CostUSD, action.ErrorMsg,
		action.CreatedAt.Format(time.RFC3339), startedAt, completedAt,
	)
	if err != nil {
		return fmt.Errorf("save recovery action %s: %w", action.ID, err)
	}
	return nil
}

func (s *SQLiteStore) UpdateRecoveryActionStatus(ctx context.Context, id string, status RecoveryActionStatus, errMsg string) error {
	now := time.Now().Format(time.RFC3339)
	var query string
	var args []any

	switch status {
	case ActionExecuting:
		query = `UPDATE recovery_actions SET status = ?, error_msg = ?, started_at = ? WHERE id = ?`
		args = []any{string(status), errMsg, now, id}
	case ActionSucceeded, ActionFailed, ActionSkipped:
		query = `UPDATE recovery_actions SET status = ?, error_msg = ?, completed_at = ? WHERE id = ?`
		args = []any{string(status), errMsg, now, id}
	default:
		query = `UPDATE recovery_actions SET status = ?, error_msg = ? WHERE id = ?`
		args = []any{string(status), errMsg, id}
	}

	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update recovery action %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrRecoveryActionNotFound
	}
	return nil
}

func (s *SQLiteStore) SaveTenant(ctx context.Context, tenant *Tenant) error {
	if tenant == nil {
		return fmt.Errorf("save tenant: nil tenant")
	}
	cp := *tenant
	cp.Normalize()
	rootsJSON, err := json.Marshal(cp.AllowedRepoRoots)
	if err != nil {
		return fmt.Errorf("save tenant %s: marshal repo roots: %w", cp.ID, err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO tenants (id, display_name, allowed_repo_roots, budget_cap_usd, trigger_token_hash, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	display_name = excluded.display_name,
	allowed_repo_roots = excluded.allowed_repo_roots,
	budget_cap_usd = excluded.budget_cap_usd,
	trigger_token_hash = excluded.trigger_token_hash,
	updated_at = excluded.updated_at
`, cp.ID, cp.DisplayName, string(rootsJSON), cp.BudgetCapUSD, cp.TriggerTokenHash, cp.CreatedAt.Format(time.RFC3339), cp.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save tenant %s: %w", cp.ID, err)
	}
	return nil
}

func (s *SQLiteStore) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	id = NormalizeTenantID(id)
	row := s.db.QueryRowContext(ctx, `
SELECT id, display_name, allowed_repo_roots, budget_cap_usd, trigger_token_hash, created_at, updated_at
FROM tenants WHERE id = ?
`, id)
	var tenant Tenant
	var rootsJSON string
	var createdAt, updatedAt string
	if err := row.Scan(&tenant.ID, &tenant.DisplayName, &rootsJSON, &tenant.BudgetCapUSD, &tenant.TriggerTokenHash, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("get tenant %s: %w", id, err)
	}
	if rootsJSON != "" && rootsJSON != "[]" {
		_ = json.Unmarshal([]byte(rootsJSON), &tenant.AllowedRepoRoots)
	}
	tenant.AllowedRepoRoots = normalizeRepoRoots(tenant.AllowedRepoRoots)
	tenant.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	tenant.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	tenant.ID = NormalizeTenantID(tenant.ID)
	return &tenant, nil
}

func (s *SQLiteStore) ListTenants(ctx context.Context) ([]*Tenant, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, display_name, allowed_repo_roots, budget_cap_usd, trigger_token_hash, created_at, updated_at
FROM tenants ORDER BY id
`)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	var result []*Tenant
	for rows.Next() {
		var tenant Tenant
		var rootsJSON string
		var createdAt, updatedAt string
		if err := rows.Scan(&tenant.ID, &tenant.DisplayName, &rootsJSON, &tenant.BudgetCapUSD, &tenant.TriggerTokenHash, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("list tenants: scan: %w", err)
		}
		if rootsJSON != "" && rootsJSON != "[]" {
			_ = json.Unmarshal([]byte(rootsJSON), &tenant.AllowedRepoRoots)
		}
		tenant.AllowedRepoRoots = normalizeRepoRoots(tenant.AllowedRepoRoots)
		tenant.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		tenant.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		tenant.ID = NormalizeTenantID(tenant.ID)
		result = append(result, &tenant)
	}
	return result, rows.Err()
}
