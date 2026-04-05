// Package patterns provides cross-session pattern mining and workflow analysis.
package patterns

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PatternType categorizes discovered patterns
type PatternType string

const (
	PatternSequential  PatternType = "sequential"   // A → B → C (ordered)
	PatternCoOccur     PatternType = "co_occur"     // A + B + C (any order)
	PatternTemporal    PatternType = "temporal"     // Time-based patterns
	PatternError       PatternType = "error"        // Error cascades
	PatternAntiPattern PatternType = "anti_pattern" // Inefficient patterns
)

// DiscoveredPattern represents a mined pattern from session data
type DiscoveredPattern struct {
	ID               string      `json:"id"`
	Type             PatternType `json:"type"`
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	ToolSequence     []string    `json:"tool_sequence"`
	Frequency        int         `json:"frequency"`
	AvgTokenCost     int64       `json:"avg_token_cost"`
	SuccessRate      float64     `json:"success_rate"`
	TimeContext      string      `json:"time_context,omitempty"` // morning, afternoon, weekly
	Confidence       float64     `json:"confidence"`
	FirstSeen        time.Time   `json:"first_seen"`
	LastSeen         time.Time   `json:"last_seen"`
	SessionCount     int         `json:"session_count"`
	OptimizationHint string      `json:"optimization_hint,omitempty"`
	Examples         []PatternExample `json:"examples,omitempty"`
}

// PatternExample shows a specific occurrence of a pattern
type PatternExample struct {
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Tools     []string  `json:"tools"`
	Success   bool      `json:"success"`
}

// PatternMiner analyzes session history to discover patterns
type PatternMiner struct {
	sessionDir   string
	patternsFile string
	minFrequency int
	minSequence  int
	maxSequence  int
}

// NewPatternMiner creates a new pattern miner
func NewPatternMiner(sessionDir string) *PatternMiner {
	if sessionDir == "" {
		home, _ := os.UserHomeDir()
		cwd, _ := os.Getwd()
		encoded := strings.ReplaceAll(cwd, "/", "-")
		sessionDir = filepath.Join(home, ".claude", "projects", encoded)
	}

	home, _ := os.UserHomeDir()
	patternsFile := filepath.Join(home, ".config", "webb", "patterns.json")

	return &PatternMiner{
		sessionDir:   sessionDir,
		patternsFile: patternsFile,
		minFrequency: 3,  // Pattern must occur at least 3 times
		minSequence:  2,  // Minimum 2 tools in sequence
		maxSequence:  6,  // Maximum 6 tools in sequence
	}
}

// ToolCall represents a single tool invocation from session history
type ToolCall struct {
	Name      string
	Timestamp time.Time
	Success   bool
	SessionID string
}

// MinePatterns analyzes sessions and returns discovered patterns
func (m *PatternMiner) MinePatterns() ([]DiscoveredPattern, error) {
	// Parse all sessions to extract tool calls
	toolCalls, err := m.extractToolCalls()
	if err != nil {
		return nil, fmt.Errorf("extracting tool calls: %w", err)
	}

	if len(toolCalls) == 0 {
		return nil, nil
	}

	var patterns []DiscoveredPattern

	// Mine sequential patterns
	seqPatterns := m.mineSequentialPatterns(toolCalls)
	patterns = append(patterns, seqPatterns...)

	// Mine co-occurrence patterns
	coPatterns := m.mineCoOccurrencePatterns(toolCalls)
	patterns = append(patterns, coPatterns...)

	// Mine temporal patterns
	tempPatterns := m.mineTemporalPatterns(toolCalls)
	patterns = append(patterns, tempPatterns...)

	// Sort by frequency
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Frequency > patterns[j].Frequency
	})

	return patterns, nil
}

