package session

import (
	"strings"
	"testing"
)

// mockEpisodicSource implements EpisodicSource for testing.
type mockEpisodicSource struct {
	episodes []CurriculumEpisode
}

func (m *mockEpisodicSource) FindSimilarEpisodes(taskType, prompt string, k int) []CurriculumEpisode {
	if k > len(m.episodes) {
		return m.episodes
	}
	return m.episodes[:k]
}

func TestScoreTask_EasyTask(t *testing.T) {
	cs := NewCurriculumSorter(nil, nil)
	task := LoopTask{
		Title:  "add unit test for parser",
		Prompt: "Write a simple test for the parser function",
	}
	td := cs.ScoreTask(task)

	if td.DifficultyScore >= 0.4 {
		t.Errorf("expected easy task score < 0.4, got %f", td.DifficultyScore)
	}
	if td.Recommendation != "cheap_provider" {
		t.Errorf("expected recommendation cheap_provider, got %s", td.Recommendation)
	}
	if td.TaskType == "" {
		t.Error("expected non-empty task type")
	}
}

func TestScoreTask_HardTask(t *testing.T) {
	mock := &mockEpisodicSource{
		episodes: []CurriculumEpisode{
			{TurnCount: 25, CostUSD: 0.80},
			{TurnCount: 30, CostUSD: 1.20},
			{TurnCount: 22, CostUSD: 0.90},
		},
	}
	cs := NewCurriculumSorter(nil, mock)
	task := LoopTask{
		Title:  "redesign authentication architecture",
		Prompt: "This is a complex overhaul that requires changes across multiple files and involves a breaking change to the auth module. " +
			"We need to migrate the existing token-based system to a new JWT approach with backward compatibility considerations. " +
			"The redesign should handle session management, token refresh, provider federation, and role-based access control. " +
			"Consider the impact on all downstream services that depend on the current auth interface and their integration points. " +
			"Each component needs thorough testing and documentation updates to reflect the new authentication flow. " +
			"The migration path must support zero-downtime deployment with a feature flag rollout strategy. " +
			"We also need to update the client SDKs, API documentation, and integration test suites for all consumers. " +
			"Security review and penetration testing should be planned for the new token validation and refresh mechanisms. " +
			"Database schema changes for token storage need careful migration scripts with rollback procedures. " +
			"Performance benchmarks comparing the old and new auth paths should be established before the cutover. " +
			"The overall architecture document needs a complete rewrite to reflect the new system design patterns. " +
			"Coordinate with the infrastructure team for load balancer and CDN configuration changes required by the new auth headers.",
	}
	td := cs.ScoreTask(task)

	if td.DifficultyScore <= 0.7 {
		t.Errorf("expected hard task score > 0.7, got %f", td.DifficultyScore)
	}
}

func TestScoreTask_NilFeedback(t *testing.T) {
	cs := NewCurriculumSorter(nil, nil)
	task := LoopTask{
		Title:  "update logging configuration",
		Prompt: "Change the log level from debug to info in production config",
	}
	td := cs.ScoreTask(task)

	if td.DifficultyScore < 0 || td.DifficultyScore > 1 {
		t.Errorf("score out of range [0,1]: %f", td.DifficultyScore)
	}
	if td.Recommendation == "" {
		t.Error("expected non-empty recommendation")
	}
}

func TestSortTasks(t *testing.T) {
	cs := NewCurriculumSorter(nil, nil)
	tasks := []LoopTask{
		{Title: "redesign complex architecture", Prompt: "overhaul the entire system across multiple files with breaking changes and migration"},
		{Title: "add simple unit test", Prompt: "write a trivial test"},
		{Title: "fix login bug", Prompt: "the login page crashes when password is empty"},
	}

	sorted := cs.SortTasks(tasks)

	// Verify input not mutated
	if tasks[0].Title != "redesign complex architecture" {
		t.Error("input slice was mutated")
	}

	// Verify ordering: test (easy) should be first, architecture (hard) should be last
	if !strings.Contains(sorted[0].Title, "test") {
		t.Errorf("expected easiest task first, got %q", sorted[0].Title)
	}
	if !strings.Contains(sorted[len(sorted)-1].Title, "architecture") {
		t.Errorf("expected hardest task last, got %q", sorted[len(sorted)-1].Title)
	}

	// Verify scores are ascending
	for i := 1; i < len(sorted); i++ {
		prev := cs.ScoreTask(sorted[i-1]).DifficultyScore
		curr := cs.ScoreTask(sorted[i]).DifficultyScore
		if prev > curr {
			t.Errorf("tasks not sorted ascending: index %d score %f > index %d score %f", i-1, prev, i, curr)
		}
	}
}

func TestShouldDecompose(t *testing.T) {
	cs := NewCurriculumSorter(nil, nil)

	tests := []struct {
		name     string
		diff     TaskDifficulty
		expected bool
	}{
		{
			name:     "high_difficulty_low_success",
			diff:     TaskDifficulty{DifficultyScore: 0.85, HistoricalSuccessRate: 0.3, SampleCount: 5},
			expected: true,
		},
		{
			name:     "high_difficulty_high_success",
			diff:     TaskDifficulty{DifficultyScore: 0.85, HistoricalSuccessRate: 0.8, SampleCount: 5},
			expected: false,
		},
		{
			name:     "low_difficulty_low_success",
			diff:     TaskDifficulty{DifficultyScore: 0.5, HistoricalSuccessRate: 0.3, SampleCount: 5},
			expected: false,
		},
		{
			name:     "high_difficulty_low_samples",
			diff:     TaskDifficulty{DifficultyScore: 0.85, HistoricalSuccessRate: 0.9, SampleCount: 2},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cs.ShouldDecompose(tc.diff)
			if got != tc.expected {
				t.Errorf("ShouldDecompose(%+v) = %v, want %v", tc.diff, got, tc.expected)
			}
		})
	}
}

