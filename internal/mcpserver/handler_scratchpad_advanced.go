package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- scratchpad_validate (OPPORTUNITY-17) ---

// violation represents a single validation check failure.
type violation struct {
	Check    string `json:"check"`
	Message  string `json:"message"`
	Severity string `json:"severity"` // "error", "warning", "info"
}

// validateResult is returned by handleScratchpadValidate.
type validateResult struct {
	Valid      bool        `json:"valid"`
	Violations []violation `json:"violations"`
	ChecksRun []string    `json:"checks_run"`
}

func (s *Server) handleScratchpadValidate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "name is required"), nil
	}
	check := getStringArg(req, "check")
	if check == "" {
		return codedError(ErrInvalidParams, "check is required (scores, paths, budget, noops, all)"), nil
	}

	repoPath, errRes := s.resolveRepoPath(getStringArg(req, "repo"))
	if errRes != nil {
		return errRes, nil
	}

	path := filepath.Join(repoPath, ".ralph", name+"_scratchpad.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return codedError(ErrFilesystem, fmt.Sprintf("scratchpad %q not found", name)), nil
		}
		return codedError(ErrFilesystem, fmt.Sprintf("read scratchpad: %v", err)), nil
	}

	content := string(data)
	checks := resolveChecks(check)
	var violations []violation

	for _, c := range checks {
		switch c {
		case "scores":
			violations = append(violations, validateScores(content)...)
		case "paths":
			violations = append(violations, validatePaths(content, repoPath)...)
		case "budget":
			violations = append(violations, validateBudget(content)...)
		case "noops":
			violations = append(violations, validateNoops(content)...)
		}
	}

	result := validateResult{
		Valid:      len(violations) == 0,
		Violations: violations,
		ChecksRun: checks,
	}

	return jsonResult(result), nil
}

// resolveChecks expands "all" into the individual check types.
func resolveChecks(check string) []string {
	if check == "all" {
		return []string{"scores", "paths", "budget", "noops"}
	}
	return splitCSV(check)
}

// scorePattern matches lines like "overall: 85" or "Overall Score: 92".
var scoreOverallPattern = regexp.MustCompile(`(?i)overall[\s_]*(?:score)?[\s:=]*(\d+)`)

// dimensionScorePattern matches lines like "clarity: 60" or "specificity: 45".
var dimensionScorePattern = regexp.MustCompile(`(?i)(clarity|specificity|structure|examples|tone|context|completeness|actionability|safety|efficiency)[\s:=]*(\d+)`)

func validateScores(content string) []violation {
	var vv []violation

	overallMatches := scoreOverallPattern.FindAllStringSubmatch(content, -1)
	dimensionMatches := dimensionScorePattern.FindAllStringSubmatch(content, -1)

	for _, om := range overallMatches {
		overall, err := strconv.Atoi(om[1])
		if err != nil {
			continue
		}

		// Collect dimension scores from the same content block.
		if len(dimensionMatches) == 0 {
			continue
		}

		var sum, count int
		for _, dm := range dimensionMatches {
			score, err := strconv.Atoi(dm[2])
			if err != nil {
				continue
			}
			sum += score
			count++
		}

		if count == 0 {
			continue
		}

		avg := float64(sum) / float64(count)
		diff := math.Abs(float64(overall) - avg)

		// Flag if overall exceeds dimension average by more than 15 points.
		if diff > 15 {
			vv = append(vv, violation{
				Check:    "scores",
				Message:  fmt.Sprintf("score inflation: overall=%d but dimension average=%.0f (diff=%.0f)", overall, avg, diff),
				Severity: "warning",
			})
		}
	}

	return vv
}

// pathPattern matches snapshot or file paths like /path/to/repo or path references.
var pathRefPattern = regexp.MustCompile(`(?:snapshot|path|repo)[\s:=]*["']?(/[^\s"']+)`)