// extractToolCalls parses session files and extracts tool calls
func (m *PatternMiner) extractToolCalls() ([]ToolCall, error) {
	files, err := filepath.Glob(filepath.Join(m.sessionDir, "*.jsonl"))
	if err != nil {
		return nil, err
	}

	var allCalls []ToolCall

	for _, file := range files {
		calls, err := m.parseSessionFile(file)
		if err != nil {
			continue // Skip problematic files
		}
		allCalls = append(allCalls, calls...)
	}

	return allCalls, nil
}

// SessionMessage for parsing JSONL
type sessionMessage struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   *struct {
		Content []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content"`
	} `json:"message"`
}

func (m *PatternMiner) parseSessionFile(path string) ([]ToolCall, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	sessionID := filepath.Base(path)
	var calls []ToolCall

	scanner := bufio.NewScanner(file)
	// Increase buffer size for large lines
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		var msg sessionMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		if msg.Message == nil {
			continue
		}

		ts, _ := time.Parse(time.RFC3339, msg.Timestamp)

		for _, content := range msg.Message.Content {
			if content.Type == "tool_use" && strings.HasPrefix(content.Name, "webb_") {
				calls = append(calls, ToolCall{
					Name:      content.Name,
					Timestamp: ts,
					Success:   true, // Simplified - assume success
					SessionID: sessionID,
				})
			}
		}
	}

	return calls, nil
}

// mineSequentialPatterns finds ordered sequences of tools
func (m *PatternMiner) mineSequentialPatterns(calls []ToolCall) []DiscoveredPattern {
	// Group calls by session
	sessions := make(map[string][]ToolCall)
	for _, call := range calls {
		sessions[call.SessionID] = append(sessions[call.SessionID], call)
	}

	// Count n-gram sequences
	sequenceCounts := make(map[string]int)
	sequenceExamples := make(map[string][]PatternExample)
	sequenceFirstSeen := make(map[string]time.Time)
	sequenceLastSeen := make(map[string]time.Time)
	sequenceSessions := make(map[string]map[string]bool)

	for sessionID, sessionCalls := range sessions {
		// Sort by timestamp
		sort.Slice(sessionCalls, func(i, j int) bool {
			return sessionCalls[i].Timestamp.Before(sessionCalls[j].Timestamp)
		})

		// Extract n-grams of various lengths
		for n := m.minSequence; n <= m.maxSequence && n <= len(sessionCalls); n++ {
			for i := 0; i <= len(sessionCalls)-n; i++ {
				seq := make([]string, n)
				for j := 0; j < n; j++ {
					seq[j] = sessionCalls[i+j].Name
				}

				// Skip if all same tool
				allSame := true
				for _, s := range seq {
					if s != seq[0] {
						allSame = false
						break
					}
				}
				if allSame {
					continue
				}

				key := strings.Join(seq, " → ")
				sequenceCounts[key]++

				// Track metadata
				if sequenceSessions[key] == nil {
					sequenceSessions[key] = make(map[string]bool)
				}
				sequenceSessions[key][sessionID] = true

				ts := sessionCalls[i].Timestamp
				if sequenceFirstSeen[key].IsZero() || ts.Before(sequenceFirstSeen[key]) {
					sequenceFirstSeen[key] = ts
				}
				if ts.After(sequenceLastSeen[key]) {
					sequenceLastSeen[key] = ts
				}

				// Store example (limit to 3)
				if len(sequenceExamples[key]) < 3 {
					sequenceExamples[key] = append(sequenceExamples[key], PatternExample{
						SessionID: sessionID,
						Timestamp: ts,
						Tools:     seq,
						Success:   true,
					})
				}
			}
		}
	}

	// Convert to patterns
	var patterns []DiscoveredPattern
	for seq, count := range sequenceCounts {
		if count < m.minFrequency {
			continue
		}

		tools := strings.Split(seq, " → ")

		// Generate a readable name
		name := generatePatternName(tools)

		patterns = append(patterns, DiscoveredPattern{
			ID:           fmt.Sprintf("seq-%s", hashSequence(seq)),
			Type:         PatternSequential,
			Name:         name,
			Description:  fmt.Sprintf("Sequential workflow: %s", seq),
			ToolSequence: tools,
			Frequency:    count,
			Confidence:   float64(count) / float64(len(sessions)),
			FirstSeen:    sequenceFirstSeen[seq],
			LastSeen:     sequenceLastSeen[seq],
			SessionCount: len(sequenceSessions[seq]),
			Examples:     sequenceExamples[seq],
		})
	}

	return patterns
}

