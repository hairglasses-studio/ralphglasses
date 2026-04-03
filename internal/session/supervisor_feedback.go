package session

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// GenerateWeeklyReport creates a weekly performance report and persists it
// to .ralph/weekly_reports/. Returns the generated report for further processing.
func (s *Supervisor) GenerateWeeklyReport() (*WeeklyReport, error) {
	s.mu.Lock()
	mgr := s.mgr
	repoPath := s.RepoPath
	s.mu.Unlock()

	if mgr == nil {
		return nil, fmt.Errorf("supervisor: manager not set")
	}

	gen := NewWeeklyReportGenerator(7)
	input := mgr.buildReportInput()
	report := gen.Generate(input)

	// Persist report to disk
	reportDir := filepath.Join(repoPath, ".ralph", "weekly_reports")
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		slog.Warn("supervisor: failed to create weekly report dir", "error", err)
	} else {
		filename := fmt.Sprintf("%s.json", report.GeneratedAt.Format("2006-01-02"))
		data, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(filepath.Join(reportDir, filename), data, 0644); err != nil {
			slog.Warn("supervisor: failed to write weekly report", "error", err)
		} else {
			slog.Info("supervisor: weekly report saved",
				"path", filepath.Join(reportDir, filename),
				"anomalies", len(report.Anomalies),
				"recommendations", len(report.Recommendations),
			)
		}
	}

	return &report, nil
}

// EscalateWeeklyAnomalies converts weekly report anomalies into improvement
// notes that the auto-optimizer can act upon.
func (s *Supervisor) EscalateWeeklyAnomalies(report *WeeklyReport) {
	if report == nil || len(report.Anomalies) == 0 {
		return
	}

	for _, anomaly := range report.Anomalies {
		slog.Info("supervisor: weekly anomaly detected",
			"severity", anomaly.Severity,
			"category", anomaly.Category,
			"description", anomaly.Description,
			"value", anomaly.Value,
			"threshold", anomaly.Threshold,
		)
	}

	slog.Info("supervisor: escalated weekly anomalies", "total", len(report.Anomalies))
}

// RunFeedbackLoop executes a single pass of the cross-subsystem feedback loop:
// 1. Generate and persist weekly report (if due)
// 2. Escalate anomalies from weekly reports
// 3. Feed reflexion rules into improvement logging
// 4. Save bandit state for persistence across restarts
//
// Called from the supervisor's tick loop at reduced frequency (every 10 ticks).
func (s *Supervisor) RunFeedbackLoop() {
	s.mu.Lock()
	tickCount := s.tickCount
	mgr := s.mgr
	repoPath := s.RepoPath
	s.mu.Unlock()

	// Run feedback loop every 10 supervisor ticks (~10 minutes at 60s tick)
	if tickCount%10 != 0 {
		return
	}

	// 1. Weekly report generation (every ~168 ticks at 60s = ~2.8 hours)
	if tickCount > 0 && tickCount%168 == 0 {
		report, err := s.GenerateWeeklyReport()
		if err != nil {
			slog.Warn("supervisor: weekly report failed", "error", err)
		} else {
			s.EscalateWeeklyAnomalies(report)
		}
	}

	// 2. Extract reflexion rules and log high-count patterns
	if mgr != nil && mgr.reflexion != nil {
		rules := mgr.reflexion.Rules()
		for _, rule := range rules {
			if rule.Count >= 3 {
				slog.Debug("supervisor: high-frequency reflexion rule",
					"pattern", rule.Pattern,
					"failure_mode", rule.FailureMode,
					"count", rule.Count,
				)
			}
		}
	}

	// 3. Evolve prompts for task types with low success rates
	if mgr != nil && mgr.promptEvolution != nil {
		for _, taskType := range []string{"lint", "test", "feature", "refactor"} {
			mgr.EvolvePrompts(taskType)
		}
	}

	// 4. Persist bandit state for cross-restart learning
	if mgr != nil {
		ralphDir := filepath.Join(repoPath, ".ralph")
		mgr.SaveBanditState(ralphDir)
	}
}

// buildReportInput creates a ReportInput from the manager's completed sessions.
func (m *Manager) buildReportInput() ReportInput {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()

	summaries := make([]SessionSummary, 0, len(m.sessions))
	for _, s := range m.sessions {
		s.mu.Lock()
		summary := SessionSummary{
			ID:        s.ID,
			Provider:  string(s.Provider),
			Status:    string(s.Status),
			CostUSD:   s.SpentUSD,
			TurnCount: s.TurnCount,
			StartedAt: s.LaunchedAt,
		}
		if s.EndedAt != nil {
			summary.DurSec = s.EndedAt.Sub(s.LaunchedAt).Seconds()
		}
		s.mu.Unlock()
		summaries = append(summaries, summary)
	}

	return ReportInput{
		Sessions: summaries,
	}
}
