package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleFindingToTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	findingID := getStringArg(req, "finding_id")
	if findingID == "" {
		return codedError(ErrInvalidParams, "finding_id required"), nil
	}
	scratchpadName := getStringArg(req, "scratchpad_name")
	if scratchpadName == "" {
		return codedError(ErrInvalidParams, "scratchpad_name required"), nil
	}

	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	scratchpadPath := filepath.Join(repoPath, ".ralph", scratchpadName+"_scratchpad.md")
	data, err := os.ReadFile(scratchpadPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("cannot read scratchpad: %v", err)), nil
	}
	content := string(data)

	// Find the finding by searching for finding_id text.
	lines := strings.Split(content, "\n")
	var findingLines []string
	found := false
	for i, line := range lines {
		if strings.Contains(line, findingID) {
			found = true
			// Collect from this line until the next section header or end.
			for j := i; j < len(lines); j++ {
				if j > i && (strings.HasPrefix(lines[j], "#") || strings.Contains(lines[j], "FINDING-")) {
					break
				}
				findingLines = append(findingLines, lines[j])
			}
			break
		}
	}

	if !found {
		return codedError(ErrInvalidParams, fmt.Sprintf("finding %q not found in scratchpad", findingID)), nil
	}

	title := strings.TrimSpace(findingLines[0])
	title = strings.TrimLeft(title, "# ")
	description := strings.TrimSpace(strings.Join(findingLines, "\n"))

	// Heuristic difficulty from word count.
	wordCount := len(strings.Fields(description))
	var difficulty float64
	var providerHint string
	switch {
	case wordCount < 50:
		difficulty = 0.3
		providerHint = "gemini"
	case wordCount < 150:
		difficulty = 0.5
		providerHint = "codex"
	default:
		difficulty = 0.8
		providerHint = "claude"
	}

	estimatedCost := difficulty * 0.20

	result := map[string]any{
		"finding_id":       findingID,
		"scratchpad":       scratchpadName,
		"title":            title,
		"description":      description,
		"difficulty_score": difficulty,
		"provider_hint":    providerHint,
		"estimated_cost":   estimatedCost,
		"status":           "ready",
	}
	return jsonResult(result), nil
}

func (s *Server) handleCycleBaseline(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	if repo == "" {
		return codedError(ErrInvalidParams, "repo required"), nil
	}
	metricsStr := getStringArg(req, "metrics")

	metricNames := []string{"test_pass_rate", "coverage_pct", "vet_clean", "build_ok", "lint_score"}
	if metricsStr != "" {
		metricNames = strings.Split(metricsStr, ",")
		for i := range metricNames {
			metricNames[i] = strings.TrimSpace(metricNames[i])
		}
	}

	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	baselineID := fmt.Sprintf("baseline-%s-%d", repo, time.Now().Unix())
	snapshot := make(map[string]float64, len(metricNames))
	for _, m := range metricNames {
		snapshot[m] = 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Run go build.
	buildCmd := exec.CommandContext(ctx, "go", "build", "./...")
	buildCmd.Dir = repoPath
	buildOut, buildErr := buildCmd.CombinedOutput()
	buildOK := buildErr == nil
	if sliceContains(metricNames, "build_ok") {
		if buildOK {
			snapshot["build_ok"] = 1
		}
	}

	// Run go test with coverage.
	testCmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-coverprofile=coverage.out", "./...")
	testCmd.Dir = repoPath
	testOut, _ := testCmd.CombinedOutput()
	testOutput := string(testOut)

	// Parse coverage.
	covRe := regexp.MustCompile(`coverage:\s+(\d+\.?\d*)%`)
	var coverages []float64
	for _, match := range covRe.FindAllStringSubmatch(testOutput, -1) {
		if v, err := strconv.ParseFloat(match[1], 64); err == nil {
			coverages = append(coverages, v)
		}
	}
	if len(coverages) > 0 && sliceContains(metricNames, "coverage_pct") {
		var sum float64
		for _, c := range coverages {
			sum += c
		}
		snapshot["coverage_pct"] = sum / float64(len(coverages))
	}

	// Count PASS/FAIL.
	passCount := strings.Count(testOutput, "--- PASS")
	failCount := strings.Count(testOutput, "--- FAIL")
	total := passCount + failCount
	if total > 0 && sliceContains(metricNames, "test_pass_rate") {
		snapshot["test_pass_rate"] = float64(passCount) / float64(total) * 100
	}

	// Run go vet.
	vetCmd := exec.CommandContext(ctx, "go", "vet", "./...")
	vetCmd.Dir = repoPath
	_, vetErr := vetCmd.CombinedOutput()
	if sliceContains(metricNames, "vet_clean") {
		if vetErr == nil {
			snapshot["vet_clean"] = 1
		}
	}

	// Write baseline to disk.
	baselinesDir := filepath.Join(repoPath, ".ralph", "cycle_baselines")
	if err := os.MkdirAll(baselinesDir, 0o755); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("cannot create baselines dir: %v", err)), nil
	}

	capturedAt := time.Now().UTC().Format(time.RFC3339)
	baselineData := map[string]any{
		"baseline_id":  baselineID,
		"repo":         repo,
		"metrics":      snapshot,
		"captured_at":  capturedAt,
		"build_output": truncate(string(buildOut), 500),
		"test_output":  truncate(testOutput, 1000),
	}
	baselineJSON, _ := json.MarshalIndent(baselineData, "", "  ")
	baselinePath := filepath.Join(baselinesDir, baselineID+".json")
	if err := os.WriteFile(baselinePath, baselineJSON, 0o644); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("cannot write baseline: %v", err)), nil
	}

	result := map[string]any{
		"baseline_id": baselineID,
		"repo":        repo,
		"metrics":     snapshot,
		"captured_at": capturedAt,
		"path":        baselinePath,
		"status":      "captured",
	}
	return jsonResult(result), nil
}

