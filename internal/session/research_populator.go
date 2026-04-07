package session

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ResearchPopulator fills the research queue from automated sources:
// staleness decay, gap analysis, roadmap prerequisites, and domain rebalancing.
type ResearchPopulator struct {
	db       *sql.DB
	docsRoot string
}

// NewResearchPopulator creates a populator backed by the docs SQLite database.
func NewResearchPopulator(db *sql.DB, docsRoot string) *ResearchPopulator {
	return &ResearchPopulator{db: db, docsRoot: docsRoot}
}

// RunAll executes all population sources and returns total topics enqueued.
func (p *ResearchPopulator) RunAll(ctx context.Context) (int, error) {
	total := 0

	if n, err := p.PopulateFromFreshness(ctx); err != nil {
		slog.Warn("populator: freshness scan failed", "error", err)
	} else {
		total += n
	}

	if n, err := p.PopulateFromGaps(ctx); err != nil {
		slog.Warn("populator: gap scan failed", "error", err)
	} else {
		total += n
	}

	if n, err := p.PopulateFromRoadmap(ctx); err != nil {
		slog.Warn("populator: roadmap scan failed", "error", err)
	} else {
		total += n
	}

	if n, err := p.Rebalance(ctx); err != nil {
		slog.Warn("populator: rebalance failed", "error", err)
	} else {
		total += n
	}

	if total > 0 {
		slog.Info("populator: enqueued topics", "total", total)
	}
	return total, nil
}

