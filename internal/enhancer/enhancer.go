// Package enhancer provides deterministic prompt enhancement using XML structuring,
// specificity improvements, context reordering, and task-type-aware formatting.
// No external LLM calls — pure Go string manipulation.
//
// Based on Anthropic's official prompt engineering best practices:
// - XML tags for structure (Claude is specifically trained to recognize them)
// - Context placement (long context before query for 20K+ token prompts)
// - Positive framing over negative (Claude 4.x responds better to "do X" than "don't Y")
// - Aggressive language downgrading (MUST/CRITICAL → normal case for Claude 4.x)
// - Few-shot example wrapping in <examples><example> tags
// - Self-check injection for code/math/analysis tasks
// - Preamble suppression for direct output
// - Format enforcement for JSON/YAML/code output requests
// - Over-tagging prevention for short single-purpose prompts
package enhancer

import (
	"fmt"
	"strings"
)

// EnhanceResult holds the output of the enhancement pipeline
type EnhanceResult struct {
	Original        string         `json:"original"`
	Enhanced        string         `json:"enhanced"`
	TaskType        TaskType       `json:"task_type"`
	StagesRun       []string       `json:"stages_run"`
	SkippedStages   []SkippedStage `json:"skipped_stages,omitempty"`
	Improvements    []string       `json:"improvements"`
	EstimatedTokens int            `json:"estimated_tokens"`
	CostTier        string         `json:"cost_tier"`
	Source          string         `json:"source,omitempty"` // "local", "llm", "llm_cached", "local_fallback", "error"
}

// SkippedStage records a pipeline stage that was skipped and why.
type SkippedStage struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// AnalyzeResult holds prompt quality analysis
type AnalyzeResult struct {
	Score             int          `json:"score"`
	ScoreReport       *ScoreReport `json:"score_report"`
	Suggestions       []string     `json:"suggestions"`
	HasXML            bool         `json:"has_xml_structure"`
	HasExamples       bool         `json:"has_examples"`
	HasContext        bool         `json:"has_context"`
	HasFormat         bool         `json:"has_output_format"`
	HasNegativeFrames bool         `json:"has_negative_framing"`
	HasAggressiveCaps bool         `json:"has_aggressive_caps"`
	WordCount         int          `json:"word_count"`
	TaskType          TaskType     `json:"task_type"`
	EstimatedTokens   int          `json:"estimated_tokens"`
	CostTier          string       `json:"cost_tier"`
	RecommendedEffort string       `json:"recommended_effort"`
}