func (s *Server) handleCyclePlan(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	previousCycleID := getStringArg(req, "previous_cycle_id")
	maxTasks := int(getNumberArg(req, "max_tasks", 10))
	budget := getNumberArg(req, "budget", 5.0)

	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	planID := fmt.Sprintf("plan-%d", time.Now().Unix())
	ralphDir := filepath.Join(repoPath, ".ralph")

	// Read all scratchpad files.
	scratchpads, err := filepath.Glob(filepath.Join(ralphDir, "*_scratchpad.md"))
	if err != nil {
		scratchpads = nil
	}

	type planItem struct {
		Text      string  `json:"text"`
		Source    string  `json:"source"`
		Score     float64 `json:"score"`
		WordCount int     `json:"word_count"`
		CostEst   float64 `json:"cost_estimate"`
	}

	var items []planItem
	for _, sp := range scratchpads {
		data, err := os.ReadFile(sp)
		if err != nil {
			continue
		}
		source := filepath.Base(sp)
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "#") {
				items = append(items, planItem{
					Text:      trimmed,
					Source:    source,
					Score:     1.0,
					WordCount: len(strings.Fields(trimmed)),
				})
			}
		}
	}

	// Read improvement_patterns.json for scoring boost.
	type patternEntry struct {
		Pattern    string `json:"pattern"`
		Frequency  int    `json:"frequency"`
		Recurrence int    `json:"recurrence"`
	}
	var patterns []patternEntry
	patternsPath := filepath.Join(ralphDir, "improvement_patterns.json")
	if pdata, err := os.ReadFile(patternsPath); err == nil {
		_ = json.Unmarshal(pdata, &patterns)
	}

	// Score items.
	for i := range items {
		textLower := strings.ToLower(items[i].Text)
		for _, p := range patterns {
			pLower := strings.ToLower(p.Pattern)
			if strings.Contains(textLower, pLower) {
				// Negative pattern match.
				items[i].Score += 0.5
				// Rule match.
				items[i].Score += 0.3
				// Recurrence.
				items[i].Score += 0.1 * float64(p.Recurrence)
			}
		}
		// Estimate cost based on word count.
		switch {
		case items[i].WordCount < 10:
			items[i].CostEst = 0.10
		case items[i].WordCount < 30:
			items[i].CostEst = 0.25
		default:
			items[i].CostEst = 0.50
		}
	}

	// Sort by score descending.
	sort.Slice(items, func(a, b int) bool {
		return items[a].Score > items[b].Score
	})

	// Limit to maxTasks and filter by budget.
	var selected []planItem
	var totalCost float64
	for _, item := range items {
		if len(selected) >= maxTasks {
			break
		}
		if totalCost+item.CostEst > budget {
			continue
		}
		totalCost += item.CostEst
		selected = append(selected, item)
	}

	// Convert to any slice for JSON.
	taskList := make([]any, len(selected))
	for i, item := range selected {
		taskList[i] = map[string]any{
			"text":          item.Text,
			"source":        item.Source,
			"score":         item.Score,
			"word_count":    item.WordCount,
			"cost_estimate": item.CostEst,
		}
	}

	result := map[string]any{
		"plan_id":           planID,
		"previous_cycle_id": previousCycleID,
		"tasks":             taskList,
		"total_cost":        totalCost,
		"constraints": map[string]any{
			"max_tasks":  maxTasks,
			"budget_usd": budget,
		},
		"status": "planned",
	}
	return jsonResult(result), nil
}

