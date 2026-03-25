package bandit

import "time"

// Arm represents a selectable action (provider + model combination).
type Arm struct {
	ID       string `json:"id"`
	Provider string `json:"provider"` // "claude", "gemini", "codex"
	Model    string `json:"model"`
}

// Reward records the outcome of pulling an arm.
type Reward struct {
	ArmID     string    `json:"arm_id"`
	Value     float64   `json:"value"`               // 0.0-1.0 composite reward
	Timestamp time.Time `json:"timestamp"`
	Context   []float64 `json:"context,omitempty"`    // feature vector for contextual bandits
}

// ArmStat holds summary statistics for an arm.
type ArmStat struct {
	Pulls      int     `json:"pulls"`
	MeanReward float64 `json:"mean_reward"`
	Alpha      float64 `json:"alpha"` // Beta dist param
	Beta       float64 `json:"beta"`  // Beta dist param
}

// Policy selects arms and updates from rewards.
type Policy interface {
	// Select chooses an arm. ctx is an optional feature vector (nil for non-contextual).
	Select(ctx []float64) Arm
	// Update records a reward for an arm.
	Update(reward Reward)
	// ArmStats returns summary statistics per arm.
	ArmStats() map[string]ArmStat
}
