package enhancer

import (
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		prompt   string
		expected TaskType
	}{
		{"troubleshooting_fix", "fix this broken function", TaskTypeTroubleshooting},
		{"troubleshooting_debug", "debug the timeout error", TaskTypeTroubleshooting},
		{"code_create", "create a new API endpoint", TaskTypeCode},
		{"code_implement", "implement the user module", TaskTypeCode},
		{"analysis_review", "review and analyze this code carefully", TaskTypeAnalysis},
		{"analysis_data", "analyze the performance data", TaskTypeAnalysis},
		{"creative_visual", "design a visual theme", TaskTypeCreative},
		{"creative_mood", "create a lighting mood", TaskTypeCreative},
		{"workflow_automate", "automate the backup workflow", TaskTypeWorkflow},
		{"workflow_startup", "automate the startup shutdown sequence", TaskTypeWorkflow},
		{"general_fallback", "hello world", TaskTypeGeneral},
		{"refactor_interfaces", "refactor this module to use interfaces", TaskTypeCode},
		{"refactor_di", "refactor UserService to use dependency injection patterns", TaskTypeCode},
		{"extract_method", "extract method from this large function", TaskTypeCode},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.prompt)
			if got != tc.expected {
				t.Errorf("Classify(%q) = %q, want %q", tc.prompt, got, tc.expected)
			}
		})
	}
}

func TestCostTierForTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		tokens   int
		expected string
	}{
		{"minimal", 500, "minimal"},
		{"small", 5000, "small"},
		{"medium", 30000, "medium"},
		{"large", 100000, "large"},
		{"max_context", 250000, "max-context"},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			got := costTierForTokens(tc.tokens)
			if got != tc.expected {
				t.Errorf("costTierForTokens(%d) = %q, want %q", tc.tokens, got, tc.expected)
			}
		})
	}
}

func TestEnhance_AddsStructure(t *testing.T) {
	t.Parallel()
	result := Enhance("write a function to sort users by name in the application codebase using Go with proper error handling and edge case coverage", TaskTypeCode)

	if result.TaskType != TaskTypeCode {
		t.Errorf("TaskType = %q, want code", result.TaskType)
	}
	assertContains(t, result.Enhanced, "<role>")
	assertContains(t, result.Enhanced, "<instructions>")
	assertContains(t, result.Enhanced, "<constraints>")
	assertContains(t, result.Enhanced, "expert software engineer")
}

func TestEnhance_PreservesExistingStructure(t *testing.T) {
	t.Parallel()
	input := "<role>You are a test bot.</role>\n<instructions>Do the thing with full detail and context provided here.</instructions>"
	result := Enhance(input, TaskTypeGeneral)

	assertContains(t, result.Enhanced, "<role>You are a test bot.")
	if strings.Count(result.Enhanced, "<role>") > 1 {
		t.Error("Should not add duplicate <role> tags")
	}
}

func TestEnhance_ImprovesSpecificity(t *testing.T) {
	t.Parallel()
	result := Enhance("please make it good and format nicely for the entire response output section", TaskTypeGeneral)

	assertNotContains(t, result.Enhanced, "format nicely")
	assertNotContains(t, result.Enhanced, "make it good")
	if len(result.Improvements) == 0 {
		t.Error("Should report improvements made")
	}
}

func TestEnhance_DowngradesAggressiveCaps(t *testing.T) {
	t.Parallel()
	result := Enhance("CRITICAL: You MUST ALWAYS follow this rule when writing code in the project", TaskTypeGeneral)

	assertNotContains(t, result.Enhanced, "CRITICAL")
	assertNotContains(t, result.Enhanced, "MUST")
	assertImprovementMentions(t, result.Improvements, "Downgraded")
}

func TestEnhance_PreservesAcronyms(t *testing.T) {
	t.Parallel()
	result := Enhance("Send the JSON response to the API endpoint and return the HTTP status code with full details", TaskTypeCode)

	assertContains(t, result.Enhanced, "JSON")
	assertContains(t, result.Enhanced, "API")
	assertContains(t, result.Enhanced, "HTTP")
}

