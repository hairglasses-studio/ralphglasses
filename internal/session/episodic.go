package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Episode records a successful session trajectory for future in-context retrieval.
type Episode struct {
	Timestamp   time.Time `json:"ts"`
	TaskType    string    `json:"task_type"`
	TaskTitle   string    `json:"task_title"`
	Prompt      string    `json:"prompt"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	CostUSD     float64   `json:"cost_usd"`
	TurnCount   int       `json:"turn_count"`
	DurationSec float64   `json:"duration_sec"`
	Worked      []string  `json:"worked"`
	KeyInsights []string  `json:"key_insights"`
}

// EpisodicMemory stores and retrieves successful session trajectories.
type EpisodicMemory struct {
	mu       sync.Mutex
	episodes []Episode
	stateDir string
	maxSize  int
	DefaultK int // default retrieval limit for FindSimilar; 0 means 5
	embedder Embedder
}

// NewEpisodicMemory creates an episodic memory store backed by a JSONL file.
// If maxSize <= 0, defaults to 500. If defaultK <= 0, defaults to 5.
func NewEpisodicMemory(stateDir string, maxSize int, defaultK int) *EpisodicMemory {
	if maxSize <= 0 {
		maxSize = 500
	}
	if defaultK <= 0 {
		defaultK = 5
	}
	em := &EpisodicMemory{
		stateDir: stateDir,
		maxSize:  maxSize,
		DefaultK: defaultK,
	}
	em.load()
	return em
}

// RecordSuccess records a successful session as an episode for future retrieval.
// Only records if the journal entry indicates success and has positive signals.
func (em *EpisodicMemory) RecordSuccess(journal JournalEntry) {
	// Only record successful sessions
	switch journal.ExitReason {
	case "", "completed", "normal":
		// OK
	default:
		return
	}

	// Must have positive signals
	if len(journal.Worked) == 0 {
		return
	}

	ep := Episode{
		Timestamp:   time.Now(),
		TaskType:    classifyTask(journal.TaskFocus),
		TaskTitle:   journal.TaskFocus,
		Prompt:      truncate(journal.TaskFocus, 500),
		Provider:    journal.Provider,
		Model:       journal.Model,
		CostUSD:     journal.SpentUSD,
		TurnCount:   journal.TurnCount,
		DurationSec: journal.DurationSec,
		Worked:      journal.Worked,
		KeyInsights: journal.Suggest,
	}

	em.mu.Lock()
	em.episodes = append(em.episodes, ep)
	em.mu.Unlock()

	em.appendToFile(ep)
	em.Prune()
}

// SetEmbedder attaches an optional embedding model for similarity search.
// When set, FindSimilar uses cosine similarity on embeddings instead of Jaccard.
func (em *EpisodicMemory) SetEmbedder(e Embedder) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.embedder = e
}

// FindSimilar returns the top k episodes most similar to the given task.
func (em *EpisodicMemory) FindSimilar(taskType string, prompt string, k int) []Episode {
	if k <= 0 {
		k = em.DefaultK
	}

	em.mu.Lock()
	defer em.mu.Unlock()

	type scored struct {
		episode Episode
		score   float64
	}

	// Pre-compute query embedding if embedder is available
	var queryVec []float64
	if em.embedder != nil {
		if v, err := em.embedder.Embed(prompt); err == nil {
			queryVec = v
		}
	}

	now := time.Now()
	var results []scored

	for _, ep := range em.episodes {
		var score float64

		// Task type exact match
		if ep.TaskType == taskType {
			score += 2.0
		}

		// Similarity: use embeddings if available, otherwise Jaccard
		if queryVec != nil {
			if epVec, err := em.embedder.Embed(ep.Prompt); err == nil {
				score += CosineSimilarity(queryVec, epVec)
			}
		} else {
			score += jaccardSimilarity(prompt, ep.Prompt)
		}

		// Recency bonus
		age := now.Sub(ep.Timestamp)
		if age <= 7*24*time.Hour {
			score += 0.5
		} else if age <= 30*24*time.Hour {
			score += 0.25
		}

		if score < 0.1 {
			continue
		}

		results = append(results, scored{episode: ep, score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > k {
		results = results[:k]
	}

	episodes := make([]Episode, len(results))
	for i, r := range results {
		episodes[i] = r.episode
	}
	return episodes
}

// FormatExamples formats episodes as a markdown section for in-context injection.
func (em *EpisodicMemory) FormatExamples(episodes []Episode) string {
	if len(episodes) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Successful Approaches for Similar Tasks\n\n")

	for i, ep := range episodes {
		sb.WriteString(fmt.Sprintf("**Example %d** (%s, %s):\n", i+1, ep.TaskType, ep.Provider))
		sb.WriteString(fmt.Sprintf("- Task: %s\n", ep.TaskTitle))
		sb.WriteString(fmt.Sprintf("- Cost: $%.2f, Turns: %d\n", ep.CostUSD, ep.TurnCount))
		sb.WriteString(fmt.Sprintf("- What worked: %s\n", strings.Join(ep.Worked, ", ")))
		if len(ep.KeyInsights) > 0 {
			sb.WriteString(fmt.Sprintf("- Key insights: %s\n", strings.Join(ep.KeyInsights, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Use these examples to guide your approach.\n")
	return sb.String()
}

// Prune removes old episodes when the store exceeds maxSize.
// It preserves at least 10 episodes per task type when possible.
func (em *EpisodicMemory) Prune() {
	em.mu.Lock()
	defer em.mu.Unlock()

	if len(em.episodes) <= em.maxSize {
		return
	}

	// Group indices by task type
	byType := make(map[string][]int)
	for i, ep := range em.episodes {
		byType[ep.TaskType] = append(byType[ep.TaskType], i)
	}

	// Determine per-type minimum: 10, but cap so total minimums don't exceed maxSize
	minPerType := 10
	numTypes := len(byType)
	if numTypes > 0 && minPerType*numTypes > em.maxSize {
		minPerType = em.maxSize / numTypes
		if minPerType < 1 {
			minPerType = 1
		}
	}

	// Mark which indices to keep: at least minPerType most recent per type
	keep := make(map[int]bool)
	for _, indices := range byType {
		start := 0
		if len(indices) > minPerType {
			start = len(indices) - minPerType
		}
		for _, idx := range indices[start:] {
			keep[idx] = true
		}
	}

	// If we still need to remove more, remove oldest non-protected episodes
	// Build list of all episodes sorted by time (oldest first = lowest index)
	var pruned []Episode

	// First pass: add all protected episodes
	// Second pass: add remaining from newest to oldest until we hit maxSize
	type indexedEp struct {
		idx int
		ep  Episode
	}
	var unprotected []indexedEp
	var protected []Episode

	for i, ep := range em.episodes {
		if keep[i] {
			protected = append(protected, ep)
		} else {
			unprotected = append(unprotected, indexedEp{idx: i, ep: ep})
		}
	}

	remaining := em.maxSize - len(protected)
	if remaining < 0 {
		remaining = 0
	}

	// Keep the most recent unprotected episodes
	if len(unprotected) > remaining {
		unprotected = unprotected[len(unprotected)-remaining:]
	}

	// Rebuild in original order
	keepSet := make(map[int]bool)
	for i := range em.episodes {
		if keep[i] {
			keepSet[i] = true
		}
	}
	for _, u := range unprotected {
		keepSet[u.idx] = true
	}

	for i, ep := range em.episodes {
		if keepSet[i] {
			pruned = append(pruned, ep)
		}
	}

	em.episodes = pruned
	em.rewriteFile()
}

func (em *EpisodicMemory) appendToFile(ep Episode) {
	if em.stateDir == "" {
		return
	}
	_ = os.MkdirAll(em.stateDir, 0755)

	data, err := json.Marshal(ep)
	if err != nil {
		return
	}
	data = append(data, '\n')

	path := filepath.Join(em.stateDir, "episodes.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(data)
}

func (em *EpisodicMemory) rewriteFile() {
	if em.stateDir == "" {
		return
	}
	_ = os.MkdirAll(em.stateDir, 0755)

	path := filepath.Join(em.stateDir, "episodes.jsonl")
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	for _, ep := range em.episodes {
		data, err := json.Marshal(ep)
		if err != nil {
			continue
		}
		_, _ = f.Write(data)
		_, _ = f.Write([]byte{'\n'})
	}
}

func (em *EpisodicMemory) load() {
	if em.stateDir == "" {
		return
	}
	path := filepath.Join(em.stateDir, "episodes.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var episodes []Episode
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var ep Episode
		if json.Unmarshal(line, &ep) == nil {
			episodes = append(episodes, ep)
		}
	}

	em.mu.Lock()
	em.episodes = episodes
	em.mu.Unlock()
}

// jaccardSimilarity computes the Jaccard similarity between two strings
// based on their lowercased word sets.
func jaccardSimilarity(a, b string) float64 {
	wordsA := wordSet(a)
	wordsB := wordSet(b)

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 0
	}

	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	union := len(wordsA)
	for w := range wordsB {
		if !wordsA[w] {
			union++
		}
	}

	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func wordSet(s string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.Fields(strings.ToLower(s)) {
		set[w] = true
	}
	return set
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// FindSimilarEpisodes satisfies the EpisodicSource interface used by CurriculumSorter.
func (em *EpisodicMemory) FindSimilarEpisodes(taskType string, prompt string, k int) []CurriculumEpisode {
	episodes := em.FindSimilar(taskType, prompt, k)
	result := make([]CurriculumEpisode, len(episodes))
	for i, ep := range episodes {
		result[i] = CurriculumEpisode{
			TurnCount: ep.TurnCount,
			CostUSD:   ep.CostUSD,
			Worked:    ep.Worked,
		}
	}
	return result
}
