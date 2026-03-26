package events

import "time"

// EventQuery defines filters for querying events.
type EventQuery struct {
	Types     []EventType // filter by multiple types (OR). Empty = all types
	Since     *time.Time  // events after this time (inclusive)
	Until     *time.Time  // events before this time (exclusive)
	SessionID string      // filter by session ID (exact match)
	RepoName  string      // filter by repo name (exact match)
	Provider  string      // filter by provider (exact match)
	Limit     int         // max results (0 = unlimited)
	Offset    int         // skip first N matches (pagination)
}

// EventQueryResult holds query results with pagination info.
type EventQueryResult struct {
	Events     []Event
	TotalCount int  // total matching events (before limit/offset)
	HasMore    bool // more results available beyond limit
}

// EventAggregation defines parameters for grouping and counting events.
type EventAggregation struct {
	GroupBy string     // field to group by: "type", "session_id", "repo_name", "provider"
	Since   *time.Time // optional time filter
	Until   *time.Time
}

// AggregationResult holds grouped event counts.
type AggregationResult struct {
	Groups map[string]int
	Total  int
}

// Query executes a complex event query with multi-filter and pagination.
func (b *Bus) Query(q EventQuery) EventQueryResult {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Build type set for O(1) lookup
	typeSet := make(map[EventType]bool, len(q.Types))
	for _, t := range q.Types {
		typeSet[t] = true
	}

	var matched []Event
	for _, e := range b.history {
		if !matchesQuery(e, q, typeSet) {
			continue
		}
		matched = append(matched, e)
	}

	total := len(matched)

	// Apply offset
	if q.Offset > 0 && q.Offset < len(matched) {
		matched = matched[q.Offset:]
	} else if q.Offset >= len(matched) {
		matched = nil
	}

	// Apply limit
	hasMore := false
	if q.Limit > 0 && len(matched) > q.Limit {
		hasMore = true
		matched = matched[:q.Limit]
	}

	return EventQueryResult{
		Events:     matched,
		TotalCount: total,
		HasMore:    hasMore,
	}
}

// Aggregate groups events and returns counts per group.
func (b *Bus) Aggregate(agg EventAggregation) AggregationResult {
	b.mu.RLock()
	defer b.mu.RUnlock()

	groups := make(map[string]int)
	total := 0

	for _, e := range b.history {
		// Apply time filters
		if agg.Since != nil && e.Timestamp.Before(*agg.Since) {
			continue
		}
		if agg.Until != nil && !e.Timestamp.Before(*agg.Until) {
			continue
		}

		var key string
		switch agg.GroupBy {
		case "type":
			key = string(e.Type)
		case "session_id":
			key = e.SessionID
		case "repo_name":
			key = e.RepoName
		case "provider":
			key = e.Provider
		default:
			key = "unknown"
		}

		groups[key]++
		total++
	}

	return AggregationResult{
		Groups: groups,
		Total:  total,
	}
}

// matchesQuery returns true if the event matches all non-zero query filters.
func matchesQuery(e Event, q EventQuery, typeSet map[EventType]bool) bool {
	// Type filter (OR across types)
	if len(typeSet) > 0 && !typeSet[e.Type] {
		return false
	}

	// Time range filters
	if q.Since != nil && e.Timestamp.Before(*q.Since) {
		return false
	}
	if q.Until != nil && !e.Timestamp.Before(*q.Until) {
		return false
	}

	// Exact match filters
	if q.SessionID != "" && e.SessionID != q.SessionID {
		return false
	}
	if q.RepoName != "" && e.RepoName != q.RepoName {
		return false
	}
	if q.Provider != "" && e.Provider != q.Provider {
		return false
	}

	return true
}
