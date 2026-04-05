package session

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/bandit"
)

// BanditState is the serializable state of the BanditRouter for persistence.
type BanditState struct {
	ArmStats       map[string]bandit.ArmStat `json:"arm_stats"`
	TotalPulls     int                       `json:"total_pulls"`
	SuccessWindows map[string][]bool         `json:"success_windows"`
	SavedAt        time.Time                 `json:"saved_at"`
}

// SaveBanditState persists the bandit router state to a JSON file.
// This enables the learned routing model to survive process restarts.
func (br *BanditRouter) SaveBanditState(dir string) error {
	if br == nil {
		return nil
	}

	br.mu.Lock()
	state := BanditState{
		ArmStats:       br.policy.ArmStats(),
		TotalPulls:     br.totalPulls,
		SuccessWindows: make(map[string][]bool),
		SavedAt:        time.Now(),
	}
	for provider, sw := range br.successTracker {
		state.SuccessWindows[provider] = append([]bool(nil), sw.outcomes...)
	}
	br.mu.Unlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bandit state: %w", err)
	}

	path := filepath.Join(dir, "bandit_state.json")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create bandit state dir: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write bandit state: %w", err)
	}

	slog.Debug("bandit: state saved", "path", path, "pulls", state.TotalPulls)
	return nil
}

// LoadBanditState restores bandit router state from a JSON file.
// Returns nil if the file doesn't exist (fresh start).
func LoadBanditState(dir string) (*BanditState, error) {
	path := filepath.Join(dir, "bandit_state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // fresh start
		}
		return nil, fmt.Errorf("read bandit state: %w", err)
	}

	var state BanditState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal bandit state: %w", err)
	}

	return &state, nil
}

// RestoreFromState hydrates the bandit router from previously saved state.
// This replays the success windows and sets the total pull count so the
// bandit resumes where it left off rather than starting from scratch.
func (br *BanditRouter) RestoreFromState(state *BanditState) {
	if br == nil || state == nil {
		return
	}

	br.mu.Lock()
	defer br.mu.Unlock()

	br.totalPulls = state.TotalPulls

	for provider, outcomes := range state.SuccessWindows {
		if sw, ok := br.successTracker[provider]; ok {
			sw.outcomes = append([]bool(nil), outcomes...)
			if len(sw.outcomes) > sw.maxSize {
				sw.outcomes = sw.outcomes[len(sw.outcomes)-sw.maxSize:]
			}
		}
	}

	slog.Info("bandit: state restored",
		"pulls", state.TotalPulls,
		"saved_at", state.SavedAt,
		"providers", len(state.SuccessWindows),
	)
}

// ObservationToBanditReward converts a loop observation into a bandit reward
// signal. This bridges the observation pipeline to the bandit training loop.
func ObservationToBanditReward(obs LoopObservation) (provider, model string, success bool, cost, quality float64, ctx CascadeContext) {
	provider = obs.WorkerProvider
	if provider == "" {
		provider = string(DefaultPrimaryProvider())
	}
	model = obs.WorkerModelUsed

	// Success = verification passed
	success = obs.VerifyPassed

	// Cost from observation
	cost = obs.TotalCostUSD

	// Quality score: blend of verification pass rate and worker efficiency
	quality = 0.0
	if obs.VerifyPassed {
		quality = 0.7
		// Bonus for efficient work (lower latency = higher quality)
		if obs.WorkerLatencyMs > 0 && obs.WorkerLatencyMs <= 60000 {
			quality += 0.3
		} else if obs.WorkerLatencyMs <= 120000 {
			quality += 0.15
		}
	}

	// Build context
	ctx = CascadeContext{
		TaskType:        obs.TaskType,
		TimeSensitivity: TimeNormal,
	}

	// Estimate complexity from difficulty score if available
	if obs.DifficultyScore > 0.7 {
		ctx.Complexity = TaskComplex
	} else if obs.DifficultyScore > 0.3 {
		ctx.Complexity = TaskMedium
	} else {
		ctx.Complexity = TaskSimple
	}

	return
}
