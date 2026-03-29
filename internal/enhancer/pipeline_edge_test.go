package enhancer

import (
	"strings"
	"testing"
)

// --- Existing tests (preserved) ---

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

// --- Phase 0.6.1.4: Pipeline edge case tests ---

func TestPipelineEdge_EmptyString(t *testing.T) {
	t.Parallel()

	t.Run("enhance_empty", func(t *testing.T) {
		result := Enhance("", TaskTypeGeneral)
		// Must not panic; may produce empty or minimal output
		if result.TaskType != TaskTypeGeneral {
			t.Errorf("expected general task type, got %q", result.TaskType)
		}
	})

	t.Run("analyze_empty", func(t *testing.T) {
		ar := Analyze("")
		if ar.ScoreReport == nil {
			t.Fatal("ScoreReport should not be nil for empty input")
		}
		if ar.ScoreReport.Overall > 45 {
			t.Errorf("empty input scored %d, expected <= 45", ar.ScoreReport.Overall)
		}
	})

	t.Run("lint_empty", func(t *testing.T) {
		lints := Lint("")
		// Should not panic; may return empty or minimal findings
		_ = lints
	})

	t.Run("classify_empty", func(t *testing.T) {
		tt := Classify("")
		if tt != TaskTypeGeneral {
			t.Errorf("empty input classified as %q, expected general", tt)
		}
	})
}

func TestPipelineEdge_VeryLongInput(t *testing.T) {
	t.Parallel()

	// Generate a 100K+ character string
	longInput := strings.Repeat("Write a Go function that processes user data and handles errors correctly. ", 1500)
	if len(longInput) < 100_000 {
		t.Fatalf("generated input is only %d chars, need 100K+", len(longInput))
	}

	t.Run("enhance_long", func(t *testing.T) {
		result := Enhance(longInput, TaskTypeCode)
		if result.Enhanced == "" {
			t.Error("expected non-empty output for long input")
		}
		if result.EstimatedTokens == 0 {
			t.Error("expected non-zero token estimate for long input")
		}
	})

	t.Run("analyze_long", func(t *testing.T) {
		ar := Analyze(longInput)
		if ar.ScoreReport == nil {
			t.Fatal("ScoreReport should not be nil for long input")
		}
		// Long input should have decent word count
		if ar.WordCount < 1000 {
			t.Errorf("WordCount = %d, expected > 1000 for long input", ar.WordCount)
		}
	})

	t.Run("score_long", func(t *testing.T) {
		lints := Lint(longInput)
		ar := &AnalyzeResult{WordCount: len(strings.Fields(longInput))}
		report := Score(longInput, TaskTypeCode, lints, ar, ProviderClaude)
		if report == nil {
			t.Fatal("Score returned nil for long input")
		}
		for _, d := range report.Dimensions {
			if d.Score < 0 || d.Score > 100 {
				t.Errorf("dimension %q score %d out of range", d.Name, d.Score)
			}
		}
	})
}

func TestPipelineEdge_UnicodeHeavy(t *testing.T) {
	t.Parallel()

	// CJK characters, emoji, RTL text, combining characters
	unicodeInput := "Write a function that handles multilingual data including " +
		"\u4f60\u597d\u4e16\u754c (Chinese), " + // 你好世界
		"\u3053\u3093\u306b\u3061\u306f (Japanese), " + // こんにちは
		"\U0001F680\U0001F31F\U0001F4BB (emoji), " + // 🚀🌟💻
		"\u0645\u0631\u062d\u0628\u0627 (Arabic RTL), " + // مرحبا
		"and Z\u0301\u0302\u0303\u0304\u0305 (combining marks). " +
		"Process each script correctly with proper encoding."

	t.Run("enhance_unicode", func(t *testing.T) {
		result := Enhance(unicodeInput, TaskTypeCode)
		if result.Enhanced == "" {
			t.Error("expected non-empty output for unicode input")
		}
		// Should preserve the unicode content
		if !strings.Contains(result.Enhanced, "\u4f60\u597d") {
			t.Error("Chinese characters should be preserved in output")
		}
	})

	t.Run("analyze_unicode", func(t *testing.T) {
		ar := Analyze(unicodeInput)
		if ar.ScoreReport == nil {
			t.Fatal("ScoreReport should not be nil for unicode input")
		}
		if ar.WordCount == 0 {
			t.Error("expected non-zero word count for unicode input")
		}
	})

	t.Run("lint_unicode", func(t *testing.T) {
		// Should not panic on unicode-heavy text
		lints := Lint(unicodeInput)
		_ = lints
	})
}

