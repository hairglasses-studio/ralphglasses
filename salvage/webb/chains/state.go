package chains

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// StateStore defines the interface for chain execution persistence
type StateStore interface {
	SaveExecution(exec *ChainExecution) error
	GetExecution(id string) (*ChainExecution, error)
	ListExecutions(chainName string, limit int) ([]*ChainExecution, error)
	SaveCheckpoint(checkpoint *Checkpoint) error
	GetLatestCheckpoint(executionID string) (*Checkpoint, error)
	GetMetrics(chainName string) (*ChainMetrics, error)
	UpdateMetrics(chainName string, duration time.Duration, success bool) error
	Close() error
}

// SQLiteStateStore implements StateStore using SQLite
type SQLiteStateStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteStateStore creates a new SQLite-backed state store
func NewSQLiteStateStore(dbPath string) (*SQLiteStateStore, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &SQLiteStateStore{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStateStore) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS executions (
		id TEXT PRIMARY KEY,
		chain_name TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		completed_at DATETIME,
		current_step TEXT,
		input_json TEXT,
		variables_json TEXT,
		step_results_json TEXT,
		error TEXT,
		triggered_by TEXT,
		parent_exec_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_executions_chain_name ON executions(chain_name);
	CREATE INDEX IF NOT EXISTS idx_executions_status ON executions(status);
	CREATE INDEX IF NOT EXISTS idx_executions_started_at ON executions(started_at);

	CREATE TABLE IF NOT EXISTS checkpoints (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		execution_id TEXT NOT NULL,
		chain_name TEXT NOT NULL,
		step_id TEXT NOT NULL,
		variables_json TEXT,
		step_results_json TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (execution_id) REFERENCES executions(id)
	);

	CREATE INDEX IF NOT EXISTS idx_checkpoints_execution_id ON checkpoints(execution_id);

	CREATE TABLE IF NOT EXISTS metrics (
		chain_name TEXT PRIMARY KEY,
		total_executions INTEGER DEFAULT 0,
		success_count INTEGER DEFAULT 0,
		failure_count INTEGER DEFAULT 0,
		total_duration_ns INTEGER DEFAULT 0,
		last_executed_at DATETIME,
		last_status TEXT
	);
	`

	_, err := s.db.Exec(schema)
	return err
}

// SaveExecution saves or updates an execution
func (s *SQLiteStateStore) SaveExecution(exec *ChainExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	inputJSON, _ := json.Marshal(exec.Input)
	varsJSON, _ := json.Marshal(exec.Variables)
	resultsJSON, _ := json.Marshal(exec.StepResults)

	var completedAt *string
	if exec.CompletedAt != nil {
		t := exec.CompletedAt.Format(time.RFC3339)
		completedAt = &t
	}

	_, err := s.db.Exec(`
		INSERT INTO executions (id, chain_name, status, started_at, completed_at, current_step,
			input_json, variables_json, step_results_json, error, triggered_by, parent_exec_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			completed_at = excluded.completed_at,
			current_step = excluded.current_step,
			variables_json = excluded.variables_json,
			step_results_json = excluded.step_results_json,
			error = excluded.error
	`,
		exec.ID, exec.ChainName, exec.Status, exec.StartedAt.Format(time.RFC3339),
		completedAt, exec.CurrentStep, string(inputJSON), string(varsJSON),
		string(resultsJSON), exec.Error, exec.TriggeredBy, exec.ParentExecID)

	return err
}

// GetExecution retrieves an execution by ID
func (s *SQLiteStateStore) GetExecution(id string) (*ChainExecution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRow(`
		SELECT id, chain_name, status, started_at, completed_at, current_step,
			input_json, variables_json, step_results_json, error, triggered_by, parent_exec_id
		FROM executions WHERE id = ?
	`, id)

	exec := &ChainExecution{}
	var startedAt, completedAt sql.NullString
	var inputJSON, varsJSON, resultsJSON string
	var currentStep, execError, triggeredBy, parentExecID sql.NullString

	err := row.Scan(&exec.ID, &exec.ChainName, &exec.Status, &startedAt, &completedAt,
		&currentStep, &inputJSON, &varsJSON, &resultsJSON, &execError, &triggeredBy, &parentExecID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("execution %s not found", id)
	}
	if err != nil {
		return nil, err
	}

	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		exec.StartedAt = t
	}
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		exec.CompletedAt = &t
	}
	if currentStep.Valid {
		exec.CurrentStep = currentStep.String
	}
	if execError.Valid {
		exec.Error = execError.String
	}
	if triggeredBy.Valid {
		exec.TriggeredBy = triggeredBy.String
	}
	if parentExecID.Valid {
		exec.ParentExecID = parentExecID.String
	}

	json.Unmarshal([]byte(inputJSON), &exec.Input)
	json.Unmarshal([]byte(varsJSON), &exec.Variables)
	json.Unmarshal([]byte(resultsJSON), &exec.StepResults)

	return exec, nil
}

// ListExecutions lists executions for a chain
func (s *SQLiteStateStore) ListExecutions(chainName string, limit int) ([]*ChainExecution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, chain_name, status, started_at, completed_at, error, triggered_by
		FROM executions
	`
	args := []interface{}{}

	if chainName != "" {
		query += " WHERE chain_name = ?"
		args = append(args, chainName)
	}

	query += " ORDER BY started_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []*ChainExecution
	for rows.Next() {
		exec := &ChainExecution{}
		var startedAt, completedAt sql.NullString
		var execError, triggeredBy sql.NullString

		if err := rows.Scan(&exec.ID, &exec.ChainName, &exec.Status, &startedAt,
			&completedAt, &execError, &triggeredBy); err != nil {
			continue
		}

		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339, startedAt.String)
			exec.StartedAt = t
		}
		if completedAt.Valid {
			t, _ := time.Parse(time.RFC3339, completedAt.String)
			exec.CompletedAt = &t
		}
		if execError.Valid {
			exec.Error = execError.String
		}
		if triggeredBy.Valid {
			exec.TriggeredBy = triggeredBy.String
		}

		executions = append(executions, exec)
	}

	return executions, nil
}