func (s *Server) handleCycleMerge(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	worktreePaths := getStringArg(req, "worktree_paths")
	if worktreePaths == "" {
		return codedError(ErrInvalidParams, "worktree_paths required"), nil
	}
	conflictStrategy := getStringArg(req, "conflict_strategy")
	if conflictStrategy == "" {
		conflictStrategy = "manual"
	}

	paths := splitCSV(worktreePaths)

	// Track which files are changed in which worktrees.
	type fileChange struct {
		worktree string
		relPath  string
	}
	fileWorktrees := make(map[string][]string) // relPath -> list of worktree paths
	worktreeFiles := make(map[string][]string) // worktree -> list of changed files

	for _, wt := range paths {
		if _, err := os.Stat(wt); err != nil {
			return codedError(ErrFilesystem, fmt.Sprintf("worktree path does not exist: %s", wt)), nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, "git", "-C", wt, "diff", "HEAD", "--name-only")
		out, err := cmd.Output()
		cancel()
		if err != nil {
			return codedError(ErrInternal, fmt.Sprintf("git diff failed in %s: %v", wt, err)), nil
		}

		changed := splitLines(string(out))
		worktreeFiles[wt] = changed
		for _, f := range changed {
			fileWorktrees[f] = append(fileWorktrees[f], wt)
		}
	}

	// Separate conflicts from non-conflicts.
	var merged []string
	var conflicts []map[string]any
	var skipped []string

	for relPath, wts := range fileWorktrees {
		if len(wts) > 1 {
			conflicts = append(conflicts, map[string]any{
				"file":       relPath,
				"worktrees":  wts,
				"resolution": "manual_required",
			})
			continue
		}

		// Non-conflicting: copy from worktree to main repo.
		srcWorktree := wts[0]
		srcPath := filepath.Join(srcWorktree, relPath)

		// Determine the main repo path. Walk up from worktree to find .git reference.
		// Use the parent of .ralph/worktrees as main repo, or just use the file
		// relative to CWD. We copy to the first path that's a parent with .git.
		mainRepo := findMainRepo(srcWorktree)
		if mainRepo == "" {
			skipped = append(skipped, relPath)
			continue
		}

		dstPath := filepath.Join(mainRepo, relPath)

		// Ensure destination directory exists.
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			skipped = append(skipped, relPath)
			continue
		}

		srcData, err := os.ReadFile(srcPath)
		if err != nil {
			skipped = append(skipped, relPath)
			continue
		}

		// Preserve file permissions.
		perm := os.FileMode(0o644)
		if info, err := os.Stat(srcPath); err == nil {
			perm = info.Mode().Perm()
		}

		if err := os.WriteFile(dstPath, srcData, perm); err != nil {
			skipped = append(skipped, relPath)
			continue
		}

		merged = append(merged, relPath)
	}

	conflictList := make([]any, len(conflicts))
	for i, c := range conflicts {
		conflictList[i] = c
	}

	result := map[string]any{
		"merge_status":      "completed",
		"worktree_count":    len(paths),
		"worktree_paths":    paths,
		"conflict_strategy": conflictStrategy,
		"merged_files":      merged,
		"conflicts":         conflictList,
		"skipped":           skipped,
		"merged_count":      len(merged),
		"conflict_count":    len(conflicts),
		"skipped_count":     len(skipped),
		"status":            "completed",
	}
	return jsonResult(result), nil
}