func validatePaths(content string, expectedRepoPath string) []violation {
	var vv []violation

	matches := pathRefPattern.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		refPath := m[1]
		// Normalize both for comparison.
		cleanRef := filepath.Clean(refPath)
		cleanExpected := filepath.Clean(expectedRepoPath)

		if !strings.HasPrefix(cleanRef, cleanExpected) {
			vv = append(vv, violation{
				Check:    "paths",
				Message:  fmt.Sprintf("path %q does not match expected repo root %q", refPath, expectedRepoPath),
				Severity: "error",
			})
		}
	}

	return vv
}

// budgetPattern matches lines like "requested_budget: $10" or "budget: 5.00" and "applied_budget: $3".
var budgetRequestedPattern = regexp.MustCompile(`(?i)requested[\s_]*budget[\s:=$]*(\d+(?:\.\d+)?)`)
var budgetAppliedPattern = regexp.MustCompile(`(?i)applied[\s_]*budget[\s:=$]*(\d+(?:\.\d+)?)`)

func validateBudget(content string) []violation {
	var vv []violation

	requestedMatches := budgetRequestedPattern.FindAllStringSubmatch(content, -1)
	appliedMatches := budgetAppliedPattern.FindAllStringSubmatch(content, -1)

	if len(requestedMatches) > 0 && len(appliedMatches) > 0 {
		requested, err1 := strconv.ParseFloat(requestedMatches[0][1], 64)
		applied, err2 := strconv.ParseFloat(appliedMatches[0][1], 64)

		if err1 == nil && err2 == nil && math.Abs(requested-applied) > 0.01 {
			vv = append(vv, violation{
				Check:    "budget",
				Message:  fmt.Sprintf("budget mismatch: requested=%.2f but applied=%.2f", requested, applied),
				Severity: "warning",
			})
		}
	}

	return vv
}

// noopPattern matches lines indicating a no-op iteration.
var noopFilesPattern = regexp.MustCompile(`(?i)files[\s_]*changed[\s:=]*0`)
var noopVerifyPattern = regexp.MustCompile(`(?i)verify[\s:=]*pass`)

func validateNoops(content string) []violation {
	var vv []violation

	lines := strings.Split(content, "\n")
	noopCount := 0

	for _, line := range lines {
		if noopFilesPattern.MatchString(line) && noopVerifyPattern.MatchString(line) {
			noopCount++
		}
	}

	if noopCount > 0 {
		sev := "info"
		if noopCount >= 3 {
			sev = "warning"
		}
		vv = append(vv, violation{
			Check:    "noops",
			Message:  fmt.Sprintf("%d no-op iteration(s) detected (files_changed=0 with verify=pass)", noopCount),
			Severity: sev,
		})
	}

	return vv
}

// --- scratchpad_context (OPPORTUNITY-18) ---

// contextAppendResult is returned by handleScratchpadContext.
type contextAppendResult struct {
	Appended   []string       `json:"appended_sections"`
	FieldCount map[string]int `json:"field_counts"`
}

