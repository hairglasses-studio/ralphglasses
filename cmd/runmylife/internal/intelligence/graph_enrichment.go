package intelligence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hairglasses-studio/runmylife/internal/knowledge"
)

// EnrichFromGraph augments suggestions with related entity context from the knowledge graph.
// For each suggestion that mentions a known entity, it appends relationship context.
func EnrichFromGraph(ctx context.Context, db *sql.DB, suggestions []Suggestion) []Suggestion {
	enriched := make([]Suggestion, len(suggestions))
	copy(enriched, suggestions)

	for i, s := range enriched {
		// Look for person entities mentioned in the suggestion
		links, err := findRelatedEntities(ctx, db, s.Title+" "+s.Description)
		if err != nil || len(links) == 0 {
			continue
		}

		// Append relationship context to the description
		var extras []string
		seen := make(map[string]bool)
		for _, link := range links {
			key := link.SourceLabel + "→" + link.TargetLabel
			if seen[key] {
				continue
			}
			seen[key] = true
			extras = append(extras, fmt.Sprintf("%s (%s %s)",
				link.SourceLabel, link.Relationship, link.TargetLabel))
		}
		if len(extras) > 0 {
			enriched[i].Description += " | Related: " + strings.Join(extras, ", ")
		}
	}

	return enriched
}

// GraphBasedSuggestions generates suggestions from knowledge graph patterns.
// It detects stale contacts (in calendar but no recent email) and related tasks to batch.
func GraphBasedSuggestions(ctx context.Context, db *sql.DB) []Suggestion {
	var suggestions []Suggestion

	// Pattern 1: Contacts who appear in recent calendar events but have no recent email
	stale := findStaleCalendarContacts(ctx, db)
	for _, contact := range stale {
		suggestions = append(suggestions, Suggestion{
			Category:    "social",
			Priority:    0.4,
			Title:       fmt.Sprintf("Follow up with %s", contact),
			Description: fmt.Sprintf("%s appears in your calendar but hasn't emailed recently.", contact),
			ActionHint:  "runmylife_social(domain=health, action=contact)",
		})
	}

	// Pattern 2: Tasks linked to the same project that could be batched
	batches := findBatchableTasks(ctx, db)
	for project, count := range batches {
		if count >= 3 {
			suggestions = append(suggestions, Suggestion{
				Category:    "personal",
				Priority:    0.35,
				Title:       fmt.Sprintf("Batch %d tasks for %s", count, project),
				Description: fmt.Sprintf("Multiple open tasks related to %s. Consider a focused session.", project),
				ActionHint:  "runmylife_tasks(domain=manage, action=list)",
			})
		}
	}

	return suggestions
}

// findRelatedEntities searches the knowledge graph for entities mentioned in text.
func findRelatedEntities(ctx context.Context, db *sql.DB, text string) ([]knowledge.EntityLink, error) {
	lower := strings.ToLower(text)

	// Get all person entities and check if any are mentioned
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT source_id, source_label FROM entity_links
		 WHERE source_type = 'person' AND source_label != ''
		 UNION
		 SELECT DISTINCT target_id, target_label FROM entity_links
		 WHERE target_type = 'person' AND target_label != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var allLinks []knowledge.EntityLink
	for rows.Next() {
		var id, label string
		if rows.Scan(&id, &label) != nil || label == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(label)) {
			links, err := knowledge.FindRelated(ctx, db, "person", id, 5)
			if err == nil {
				allLinks = append(allLinks, links...)
			}
		}
	}
	return allLinks, nil
}

// findStaleCalendarContacts finds people in recent calendar events who haven't emailed recently.
func findStaleCalendarContacts(ctx context.Context, db *sql.DB) []string {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT el.target_label
		 FROM entity_links el
		 WHERE el.source_type = 'event'
		   AND el.relationship = 'attends'
		   AND el.target_type = 'person'
		   AND el.target_label != ''
		   AND el.target_id NOT IN (
		       SELECT el2.target_id FROM entity_links el2
		       WHERE el2.source_type = 'person'
		         AND el2.relationship = 'communicates'
		         AND el2.created_at > datetime('now', '-14 days')
		   )
		 LIMIT 5`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var contacts []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			contacts = append(contacts, name)
		}
	}
	return contacts
}

// findBatchableTasks finds projects with multiple open tasks.
func findBatchableTasks(ctx context.Context, db *sql.DB) map[string]int {
	rows, err := db.QueryContext(ctx,
		`SELECT el.target_label, COUNT(*)
		 FROM entity_links el
		 JOIN tasks t ON t.id = el.source_id
		 WHERE el.source_type = 'task'
		   AND el.relationship = 'about'
		   AND t.completed = 0
		 GROUP BY el.target_label
		 HAVING COUNT(*) >= 2`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	batches := make(map[string]int)
	for rows.Next() {
		var project string
		var count int
		if rows.Scan(&project, &count) == nil {
			batches[project] = count
		}
	}
	return batches
}