func (s *Server) handleCycleSchedule(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cronExpr := getStringArg(req, "cron_expr")
	if cronExpr == "" {
		return codedError(ErrInvalidParams, "cron_expr required"), nil
	}
	cycleConfig := getStringArg(req, "cycle_config")

	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	// Validate cron expression (5 fields: min hour dom month dow).
	cronFields := strings.Fields(cronExpr)
	if len(cronFields) != 5 {
		return codedError(ErrInvalidParams, "cron_expr must have exactly 5 fields (min hour dom month dow)"), nil
	}

	cronFieldRe := regexp.MustCompile(`^(\*|\d+|\*/\d+)$`)
	for i, field := range cronFields {
		if !cronFieldRe.MatchString(field) {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid cron field %d: %q", i, field)), nil
		}
	}

	scheduleID := fmt.Sprintf("sched-%d", time.Now().Unix())

	// Compute next 3 run times.
	nextRuns := computeNextCronRuns(cronFields, time.Now().UTC(), 3)
	nextRunStrs := make([]string, len(nextRuns))
	for i, t := range nextRuns {
		nextRunStrs[i] = t.Format(time.RFC3339)
	}

	// Write schedule to disk.
	schedDir := filepath.Join(repoPath, ".ralph", "schedules")
	if err := os.MkdirAll(schedDir, 0o755); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("cannot create schedules dir: %v", err)), nil
	}

	createdAt := time.Now().UTC().Format(time.RFC3339)
	schedData := map[string]any{
		"schedule_id":  scheduleID,
		"cron_expr":    cronExpr,
		"cycle_config": cycleConfig,
		"created_at":   createdAt,
		"next_runs":    nextRunStrs,
	}
	schedJSON, _ := json.MarshalIndent(schedData, "", "  ")
	schedPath := filepath.Join(schedDir, scheduleID+".json")
	if err := os.WriteFile(schedPath, schedJSON, 0o644); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("cannot write schedule: %v", err)), nil
	}

	result := map[string]any{
		"schedule_id":  scheduleID,
		"cron_expr":    cronExpr,
		"cycle_config": cycleConfig,
		"created_at":   createdAt,
		"next_runs":    nextRunStrs,
		"path":         schedPath,
		"status":       "created",
	}
	return jsonResult(result), nil
}

// --- Helper functions ---

// sliceContains checks if a string slice contains a value.
func sliceContains(ss []string, val string) bool {
	for _, s := range ss {
		if s == val {
			return true
		}
	}
	return false
}

// truncate limits a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}

