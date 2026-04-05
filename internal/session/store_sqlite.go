package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
CREATE TABLE IF NOT EXISTS sessions (
	id               TEXT PRIMARY KEY,
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
`
	_, err := s.db.Exec(ddl)
	return err
}

func (s *SQLiteStore) SaveSession(ctx context.Context, sess *Session) error {
	if sess == nil || sess.ID == "" {
		return fmt.Errorf("save session: nil session or empty ID")
	}

	costJSON, _ := json.Marshal(sess.CostHistory)
	if costJSON == nil {
		costJSON = []byte("[]")
	}

	var endedAt *string
	if sess.EndedAt != nil {
		t := sess.EndedAt.Format(time.RFC3339)
		endedAt = &t
	}

	const query = `
INSERT INTO sessions (
	id, provider, provider_session, repo_path, repo_name, status, prompt, model,
	agent_name, team_name, budget_usd, spend_usd, turn_count, max_turns,
	error_msg, exit_reason, last_output, last_event_type, pid,
	enhancement_source, enhancement_pre_score, cost_history,
	created_at, updated_at, ended_at
) VALUES (
	?, ?, ?, ?, ?, ?, ?, ?,
	?, ?, ?, ?, ?, ?,
	?, ?, ?, ?, ?,
	?, ?, ?,
	?, ?, ?
)
ON CONFLICT(id) DO UPDATE SET
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
		sess.ID, string(sess.Provider), sess.ProviderSessionID,
		sess.RepoPath, sess.RepoName, string(sess.Status),
		sess.Prompt, sess.Model,
		sess.AgentName, sess.TeamName,
		sess.BudgetUSD, sess.SpentUSD, sess.TurnCount, sess.MaxTurns,
		sess.Error, sess.ExitReason, sess.LastOutput, sess.LastEventType,
		sess.Pid,
		sess.EnhancementSource, sess.EnhancementPreScore,
		string(costJSON),
		sess.LaunchedAt.Format(time.RFC3339),
		sess.LastActivity.Format(time.RFC3339),
		endedAt,
	)
	if err != nil {
		return fmt.Errorf("save session %s: %w", sess.ID, err)
	}
	return nil
}

func (s *SQLiteStore) GetSession(ctx context.Context, id string) (*Session, error) {
	const query = `
SELECT id, provider, provider_session, repo_path, repo_name, status, prompt, model,
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
	query := "SELECT id, provider, provider_session, repo_path, repo_name, status, prompt, model, agent_name, team_name, budget_usd, spend_usd, turn_count, max_turns, error_msg, exit_reason, last_output, last_event_type, pid, enhancement_source, enhancement_pre_score, cost_history, created_at, updated_at, ended_at FROM sessions WHERE 1=1"
	var args []any

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

func (s *SQLiteStore) AggregateSpend(ctx context.Context, repo string) (float64, error) {
	query := "SELECT COALESCE(SUM(spend_usd), 0) FROM sessions"
	var args []any
	if repo != "" {
		query += " WHERE repo_path = ?"
		args = append(args, repo)
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
		provider        string
		status          string
		createdAt       string
		updatedAt       string
		endedAt         *string
		costHistoryJSON string
	)

	err := sc.Scan(
		&sess.ID, &provider, &sess.ProviderSessionID,
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
	id, repo_path, repo_name, status, profile, iterations,
	last_error, paused, created_at, updated_at, deadline
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	repo_path=excluded.repo_path, repo_name=excluded.repo_name,
	status=excluded.status, profile=excluded.profile, iterations=excluded.iterations,
	last_error=excluded.last_error, paused=excluded.paused,
	updated_at=excluded.updated_at, deadline=excluded.deadline
`
	_, err = s.db.ExecContext(ctx, query,
		run.ID, run.RepoPath, run.RepoName, run.Status,
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
SELECT id, repo_path, repo_name, status, profile, iterations,
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
	query := `SELECT id, repo_path, repo_name, status, profile, iterations,
		last_error, paused, created_at, updated_at, deadline
		FROM loop_runs WHERE 1=1`
	var args []any

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
		profileJSON string
		iterJSON    string
		paused      int
		createdAt   string
		updatedAt   string
		deadline    *string
	)

	err := sc.Scan(
		&run.ID, &run.RepoPath, &run.RepoName, &run.Status,
		&profileJSON, &iterJSON,
		&run.LastError, &paused,
		&createdAt, &updatedAt, &deadline,
	)
	if err != nil {
		return nil, err
	}

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

	const query = `
INSERT INTO cost_ledger (session_id, loop_id, provider, model, spend_usd, turn_count, elapsed_sec, recorded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`
	recordedAt := entry.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now()
	}

	res, err := s.db.ExecContext(ctx, query,
		entry.SessionID, entry.LoopID, entry.Provider, entry.Model,
		entry.SpendUSD, entry.TurnCount, entry.ElapsedSec,
		recordedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("record cost: %w", err)
	}
	entry.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLiteStore) AggregateCostByProvider(ctx context.Context, since time.Time) (map[string]float64, error) {
	const query = `SELECT provider, COALESCE(SUM(spend_usd), 0) FROM cost_ledger WHERE recorded_at >= ? GROUP BY provider`
	rows, err := s.db.QueryContext(ctx, query, since.Format(time.RFC3339))
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
