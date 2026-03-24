package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// JournalEntry records what worked, failed, and suggestions from a session.
type JournalEntry struct {
	Timestamp   time.Time `json:"ts"`
	SessionID   string    `json:"session_id"`
	Provider    string    `json:"provider"`
	RepoName    string    `json:"repo_name"`
	Model       string    `json:"model"`
	SpentUSD    float64   `json:"spent_usd"`
	TurnCount   int       `json:"turn_count"`
	DurationSec float64   `json:"duration_sec"`
	Worked            []string `json:"worked"`
	Failed            []string `json:"failed"`
	Suggest           []string `json:"suggest"`
	SignalSource      string   `json:"signal_source,omitempty"`
	ExitReason        string   `json:"exit_reason"`
	TaskFocus         string   `json:"task_focus"`
	EnhancementSource string   `json:"enhancement_source,omitempty"`
	EnhancementScore  int      `json:"enhancement_score,omitempty"`
}

// ConsolidatedPatterns holds durable patterns extracted from journal history.
type ConsolidatedPatterns struct {
	UpdatedAt time.Time          `json:"updated_at"`
	Positive  []ConsolidatedItem `json:"positive"`
	Negative  []ConsolidatedItem `json:"negative"`
	Rules     []string           `json:"rules"`
}

// ConsolidatedItem is a pattern that appeared multiple times.
type ConsolidatedItem struct {
	Text     string    `json:"text"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
	Category string    `json:"category"`
}

const (
	journalFile  = ".ralph/improvement_journal.jsonl"
	patternsFile = ".ralph/improvement_patterns.json"
)

// WriteJournalEntry appends a journal entry for a completed session.
func WriteJournalEntry(s *Session) error {
	s.mu.Lock()
	entry := JournalEntry{
		Timestamp:  time.Now(),
		SessionID:  s.ID,
		Provider:   string(s.Provider),
		RepoName:   s.RepoName,
		Model:      s.Model,
		SpentUSD:   s.SpentUSD,
		TurnCount:  s.TurnCount,
		ExitReason:        s.ExitReason,
		TaskFocus:         extractTaskFocus(s.Prompt),
		EnhancementSource: s.EnhancementSource,
		EnhancementScore:  s.EnhancementPreScore,
	}
	if s.EndedAt != nil {
		entry.DurationSec = s.EndedAt.Sub(s.LaunchedAt).Seconds()
	} else {
		entry.DurationSec = time.Since(s.LaunchedAt).Seconds()
	}
	// Parse output history for improvement markers
	entry.Worked, entry.Failed, entry.Suggest = parseImprovementMarkers(s.OutputHistory)
	if len(entry.Worked) > 0 || len(entry.Failed) > 0 || len(entry.Suggest) > 0 {
		entry.SignalSource = "markers"
	}

	// Fallback: auto-populate from session status when markers aren't found
	if len(entry.Worked) == 0 && len(entry.Failed) == 0 {
		if s.Status == StatusCompleted && entry.TaskFocus != "" {
			entry.Worked = []string{entry.TaskFocus}
		} else if s.Status == StatusErrored {
			errMsg := s.Error
			if errMsg == "" {
				errMsg = s.ExitReason
			}
			if errMsg != "" {
				entry.Failed = []string{errMsg}
			}
		}
	}

	// Third tier: heuristic signal extraction from output history
	if entry.SignalSource == "" {
		entry.SignalSource = "fallback"
	}
	if entry.SignalSource == "fallback" {
		w, f, sg := extractSignalsFromOutput(s.OutputHistory, s.Status, s.SpentUSD, s.TurnCount)
		if len(w) > 0 || len(f) > 0 || len(sg) > 0 {
			entry.Worked = append(entry.Worked, w...)
			entry.Failed = append(entry.Failed, f...)
			entry.Suggest = append(entry.Suggest, sg...)
			entry.SignalSource = "heuristic"
		}
	}

	repoPath := s.RepoPath
	s.mu.Unlock()

	return writeJournalEntryToFile(repoPath, entry)
}

// WriteJournalEntryManual writes a manually constructed journal entry.
func WriteJournalEntryManual(repoPath string, entry JournalEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	return writeJournalEntryToFile(repoPath, entry)
}

func writeJournalEntryToFile(repoPath string, entry JournalEntry) error {
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		return fmt.Errorf("create .ralph dir: %w", err)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := filepath.Join(repoPath, journalFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// ReadRecentJournal reads the last maxEntries from the journal file.
func ReadRecentJournal(repoPath string, maxEntries int) ([]JournalEntry, error) {
	if maxEntries <= 0 {
		maxEntries = 5
	}

	path := filepath.Join(repoPath, journalFile)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	// Read all lines, keep last N
	var allEntries []JournalEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry JournalEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip malformed
		}
		allEntries = append(allEntries, entry)
	}

	if len(allEntries) <= maxEntries {
		return allEntries, nil
	}
	return allEntries[len(allEntries)-maxEntries:], nil
}

// SynthesizeContext produces a bounded markdown summary from journal entries.
// Output is capped at 2000 characters.
func SynthesizeContext(entries []JournalEntry) string {
	if len(entries) == 0 {
		return ""
	}

	workedSet := dedup(collectAll(entries, func(e JournalEntry) []string { return e.Worked }))
	failedSet := dedup(collectAll(entries, func(e JournalEntry) []string { return e.Failed }))

	// Suggestions: prioritize last 2 sessions
	var suggestItems []string
	start := 0
	if len(entries) > 2 {
		start = len(entries) - 2
	}
	for _, e := range entries[start:] {
		suggestItems = append(suggestItems, e.Suggest...)
	}
	suggestSet := dedup(suggestItems)

	var sb strings.Builder
	sb.WriteString("## Session Improvement Context\n\n")

	if len(workedSet) > 0 {
		sb.WriteString("### Reinforce\n")
		for _, item := range workedSet {
			sb.WriteString("- " + item + "\n")
		}
		sb.WriteString("\n")
	}

	if len(failedSet) > 0 {
		sb.WriteString("### Avoid\n")
		for _, item := range failedSet {
			sb.WriteString("- " + item + "\n")
		}
		sb.WriteString("\n")
	}

	if len(suggestSet) > 0 {
		sb.WriteString("### Apply Now\n")
		for _, item := range suggestSet {
			sb.WriteString("- " + item + "\n")
		}
		sb.WriteString("\n")
	}

	result := sb.String()
	if len(result) > 2000 {
		result = result[:1997] + "..."
	}
	return result
}

// ConsolidatePatterns extracts frequently recurring items from the full journal.
func ConsolidatePatterns(repoPath string) error {
	entries, err := ReadRecentJournal(repoPath, 10000) // read all
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}

	workedCounts := countItems(collectAll(entries, func(e JournalEntry) []string { return e.Worked }))
	failedCounts := countItems(collectAll(entries, func(e JournalEntry) []string { return e.Failed }))

	patterns := ConsolidatedPatterns{
		UpdatedAt: time.Now(),
	}

	for text, count := range workedCounts {
		if count >= 3 {
			patterns.Positive = append(patterns.Positive, ConsolidatedItem{
				Text:     text,
				Count:    count,
				LastSeen: findLastSeen(entries, text, func(e JournalEntry) []string { return e.Worked }),
				Category: "positive",
			})
		}
	}

	for text, count := range failedCounts {
		if count >= 3 {
			patterns.Negative = append(patterns.Negative, ConsolidatedItem{
				Text:     text,
				Count:    count,
				LastSeen: findLastSeen(entries, text, func(e JournalEntry) []string { return e.Failed }),
				Category: "negative",
			})
		}
	}

	// Extract rules from frequent suggestions
	suggestCounts := countItems(collectAll(entries, func(e JournalEntry) []string { return e.Suggest }))
	for text, count := range suggestCounts {
		if count >= 3 {
			patterns.Rules = append(patterns.Rules, text)
		}
	}

	data, err := json.MarshalIndent(patterns, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(repoPath, patternsFile), data, 0644)
}

// PruneJournal keeps only the last keepN entries, consolidating patterns first.
func PruneJournal(repoPath string, keepN int) (pruned int, err error) {
	if keepN <= 0 {
		keepN = 100
	}

	// Consolidate before pruning
	if err := ConsolidatePatterns(repoPath); err != nil {
		return 0, fmt.Errorf("consolidate: %w", err)
	}

	entries, err := ReadRecentJournal(repoPath, 10000) // read all
	if err != nil {
		return 0, err
	}

	if len(entries) <= keepN {
		return 0, nil
	}

	prunedCount := len(entries) - keepN
	kept := entries[len(entries)-keepN:]

	// Rewrite file
	path := filepath.Join(repoPath, journalFile)
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	for _, entry := range kept {
		data, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		if _, err := f.Write(data); err != nil {
			continue
		}
		if _, err := f.Write([]byte{'\n'}); err != nil {
			continue
		}
	}

	return prunedCount, nil
}

// parseImprovementMarkers extracts worked/failed/suggest from output history.
// Looks for ---RALPH_STATUS--- blocks or falls back to empty.
func parseImprovementMarkers(history []string) (worked, failed, suggest []string) {
	combined := strings.Join(history, "\n")

	// Look for structured status block
	if idx := strings.Index(combined, "---RALPH_STATUS---"); idx != -1 {
		block := combined[idx:]
		if endIdx := strings.Index(block, "---END_STATUS---"); endIdx != -1 {
			block = block[:endIdx]
		}
		worked = extractSection(block, "WORKED:")
		failed = extractSection(block, "FAILED:")
		suggest = extractSection(block, "SUGGEST:")
	}

	return worked, failed, suggest
}

// extractSignalsFromOutput scans session output history for improvement signals
// using heuristic pattern matching. This is a third-tier fallback when explicit
// markers and simple status-based fallbacks produce no data.
func extractSignalsFromOutput(history []string, status SessionStatus, spentUSD float64, turnCount int) (worked, failed, suggest []string) {
	errorPatterns := []string{"error:", "panic:", "FAIL", "failed to", "cannot ", "undefined:"}
	successPatterns := []string{"PASS", "ok  \t", "created ", "implemented ", "fixed ", "added ", "updated "}

	errorCounts := make(map[string]int) // track repeated errors

	for _, line := range history {
		lower := strings.ToLower(line)

		// Error detection
		for _, pat := range errorPatterns {
			if strings.Contains(lower, strings.ToLower(pat)) {
				// Deduplicate: use first 80 chars as key
				key := line
				if len(key) > 80 {
					key = key[:80]
				}
				key = strings.TrimSpace(key)
				if key == "" {
					continue
				}
				errorCounts[key]++
				if errorCounts[key] == 1 {
					failed = append(failed, key)
				}
				break
			}
		}

		// Success detection
		for _, pat := range successPatterns {
			if strings.Contains(line, pat) {
				item := strings.TrimSpace(line)
				if len(item) > 120 {
					item = item[:120]
				}
				if item != "" {
					worked = append(worked, item)
				}
				break
			}
		}
	}

	// Friction: repeated identical errors suggest systematic issues
	for errLine, count := range errorCounts {
		if count >= 3 {
			suggest = append(suggest, fmt.Sprintf("investigate repeated error (%dx): %s", count, errLine))
		}
	}

	// Cost anomaly: high cost per turn
	if turnCount > 0 && spentUSD > 0 {
		costPerTurn := spentUSD / float64(turnCount)
		if costPerTurn > 0.10 { // >$0.10/turn is expensive
			suggest = append(suggest, fmt.Sprintf("high cost per turn: $%.2f/turn (total $%.2f over %d turns)", costPerTurn, spentUSD, turnCount))
		}
	}

	// Cap results to prevent journal bloat
	if len(worked) > 5 {
		worked = worked[:5]
	}
	if len(failed) > 5 {
		failed = failed[:5]
	}
	if len(suggest) > 3 {
		suggest = suggest[:3]
	}

	return worked, failed, suggest
}

func extractSection(block, header string) []string {
	idx := strings.Index(block, header)
	if idx == -1 {
		return nil
	}
	rest := block[idx+len(header):]
	// Read until next header or end
	for _, hdr := range []string{"WORKED:", "FAILED:", "SUGGEST:", "---"} {
		if hdr == header {
			continue
		}
		if endIdx := strings.Index(rest, hdr); endIdx != -1 {
			rest = rest[:endIdx]
		}
	}

	var items []string
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		if line != "" {
			items = append(items, line)
		}
	}
	return items
}

func extractTaskFocus(prompt string) string {
	// First line of prompt, capped
	if idx := strings.IndexByte(prompt, '\n'); idx != -1 {
		prompt = prompt[:idx]
	}
	if len(prompt) > 200 {
		prompt = prompt[:200]
	}
	return strings.TrimSpace(prompt)
}

func collectAll(entries []JournalEntry, getter func(JournalEntry) []string) []string {
	var all []string
	for _, e := range entries {
		all = append(all, getter(e)...)
	}
	return all
}

func dedup(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		normalized := strings.TrimSpace(strings.ToLower(item))
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		result = append(result, item)
	}
	return result
}

func countItems(items []string) map[string]int {
	counts := make(map[string]int)
	for _, item := range items {
		normalized := strings.TrimSpace(strings.ToLower(item))
		if normalized != "" {
			counts[normalized]++
		}
	}
	return counts
}

func findLastSeen(entries []JournalEntry, text string, getter func(JournalEntry) []string) time.Time {
	normalized := strings.TrimSpace(strings.ToLower(text))
	for i := len(entries) - 1; i >= 0; i-- {
		for _, item := range getter(entries[i]) {
			if strings.TrimSpace(strings.ToLower(item)) == normalized {
				return entries[i].Timestamp
			}
		}
	}
	return time.Time{}
}