// mineCoOccurrencePatterns finds tools frequently used together
func (m *PatternMiner) mineCoOccurrencePatterns(calls []ToolCall) []DiscoveredPattern {
	// Group by session
	sessions := make(map[string]map[string]bool)
	for _, call := range calls {
		if sessions[call.SessionID] == nil {
			sessions[call.SessionID] = make(map[string]bool)
		}
		sessions[call.SessionID][call.Name] = true
	}

	// Count pairs
	pairCounts := make(map[string]int)
	for _, tools := range sessions {
		toolList := make([]string, 0, len(tools))
		for t := range tools {
			toolList = append(toolList, t)
		}
		sort.Strings(toolList)

		// Generate pairs
		for i := 0; i < len(toolList); i++ {
			for j := i + 1; j < len(toolList); j++ {
				key := toolList[i] + " + " + toolList[j]
				pairCounts[key]++
			}
		}
	}

	// Convert high-frequency pairs to patterns
	var patterns []DiscoveredPattern
	for pair, count := range pairCounts {
		if count < m.minFrequency*2 { // Higher threshold for co-occurrence
			continue
		}

		tools := strings.Split(pair, " + ")

		patterns = append(patterns, DiscoveredPattern{
			ID:           fmt.Sprintf("cooccur-%s", hashSequence(pair)),
			Type:         PatternCoOccur,
			Name:         fmt.Sprintf("%s with %s", simplifyToolName(tools[0]), simplifyToolName(tools[1])),
			Description:  fmt.Sprintf("Tools frequently used together: %s", pair),
			ToolSequence: tools,
			Frequency:    count,
			Confidence:   float64(count) / float64(len(sessions)),
			SessionCount: count,
		})
	}

	return patterns
}

// mineTemporalPatterns finds time-based patterns
func (m *PatternMiner) mineTemporalPatterns(calls []ToolCall) []DiscoveredPattern {
	// Group calls by hour of day
	hourCounts := make(map[int]map[string]int)
	for i := 0; i < 24; i++ {
		hourCounts[i] = make(map[string]int)
	}

	for _, call := range calls {
		hour := call.Timestamp.Hour()
		hourCounts[hour][call.Name]++
	}

	// Find tools with strong time preferences
	var patterns []DiscoveredPattern

	// Morning tools (6-10am)
	morningTools := make(map[string]int)
	for hour := 6; hour <= 10; hour++ {
		for tool, count := range hourCounts[hour] {
			morningTools[tool] += count
		}
	}

	for tool, count := range morningTools {
		if count >= m.minFrequency*3 {
			patterns = append(patterns, DiscoveredPattern{
				ID:           fmt.Sprintf("temp-morning-%s", hashSequence(tool)),
				Type:         PatternTemporal,
				Name:         fmt.Sprintf("Morning: %s", simplifyToolName(tool)),
				Description:  fmt.Sprintf("%s is frequently used in the morning (6-10am)", tool),
				ToolSequence: []string{tool},
				Frequency:    count,
				TimeContext:  "morning",
				Confidence:   0.7,
			})
		}
	}

	return patterns
}

// Helper functions

