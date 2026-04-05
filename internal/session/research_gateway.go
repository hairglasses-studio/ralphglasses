package session

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver.
)

// DocsResearchGateway implements ResearchGateway by opening the docs repo's
// SQLite database directly and manipulating the docs filesystem. This avoids
// importing docs/internal/registries while using the same schema.
type DocsResearchGateway struct {
	db       *sql.DB
	docsRoot string
}

// NewDocsResearchGateway opens the docs SQLite database and returns a gateway.
// docsRoot is the absolute path to the docs repo (e.g. ~/hairglasses-studio/docs).
func NewDocsResearchGateway(docsRoot string) (*DocsResearchGateway, error) {
	dbPath := filepath.Join(docsRoot, ".docs.sqlite")
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("research gateway: open db: %w", err)
	}
	// Ensure queue table exists (idempotent).
	if err := ensureQueueTable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("research gateway: init schema: %w", err)
	}
	return &DocsResearchGateway{db: db, docsRoot: docsRoot}, nil
}

// Close releases the database connection.
func (g *DocsResearchGateway) Close() error {
	if g.db != nil {
		return g.db.Close()
	}
	return nil
}

func ensureQueueTable(db *sql.DB) error {
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
		`CREATE INDEX IF NOT EXISTS idx_queue_status ON research_queue(status)`,
		`CREATE INDEX IF NOT EXISTS idx_queue_priority ON research_queue(priority_score DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_queue_domain ON research_queue(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_queue_expires ON research_queue(expires_at)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// ── ResearchGateway interface ───────────────────────────────────────────────

func (g *DocsResearchGateway) ExpireStale(ctx context.Context) (int, error) {
	result, err := g.db.ExecContext(ctx, `UPDATE research_queue SET
		status = 'pending',
		claimed_by = '',
		claimed_at = '',
		expires_at = '',
		retry_count = retry_count + 1,
		updated = datetime('now')
		WHERE status = 'claimed' AND expires_at != '' AND expires_at < datetime('now')`)
	if err != nil {
		return 0, fmt.Errorf("expire stale: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (g *DocsResearchGateway) DequeueNext(ctx context.Context, agent string, claimTTL int) (*ResearchEntry, error) {
	if claimTTL <= 0 {
		claimTTL = 7200
	}

	var targetID int64
	err := g.db.QueryRowContext(ctx,
		`SELECT id FROM research_queue WHERE status = 'pending' ORDER BY priority_score DESC LIMIT 1`,
	).Scan(&targetID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dequeue: select: %w", err)
	}

	now := time.Now().UTC()
	claimedAt := now.Format("2006-01-02 15:04:05")
	expiresAt := now.Add(time.Duration(claimTTL) * time.Second).Format("2006-01-02 15:04:05")

	_, err = g.db.ExecContext(ctx, `UPDATE research_queue SET
		status = 'claimed', claimed_by = ?, claimed_at = ?, expires_at = ?, updated = datetime('now')
		WHERE id = ? AND status = 'pending'`,
		agent, claimedAt, expiresAt, targetID)
	if err != nil {
		return nil, fmt.Errorf("dequeue: claim: %w", err)
	}

	// Read back the claimed entry.
	var e ResearchEntry
	err = g.db.QueryRowContext(ctx, `SELECT topic, domain, source, priority_score, model_tier, budget_usd
		FROM research_queue WHERE id = ?`, targetID).Scan(
		&e.Topic, &e.Domain, &e.Source, &e.PriorityScore, &e.ModelTier, &e.BudgetUSD)
	if err != nil {
		return nil, fmt.Errorf("dequeue: read back: %w", err)
	}
	return &e, nil
}

func (g *DocsResearchGateway) DedupCheck(ctx context.Context, topic, domain string) (float64, string, error) {
	// Lightweight dedup: check if research_queue already has a completed entry,
	// and grep the docs filesystem for existing coverage.
	var completedCount int
	err := g.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM research_queue WHERE topic = ? AND domain = ? AND status = 'completed'`,
		topic, domain).Scan(&completedCount)
	if err != nil && err != sql.ErrNoRows {
		return 0, "", fmt.Errorf("dedup: queue check: %w", err)
	}

	// Check for existing research files via ripgrep.
	researchDir := filepath.Join(g.docsRoot, "research")
	if domain != "" {
		researchDir = filepath.Join(researchDir, domain)
	}
	fileCount := 0
	if _, err := os.Stat(researchDir); err == nil {
		cmd := exec.CommandContext(ctx, "rg", "-l", "-i", topic, researchDir)
		if out, err := cmd.Output(); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if line != "" {
					fileCount++
				}
			}
		}
	}

	// Compute confidence: completed queue entries + file matches.
	confidence := 0.0
	if completedCount > 0 {
		confidence += 0.5
	}
	if fileCount > 0 {
		confidence += 0.3
		if fileCount >= 3 {
			confidence += 0.2 // well-covered
		}
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	recommendation := "proceed"
	if confidence >= 0.7 {
		recommendation = "exists"
	} else if confidence >= 0.4 {
		recommendation = "partial"
	}

	return confidence, recommendation, nil
}

