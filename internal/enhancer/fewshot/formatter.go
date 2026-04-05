package fewshot

import (
	"fmt"
	"strings"
)

// FormatXML formats retrieved examples as an XML block for Claude injection.
func FormatXML(examples []RetrievedExample, queryTaskType string) string {
	if len(examples) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<few_shot_examples source=\"prompt_registry\" count=\"%d\" query_task_type=\"%s\">\n",
		len(examples), queryTaskType)

	for i, ex := range examples {
		fmt.Fprintf(&b, "<example index=\"%d\" hash=\"%s\" score=\"%d\" grade=\"%s\" task_type=\"%s\" similarity=\"%.2f\">\n",
			i+1, ex.ShortHash, ex.Score, ex.Grade, ex.TaskType, ex.Similarity)
		b.WriteString("<example_prompt>\n")
		b.WriteString(strings.TrimSpace(ex.Prompt))
		b.WriteString("\n</example_prompt>\n")
		b.WriteString("</example>\n")
	}

	b.WriteString("</few_shot_examples>")
	return b.String()
}

// FormatMarkdown formats examples for Gemini/OpenAI targets using markdown.
func FormatMarkdown(examples []RetrievedExample, queryTaskType string) string {
	if len(examples) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Similar High-Quality Prompts (task: %s)\n\n", queryTaskType)

	for i, ex := range examples {
		fmt.Fprintf(&b, "### Example %d (score: %d/%s, similarity: %.0f%%)\n\n",
			i+1, ex.Score, ex.Grade, ex.Similarity*100)
		b.WriteString("```\n")
		b.WriteString(strings.TrimSpace(ex.Prompt))
		b.WriteString("\n```\n\n")
	}

	return b.String()
}