func generatePatternName(tools []string) string {
	if len(tools) == 0 {
		return "Unknown"
	}

	// Extract common prefixes
	categories := make(map[string]int)
	for _, t := range tools {
		parts := strings.Split(strings.TrimPrefix(t, "webb_"), "_")
		if len(parts) > 0 {
			categories[parts[0]]++
		}
	}

	// Find dominant category
	var dominant string
	maxCount := 0
	for cat, count := range categories {
		if count > maxCount {
			dominant = cat
			maxCount = count
		}
	}

	if maxCount == len(tools) {
		return fmt.Sprintf("%s workflow (%d steps)", dominant, len(tools))
	}

	return fmt.Sprintf("Multi-tool workflow (%d steps)", len(tools))
}

func simplifyToolName(name string) string {
	name = strings.TrimPrefix(name, "webb_")
	parts := strings.Split(name, "_")
	if len(parts) >= 2 {
		return parts[0] + " " + parts[1]
	}
	return name
}

func hashSequence(s string) string {
	// Simple hash for ID generation
	h := 0
	for _, c := range s {
		h = 31*h + int(c)
	}
	if h < 0 {
		h = -h
	}
	return fmt.Sprintf("%x", h)[:8]
}

// SavePatterns persists patterns to disk
func (m *PatternMiner) SavePatterns(patterns []DiscoveredPattern) error {
	if err := os.MkdirAll(filepath.Dir(m.patternsFile), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(patterns, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.patternsFile, data, 0644)
}

// LoadPatterns loads previously discovered patterns
func (m *PatternMiner) LoadPatterns() ([]DiscoveredPattern, error) {
	data, err := os.ReadFile(m.patternsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var patterns []DiscoveredPattern
	if err := json.Unmarshal(data, &patterns); err != nil {
		return nil, err
	}

	return patterns, nil
}

// FormatPatternSummary returns a markdown summary of patterns
func FormatPatternSummary(patterns []DiscoveredPattern) string {
	var sb strings.Builder

	sb.WriteString("# Discovered Workflow Patterns\n\n")

	if len(patterns) == 0 {
		sb.WriteString("No patterns discovered yet. Use more tools to build up pattern data.\n")
		return sb.String()
	}

	// Group by type
	byType := make(map[PatternType][]DiscoveredPattern)
	for _, p := range patterns {
		byType[p.Type] = append(byType[p.Type], p)
	}

	// Sequential patterns
	if seqs := byType[PatternSequential]; len(seqs) > 0 {
		sb.WriteString("## Sequential Workflows\n\n")
		sb.WriteString("Tools used in specific order:\n\n")
		sb.WriteString("| Pattern | Frequency | Sessions | Confidence |\n")
		sb.WriteString("|---------|-----------|----------|------------|\n")
		for _, p := range seqs[:min(10, len(seqs))] {
			sb.WriteString(fmt.Sprintf("| %s | %d | %d | %.0f%% |\n",
				p.Name, p.Frequency, p.SessionCount, p.Confidence*100))
		}
		sb.WriteString("\n")
	}

	// Co-occurrence patterns
	if coocs := byType[PatternCoOccur]; len(coocs) > 0 {
		sb.WriteString("## Co-occurrence Patterns\n\n")
		sb.WriteString("Tools frequently used together:\n\n")
		for _, p := range coocs[:min(5, len(coocs))] {
			sb.WriteString(fmt.Sprintf("- **%s** (%d sessions)\n", p.Name, p.Frequency))
		}
		sb.WriteString("\n")
	}

	// Temporal patterns
	if temps := byType[PatternTemporal]; len(temps) > 0 {
		sb.WriteString("## Time-Based Patterns\n\n")
		for _, p := range temps[:min(5, len(temps))] {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", p.TimeContext, p.Name))
		}
		sb.WriteString("\n")
	}

	// Summary stats
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Patterns:** %d\n", len(patterns)))
	sb.WriteString(fmt.Sprintf("- **Sequential:** %d\n", len(byType[PatternSequential])))
	sb.WriteString(fmt.Sprintf("- **Co-occurrence:** %d\n", len(byType[PatternCoOccur])))
	sb.WriteString(fmt.Sprintf("- **Temporal:** %d\n", len(byType[PatternTemporal])))

	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