func (s *Server) handleScratchpadContext(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "name is required"), nil
	}
	sectionsStr := getStringArg(req, "sections")
	if sectionsStr == "" {
		return codedError(ErrInvalidParams, "sections is required (fleet, observations, routing, autonomy, all)"), nil
	}

	repoPath, errRes := s.resolveRepoPath(getStringArg(req, "repo"))
	if errRes != nil {
		return errRes, nil
	}

	sections := resolveSections(sectionsStr)

	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("create .ralph dir: %v", err)), nil
	}

	path := filepath.Join(dir, name+"_scratchpad.md")

	var buf strings.Builder
	var appended []string
	fieldCounts := make(map[string]int)

	ts := time.Now().UTC().Format(time.RFC3339)
	buf.WriteString(fmt.Sprintf("\n## System Context (%s)\n\n", ts))

	for _, sec := range sections {
		switch sec {
		case "fleet":
			fields := s.gatherFleetContext()
			if len(fields) > 0 {
				buf.WriteString("### Fleet Status\n\n")
				for k, v := range fields {
					buf.WriteString(fmt.Sprintf("- **%s**: %s\n", k, v))
				}
				buf.WriteString("\n")
				appended = append(appended, "fleet")
				fieldCounts["fleet"] = len(fields)
			}

		case "observations":
			fields := s.gatherObservationContext(repoPath)
			if len(fields) > 0 {
				buf.WriteString("### Observations Summary\n\n")
				for k, v := range fields {
					buf.WriteString(fmt.Sprintf("- **%s**: %s\n", k, v))
				}
				buf.WriteString("\n")
				appended = append(appended, "observations")
				fieldCounts["observations"] = len(fields)
			}

		case "routing":
			fields := s.gatherRoutingContext()
			if len(fields) > 0 {
				buf.WriteString("### Routing Metadata\n\n")
				for k, v := range fields {
					buf.WriteString(fmt.Sprintf("- **%s**: %s\n", k, v))
				}
				buf.WriteString("\n")
				appended = append(appended, "routing")
				fieldCounts["routing"] = len(fields)
			}

		case "autonomy":
			fields := s.gatherAutonomyContext()
			if len(fields) > 0 {
				buf.WriteString("### Autonomy Level\n\n")
				for k, v := range fields {
					buf.WriteString(fmt.Sprintf("- **%s**: %s\n", k, v))
				}
				buf.WriteString("\n")
				appended = append(appended, "autonomy")
				fieldCounts["autonomy"] = len(fields)
			}
		}
	}

	// Write to scratchpad file.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("open scratchpad: %v", err)), nil
	}
	defer f.Close()

	if _, err := f.WriteString(buf.String()); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("write scratchpad: %v", err)), nil
	}

	result := contextAppendResult{
		Appended:   appended,
		FieldCount: fieldCounts,
	}

	return jsonResult(result), nil
}

func resolveSections(sectionsStr string) []string {
	parts := splitCSV(sectionsStr)
	for _, p := range parts {
		if p == "all" {
			return []string{"fleet", "observations", "routing", "autonomy"}
		}
	}
	return parts
}

// gatherFleetContext collects fleet status information.
func (s *Server) gatherFleetContext() map[string]string {
	fields := make(map[string]string)

	repos := s.reposCopy()
	fields["repos_discovered"] = strconv.Itoa(len(repos))

	activeSessions := 0
	if s.SessMgr != nil {
		for _, sess := range s.SessMgr.List("") {
			if sess.Status == "running" {
				activeSessions++
			}
		}
	}
	fields["active_sessions"] = strconv.Itoa(activeSessions)

	if s.FleetCoordinator != nil {
		fields["fleet_mode"] = "active"
	} else {
		fields["fleet_mode"] = "inactive"
	}

	return fields
}

// gatherObservationContext collects recent observation summary info.
func (s *Server) gatherObservationContext(repoPath string) map[string]string {
	fields := make(map[string]string)

	obsPath := filepath.Join(repoPath, ".ralph", "logs", "loop_observations.jsonl")
	info, err := os.Stat(obsPath)
	if err != nil {
		fields["status"] = "no observations file"
		return fields
	}

	fields["last_modified"] = info.ModTime().UTC().Format(time.RFC3339)
	fields["size_bytes"] = strconv.FormatInt(info.Size(), 10)

	// Count lines as a rough observation count.
	data, err := os.ReadFile(obsPath)
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		count := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				count++
			}
		}
		fields["observation_count"] = strconv.Itoa(count)
	}

	return fields
}

// gatherRoutingContext collects provider routing metadata.
func (s *Server) gatherRoutingContext() map[string]string {
	fields := make(map[string]string)

	if s.Bandit != nil {
		fields["bandit_selector"] = "active"
	} else {
		fields["bandit_selector"] = "inactive"
	}

	if s.Engine != nil {
		fields["enhancer_engine"] = "active"
	} else {
		fields["enhancer_engine"] = "inactive"
	}

	if s.CostPredictor != nil {
		fields["cost_predictor"] = "active"
	} else {
		fields["cost_predictor"] = "inactive"
	}

	return fields
}

