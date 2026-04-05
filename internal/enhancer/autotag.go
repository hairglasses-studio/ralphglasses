package enhancer

import (
	"sort"
	"strings"
)

// DomainTag represents a semantic domain classification.
type DomainTag string

const (
	DomainGo         DomainTag = "go"
	DomainMCP        DomainTag = "mcp"
	DomainShader     DomainTag = "shader"
	DomainTerminal   DomainTag = "terminal"
	DomainAgents     DomainTag = "agents"
	DomainRice       DomainTag = "rice"
	DomainTUI        DomainTag = "tui"
	DomainTesting    DomainTag = "testing"
	DomainSecurity   DomainTag = "security"
	DomainDeployment DomainTag = "deployment"
	DomainInfra      DomainTag = "infra"
	DomainCost       DomainTag = "cost"
	DomainOrch       DomainTag = "orchestration"
	DomainGeneral    DomainTag = "general"
)

// AllDomainTags is the canonical taxonomy of domain tags.
var AllDomainTags = []DomainTag{
	DomainGo, DomainMCP, DomainShader, DomainTerminal, DomainAgents,
	DomainRice, DomainTUI, DomainTesting, DomainSecurity, DomainDeployment,
	DomainInfra, DomainCost, DomainOrch, DomainGeneral,
}

// domainKeywords maps each domain to its trigger keywords.
var domainKeywords = map[DomainTag][]string{
	DomainGo: {
		"golang", "go build", "go test", "go.mod", "go func", "package main",
		"goroutine", "channel", "sync.mutex", "interface{}", "go vet",
		"go install", "go get", "go module", "go workspace",
	},
	DomainMCP: {
		"mcp", "tool handler", "toolmodule", "registry", "mcp server",
		"mcp tool", "mcp-go", "mcpkit", "tool definition", "tool call",
	},
	DomainShader: {
		"shader", "glsl", "spirv", "fragment shader", "vertex shader",
		"uniform", "sampler2d", "gl_fragcolor", "ghostty", "custom-shader",
	},
	DomainTerminal: {
		"terminal", "ghostty", "foot", "tmux", "shell", "zsh", "bash",
		"starship", "prompt", "ansi", "escape sequence", "pty",
	},
	DomainAgents: {
		"agent", "orchestrat", "fleet", "ralph", "multi-agent",
		"a2a", "dispatch", "session", "provider", "cascade",
	},
	DomainRice: {
		"rice", "hyprland", "eww", "waybar", "mako", "wofi", "sway",
		"wayland", "compositor", "wallpaper", "theme", "snazzy",
	},
	DomainTUI: {
		"tui", "bubble tea", "bubbletea", "tcell", "view model",
		"lipgloss", "bubbles", "charm", "terminal ui",
	},
	DomainTesting: {
		"test", "bench", "coverage", "assert", "mock", "stub",
		"integration test", "unit test", "race detector", "golden test",
	},
	DomainSecurity: {
		"security", "auth", "credential", "secret", "encrypt",
		"tls", "certificate", "oauth", "jwt", "rbac", "permission",
	},
	DomainDeployment: {
		"deploy", "ci", "cd", "pipeline", "docker", "k8s", "kubernetes",
		"github actions", "workflow", "release", "container",
	},
	DomainInfra: {
		"server", "unraid", "opnsense", "firewall", "terraform",
		"rclone", "systemd", "network", "dns", "nginx",
	},
	DomainCost: {
		"cost", "budget", "token", "pricing", "spend", "burn rate",
		"optimization", "savings", "expensive", "cheap",
	},
	DomainOrch: {
		"orchestrat", "coordinator", "worker", "fleet", "dispatch",
		"blackboard", "loop", "sweep", "batch", "parallel",
	},
}

// domainPhrases are multi-word phrases worth double weight.
var domainPhrases = map[DomainTag][]string{
	DomainGo:       {"go build", "go test", "go module", "go workspace", "go vet"},
	DomainMCP:      {"mcp server", "mcp tool", "tool handler", "tool definition"},
	DomainShader:   {"fragment shader", "vertex shader", "custom-shader"},
	DomainTerminal: {"escape sequence", "terminal emulator"},
	DomainAgents:   {"multi-agent", "agent fleet", "cascade routing"},
	DomainRice:     {"window manager", "status bar", "notification daemon"},
	DomainTUI:      {"bubble tea", "terminal ui"},
	DomainTesting:  {"integration test", "unit test", "race detector", "golden test"},
	DomainSecurity: {"access control", "rate limit", "api key"},
	DomainDeployment: {"github actions", "ci/cd pipeline"},
}

// AutoTagResult holds the output of domain auto-tagging.
type AutoTagResult struct {
	Tags       []string         `json:"tags"`
	Scores     map[string]int   `json:"scores"`     // domain -> score
	Confidence map[string]float64 `json:"confidence"` // domain -> 0-1
	TaskType   TaskType         `json:"task_type"`
}

// AutoTag classifies a prompt into domain tags using keyword matching.
// Returns tags sorted by score (highest first). Only includes tags with score > 0.
func AutoTag(prompt string) AutoTagResult {
	lower := strings.ToLower(prompt)
	scores := make(map[DomainTag]int)

	// Single keywords: 1 point each
	for domain, keywords := range domainKeywords {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				scores[domain]++
			}
		}
	}

	// Multi-word phrases: 2 points each
	for domain, phrases := range domainPhrases {
		for _, phrase := range phrases {
			if strings.Contains(lower, phrase) {
				scores[domain] += 2
			}
		}
	}

	// Compute total for confidence normalization
	var totalScore int
	for _, s := range scores {
		totalScore += s
	}

	// Build result
	result := AutoTagResult{
		Scores:     make(map[string]int),
		Confidence: make(map[string]float64),
		TaskType:   Classify(prompt),
	}

	type tagScore struct {
		tag   DomainTag
		score int
	}
	var ranked []tagScore
	for tag, score := range scores {
		if score > 0 {
			ranked = append(ranked, tagScore{tag, score})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	for _, ts := range ranked {
		result.Tags = append(result.Tags, string(ts.tag))
		result.Scores[string(ts.tag)] = ts.score
		conf := 0.3 // minimum confidence
		if totalScore > 0 {
			conf = float64(ts.score) / float64(totalScore)
			if conf < 0.3 {
				conf = 0.3
			}
		}
		result.Confidence[string(ts.tag)] = conf
	}

	if len(result.Tags) == 0 {
		result.Tags = []string{string(DomainGeneral)}
		result.Scores[string(DomainGeneral)] = 0
		result.Confidence[string(DomainGeneral)] = 0.3
	}

	return result
}

// AutoTagWithTaskType classifies both domain tags and task type for a prompt.
func AutoTagWithTaskType(prompt string) (tags []string, taskType TaskType) {
	r := AutoTag(prompt)
	return r.Tags, r.TaskType
}
