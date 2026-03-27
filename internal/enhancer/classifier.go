package enhancer

import "strings"

// TaskType represents the type of task a prompt is requesting
type TaskType string

const (
	TaskTypeCode            TaskType = "code"
	TaskTypeCreative        TaskType = "creative"
	TaskTypeAnalysis        TaskType = "analysis"
	TaskTypeTroubleshooting TaskType = "troubleshooting"
	TaskTypeWorkflow        TaskType = "workflow"
	TaskTypeGeneral         TaskType = "general"
)

// taskTypePatterns maps keyword patterns to task types
var taskTypePatterns = map[TaskType][]string{
	TaskTypeTroubleshooting: {
		"debug", "error", "fix", "broken", "not working", "crash", "fail",
		"issue", "problem", "troubleshoot", "diagnose", "wrong", "stuck",
		"bug", "exception", "timeout", "disconnect",
	},
	TaskTypeCode: {
		"create", "build", "implement", "code", "write", "function",
		"class", "api", "endpoint", "refactor", "add feature", "develop",
		"program", "script", "module", "package", "library",
	},
	TaskTypeAnalysis: {
		"review", "analyze", "compare", "evaluate", "assess", "audit",
		"inspect", "examine", "investigate", "explain", "understand",
		"performance", "benchmark", "profile", "measure",
	},
	TaskTypeCreative: {
		"design", "visual", "music", "audio", "video", "art",
		"creative", "mood", "aesthetic", "style", "theme", "vibe",
		"show", "performance", "set", "mix", "lighting",
	},
	TaskTypeWorkflow: {
		"workflow", "automate", "sequence", "pipeline", "chain",
		"schedule", "orchestrate", "batch", "process", "routine",
		"startup", "shutdown", "backup",
	},
}

// taskTypePhrases are multi-word patterns worth 2 points each (higher signal).
var taskTypePhrases = map[TaskType][]string{
	TaskTypeCode: {
		"dependency injection", "design pattern", "use interfaces",
		"extract method", "pull request", "unit test", "test coverage",
		"type safety", "error handling", "add support",
	},
	TaskTypeWorkflow: {
		"ci/cd", "github actions", "cron job", "build pipeline",
	},
	TaskTypeAnalysis: {
		"root cause", "code review", "security audit",
	},
}

// classifyPriority defines deterministic tie-breaking order.
var classifyPriority = []TaskType{
	TaskTypeCode, TaskTypeAnalysis, TaskTypeWorkflow,
	TaskTypeTroubleshooting, TaskTypeCreative,
}

// Classify determines the most likely task type for a prompt
func Classify(prompt string) TaskType {
	lower := strings.ToLower(prompt)
	scores := make(map[TaskType]int)

	for taskType, patterns := range taskTypePatterns {
		for _, pattern := range patterns {
			if strings.Contains(lower, pattern) {
				scores[taskType]++
			}
		}
	}

	// Phrase-level patterns worth 2 points each
	for taskType, phrases := range taskTypePhrases {
		for _, phrase := range phrases {
			if strings.Contains(lower, phrase) {
				scores[taskType] += 2
			}
		}
	}

	// Deterministic tie-breaking via priority order
	bestType := TaskTypeGeneral
	bestScore := 0
	for _, taskType := range classifyPriority {
		if score := scores[taskType]; score > bestScore {
			bestScore = score
			bestType = taskType
		}
	}

	return bestType
}

// ClassifyScore holds a task type with its raw match score and normalized confidence.
type ClassifyScore struct {
	TaskType   TaskType `json:"task_type"`
	RawScore   int      `json:"raw_score"`
	Confidence float64  `json:"confidence"`
}

// ClassifyDetailed returns the best task type along with confidence and runner-up alternatives.
// Confidence is derived by normalizing the raw keyword match scores.
func ClassifyDetailed(prompt string) (best ClassifyScore, alternatives []ClassifyScore) {
	lower := strings.ToLower(prompt)
	scores := make(map[TaskType]int)

	for taskType, patterns := range taskTypePatterns {
		for _, pattern := range patterns {
			if strings.Contains(lower, pattern) {
				scores[taskType]++
			}
		}
	}

	for taskType, phrases := range taskTypePhrases {
		for _, phrase := range phrases {
			if strings.Contains(lower, phrase) {
				scores[taskType] += 2
			}
		}
	}

	// Total score across all types for normalization
	totalScore := 0
	for _, s := range scores {
		totalScore += s
	}

	// Deterministic tie-breaking via priority order
	bestType := TaskTypeGeneral
	bestRawScore := 0
	for _, taskType := range classifyPriority {
		if score := scores[taskType]; score > bestRawScore {
			bestRawScore = score
			bestType = taskType
		}
	}

	// Compute confidence: if no patterns matched, assign low default confidence to "general"
	if totalScore == 0 {
		best = ClassifyScore{TaskType: TaskTypeGeneral, RawScore: 0, Confidence: 0.3}
		return best, nil
	}

	best = ClassifyScore{
		TaskType:   bestType,
		RawScore:   bestRawScore,
		Confidence: float64(bestRawScore) / float64(totalScore),
	}
	// Ensure minimum confidence of 0.3 for the winner
	if best.Confidence < 0.3 {
		best.Confidence = 0.3
	}

	// Collect runner-ups sorted by priority order (deterministic)
	for _, taskType := range classifyPriority {
		if taskType == bestType {
			continue
		}
		if s := scores[taskType]; s > 0 {
			conf := float64(s) / float64(totalScore)
			alternatives = append(alternatives, ClassifyScore{
				TaskType:   taskType,
				RawScore:   s,
				Confidence: conf,
			})
		}
	}

	return best, alternatives
}

// ValidTaskType checks if a string is a valid task type
func ValidTaskType(s string) TaskType {
	switch TaskType(strings.ToLower(s)) {
	case TaskTypeCode, TaskTypeCreative, TaskTypeAnalysis,
		TaskTypeTroubleshooting, TaskTypeWorkflow, TaskTypeGeneral:
		return TaskType(strings.ToLower(s))
	default:
		return ""
	}
}