// EnhanceWithConfig runs the full enhancement pipeline with optional project config.
func EnhanceWithConfig(raw string, taskType TaskType, cfg Config) EnhanceResult {
	if taskType == "" {
		if cfg.DefaultTaskType != "" {
			taskType = ValidTaskType(cfg.DefaultTaskType)
		}
		if taskType == "" {
			taskType = Classify(raw)
		}
	}

	result := EnhanceResult{
		Original: raw,
		TaskType: taskType,
	}

	text := raw
	var imps []string

	// Stage 0: Apply config rules (prepend/append based on pattern matches)
	if len(cfg.Rules) > 0 {
		text, imps = cfg.ApplyRules(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "config_rules")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "config_rules", Reason: "no config rules matched the prompt",
			})
		}
	}

	// Stage 1: Specificity — replace vague phrases with concrete instructions
	if cfg.IsStageDisabled("specificity") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "specificity", Reason: "disabled in config",
		})
	} else {
		text, imps = improveSpecificity(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "specificity")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "specificity", Reason: "no vague phrases detected",
			})
		}
	}

	// Stage 2: Positive reframing — rewrite known negative patterns first
	if cfg.IsStageDisabled("positive_reframe") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "positive_reframe", Reason: "disabled in config",
		})
	} else {
		text, imps = reframeNegatives(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "positive_reframe")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "positive_reframe", Reason: "no negative patterns detected",
			})
		}
	}

	// Stage 3: Tone — downgrade remaining aggressive ALL-CAPS for Claude 4.x
	// Skip for non-Claude targets — other models don't overtrigger on aggressive language
	if cfg.IsStageDisabled("tone_downgrade") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "tone_downgrade", Reason: "disabled in config",
		})
	} else if cfg.TargetProvider != "" && cfg.TargetProvider != ProviderClaude {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "tone_downgrade", Reason: "not applicable for target provider " + string(cfg.TargetProvider),
		})
	} else {
		text, imps = downgradeTone(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "tone_downgrade")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "tone_downgrade", Reason: "no aggressive caps detected",
			})
		}
	}

	// Stage 4: Overtrigger rewrite — soften aggressive anti-laziness phrases for Claude 4.x
	// Skip for non-Claude targets — aggressive prefixes may be useful for other models
	if cfg.IsStageDisabled("overtrigger_rewrite") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "overtrigger_rewrite", Reason: "disabled in config",
		})
	} else if cfg.TargetProvider != "" && cfg.TargetProvider != ProviderClaude {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "overtrigger_rewrite", Reason: "not applicable for target provider " + string(cfg.TargetProvider),
		})
	} else {
		text, imps = rewriteOvertriggerPhrases(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "overtrigger_rewrite")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "overtrigger_rewrite", Reason: "no overtrigger phrases detected",
			})
		}
	}

	// Stage 5: Example detection — wrap bare Input/Output pairs in <example> tags
	if cfg.IsStageDisabled("examples") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "example_wrapping", Reason: "disabled in config",
		})
	} else {
		text, imps = DetectAndWrapExamples(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "example_wrapping")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "example_wrapping", Reason: "no bare example pairs detected",
			})
		}
	}

	// Stage 6: Structure — wrap in XML tags (Claude) or markdown sections (Gemini/OpenAI)
	if cfg.IsStageDisabled("structure") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "structure", Reason: "disabled in config",
		})
	} else {
		if cfg.TargetProvider != "" && cfg.TargetProvider != ProviderClaude {
			text, imps = addMarkdownStructure(text, taskType)
		} else {
			text, imps = addStructure(text, taskType)
		}
		result.StagesRun = append(result.StagesRun, "structure")
		result.Improvements = append(result.Improvements, imps...)
	}

	// Stage 7: Long-context reordering — move bulk context before query
	if cfg.IsStageDisabled("context_reorder") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "context_reorder", Reason: "disabled in config",
		})
	} else {
		text, imps = ReorderLongContext(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "context_reorder")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "context_reorder", Reason: "prompt too short for long-context reordering",
			})
		}
	}

	// Stage 8: Format enforcement — detect output format requests
	if cfg.IsStageDisabled("format_enforcement") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "format_enforcement", Reason: "disabled in config",
		})
	} else {
		text, imps = enforceOutputFormat(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "format_enforcement")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "format_enforcement", Reason: "no format request detected or already specified",
			})
		}
	}

	// Stage 9: Quote grounding — inject "find quotes first" for long-context analysis
	if cfg.IsStageDisabled("quote_grounding") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "quote_grounding", Reason: "disabled in config",
		})
	} else {
		text, imps = InjectQuoteGrounding(text, taskType)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "quote_grounding")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			reason := "prompt too short for quote grounding"
			if taskType != TaskTypeAnalysis && taskType != TaskTypeGeneral {
				reason = "not applicable for task type " + string(taskType)
			}
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "quote_grounding", Reason: reason,
			})
		}
	}

	// Stage 10: Self-check — inject verification for code/math/analysis
	if cfg.IsStageDisabled("self_check") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "self_check", Reason: "disabled in config",
		})
	} else {
		text, imps = injectSelfCheck(text, taskType)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "self_check")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			reason := "not applicable for task type " + string(taskType)
			if taskType == TaskTypeCode || taskType == TaskTypeAnalysis || taskType == TaskTypeTroubleshooting {
				reason = "verification instruction already present"
			}
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "self_check", Reason: reason,
			})
		}
	}

	// Stage 11: Overengineering guard — prevent unnecessary abstractions (code tasks only)
	if cfg.IsStageDisabled("overengineering_guard") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "overengineering_guard", Reason: "disabled in config",
		})
	} else {
		text, imps = injectOverengineeringGuard(text, taskType)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "overengineering_guard")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			reason := "not applicable for task type " + string(taskType)
			if taskType == TaskTypeCode {
				reason = "scaffolding prompt or guard already present"
			}
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "overengineering_guard", Reason: reason,
			})
		}
	}

	// Stage 12: Preamble suppression — add direct response instruction
	if cfg.IsStageDisabled("preamble_suppression") {
		result.SkippedStages = append(result.SkippedStages, SkippedStage{
			Name: "preamble_suppression", Reason: "disabled in config",
		})
	} else {
		text, imps = suppressPreamble(text, taskType)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "preamble_suppression")
			result.Improvements = append(result.Improvements, imps...)
		} else {
			reason := "not applicable for task type " + string(taskType)
			if taskType == TaskTypeCode || taskType == TaskTypeWorkflow {
				reason = "preamble suppression already present"
			}
			result.SkippedStages = append(result.SkippedStages, SkippedStage{
				Name: "preamble_suppression", Reason: reason,
			})
		}
	}

	// Prepend config preamble if set
	if cfg.Preamble != "" {
		text = cfg.Preamble + "\n\n" + text
		result.Improvements = append(result.Improvements, "Prepended project-specific preamble from config")
	}

	result.Enhanced = text

	// Populate token estimate and cost tier
	result.EstimatedTokens = EstimateTokens(text)
	result.CostTier = costTierForTokens(result.EstimatedTokens)

	return result
}

// Enhance runs the full enhancement pipeline on a raw prompt (no config).
func Enhance(raw string, taskType TaskType) EnhanceResult {
	return EnhanceWithConfig(raw, taskType, Config{})
}

