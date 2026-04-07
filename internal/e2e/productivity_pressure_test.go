package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/testutil/productivitytest"
)

func TestProductivePressureFullStack(t *testing.T) {
	fix := productivitytest.NewFixture(t)
	mgr := fix.NewManager()

	var mu sync.Mutex
	workerWrites := 0
	plannerTasks := 0

	mgr.SetHooksForTesting(
		func(_ context.Context, opts session.LaunchOptions) (*session.Session, error) {
			sess := &session.Session{
				ID:         sanitizeID(opts.SessionName, time.Now().UnixNano()),
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   filepath.Base(opts.RepoPath),
				Prompt:     opts.Prompt,
				Model:      opts.Model,
				Status:     session.StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}

			if strings.HasPrefix(opts.SessionName, "loop-plan-") {
				mu.Lock()
				plannerTasks++
				taskNum := plannerTasks
				mu.Unlock()
				sess.LastOutput = fmt.Sprintf(`{"title":"Pressure task %d","prompt":"Write productive pressure artifact %d."}`, taskNum, taskNum)
				sess.OutputHistory = []string{sess.LastOutput}
				return sess, nil
			}

			mu.Lock()
			workerWrites++
			writeNum := workerWrites
			mu.Unlock()

			target := filepath.Join(opts.RepoPath, "README.md")
			f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				return nil, err
			}
			if _, err := fmt.Fprintf(f, "\npressure iteration %d\n", writeNum); err != nil {
				_ = f.Close()
				return nil, err
			}
			if err := f.Close(); err != nil {
				return nil, err
			}
			sess.LastOutput = "worker complete"
			sess.OutputHistory = []string{"worker complete"}
			return sess, nil
		},
		func(_ context.Context, sess *session.Session) error {
			sess.Lock()
			sess.Status = session.StatusCompleted
			now := time.Now()
			sess.EndedAt = &now
			sess.Unlock()
			return nil
		},
	)

	profile := session.SelfImprovementProfile()
	profile.VerifyCommands = []string{"true"}
	profile.MaxIterations = 2
	profile.MaxDurationSecs = 300
	profile.NoopPlateauLimit = 3

	run, err := mgr.StartLoop(context.Background(), fix.RepoPath, profile)
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}
	if err := mgr.StepLoop(context.Background(), run.ID); err != nil {
		t.Fatalf("StepLoop #1: %v", err)
	}
	if err := mgr.StepLoop(context.Background(), run.ID); err != nil {
		t.Fatalf("StepLoop #2: %v", err)
	}

	gw := productivitytest.NewGateway(
		&session.ResearchEntry{Topic: "pressure-topic-1", Domain: "mcp", Source: "manual", PriorityScore: 0.7, ModelTier: "sonnet", BudgetUSD: 1.0},
		&session.ResearchEntry{Topic: "pressure-topic-2", Domain: "mcp", Source: "manual", PriorityScore: 0.7, ModelTier: "sonnet", BudgetUSD: 1.0},
		&session.ResearchEntry{Topic: "pressure-topic-3", Domain: "mcp", Source: "manual", PriorityScore: 0.7, ModelTier: "sonnet", BudgetUSD: 1.0},
	)
	gw.SetDedup(0.5, "proceed")
	rd := session.NewResearchDaemon(gw, session.ResearchDaemonConfig{
		Enabled:         true,
		TickInterval:    1,
		MaxTopicsPerRun: 3,
		BudgetPerRunUSD: 10,
		BudgetDailyUSD:  25,
		MaxComplexity:   3,
		ClaimTTLSecs:    60,
		AgentID:         "pressure-test-daemon",
	})
	rd.Tick(context.Background())

	sup := session.NewSupervisor(mgr, fix.RepoPath)
	sup.SetResearchDaemon(rd)
	status := sup.Status()

	if !status.Productivity.Productive {
		t.Fatalf("expected productive full-stack pressure status, got %+v", status.Productivity)
	}
	if status.Productivity.Score < 80 {
		t.Fatalf("score = %d, want >= 80", status.Productivity.Score)
	}
	if status.Productivity.ResearchOutputs < 1 || status.Productivity.TopicsCompleted < 1 {
		t.Fatalf("expected durable research output, got %+v", status.Productivity)
	}
	if status.Productivity.DevelopmentOutputs < 2 {
		t.Fatalf("development_outputs = %d, want at least 2", status.Productivity.DevelopmentOutputs)
	}
	if status.Productivity.VerificationFailures != 0 {
		t.Fatalf("verification_failures = %d, want 0", status.Productivity.VerificationFailures)
	}
	if status.Productivity.NoopPlateau {
		t.Fatalf("expected no noop plateau, got %+v", status.Productivity)
	}
}

func sanitizeID(prefix string, n int64) string {
	prefix = strings.ReplaceAll(prefix, " ", "-")
	prefix = strings.ReplaceAll(prefix, "/", "-")
	return fmt.Sprintf("%s-%d", prefix, n)
}
