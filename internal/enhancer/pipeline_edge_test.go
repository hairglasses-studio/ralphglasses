package enhancer

import (
	"strings"
	"testing"
)

func TestScore_EmptyInput(t *testing.T) {
	t.Parallel()

	lints := Lint("")
	ar := &AnalyzeResult{}
	report := Score("", TaskTypeGeneral, lints, ar, ProviderClaude)
	if report == nil {
		t.Fatal("Score returned nil for empty input")
	}
	if report.Grade == "" {
		t.Error("expected a non-empty grade")
	}
	if len(report.Dimensions) == 0 {
		t.Error("expected dimensions to be populated")
	}
}

func TestScore_AllProviders(t *testing.T) {
	t.Parallel()

	text := "Write a Go function that sorts a slice of integers using quicksort with proper error handling"
	lints := Lint(text)
	ar := &AnalyzeResult{}

	for _, provider := range []ProviderName{ProviderClaude, ProviderGemini, ProviderOpenAI} {
		t.Run(string(provider), func(t *testing.T) {
			report := Score(text, TaskTypeCode, lints, ar, provider)
			if report == nil {
				t.Fatalf("Score returned nil for provider %s", provider)
			}
			if report.Overall < 0 || report.Overall > 100 {
				t.Errorf("overall score out of range: %d", report.Overall)
			}
		})
	}
}

func TestScore_AllTaskTypes(t *testing.T) {
	t.Parallel()

	text := "Analyze the performance of our API endpoints and create a report with recommendations"
	lints := Lint(text)
	ar := &AnalyzeResult{}

	for _, tt := range []TaskType{TaskTypeCode, TaskTypeAnalysis, TaskTypeCreative, TaskTypeWorkflow, TaskTypeGeneral} {
		t.Run(string(tt), func(t *testing.T) {
			report := Score(text, tt, lints, ar, ProviderClaude)
			if report == nil {
				t.Fatalf("Score returned nil for task type %s", tt)
			}
		})
	}
}

func TestLint_NoFindings(t *testing.T) {
	t.Parallel()

	// A well-formed prompt should have few or no lint findings.
	text := "Write a Go function that takes a slice of integers and returns the sorted result."
	results := Lint(text)
	// We just verify it doesn't panic — we don't assert zero findings
	// since lint rules may flag various things.
	_ = results
}

func TestLint_MultipleIssues(t *testing.T) {
	t.Parallel()

	// Prompt with known lint-triggering patterns.
	text := `NEVER use global variables.
DO NOT forget to handle errors.
Always use proper naming conventions.
Make sure everything is correct.
IMPORTANT: ALWAYS follow the rules.`

	results := Lint(text)
	if len(results) == 0 {
		t.Error("expected at least one lint finding for problematic prompt")
	}

	// Check that results have required fields populated.
	for i, r := range results {
		if r.Category == "" {
			t.Errorf("result[%d]: empty category", i)
		}
		if r.Severity == "" {
			t.Errorf("result[%d]: empty severity", i)
		}
	}
}

func TestEnhanceWithConfig_DisabledStages(t *testing.T) {
	t.Parallel()

	cfg := Config{
		DisabledStages: []string{"specificity", "positive_reframe", "tone_downgrade", "structure"},
	}
	result := EnhanceWithConfig("write a function to sort users by name with error handling", TaskTypeCode, cfg)
	if result.Enhanced == "" {
		t.Error("should produce output even with many stages disabled")
	}
	for _, stage := range cfg.DisabledStages {
		for _, ran := range result.StagesRun {
			if ran == stage {
				t.Errorf("stage %q should have been disabled but was run", stage)
			}
		}
	}
}

func TestEnhance_PreservesTaskType(t *testing.T) {
	t.Parallel()

	for _, tt := range []TaskType{TaskTypeCode, TaskTypeAnalysis, TaskTypeCreative, TaskTypeWorkflow, TaskTypeGeneral} {
		t.Run(string(tt), func(t *testing.T) {
			result := Enhance("do something interesting with the data", tt)
			if result.TaskType != tt {
				t.Errorf("expected TaskType=%s, got %s", tt, result.TaskType)
			}
		})
	}
}

func TestAnalyze_DetectsStructure(t *testing.T) {
	t.Parallel()

	t.Run("with_xml", func(t *testing.T) {
		input := `<instructions>Write a function</instructions>
<context>This is for the user service</context>`
		result := Analyze(input)
		if !result.HasXML {
			t.Error("expected HasXML=true for input with XML tags")
		}
		if !result.HasContext {
			t.Error("expected HasContext=true for input with context tag")
		}
	})

	t.Run("plain_text", func(t *testing.T) {
		result := Analyze("write a simple hello world function")
		if result.HasXML {
			t.Error("expected HasXML=false for plain text")
		}
	})
}

func TestAnalyze_WordCount(t *testing.T) {
	t.Parallel()

	result := Analyze("one two three four five")
	if result.WordCount != 5 {
		t.Errorf("expected WordCount=5, got %d", result.WordCount)
	}
}

func TestEnhance_WhitespaceOnly(t *testing.T) {
	t.Parallel()

	result := Enhance("   \t\n  ", TaskTypeGeneral)
	// Should not panic. The result may be empty or whitespace.
	if result.TaskType != TaskTypeGeneral {
		t.Errorf("expected general task type, got %q", result.TaskType)
	}
}

func TestClassify_KnownPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected TaskType
	}{
		{"create a new API endpoint for user management", TaskTypeCode},
		{"implement the database migration", TaskTypeCode},
		{"write a test for the authentication module", TaskTypeCode},
	}

	for _, tc := range tests {
		t.Run(tc.input[:20], func(t *testing.T) {
			got := Classify(tc.input)
			if got != tc.expected {
				t.Errorf("Classify(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestEstimateTokens_Boundaries(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		tokens := EstimateTokens("")
		if tokens != 0 {
			t.Errorf("expected 0 tokens for empty string, got %d", tokens)
		}
	})

	t.Run("single_word", func(t *testing.T) {
		tokens := EstimateTokens("hello")
		if tokens <= 0 {
			t.Errorf("expected positive tokens for single word, got %d", tokens)
		}
	})

	t.Run("scales_with_input", func(t *testing.T) {
		short := EstimateTokens("hello world")
		long := EstimateTokens(strings.Repeat("hello world ", 100))
		if long <= short {
			t.Errorf("longer input should have more tokens: short=%d, long=%d", short, long)
		}
	})
}