func TestPipelineEdge_InputWithXMLTags(t *testing.T) {
	t.Parallel()

	// Input that already contains pipeline-relevant XML tags
	xmlInput := `<verification>
This prompt already has a verification section.
Check that all assertions pass.
</verification>

<context>
The application processes financial transactions.
Each transaction has an amount, currency, and timestamp.
</context>

<example>
Input: {"amount": 100, "currency": "USD"}
Output: Transaction processed successfully.
</example>

Write a function to validate transaction amounts are positive and currencies are supported.`

	t.Run("enhance_does_not_panic", func(t *testing.T) {
		result := Enhance(xmlInput, TaskTypeCode)
		// Must produce output without panicking on input with existing XML tags
		if result.Enhanced == "" {
			t.Error("expected non-empty output for XML-tagged input")
		}
		// Context tag should be preserved
		if !strings.Contains(result.Enhanced, "financial transactions") {
			t.Error("should preserve content from <context> section")
		}
	})

	t.Run("analyze_detects_structure", func(t *testing.T) {
		ar := Analyze(xmlInput)
		if !ar.HasContext {
			t.Error("expected HasContext=true for input with <context> tag")
		}
		if !ar.HasExamples {
			t.Error("expected HasExamples=true for input with <example> tag")
		}
	})

	t.Run("score_rewards_structure", func(t *testing.T) {
		ar := Analyze(xmlInput)
		structure := findDimension(ar.ScoreReport, "Structure")
		if structure.Score < 50 {
			t.Errorf("structured XML input scored %d on Structure, expected >= 50", structure.Score)
		}
	})
}

func TestPipelineEdge_WellStructuredInput(t *testing.T) {
	t.Parallel()

	// Already well-structured prompt should not be degraded
	wellStructured := `<role>You are an expert database engineer with 15 years of PostgreSQL experience.</role>

<context>
We are migrating from MySQL to PostgreSQL because we need better JSON support
and more advanced indexing capabilities.
</context>

<instructions>
Review this SQL migration script for correctness.
Check for data type mismatches, missing indexes, and foreign key constraints.
Return exactly 3 findings sorted by severity.
</instructions>

<examples>
<example index="1">
Finding: VARCHAR(255) should be TEXT in PostgreSQL — no performance difference, simpler.
Severity: Low
</example>
<example index="2">
Finding: Missing GIN index on JSONB column — queries will full-scan.
Severity: High
</example>
<example index="3">
Finding: ON DELETE CASCADE missing on child table FK — orphaned rows on parent delete.
Severity: Critical
</example>
</examples>

<output_format>
Numbered list with severity, finding, and recommended fix.
</output_format>

<constraints>
- Only flag issues specific to the MySQL-to-PostgreSQL migration
- Ignore cosmetic differences between dialects
</constraints>`

	t.Run("enhance_preserves_quality", func(t *testing.T) {
		result := Enhance(wellStructured, TaskTypeCode)
		// Should not strip or damage existing structure
		if !strings.Contains(result.Enhanced, "expert database engineer") {
			t.Error("should preserve role definition")
		}
	})

	t.Run("score_is_high", func(t *testing.T) {
		ar := Analyze(wellStructured)
		if ar.ScoreReport.Overall < 65 {
			t.Errorf("well-structured prompt scored %d, expected >= 65", ar.ScoreReport.Overall)
			for _, d := range ar.ScoreReport.Dimensions {
				t.Logf("  %s: %d (%s)", d.Name, d.Score, d.Grade)
			}
		}
	})
}