// findMainRepo walks up from a worktree path to find the main repo
// (directory containing .git as a file or directory).
func findMainRepo(worktreePath string) string {
	// Git worktrees have a .git file (not directory) pointing to the main repo.
	dir := worktreePath
	for i := 0; i < 10; i++ {
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				// This is the main repo.
				return dir
			}
			// .git is a file — read it to find the main repo's gitdir.
			data, err := os.ReadFile(gitPath)
			if err == nil {
				line := strings.TrimSpace(string(data))
				if strings.HasPrefix(line, "gitdir:") {
					gitdir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
					if !filepath.IsAbs(gitdir) {
						gitdir = filepath.Join(dir, gitdir)
					}
					// gitdir points to .git/worktrees/<name>, main repo is two levels up.
					mainGit := filepath.Dir(filepath.Dir(gitdir))
					return filepath.Dir(mainGit)
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// computeNextCronRuns computes the next N run times from a parsed cron expression.
// Supports: *, specific numbers, */N for each of the 5 fields.
func computeNextCronRuns(fields []string, from time.Time, count int) []time.Time {
	var runs []time.Time
	// Start from the next minute.
	t := from.Truncate(time.Minute).Add(time.Minute)

	maxIter := 525600 // scan up to 1 year of minutes
	for i := 0; i < maxIter && len(runs) < count; i++ {
		if cronMatch(fields, t) {
			runs = append(runs, t)
		}
		t = t.Add(time.Minute)
	}
	return runs
}

// cronMatch checks if a time matches a 5-field cron expression.
func cronMatch(fields []string, t time.Time) bool {
	values := []int{
		t.Minute(),
		t.Hour(),
		t.Day(),
		int(t.Month()),
		int(t.Weekday()), // 0=Sunday
	}
	for i, field := range fields {
		if !cronFieldMatch(field, values[i]) {
			return false
		}
	}
	return true
}

// cronFieldMatch checks if a single cron field matches a value.
func cronFieldMatch(field string, value int) bool {
	if field == "*" {
		return true
	}
	if strings.HasPrefix(field, "*/") {
		n, err := strconv.Atoi(strings.TrimPrefix(field, "*/"))
		if err != nil || n <= 0 {
			return false
		}
		return value%n == 0
	}
	n, err := strconv.Atoi(field)
	if err != nil {
		return false
	}
	return value == n
}

// --- Tier 2 rdcycle tools ---

func (s *Server) handleLoopReplay(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	loopID := getStringArg(req, "loop_id")
	if loopID == "" {
		return codedError(ErrInvalidParams, "loop_id is required"), nil
	}
	iteration := int(getNumberArg(req, "iteration", -1))
	if iteration < 0 {
		return codedError(ErrInvalidParams, "iteration is required"), nil
	}
	overridesStr := getStringArg(req, "overrides")

	// Find loop run file in the first available repo's .ralph dir.
	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	runPath := filepath.Join(repoPath, ".ralph", "loop_runs", loopID+".json")
	data, err := os.ReadFile(runPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("cannot read loop run file: %v", err)), nil
	}

	var loopRun map[string]any
	if err := json.Unmarshal(data, &loopRun); err != nil {
		return codedError(ErrInternal, fmt.Sprintf("invalid loop run JSON: %v", err)), nil
	}

	// Find the specified iteration.
	iterations, _ := loopRun["iterations"].([]any)
	if iterations == nil {
		return codedError(ErrInvalidParams, "loop run has no iterations"), nil
	}
	if iteration >= len(iterations) {
		return codedError(ErrInvalidParams, fmt.Sprintf("iteration %d out of range (max %d)", iteration, len(iterations)-1)), nil
	}

	iterData, ok := iterations[iteration].(map[string]any)
	if !ok {
		return codedError(ErrInternal, "iteration data is not a valid object"), nil
	}

	// Build merged config from iteration data.
	newConfig := make(map[string]any)
	if cfg, ok := iterData["config"].(map[string]any); ok {
		for k, v := range cfg {
			newConfig[k] = v
		}
	}
	// Copy top-level loop fields as defaults.
	for _, key := range []string{"model", "provider", "budget", "prompt"} {
		if v, ok := loopRun[key]; ok {
			if _, exists := newConfig[key]; !exists {
				newConfig[key] = v
			}
		}
	}

	originalError, _ := iterData["error"].(string)

	var overridesApplied map[string]any
	if overridesStr != "" {
		if err := json.Unmarshal([]byte(overridesStr), &overridesApplied); err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid overrides JSON: %v", err)), nil
		}
		for k, v := range overridesApplied {
			newConfig[k] = v
		}
	}

	result := map[string]any{
		"loop_id":           loopID,
		"iteration":         iteration,
		"new_config":        newConfig,
		"original_error":    originalError,
		"overrides_applied": overridesApplied,
		"status":            "ready_to_replay",
	}
	return jsonResult(result), nil
}

func (s *Server) handleBudgetForecast(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	loopID := getStringArg(req, "loop_id")
	if loopID == "" {
		return codedError(ErrInvalidParams, "loop_id is required"), nil
	}
	iterations := int(getNumberArg(req, "iterations", 10))

	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	costPath := filepath.Join(repoPath, ".ralph", "cost_observations.json")
	data, err := os.ReadFile(costPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("cannot read cost_observations.json: %v", err)), nil
	}

	var observations []map[string]any
	if err := json.Unmarshal(data, &observations); err != nil {
		return codedError(ErrInternal, fmt.Sprintf("invalid cost_observations JSON: %v", err)), nil
	}

	// Filter entries matching loop_id.
	var costs []float64
	for _, obs := range observations {
		obsLoop, _ := obs["loop_id"].(string)
		if obsLoop != loopID {
			continue
		}
		cost, ok := obs["cost"].(float64)
		if !ok {
			if costUSD, ok := obs["cost_usd"].(float64); ok {
				cost = costUSD
			} else {
				continue
			}
		}
		costs = append(costs, cost)
	}

	if len(costs) == 0 {
		return codedError(ErrInvalidParams, fmt.Sprintf("no cost observations found for loop_id %q", loopID)), nil
	}

	// Compute mean.
	var sum float64
	for _, c := range costs {
		sum += c
	}
	mean := sum / float64(len(costs))

	// Sort for percentiles.
	sort.Float64s(costs)
	p50 := percentileFloat(costs, 50)
	p95 := percentileFloat(costs, 95)

	// Confidence based on sample size.
	confidence := float64(len(costs)) / float64(len(costs)+5) * 100 // asymptotic to 100%
	if confidence > 99 {
		confidence = 99
	}

	result := map[string]any{
		"loop_id":                 loopID,
		"estimated_cost_p50":      p50 * float64(iterations),
		"estimated_cost_p95":      p95 * float64(iterations),
		"cost_per_iteration_mean": mean,
		"cost_per_iteration_p50":  p50,
		"cost_per_iteration_p95":  p95,
		"iterations_analyzed":     len(costs),
		"iterations_requested":    iterations,
		"confidence_pct":          confidence,
	}
	return jsonResult(result), nil
}

