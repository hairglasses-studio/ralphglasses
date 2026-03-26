package enhancer

import (
	"fmt"
	"regexp"
	"strings"
)

// --- Stage 4a: Overtrigger phrase rewriting (Claude 4.x) ---

// overtriggerPattern detects aggressive anti-laziness prefixes that cause Claude 4.x to overtrigger.
// Per Anthropic: "CRITICAL: You MUST use this tool" should become "Use this tool when..."
var overtriggerPattern = regexp.MustCompile(
	`(?i)(CRITICAL|IMPORTANT|REQUIRED|WARNING)\s*[:!]\s*(You\s+)?(MUST|ALWAYS|NEVER|SHOULD)\s+`,
)

func rewriteOvertriggerPhrases(text string) (string, []string) {
	matches := overtriggerPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}

	var improvements []string
	count := 0
	result := overtriggerPattern.ReplaceAllStringFunc(text, func(match string) string {
		count++
		// Extract the action verb that follows the aggressive prefix
		// The pattern captures up to and including "MUST ", "ALWAYS ", etc.
		// We want to strip the prefix and keep just the verb context
		lower := strings.ToLower(match)
		// Find the modal verb position
		for _, modal := range []string{"must ", "always ", "never ", "should "} {
			if idx := strings.Index(lower, modal); idx != -1 {
				verb := strings.TrimSpace(match[idx:])
				// Convert "MUST do" to "do" or "NEVER do" to "avoid doing" / keep "never"
				verbLower := strings.ToLower(verb)
				if strings.HasPrefix(verbLower, "never ") {
					return "Avoid: " + strings.TrimSpace(verb[6:])
				}
				if strings.HasPrefix(verbLower, "must ") {
					return strings.TrimSpace(verb[5:])
				}
				if strings.HasPrefix(verbLower, "always ") {
					return strings.TrimSpace(verb[7:])
				}
				if strings.HasPrefix(verbLower, "should ") {
					return strings.TrimSpace(verb[7:])
				}
			}
		}
		return match
	})

	if count > 0 {
		improvements = append(improvements, fmt.Sprintf("Rewrote %d overtrigger phrase(s) — Claude 4.x overtriggers on aggressive 'CRITICAL: You MUST' prefixes", count))
	}
	return result, improvements
}

// --- Stage 11: Overengineering guard (Claude 4.x, code tasks only) ---

// overengineeringExemptPattern matches prompts that explicitly request new scaffolding
var overengineeringExemptPattern = regexp.MustCompile(`(?i)(create\s+new|scaffold|generate\s+boilerplate|set\s+up\s+a\s+new|initialize\s+a\s+new)`)

func injectOverengineeringGuard(text string, taskType TaskType) (string, []string) {
	if taskType != TaskTypeCode {
		return text, nil
	}

	lower := strings.ToLower(text)
	// Skip if prompt is asking for new creation/scaffolding
	if overengineeringExemptPattern.MatchString(text) {
		return text, nil
	}
	// Skip if guard already present
	if strings.Contains(lower, "only make changes that are directly requested") ||
		strings.Contains(lower, "overengineering") {
		return text, nil
	}

	guard := "\n\nOnly make changes that are directly requested or clearly necessary. Prefer editing existing files to creating new ones. Do not add abstractions, helpers, or defensive code for scenarios that cannot happen."
	return text + guard, []string{"Injected overengineering guard — Claude 4.x tends to over-abstract and create unnecessary files"}
}

// --- Token budget and cost tier ---

func costTierForTokens(tokens int) string {
	switch {
	case tokens < 1_000:
		return "minimal"
	case tokens < 10_000:
		return "small"
	case tokens < 50_000:
		return "medium"
	case tokens < 200_000:
		return "large"
	default:
		return "max-context"
	}
}

// --- Effort level recommendation ---

// complexityKeywords indicate prompts that benefit from high effort
var complexityKeywords = regexp.MustCompile(`(?i)\b(refactor|architecture|across\s+multiple\s+files|redesign|migrate|rewrite|comprehensive|exhaustive|all\s+edge\s+cases|full\s+audit)\b`)

func recommendEffort(prompt string, taskType TaskType) string {
	words := strings.Fields(prompt)
	wordCount := len(words)
	hasComplexity := complexityKeywords.MatchString(prompt)

	// High effort: complex tasks, long prompts, or explicit complexity keywords
	if hasComplexity || wordCount > 200 {
		return "high"
	}

	// Low effort: short general prompts
	if taskType == TaskTypeGeneral && wordCount < 20 {
		return "low"
	}

	// Medium: everything else
	return "medium"
}

