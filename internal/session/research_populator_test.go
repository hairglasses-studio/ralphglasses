package session

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func setupPopulatorDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// In-memory SQLite: each connection gets its own database.
	// Force single connection so all operations share the same schema.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	// Create queue table.
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS research_queue (
			id INTEGER PRIMARY KEY,
			topic TEXT NOT NULL,
			domain TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'manual',
			status TEXT NOT NULL DEFAULT 'pending',
			priority_score REAL NOT NULL DEFAULT 0,
			model_tier TEXT NOT NULL DEFAULT 'sonnet',
			budget_usd REAL NOT NULL DEFAULT 3.00,
			claimed_by TEXT DEFAULT '',
			claimed_at TEXT DEFAULT '',
			retry_count INTEGER DEFAULT 0,
			tags JSON DEFAULT '[]',
			created TEXT DEFAULT (datetime('now')),
			updated TEXT DEFAULT (datetime('now')),
			expires_at TEXT DEFAULT '',
			UNIQUE(topic, domain)
		)`,
		`CREATE TABLE IF NOT EXISTS research_freshness (
			id INTEGER PRIMARY KEY,
			topic TEXT NOT NULL,
			domain TEXT NOT NULL DEFAULT '',
			staleness_score REAL NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS docs_meta (
			path TEXT PRIMARY KEY,
			domain TEXT DEFAULT '',
			type TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS research_registry (
			id INTEGER PRIMARY KEY,
			topic TEXT NOT NULL,
			domain TEXT DEFAULT '',
			status TEXT DEFAULT 'planned',
			session_ids JSON DEFAULT '[]',
			file_paths JSON DEFAULT '[]',
			agent TEXT DEFAULT '',
			created TEXT DEFAULT (datetime('now')),
			updated TEXT DEFAULT (datetime('now')),
			UNIQUE(topic, domain)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup: %v\n%s", err, stmt)
		}
	}
	return db
}

func TestPopulateFromFreshness(t *testing.T) {
	db := setupPopulatorDB(t)
	p := NewResearchPopulator(db, t.TempDir())

	// Seed stale entries.
	db.Exec(`INSERT INTO research_freshness (topic, domain, staleness_score) VALUES ('stale-topic', 'mcp', 2.0)`)
	db.Exec(`INSERT INTO research_freshness (topic, domain, staleness_score) VALUES ('fresh-topic', 'mcp', 0.5)`)

	ctx := context.Background()
	n, err := p.PopulateFromFreshness(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 enqueued (stale only), got %d", n)
	}

	// Verify the entry exists in queue.
	var topic string
	db.QueryRow(`SELECT topic FROM research_queue WHERE topic = 'stale-topic'`).Scan(&topic)
	if topic != "stale-topic" {
		t.Error("stale-topic not found in queue")
	}
}

func TestPopulateFromGaps(t *testing.T) {
	docsRoot := t.TempDir()
	researchDir := filepath.Join(docsRoot, "research")
	os.MkdirAll(researchDir, 0o755)

	// Create a knowledge-base.md with gaps.
	content := `# Knowledge Base

- **MCP caching** — GAP
- **Agent protocols** — COMPLETE
- **Cost optimization** — PARTIAL
`
	os.WriteFile(filepath.Join(researchDir, "knowledge-base.md"), []byte(content), 0o644)

	db := setupPopulatorDB(t)
	p := NewResearchPopulator(db, docsRoot)

	ctx := context.Background()
	n, err := p.PopulateFromGaps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 { // GAP + PARTIAL
		t.Errorf("expected 2 enqueued, got %d", n)
	}
}

func TestPopulateFromRoadmap(t *testing.T) {
	docsRoot := t.TempDir()
	stratDir := filepath.Join(docsRoot, "strategy")
	os.MkdirAll(stratDir, 0o755)

	content := `# META-ROADMAP

## Phase 1
- [x] Build the thing
- [ ] Research new MCP streaming protocol
- [ ] Implement feature X
- [ ] Investigate agent memory patterns
`
	os.WriteFile(filepath.Join(stratDir, "META-ROADMAP.md"), []byte(content), 0o644)

	db := setupPopulatorDB(t)
	p := NewResearchPopulator(db, docsRoot)

	ctx := context.Background()
	n, err := p.PopulateFromRoadmap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// "Research new MCP..." and "Investigate agent memory..." match
	if n != 2 {
		t.Errorf("expected 2 enqueued, got %d", n)
	}
}

func TestRebalance(t *testing.T) {
	db := setupPopulatorDB(t)
	p := NewResearchPopulator(db, t.TempDir())

	// Seed docs_meta — "agents" has 10 docs but 0 completed.
	for i := 0; i < 10; i++ {
		db.Exec(`INSERT INTO docs_meta (path, domain, type) VALUES (?, 'agents', 'research')`,
			filepath.Join("a", "b", string(rune('a'+i))+".md"))
	}

	// Seed a pending queue entry in the under-covered domain.
	db.Exec(`INSERT INTO research_queue (topic, domain, source, priority_score, status) VALUES ('agent-topic', 'agents', 'manual', 0.5, 'pending')`)

	ctx := context.Background()
	n, err := p.Rebalance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 boosted, got %d", n)
	}

	// Check priority was boosted.
	var score float64
	db.QueryRow(`SELECT priority_score FROM research_queue WHERE topic = 'agent-topic'`).Scan(&score)
	if score < 0.59 || score > 0.61 {
		t.Errorf("expected ~0.60 (0.50 * 1.20), got %v", score)
	}
}

func TestRunAll(t *testing.T) {
	db := setupPopulatorDB(t)
	docsRoot := t.TempDir()
	p := NewResearchPopulator(db, docsRoot)

	// Seed one stale entry.
	db.Exec(`INSERT INTO research_freshness (topic, domain, staleness_score) VALUES ('stale', 'mcp', 3.0)`)

	ctx := context.Background()
	total, err := p.RunAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if total < 1 {
		t.Errorf("expected >= 1 total enqueued, got %d", total)
	}
}

func TestExtractTopicFromLine(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"- **MCP caching** — GAP", "MCP caching"},
		{"- Cost optimization (PARTIAL)", "Cost optimization"},
		{"- Simple topic: details", "Simple topic"},
		{"  - Agent protocols - needs work", "Agent protocols"},
	}
	for _, tt := range tests {
		got := extractTopicFromLine(tt.line)
		if got != tt.want {
			t.Errorf("extractTopicFromLine(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestInferDomain(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{"MCP caching patterns", "mcp"},
		{"agent memory architecture", "agents"},
		{"token cost analysis", "cost-optimization"},
		{"terminal UI rendering", "terminal"},
		{"something unknown", "orchestration"},
	}
	for _, tt := range tests {
		got := inferDomain(tt.topic)
		if got != tt.want {
			t.Errorf("inferDomain(%q) = %q, want %q", tt.topic, got, tt.want)
		}
	}
}
