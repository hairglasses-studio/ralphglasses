// Package chains provides workflow chain functionality.
package chains

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hairglasses-studio/webb/internal/patterns"
	"gopkg.in/yaml.v3"
)

// PatternConverter converts discovered patterns to workflow chains
type PatternConverter struct {
	registry *Registry
	chainsDir string
}

// NewPatternConverter creates a new pattern converter
func NewPatternConverter(registry *Registry, chainsDir string) *PatternConverter {
	if chainsDir == "" {
		home, _ := os.UserHomeDir()
		chainsDir = filepath.Join(home, ".config", "webb", "chains")
	}
	return &PatternConverter{
		registry:  registry,
		chainsDir: chainsDir,
	}
}

// ConvertResult represents the result of a pattern conversion
type ConvertResult struct {
	Chain      *ChainDefinition `json:"chain"`
	ChainName  string           `json:"chain_name"`
	SavedPath  string           `json:"saved_path,omitempty"`
	Registered bool             `json:"registered"`
}

// ConvertPattern converts a discovered pattern to a chain definition
func (c *PatternConverter) ConvertPattern(pattern patterns.DiscoveredPattern, customName string) *ChainDefinition {
	// Generate chain name
	chainName := customName
	if chainName == "" {
		chainName = c.generateChainName(pattern)
	}

	// Build steps from tool sequence
	steps := make([]ChainStep, len(pattern.ToolSequence))
	for i, toolName := range pattern.ToolSequence {
		steps[i] = ChainStep{
			ID:      fmt.Sprintf("step_%d", i+1),
			Type:    StepTypeTool,
			Name:    c.humanizeName(toolName),
			Tool:    toolName,
			StoreAs: fmt.Sprintf("result_%d", i+1),
		}
	}

	// Determine category based on tools
	category := c.inferCategory(pattern.ToolSequence)

	// Calculate timeout based on average execution time
	timeout := c.calculateTimeout(pattern)

	// Build description
	description := fmt.Sprintf("Auto-generated from pattern observed %d times (%.1f%% success rate)",
		pattern.Frequency, pattern.SuccessRate*100)
	if pattern.OptimizationHint != "" {
		description += ". " + pattern.OptimizationHint
	}

	return &ChainDefinition{
		Name:        chainName,
		Description: description,
		Category:    category,
		Trigger: ChainTrigger{
			Type: TriggerManual,
		},
		Steps:   steps,
		Timeout: timeout,
		Tags:    []string{"auto-generated", "pattern-mined", fmt.Sprintf("frequency:%d", pattern.Frequency)},
	}
}

// RegisterChain converts a pattern and registers it in the registry
func (c *PatternConverter) RegisterChain(pattern patterns.DiscoveredPattern, customName string) (*ConvertResult, error) {
	chain := c.ConvertPattern(pattern, customName)

	if err := c.registry.Register(chain); err != nil {
		return nil, fmt.Errorf("failed to register chain: %w", err)
	}

	return &ConvertResult{
		Chain:      chain,
		ChainName:  chain.Name,
		Registered: true,
	}, nil
}

// SaveAsYAML converts a pattern and saves it to a YAML file
func (c *PatternConverter) SaveAsYAML(pattern patterns.DiscoveredPattern, customName string) (*ConvertResult, error) {
	chain := c.ConvertPattern(pattern, customName)

	// Ensure directory exists
	if err := os.MkdirAll(c.chainsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create chains directory: %w", err)
	}

	// Generate filename
	filename := fmt.Sprintf("%s.yaml", chain.Name)
	filepath := filepath.Join(c.chainsDir, filename)

	// Marshal to YAML
	data, err := yaml.Marshal(chain)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chain: %w", err)
	}

	// Add header comment
	header := fmt.Sprintf("# Auto-generated chain from discovered pattern\n# Pattern ID: %s\n# Generated: %s\n# Frequency: %d\n# Success Rate: %.1f%%\n\n",
		pattern.ID, time.Now().Format(time.RFC3339), pattern.Frequency, pattern.SuccessRate*100)
	data = append([]byte(header), data...)

	// Write file
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write chain file: %w", err)
	}

	return &ConvertResult{
		Chain:     chain,
		ChainName: chain.Name,
		SavedPath: filepath,
	}, nil
}

// SaveAndRegister both saves and registers the chain
func (c *PatternConverter) SaveAndRegister(pattern patterns.DiscoveredPattern, customName string) (*ConvertResult, error) {
	result, err := c.SaveAsYAML(pattern, customName)
	if err != nil {
		return nil, err
	}

	if err := c.registry.Register(result.Chain); err != nil {
		// File was saved but registration failed
		return result, fmt.Errorf("chain saved to %s but failed to register: %w", result.SavedPath, err)
	}

	result.Registered = true
	return result, nil
}