// percentile computes the p-th percentile from a sorted float64 slice.
func percentileFloat(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := p / 100 * float64(len(sorted)-1)
	lower := int(idx)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

func (s *Server) handleDiffReview(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoArg := getStringArg(req, "repo")
	if repoArg == "" {
		return codedError(ErrInvalidParams, "repo is required"), nil
	}
	ref := getStringArg(req, "ref")
	if ref == "" {
		ref = "HEAD"
	}
	checksStr := getStringArg(req, "checks")
	checks := map[string]bool{
		"scope_creep":   true,
		"missing_tests": true,
		"todos":         true,
		"style":         true,
	}
	if checksStr != "" {
		checks = make(map[string]bool)
		for _, c := range splitCSV(checksStr) {
			checks[c] = true
		}
	}

	repoPath, errRes := s.resolveRepoPath(repoArg)
	if errRes != nil {
		return errRes, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "diff", ref+"~1.."+ref)
	out, err := cmd.Output()
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("git diff failed: %v", err)), nil
	}

	diffOutput := string(out)
	if diffOutput == "" {
		return jsonResult(map[string]any{
			"issues":         []any{},
			"files_reviewed": 0,
			"ref":            ref,
			"status":         "clean",
		}), nil
	}

	// Parse diff into per-file hunks.
	type fileHunk struct {
		path         string
		addedLines   []string
		linesChanged int
	}

	var hunks []fileHunk
	fileRe := regexp.MustCompile(`^diff --git a/.+ b/(.+)$`)
	dirs := make(map[string]bool)

	var current *fileHunk
	for _, line := range strings.Split(diffOutput, "\n") {
		if m := fileRe.FindStringSubmatch(line); m != nil {
			if current != nil {
				hunks = append(hunks, *current)
			}
			current = &fileHunk{path: m[1]}
			dirs[filepath.Dir(m[1])] = true
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			current.addedLines = append(current.addedLines, line[1:])
			current.linesChanged++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			current.linesChanged++
		}
	}
	if current != nil {
		hunks = append(hunks, *current)
	}

	type issue struct {
		Severity       string `json:"severity"`
		File           string `json:"file"`
		Line           int    `json:"line,omitempty"`
		Check          string `json:"check"`
		Recommendation string `json:"recommendation"`
	}
	var issues []issue

	// Check: scope_creep — flag if > 5 distinct directories changed.
	if checks["scope_creep"] && len(dirs) > 5 {
		issues = append(issues, issue{
			Severity:       "warning",
			File:           "",
			Check:          "scope_creep",
			Recommendation: fmt.Sprintf("Changes span %d directories — consider splitting into smaller PRs", len(dirs)),
		})
	}

	// Check: missing_tests — .go files changed but no _test.go files.
	if checks["missing_tests"] {
		hasGoChanges := false
		hasTestChanges := false
		for _, h := range hunks {
			if strings.HasSuffix(h.path, ".go") && !strings.HasSuffix(h.path, "_test.go") {
				hasGoChanges = true
			}
			if strings.HasSuffix(h.path, "_test.go") {
				hasTestChanges = true
			}
		}
		if hasGoChanges && !hasTestChanges {
			issues = append(issues, issue{
				Severity:       "warning",
				File:           "",
				Check:          "missing_tests",
				Recommendation: "Go source files changed but no test files were modified — add or update tests",
			})
		}
	}

	// Check: todos — grep for TODO/FIXME/HACK in added lines.
	if checks["todos"] {
		todoRe := regexp.MustCompile(`(?i)\b(TODO|FIXME|HACK)\b`)
		for _, h := range hunks {
			for lineNum, line := range h.addedLines {
				if todoRe.MatchString(line) {
					issues = append(issues, issue{
						Severity:       "info",
						File:           h.path,
						Line:           lineNum + 1,
						Check:          "todos",
						Recommendation: fmt.Sprintf("New TODO/FIXME/HACK marker: %s", strings.TrimSpace(line)),
					})
				}
			}
		}
	}

	// Check: style — flag files with > 500 lines changed.
	if checks["style"] {
		for _, h := range hunks {
			if h.linesChanged > 500 {
				issues = append(issues, issue{
					Severity:       "warning",
					File:           h.path,
					Check:          "style",
					Recommendation: fmt.Sprintf("File has %d lines changed — consider breaking into smaller changes", h.linesChanged),
				})
			}
		}
	}

	issueList := make([]any, len(issues))
	for i, iss := range issues {
		issueList[i] = map[string]any{
			"severity":       iss.Severity,
			"file":           iss.File,
			"line":           iss.Line,
			"check":          iss.Check,
			"recommendation": iss.Recommendation,
		}
	}

	result := map[string]any{
		"issues":         issueList,
		"files_reviewed": len(hunks),
		"ref":            ref,
		"directories":    len(dirs),
		"status":         "reviewed",
	}
	return jsonResult(result), nil
}