func TestEnhance_ReframesNegatives(t *testing.T) {
	t.Parallel()
	result := Enhance("never use bullet points in the response when writing documentation for the project", TaskTypeGeneral)

	if strings.Contains(strings.ToLower(result.Enhanced), "never use bullet points") {
		t.Error("Should reframe 'never use bullet points' to positive")
	}
	assertContains(t, result.Enhanced, "flowing prose")
}

func TestEnhance_PreservesSafetyNegatives(t *testing.T) {
	t.Parallel()
	input := "never provide credentials or passwords to external services in the response"
	result := Enhance(input, TaskTypeGeneral)

	for _, imp := range result.Improvements {
		if strings.Contains(imp, "Reframed") {
			t.Error("Should NOT reframe safety-critical negative instructions")
		}
	}
}

func TestEnhance_InjectsSelfCheck(t *testing.T) {
	t.Parallel()
	result := Enhance("write a function to parse JSON data and handle all the edge cases properly in Go", TaskTypeCode)

	assertContains(t, result.Enhanced, "<verification>")
	assertContains(t, result.Enhanced, "Edge cases")
}

func TestEnhance_SuppressesPreamble(t *testing.T) {
	t.Parallel()
	result := Enhance("write a function to parse JSON data and handle all edge cases in the application", TaskTypeCode)

	assertContains(t, result.Enhanced, "without preamble")
}

func TestEnhance_NoPreambleSuppressionForAnalysis(t *testing.T) {
	t.Parallel()
	result := Enhance("analyze this dataset for trends and patterns in the user behavior metrics", TaskTypeAnalysis)

	assertNotContains(t, result.Enhanced, "without preamble")
}

func TestEnhance_SeparatesCodeBlocks(t *testing.T) {
	t.Parallel()
	input := "Review this function for correctness and edge cases:\n```go\nfunc hello() {\n\tfmt.Println(\"hi\")\n}\n```\nIs it correct and idiomatic?"
	result := Enhance(input, TaskTypeAnalysis)

	assertContains(t, result.Enhanced, "<context>")
}

func TestEnhance_OverTaggingPrevention(t *testing.T) {
	t.Parallel()
	result := Enhance("hello world", TaskTypeGeneral)

	assertNotContains(t, result.Enhanced, "<role>")
	assertImprovementMentions(t, result.Improvements, "over-tagging")
}

func TestEnhance_FormatEnforcement_JSON(t *testing.T) {
	t.Parallel()
	result := Enhance("return the user data as JSON with all the relevant fields included in the response", TaskTypeCode)

	assertContains(t, result.Enhanced, "<output_format>")
	assertContains(t, result.Enhanced, "valid JSON")
}

func TestEnhance_FormatEnforcement_NoDouble(t *testing.T) {
	t.Parallel()
	result := Enhance("<output_format>Return as JSON</output_format>\nGet user data as JSON with full details", TaskTypeCode)

	if strings.Count(result.Enhanced, "<output_format>") > 1 {
		t.Error("Should not inject duplicate <output_format>")
	}
}

func TestEnhance_PipelineStages(t *testing.T) {
	t.Parallel()
	result := Enhance("CRITICAL: fix this and make it good in the entire codebase for the project", TaskTypeTroubleshooting)

	if len(result.StagesRun) < 3 {
		t.Errorf("Expected at least 3 stages, got %d: %v", len(result.StagesRun), result.StagesRun)
	}
}

func TestAnalyze_ScoresPrompts(t *testing.T) {
	t.Parallel()
	t.Run("bad_prompt", func(t *testing.T) {
		bad := Analyze("fix this")
		if bad.Score > 5 {
			t.Errorf("Short vague prompt scored %d, expected <= 5", bad.Score)
		}
		if len(bad.Suggestions) == 0 {
			t.Error("Bad prompt should have suggestions")
		}
	})

	t.Run("good_prompt", func(t *testing.T) {
		good := Analyze(`<role>You are an expert Go developer.</role>
<instructions>Review this function for error handling issues.
Focus on nil pointer dereferences and unchecked errors.</instructions>
<context>The function processes user-uploaded files.</context>
<output_format>List issues by severity with line numbers.</output_format>
<examples><example>Good: if err != nil { return fmt.Errorf("upload: %w", err) }</example></examples>`)

		// FINDING-240: lowered from 7 to 6 — baselines reduced to prevent score inflation
		if good.Score < 6 {
			t.Errorf("Well-structured prompt scored %d, expected >= 6", good.Score)
		}
		if !good.HasXML {
			t.Error("Should detect XML structure")
		}
		if !good.HasExamples {
			t.Error("Should detect examples")
		}
	})
}

