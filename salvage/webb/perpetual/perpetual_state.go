// Package clients provides client implementations for external services.
package clients

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// PerpetualStateStore handles state persistence for the perpetual engine
type PerpetualStateStore struct {
	db     *sql.DB
	dbPath string
}

// NewPerpetualStateStore creates a new state store
func NewPerpetualStateStore() (*PerpetualStateStore, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(configPath, "perpetual.db")

	// Open SQLite with WAL mode for crash recovery
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &PerpetualStateStore{
		db:     db,
		dbPath: dbPath,
	}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the database tables
func (s *PerpetualStateStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS engine_state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		state TEXT NOT NULL,
		phase TEXT NOT NULL,
		current_task_ids TEXT NOT NULL DEFAULT '[]',
		config JSON NOT NULL,
		metrics JSON NOT NULL,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS proposals (
		id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		evidence TEXT NOT NULL DEFAULT '[]',
		impact INTEGER NOT NULL DEFAULT 50,
		effort TEXT NOT NULL DEFAULT 'medium',
		score REAL NOT NULL DEFAULT 0,
		content_hash TEXT NOT NULL,
		discovered_at TIMESTAMP NOT NULL,
		status TEXT NOT NULL DEFAULT 'queued',
		dev_task_id TEXT,
		pr_number INTEGER,
		failure_count INTEGER NOT NULL DEFAULT 0,
		last_failure TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_proposals_status ON proposals(status);
	CREATE INDEX IF NOT EXISTS idx_proposals_content_hash ON proposals(content_hash);
	CREATE INDEX IF NOT EXISTS idx_proposals_source ON proposals(source);
	CREATE INDEX IF NOT EXISTS idx_proposals_score ON proposals(score DESC);

	CREATE TABLE IF NOT EXISTS proposal_outcomes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		proposal_id TEXT NOT NULL,
		source TEXT NOT NULL,
		outcome TEXT NOT NULL,
		pr_number INTEGER,
		merge_time_hours REAL,
		review_comments INTEGER,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (proposal_id) REFERENCES proposals(id)
	);

	CREATE TABLE IF NOT EXISTS source_weights (
		source TEXT PRIMARY KEY,
		weight REAL NOT NULL DEFAULT 1.0,
		success_count INTEGER NOT NULL DEFAULT 0,
		failure_count INTEGER NOT NULL DEFAULT 0,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS consensus_votes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		task_name TEXT NOT NULL,
		provider TEXT NOT NULL,
		vote TEXT NOT NULL,
		confidence REAL NOT NULL DEFAULT 0.7,
		reasoning TEXT,
		suggestions TEXT,
		approval_rate REAL,
		consensus_reached BOOLEAN DEFAULT 0,
		voted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(task_id, provider)
	);
	CREATE INDEX IF NOT EXISTS idx_consensus_votes_task ON consensus_votes(task_id);
	CREATE INDEX IF NOT EXISTS idx_consensus_votes_provider ON consensus_votes(provider);
	CREATE INDEX IF NOT EXISTS idx_consensus_votes_voted_at ON consensus_votes(voted_at);

	CREATE TABLE IF NOT EXISTS test_suites (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		source_file TEXT NOT NULL,
		test_file TEXT NOT NULL,
		package_name TEXT NOT NULL,
		test_count INTEGER NOT NULL DEFAULT 0,
		passing_count INTEGER NOT NULL DEFAULT 0,
		failing_count INTEGER NOT NULL DEFAULT 0,
		coverage REAL NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending',
		heal_attempts INTEGER NOT NULL DEFAULT 0,
		llm_provider TEXT,
		test_code TEXT,
		error_message TEXT,
		generated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_run_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_test_suites_task ON test_suites(task_id);
	CREATE INDEX IF NOT EXISTS idx_test_suites_status ON test_suites(status);

	-- v24.0: Cross-proposal intelligence
	CREATE TABLE IF NOT EXISTS proposal_relationships (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		proposal_id TEXT NOT NULL,
		related_proposal_id TEXT NOT NULL,
		relationship_type TEXT NOT NULL, -- 'depends_on', 'conflicts_with', 'bundles_with', 'similar_to'
		confidence REAL NOT NULL DEFAULT 0.7,
		detected_by TEXT, -- 'file_overlap', 'semantic_similarity', 'source_correlation'
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (proposal_id) REFERENCES proposals(id),
		FOREIGN KEY (related_proposal_id) REFERENCES proposals(id),
		UNIQUE(proposal_id, related_proposal_id, relationship_type)
	);
	CREATE INDEX IF NOT EXISTS idx_proposal_rel_proposal ON proposal_relationships(proposal_id);
	CREATE INDEX IF NOT EXISTS idx_proposal_rel_related ON proposal_relationships(related_proposal_id);
	CREATE INDEX IF NOT EXISTS idx_proposal_rel_type ON proposal_relationships(relationship_type);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Save persists the engine state
func (s *PerpetualStateStore) Save(state *PerpetualEngineState) error {
	configJSON, err := json.Marshal(state.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	metricsJSON, err := json.Marshal(state.Metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	taskIDsJSON, err := json.Marshal(state.CurrentTaskIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal task IDs: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO engine_state (id, state, phase, current_task_ids, config, metrics, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			state = excluded.state,
			phase = excluded.phase,
			current_task_ids = excluded.current_task_ids,
			config = excluded.config,
			metrics = excluded.metrics,
			updated_at = excluded.updated_at
	`, state.State, state.Phase, taskIDsJSON, configJSON, metricsJSON, time.Now())

	if err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	state.LastPersistedAt = time.Now()
	return nil
}

// Load retrieves the engine state
func (s *PerpetualStateStore) Load() (*PerpetualEngineState, error) {
	var stateStr, phaseStr string
	var taskIDsJSON, configJSON, metricsJSON []byte
	var updatedAt time.Time

	err := s.db.QueryRow(`
		SELECT state, phase, current_task_ids, config, metrics, updated_at
		FROM engine_state
		WHERE id = 1
	`).Scan(&stateStr, &phaseStr, &taskIDsJSON, &configJSON, &metricsJSON, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no saved state found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	state := &PerpetualEngineState{
		State:           PerpetualState(stateStr),
		Phase:           PerpetualPhase(phaseStr),
		LastPersistedAt: updatedAt,
	}

	if err := json.Unmarshal(taskIDsJSON, &state.CurrentTaskIDs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task IDs: %w", err)
	}

	state.Config = &PerpetualConfig{}
	if err := json.Unmarshal(configJSON, state.Config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	state.Metrics = &PerpetualMetrics{}
	if err := json.Unmarshal(metricsJSON, state.Metrics); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metrics: %w", err)
	}

	return state, nil
}

// SaveProposal persists a proposal
func (s *PerpetualStateStore) SaveProposal(p *PerpetualProposal) error {
	evidenceJSON, err := json.Marshal(p.Evidence)
	if err != nil {
		return fmt.Errorf("failed to marshal evidence: %w", err)
	}

	var lastFailure interface{}
	if !p.LastFailure.IsZero() {
		lastFailure = p.LastFailure
	}

	_, err = s.db.Exec(`
		INSERT INTO proposals (id, source, title, description, evidence, impact, effort, score, content_hash, discovered_at, status, dev_task_id, pr_number, failure_count, last_failure)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			score = excluded.score,
			dev_task_id = excluded.dev_task_id,
			pr_number = excluded.pr_number,
			failure_count = excluded.failure_count,
			last_failure = excluded.last_failure,
			updated_at = CURRENT_TIMESTAMP
	`, p.ID, p.Source, p.Title, p.Description, evidenceJSON, p.Impact, p.Effort, p.Score, p.ContentHash, p.DiscoveredAt, p.Status, p.DevTaskID, p.PRNumber, p.FailureCount, lastFailure)

	return err
}

// LoadQueuedProposals retrieves all queued proposals
func (s *PerpetualStateStore) LoadQueuedProposals() ([]*PerpetualProposal, error) {
	rows, err := s.db.Query(`
		SELECT id, source, title, description, evidence, impact, effort, score, content_hash, discovered_at, status, dev_task_id, pr_number, failure_count, last_failure
		FROM proposals
		WHERE status = 'queued'
		ORDER BY score DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query proposals: %w", err)
	}
	defer rows.Close()

	var proposals []*PerpetualProposal
	for rows.Next() {
		p := &PerpetualProposal{}
		var evidenceJSON []byte
		var devTaskID, lastFailure sql.NullString
		var prNumber sql.NullInt64

		err := rows.Scan(&p.ID, &p.Source, &p.Title, &p.Description, &evidenceJSON, &p.Impact, &p.Effort, &p.Score, &p.ContentHash, &p.DiscoveredAt, &p.Status, &devTaskID, &prNumber, &p.FailureCount, &lastFailure)
		if err != nil {
			return nil, fmt.Errorf("failed to scan proposal: %w", err)
		}

		if err := json.Unmarshal(evidenceJSON, &p.Evidence); err != nil {
			p.Evidence = []string{}
		}

		if devTaskID.Valid {
			p.DevTaskID = devTaskID.String
		}
		if prNumber.Valid {
			p.PRNumber = int(prNumber.Int64)
		}
		if lastFailure.Valid {
			p.LastFailure, _ = time.Parse(time.RFC3339, lastFailure.String)
		}

		proposals = append(proposals, p)
	}

	return proposals, nil
}

// RecordOutcome records a proposal outcome for learning
func (s *PerpetualStateStore) RecordOutcome(proposalID string, source FeatureSource, outcome string, prNumber int, mergeTimeHours float64, reviewComments int) error {
	_, err := s.db.Exec(`
		INSERT INTO proposal_outcomes (proposal_id, source, outcome, pr_number, merge_time_hours, review_comments)
		VALUES (?, ?, ?, ?, ?, ?)
	`, proposalID, source, outcome, prNumber, mergeTimeHours, reviewComments)
	return err
}

// GetSourceStats returns success/failure stats for learning
func (s *PerpetualStateStore) GetSourceStats() (map[FeatureSource]*SourceStats, error) {
	rows, err := s.db.Query(`
		SELECT source,
			   COUNT(CASE WHEN outcome = 'merged' THEN 1 END) as successes,
			   COUNT(CASE WHEN outcome IN ('rejected', 'failed') THEN 1 END) as failures
		FROM proposal_outcomes
		GROUP BY source
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[FeatureSource]*SourceStats)
	for rows.Next() {
		var source string
		var successes, failures int
		if err := rows.Scan(&source, &successes, &failures); err != nil {
			return nil, err
		}
		stats[FeatureSource(source)] = &SourceStats{
			Successes: successes,
			Failures:  failures,
		}
	}

	return stats, nil
}

// SourceStats tracks source performance
type SourceStats struct {
	Successes int
	Failures  int
}

// UpdateSourceWeight updates a source weight (for learning)
func (s *PerpetualStateStore) UpdateSourceWeight(source FeatureSource, weight float64, successes, failures int) error {
	_, err := s.db.Exec(`
		INSERT INTO source_weights (source, weight, success_count, failure_count, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(source) DO UPDATE SET
			weight = excluded.weight,
			success_count = excluded.success_count,
			failure_count = excluded.failure_count,
			updated_at = excluded.updated_at
	`, source, weight, successes, failures)
	return err
}

// GetSourceWeights retrieves learned source weights
func (s *PerpetualStateStore) GetSourceWeights() (map[FeatureSource]float64, error) {
	rows, err := s.db.Query(`SELECT source, weight FROM source_weights`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	weights := make(map[FeatureSource]float64)
	for rows.Next() {
		var source string
		var weight float64
		if err := rows.Scan(&source, &weight); err != nil {
			return nil, err
		}
		weights[FeatureSource(source)] = weight
	}

	return weights, nil
}

// CheckContentHashExists checks if a proposal with the given hash exists
func (s *PerpetualStateStore) CheckContentHashExists(contentHash string, windowDays int) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM proposals
		WHERE content_hash = ?
		AND discovered_at > datetime('now', '-' || ? || ' days')
	`, contentHash, windowDays).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetProposalStats returns statistics about proposals
func (s *PerpetualStateStore) GetProposalStats() (*ProposalStats, error) {
	stats := &ProposalStats{
		BySource: make(map[FeatureSource]int),
		ByStatus: make(map[string]int),
	}

	// Total count
	err := s.db.QueryRow(`SELECT COUNT(*) FROM proposals`).Scan(&stats.Total)
	if err != nil {
		return nil, err
	}

	// By source
	rows, err := s.db.Query(`SELECT source, COUNT(*) FROM proposals GROUP BY source`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.BySource[FeatureSource(source)] = count
	}
	rows.Close()

	// By status
	rows, err = s.db.Query(`SELECT status, COUNT(*) FROM proposals GROUP BY status`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.ByStatus[status] = count
	}
	rows.Close()

	return stats, nil
}

// ProposalStats contains proposal statistics
type ProposalStats struct {
	Total    int
	BySource map[FeatureSource]int
	ByStatus map[string]int
}

// Close closes the database connection
func (s *PerpetualStateStore) Close() error {
	return s.db.Close()
}

// GetRecentOutcomes retrieves recent proposal outcomes for analysis
func (s *PerpetualStateStore) GetRecentOutcomes(limit int) ([]OutcomeRecord, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT proposal_id, source, outcome, pr_number, merge_time_hours, review_comments, created_at
		FROM proposal_outcomes
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outcomes []OutcomeRecord
	for rows.Next() {
		var o OutcomeRecord
		var prNumber sql.NullInt64
		var mergeTime, reviewComments sql.NullFloat64
		var source, outcome string

		if err := rows.Scan(&o.ProposalID, &source, &outcome, &prNumber, &mergeTime, &reviewComments, &o.CreatedAt); err != nil {
			return nil, err
		}

		o.Source = FeatureSource(source)
		o.Outcome = OutcomeType(outcome)
		if prNumber.Valid {
			o.PRNumber = int(prNumber.Int64)
		}
		if mergeTime.Valid {
			o.MergeTimeHours = mergeTime.Float64
		}
		if reviewComments.Valid {
			o.ReviewComments = int(reviewComments.Float64)
		}

		outcomes = append(outcomes, o)
	}

	return outcomes, nil
}

// GetOutcomesBySource retrieves outcomes filtered by source
func (s *PerpetualStateStore) GetOutcomesBySource(source FeatureSource, limit int) ([]OutcomeRecord, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT proposal_id, source, outcome, pr_number, merge_time_hours, review_comments, created_at
		FROM proposal_outcomes
		WHERE source = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, source, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outcomes []OutcomeRecord
	for rows.Next() {
		var o OutcomeRecord
		var prNumber sql.NullInt64
		var mergeTime, reviewComments sql.NullFloat64
		var sourceStr, outcomeStr string

		if err := rows.Scan(&o.ProposalID, &sourceStr, &outcomeStr, &prNumber, &mergeTime, &reviewComments, &o.CreatedAt); err != nil {
			return nil, err
		}

		o.Source = FeatureSource(sourceStr)
		o.Outcome = OutcomeType(outcomeStr)
		if prNumber.Valid {
			o.PRNumber = int(prNumber.Int64)
		}
		if mergeTime.Valid {
			o.MergeTimeHours = mergeTime.Float64
		}
		if reviewComments.Valid {
			o.ReviewComments = int(reviewComments.Float64)
		}

		outcomes = append(outcomes, o)
	}

	return outcomes, nil
}

// GetOutcomesInPeriod retrieves outcomes within a time period for trend analysis
func (s *PerpetualStateStore) GetOutcomesInPeriod(days int) ([]OutcomeRecord, error) {
	rows, err := s.db.Query(`
		SELECT proposal_id, source, outcome, pr_number, merge_time_hours, review_comments, created_at
		FROM proposal_outcomes
		WHERE created_at > datetime('now', '-' || ? || ' days')
		ORDER BY created_at DESC
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outcomes []OutcomeRecord
	for rows.Next() {
		var o OutcomeRecord
		var prNumber sql.NullInt64
		var mergeTime, reviewComments sql.NullFloat64
		var source, outcome string

		if err := rows.Scan(&o.ProposalID, &source, &outcome, &prNumber, &mergeTime, &reviewComments, &o.CreatedAt); err != nil {
			return nil, err
		}

		o.Source = FeatureSource(source)
		o.Outcome = OutcomeType(outcome)
		if prNumber.Valid {
			o.PRNumber = int(prNumber.Int64)
		}
		if mergeTime.Valid {
			o.MergeTimeHours = mergeTime.Float64
		}
		if reviewComments.Valid {
			o.ReviewComments = int(reviewComments.Float64)
		}

		outcomes = append(outcomes, o)
	}

	return outcomes, nil
}

// GetSourceStatsInPeriod returns success/failure stats for a specific time period
func (s *PerpetualStateStore) GetSourceStatsInPeriod(days int) (map[FeatureSource]*SourceStats, error) {
	rows, err := s.db.Query(`
		SELECT source,
			   COUNT(CASE WHEN outcome = 'merged' THEN 1 END) as successes,
			   COUNT(CASE WHEN outcome IN ('rejected', 'failed') THEN 1 END) as failures
		FROM proposal_outcomes
		WHERE created_at > datetime('now', '-' || ? || ' days')
		GROUP BY source
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[FeatureSource]*SourceStats)
	for rows.Next() {
		var source string
		var successes, failures int
		if err := rows.Scan(&source, &successes, &failures); err != nil {
			return nil, err
		}
		stats[FeatureSource(source)] = &SourceStats{
			Successes: successes,
			Failures:  failures,
		}
	}

	return stats, nil
}

// GetLearningMetrics returns metrics useful for monitoring the learning system
func (s *PerpetualStateStore) GetLearningMetrics() (*LearningMetrics, error) {
	metrics := &LearningMetrics{
		SourceWeights:       make(map[FeatureSource]float64),
		SourceSuccessRates:  make(map[FeatureSource]float64),
		SourceSampleCounts:  make(map[FeatureSource]int),
	}

	// Get weights
	weights, err := s.GetSourceWeights()
	if err == nil {
		metrics.SourceWeights = weights
	}

	// Get stats
	stats, err := s.GetSourceStats()
	if err == nil {
		for source, st := range stats {
			total := st.Successes + st.Failures
			metrics.SourceSampleCounts[source] = total
			if total > 0 {
				metrics.SourceSuccessRates[source] = float64(st.Successes) / float64(total)
			}
		}
	}

	// Get total outcomes
	err = s.db.QueryRow(`SELECT COUNT(*) FROM proposal_outcomes`).Scan(&metrics.TotalOutcomes)
	if err != nil {
		metrics.TotalOutcomes = 0
	}

	// Get outcomes in last 7 days
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM proposal_outcomes
		WHERE created_at > datetime('now', '-7 days')
	`).Scan(&metrics.OutcomesLast7Days)
	if err != nil {
		metrics.OutcomesLast7Days = 0
	}

	// Get overall success rate
	var totalSuccesses, totalFailures int
	err = s.db.QueryRow(`
		SELECT
			COUNT(CASE WHEN outcome = 'merged' THEN 1 END),
			COUNT(CASE WHEN outcome IN ('rejected', 'failed') THEN 1 END)
		FROM proposal_outcomes
	`).Scan(&totalSuccesses, &totalFailures)
	if err == nil && (totalSuccesses+totalFailures) > 0 {
		metrics.OverallSuccessRate = float64(totalSuccesses) / float64(totalSuccesses+totalFailures)
	}

	return metrics, nil
}

// LearningMetrics contains metrics about the learning system
type LearningMetrics struct {
	TotalOutcomes       int                        `json:"total_outcomes"`
	OutcomesLast7Days   int                        `json:"outcomes_last_7_days"`
	OverallSuccessRate  float64                    `json:"overall_success_rate"`
	SourceWeights       map[FeatureSource]float64  `json:"source_weights"`
	SourceSuccessRates  map[FeatureSource]float64  `json:"source_success_rates"`
	SourceSampleCounts  map[FeatureSource]int      `json:"source_sample_counts"`
}

// BackupToVault creates a backup of the state in the Obsidian vault
func (s *PerpetualStateStore) BackupToVault() error {
	vaultPath := os.Getenv("OBSIDIAN_VAULT_PATH")
	if vaultPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		vaultPath = filepath.Join(home, "obsidian-vaults", "webb", "webb-dev", "perpetual-backups")
	} else {
		vaultPath = filepath.Join(vaultPath, "webb-dev", "perpetual-backups")
	}

	if err := os.MkdirAll(vaultPath, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Load current state
	state, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Load proposals
	proposals, err := s.LoadQueuedProposals()
	if err != nil {
		proposals = []*PerpetualProposal{}
	}

	// Create backup data
	backup := struct {
		State     *PerpetualEngineState `json:"state"`
		Proposals []*PerpetualProposal  `json:"proposals"`
		BackupAt  time.Time             `json:"backup_at"`
	}{
		State:     state,
		Proposals: proposals,
		BackupAt:  time.Now(),
	}

	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal backup: %w", err)
	}

	filename := fmt.Sprintf("backup-%s.json", time.Now().Format("2006-01-02-150405"))
	backupPath := filepath.Join(vaultPath, filename)

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write backup: %w", err)
	}

	return nil
}

// ReadPerpetualStatusFromDB reads the perpetual engine status directly from SQLite
// This can be called from any process without needing the engine running
func ReadPerpetualStatusFromDB() (*PerpetualEngineState, []*PerpetualProposal, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, nil, err
	}

	dbPath := filepath.Join(configPath, "perpetual.db")

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("perpetual database not found at %s", dbPath)
	}

	// Open read-only
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Read engine state
	var stateStr, phaseStr string
	var taskIDsJSON, configJSON, metricsJSON []byte
	var updatedAt time.Time

	err = db.QueryRow(`
		SELECT state, phase, current_task_ids, config, metrics, updated_at
		FROM engine_state WHERE id = 1
	`).Scan(&stateStr, &phaseStr, &taskIDsJSON, &configJSON, &metricsJSON, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil, fmt.Errorf("no engine state found")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load state: %w", err)
	}

	state := &PerpetualEngineState{
		State:           PerpetualState(stateStr),
		Phase:           PerpetualPhase(phaseStr),
		LastPersistedAt: updatedAt,
	}

	if err := json.Unmarshal(taskIDsJSON, &state.CurrentTaskIDs); err != nil {
		state.CurrentTaskIDs = []string{}
	}

	state.Config = &PerpetualConfig{}
	if err := json.Unmarshal(configJSON, state.Config); err != nil {
		state.Config = DefaultPerpetualConfig()
	}

	state.Metrics = &PerpetualMetrics{}
	if err := json.Unmarshal(metricsJSON, state.Metrics); err != nil {
		state.Metrics = &PerpetualMetrics{}
	}

	// Read queued proposals
	rows, err := db.Query(`
		SELECT id, source, title, description, evidence, impact, effort, score, content_hash, discovered_at, status, dev_task_id, pr_number, failure_count
		FROM proposals
		WHERE status IN ('queued', 'implementing')
		ORDER BY score DESC
		LIMIT 50
	`)
	if err != nil {
		return state, nil, nil // Return state even if proposals fail
	}
	defer rows.Close()

	var proposals []*PerpetualProposal
	for rows.Next() {
		p := &PerpetualProposal{}
		var evidenceJSON []byte
		var devTaskID sql.NullString
		var prNumber sql.NullInt64

		err := rows.Scan(&p.ID, &p.Source, &p.Title, &p.Description, &evidenceJSON, &p.Impact, &p.Effort, &p.Score, &p.ContentHash, &p.DiscoveredAt, &p.Status, &devTaskID, &prNumber, &p.FailureCount)
		if err != nil {
			continue
		}

		if err := json.Unmarshal(evidenceJSON, &p.Evidence); err != nil {
			p.Evidence = []string{}
		}
		if devTaskID.Valid {
			p.DevTaskID = devTaskID.String
		}
		if prNumber.Valid {
			p.PRNumber = int(prNumber.Int64)
		}

		proposals = append(proposals, p)
	}

	return state, proposals, nil
}

// ConsensusMetrics holds consensus voting statistics
type ConsensusMetrics struct {
	TotalVotes         int                        `json:"total_votes"`
	VotesLast7Days     int                        `json:"votes_last_7_days"`
	OverallApprovalRate float64                   `json:"overall_approval_rate"`
	ConsensusReachedRate float64                  `json:"consensus_reached_rate"`
	TasksVotedOn       int                        `json:"tasks_voted_on"`
	ProviderStats      map[string]*ProviderVoteStats `json:"provider_stats"`
	RecentVotes        []*ConsensusVoteRecord     `json:"recent_votes"`
}

// ProviderVoteStats holds voting statistics for a single provider
type ProviderVoteStats struct {
	Provider      string  `json:"provider"`
	TotalVotes    int     `json:"total_votes"`
	Approvals     int     `json:"approvals"`
	Rejections    int     `json:"rejections"`
	Abstentions   int     `json:"abstentions"`
	ApprovalRate  float64 `json:"approval_rate"`
	AvgConfidence float64 `json:"avg_confidence"`
}

// ConsensusVoteRecord holds a single consensus vote record
type ConsensusVoteRecord struct {
	TaskID           string    `json:"task_id"`
	TaskName         string    `json:"task_name"`
	Provider         string    `json:"provider"`
	Vote             string    `json:"vote"`
	Confidence       float64   `json:"confidence"`
	Reasoning        string    `json:"reasoning,omitempty"`
	ApprovalRate     float64   `json:"approval_rate"`
	ConsensusReached bool      `json:"consensus_reached"`
	VotedAt          time.Time `json:"voted_at"`
}

// RecordConsensusVote saves a consensus vote to the database
func (s *PerpetualStateStore) RecordConsensusVote(taskID, taskName, provider, vote string, confidence float64, reasoning string, suggestions []string, approvalRate float64, consensusReached bool) error {
	suggestionsJSON, _ := json.Marshal(suggestions)

	_, err := s.db.Exec(`
		INSERT INTO consensus_votes (task_id, task_name, provider, vote, confidence, reasoning, suggestions, approval_rate, consensus_reached, voted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, provider) DO UPDATE SET
			vote = excluded.vote,
			confidence = excluded.confidence,
			reasoning = excluded.reasoning,
			suggestions = excluded.suggestions,
			approval_rate = excluded.approval_rate,
			consensus_reached = excluded.consensus_reached,
			voted_at = excluded.voted_at
	`, taskID, taskName, provider, vote, confidence, reasoning, string(suggestionsJSON), approvalRate, consensusReached, time.Now())

	return err
}

// GetConsensusMetrics returns comprehensive consensus voting metrics
func (s *PerpetualStateStore) GetConsensusMetrics(limit int) (*ConsensusMetrics, error) {
	metrics := &ConsensusMetrics{
		ProviderStats: make(map[string]*ProviderVoteStats),
		RecentVotes:   make([]*ConsensusVoteRecord, 0),
	}

	// Get overall stats
	err := s.db.QueryRow(`
		SELECT COUNT(*), COUNT(DISTINCT task_id)
		FROM consensus_votes
	`).Scan(&metrics.TotalVotes, &metrics.TasksVotedOn)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get vote counts: %w", err)
	}

	// Get votes in last 7 days
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM consensus_votes WHERE voted_at > ?
	`, sevenDaysAgo).Scan(&metrics.VotesLast7Days)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get recent vote count: %w", err)
	}

	// Get approval rate
	var approveCount int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM consensus_votes WHERE vote = 'approve'
	`).Scan(&approveCount)
	if err == nil && metrics.TotalVotes > 0 {
		metrics.OverallApprovalRate = float64(approveCount) / float64(metrics.TotalVotes)
	}

	// Get consensus reached rate (per task)
	var tasksWithConsensus int
	err = s.db.QueryRow(`
		SELECT COUNT(DISTINCT task_id) FROM consensus_votes WHERE consensus_reached = 1
	`).Scan(&tasksWithConsensus)
	if err == nil && metrics.TasksVotedOn > 0 {
		metrics.ConsensusReachedRate = float64(tasksWithConsensus) / float64(metrics.TasksVotedOn)
	}

	// Get per-provider stats
	rows, err := s.db.Query(`
		SELECT
			provider,
			COUNT(*) as total_votes,
			SUM(CASE WHEN vote = 'approve' THEN 1 ELSE 0 END) as approvals,
			SUM(CASE WHEN vote = 'reject' THEN 1 ELSE 0 END) as rejections,
			SUM(CASE WHEN vote = 'abstain' THEN 1 ELSE 0 END) as abstentions,
			AVG(confidence) as avg_confidence
		FROM consensus_votes
		GROUP BY provider
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			stat := &ProviderVoteStats{}
			err := rows.Scan(&stat.Provider, &stat.TotalVotes, &stat.Approvals, &stat.Rejections, &stat.Abstentions, &stat.AvgConfidence)
			if err != nil {
				continue
			}
			if stat.TotalVotes > 0 {
				stat.ApprovalRate = float64(stat.Approvals) / float64(stat.TotalVotes)
			}
			metrics.ProviderStats[stat.Provider] = stat
		}
	}

	// Get recent votes
	if limit <= 0 {
		limit = 20
	}
	rows, err = s.db.Query(`
		SELECT task_id, task_name, provider, vote, confidence, reasoning, approval_rate, consensus_reached, voted_at
		FROM consensus_votes
		ORDER BY voted_at DESC
		LIMIT ?
	`, limit)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			vote := &ConsensusVoteRecord{}
			var reasoning sql.NullString
			var approvalRate sql.NullFloat64
			err := rows.Scan(&vote.TaskID, &vote.TaskName, &vote.Provider, &vote.Vote, &vote.Confidence, &reasoning, &approvalRate, &vote.ConsensusReached, &vote.VotedAt)
			if err != nil {
				continue
			}
			if reasoning.Valid {
				vote.Reasoning = reasoning.String
			}
			if approvalRate.Valid {
				vote.ApprovalRate = approvalRate.Float64
			}
			metrics.RecentVotes = append(metrics.RecentVotes, vote)
		}
	}

	return metrics, nil
}

// GetConsensusVotesForTask returns all votes for a specific task
func (s *PerpetualStateStore) GetConsensusVotesForTask(taskID string) ([]*ConsensusVoteRecord, error) {
	rows, err := s.db.Query(`
		SELECT task_id, task_name, provider, vote, confidence, reasoning, approval_rate, consensus_reached, voted_at
		FROM consensus_votes
		WHERE task_id = ?
		ORDER BY voted_at DESC
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to query votes: %w", err)
	}
	defer rows.Close()

	var votes []*ConsensusVoteRecord
	for rows.Next() {
		vote := &ConsensusVoteRecord{}
		var reasoning sql.NullString
		var approvalRate sql.NullFloat64
		err := rows.Scan(&vote.TaskID, &vote.TaskName, &vote.Provider, &vote.Vote, &vote.Confidence, &reasoning, &approvalRate, &vote.ConsensusReached, &vote.VotedAt)
		if err != nil {
			continue
		}
		if reasoning.Valid {
			vote.Reasoning = reasoning.String
		}
		if approvalRate.Valid {
			vote.ApprovalRate = approvalRate.Float64
		}
		votes = append(votes, vote)
	}

	return votes, nil
}

// TestSuiteRecord represents a test suite in the database
type TestSuiteRecord struct {
	ID           int       `json:"id"`
	TaskID       string    `json:"task_id"`
	SourceFile   string    `json:"source_file"`
	TestFile     string    `json:"test_file"`
	PackageName  string    `json:"package_name"`
	TestCount    int       `json:"test_count"`
	PassingCount int       `json:"passing_count"`
	FailingCount int       `json:"failing_count"`
	Coverage     float64   `json:"coverage"`
	Status       string    `json:"status"`
	HealAttempts int       `json:"heal_attempts"`
	LLMProvider  string    `json:"llm_provider"`
	TestCode     string    `json:"test_code,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	GeneratedAt  time.Time `json:"generated_at"`
	LastRunAt    time.Time `json:"last_run_at,omitempty"`
}

// TestingMetrics contains test generation metrics
type TestingMetrics struct {
	TotalSuites     int     `json:"total_suites"`
	PassingSuites   int     `json:"passing_suites"`
	FailingSuites   int     `json:"failing_suites"`
	HealedSuites    int     `json:"healed_suites"`
	TotalTests      int     `json:"total_tests"`
	PassingTests    int     `json:"passing_tests"`
	AvgCoverage     float64 `json:"avg_coverage"`
	AvgHealAttempts float64 `json:"avg_heal_attempts"`
	HealSuccessRate float64 `json:"heal_success_rate"`
	RecentSuites    []*TestSuiteRecord `json:"recent_suites"`
}

// SaveTestSuite saves a test suite to the database
func (s *PerpetualStateStore) SaveTestSuite(taskID, sourceFile, testFile, packageName string, testCount, passingCount, failingCount int, coverage float64, status string, healAttempts int, llmProvider, testCode, errorMessage string) error {
	_, err := s.db.Exec(`
		INSERT INTO test_suites (task_id, source_file, test_file, package_name, test_count, passing_count, failing_count, coverage, status, heal_attempts, llm_provider, test_code, error_message, generated_at, last_run_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			test_count = excluded.test_count,
			passing_count = excluded.passing_count,
			failing_count = excluded.failing_count,
			coverage = excluded.coverage,
			status = excluded.status,
			heal_attempts = excluded.heal_attempts,
			test_code = excluded.test_code,
			error_message = excluded.error_message,
			last_run_at = excluded.last_run_at
	`, taskID, sourceFile, testFile, packageName, testCount, passingCount, failingCount, coverage, status, healAttempts, llmProvider, testCode, errorMessage, time.Now(), time.Now())

	if err != nil {
		return fmt.Errorf("failed to save test suite: %w", err)
	}
	return nil
}

// GetTestingMetrics returns test generation metrics
func (s *PerpetualStateStore) GetTestingMetrics(limit int) (*TestingMetrics, error) {
	metrics := &TestingMetrics{}

	// Get suite counts by status
	err := s.db.QueryRow(`SELECT COUNT(*) FROM test_suites`).Scan(&metrics.TotalSuites)
	if err != nil {
		return nil, fmt.Errorf("failed to count test suites: %w", err)
	}

	err = s.db.QueryRow(`SELECT COUNT(*) FROM test_suites WHERE status = 'passing'`).Scan(&metrics.PassingSuites)
	if err != nil {
		metrics.PassingSuites = 0
	}

	err = s.db.QueryRow(`SELECT COUNT(*) FROM test_suites WHERE status = 'failing' OR status = 'failed'`).Scan(&metrics.FailingSuites)
	if err != nil {
		metrics.FailingSuites = 0
	}

	err = s.db.QueryRow(`SELECT COUNT(*) FROM test_suites WHERE status = 'healed'`).Scan(&metrics.HealedSuites)
	if err != nil {
		metrics.HealedSuites = 0
	}

	// Get aggregate metrics
	err = s.db.QueryRow(`
		SELECT
			COALESCE(SUM(test_count), 0),
			COALESCE(SUM(passing_count), 0),
			COALESCE(AVG(coverage), 0),
			COALESCE(AVG(heal_attempts), 0)
		FROM test_suites
	`).Scan(&metrics.TotalTests, &metrics.PassingTests, &metrics.AvgCoverage, &metrics.AvgHealAttempts)
	if err != nil {
		// Use defaults
	}

	// Calculate heal success rate
	if metrics.HealedSuites+metrics.FailingSuites > 0 {
		metrics.HealSuccessRate = float64(metrics.HealedSuites) / float64(metrics.HealedSuites+metrics.FailingSuites) * 100
	}

	// Get recent suites
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT id, task_id, source_file, test_file, package_name, test_count, passing_count, failing_count,
		       coverage, status, heal_attempts, COALESCE(llm_provider, ''), COALESCE(error_message, ''), generated_at, COALESCE(last_run_at, generated_at)
		FROM test_suites
		ORDER BY generated_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return metrics, nil // Return metrics without recent suites
	}
	defer rows.Close()

	for rows.Next() {
		suite := &TestSuiteRecord{}
		err := rows.Scan(&suite.ID, &suite.TaskID, &suite.SourceFile, &suite.TestFile, &suite.PackageName,
			&suite.TestCount, &suite.PassingCount, &suite.FailingCount, &suite.Coverage, &suite.Status,
			&suite.HealAttempts, &suite.LLMProvider, &suite.ErrorMessage, &suite.GeneratedAt, &suite.LastRunAt)
		if err != nil {
			continue
		}
		metrics.RecentSuites = append(metrics.RecentSuites, suite)
	}

	return metrics, nil
}

// GetTestSuitesForTask returns all test suites for a specific task
func (s *PerpetualStateStore) GetTestSuitesForTask(taskID string) ([]*TestSuiteRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, task_id, source_file, test_file, package_name, test_count, passing_count, failing_count,
		       coverage, status, heal_attempts, COALESCE(llm_provider, ''), COALESCE(test_code, ''), COALESCE(error_message, ''), generated_at, COALESCE(last_run_at, generated_at)
		FROM test_suites
		WHERE task_id = ?
		ORDER BY generated_at DESC
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test suites: %w", err)
	}
	defer rows.Close()

	var suites []*TestSuiteRecord
	for rows.Next() {
		suite := &TestSuiteRecord{}
		err := rows.Scan(&suite.ID, &suite.TaskID, &suite.SourceFile, &suite.TestFile, &suite.PackageName,
			&suite.TestCount, &suite.PassingCount, &suite.FailingCount, &suite.Coverage, &suite.Status,
			&suite.HealAttempts, &suite.LLMProvider, &suite.TestCode, &suite.ErrorMessage, &suite.GeneratedAt, &suite.LastRunAt)
		if err != nil {
			continue
		}
		suites = append(suites, suite)
	}

	return suites, nil
}

// UpdateTestSuiteStatus updates the status and results of a test suite
func (s *PerpetualStateStore) UpdateTestSuiteStatus(id int, status string, passingCount, failingCount int, coverage float64, errorMessage string) error {
	_, err := s.db.Exec(`
		UPDATE test_suites
		SET status = ?, passing_count = ?, failing_count = ?, coverage = ?, error_message = ?, last_run_at = ?
		WHERE id = ?
	`, status, passingCount, failingCount, coverage, errorMessage, time.Now(), id)

	if err != nil {
		return fmt.Errorf("failed to update test suite: %w", err)
	}
	return nil
}

// IncrementHealAttempts increments the heal attempt counter
func (s *PerpetualStateStore) IncrementHealAttempts(id int) error {
	_, err := s.db.Exec(`UPDATE test_suites SET heal_attempts = heal_attempts + 1 WHERE id = ?`, id)
	return err
}

// v24.0: Proposal Relationship Management

// ProposalRelationship represents a relationship between two proposals
type ProposalRelationship struct {
	ID                int       `json:"id"`
	ProposalID        string    `json:"proposal_id"`
	RelatedProposalID string    `json:"related_proposal_id"`
	RelationshipType  string    `json:"relationship_type"` // depends_on, conflicts_with, bundles_with, similar_to
	Confidence        float64   `json:"confidence"`
	DetectedBy        string    `json:"detected_by"` // file_overlap, semantic_similarity, source_correlation
	CreatedAt         time.Time `json:"created_at"`
}

// AddProposalRelationship adds a relationship between two proposals
func (s *PerpetualStateStore) AddProposalRelationship(proposalID, relatedID, relType string, confidence float64, detectedBy string) error {
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO proposal_relationships (proposal_id, related_proposal_id, relationship_type, confidence, detected_by)
		VALUES (?, ?, ?, ?, ?)
	`, proposalID, relatedID, relType, confidence, detectedBy)
	if err != nil {
		return fmt.Errorf("failed to add relationship: %w", err)
	}
	return nil
}

// GetProposalRelationships returns all relationships for a proposal
func (s *PerpetualStateStore) GetProposalRelationships(proposalID string) ([]*ProposalRelationship, error) {
	rows, err := s.db.Query(`
		SELECT id, proposal_id, related_proposal_id, relationship_type, confidence, COALESCE(detected_by, ''), created_at
		FROM proposal_relationships
		WHERE proposal_id = ? OR related_proposal_id = ?
		ORDER BY confidence DESC
	`, proposalID, proposalID)
	if err != nil {
		return nil, fmt.Errorf("failed to query relationships: %w", err)
	}
	defer rows.Close()

	var rels []*ProposalRelationship
	for rows.Next() {
		rel := &ProposalRelationship{}
		err := rows.Scan(&rel.ID, &rel.ProposalID, &rel.RelatedProposalID, &rel.RelationshipType, &rel.Confidence, &rel.DetectedBy, &rel.CreatedAt)
		if err != nil {
			continue
		}
		rels = append(rels, rel)
	}
	return rels, nil
}

// GetDependencies returns proposals that this proposal depends on
func (s *PerpetualStateStore) GetDependencies(proposalID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT related_proposal_id FROM proposal_relationships
		WHERE proposal_id = ? AND relationship_type = 'depends_on'
	`, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []string
	for rows.Next() {
		var depID string
		if err := rows.Scan(&depID); err == nil {
			deps = append(deps, depID)
		}
	}
	return deps, nil
}

// GetConflicts returns proposals that conflict with this proposal
func (s *PerpetualStateStore) GetConflicts(proposalID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT related_proposal_id FROM proposal_relationships
		WHERE proposal_id = ? AND relationship_type = 'conflicts_with'
		UNION
		SELECT proposal_id FROM proposal_relationships
		WHERE related_proposal_id = ? AND relationship_type = 'conflicts_with'
	`, proposalID, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conflicts []string
	for rows.Next() {
		var conflictID string
		if err := rows.Scan(&conflictID); err == nil {
			conflicts = append(conflicts, conflictID)
		}
	}
	return conflicts, nil
}

// GetBundles returns proposals bundled with this proposal
func (s *PerpetualStateStore) GetBundles(proposalID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT related_proposal_id FROM proposal_relationships
		WHERE proposal_id = ? AND relationship_type = 'bundles_with'
		UNION
		SELECT proposal_id FROM proposal_relationships
		WHERE related_proposal_id = ? AND relationship_type = 'bundles_with'
	`, proposalID, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bundles []string
	for rows.Next() {
		var bundleID string
		if err := rows.Scan(&bundleID); err == nil {
			bundles = append(bundles, bundleID)
		}
	}
	return bundles, nil
}