func TestDecompositionPrompt(t *testing.T) {
	cs := NewCurriculumSorter(nil, nil)
	task := LoopTask{
		Title:  "redesign auth system",
		Prompt: "Migrate from sessions to JWT tokens",
	}

	prompt := cs.DecompositionPrompt(task)

	if !strings.Contains(prompt, task.Title) {
		t.Error("decomposition prompt should contain the task title")
	}
	if !strings.Contains(prompt, "2-3 smaller") {
		t.Error("decomposition prompt should mention breaking into 2-3 smaller sub-tasks")
	}
	if !strings.Contains(prompt, "independently") {
		t.Error("decomposition prompt should mention independent sub-tasks")
	}
	if !strings.Contains(prompt, task.Prompt) {
		t.Error("decomposition prompt should contain the task prompt")
	}
}

func TestPromptComplexity(t *testing.T) {
	tests := []struct {
		name  string
		words int
		extra string
		minScore float64
		maxScore float64
	}{
		{"very short", 5, "", 0.0, 0.3},
		{"short", 30, "", 0.3, 0.5},
		{"medium", 75, "", 0.5, 0.7},
		{"long", 150, "", 0.6, 0.8},
		{"very long", 250, "", 0.7, 0.9},
		{"multi files", 30, " across multiple files", 0.4, 0.6},
		{"breaking change", 30, " breaking change and migration", 0.5, 0.7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := strings.Repeat("word ", tt.words) + tt.extra
			score := promptComplexity(prompt)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("promptComplexity(%d words + %q) = %f, want [%f, %f]",
					tt.words, tt.extra, score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestWordCount(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
		{"  spaced  out  words  ", 3},
		{"one two three four five", 5},
		{"tabs\tand\nnewlines", 3},
	}

	for _, tc := range tests {
		got := wordCount(tc.input)
		if got != tc.expected {
			t.Errorf("wordCount(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

func TestSortTasks_MultiTaskDiverseTypes(t *testing.T) {
	cs := NewCurriculumSorter(nil, nil)

	tasks := []LoopTask{
		{Title: "add unit test for config parser", Prompt: "Write a simple test for the config parser function"},
		{Title: "update API docs for auth endpoints", Prompt: "Add docs comments to the authentication handlers"},
		{Title: "fix null pointer crash on login", Prompt: "The login handler panics when email field is nil"},
		{Title: "refactor database connection pooling", Prompt: "Extract connection pool logic into a separate module with breaking changes across multiple files"},
		{Title: "implement real-time notification system", Prompt: "Design and build a complex WebSocket-based notification system with pub/sub, delivery guarantees, and backward compatibility considerations"},
	}

	sorted := cs.SortTasks(tasks)

	// Verify length preserved
	if len(sorted) != len(tasks) {
		t.Fatalf("expected %d tasks, got %d", len(tasks), len(sorted))
	}

	// Verify ascending difficulty order
	for i := 1; i < len(sorted); i++ {
		prev := cs.ScoreTask(sorted[i-1]).DifficultyScore
		curr := cs.ScoreTask(sorted[i]).DifficultyScore
		if prev > curr {
			t.Errorf("tasks not sorted ascending: index %d (%q, score %.3f) > index %d (%q, score %.3f)",
				i-1, sorted[i-1].Title, prev, i, sorted[i].Title, curr)
		}
	}

	// The test/docs tasks (easy) should appear before the feature/refactor tasks (hard)
	firstScore := cs.ScoreTask(sorted[0]).DifficultyScore
	lastScore := cs.ScoreTask(sorted[len(sorted)-1]).DifficultyScore
	if firstScore >= lastScore {
		t.Errorf("first task score (%.3f) should be less than last task score (%.3f)", firstScore, lastScore)
	}

	// Verify ShouldDecompose triggers for a task with difficulty > 0.8.
	// We construct a TaskDifficulty directly since exceeding 0.8 via ScoreTask
	// requires feedback analyzer data (historical failure rate). The scoring weights
	// cap pure heuristic scoring around 0.75 without real history.
	decomposeDiff := TaskDifficulty{
		DifficultyScore:       0.85,
		HistoricalSuccessRate: 0.3,
		SampleCount:           2, // low sample count triggers decompose
	}
	if !cs.ShouldDecompose(decomposeDiff) {
		t.Errorf("expected ShouldDecompose=true for task with difficulty %.3f and sample_count=%d",
			decomposeDiff.DifficultyScore, decomposeDiff.SampleCount)
	}

	// Verify the hardest scored task from our diverse list is at least moderately hard
	hardestScore := cs.ScoreTask(sorted[len(sorted)-1]).DifficultyScore
	if hardestScore < 0.5 {
		t.Errorf("expected hardest task score >= 0.5, got %.3f", hardestScore)
	}
}

func TestScoreTask_WithEpisodicSource(t *testing.T) {
	mock := &mockEpisodicSource{
		episodes: []CurriculumEpisode{
			{TurnCount: 25, CostUSD: 0.50},
			{TurnCount: 30, CostUSD: 0.60},
		},
	}
	cs := NewCurriculumSorter(nil, mock)
	task := LoopTask{
		Title:  "implement new feature",
		Prompt: "Add a complex new capability",
	}

	td := cs.ScoreTask(task)

	// With high-turn episodic evidence, score should be higher
	if td.DifficultyScore < 0.4 {
		t.Errorf("expected higher score with high-turn episodes, got %f", td.DifficultyScore)
	}
}