// gatherAutonomyContext collects autonomy level information.
func (s *Server) gatherAutonomyContext() map[string]string {
	fields := make(map[string]string)

	if s.DecisionLog != nil {
		fields["decision_log"] = "active"
	} else {
		fields["decision_log"] = "inactive"
	}

	if s.HITLTracker != nil {
		fields["hitl_tracker"] = "active"
	} else {
		fields["hitl_tracker"] = "inactive"
	}

	if s.AutoOptimizer != nil {
		fields["auto_optimizer"] = "active"
	} else {
		fields["auto_optimizer"] = "inactive"
	}

	return fields
}

// --- scratchpad_reason (OPPORTUNITY-19) ---

// reasonResult is returned by handleScratchpadReason.
type reasonResult struct {
	Reasoning      map[string]any `json:"reasoning"`
	Recommendation string         `json:"recommendation"`
	Confidence     string         `json:"confidence"` // "low", "medium", "high"
}

func (s *Server) handleScratchpadReason(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "name is required"), nil
	}
	topic := getStringArg(req, "topic")
	if topic == "" {
		return codedError(ErrInvalidParams, "topic is required (enhance_stages, rate_cards, prune_thresholds, provider_selection)"), nil
	}
	inputStr := getStringArg(req, "input")

	repoPath, errRes := s.resolveRepoPath(getStringArg(req, "repo"))
	if errRes != nil {
		return errRes, nil
	}

	// Parse optional input JSON.
	var inputData map[string]any
	if inputStr != "" {
		if err := json.Unmarshal([]byte(inputStr), &inputData); err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid input JSON: %v", err)), nil
		}
	}

	var result reasonResult

	switch topic {
	case "enhance_stages":
		result = reasonEnhanceStages(inputData)
	case "rate_cards":
		result = reasonRateCards(inputData)
	case "prune_thresholds":
		result = reasonPruneThresholds(inputData, repoPath)
	case "provider_selection":
		result = reasonProviderSelection(inputData, s)
	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unknown topic: %s (valid: enhance_stages, rate_cards, prune_thresholds, provider_selection)", topic)), nil
	}

	// Append reasoning to scratchpad.
	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("create .ralph dir: %v", err)), nil
	}

	path := filepath.Join(dir, name+"_scratchpad.md")
	f, fErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if fErr != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("open scratchpad: %v", fErr)), nil
	}
	defer f.Close()

	ts := time.Now().UTC().Format(time.RFC3339)
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("\n## Reasoning: %s (%s)\n\n", topic, ts))
	buf.WriteString(fmt.Sprintf("**Recommendation**: %s\n", result.Recommendation))
	buf.WriteString(fmt.Sprintf("**Confidence**: %s\n\n", result.Confidence))

	reasoningJSON, _ := json.MarshalIndent(result.Reasoning, "", "  ")
	buf.WriteString("```json\n")
	buf.WriteString(string(reasoningJSON))
	buf.WriteString("\n```\n")

	if _, err := f.WriteString(buf.String()); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("write scratchpad: %v", err)), nil
	}

	return jsonResult(result), nil
}

// reasonEnhanceStages maps prompt dimensions to enhancer pipeline stages.
func reasonEnhanceStages(input map[string]any) reasonResult {
	// The 13-stage pipeline and which dimensions each stage affects.
	stageMap := map[string][]string{
		"specificity":       {"clarity", "specificity", "actionability"},
		"positive_reframe":  {"tone", "clarity"},
		"tone_downgrade":    {"tone"},
		"xml_structure":     {"structure", "clarity"},
		"markdown_structure": {"structure", "clarity"},
		"context_reorder":   {"context", "efficiency"},
		"format_enforce":    {"structure", "examples"},
		"self_check":        {"completeness", "safety"},
		"quantifier_fix":    {"specificity", "clarity"},
		"caps_normalize":    {"tone"},
		"injection_guard":   {"safety"},
		"cache_reorder":     {"efficiency"},
		"overtrigger":       {"tone"},
	}

	reasoning := map[string]any{
		"stage_to_dimensions": stageMap,
		"stage_count":         len(stageMap),
	}

	// If input has a target_provider, note which stages are provider-specific.
	targetProvider, _ := input["target_provider"].(string)
	if targetProvider != "" {
		providerSpecific := map[string]string{
			"tone_downgrade": "claude-only",
			"overtrigger":    "claude-only",
			"xml_structure":  "claude",
			"markdown_structure": "gemini, openai",
		}
		reasoning["provider_specific_stages"] = providerSpecific
		reasoning["target_provider"] = targetProvider
	}

	// If input has low-scoring dimensions, recommend stages.
	if dims, ok := input["low_dimensions"].([]any); ok && len(dims) > 0 {
		recommended := make(map[string]bool)
		for _, d := range dims {
			dimStr, _ := d.(string)
			for stage, affects := range stageMap {
				for _, a := range affects {
					if a == dimStr {
						recommended[stage] = true
					}
				}
			}
		}
		stages := make([]string, 0, len(recommended))
		for st := range recommended {
			stages = append(stages, st)
		}
		reasoning["recommended_stages"] = stages
	}

	return reasonResult{
		Reasoning:      reasoning,
		Recommendation: "Run the full pipeline; provider-specific stages auto-activate based on target_provider",
		Confidence:     "high",
	}
}