// SaveCheckpoint saves an execution checkpoint
func (s *SQLiteStateStore) SaveCheckpoint(checkpoint *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	varsJSON, _ := json.Marshal(checkpoint.Variables)
	resultsJSON, _ := json.Marshal(checkpoint.StepResults)

	_, err := s.db.Exec(`
		INSERT INTO checkpoints (execution_id, chain_name, step_id, variables_json, step_results_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, checkpoint.ExecutionID, checkpoint.ChainName, checkpoint.StepID,
		string(varsJSON), string(resultsJSON), checkpoint.CreatedAt.Format(time.RFC3339))

	return err
}

// GetLatestCheckpoint gets the most recent checkpoint for an execution
func (s *SQLiteStateStore) GetLatestCheckpoint(executionID string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRow(`
		SELECT execution_id, chain_name, step_id, variables_json, step_results_json, created_at
		FROM checkpoints
		WHERE execution_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, executionID)

	checkpoint := &Checkpoint{}
	var varsJSON, resultsJSON string
	var createdAt string

	err := row.Scan(&checkpoint.ExecutionID, &checkpoint.ChainName, &checkpoint.StepID,
		&varsJSON, &resultsJSON, &createdAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no checkpoint found for execution %s", executionID)
	}
	if err != nil {
		return nil, err
	}

	checkpoint.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	json.Unmarshal([]byte(varsJSON), &checkpoint.Variables)
	json.Unmarshal([]byte(resultsJSON), &checkpoint.StepResults)

	return checkpoint, nil
}

// GetMetrics gets execution metrics for a chain
func (s *SQLiteStateStore) GetMetrics(chainName string) (*ChainMetrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRow(`
		SELECT chain_name, total_executions, success_count, failure_count,
			total_duration_ns, last_executed_at, last_status
		FROM metrics WHERE chain_name = ?
	`, chainName)

	metrics := &ChainMetrics{ChainName: chainName}
	var totalDurationNs int64
	var lastExecutedAt sql.NullString
	var lastStatus sql.NullString

	err := row.Scan(&metrics.ChainName, &metrics.TotalExecutions, &metrics.SuccessCount,
		&metrics.FailureCount, &totalDurationNs, &lastExecutedAt, &lastStatus)
	if err == sql.ErrNoRows {
		return metrics, nil // Return empty metrics
	}
	if err != nil {
		return nil, err
	}

	// Calculate average duration in milliseconds
	if metrics.TotalExecutions > 0 {
		metrics.AvgDurationMs = float64(totalDurationNs) / float64(metrics.TotalExecutions) / 1e6
	}
	if lastExecutedAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastExecutedAt.String)
		metrics.LastExecutedAt = &t
	}
	// Map last_status to LastSuccessAt or LastFailureAt based on status
	if lastStatus.Valid && lastExecutedAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastExecutedAt.String)
		if ExecutionStatus(lastStatus.String) == StatusCompleted {
			metrics.LastSuccessAt = &t
		} else if ExecutionStatus(lastStatus.String) == StatusFailed {
			metrics.LastFailureAt = &t
		}
	}

	return metrics, nil
}

