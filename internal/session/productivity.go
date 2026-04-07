package session

import (
	"strings"
	"time"
)

const defaultProductivityNoopThreshold = 2

// ProductivitySnapshot captures whether a run is producing durable research
// and development outcomes rather than only consuming budget and time.
type ProductivitySnapshot struct {
	GeneratedAt          time.Time `json:"generated_at"`
	Productive           bool      `json:"productive"`
	Score                int       `json:"score"`
	ResearchOutputs      int       `json:"research_outputs"`
	TopicsCompleted      int       `json:"topics_completed"`
	DevelopmentOutputs   int       `json:"development_outputs"`
	VerificationFailures int       `json:"verification_failures"`
	ConsecutiveNoops     int       `json:"consecutive_noops"`
	NoopThreshold        int       `json:"noop_threshold"`
	NoopPlateau          bool      `json:"noop_plateau"`
	DedupSkips           int       `json:"dedup_skips"`
	AutonomyRejections   int       `json:"autonomy_rejections"`
	Reasons              []string  `json:"reasons,omitempty"`
}

// EmptyProductivitySnapshot returns the zero-state productivity payload used by
// status surfaces before any work has been completed.
func EmptyProductivitySnapshot() ProductivitySnapshot {
	return finalizeProductivitySnapshot(ProductivitySnapshot{
		GeneratedAt:   time.Now(),
		NoopThreshold: defaultProductivityNoopThreshold,
	})
}

// ProductivitySnapshot returns a composite productivity view for the selected
// repo. When repoPath is empty, loop-level data is aggregated across all repos.
func (m *Manager) ProductivitySnapshot(repoPath string, since time.Time) ProductivitySnapshot {
	return m.productivitySnapshot(repoPath, since, nil)
}

func (m *Manager) productivitySnapshot(repoPath string, since time.Time, rd *ResearchDaemon) ProductivitySnapshot {
	loops := m.ListLoops()
	cycles := make([]*CycleRun, 0)
	if strings.TrimSpace(repoPath) != "" {
		if loaded, err := ListCycles(repoPath); err == nil {
			cycles = loaded
		}
	}

	var rdStats *ResearchDaemonStats
	if rd != nil {
		stats := rd.Stats()
		rdStats = &stats
	}

	return buildProductivitySnapshot(repoPath, since, rdStats, loops, cycles)
}

func buildProductivitySnapshot(repoPath string, since time.Time, rdStats *ResearchDaemonStats, loops []*LoopRun, cycles []*CycleRun) ProductivitySnapshot {
	snapshot := ProductivitySnapshot{
		GeneratedAt:   time.Now(),
		NoopThreshold: defaultProductivityNoopThreshold,
	}

	if rdStats != nil {
		snapshot.ResearchOutputs = rdStats.ResearchOutputs
		snapshot.TopicsCompleted = rdStats.TopicsCompleted
		snapshot.DedupSkips = rdStats.DedupSkips
		snapshot.AutonomyRejections = rdStats.AutonomyRejections
	}

	for _, run := range loops {
		accumulateLoopProductivity(&snapshot, run, repoPath, since)
	}
	for _, cycle := range cycles {
		accumulateCycleProductivity(&snapshot, cycle, since)
	}

	return finalizeProductivitySnapshot(snapshot)
}