// reasonRateCards provides cost estimation reasoning.
func reasonRateCards(input map[string]any) reasonResult {
	// Standard rate cards per 1M tokens (as of knowledge cutoff).
	rateCards := map[string]map[string]float64{
		"claude": {
			"sonnet_input":  3.00,
			"sonnet_output": 15.00,
			"opus_input":    15.00,
			"opus_output":   75.00,
			"haiku_input":   0.25,
			"haiku_output":  1.25,
		},
		"gemini": {
			"pro_input":   1.25,
			"pro_output":  5.00,
			"flash_input": 0.075,
			"flash_output": 0.30,
		},
		"openai": {
			"gpt4o_input":     2.50,
			"gpt4o_output":    10.00,
			"gpt4o_mini_input": 0.15,
			"gpt4o_mini_output": 0.60,
		},
	}

	reasoning := map[string]any{
		"rate_cards_per_1m_tokens": rateCards,
	}

	// If input has token estimates, calculate projected cost.
	provider, _ := input["provider"].(string)
	model, _ := input["model"].(string)
	inputTokens, _ := input["input_tokens"].(float64)
	outputTokens, _ := input["output_tokens"].(float64)

	recommendation := "Use rate cards with actual token counts for accurate cost projection"
	confidence := "medium"

	if provider != "" && inputTokens > 0 {
		key := provider
		if rates, ok := rateCards[key]; ok {
			// Find best matching rate.
			var inputRate, outputRate float64
			for k, v := range rates {
				if strings.Contains(k, "input") && (strings.Contains(k, model) || model == "") {
					inputRate = v
				}
				if strings.Contains(k, "output") && (strings.Contains(k, model) || model == "") {
					outputRate = v
				}
			}
			if inputRate > 0 {
				cost := (inputTokens/1_000_000)*inputRate + (outputTokens/1_000_000)*outputRate
				reasoning["projected_cost_usd"] = fmt.Sprintf("%.4f", cost)
				reasoning["input_rate"] = inputRate
				reasoning["output_rate"] = outputRate
				recommendation = fmt.Sprintf("Projected cost: $%.4f for %.0f input + %.0f output tokens on %s", cost, inputTokens, outputTokens, provider)
				confidence = "high"
			}
		}
	}

	return reasonResult{
		Reasoning:      reasoning,
		Recommendation: recommendation,
		Confidence:     confidence,
	}
}