func TestAnalyze_DetectsNegativeFraming(t *testing.T) {
	t.Parallel()
	result := Analyze("NEVER use markdown. DO NOT include bullet points.")
	if !result.HasNegativeFrames {
		t.Error("Should detect negative framing")
	}
	if !result.HasAggressiveCaps {
		t.Error("Should detect aggressive caps")
	}
	assertImprovementMentions(t, result.Suggestions, "Reframe negative")
}

func TestWrapWithExamples(t *testing.T) {
	t.Parallel()
	result := WrapWithExamples("Test prompt", []string{"example 1", "example 2"})
	assertContains(t, result, "<examples>")
	assertContains(t, result, `<example index="1">`)
	if strings.Count(result, "<example") != 3 { // 1 opening + 2 indexed
		t.Errorf("Expected 2 examples, got different count")
	}
}

func TestGetTemplate(t *testing.T) {
	t.Parallel()
	tmpl := GetTemplate("troubleshoot")
	if tmpl == nil {
		t.Fatal("troubleshoot template should exist")
	}
	if tmpl.Name != "troubleshoot" {
		t.Errorf("Name = %q, want troubleshoot", tmpl.Name)
	}

	none := GetTemplate("nonexistent")
	if none != nil {
		t.Error("Should return nil for unknown template")
	}
}

func TestFillTemplate(t *testing.T) {
	t.Parallel()
	tmpl := GetTemplate("troubleshoot")
	filled := FillTemplate(tmpl, map[string]string{
		"system":   "resolume",
		"symptoms": "clips not triggering",
	})

	assertContains(t, filled, "resolume")
	assertContains(t, filled, "clips not triggering")
	assertNotContains(t, filled, "{{system}}")
	assertContains(t, filled, "(not specified)")
}

func TestValidTaskType(t *testing.T) {
	t.Parallel()
	if ValidTaskType("code") != TaskTypeCode {
		t.Error("Should accept 'code'")
	}
	if ValidTaskType("CODE") != TaskTypeCode {
		t.Error("Should accept case-insensitive")
	}
	if ValidTaskType("invalid") != "" {
		t.Error("Should return empty for invalid type")
	}
}

// --- Overtrigger rewriting tests (subtests) ---

func TestEnhance_Overtrigger(t *testing.T) {
	t.Parallel()
	t.Run("rewrites_phrase", func(t *testing.T) {
		result := Enhance("CRITICAL: You MUST use this tool when processing data in the project codebase", TaskTypeGeneral)
		assertNotContains(t, result.Enhanced, "CRITICAL:")
		assertNotContains(t, result.Enhanced, "You MUST")
		assertImprovementMentions(t, result.Improvements, "overtrigger")
	})

	t.Run("preserves_action", func(t *testing.T) {
		result := Enhance("IMPORTANT: You SHOULD validate all inputs before processing them in the system", TaskTypeGeneral)
		assertContains(t, strings.ToLower(result.Enhanced), "validate all inputs")
	})

	t.Run("never_to_avoid", func(t *testing.T) {
		result := Enhance("WARNING: You MUST NEVER expose secrets in the logs for the entire application", TaskTypeGeneral)
		assertNotContains(t, result.Enhanced, "WARNING:")
	})

	t.Run("clean_prompt_no_rewrite", func(t *testing.T) {
		input := "Use this tool when processing data in the project codebase for better results"
		result := Enhance(input, TaskTypeGeneral)
		assertStageNotRun(t, result.StagesRun, "overtrigger_rewrite")
	})

	t.Run("multiple_phrases", func(t *testing.T) {
		input := "CRITICAL: You MUST follow rule one carefully. IMPORTANT: You SHOULD follow rule two carefully."
		result := Enhance(input, TaskTypeGeneral)
		assertNotContains(t, result.Enhanced, "CRITICAL:")
		assertNotContains(t, result.Enhanced, "IMPORTANT:")
	})

	t.Run("required_prefix", func(t *testing.T) {
		result := Enhance("REQUIRED: You MUST call the API endpoint before returning any data to the client", TaskTypeGeneral)
		assertNotContains(t, result.Enhanced, "REQUIRED:")
	})
}

