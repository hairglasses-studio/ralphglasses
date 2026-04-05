package promptdj

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/blackboard"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

const bbNamespace = "prompt_quality"

// BridgeThresholds configures when event-driven signals are published.
type BridgeThresholds struct {
	DiscoveryScore  int // score >= this triggers immediate discovery signal (default 85)
	RegressionDelta int // score drop >= this triggers regression signal (default 20)
}

type scoreSample struct {
	Score, Grade, TaskType, Repo, SessionID, Provider string
	ScoreNum                                          int
	Timestamp                                         time.Time
}

// PromptBridge syncs prompt quality signals to the blackboard.
type PromptBridge struct {
	bb           *blackboard.Blackboard
	writerID     string
	syncInterval time.Duration
	thresholds   BridgeThresholds
	mu           sync.Mutex
	recentScores []scoreSample
	maxSamples   int
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewPromptBridge creates a bridge publishing prompt quality data to the blackboard.
func NewPromptBridge(bb *blackboard.Blackboard, writerID string) *PromptBridge {
	return &PromptBridge{
		bb: bb, writerID: writerID,
		syncInterval: 60 * time.Second, maxSamples: 1000,
		thresholds: BridgeThresholds{DiscoveryScore: 85, RegressionDelta: 20},
		stopCh:     make(chan struct{}), doneCh: make(chan struct{}),
	}
}

// RecordScore records a prompt quality observation.
func (pb *PromptBridge) RecordScore(result enhancer.EnhanceResult, report *enhancer.ScoreReport,
	repo, sessionID, provider string) {
	score := 0
	grade := ""
	if report != nil {
		score = report.Overall
		grade = report.Grade
	}
	s := scoreSample{
		ScoreNum: score, Grade: grade, TaskType: string(result.TaskType),
		Repo: repo, SessionID: sessionID, Provider: provider, Timestamp: time.Now(),
	}
	pb.mu.Lock()
	pb.recentScores = append(pb.recentScores, s)
	if len(pb.recentScores) > pb.maxSamples {
		pb.recentScores = pb.recentScores[len(pb.recentScores)-pb.maxSamples:]
	}
	pb.mu.Unlock()
	if score >= pb.thresholds.DiscoveryScore {
		pb.publishDiscovery(s)
	}
}

// RecordOutcome records the outcome of a routing decision.
func (pb *PromptBridge) RecordOutcome(decisionID string, success bool, costUSD, durationSec float64) {
	if pb.bb == nil {
		return
	}
	_ = pb.bb.Put(blackboard.Entry{
		Key: fmt.Sprintf("outcome/%s", decisionID), Namespace: bbNamespace,
		Value: map[string]any{
			"decision_id": decisionID, "success": success,
			"cost_usd": costUSD, "duration_sec": durationSec,
			"recorded_at": time.Now().UTC().Format(time.RFC3339),
		},
		WriterID: pb.writerID, TTL: 30 * time.Minute,
	})
}

// Start launches the background aggregation loop.
func (pb *PromptBridge) Start() { go pb.loop() }

// Stop signals the loop to stop and waits.
func (pb *PromptBridge) Stop() { close(pb.stopCh); <-pb.doneCh }

func (pb *PromptBridge) loop() {
	defer close(pb.doneCh)
	t := time.NewTicker(pb.syncInterval)
	defer t.Stop()
	for {
		select {
		case <-pb.stopCh:
			return
		case <-t.C:
			pb.publishFleetStats()
		}
	}
}

func (pb *PromptBridge) publishFleetStats() {
	pb.mu.Lock()
	samples := make([]scoreSample, len(pb.recentScores))
	copy(samples, pb.recentScores)
	pb.mu.Unlock()
	if len(samples) == 0 || pb.bb == nil {
		return
	}
	var total int
	byRepo := map[string]int{}
	byType := map[string]int{}
	byGrade := map[string]int{}
	for _, s := range samples {
		total += s.ScoreNum
		byRepo[s.Repo]++
		byType[s.TaskType]++
		byGrade[s.Grade]++
	}
	if err := pb.bb.Put(blackboard.Entry{
		Key: "stats/fleet/summary", Namespace: bbNamespace,
		Value: map[string]any{
			"sample_count": len(samples), "average_score": float64(total) / float64(len(samples)),
			"by_repo": byRepo, "by_task_type": byType, "by_grade": byGrade,
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		},
		WriterID: pb.writerID, TTL: 15 * time.Minute,
	}); err != nil {
		slog.Warn("prompt bridge: failed to publish fleet stats", "error", err)
	}
}

func (pb *PromptBridge) publishDiscovery(s scoreSample) {
	if pb.bb == nil {
		return
	}
	_ = pb.bb.Put(blackboard.Entry{
		Key: fmt.Sprintf("signal/%s/discovery", s.SessionID), Namespace: bbNamespace,
		Value: map[string]any{
			"score": s.ScoreNum, "grade": s.Grade, "task_type": s.TaskType,
			"repo": s.Repo, "session_id": s.SessionID, "provider": s.Provider,
			"discovered": time.Now().UTC().Format(time.RFC3339),
		},
		WriterID: pb.writerID, TTL: 10 * time.Minute,
	})
}