// reasonPruneThresholds calculates auto-prune thresholds for scratchpads/journals.
func reasonPruneThresholds(input map[string]any, repoPath string) reasonResult {
	reasoning := map[string]any{}

	// Scan .ralph directory for size info.
	ralphDir := filepath.Join(repoPath, ".ralph")
	entries, err := os.ReadDir(ralphDir)
	if err != nil {
		reasoning["error"] = fmt.Sprintf("cannot read .ralph dir: %v", err)
		return reasonResult{
			Reasoning:      reasoning,
			Recommendation: "Cannot calculate thresholds — .ralph directory not accessible",
			Confidence:     "low",
		}
	}

	var totalSize int64
	scratchpadCount := 0
	scratchpadSizes := make(map[string]int64)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		totalSize += info.Size()
		if strings.HasSuffix(e.Name(), "_scratchpad.md") {
			scratchpadCount++
			name := strings.TrimSuffix(e.Name(), "_scratchpad.md")
			scratchpadSizes[name] = info.Size()
		}
	}

	reasoning["ralph_dir_total_bytes"] = totalSize
	reasoning["scratchpad_count"] = scratchpadCount
	reasoning["scratchpad_sizes"] = scratchpadSizes

	// Threshold recommendations based on size.
	maxItemsDefault := 100
	if maxItems, ok := input["max_items"].(float64); ok && maxItems > 0 {
		maxItemsDefault = int(maxItems)
	}

	pruneNeeded := make([]string, 0)
	const pruneThresholdBytes = 50 * 1024 // 50KB

	for name, size := range scratchpadSizes {
		if size > pruneThresholdBytes {
			pruneNeeded = append(pruneNeeded, name)
		}
	}

	reasoning["prune_threshold_bytes"] = pruneThresholdBytes
	reasoning["max_items_setting"] = maxItemsDefault
	reasoning["needs_pruning"] = pruneNeeded

	recommendation := "No scratchpads need pruning"
	confidence := "high"
	if len(pruneNeeded) > 0 {
		recommendation = fmt.Sprintf("%d scratchpad(s) exceed %dKB threshold: %s — run journal_prune with keep=%d",
			len(pruneNeeded), pruneThresholdBytes/1024, strings.Join(pruneNeeded, ", "), maxItemsDefault)
		confidence = "medium"
	}

	return reasonResult{
		Reasoning:      reasoning,
		Recommendation: recommendation,
		Confidence:     confidence,
	}
}

// reasonProviderSelection explains provider selection logic.
func reasonProviderSelection(input map[string]any, srv *Server) reasonResult {
	reasoning := map[string]any{}

	taskType, _ := input["task_type"].(string)
	if taskType == "" {
		taskType = "general"
	}
	reasoning["task_type"] = taskType

	// Provider capabilities matrix.
	capabilities := map[string]map[string]string{
		"claude": {
			"strength":  "code generation, debugging, architecture",
			"weakness":  "higher cost for simple tasks",
			"best_for":  "complex multi-file changes, refactoring",
			"cache":     "cache_control breakpoints",
		},
		"gemini": {
			"strength":  "cost-effective, large context window",
			"weakness":  "less precise code edits",
			"best_for":  "documentation, analysis, research tasks",
			"cache":     "cachedContents API",
		},
		"openai": {
			"strength":  "balanced performance/cost, good at tests",
			"weakness":  "smaller context than gemini",
			"best_for":  "test generation, code review, formatting",
			"cache":     "automatic prefix caching",
		},
	}
	reasoning["provider_capabilities"] = capabilities

	// Recommendation based on task type.
	recommendation := "Use Claude for complex code changes, Gemini for analysis/docs, OpenAI for tests"
	confidence := "medium"

	taskProviderMap := map[string]string{
		"refactor":      "claude",
		"debug":         "claude",
		"architecture":  "claude",
		"documentation": "gemini",
		"research":      "gemini",
		"analysis":      "gemini",
		"test":          "openai",
		"review":        "openai",
		"lint":          "openai",
		"general":       "claude",
	}

	if provider, ok := taskProviderMap[taskType]; ok {
		reasoning["recommended_provider"] = provider
		reasoning["reason"] = capabilities[provider]["best_for"]
		recommendation = fmt.Sprintf("Use %s for %s tasks (%s)", provider, taskType, capabilities[provider]["best_for"])
		confidence = "high"
	}

	// Check if bandit has data.
	if srv.Bandit != nil {
		reasoning["bandit_active"] = true
		recommendation += " — bandit selector is active and will adapt based on outcomes"
	}

	return reasonResult{
		Reasoning:      reasoning,
		Recommendation: recommendation,
		Confidence:     confidence,
	}
}