func (s *Server) handleFindingReason(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "name is required"), nil
	}

	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	// Try scratchpads/<name>.md first, then <name>_scratchpad.md.
	scratchpadPath := filepath.Join(repoPath, ".ralph", "scratchpads", name+".md")
	data, err := os.ReadFile(scratchpadPath)
	if err != nil {
		scratchpadPath = filepath.Join(repoPath, ".ralph", name+"_scratchpad.md")
		data, err = os.ReadFile(scratchpadPath)
		if err != nil {
			return codedError(ErrFilesystem, fmt.Sprintf("cannot read scratchpad: %v", err)), nil
		}
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	// Parse findings: lines starting with FINDING- prefix or ## sections.
	type finding struct {
		ID   string
		Text string
	}
	var findings []finding

	findingRe := regexp.MustCompile(`FINDING-\d+`)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if findingRe.MatchString(trimmed) || strings.HasPrefix(trimmed, "## ") {
			// Collect text until next finding or section.
			var text []string
			for j := i; j < len(lines); j++ {
				if j > i && (findingRe.MatchString(strings.TrimSpace(lines[j])) || (strings.HasPrefix(strings.TrimSpace(lines[j]), "## ") && j != i)) {
					break
				}
				text = append(text, lines[j])
			}
			id := trimmed
			if m := findingRe.FindString(trimmed); m != "" {
				id = m
			}
			findings = append(findings, finding{ID: id, Text: strings.Join(text, "\n")})
		}
	}

	// Categorize findings by keyword matching.
	categories := map[string][]string{
		"test":        {},
		"coverage":    {},
		"error":       {},
		"performance": {},
		"config":      {},
		"docs":        {},
	}
	categoryKeywords := map[string][]string{
		"test":        {"test", "assert", "expect", "mock", "stub"},
		"coverage":    {"coverage", "cover", "uncovered", "untested"},
		"error":       {"error", "fail", "panic", "crash", "bug", "fix"},
		"performance": {"perf", "slow", "latency", "timeout", "memory", "cpu"},
		"config":      {"config", "env", "setting", "flag", "option"},
		"docs":        {"doc", "readme", "comment", "godoc"},
	}

	for _, f := range findings {
		textLower := strings.ToLower(f.Text)
		matched := false
		for cat, keywords := range categoryKeywords {
			for _, kw := range keywords {
				if strings.Contains(textLower, kw) {
					categories[cat] = append(categories[cat], f.ID)
					matched = true
					break
				}
			}
		}
		if !matched {
			// Uncategorized findings go into a general bucket.
			categories["other"] = append(categories["other"], f.ID)
		}
	}

	// Build root causes: categories with >= 3 findings.
	type rootCause struct {
		Category       string   `json:"category"`
		Count          int      `json:"count"`
		Findings       []string `json:"findings"`
		Recommendation string   `json:"recommendation"`
	}

	recommendations := map[string]string{
		"test":        "Systemic test gaps — consider adding a test coverage gate to CI",
		"coverage":    "Coverage debt accumulating — run coverage_report and set thresholds",
		"error":       "Recurring errors — investigate shared error patterns or missing validation",
		"performance": "Performance issues clustering — profile hot paths and add benchmarks",
		"config":      "Configuration complexity — consider consolidating config sources",
		"docs":        "Documentation gaps — schedule a docs improvement cycle",
		"other":       "Uncategorized findings — review and add proper categorization",
	}

	var rootCauses []any
	categoryCount := 0
	for cat, findingIDs := range categories {
		if len(findingIDs) == 0 {
			continue
		}
		categoryCount++
		if len(findingIDs) >= 3 {
			rec := recommendations[cat]
			if rec == "" {
				rec = fmt.Sprintf("Multiple findings in %q suggest systemic issues", cat)
			}
			rootCauses = append(rootCauses, map[string]any{
				"category":       cat,
				"count":          len(findingIDs),
				"findings":       findingIDs,
				"recommendation": rec,
			})
		}
	}

	result := map[string]any{
		"root_causes":    rootCauses,
		"total_findings": len(findings),
		"categories":     categoryCount,
		"scratchpad":     name,
		"status":         "analyzed",
	}
	return jsonResult(result), nil
}

