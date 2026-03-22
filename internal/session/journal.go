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
	Worked      []string  `json:"worked"`
	Failed      []string  `json:"failed"`
	Suggest     []string  `json:"suggest"`
	ExitReason  string    `json:"exit_reason"`
	TaskFocus   string    `json:"task_focus"`
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
		ExitReason: s.ExitReason,
		TaskFocus:  extractTaskFocus(s.Prompt),
	}
	if s.EndedAt != nil {
		entry.DurationSec = s.EndedAt.Sub(s.LaunchedAt).Seconds()
	} else {
		entry.DurationSec = time.Since(s.LaunchedAt).Seconds()
	}
	// Parse output history for improvement markers
	entry.Worked, entry.Failed, entry.Suggest = parseImprovementMarkers(s.OutputHistory)
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
		f.Write(data)
		f.Write([]byte{'\n'})
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
