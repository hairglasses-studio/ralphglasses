package enhancer

import (
	"fmt"
	"regexp"
	"strings"
)

// --- Stage 1: Specificity ---

var vagueReplacements = map[string]string{
	"format nicely":       "Format using markdown with headers and code blocks",
	"make it good":        "Ensure correctness, clarity, and completeness",
	"make it better":      "Improve clarity, reduce redundancy, and strengthen specificity",
	"clean it up":         "Refactor for readability: consistent naming, clear structure, remove dead code",
	"do your best":        "Provide a thorough, well-structured response",
	"be creative":         "Explore unconventional approaches while remaining practical",
	"be thorough":         "Cover all edge cases and provide step-by-step detail",
	"keep it simple":      "Use the minimum complexity needed — prefer standard patterns over abstractions",
	"make it fast":        "Optimize for performance: minimize allocations, reduce iterations, cache where appropriate",
	"make it secure":      "Follow security best practices: validate inputs, use parameterized queries, apply least privilege",
	"handle errors":       "Return descriptive errors with context, wrap errors at boundaries, never swallow errors silently",
	"add tests":           "Write unit tests covering happy path, edge cases, and error conditions",
	"fix this":            "Identify the root cause, apply a minimal fix, and verify it resolves the issue",
	"help me":             "Guide me step-by-step to",
	"i need":              "The goal is to",
	"can you":             "Please",
	"as soon as possible": "by [specific deadline]",
	"be concise":          "Limit each point to one sentence. Use 5 bullets maximum",
	"be brief":            "Respond in 3 sentences or fewer",
	"summarize":           "Extract the 3-5 most important points, each in one sentence",
}

func improveSpecificity(text string) (string, []string) {
	lower := strings.ToLower(text)
	var improvements []string

	for vague, specific := range vagueReplacements {
		if idx := strings.Index(lower, vague); idx != -1 {
			text = text[:idx] + specific + text[idx+len(vague):]
			lower = strings.ToLower(text)
			improvements = append(improvements, fmt.Sprintf("Replaced '%s' → '%s'", vague, specific))
		}
	}

	return text, improvements
}

// --- Stage 2: Negative-to-positive reframing ---

var negativePattern = regexp.MustCompile(`(?i)\b(NEVER|DO NOT|DON'T|MUST NOT|SHOULD NOT|CANNOT|CAN'T)\b`)

// safetyNegativePattern matches safety-critical negatives that should NOT be reframed
var safetyNegativePattern = regexp.MustCompile(`(?i)\b(never|do\s+not|must\s+not)\s+(provide|generate|create|produce|reveal|disclose|log|store|execute|run|delete|drop|rm\s+-rf)\s+.*(harm|weapon|illegal|credential|password|secret|PII|personal.?data|private.?key|token|database)`)

var negativeReframings = map[string]string{
	"never use bullet points": "Write in flowing prose paragraphs",
	"don't use markdown":      "Write in plain text with clear paragraph breaks",
	"do not use markdown":     "Write in plain text with clear paragraph breaks",
	"never use markdown":      "Write in plain text with clear paragraph breaks",
	"never use emojis":        "Write using text only, without decorative symbols",
	"don't make assumptions":  "Ask clarifying questions when information is missing",
	"do not make assumptions": "Ask clarifying questions when information is missing",
	"don't guess":             "State when you are uncertain and ask for clarification",
	"do not guess":            "State when you are uncertain and ask for clarification",
	"never hallucinate":       "Only include information you can verify from the provided context",
	"don't be verbose":        "Limit each point to one sentence",
	"do not be verbose":       "Limit each point to one sentence",
	"don't overthink":         "Choose an approach and commit to it",
	"do not overthink":        "Choose an approach and commit to it",
	"never skip steps":        "Show each step of your work",
	"don't repeat yourself":   "State each point once, clearly",
	"do not repeat yourself":  "State each point once, clearly",
}

func reframeNegatives(text string) (string, []string) {
	// Skip safety-critical negatives entirely
	if safetyNegativePattern.MatchString(text) {
		return text, nil
	}

	lower := strings.ToLower(text)
	var improvements []string

	for negative, positive := range negativeReframings {
		if idx := strings.Index(lower, negative); idx != -1 {
			text = text[:idx] + positive + text[idx+len(negative):]
			lower = strings.ToLower(text)
			improvements = append(improvements, fmt.Sprintf("Reframed '%s' → '%s' (positive framing)", negative, positive))
		}
	}

	return text, improvements
}