func (s *Server) handleObservationCorrelate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoArg := getStringArg(req, "repo")
	if repoArg == "" {
		return codedError(ErrInvalidParams, "repo is required"), nil
	}
	hours := int(getNumberArg(req, "hours", 24))

	repoPath, errRes := s.resolveRepoPath(repoArg)
	if errRes != nil {
		return errRes, nil
	}

	// Read observations from .ralph/observations/ directory (JSONL files).
	obsDir := filepath.Join(repoPath, ".ralph", "observations")
	obsFiles, err := filepath.Glob(filepath.Join(obsDir, "*.jsonl"))
	if err != nil {
		obsFiles = nil
	}
	// Also check for single-file observations.
	if singleFile := filepath.Join(repoPath, ".ralph", "logs", "loop_observations.jsonl"); fileExists(singleFile) {
		obsFiles = append(obsFiles, singleFile)
	}

	type observation struct {
		ID        string    `json:"id"`
		Timestamp time.Time `json:"timestamp"`
		SessionID string    `json:"session_id"`
		LoopID    string    `json:"loop_id"`
		Status    string    `json:"status"`
	}

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	var observations []observation

	for _, f := range obsFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var raw map[string]any
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				continue
			}
			var obs observation
			obs.ID, _ = raw["id"].(string)
			if obs.ID == "" {
				obs.ID, _ = raw["observation_id"].(string)
			}
			obs.SessionID, _ = raw["session_id"].(string)
			obs.LoopID, _ = raw["loop_id"].(string)
			obs.Status, _ = raw["status"].(string)

			// Parse timestamp from various fields.
			for _, tsKey := range []string{"timestamp", "ts", "created_at", "time"} {
				if tsStr, ok := raw[tsKey].(string); ok {
					if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
						obs.Timestamp = t
						break
					}
					if t, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
						obs.Timestamp = t
						break
					}
				}
			}
			if obs.Timestamp.IsZero() || obs.Timestamp.Before(cutoff) {
				continue
			}
			observations = append(observations, obs)
		}
	}

	// Run git log.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "log",
		fmt.Sprintf("--since=%dh", hours),
		"--format=%H|%s|%ai",
	)
	out, err := cmd.Output()
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("git log failed: %v", err)), nil
	}

	type commit struct {
		SHA     string
		Msg     string
		Time    time.Time
		Matched bool
	}
	var commits []commit
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		t, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[2])
		commits = append(commits, commit{SHA: parts[0], Msg: parts[1], Time: t})
	}

	// Match observations to commits by timestamp proximity (within 5 minutes).
	type correlation struct {
		ObservationID string `json:"observation_id"`
		Timestamp     string `json:"timestamp"`
		CommitSHA     string `json:"commit_sha"`
		CommitMsg     string `json:"commit_msg"`
		SessionID     string `json:"session_id"`
		DeltaSeconds  int    `json:"delta_seconds"`
	}

	var correlations []any
	matchedObs := make(map[int]bool)
	for i, obs := range observations {
		for j := range commits {
			delta := obs.Timestamp.Sub(commits[j].Time)
			if delta < 0 {
				delta = -delta
			}
			if delta <= 5*time.Minute {
				correlations = append(correlations, map[string]any{
					"observation_id": obs.ID,
					"timestamp":      obs.Timestamp.Format(time.RFC3339),
					"commit_sha":     commits[j].SHA,
					"commit_msg":     commits[j].Msg,
					"session_id":     obs.SessionID,
					"delta_seconds":  int(delta.Seconds()),
				})
				matchedObs[i] = true
				commits[j].Matched = true
				break
			}
		}
	}

	unmatchedObs := len(observations) - len(matchedObs)
	unmatchedCommits := 0
	for _, c := range commits {
		if !c.Matched {
			unmatchedCommits++
		}
	}

	result := map[string]any{
		"correlations":           correlations,
		"total_observations":     len(observations),
		"total_commits":          len(commits),
		"unmatched_observations": unmatchedObs,
		"unmatched_commits":      unmatchedCommits,
		"hours":                  hours,
		"status":                 "correlated",
	}
	return jsonResult(result), nil
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
