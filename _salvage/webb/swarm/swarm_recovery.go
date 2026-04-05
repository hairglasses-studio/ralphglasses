// Package clients provides API clients for webb.
// v23.0: Swarm Recovery Manager for crash recovery and checkpoint restoration
package clients

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SwarmRecoveryManager handles crash recovery and checkpointing
type SwarmRecoveryManager struct {
	orchestrator *SwarmOrchestrator
	db           *sql.DB
	dbPath       string
	checkpoints  []*SwarmCheckpoint
	maxHistory   int
	mu           sync.RWMutex
}

// NewSwarmRecoveryManager creates a new recovery manager
func NewSwarmRecoveryManager(orchestrator *SwarmOrchestrator) *SwarmRecoveryManager {
	homeDir, _ := os.UserHomeDir()
	dbPath := filepath.Join(homeDir, ".webb", "swarm-recovery.db")

	rm := &SwarmRecoveryManager{
		orchestrator: orchestrator,
		dbPath:       dbPath,
		checkpoints:  make([]*SwarmCheckpoint, 0),
		maxHistory:   100,
	}

	// Initialize database
	if err := rm.initDB(); err != nil {
		// Log error but don't fail - recovery is optional
		fmt.Printf("Warning: failed to initialize swarm recovery DB: %v\n", err)
	}

	return rm
}