// UpdateMetrics updates execution metrics for a chain
func (s *SQLiteStateStore) UpdateMetrics(chainName string, duration time.Duration, success bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := StatusCompleted
	if !success {
		status = StatusFailed
	}

	_, err := s.db.Exec(`
		INSERT INTO metrics (chain_name, total_executions, success_count, failure_count,
			total_duration_ns, last_executed_at, last_status)
		VALUES (?, 1, ?, ?, ?, ?, ?)
		ON CONFLICT(chain_name) DO UPDATE SET
			total_executions = total_executions + 1,
			success_count = success_count + ?,
			failure_count = failure_count + ?,
			total_duration_ns = total_duration_ns + ?,
			last_executed_at = excluded.last_executed_at,
			last_status = excluded.last_status
	`,
		chainName,
		boolToInt(success), boolToInt(!success), duration.Nanoseconds(),
		time.Now().Format(time.RFC3339), status,
		boolToInt(success), boolToInt(!success), duration.Nanoseconds())

	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Close closes the database connection
func (s *SQLiteStateStore) Close() error {
	return s.db.Close()
}

// MemoryStateStore implements StateStore using in-memory storage (for testing)
type MemoryStateStore struct {
	mu          sync.RWMutex
	executions  map[string]*ChainExecution
	checkpoints map[string][]*Checkpoint
	metrics     map[string]*ChainMetrics
}

// NewMemoryStateStore creates a new in-memory state store
func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{
		executions:  make(map[string]*ChainExecution),
		checkpoints: make(map[string][]*Checkpoint),
		metrics:     make(map[string]*ChainMetrics),
	}
}

func (s *MemoryStateStore) SaveExecution(exec *ChainExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions[exec.ID] = exec
	return nil
}

func (s *MemoryStateStore) GetExecution(id string) (*ChainExecution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	exec, exists := s.executions[id]
	if !exists {
		return nil, fmt.Errorf("execution %s not found", id)
	}
	return exec, nil
}

func (s *MemoryStateStore) ListExecutions(chainName string, limit int) ([]*ChainExecution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var executions []*ChainExecution
	for _, exec := range s.executions {
		if chainName == "" || exec.ChainName == chainName {
			executions = append(executions, exec)
		}
	}

	if limit > 0 && len(executions) > limit {
		executions = executions[:limit]
	}
	return executions, nil
}

func (s *MemoryStateStore) SaveCheckpoint(checkpoint *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkpoints[checkpoint.ExecutionID] = append(s.checkpoints[checkpoint.ExecutionID], checkpoint)
	return nil
}

func (s *MemoryStateStore) GetLatestCheckpoint(executionID string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	checkpoints := s.checkpoints[executionID]
	if len(checkpoints) == 0 {
		return nil, fmt.Errorf("no checkpoint found")
	}
	return checkpoints[len(checkpoints)-1], nil
}

func (s *MemoryStateStore) GetMetrics(chainName string) (*ChainMetrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, exists := s.metrics[chainName]
	if !exists {
		return &ChainMetrics{ChainName: chainName}, nil
	}
	return m, nil
}

func (s *MemoryStateStore) UpdateMetrics(chainName string, duration time.Duration, success bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, exists := s.metrics[chainName]
	if !exists {
		m = &ChainMetrics{ChainName: chainName}
		s.metrics[chainName] = m
	}

	m.TotalExecutions++
	if success {
		m.SuccessCount++
	} else {
		m.FailureCount++
	}
	now := time.Now()
	m.LastExecutedAt = &now

	return nil
}

func (s *MemoryStateStore) Close() error {
	return nil
}
