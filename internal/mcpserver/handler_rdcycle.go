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
	repoPath, err := s.resolveRepoPath(repo)
	if err != nil {
		return codedError(ErrRepoNotFound, err.Error()), nil
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
		providerHint = "claude"
	default:
		difficulty = 0.8
		providerHint = "claude"
	}

	estimatedCost := difficulty * 0.20

	result := map[string]any{
		"finding_id":      findingID,
		"scratchpad":      scratchpadName,
		"title":           title,
		"description":     description,
		"difficulty_score": difficulty,
		"provider_hint":   providerHint,
		"estimated_cost":  estimatedCost,
		"status":          "ready",
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

	repoPath, err := s.resolveRepoPath(repo)
	if err != nil {
		return codedError(ErrRepoNotFound, err.Error()), nil
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
		"baseline_id": baselineID,
		"repo":        repo,
		"metrics":     snapshot,
		"captured_at": capturedAt,
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
	repoPath, err := s.resolveRepoPath(repo)
	if err != nil {
		return codedError(ErrRepoNotFound, err.Error()), nil
	}

	planID := fmt.Sprintf("plan-%d", time.Now().Unix())
	ralphDir := filepath.Join(repoPath, ".ralph")

	// Read all scratchpad files.
	scratchpads, err := filepath.Glob(filepath.Join(ralphDir, "*_scratchpad.md"))
	if err != nil {
		scratchpads = nil
	}

	type planItem struct {
		Text       string  `json:"text"`
		Source     string  `json:"source"`
		Score      float64 `json:"score"`
		WordCount  int     `json:"word_count"`
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
	repoPath, err := s.resolveRepoPath(repo)
	if err != nil {
		return codedError(ErrRepoNotFound, err.Error()), nil
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