func (g *DocsResearchGateway) Complete(ctx context.Context, topic, domain string) error {
	_, err := g.db.ExecContext(ctx, `UPDATE research_queue SET
		status = 'completed', updated = datetime('now')
		WHERE topic = ? AND domain = ? AND status = 'claimed'`,
		topic, domain)
	if err != nil {
		return fmt.Errorf("complete: %w", err)
	}
	return nil
}

func (g *DocsResearchGateway) Abandon(ctx context.Context, topic, domain, reason string) error {
	_, err := g.db.ExecContext(ctx, `UPDATE research_queue SET
		status = 'pending',
		claimed_by = '',
		claimed_at = '',
		expires_at = '',
		retry_count = retry_count + 1,
		priority_score = MIN(1.0, priority_score * 1.10),
		updated = datetime('now')
		WHERE topic = ? AND domain = ? AND status = 'claimed'`,
		topic, domain)
	if err != nil {
		return fmt.Errorf("abandon: %w", err)
	}
	slog.Debug("research-gateway: abandoned", "topic", topic, "reason", reason)
	return nil
}

func (g *DocsResearchGateway) WriteResearch(ctx context.Context, domain, title, content string, urls []string) error {
	// Validate domain.
	validDomains := map[string]bool{
		"mcp": true, "agents": true, "orchestration": true,
		"cost-optimization": true, "go-ecosystem": true,
		"terminal": true, "competitive": true,
	}
	if !validDomains[domain] {
		return fmt.Errorf("invalid domain %q", domain)
	}

	dir := filepath.Join(g.docsRoot, "research", domain)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	// Generate filename from title (kebab-case).
	filename := toKebabCase(title) + ".md"
	path := filepath.Join(dir, filename)

	// Build frontmatter.
	now := time.Now().Format("2006-01-02")
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "title: %q\n", title)
	fmt.Fprintf(&sb, "domain: %s\n", domain)
	fmt.Fprintf(&sb, "generated: %s\n", now)
	sb.WriteString("agent: research-daemon\n")
	sb.WriteString("source: passive-research\n")
	sb.WriteString("---\n\n")
	sb.WriteString(content)
	sb.WriteString("\n")

	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	// Append URLs to tagged index if any were provided.
	if len(urls) > 0 {
		g.appendURLs(domain, urls)
	}

	slog.Info("research-gateway: wrote finding",
		"path", filepath.Join("research", domain, filename),
		"bytes", sb.Len())
	return nil
}

func (g *DocsResearchGateway) CommitAndPush(ctx context.Context, message string) error {
	script := filepath.Join(g.docsRoot, "scripts", "push-docs.sh")
	if _, err := os.Stat(script); err != nil {
		// Fallback: git add + commit + push directly.
		return g.gitCommitAndPush(ctx, message)
	}
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.Dir = g.docsRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("push-docs.sh: %v: %s", err, string(out))
	}
	return nil
}

func (g *DocsResearchGateway) gitCommitAndPush(ctx context.Context, message string) error {
	cmds := [][]string{
		{"git", "add", "research/"},
		{"git", "commit", "-m", message},
		{"git", "push"},
	}
	for _, args := range cmds {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = g.docsRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %v: %s", args[0], err, string(out))
		}
	}
	return nil
}

func (g *DocsResearchGateway) appendURLs(domain string, urls []string) {
	indexPath := filepath.Join(g.docsRoot, "indexes", "TAGGED-URL-INDEX.md")
	f, err := os.OpenFile(indexPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Warn("research-gateway: append urls failed", "error", err)
		return
	}
	defer f.Close()
	for _, u := range urls {
		fmt.Fprintf(f, "| %s | %s | research | passive | - | [research] [%s] | research-daemon |\n",
			u, domain, domain)
	}
}

// toKebabCase converts a title string to kebab-case filename.
func toKebabCase(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, s)
	// Collapse consecutive dashes.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