// generateChainName creates a readable chain name from pattern
func (c *PatternConverter) generateChainName(pattern patterns.DiscoveredPattern) string {
	if pattern.Name != "" && pattern.Name != "Unknown" {
		// Clean up the pattern name
		name := strings.ToLower(pattern.Name)
		name = strings.ReplaceAll(name, " ", "-")
		name = strings.ReplaceAll(name, "_", "-")
		return fmt.Sprintf("pattern-%s", name)
	}

	// Generate from first and last tool
	if len(pattern.ToolSequence) == 0 {
		return fmt.Sprintf("pattern-%s", pattern.ID)
	}

	first := c.simplifyToolName(pattern.ToolSequence[0])
	if len(pattern.ToolSequence) == 1 {
		return fmt.Sprintf("pattern-%s", first)
	}

	last := c.simplifyToolName(pattern.ToolSequence[len(pattern.ToolSequence)-1])
	return fmt.Sprintf("pattern-%s-to-%s", first, last)
}

// simplifyToolName removes the webb_ prefix and simplifies
func (c *PatternConverter) simplifyToolName(name string) string {
	name = strings.TrimPrefix(name, "webb_")
	parts := strings.Split(name, "_")
	if len(parts) >= 2 {
		return parts[0] + "-" + parts[1]
	}
	return name
}

// humanizeName creates a human-readable name from a tool name
func (c *PatternConverter) humanizeName(name string) string {
	name = strings.TrimPrefix(name, "webb_")
	name = strings.ReplaceAll(name, "_", " ")
	return strings.Title(name)
}

// inferCategory determines the chain category from tools
func (c *PatternConverter) inferCategory(tools []string) ChainCategory {
	categories := map[string]int{
		"k8s":        0,
		"kubernetes": 0,
		"cluster":    0,
		"pod":        0,
		"deploy":     0,
		"helm":       0,
		"pylon":      0,
		"incident":   0,
		"ticket":     0,
		"shortcut":   0,
		"customer":   0,
		"investigate": 0,
		"debug":      0,
		"remediate":  0,
		"fix":        0,
		"standup":    0,
		"morning":    0,
	}

	for _, tool := range tools {
		toolLower := strings.ToLower(tool)
		for key := range categories {
			if strings.Contains(toolLower, key) {
				categories[key]++
			}
		}
	}

	// Determine dominant category
	if categories["remediate"]+categories["fix"] > 0 {
		return CategoryRemediation
	}
	if categories["pylon"]+categories["ticket"]+categories["shortcut"]+categories["incident"] >= 2 {
		return CategoryCustomer
	}
	if categories["investigate"]+categories["debug"] >= 2 {
		return CategoryInvestigative
	}
	if categories["k8s"]+categories["kubernetes"]+categories["cluster"]+categories["pod"]+categories["deploy"]+categories["helm"] >= 2 {
		return CategoryOperational
	}

	// Default
	return CategoryInvestigative
}

// calculateTimeout estimates a reasonable timeout for the chain
func (c *PatternConverter) calculateTimeout(pattern patterns.DiscoveredPattern) string {
	// Base: 30 seconds per tool, minimum 1 minute
	baseSeconds := len(pattern.ToolSequence) * 30
	if baseSeconds < 60 {
		baseSeconds = 60
	}

	// Add buffer for complex patterns
	if len(pattern.ToolSequence) > 4 {
		baseSeconds = int(float64(baseSeconds) * 1.5)
	}

	// Round to nearest minute
	minutes := (baseSeconds + 30) / 60

	return fmt.Sprintf("%dm", minutes)
}

// ListConvertiblePatterns returns patterns suitable for chain conversion
func (c *PatternConverter) ListConvertiblePatterns(discoveredPatterns []patterns.DiscoveredPattern, minFrequency int) []patterns.DiscoveredPattern {
	if minFrequency <= 0 {
		minFrequency = 3
	}

	var convertible []patterns.DiscoveredPattern
	for _, p := range discoveredPatterns {
		// Only sequential patterns with sufficient frequency
		if p.Type == patterns.PatternSequential && p.Frequency >= minFrequency && len(p.ToolSequence) >= 2 {
			convertible = append(convertible, p)
		}
	}
	return convertible
}

// FormatConversionSuggestions formats patterns as suggestions for conversion
func FormatConversionSuggestions(discoveredPatterns []patterns.DiscoveredPattern) string {
	if len(discoveredPatterns) == 0 {
		return "No patterns available for conversion.\n"
	}

	var sb strings.Builder
	sb.WriteString("# Patterns Available for Chain Conversion\n\n")
	sb.WriteString("| Pattern | Tools | Frequency | Success Rate | Suggested Chain Name |\n")
	sb.WriteString("|---------|-------|-----------|--------------|---------------------|\n")

	converter := NewPatternConverter(nil, "")
	for _, p := range discoveredPatterns {
		if len(p.ToolSequence) < 2 {
			continue
		}
		chainName := converter.generateChainName(p)
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %.0f%% | `%s` |\n",
			p.Name, len(p.ToolSequence), p.Frequency, p.SuccessRate*100, chainName))
	}

	sb.WriteString("\n## Usage\n\n")
	sb.WriteString("```\n")
	sb.WriteString("# Convert a pattern to a chain:\n")
	sb.WriteString("webb_pattern_to_chain(pattern_id=\"<id>\", save=true)\n")
	sb.WriteString("```\n")

	return sb.String()
}