// initDB initializes the SQLite database for recovery
func (rm *SwarmRecoveryManager) initDB() error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(rm.dbPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	db, err := sql.Open("sqlite", rm.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for crash recovery
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return fmt.Errorf("failed to enable WAL: %w", err)
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS swarm_checkpoints (
		id TEXT PRIMARY KEY,
		swarm_id TEXT NOT NULL,
		state TEXT NOT NULL,
		data TEXT NOT NULL,
		created_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS swarm_findings (
		id TEXT PRIMARY KEY,
		swarm_id TEXT NOT NULL,
		worker_id TEXT NOT NULL,
		category TEXT NOT NULL,
		title TEXT NOT NULL,
		data TEXT NOT NULL,
		created_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS swarm_runs (
		id TEXT PRIMARY KEY,
		config TEXT NOT NULL,
		state TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		stopped_at DATETIME,
		final_metrics TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_checkpoints_swarm ON swarm_checkpoints(swarm_id);
	CREATE INDEX IF NOT EXISTS idx_checkpoints_created ON swarm_checkpoints(created_at);
	CREATE INDEX IF NOT EXISTS idx_findings_swarm ON swarm_findings(swarm_id);
	CREATE INDEX IF NOT EXISTS idx_runs_started ON swarm_runs(started_at);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// v31.0: Add embedding column for semantic deduplication (ignore if exists)
	_, _ = db.Exec(`ALTER TABLE swarm_findings ADD COLUMN embedding BLOB`)

	rm.db = db
	return nil
}

// Close closes the recovery manager
func (rm *SwarmRecoveryManager) Close() error {
	if rm.db != nil {
		return rm.db.Close()
	}
	return nil
}

// SaveCheckpoint saves a checkpoint to the database
func (rm *SwarmRecoveryManager) SaveCheckpoint(checkpoint *SwarmCheckpoint) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Add to in-memory list
	rm.checkpoints = append(rm.checkpoints, checkpoint)
	if len(rm.checkpoints) > rm.maxHistory {
		rm.checkpoints = rm.checkpoints[1:]
	}

	// Save to database if available
	if rm.db == nil {
		return nil
	}

	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	_, err = rm.db.Exec(`
		INSERT OR REPLACE INTO swarm_checkpoints (id, swarm_id, state, data, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, checkpoint.ID, checkpoint.SwarmID, checkpoint.State, string(data), checkpoint.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	// Prune old checkpoints (keep last 50 per swarm)
	_, _ = rm.db.Exec(`
		DELETE FROM swarm_checkpoints
		WHERE swarm_id = ? AND id NOT IN (
			SELECT id FROM swarm_checkpoints
			WHERE swarm_id = ?
			ORDER BY created_at DESC
			LIMIT 50
		)
	`, checkpoint.SwarmID, checkpoint.SwarmID)

	return nil
}

// LoadLastCheckpoint loads the most recent checkpoint for a swarm
func (rm *SwarmRecoveryManager) LoadLastCheckpoint(swarmID string) (*SwarmCheckpoint, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.db == nil {
		// Return from in-memory if available
		for i := len(rm.checkpoints) - 1; i >= 0; i-- {
			if rm.checkpoints[i].SwarmID == swarmID {
				return rm.checkpoints[i], nil
			}
		}
		return nil, fmt.Errorf("no checkpoint found for swarm %s", swarmID)
	}

	var data string
	err := rm.db.QueryRow(`
		SELECT data FROM swarm_checkpoints
		WHERE swarm_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, swarmID).Scan(&data)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no checkpoint found for swarm %s", swarmID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	var checkpoint SwarmCheckpoint
	if err := json.Unmarshal([]byte(data), &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// ListCheckpoints lists checkpoints for a swarm
func (rm *SwarmRecoveryManager) ListCheckpoints(swarmID string, limit int) ([]*SwarmCheckpoint, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	if rm.db == nil {
		// Return from in-memory
		result := make([]*SwarmCheckpoint, 0)
		for i := len(rm.checkpoints) - 1; i >= 0 && len(result) < limit; i-- {
			if rm.checkpoints[i].SwarmID == swarmID {
				result = append(result, rm.checkpoints[i])
			}
		}
		return result, nil
	}

	rows, err := rm.db.Query(`
		SELECT data FROM swarm_checkpoints
		WHERE swarm_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, swarmID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query checkpoints: %w", err)
	}
	defer rows.Close()

	result := make([]*SwarmCheckpoint, 0)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var cp SwarmCheckpoint
		if err := json.Unmarshal([]byte(data), &cp); err != nil {
			continue
		}
		result = append(result, &cp)
	}

	return result, nil
}

// SaveFinding saves a finding to the database
func (rm *SwarmRecoveryManager) SaveFinding(swarmID string, finding *SwarmResearchFinding) error {
	if rm.db == nil {
		return nil
	}

	data, err := json.Marshal(finding)
	if err != nil {
		return fmt.Errorf("failed to marshal finding: %w", err)
	}

	_, err = rm.db.Exec(`
		INSERT OR REPLACE INTO swarm_findings (id, swarm_id, worker_id, category, title, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, finding.ID, swarmID, finding.WorkerID, finding.Category, finding.Title, string(data), finding.CreatedAt)

	return err
}

// LoadFindings loads findings for a swarm
func (rm *SwarmRecoveryManager) LoadFindings(swarmID string, limit int) ([]*SwarmResearchFinding, error) {
	if rm.db == nil {
		return nil, nil
	}

	if limit <= 0 {
		limit = 100
	}

	rows, err := rm.db.Query(`
		SELECT data FROM swarm_findings
		WHERE swarm_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, swarmID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query findings: %w", err)
	}
	defer rows.Close()

	result := make([]*SwarmResearchFinding, 0)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var f SwarmResearchFinding
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			continue
		}
		result = append(result, &f)
	}

	return result, nil
}

// SaveRun saves a swarm run record
func (rm *SwarmRecoveryManager) SaveRun(swarmID string, config *SwarmConfig, state SwarmState, startedAt time.Time) error {
	if rm.db == nil {
		return nil
	}

	configData, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	_, err = rm.db.Exec(`
		INSERT OR REPLACE INTO swarm_runs (id, config, state, started_at)
		VALUES (?, ?, ?, ?)
	`, swarmID, string(configData), state, startedAt)

	return err
}

// CompleteRun marks a run as complete
func (rm *SwarmRecoveryManager) CompleteRun(swarmID string, state SwarmState, metrics *SwarmMetrics) error {
	if rm.db == nil {
		return nil
	}

	metricsData, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	_, err = rm.db.Exec(`
		UPDATE swarm_runs
		SET state = ?, stopped_at = ?, final_metrics = ?
		WHERE id = ?
	`, state, time.Now(), string(metricsData), swarmID)

	return err
}

// GetIncompleteRuns returns runs that didn't complete properly
func (rm *SwarmRecoveryManager) GetIncompleteRuns() ([]string, error) {
	if rm.db == nil {
		return nil, nil
	}

	rows, err := rm.db.Query(`
		SELECT id FROM swarm_runs
		WHERE state IN ('running', 'paused', 'initializing')
		AND stopped_at IS NULL
		ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query incomplete runs: %w", err)
	}
	defer rows.Close()

	result := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		result = append(result, id)
	}

	return result, nil
}

// RecoverSwarm attempts to recover a swarm from its last checkpoint
func (rm *SwarmRecoveryManager) RecoverSwarm(swarmID string) (*SwarmRecoveryResult, error) {
	// Load last checkpoint
	checkpoint, err := rm.LoadLastCheckpoint(swarmID)
	if err != nil {
		return nil, fmt.Errorf("no checkpoint available: %w", err)
	}

	// Load findings
	findings, err := rm.LoadFindings(swarmID, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to load findings: %w", err)
	}

	return &SwarmRecoveryResult{
		Checkpoint:    checkpoint,
		Findings:      findings,
		RecoveredAt:   time.Now(),
		WorkersToSpawn: getWorkersFromCheckpoint(checkpoint),
	}, nil
}

// SwarmRecoveryResult contains recovery data
type SwarmRecoveryResult struct {
	Checkpoint     *SwarmCheckpoint `json:"checkpoint"`
	Findings       []*SwarmResearchFinding  `json:"findings"`
	RecoveredAt    time.Time        `json:"recovered_at"`
	WorkersToSpawn []string         `json:"workers_to_spawn"`
}

// getWorkersFromCheckpoint extracts worker IDs that need respawning
func getWorkersFromCheckpoint(cp *SwarmCheckpoint) []string {
	if cp == nil {
		return nil
	}

	result := make([]string, 0)
	for workerID, status := range cp.WorkerStates {
		if status.State == "running" || status.State == "paused" {
			result = append(result, workerID)
		}
	}
	return result
}

// CleanupOldData removes data older than the specified duration
func (rm *SwarmRecoveryManager) CleanupOldData(olderThan time.Duration) error {
	if rm.db == nil {
		return nil
	}

	cutoff := time.Now().Add(-olderThan)

	// Delete old checkpoints
	_, err := rm.db.Exec(`
		DELETE FROM swarm_checkpoints
		WHERE created_at < ?
	`, cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup checkpoints: %w", err)
	}

	// Delete old findings
	_, err = rm.db.Exec(`
		DELETE FROM swarm_findings
		WHERE created_at < ?
	`, cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup findings: %w", err)
	}

	// Delete old runs
	_, err = rm.db.Exec(`
		DELETE FROM swarm_runs
		WHERE started_at < ?
	`, cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup runs: %w", err)
	}

	return nil
}

// GetStats returns recovery system statistics
func (rm *SwarmRecoveryManager) GetStats() *SwarmRecoveryStats {
	if rm.db == nil {
		return &SwarmRecoveryStats{}
	}

	stats := &SwarmRecoveryStats{}

	rm.db.QueryRow(`SELECT COUNT(*) FROM swarm_checkpoints`).Scan(&stats.TotalCheckpoints)
	rm.db.QueryRow(`SELECT COUNT(*) FROM swarm_findings`).Scan(&stats.TotalFindings)
	rm.db.QueryRow(`SELECT COUNT(*) FROM swarm_runs`).Scan(&stats.TotalRuns)
	rm.db.QueryRow(`
		SELECT COUNT(*) FROM swarm_runs
		WHERE state IN ('running', 'paused', 'initializing')
		AND stopped_at IS NULL
	`).Scan(&stats.IncompleteRuns)

	// Get database size
	if fi, err := os.Stat(rm.dbPath); err == nil {
		stats.DatabaseSizeBytes = fi.Size()
	}

	return stats
}

// SwarmRecoveryStats contains recovery system statistics
type SwarmRecoveryStats struct {
	TotalCheckpoints  int   `json:"total_checkpoints"`
	TotalFindings     int   `json:"total_findings"`
	TotalRuns         int   `json:"total_runs"`
	IncompleteRuns    int   `json:"incomplete_runs"`
	DatabaseSizeBytes int64 `json:"database_size_bytes"`
}

// v31.0: Embedding-based semantic deduplication

// SaveFindingWithEmbedding saves a finding with its embedding for semantic search
func (rm *SwarmRecoveryManager) SaveFindingWithEmbedding(swarmID string, finding *SwarmResearchFinding, embedding []float64) error {
	if rm.db == nil {
		return nil
	}

	data, err := json.Marshal(finding)
	if err != nil {
		return fmt.Errorf("failed to marshal finding: %w", err)
	}

	embeddingBlob := encodeEmbedding(embedding)

	_, err = rm.db.Exec(`
		INSERT OR REPLACE INTO swarm_findings (id, swarm_id, worker_id, category, title, data, created_at, embedding)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, finding.ID, swarmID, finding.WorkerID, finding.Category, finding.Title, string(data), finding.CreatedAt, embeddingBlob)

	return err
}

// FindSimilarFindings finds findings similar to the given embedding
func (rm *SwarmRecoveryManager) FindSimilarFindings(embedding []float64, threshold float64, limit int) ([]*SimilarFinding, error) {
	if rm.db == nil {
		return nil, nil
	}

	if limit <= 0 {
		limit = 10
	}

	// Query all findings with embeddings
	rows, err := rm.db.Query(`
		SELECT id, swarm_id, title, category, data, embedding FROM swarm_findings
		WHERE embedding IS NOT NULL
		ORDER BY created_at DESC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query findings: %w", err)
	}
	defer rows.Close()

	var results []*SimilarFinding
	for rows.Next() {
		var id, swarmID, title, category, data string
		var embeddingBlob []byte
		if err := rows.Scan(&id, &swarmID, &title, &category, &data, &embeddingBlob); err != nil {
			continue
		}

		storedEmbedding := decodeEmbedding(embeddingBlob)
		if len(storedEmbedding) == 0 {
			continue
		}

		similarity := cosineSimilarityFloat64(embedding, storedEmbedding)
		if similarity >= threshold {
			var finding SwarmResearchFinding
			if err := json.Unmarshal([]byte(data), &finding); err != nil {
				continue
			}
			results = append(results, &SimilarFinding{
				Finding:    &finding,
				Similarity: similarity,
			})
		}
	}

	// Sort by similarity descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Similarity > results[i].Similarity {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Limit results
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// SimilarFinding represents a finding with its similarity score
type SimilarFinding struct {
	Finding    *SwarmResearchFinding `json:"finding"`
	Similarity float64               `json:"similarity"`
}

// encodeEmbedding encodes a float64 slice to bytes for SQLite BLOB storage
func encodeEmbedding(embedding []float64) []byte {
	if len(embedding) == 0 {
		return nil
	}
	data, _ := json.Marshal(embedding)
	return data
}

// decodeEmbedding decodes bytes from SQLite BLOB to float64 slice
func decodeEmbedding(data []byte) []float64 {
	if len(data) == 0 {
		return nil
	}
	var embedding []float64
	if err := json.Unmarshal(data, &embedding); err != nil {
		return nil
	}
	return embedding
}

// cosineSimilarityFloat64 calculates cosine similarity between two float64 embeddings
func cosineSimilarityFloat64(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrtFloat64(normA) * sqrtFloat64(normB))
}

// sqrtFloat64 calculates square root using Newton-Raphson method
func sqrtFloat64(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z -= (z*z - x) / (2 * z)
	}
	return z
}