// --- Stage 3: Tone downgrade (Claude 4.x best practice) ---

// aggressiveCapsPattern detects ALL-CAPS emphasis words (not acronyms)
var aggressiveCapsPattern = regexp.MustCompile(`\b(CRITICAL|IMPORTANT|MUST|ALWAYS|NEVER|WARNING|REQUIRED|MANDATORY|ABSOLUTELY|ESSENTIAL)\b`)

// acronymWhitelist contains ALL-CAPS words that are acronyms, not emphasis
var acronymWhitelist = map[string]bool{
	"API": true, "URL": true, "HTTP": true, "HTTPS": true,
	"JSON": true, "XML": true, "HTML": true, "CSS": true,
	"SQL": true, "SSH": true, "TCP": true, "UDP": true,
	"DNS": true, "TLS": true, "SSL": true, "JWT": true,
	"UUID": true, "URI": true, "REST": true, "GRPC": true,
	"MCP": true, "SSE": true, "MIDI": true, "NDI": true,
	"OTEL": true, "PII": true, "YAML": true, "TOML": true,
	"CI": true, "CD": true, "PR": true, "UI": true,
	"OS": true, "IO": true, "ID": true, "OK": true,
	"BPM": true, "OSC": true, "DMX": true, "OBS": true,
	"AWS": true, "GCP": true, "CLI": true, "SDK": true,
	"CSV": true, "PDF": true, "PNG": true, "JPG": true,
	"EOF": true, "NULL": true, "TRUE": true, "FALSE": true,
}

var aggressiveCapsReplacements = map[string]string{
	"CRITICAL":   "critical",
	"IMPORTANT":  "important",
	"MUST":       "must",
	"ALWAYS":     "always",
	"NEVER":      "never",
	"WARNING":    "warning",
	"REQUIRED":   "required",
	"MANDATORY":  "required",
	"ABSOLUTELY": "",
	"ESSENTIAL":  "essential",
}

func downgradeTone(text string) (string, []string) {
	var improvements []string

	matches := aggressiveCapsPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return text, nil
	}

	downgraded := 0
	for _, match := range matches {
		// Skip acronyms
		if acronymWhitelist[match] {
			continue
		}
		replacement, ok := aggressiveCapsReplacements[match]
		if !ok {
			continue
		}
		if replacement == "" {
			text = strings.Replace(text, match+" ", "", 1)
			text = strings.Replace(text, " "+match, "", 1)
		} else {
			text = strings.Replace(text, match, replacement, 1)
		}
		downgraded++
	}

	if downgraded > 0 {
		improvements = append(improvements, fmt.Sprintf("Downgraded %d aggressive ALL-CAPS words to normal case (Claude 4.x best practice — overtriggers on aggressive language)", downgraded))
	}
	return text, improvements
}

// --- Stage 6: XML Structure (with over-tagging prevention) ---

func roleForTaskType(tt TaskType) string {
	switch tt {
	case TaskTypeCode:
		return "You are an expert software engineer."
	case TaskTypeCreative:
		return "You are a creative director with deep technical knowledge."
	case TaskTypeAnalysis:
		return "You are a thorough analytical reviewer."
	case TaskTypeTroubleshooting:
		return "You are a systems diagnostician focused on root cause analysis."
	case TaskTypeWorkflow:
		return "You are a workflow architect focused on reliability and simplicity."
	default:
		return "You are a knowledgeable assistant."
	}
}

func constraintsForTaskType(tt TaskType) string {
	switch tt {
	case TaskTypeCode:
		return `- Write clean, idiomatic code
- Handle errors explicitly — return descriptive errors with context
- Prefer simplicity over cleverness
- Only implement what was requested`
	case TaskTypeCreative:
		return `- Provide specific parameters and values, not vague descriptions
- Balance creative ambition with practical constraints
- Include concrete examples of the aesthetic you're describing`
	case TaskTypeAnalysis:
		return `- Support every claim with evidence from the provided data
- Distinguish facts from inferences, note confidence levels
- Use structured comparisons when evaluating alternatives`
	case TaskTypeTroubleshooting:
		return `- Start with the least disruptive diagnostic checks
- Identify root cause, not surface symptoms
- Propose fixes with rollback steps`
	case TaskTypeWorkflow:
		return `- Each step must have a clear success/failure condition
- Include error handling and rollback for each step
- Run independent steps in parallel where dependencies allow`
	default:
		return `- Be specific and actionable
- Structure output clearly with headers
- Respond directly without preamble`
	}
}