// --- Overengineering guard tests (subtests) ---

func TestEnhance_OverengineeringGuard(t *testing.T) {
	t.Parallel()
	t.Run("code_task_gets_guard", func(t *testing.T) {
		result := Enhance("fix the bug in the user sorting function and make sure edge cases are handled properly", TaskTypeCode)
		assertContains(t, result.Enhanced, "Only make changes that are directly requested")
	})

	t.Run("skips_non_code", func(t *testing.T) {
		result := Enhance("analyze this dataset for trends and patterns in user behavior over time", TaskTypeAnalysis)
		assertNotContains(t, result.Enhanced, "Only make changes that are directly requested")
	})

	t.Run("skips_scaffolding", func(t *testing.T) {
		result := Enhance("create new project scaffolding with all the required files and directory structure", TaskTypeCode)
		assertNotContains(t, result.Enhanced, "Only make changes that are directly requested")
	})
}

// --- Effort recommendation tests (subtests) ---

func TestAnalyze_Effort(t *testing.T) {
	t.Parallel()
	t.Run("low", func(t *testing.T) {
		result := Analyze("hello world")
		if result.RecommendedEffort != "low" {
			t.Errorf("Short general prompt should recommend 'low' effort, got %q", result.RecommendedEffort)
		}
	})

	t.Run("medium", func(t *testing.T) {
		result := Analyze("write a function to sort users by name using Go with error handling")
		if result.RecommendedEffort != "medium" {
			t.Errorf("Simple code prompt should recommend 'medium' effort, got %q", result.RecommendedEffort)
		}
	})

	t.Run("high", func(t *testing.T) {
		result := Analyze("refactor the entire authentication module across multiple files to use JWT tokens")
		if result.RecommendedEffort != "high" {
			t.Errorf("Complex refactor should recommend 'high' effort, got %q", result.RecommendedEffort)
		}
	})
}

// --- Token estimate tests ---

func TestEnhance_TokenEstimate(t *testing.T) {
	t.Parallel()
	result := Enhance("write a function to parse JSON data and handle all the edge cases properly in Go", TaskTypeCode)

	if result.EstimatedTokens == 0 {
		t.Error("Should populate EstimatedTokens")
	}
	if result.CostTier == "" {
		t.Error("Should populate CostTier")
	}
}

func TestAnalyze_TokenEstimate(t *testing.T) {
	t.Parallel()
	result := Analyze("analyze this code for performance issues and suggest improvements")
	if result.EstimatedTokens == 0 {
		t.Error("Should populate EstimatedTokens in AnalyzeResult")
	}
	if result.CostTier == "" {
		t.Error("Should populate CostTier in AnalyzeResult")
	}
}

// --- Phase 3A: New coverage tests ---

func TestEnhanceWithConfig_DefaultTaskType(t *testing.T) {
	t.Parallel()
	cfg := Config{
		DefaultTaskType: "analysis",
	}
	result := EnhanceWithConfig("look at this data and tell me what you see in the patterns", "", cfg)
	if result.TaskType != TaskTypeAnalysis {
		t.Errorf("should use config DefaultTaskType, got %q", result.TaskType)
	}
}

