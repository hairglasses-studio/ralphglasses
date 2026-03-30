package session

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Insight is a transferable learning derived from past sessions.
type Insight struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`        // "success_pattern", "failure_pattern", "provider_hint", "budget_hint"
	Description string    `json:"description"`
	Confidence  float64   `json:"confidence"`  // 0-1
	SourceCount int       `json:"source_count"` // how many sessions contributed
	Provider    string    `json:"provider,omitempty"`
	TaskType    string    `json:"task_type,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// SessionLearning captures distilled learnings from a single session.
type SessionLearning struct {
	SessionID   string    `json:"session_id"`
	TaskType    string    `json:"task_type"`
	Provider    string    `json:"provider"`
	Success     bool      `json:"success"`
	CostUSD     float64   `json:"cost_usd"`
	TurnCount   int       `json:"turn_count"`
	DurationSec float64   `json:"duration_sec"`
	Worked      []string  `json:"worked,omitempty"`
	Failed      []string  `json:"failed,omitempty"`
	Suggest     []string  `json:"suggest,omitempty"`
	RecordedAt  time.Time `json:"recorded_at"`
}

// learningsStore is the JSON structure persisted to disk.
type learningsStore struct {
	Sessions  []SessionLearning `json:"sessions"`
	Insights  []Insight         `json:"insights"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// LearningTransfer aggregates learnings across sessions and applies them
// to new sessions via TransferInsights.
type LearningTransfer struct {
	mu       sync.Mutex
	sessions []SessionLearning
	insights []Insight
	stateDir string
}

// NewLearningTransfer creates a LearningTransfer backed by the given state directory.
func NewLearningTransfer(stateDir string) *LearningTransfer {
	lt := &LearningTransfer{
		stateDir: stateDir,
	}
	lt.load()
	return lt
}

// RecordSession stores a session learning for future transfer.
func (lt *LearningTransfer) RecordSession(learning SessionLearning) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if learning.RecordedAt.IsZero() {
		learning.RecordedAt = time.Now()
	}
	lt.sessions = append(lt.sessions, learning)
	lt.rebuildInsights()
	lt.save()
}

// RecordFromJournalEntry converts a JournalEntry into a SessionLearning and records it.
func (lt *LearningTransfer) RecordFromJournalEntry(entry JournalEntry) {
	success := entry.ExitReason == "" || entry.ExitReason == "completed" || entry.ExitReason == "normal"
	lt.RecordSession(SessionLearning{
		SessionID:   entry.SessionID,
		TaskType:    classifyTask(entry.TaskFocus),
		Provider:    entry.Provider,
		Success:     success,
		CostUSD:     entry.SpentUSD,
		TurnCount:   entry.TurnCount,
		DurationSec: entry.DurationSec,
		Worked:      entry.Worked,
		Failed:      entry.Failed,
		Suggest:     entry.Suggest,
		RecordedAt:  entry.Timestamp,
	})
}

// TransferInsights returns insights derived from the given fromSessions that are
// applicable to toSession. If fromSessions is empty, all stored sessions are used.
func (lt *LearningTransfer) TransferInsights(fromSessions []string, toSession string) []Insight {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if len(lt.insights) == 0 {
		return nil
	}

	// If no filter, return all insights.
	if len(fromSessions) == 0 {
		result := make([]Insight, len(lt.insights))
		copy(result, lt.insights)
		return result
	}

	// Filter sessions to those in fromSessions.
	fromSet := make(map[string]bool, len(fromSessions))
	for _, id := range fromSessions {
		fromSet[id] = true
	}

	var filtered []SessionLearning
	for _, s := range lt.sessions {
		if fromSet[s.SessionID] {
			filtered = append(filtered, s)
		}
	}

	if len(filtered) == 0 {
		return nil
	}

	return lt.buildInsightsFrom(filtered)
}

// AllInsights returns all current insights.
func (lt *LearningTransfer) AllInsights() []Insight {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	result := make([]Insight, len(lt.insights))
	copy(result, lt.insights)
	return result
}

// AllSessions returns all recorded session learnings.
func (lt *LearningTransfer) AllSessions() []SessionLearning {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	result := make([]SessionLearning, len(lt.sessions))
	copy(result, lt.sessions)
	return result
}

// rebuildInsights regenerates insights from all stored sessions.
// Must be called with lt.mu held.
func (lt *LearningTransfer) rebuildInsights() {
	lt.insights = lt.buildInsightsFrom(lt.sessions)
}

// buildInsightsFrom extracts insights from a set of session learnings.
func (lt *LearningTransfer) buildInsightsFrom(sessions []SessionLearning) []Insight {
	if len(sessions) == 0 {
		return nil
	}

	var insights []Insight

	// --- Success/failure pattern extraction ---
	workedCounts := make(map[string]int)
	failedCounts := make(map[string]int)
	suggestCounts := make(map[string]int)
	for _, s := range sessions {
		for _, w := range s.Worked {
			if w != "" {
				workedCounts[w]++
			}
		}
		for _, f := range s.Failed {
			if f != "" {
				failedCounts[f]++
			}
		}
		for _, sg := range s.Suggest {
			if sg != "" {
				suggestCounts[sg]++
			}
		}
	}

	total := len(sessions)
	for pattern, count := range workedCounts {
		if count >= 2 {
			insights = append(insights, Insight{
				ID:          "success-" + hashStr(pattern),
				Type:        "success_pattern",
				Description: pattern,
				Confidence:  float64(count) / float64(total),
				SourceCount: count,
				CreatedAt:   time.Now(),
			})
		}
	}
	for pattern, count := range failedCounts {
		if count >= 2 {
			insights = append(insights, Insight{
				ID:          "failure-" + hashStr(pattern),
				Type:        "failure_pattern",
				Description: pattern,
				Confidence:  float64(count) / float64(total),
				SourceCount: count,
				CreatedAt:   time.Now(),
			})
		}
	}
	for pattern, count := range suggestCounts {
		if count >= 2 {
			insights = append(insights, Insight{
				ID:          "suggest-" + hashStr(pattern),
				Type:        "success_pattern",
				Description: "Suggestion: " + pattern,
				Confidence:  float64(count) / float64(total),
				SourceCount: count,
				CreatedAt:   time.Now(),
			})
		}
	}

	// --- Provider hint: best provider by task type ---
	type providerTaskKey struct {
		provider string
		taskType string
	}
	type providerStats struct {
		success int
		total   int
	}
	provStats := make(map[providerTaskKey]*providerStats)
	for _, s := range sessions {
		key := providerTaskKey{provider: s.Provider, taskType: s.TaskType}
		if provStats[key] == nil {
			provStats[key] = &providerStats{}
		}
		provStats[key].total++
		if s.Success {
			provStats[key].success++
		}
	}

	// Group by task type to find the best provider.
	taskProviders := make(map[string]map[string]*providerStats)
	for key, stats := range provStats {
		if taskProviders[key.taskType] == nil {
			taskProviders[key.taskType] = make(map[string]*providerStats)
		}
		taskProviders[key.taskType][key.provider] = stats
	}
	for taskType, providers := range taskProviders {
		var bestProvider string
		bestRate := -1.0
		for provider, stats := range providers {
			if stats.total < 2 {
				continue
			}
			rate := float64(stats.success) / float64(stats.total)
			if rate > bestRate {
				bestRate = rate
				bestProvider = provider
			}
		}
		if bestProvider != "" && bestRate >= 0.5 {
			insights = append(insights, Insight{
				ID:          "provider-" + taskType + "-" + bestProvider,
				Type:        "provider_hint",
				Description: "Use " + bestProvider + " for " + taskType + " tasks",
				Confidence:  bestRate,
				SourceCount: provStats[providerTaskKey{provider: bestProvider, taskType: taskType}].total,
				Provider:    bestProvider,
				TaskType:    taskType,
				CreatedAt:   time.Now(),
			})
		}
	}

	// --- Budget hint: avg cost per task type ---
	taskCosts := make(map[string][]float64)
	for _, s := range sessions {
		if s.Success && s.CostUSD > 0 {
			taskCosts[s.TaskType] = append(taskCosts[s.TaskType], s.CostUSD)
		}
	}
	for taskType, costs := range taskCosts {
		if len(costs) < 2 {
			continue
		}
		var sum float64
		for _, c := range costs {
			sum += c
		}
		avg := sum / float64(len(costs))
		insights = append(insights, Insight{
			ID:          "budget-" + taskType,
			Type:        "budget_hint",
			Description: "Typical budget for " + taskType + " tasks: $" + formatFloat(avg*1.5),
			Confidence:  float64(len(costs)) / float64(total),
			SourceCount: len(costs),
			TaskType:    taskType,
			CreatedAt:   time.Now(),
		})
	}

	return insights
}

func (lt *LearningTransfer) save() {
	if lt.stateDir == "" {
		return
	}
	if err := os.MkdirAll(lt.stateDir, 0755); err != nil {
		slog.Warn("learning_transfer: failed to create state dir", "dir", lt.stateDir, "error", err)
		return
	}

	store := learningsStore{
		Sessions:  lt.sessions,
		Insights:  lt.insights,
		UpdatedAt: time.Now(),
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		slog.Warn("learning_transfer: failed to marshal", "error", err)
		return
	}
	path := filepath.Join(lt.stateDir, "learnings.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		slog.Warn("learning_transfer: failed to write", "path", path, "error", err)
	}
}

func (lt *LearningTransfer) load() {
	if lt.stateDir == "" {
		return
	}
	path := filepath.Join(lt.stateDir, "learnings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var store learningsStore
	if json.Unmarshal(data, &store) == nil {
		lt.sessions = store.Sessions
		lt.insights = store.Insights
	}
}

// hashStr returns a short hash string for use in IDs.
func hashStr(s string) string {
	// Use a simple FNV-style hash to produce a short hex string.
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return strings.ToLower(formatHex(h))
}

func formatHex(v uint32) string {
	const digits = "0123456789abcdef"
	buf := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		buf[i] = digits[v&0xf]
		v >>= 4
	}
	return string(buf)
}

func formatFloat(f float64) string {
	// Simple float formatter to 2 decimal places.
	if f == 0 {
		return "0.00"
	}
	whole := int(f)
	frac := int((f-float64(whole))*100 + 0.5)
	if frac >= 100 {
		whole++
		frac -= 100
	}
	return ltItoa(whole) + "." + ltPad2(frac)
}

func ltItoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

func ltPad2(n int) string {
	if n < 10 {
		return "0" + ltItoa(n)
	}
	return ltItoa(n)
}