// shouldAddStructure implements over-tagging prevention.
// Anthropic docs: "Tags help most when prompts mix instructions, context, examples, and variable inputs."
// Simple, short, single-purpose prompts should NOT get XML tags — they add noise.
func shouldAddStructure(text string) bool {
	words := strings.Fields(text)
	// Too short — XML tags would be noise
	if len(words) < 15 {
		return false
	}
	// Already has XML tags
	lower := strings.ToLower(text)
	if strings.Contains(lower, "<instructions") || strings.Contains(lower, "<role") {
		return false
	}
	return true
}

func addStructure(text string, taskType TaskType) (string, []string) {
	if !shouldAddStructure(text) {
		return text, []string{"Prompt is short/simple — skipped XML wrapping (over-tagging prevention)"}
	}

	var b strings.Builder
	var improvements []string

	b.WriteString("<role>")
	b.WriteString(roleForTaskType(taskType))
	b.WriteString("</role>\n\n")
	improvements = append(improvements, "Added <role> tag with task-appropriate persona")

	// Detect if there's a code block or long context that should be separated
	if codeBlock, query := extractCodeBlock(text); codeBlock != "" {
		b.WriteString("<context>\n")
		b.WriteString(codeBlock)
		b.WriteString("\n</context>\n\n")
		b.WriteString("<instructions>\n")
		b.WriteString(strings.TrimSpace(query))
		b.WriteString("\n</instructions>\n\n")
		improvements = append(improvements, "Separated code/context block from instructions (long context before query — up to 30% quality improvement per Anthropic)")
	} else {
		b.WriteString("<instructions>\n")
		b.WriteString(strings.TrimSpace(text))
		b.WriteString("\n</instructions>\n\n")
	}
	improvements = append(improvements, "Wrapped prompt in XML structure tags")

	b.WriteString("<constraints>\n")
	b.WriteString(constraintsForTaskType(taskType))
	b.WriteString("\n</constraints>\n")
	improvements = append(improvements, "Added task-type-specific <constraints>")

	return b.String(), improvements
}

// addMarkdownStructure wraps the prompt in markdown sections instead of XML tags.
// Used for Gemini and OpenAI targets, which respond better to markdown structure.
func addMarkdownStructure(text string, taskType TaskType) (string, []string) {
	if !shouldAddStructure(text) {
		return text, []string{"Prompt is short/simple — skipped structural wrapping (over-tagging prevention)"}
	}

	var b strings.Builder
	var improvements []string

	b.WriteString("## Role\n")
	b.WriteString(roleForTaskType(taskType))
	b.WriteString("\n\n")
	improvements = append(improvements, "Added ## Role section with task-appropriate persona")

	if codeBlock, query := extractCodeBlock(text); codeBlock != "" {
		b.WriteString("## Context\n")
		b.WriteString(codeBlock)
		b.WriteString("\n\n")
		b.WriteString("## Instructions\n")
		b.WriteString(strings.TrimSpace(query))
		b.WriteString("\n\n")
		improvements = append(improvements, "Separated code/context block from instructions")
	} else {
		b.WriteString("## Instructions\n")
		b.WriteString(strings.TrimSpace(text))
		b.WriteString("\n\n")
	}
	improvements = append(improvements, "Wrapped prompt in structured markdown sections")

	b.WriteString("## Constraints\n")
	b.WriteString(constraintsForTaskType(taskType))
	b.WriteString("\n")
	improvements = append(improvements, "Added task-type-specific constraints section")

	return b.String(), improvements
}

// extractCodeBlock detects if the prompt contains a code block (``` delimited) and
// separates it from the surrounding text. Returns (code, rest) or ("", original).
func extractCodeBlock(text string) (string, string) {
	startIdx := strings.Index(text, "```")
	if startIdx == -1 {
		return "", text
	}
	endIdx := strings.Index(text[startIdx+3:], "```")
	if endIdx == -1 {
		return "", text
	}
	endIdx += startIdx + 3 + 3

	codeBlock := text[startIdx:endIdx]
	rest := strings.TrimSpace(text[:startIdx] + text[endIdx:])
	if len(rest) < 10 {
		return "", text
	}
	return codeBlock, rest
}