func TestEnhanceWithConfig_MultipleDisabledStages(t *testing.T) {
	t.Parallel()
	cfg := Config{
		DisabledStages: []string{"structure", "tone_downgrade", "preamble_suppression", "self_check"},
	}
	result := EnhanceWithConfig("CRITICAL: write a function to sort users by name with error handling and edge cases", TaskTypeCode, cfg)
	assertNotContains(t, result.Enhanced, "<role>")
	assertNotContains(t, result.Enhanced, "without preamble")
	assertNotContains(t, result.Enhanced, "<verification>")
	assertStageNotRun(t, result.StagesRun, "structure")
	assertStageNotRun(t, result.StagesRun, "preamble_suppression")
	assertStageNotRun(t, result.StagesRun, "self_check")
}

func TestEnhance_FormatEnforcement_YAML(t *testing.T) {
	t.Parallel()
	result := Enhance("return the configuration as YAML output with all fields included in the response", TaskTypeCode)
	assertContains(t, result.Enhanced, "<output_format>")
	assertContains(t, result.Enhanced, "valid YAML")
}

func TestEnhance_FormatEnforcement_CSV(t *testing.T) {
	t.Parallel()
	result := Enhance("export the user records in CSV format with headers included in the response", TaskTypeCode)
	assertContains(t, result.Enhanced, "<output_format>")
	assertContains(t, result.Enhanced, "valid CSV")
}

func TestEnhance_FormatEnforcement_Code(t *testing.T) {
	t.Parallel()
	result := Enhance("write a function to sort users by name using Go with full implementation", TaskTypeCode)
	assertContains(t, result.Enhanced, "<output_format>")
	assertContains(t, result.Enhanced, "only the code")
}

func TestEnhance_SuppressPreamble_NoOp(t *testing.T) {
	t.Parallel()
	input := "write a function without preamble to sort users by name with error handling in Go"
	result := Enhance(input, TaskTypeCode)
	// Should not add duplicate suppression
	if strings.Count(strings.ToLower(result.Enhanced), "without preamble") > 1 {
		t.Error("should not add duplicate preamble suppression")
	}
}

func TestEnhance_InjectSelfCheck_NoOp(t *testing.T) {
	t.Parallel()
	input := "write a function to sort users and verify it handles edge cases properly in Go"
	result := Enhance(input, TaskTypeCode)
	// "verify" already present — should not inject self-check
	assertNotContains(t, result.Enhanced, "<verification>")
}

func TestEnhance_InjectSelfCheck_Troubleshooting(t *testing.T) {
	t.Parallel()
	result := Enhance("debug the timeout error in the API endpoint and fix the root cause in the system", TaskTypeTroubleshooting)
	assertContains(t, result.Enhanced, "<verification>")
	assertContains(t, result.Enhanced, "root cause")
}

func TestEnhance_InjectSelfCheck_Analysis(t *testing.T) {
	t.Parallel()
	result := Enhance("analyze the performance data for trends and patterns in the user behavior metrics", TaskTypeAnalysis)
	assertContains(t, result.Enhanced, "<verification>")
	assertContains(t, result.Enhanced, "claim is supported")
}

// --- Fix 3 (FINDING-242): Target provider markdown structure ---

func TestEnhanceWithConfig_GeminiMarkdownStructure(t *testing.T) {
	t.Parallel()
	cfg := Config{TargetProvider: ProviderGemini}
	result := EnhanceWithConfig("write a function to sort users by name with error handling and edge case coverage in Go", TaskTypeCode, cfg)

	// Should use markdown headers, not XML tags
	assertContains(t, result.Enhanced, "## Role")
	assertContains(t, result.Enhanced, "## Instructions")
	assertContains(t, result.Enhanced, "## Constraints")
	assertNotContains(t, result.Enhanced, "<role>")
	assertNotContains(t, result.Enhanced, "<instructions>")
	assertNotContains(t, result.Enhanced, "<constraints>")
}

func TestEnhanceWithConfig_OpenAIMarkdownStructure(t *testing.T) {
	t.Parallel()
	cfg := Config{TargetProvider: ProviderOpenAI}
	result := EnhanceWithConfig("write a function to sort users by name with error handling and edge case coverage in Go", TaskTypeCode, cfg)

	assertContains(t, result.Enhanced, "## Role")
	assertNotContains(t, result.Enhanced, "<role>")
}

