package promptdj

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer/fewshot"
)

// MinedTemplate is a template pattern extracted from high-scoring registry prompts.
type MinedTemplate struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TaskType    string   `json:"task_type"`
	DomainTags  []string `json:"domain_tags"`
	Template    string   `json:"template"`    // with {{VARIABLE}} placeholders
	Variables   []string `json:"variables"`
	SourceCount int      `json:"source_count"` // how many prompts contributed
	AvgScore    float64  `json:"avg_score"`
	ExampleHash string   `json:"example_hash"` // best-scoring source prompt
}

// TemplateMinerConfig controls template extraction.
type TemplateMinerConfig struct {
	MinScore       int     // minimum quality score for source prompts (default 75)
	MinClusterSize int     // minimum prompts in a cluster to generate template (default 2)
	SimilarityThreshold float64 // minimum similarity to cluster together (default 0.4)
}

// DefaultTemplateMinerConfig returns sensible defaults.
func DefaultTemplateMinerConfig() TemplateMinerConfig {
	return TemplateMinerConfig{
		MinScore:            75,
		MinClusterSize:      2,
		SimilarityThreshold: 0.4,
	}
}

// MineTemplates extracts template patterns from high-scoring prompts in the registry.
// Groups similar prompts by task type + keyword overlap, then extracts the common
// structure as a template with variable placeholders.
func MineTemplates(entries []fewshot.PromptEntry, cfg TemplateMinerConfig) []MinedTemplate {
	// Filter to high-quality scored prompts
	var qualified []fewshot.PromptEntry
	for _, e := range entries {
		if e.Score >= cfg.MinScore && (e.Status == "scored" || e.Status == "improved") {
			qualified = append(qualified, e)
		}
	}

	if len(qualified) == 0 {
		return nil
	}

	// Group by task type
	byType := make(map[string][]fewshot.PromptEntry)
	for _, e := range qualified {
		byType[e.TaskType] = append(byType[e.TaskType], e)
	}

	var templates []MinedTemplate

	for taskType, group := range byType {
		if len(group) < cfg.MinClusterSize {
			continue
		}

		// Find common structural patterns
		tmpl := extractTemplate(group, taskType)
		if tmpl != nil {
			templates = append(templates, *tmpl)
		}
	}

	// Sort by average score descending
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].AvgScore > templates[j].AvgScore
	})

	return templates
}

// extractTemplate finds the common structure in a group of similar prompts
// and generates a template with variable placeholders.
func extractTemplate(group []fewshot.PromptEntry, taskType string) *MinedTemplate {
	if len(group) == 0 {
		return nil
	}

	// Find the best-scoring prompt as the template base
	best := group[0]
	for _, e := range group[1:] {
		if e.Score > best.Score {
			best = e
		}
	}

	// Extract structural elements
	template := best.Prompt

	// Replace specific identifiers with variables
	template = generalize(template)

	// Detect variables
	variables := extractVariables(template)

	// Compute average score
	var totalScore int
	var allTags []string
	tagSet := make(map[string]bool)
	for _, e := range group {
		totalScore += e.Score
		for _, t := range e.Tags {
			if !tagSet[t] {
				tagSet[t] = true
				allTags = append(allTags, t)
			}
		}
	}
	avgScore := float64(totalScore) / float64(len(group))

	name := fmt.Sprintf("%s_pattern_%s", taskType, best.ShortHash[:6])

	return &MinedTemplate{
		Name:        name,
		Description: fmt.Sprintf("Auto-mined %s template from %d high-scoring prompts (avg %.0f/100)", taskType, len(group), avgScore),
		TaskType:    taskType,
		DomainTags:  allTags,
		Template:    template,
		Variables:   variables,
		SourceCount: len(group),
		AvgScore:    avgScore,
		ExampleHash: best.Hash,
	}
}

// generalize replaces specific values in a prompt with {{VARIABLE}} placeholders.
var specificPatterns = []struct {
	pattern     *regexp.Regexp
	replacement string
	variable    string
}{
	{regexp.MustCompile(`(?i)\b(golang|python|rust|typescript|javascript|java|ruby)\b`), "{{LANGUAGE}}", "LANGUAGE"},
	{regexp.MustCompile(`(?i)\b(REST|GraphQL|gRPC|WebSocket)\s+API\b`), "{{API_TYPE}} API", "API_TYPE"},
	{regexp.MustCompile(`(?i)\b(PostgreSQL|MySQL|SQLite|MongoDB|Redis)\b`), "{{DATABASE}}", "DATABASE"},
	{regexp.MustCompile(`(?i)\b(Docker|Kubernetes|Terraform|Ansible)\b`), "{{TOOL}}", "TOOL"},
	{regexp.MustCompile(`(?i)~/[\w\-/]+`), "{{PATH}}", "PATH"},
}

func generalize(prompt string) string {
	result := prompt
	for _, sp := range specificPatterns {
		result = sp.pattern.ReplaceAllString(result, sp.replacement)
	}
	return result
}

func extractVariables(template string) []string {
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	matches := re.FindAllStringSubmatch(template, -1)
	seen := make(map[string]bool)
	var vars []string
	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			vars = append(vars, m[1])
		}
	}
	return vars
}

// suppress unused import for enhancer
var _ = enhancer.Classify
var _ = strings.Contains