// --- Stage 5: Format enforcement ---

// Format detection patterns (from Anthropic docs: positive format instructions work best)
var (
	jsonFormatPattern = regexp.MustCompile(`(?i)\b(json|JSON)\b.*(output|response|format|return|respond)|return\s+(?:as\s+)?json|(as|in)\s+json`)
	yamlFormatPattern = regexp.MustCompile(`(?i)\b(yaml|YAML)\b.*(output|response|format|return)|(as|in)\s+yaml`)
	codeFormatPattern = regexp.MustCompile(`(?i)^(write|create|implement|generate)\s+(a\s+)?(function|class|method|script|program|module)\b`)
	csvFormatPattern  = regexp.MustCompile(`(?i)\b(csv|CSV)\b.*(output|response|format|return)|(as|in)\s+csv`)
)

func enforceOutputFormat(text string) (string, []string) {
	lower := strings.ToLower(text)

	// Already has format specification
	if strings.Contains(lower, "<output_format>") || strings.Contains(lower, "output_format") {
		return text, nil
	}

	var formatBlock string
	var desc string

	switch {
	case jsonFormatPattern.MatchString(text):
		formatBlock = "\n\n<output_format>\nYour entire response must be valid JSON. Do not wrap it in markdown code fences. Do not include any text before or after the JSON. The response must parse with a standard JSON parser.\n</output_format>"
		desc = "Injected JSON format enforcement in <output_format> tags"
	case yamlFormatPattern.MatchString(text):
		formatBlock = "\n\n<output_format>\nYour entire response must be valid YAML. Do not wrap it in markdown code fences. Do not include any text before or after the YAML.\n</output_format>"
		desc = "Injected YAML format enforcement in <output_format> tags"
	case csvFormatPattern.MatchString(text):
		formatBlock = "\n\n<output_format>\nYour entire response must be valid CSV. Include a header row. Do not wrap in code fences or include any text before or after the CSV data.\n</output_format>"
		desc = "Injected CSV format enforcement in <output_format> tags"
	case codeFormatPattern.MatchString(text):
		formatBlock = "\n\n<output_format>\nReturn only the code. Do not include markdown code fences unless explicitly asked. Do not include explanatory text before or after the code unless asked.\n</output_format>"
		desc = "Injected code format enforcement in <output_format> tags"
	default:
		return text, nil
	}

	return text + formatBlock, []string{desc}
}

// --- Stage 10: Self-check injection ---

func injectSelfCheck(text string, taskType TaskType) (string, []string) {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "verify") || strings.Contains(lower, "double-check") ||
		strings.Contains(lower, "self-check") || strings.Contains(lower, "before you finish") {
		return text, nil
	}

	var check string
	switch taskType {
	case TaskTypeCode:
		check = "\n\n<verification>\nBefore finishing, verify:\n- The code compiles/runs without errors\n- Edge cases are handled (empty input, nil, zero values)\n- Error messages are descriptive\n</verification>"
	case TaskTypeAnalysis:
		check = "\n\n<verification>\nBefore finishing, verify:\n- Every claim is supported by specific evidence\n- You have distinguished facts from inferences\n- Alternative interpretations have been considered\n</verification>"
	case TaskTypeTroubleshooting:
		check = "\n\n<verification>\nBefore finishing, verify:\n- The proposed fix addresses the root cause, not a symptom\n- Rollback steps are included\n- No other systems are affected by the fix\n</verification>"
	default:
		return text, nil
	}

	return text + check, []string{"Injected self-verification checklist for " + string(taskType) + " task"}
}

// --- Stage 12: Preamble suppression ---

func suppressPreamble(text string, taskType TaskType) (string, []string) {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "without preamble") || strings.Contains(lower, "respond directly") ||
		strings.Contains(lower, "no preamble") {
		return text, nil
	}

	switch taskType {
	case TaskTypeCode, TaskTypeWorkflow:
		return text + "\n\nRespond directly without preamble. Do not start with phrases like 'Here is...', 'Sure,...', or 'Based on...'.",
			[]string{"Added preamble suppression for direct output"}
	default:
		return text, nil
	}
}
