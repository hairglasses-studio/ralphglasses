package mcpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

type repoScheduleEntry struct {
	ScheduleID  string   `json:"schedule_id"`
	CronExpr    string   `json:"cron_expr"`
	CycleConfig string   `json:"cycle_config"`
	CreatedAt   string   `json:"created_at"`
	Enabled     *bool    `json:"enabled,omitempty"`
	NextRuns    []string `json:"next_runs,omitempty"`
}

func defaultEnabledPointer(v bool) *bool {
	return &v
}

func listRepoScheduleEntries(repoPath string) ([]repoScheduleEntry, error) {
	dir := filepath.Join(repoPath, ".ralph", "schedules")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	out := make([]repoScheduleEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var schedule repoScheduleEntry
		if err := json.Unmarshal(data, &schedule); err != nil {
			return nil, err
		}
		if schedule.ScheduleID == "" || schedule.CronExpr == "" {
			continue
		}
		out = append(out, schedule)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ScheduleID < out[j].ScheduleID
	})
	return out, nil
}

func computeScheduleNextRuns(cronExpr string, count int) ([]string, error) {
	runs, err := session.ComputeNextAutomationCronRuns(cronExpr, time.Now().UTC(), count)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(runs))
	for _, runAt := range runs {
		out = append(out, runAt.Format(time.RFC3339))
	}
	return out, nil
}

func writeRepoScheduleEntry(repoPath string, entry repoScheduleEntry) (string, error) {
	if err := session.ValidateAutomationCron(entry.CronExpr); err != nil {
		return "", err
	}
	if entry.ScheduleID == "" {
		return "", fmt.Errorf("schedule_id required")
	}
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if entry.Enabled == nil {
		entry.Enabled = defaultEnabledPointer(true)
	}
	nextRuns, err := computeScheduleNextRuns(entry.CronExpr, 3)
	if err != nil {
		return "", err
	}
	entry.NextRuns = nextRuns

	dir := filepath.Join(repoPath, ".ralph", "schedules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, entry.ScheduleID+".json")
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return "", err
	}
	// Atomic write: write to temp file then rename to avoid corruption on crash.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup
		return "", err
	}
	return path, nil
}

func updateRepoScheduleEnabled(repoPath, scheduleID string, enabled bool) (*repoScheduleEntry, string, error) {
	entries, err := listRepoScheduleEntries(repoPath)
	if err != nil {
		return nil, "", err
	}
	for i := range entries {
		if entries[i].ScheduleID != scheduleID {
			continue
		}
		entries[i].Enabled = defaultEnabledPointer(enabled)
		path, err := writeRepoScheduleEntry(repoPath, entries[i])
		if err != nil {
			return nil, "", err
		}
		return &entries[i], path, nil
	}
	return nil, "", fmt.Errorf("schedule not found: %s", scheduleID)
}

func decodeScheduleConfig(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func repoSchedulePresentation(entry repoScheduleEntry) map[string]any {
	config := decodeScheduleConfig(entry.CycleConfig)
	jobKind := "session"
	prompt := ""
	objective := ""
	name := ""
	maxTasks := 0
	if config != nil {
		if v, ok := config["job_kind"].(string); ok && strings.TrimSpace(v) != "" {
			jobKind = strings.TrimSpace(v)
		}
		if v, ok := config["prompt"].(string); ok {
			prompt = strings.TrimSpace(v)
		}
		if v, ok := config["objective"].(string); ok {
			objective = strings.TrimSpace(v)
		}
		if v, ok := config["name"].(string); ok {
			name = strings.TrimSpace(v)
		}
		if v, ok := config["max_tasks"].(float64); ok {
			maxTasks = int(v)
		}
	}
	enabled := true
	if entry.Enabled != nil {
		enabled = *entry.Enabled
	}
	return map[string]any{
		"id":              entry.ScheduleID,
		"schedule_id":     entry.ScheduleID,
		"cron_expression": entry.CronExpr,
		"cron_expr":       entry.CronExpr,
		"enabled":         enabled,
		"created_at":      entry.CreatedAt,
		"next_runs":       entry.NextRuns,
		"job_kind":        jobKind,
		"prompt":          prompt,
		"objective":       objective,
		"name":            name,
		"max_tasks":       maxTasks,
		"cycle_config":    entry.CycleConfig,
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