func accumulateLoopProductivity(snapshot *ProductivitySnapshot, run *LoopRun, repoPath string, since time.Time) {
	if run == nil {
		return
	}

	run.mu.Lock()
	defer run.mu.Unlock()

	if repoPath != "" && run.RepoPath != repoPath {
		return
	}
	if !since.IsZero() && run.CreatedAt.Before(since) && run.UpdatedAt.Before(since) {
		return
	}

	threshold := run.Profile.NoopPlateauLimit
	if threshold <= 0 {
		threshold = defaultProductivityNoopThreshold
	}
	if snapshot.NoopThreshold <= 0 || threshold < snapshot.NoopThreshold {
		snapshot.NoopThreshold = threshold
	}

	trailingNoops := 0
	for i := len(run.Iterations) - 1; i >= 0; i-- {
		if isNoopIteration(run.Iterations[i]) {
			trailingNoops++
			continue
		}
		break
	}
	if trailingNoops > snapshot.ConsecutiveNoops {
		snapshot.ConsecutiveNoops = trailingNoops
	}
	if run.Status == "converged" && strings.Contains(run.LastError, "no-op plateau") {
		snapshot.NoopPlateau = true
	}

	for _, iter := range run.Iterations {
		if !since.IsZero() && iter.StartedAt.Before(since) {
			continue
		}
		snapshot.VerificationFailures += verificationFailuresForIteration(iter)
		if iterationProducedDevelopmentOutput(iter) {
			snapshot.DevelopmentOutputs++
		}
	}
}

func accumulateCycleProductivity(snapshot *ProductivitySnapshot, cycle *CycleRun, since time.Time) {
	if cycle == nil {
		return
	}
	if !since.IsZero() && cycle.CreatedAt.Before(since) && cycle.UpdatedAt.Before(since) {
		return
	}

	if cycle.Synthesis != nil && len(cycle.Synthesis.Accomplished) > 0 {
		snapshot.DevelopmentOutputs += len(cycle.Synthesis.Accomplished)
		return
	}

	for _, task := range cycle.Tasks {
		if task.Status == "done" {
			snapshot.DevelopmentOutputs++
		}
	}
}

func verificationFailuresForIteration(iter LoopIteration) int {
	failures := 0
	for _, check := range iter.Verification {
		if check.Status == "failed" || check.ExitCode != 0 {
			failures++
		}
	}
	return failures
}

func iterationProducedDevelopmentOutput(iter LoopIteration) bool {
	if isNoopIteration(iter) {
		return false
	}
	if iter.Acceptance != nil && (iter.Acceptance.AutoMerged || iter.Acceptance.PRCreated) {
		return true
	}
	if iter.AcceptanceReason == "auto_merged" || iter.AcceptanceReason == "pr_created" {
		return true
	}
	if iter.StagedFilesCount > 0 {
		return true
	}
	if iter.Acceptance != nil && (len(iter.Acceptance.SafePaths) > 0 || len(iter.Acceptance.ReviewPaths) > 0) {
		return true
	}
	return false
}

func isNoopIteration(iter LoopIteration) bool {
	return iter.AcceptanceReason == "no_staged_files" || iter.AcceptanceReason == "worker_no_changes"
}

func finalizeProductivitySnapshot(snapshot ProductivitySnapshot) ProductivitySnapshot {
	if snapshot.GeneratedAt.IsZero() {
		snapshot.GeneratedAt = time.Now()
	}
	if snapshot.NoopThreshold <= 0 {
		snapshot.NoopThreshold = defaultProductivityNoopThreshold
	}

	reasons := make([]string, 0, 4)
	score := 0

	if snapshot.ResearchOutputs > 0 && snapshot.TopicsCompleted > 0 {
		score += 35
	} else {
		reasons = append(reasons, "no durable research output")
	}

	if snapshot.DevelopmentOutputs > 0 {
		score += 35
	} else {
		reasons = append(reasons, "no concrete development progress")
	}

	if snapshot.VerificationFailures == 0 {
		score += 20
	} else {
		reasons = append(reasons, "verification failures present")
	}

	if !snapshot.NoopPlateau && snapshot.ConsecutiveNoops < snapshot.NoopThreshold {
		score += 10
	} else {
		reasons = append(reasons, "loop stalled in no-op churn")
	}

	snapshot.Score = score
	snapshot.Productive = score >= 80 &&
		snapshot.ResearchOutputs > 0 &&
		snapshot.TopicsCompleted > 0 &&
		snapshot.DevelopmentOutputs > 0 &&
		snapshot.VerificationFailures == 0 &&
		!snapshot.NoopPlateau
	if !snapshot.Productive && len(reasons) == 0 {
		reasons = append(reasons, "below productivity threshold")
	}
	snapshot.Reasons = reasons
	return snapshot
}