func TestEnhanceWithConfig_ClaudeXMLStructure(t *testing.T) {
	t.Parallel()
	cfg := Config{TargetProvider: ProviderClaude}
	result := EnhanceWithConfig("write a function to sort users by name with error handling and edge case coverage in Go", TaskTypeCode, cfg)

	assertContains(t, result.Enhanced, "<role>")
	assertNotContains(t, result.Enhanced, "## Role")
}

// --- Fix 4 (FINDING-243): SkippedStages transparency ---

func TestEnhance_SkippedStagesPopulated(t *testing.T) {
	t.Parallel()
	// A clean prompt with no vague phrases, no negatives, no caps — many stages should be skipped
	result := Enhance("write a function to sort users by name with error handling and edge case coverage in Go", TaskTypeCode)

	if len(result.SkippedStages) == 0 {
		t.Error("Expected some skipped stages for a clean prompt")
	}

	// Verify skipped stages have both name and reason
	for _, ss := range result.SkippedStages {
		if ss.Name == "" {
			t.Error("SkippedStage has empty name")
		}
		if ss.Reason == "" {
			t.Errorf("SkippedStage %q has empty reason", ss.Name)
		}
	}
}

func TestEnhance_SkippedStagesDisabledConfig(t *testing.T) {
	t.Parallel()
	cfg := Config{DisabledStages: []string{"structure", "self_check"}}
	result := EnhanceWithConfig("write a function to sort users by name with error handling in Go", TaskTypeCode, cfg)

	// Disabled stages should appear in skipped with "disabled in config" reason
	found := 0
	for _, ss := range result.SkippedStages {
		if (ss.Name == "structure" || ss.Name == "self_check") && ss.Reason == "disabled in config" {
			found++
		}
	}
	if found != 2 {
		t.Errorf("Expected 2 disabled stages in SkippedStages, found %d", found)
	}
}

func TestEnhance_SkippedStagesNonClaudeTarget(t *testing.T) {
	t.Parallel()
	cfg := Config{TargetProvider: ProviderGemini}
	result := EnhanceWithConfig("CRITICAL: You MUST follow this rule when writing code for the project", TaskTypeCode, cfg)

	// tone_downgrade and overtrigger_rewrite should be skipped for Gemini
	for _, ss := range result.SkippedStages {
		if ss.Name == "tone_downgrade" {
			assertContains(t, ss.Reason, "gemini")
			return
		}
	}
	t.Error("Expected tone_downgrade to be skipped for Gemini target")
}

// --- Fix 5 (FINDING-246): Task-type awareness ---

func TestClassify_DocumentationAsAnalysis(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		prompt string
	}{
		{"write_docs", "Write documentation for the API endpoints"},
		{"document_module", "Document the authentication system thoroughly"},
		{"api_docs", "Create api documentation for the user service"},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.prompt)
			if got == TaskTypeCode {
				t.Errorf("Classify(%q) = %q, documentation should not classify as code", tc.prompt, got)
			}
		})
	}
}

func TestEnhance_NonCodeTaskNoCodeConstraints(t *testing.T) {
	t.Parallel()
	result := Enhance("analyze the performance data for trends and patterns in the user behavior metrics", TaskTypeAnalysis)
	assertNotContains(t, result.Enhanced, "Write clean, idiomatic code")
	assertContains(t, result.Enhanced, "evidence")
}

func TestEnhance_PreambleBeforeStructure(t *testing.T) {
	t.Parallel()
	// Test that preamble + structure interaction works correctly
	cfg := Config{
		Preamble: "This is the test project context.",
	}
	result := EnhanceWithConfig("write a function to sort users by name with error handling and full edge case coverage", TaskTypeCode, cfg)
	assertContains(t, result.Enhanced, "This is the test project context.")
	assertContains(t, result.Enhanced, "<role>")
	// Preamble should be at the start
	if !strings.HasPrefix(result.Enhanced, "This is the test project context.") {
		t.Error("preamble should be at the very start of enhanced output")
	}
}