// PopulateFromFreshness enqueues topics from stale research documents.
// Queries the research_freshness table for staleness_score > 1.5.
func (p *ResearchPopulator) PopulateFromFreshness(ctx context.Context) (int, error) {
	// Check if research_freshness table exists.
	var exists int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='research_freshness'`,
	).Scan(&exists)
	if err != nil || exists == 0 {
		return 0, nil // table doesn't exist yet
	}

	rows, err := p.db.QueryContext(ctx,
		`SELECT topic, domain FROM research_freshness WHERE staleness_score > 1.5
		 AND topic NOT IN (SELECT topic FROM research_queue WHERE status IN ('pending','claimed'))
		 ORDER BY staleness_score DESC LIMIT 10`)
	if err != nil {
		return 0, fmt.Errorf("freshness query: %w", err)
	}
	defer rows.Close()

	type queuedTopic struct {
		topic  string
		domain string
	}
	var topics []queuedTopic
	for rows.Next() {
		var topic, domain string
		if err := rows.Scan(&topic, &domain); err != nil {
			continue
		}
		topics = append(topics, queuedTopic{topic: topic, domain: domain})
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("freshness scan: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("freshness close: %w", err)
	}

	count := 0
	for _, item := range topics {
		if err := p.enqueue(ctx, item.topic, item.domain, "freshness", 0.3, "haiku"); err != nil {
			slog.Debug("populator: freshness enqueue failed", "topic", item.topic, "error", err)
			continue
		}
		count++
	}
	return count, nil
}

// PopulateFromGaps enqueues topics identified as GAP or PARTIAL in the
// research plan (typically research/knowledge-base.md or similar).
func (p *ResearchPopulator) PopulateFromGaps(ctx context.Context) (int, error) {
	planPath := filepath.Join(p.docsRoot, "research", "knowledge-base.md")
	f, err := os.Open(planPath)
	if err != nil {
		return 0, nil // file doesn't exist, not an error
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "gap") && !strings.Contains(lower, "partial") {
			continue
		}
		// Extract topic from markdown list items: "- **topic** — GAP"
		topic := extractTopicFromLine(line)
		if topic == "" {
			continue
		}
		domain := inferDomain(topic)
		priority := 0.5
		if strings.Contains(lower, "gap") {
			priority = 0.6
		}
		if err := p.enqueue(ctx, topic, domain, "gap", priority, "sonnet"); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// PopulateFromRoadmap enqueues unchecked research items from META-ROADMAP.md.
func (p *ResearchPopulator) PopulateFromRoadmap(ctx context.Context) (int, error) {
	metaPath := filepath.Join(p.docsRoot, "strategy", "META-ROADMAP.md")
	f, err := os.Open(metaPath)
	if err != nil {
		return 0, nil
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		// Only unchecked items that look like research tasks.
		if !strings.HasPrefix(trimmed, "- [ ]") {
			continue
		}
		lower := strings.ToLower(trimmed)
		if !strings.Contains(lower, "research") && !strings.Contains(lower, "investigate") &&
			!strings.Contains(lower, "explore") && !strings.Contains(lower, "evaluate") {
			continue
		}
		topic := strings.TrimSpace(strings.TrimPrefix(trimmed, "- [ ]"))
		if topic == "" {
			continue
		}
		domain := inferDomain(topic)
		if err := p.enqueue(ctx, topic, domain, "roadmap", 0.7, "sonnet"); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// Rebalance boosts priority of topics in under-covered domains.
// Domains with < 30% research coverage get a 20% priority boost.
func (p *ResearchPopulator) Rebalance(ctx context.Context) (int, error) {
	// Check for required tables.
	var queueExists, metaExists int
	p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='research_queue'`).Scan(&queueExists)
	p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='docs_meta'`).Scan(&metaExists)
	if queueExists == 0 || metaExists == 0 {
		return 0, nil
	}

	// Find under-covered domains.
	rows, err := p.db.QueryContext(ctx, `
		SELECT dm.domain,
			COUNT(DISTINCT dm.path) AS total_docs,
			COUNT(DISTINCT CASE WHEN rr.status = 'complete' THEN rr.topic END) AS completed
		FROM docs_meta dm
		LEFT JOIN research_registry rr ON rr.domain = dm.domain
		WHERE dm.domain != ''
		GROUP BY dm.domain
		HAVING total_docs > 0 AND (CAST(completed AS REAL) / total_docs) < 0.30`)
	if err != nil {
		return 0, fmt.Errorf("rebalance query: %w", err)
	}
	defer rows.Close()

	var underCovered []string
	for rows.Next() {
		var domain string
		var total, completed int
		if rows.Scan(&domain, &total, &completed) == nil {
			underCovered = append(underCovered, domain)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("rebalance scan: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("rebalance close: %w", err)
	}

	if len(underCovered) == 0 {
		return 0, nil
	}

	// Boost pending items in under-covered domains by 20%.
	count := 0
	for _, domain := range underCovered {
		result, err := p.db.ExecContext(ctx, `UPDATE research_queue SET
			priority_score = MIN(1.0, priority_score * 1.20),
			updated = datetime('now')
			WHERE domain = ? AND status = 'pending'`, domain)
		if err != nil {
			continue
		}
		n, _ := result.RowsAffected()
		count += int(n)
	}

	if count > 0 {
		slog.Info("populator: rebalanced under-covered domains",
			"domains", underCovered, "boosted", count)
	}
	return count, nil
}

func (p *ResearchPopulator) enqueue(ctx context.Context, topic, domain, source string, priority float64, modelTier string) error {
	budget := 3.0
	switch modelTier {
	case "haiku":
		budget = 0.50
	case "opus":
		budget = 10.00
	}

	_, err := p.db.ExecContext(ctx, `INSERT INTO research_queue (topic, domain, source, priority_score, model_tier, budget_usd)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(topic, domain) DO UPDATE SET
			priority_score = MAX(research_queue.priority_score, excluded.priority_score),
			source = excluded.source,
			updated = datetime('now')
		WHERE research_queue.status = 'pending'`,
		topic, domain, source, priority, modelTier, budget)
	return err
}

// extractTopicFromLine pulls a topic name from markdown list items like:
// "- **MCP caching patterns** — GAP" or "- MCP caching (PARTIAL)".
func extractTopicFromLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "-")
	line = strings.TrimPrefix(line, "*")
	line = strings.TrimSpace(line)

	// Strip bold markers.
	line = strings.ReplaceAll(line, "**", "")

	// Take text before common separators.
	for _, sep := range []string{" — ", " - ", " (", ":"} {
		if idx := strings.Index(line, sep); idx > 0 {
			line = line[:idx]
		}
	}
	return strings.TrimSpace(line)
}

// inferDomain guesses the research domain from a topic string.
func inferDomain(topic string) string {
	lower := strings.ToLower(topic)
	domainKeywords := map[string][]string{
		"mcp":               {"mcp", "model context protocol", "tool server"},
		"agents":            {"agent", "autonomous", "multi-agent", "swarm"},
		"orchestration":     {"orchestr", "supervisor", "loop", "daemon"},
		"cost-optimization": {"cost", "budget", "token", "pricing", "batch"},
		"go-ecosystem":      {"golang", "go module", "go package", "go ecosystem"},
		"terminal":          {"terminal", "tui", "cli", "shell", "console"},
		"competitive":       {"competitive", "comparison", "benchmark", "alternative"},
	}
	for domain, keywords := range domainKeywords {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				return domain
			}
		}
	}
	return "orchestration" // default
}

// LastPopulated returns the timestamp of the most recent population run
// based on queue entry creation times.
func (p *ResearchPopulator) LastPopulated() time.Time {
	var ts string
	err := p.db.QueryRow(`SELECT MAX(created) FROM research_queue WHERE source IN ('freshness','gap','roadmap')`).Scan(&ts)
	if err != nil || ts == "" {
		return time.Time{}
	}
	t, _ := time.Parse("2006-01-02 15:04:05", ts)
	return t
}