// Analyze scores a prompt and returns improvement suggestions
func Analyze(prompt string) AnalyzeResult {
	lower := strings.ToLower(prompt)
	words := strings.Fields(prompt)
	taskType := Classify(prompt)

	result := AnalyzeResult{
		WordCount: len(words),
		TaskType:  taskType,
	}

	// Check for existing structure
	result.HasXML = strings.Contains(prompt, "<") && strings.Contains(prompt, ">") &&
		(strings.Contains(lower, "<instructions") || strings.Contains(lower, "<context") ||
			strings.Contains(lower, "<role") || strings.Contains(lower, "<constraints"))
	result.HasExamples = strings.Contains(lower, "example") || strings.Contains(lower, "<example")
	result.HasContext = strings.Contains(lower, "context") || strings.Contains(lower, "<context")
	result.HasFormat = strings.Contains(lower, "format") || strings.Contains(lower, "<output")
	result.HasNegativeFrames = negativePattern.MatchString(prompt)
	result.HasAggressiveCaps = aggressiveCapsPattern.MatchString(prompt)

	// Count vague phrases for suggestions
	vagueCount := 0
	for pattern := range vagueReplacements {
		if strings.Contains(lower, pattern) {
			vagueCount++
		}
	}

	// Suggestions
	if !result.HasXML {
		result.Suggestions = append(result.Suggestions, "Add XML structure tags (<role>, <instructions>, <constraints>) — Claude is specifically trained to recognize XML as prompt structure")
	}
	if !result.HasExamples {
		result.Suggestions = append(result.Suggestions, "Include 3-5 examples in <examples><example> tags — Claude replicates formatting details from examples")
	}
	if !result.HasContext {
		result.Suggestions = append(result.Suggestions, "Add a <context> section with relevant background — place long context BEFORE the query for best results")
	}
	if !result.HasFormat {
		result.Suggestions = append(result.Suggestions, "Specify desired output format in an <output_format> section — use positive format instructions ('write in prose') not negative ('don't use bullets')")
	}
	if len(words) < 20 {
		result.Suggestions = append(result.Suggestions, "Add more detail — prompts under 20 words produce inconsistent results. Quantify constraints: '5 bullets, each under 15 words' instead of 'be concise'")
	}
	if vagueCount > 0 {
		result.Suggestions = append(result.Suggestions, fmt.Sprintf("Replace %d vague phrases with specific instructions (e.g., 'format nicely' → 'Format using markdown with headers and code blocks')", vagueCount))
	}
	if result.HasNegativeFrames {
		result.Suggestions = append(result.Suggestions, "Reframe negative instructions as positive — 'Write in flowing prose paragraphs' works better than 'NEVER use bullet points'. Claude 4.x can exhibit reverse psychology with heavy negative framing")
	}
	if result.HasAggressiveCaps {
		result.Suggestions = append(result.Suggestions, "Downgrade ALL-CAPS emphasis — Claude 4.x overtriggers on aggressive language like CRITICAL/MUST/IMPORTANT. Normal case is equally effective")
	}

	// Populate token estimate, cost tier, and recommended effort
	result.EstimatedTokens = EstimateTokens(prompt)
	result.CostTier = costTierForTokens(result.EstimatedTokens)
	result.RecommendedEffort = recommendEffort(prompt, taskType)

	// Multi-dimensional scoring
	allLints := Lint(prompt)
	allLints = append(allLints, VerifyCacheFriendlyOrder(prompt)...)
	report := Score(prompt, taskType, allLints, &result, "")
	result.ScoreReport = report

	// Derive legacy score from overall
	legacyScore := max(report.Overall/10, 1)
	if legacyScore > 10 {
		legacyScore = 10
	}
	result.Score = legacyScore

	// Task-specific suggestions
	switch taskType {
	case TaskTypeCode:
		if !strings.Contains(lower, "language") && !strings.Contains(lower, "go") &&
			!strings.Contains(lower, "python") && !strings.Contains(lower, "typescript") {
			result.Suggestions = append(result.Suggestions, "Specify the programming language")
		}
	case TaskTypeTroubleshooting:
		if !strings.Contains(lower, "error") && !strings.Contains(lower, "symptom") {
			result.Suggestions = append(result.Suggestions, "Include the exact error message or symptoms")
		}
	case TaskTypeAnalysis:
		if !strings.Contains(lower, "criteria") && !strings.Contains(lower, "focus") {
			result.Suggestions = append(result.Suggestions, "Specify evaluation criteria or focus areas")
		}
	}

	return result
}

// WrapWithExamples wraps a prompt and examples into proper XML few-shot format
func WrapWithExamples(prompt string, examples []string) string {
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n<examples>\n")
	for i, ex := range examples {
		fmt.Fprintf(&b, "<example index=\"%d\">\n%s\n</example>\n", i+1, strings.TrimSpace(ex))
	}
	b.WriteString("</examples>")
	return b.String()
}
