// Package clients provides API clients for webb.
// v32.0: SwarmReflectionPhase for automated tool enhancement before release
package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ReflectionPhase represents a mandatory reflection cycle before pushing to main
type ReflectionPhase struct {
	ID              string                  `json:"id"`
	SessionID       string                  `json:"session_id"`
	RoadmapVersion  string                  `json:"roadmap_version"` // e.g., "v32.0"
	Status          ReflectionStatus        `json:"status"`
	ToolUsage       []ToolUsageRecord       `json:"tool_usage"`
	PainPoints      []PainPoint             `json:"pain_points"`
	Enhancements    []ToolEnhancement       `json:"enhancements"`
	TokenAnalysis   *TokenEfficiencyAnalysis `json:"token_analysis,omitempty"`
	Approved        bool                    `json:"approved"`
	ApprovalNotes   string                  `json:"approval_notes,omitempty"`
	CreatedAt       time.Time               `json:"created_at"`
	CompletedAt     *time.Time              `json:"completed_at,omitempty"`
}

// ReflectionStatus tracks the phase status
type ReflectionStatus string

const (
	ReflectionStatusPending    ReflectionStatus = "pending"
	ReflectionStatusAnalyzing  ReflectionStatus = "analyzing"
	ReflectionStatusEnhancing  ReflectionStatus = "enhancing"
	ReflectionStatusValidating ReflectionStatus = "validating"
	ReflectionStatusApproved   ReflectionStatus = "approved"
	ReflectionStatusRejected   ReflectionStatus = "rejected"
)

// ToolUsageRecord tracks how a tool was used during a session
type ToolUsageRecord struct {
	ToolName      string        `json:"tool_name"`
	CallCount     int           `json:"call_count"`
	TotalTokens   int64         `json:"total_tokens"`
	AvgTokens     float64       `json:"avg_tokens"`
	SuccessRate   float64       `json:"success_rate"` // 0-1
	AvgLatencyMs  int64         `json:"avg_latency_ms"`
	Errors        []string      `json:"errors,omitempty"`
	UsagePatterns []string      `json:"usage_patterns,omitempty"` // Common parameter combinations
}

// PainPoint represents a friction point identified during reflection
type PainPoint struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"` // token-cost, latency, missing-feature, error-prone, ux
	ToolName    string   `json:"tool_name"`
	Description string   `json:"description"`
	Impact      int      `json:"impact"` // 1-10
	Frequency   int      `json:"frequency"` // How often encountered
	Evidence    []string `json:"evidence"` // Specific examples
}

// ToolEnhancement represents a proposed tool improvement
type ToolEnhancement struct {
	ID              string             `json:"id"`
	ToolName        string             `json:"tool_name"`
	EnhancementType EnhancementType    `json:"enhancement_type"`
	Title           string             `json:"title"`
	Description     string             `json:"description"`
	TokenSavings    int64              `json:"token_savings"` // Estimated tokens saved per call
	Implementation  string             `json:"implementation,omitempty"` // Code snippet or approach
	Priority        EnhancementPriority `json:"priority"`
	Status          EnhancementStatus   `json:"status"`
	RelatedPainPoint string            `json:"related_pain_point,omitempty"`
}

// EnhancementType categorizes the type of improvement
type EnhancementType string

const (
	EnhanceAddParameter     EnhancementType = "add_parameter"      // Add new parameter
	EnhanceConsolidate      EnhancementType = "consolidate"        // Merge with similar tool
	EnhanceOutputFormat     EnhancementType = "output_format"      // Improve output structure
	EnhanceCaching          EnhancementType = "caching"            // Add/improve caching
	EnhanceDefaultValues    EnhancementType = "default_values"     // Better defaults
	EnhanceErrorMessages    EnhancementType = "error_messages"     // Clearer errors
	EnhanceDocumentation    EnhancementType = "documentation"      // Better descriptions
	EnhanceNewTool          EnhancementType = "new_tool"           // Create new tool
)

// EnhancementPriority represents enhancement priority
type EnhancementPriority string

const (
	PriorityP0 EnhancementPriority = "p0" // Critical - blocking release
	PriorityP1 EnhancementPriority = "p1" // High - significant token savings
	PriorityP2 EnhancementPriority = "p2" // Medium - quality of life
	PriorityP3 EnhancementPriority = "p3" // Low - nice to have
)

// EnhancementStatus tracks enhancement implementation
type EnhancementStatus string

const (
	EnhancementProposed   EnhancementStatus = "proposed"
	EnhancementApproved   EnhancementStatus = "approved"
	EnhancementImplemented EnhancementStatus = "implemented"
	EnhancementRejected   EnhancementStatus = "rejected"
)

// TokenEfficiencyAnalysis summarizes token usage efficiency
type TokenEfficiencyAnalysis struct {
	TotalTokensUsed      int64              `json:"total_tokens_used"`
	TokensByTool         map[string]int64   `json:"tokens_by_tool"`
	TopTokenConsumers    []string           `json:"top_token_consumers"` // Tools using most tokens
	WastedTokens         int64              `json:"wasted_tokens"`       // Tokens from failed/repeated calls
	EfficiencyScore      float64            `json:"efficiency_score"`    // 0-100
	SavingsOpportunities []SavingsOpportunity `json:"savings_opportunities"`
}

// SavingsOpportunity represents a token savings opportunity
type SavingsOpportunity struct {
	ToolName       string `json:"tool_name"`
	CurrentTokens  int64  `json:"current_tokens"`
	PotentialSave  int64  `json:"potential_save"`
	SavePercentage float64 `json:"save_percentage"`
	Strategy       string `json:"strategy"` // How to achieve savings
}

// SwarmReflectionManager manages reflection phases
type SwarmReflectionManager struct {
	vaultPath string
	phases    map[string]*ReflectionPhase
	mu        sync.RWMutex

	// Integration with existing components
	mcpClient       *SwarmMCPClient
	benchmarkClient *BenchmarkClient
}

// NewSwarmReflectionManager creates a new reflection manager
func NewSwarmReflectionManager(vaultPath string) *SwarmReflectionManager {
	if vaultPath == "" {
		home, _ := os.UserHomeDir()
		vaultPath = filepath.Join(home, "webb-vault")
	}
	return &SwarmReflectionManager{
		vaultPath: vaultPath,
		phases:    make(map[string]*ReflectionPhase),
	}
}

// SetMCPClient sets the MCP client for tool analysis
func (m *SwarmReflectionManager) SetMCPClient(client *SwarmMCPClient) {
	m.mcpClient = client
}

// SetBenchmarkClient sets the benchmark client for capability tracking
func (m *SwarmReflectionManager) SetBenchmarkClient(client *BenchmarkClient) {
	m.benchmarkClient = client
}

// StartReflection begins a new reflection phase for a roadmap version
func (m *SwarmReflectionManager) StartReflection(ctx context.Context, version string, sessionID string) (*ReflectionPhase, error) {
	phase := &ReflectionPhase{
		ID:             fmt.Sprintf("reflect-%s-%d", version, time.Now().UnixMilli()),
		SessionID:      sessionID,
		RoadmapVersion: version,
		Status:         ReflectionStatusPending,
		ToolUsage:      []ToolUsageRecord{},
		PainPoints:     []PainPoint{},
		Enhancements:   []ToolEnhancement{},
		CreatedAt:      time.Now(),
	}

	m.mu.Lock()
	m.phases[phase.ID] = phase
	m.mu.Unlock()

	// Save initial state
	if err := m.savePhase(phase); err != nil {
		return nil, fmt.Errorf("failed to save reflection phase: %w", err)
	}

	return phase, nil
}

// AnalyzeToolUsage analyzes tool usage from a session
func (m *SwarmReflectionManager) AnalyzeToolUsage(ctx context.Context, phaseID string, usageData map[string]*ToolUsageRecord) error {
	m.mu.Lock()
	phase, ok := m.phases[phaseID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("reflection phase not found: %s", phaseID)
	}
	phase.Status = ReflectionStatusAnalyzing
	m.mu.Unlock()

	// Convert map to sorted slice (by token usage, descending)
	usage := make([]ToolUsageRecord, 0, len(usageData))
	for name, record := range usageData {
		record.ToolName = name
		usage = append(usage, *record)
	}
	sort.Slice(usage, func(i, j int) bool {
		return usage[i].TotalTokens > usage[j].TotalTokens
	})

	m.mu.Lock()
	phase.ToolUsage = usage
	m.mu.Unlock()

	// Build token analysis
	analysis := m.buildTokenAnalysis(usage)
	m.mu.Lock()
	phase.TokenAnalysis = analysis
	m.mu.Unlock()

	return m.savePhase(phase)
}

// IdentifyPainPoints identifies pain points from tool usage patterns
func (m *SwarmReflectionManager) IdentifyPainPoints(ctx context.Context, phaseID string) ([]PainPoint, error) {
	m.mu.RLock()
	phase, ok := m.phases[phaseID]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("reflection phase not found: %s", phaseID)
	}
	usage := phase.ToolUsage
	m.mu.RUnlock()

	var painPoints []PainPoint
	ppID := 0

	for _, record := range usage {
		// High token cost with frequent usage
		if record.TotalTokens > 50000 && record.CallCount > 5 {
			ppID++
			painPoints = append(painPoints, PainPoint{
				ID:          fmt.Sprintf("pp-%d", ppID),
				Category:    "token-cost",
				ToolName:    record.ToolName,
				Description: fmt.Sprintf("Tool %s used %d times consuming %d tokens (avg %d/call)", record.ToolName, record.CallCount, record.TotalTokens, int(record.AvgTokens)),
				Impact:      int(float64(record.TotalTokens) / 10000), // Scale impact by token usage
				Frequency:   record.CallCount,
				Evidence:    []string{fmt.Sprintf("Total tokens: %d", record.TotalTokens)},
			})
		}

		// Low success rate
		if record.SuccessRate < 0.8 && record.CallCount > 2 {
			ppID++
			painPoints = append(painPoints, PainPoint{
				ID:          fmt.Sprintf("pp-%d", ppID),
				Category:    "error-prone",
				ToolName:    record.ToolName,
				Description: fmt.Sprintf("Tool %s has %.0f%% success rate with errors: %v", record.ToolName, record.SuccessRate*100, record.Errors),
				Impact:      int((1 - record.SuccessRate) * 10),
				Frequency:   record.CallCount,
				Evidence:    record.Errors,
			})
		}

		// High latency
		if record.AvgLatencyMs > 5000 && record.CallCount > 3 {
			ppID++
			painPoints = append(painPoints, PainPoint{
				ID:          fmt.Sprintf("pp-%d", ppID),
				Category:    "latency",
				ToolName:    record.ToolName,
				Description: fmt.Sprintf("Tool %s has high latency (avg %dms)", record.ToolName, record.AvgLatencyMs),
				Impact:      int(record.AvgLatencyMs / 1000),
				Frequency:   record.CallCount,
				Evidence:    []string{fmt.Sprintf("Average latency: %dms", record.AvgLatencyMs)},
			})
		}
	}

	// Sort by impact
	sort.Slice(painPoints, func(i, j int) bool {
		return painPoints[i].Impact > painPoints[j].Impact
	})

	m.mu.Lock()
	phase.PainPoints = painPoints
	m.mu.Unlock()

	return painPoints, m.savePhase(phase)
}

// GenerateEnhancements generates tool enhancements from pain points
func (m *SwarmReflectionManager) GenerateEnhancements(ctx context.Context, phaseID string) ([]ToolEnhancement, error) {
	m.mu.Lock()
	phase, ok := m.phases[phaseID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("reflection phase not found: %s", phaseID)
	}
	phase.Status = ReflectionStatusEnhancing
	painPoints := phase.PainPoints
	tokenAnalysis := phase.TokenAnalysis
	m.mu.Unlock()

	var enhancements []ToolEnhancement
	enhID := 0

	// Generate enhancements from pain points
	for _, pp := range painPoints {
		enhID++
		enh := ToolEnhancement{
			ID:               fmt.Sprintf("enh-%d", enhID),
			ToolName:         pp.ToolName,
			RelatedPainPoint: pp.ID,
		}

		switch pp.Category {
		case "token-cost":
			enh.EnhancementType = EnhanceOutputFormat
			enh.Title = fmt.Sprintf("Optimize %s output format", pp.ToolName)
			enh.Description = fmt.Sprintf("Reduce token usage by making output more concise. Current: %s", pp.Description)
			enh.TokenSavings = int64(float64(pp.Impact) * 1000) // Rough estimate
			enh.Priority = PriorityP1
			enh.Implementation = `Consider:
1. Add 'compact' mode parameter (default: true)
2. Remove verbose headers/footers
3. Use structured output for LLM parsing
4. Consolidate repeated information`

		case "error-prone":
			enh.EnhancementType = EnhanceErrorMessages
			enh.Title = fmt.Sprintf("Improve %s error handling", pp.ToolName)
			enh.Description = fmt.Sprintf("Reduce errors and improve messages. %s", pp.Description)
			enh.TokenSavings = int64(pp.Frequency * 500) // Saved retry tokens
			enh.Priority = PriorityP1
			enh.Implementation = `Consider:
1. Add parameter validation with helpful messages
2. Include fix suggestions in error responses
3. Add retry hints for transient failures
4. Log common error patterns for future improvement`

		case "latency":
			enh.EnhancementType = EnhanceCaching
			enh.Title = fmt.Sprintf("Add caching to %s", pp.ToolName)
			enh.Description = fmt.Sprintf("Improve performance by caching results. %s", pp.Description)
			enh.TokenSavings = 0 // Latency, not tokens
			enh.Priority = PriorityP2
			enh.Implementation = `Consider:
1. Add ResultCache integration with TTL
2. Cache at both tool level and underlying client
3. Add cache invalidation parameters
4. Track cache hit rate in metrics`

		default:
			enh.EnhancementType = EnhanceDocumentation
			enh.Title = fmt.Sprintf("Improve %s documentation", pp.ToolName)
			enh.Description = pp.Description
			enh.Priority = PriorityP3
		}

		enh.Status = EnhancementProposed
		enhancements = append(enhancements, enh)
	}

	// Add enhancements from token analysis savings opportunities
	if tokenAnalysis != nil {
		for _, opp := range tokenAnalysis.SavingsOpportunities {
			enhID++
			enh := ToolEnhancement{
				ID:              fmt.Sprintf("enh-%d", enhID),
				ToolName:        opp.ToolName,
				EnhancementType: EnhanceOutputFormat,
				Title:           fmt.Sprintf("Token optimization for %s", opp.ToolName),
				Description:     fmt.Sprintf("Strategy: %s (potential %d token savings, %.0f%%)", opp.Strategy, opp.PotentialSave, opp.SavePercentage),
				TokenSavings:    opp.PotentialSave,
				Priority:        m.priorityFromSavings(opp.PotentialSave),
				Status:          EnhancementProposed,
				Implementation:  opp.Strategy,
			}
			enhancements = append(enhancements, enh)
		}
	}

	// Sort by priority then token savings
	sort.Slice(enhancements, func(i, j int) bool {
		if enhancements[i].Priority != enhancements[j].Priority {
			return enhancements[i].Priority < enhancements[j].Priority
		}
		return enhancements[i].TokenSavings > enhancements[j].TokenSavings
	})

	m.mu.Lock()
	phase.Enhancements = enhancements
	m.mu.Unlock()

	return enhancements, m.savePhase(phase)
}

// ValidateForRelease validates that reflection requirements are met for release
func (m *SwarmReflectionManager) ValidateForRelease(ctx context.Context, phaseID string) (*ReflectionValidation, error) {
	m.mu.Lock()
	phase, ok := m.phases[phaseID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("reflection phase not found: %s", phaseID)
	}
	phase.Status = ReflectionStatusValidating
	m.mu.Unlock()

	validation := &ReflectionValidation{
		PhaseID:     phaseID,
		Version:     phase.RoadmapVersion,
		Checks:      []ReflectionCheck{},
		CanRelease:  true,
		ValidatedAt: time.Now(),
	}

	// Check 1: Reflection was performed
	check1 := ReflectionCheck{
		Name:   "reflection_performed",
		Passed: len(phase.ToolUsage) > 0,
		Detail: fmt.Sprintf("Analyzed %d tools", len(phase.ToolUsage)),
	}
	validation.Checks = append(validation.Checks, check1)
	if !check1.Passed {
		validation.CanRelease = false
		validation.BlockingReason = "Reflection phase not completed - no tool usage analysis"
	}

	// Check 2: Pain points identified
	check2 := ReflectionCheck{
		Name:   "pain_points_identified",
		Passed: true, // Pain points are optional - 0 is valid
		Detail: fmt.Sprintf("Identified %d pain points", len(phase.PainPoints)),
	}
	validation.Checks = append(validation.Checks, check2)

	// Check 3: High-priority enhancements addressed
	p0Count := 0
	p0Implemented := 0
	for _, enh := range phase.Enhancements {
		if enh.Priority == PriorityP0 {
			p0Count++
			if enh.Status == EnhancementImplemented || enh.Status == EnhancementRejected {
				p0Implemented++
			}
		}
	}
	check3 := ReflectionCheck{
		Name:   "p0_enhancements_addressed",
		Passed: p0Count == 0 || p0Implemented == p0Count,
		Detail: fmt.Sprintf("P0 enhancements: %d/%d addressed", p0Implemented, p0Count),
	}
	validation.Checks = append(validation.Checks, check3)
	if !check3.Passed {
		validation.CanRelease = false
		validation.BlockingReason = fmt.Sprintf("Unaddressed P0 enhancements: %d remaining", p0Count-p0Implemented)
	}

	// Check 4: Token efficiency threshold
	efficiencyPassed := true
	efficiencyDetail := "No token analysis available"
	if phase.TokenAnalysis != nil {
		efficiencyPassed = phase.TokenAnalysis.EfficiencyScore >= 60 // 60% minimum
		efficiencyDetail = fmt.Sprintf("Efficiency score: %.0f%%", phase.TokenAnalysis.EfficiencyScore)
	}
	check4 := ReflectionCheck{
		Name:   "token_efficiency",
		Passed: efficiencyPassed,
		Detail: efficiencyDetail,
	}
	validation.Checks = append(validation.Checks, check4)

	return validation, m.savePhase(phase)
}

// ApproveRelease marks the reflection phase as approved for release
func (m *SwarmReflectionManager) ApproveRelease(ctx context.Context, phaseID string, notes string) error {
	m.mu.Lock()
	phase, ok := m.phases[phaseID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("reflection phase not found: %s", phaseID)
	}

	now := time.Now()
	phase.Status = ReflectionStatusApproved
	phase.Approved = true
	phase.ApprovalNotes = notes
	phase.CompletedAt = &now
	m.mu.Unlock()

	return m.savePhase(phase)
}

// RejectRelease marks the reflection phase as rejected
func (m *SwarmReflectionManager) RejectRelease(ctx context.Context, phaseID string, reason string) error {
	m.mu.Lock()
	phase, ok := m.phases[phaseID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("reflection phase not found: %s", phaseID)
	}

	now := time.Now()
	phase.Status = ReflectionStatusRejected
	phase.Approved = false
	phase.ApprovalNotes = reason
	phase.CompletedAt = &now
	m.mu.Unlock()

	return m.savePhase(phase)
}

// ReflectionValidation represents validation results for a release
type ReflectionValidation struct {
	PhaseID        string              `json:"phase_id"`
	Version        string              `json:"version"`
	Checks         []ReflectionCheck   `json:"checks"`
	CanRelease     bool                `json:"can_release"`
	BlockingReason string              `json:"blocking_reason,omitempty"`
	ValidatedAt    time.Time           `json:"validated_at"`
}

// ReflectionCheck represents a single validation check
type ReflectionCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// buildTokenAnalysis builds token efficiency analysis from usage data
func (m *SwarmReflectionManager) buildTokenAnalysis(usage []ToolUsageRecord) *TokenEfficiencyAnalysis {
	analysis := &TokenEfficiencyAnalysis{
		TokensByTool:         make(map[string]int64),
		TopTokenConsumers:    []string{},
		SavingsOpportunities: []SavingsOpportunity{},
	}

	var totalTokens int64
	var wastedTokens int64

	for _, record := range usage {
		analysis.TokensByTool[record.ToolName] = record.TotalTokens
		totalTokens += record.TotalTokens

		// Estimate wasted tokens from failed calls
		failedCalls := int(float64(record.CallCount) * (1 - record.SuccessRate))
		wastedTokens += int64(failedCalls) * int64(record.AvgTokens)

		// Add to top consumers (already sorted by total tokens)
		if len(analysis.TopTokenConsumers) < 10 {
			analysis.TopTokenConsumers = append(analysis.TopTokenConsumers, record.ToolName)
		}

		// Identify savings opportunities
		if record.TotalTokens > 10000 && record.CallCount > 3 {
			potentialSave := int64(float64(record.TotalTokens) * 0.3) // 30% potential savings
			analysis.SavingsOpportunities = append(analysis.SavingsOpportunities, SavingsOpportunity{
				ToolName:       record.ToolName,
				CurrentTokens:  record.TotalTokens,
				PotentialSave:  potentialSave,
				SavePercentage: 30,
				Strategy:       m.getSavingsStrategy(record),
			})
		}
	}

	analysis.TotalTokensUsed = totalTokens
	analysis.WastedTokens = wastedTokens

	// Calculate efficiency score
	if totalTokens > 0 {
		wasteRatio := float64(wastedTokens) / float64(totalTokens)
		analysis.EfficiencyScore = (1 - wasteRatio) * 100
		if analysis.EfficiencyScore < 0 {
			analysis.EfficiencyScore = 0
		}
	} else {
		analysis.EfficiencyScore = 100
	}

	return analysis
}

// getSavingsStrategy returns a strategy for saving tokens on a tool
func (m *SwarmReflectionManager) getSavingsStrategy(record ToolUsageRecord) string {
	strategies := []string{}

	// High call count with high avg tokens = output optimization
	if record.CallCount > 5 && record.AvgTokens > 2000 {
		strategies = append(strategies, "Add compact output mode")
	}

	// Low success rate = validation improvement
	if record.SuccessRate < 0.9 {
		strategies = append(strategies, "Improve parameter validation to reduce failed calls")
	}

	// High latency = caching
	if record.AvgLatencyMs > 3000 {
		strategies = append(strategies, "Add result caching with appropriate TTL")
	}

	// Repeated patterns = consolidation
	if len(record.UsagePatterns) > 0 {
		strategies = append(strategies, "Consider consolidating common parameter patterns into defaults")
	}

	if len(strategies) == 0 {
		return "Review output format for redundant information"
	}

	return strings.Join(strategies, "; ")
}

// priorityFromSavings determines priority from token savings
func (m *SwarmReflectionManager) priorityFromSavings(savings int64) EnhancementPriority {
	if savings > 50000 {
		return PriorityP0
	} else if savings > 20000 {
		return PriorityP1
	} else if savings > 5000 {
		return PriorityP2
	}
	return PriorityP3
}

// savePhase persists a reflection phase to disk
func (m *SwarmReflectionManager) savePhase(phase *ReflectionPhase) error {
	dir := filepath.Join(m.vaultPath, "webb-dev", "reflections")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create reflections directory: %w", err)
	}

	filename := filepath.Join(dir, phase.ID+".json")
	data, err := json.MarshalIndent(phase, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal phase: %w", err)
	}

	return os.WriteFile(filename, data, 0644)
}

// LoadPhase loads a reflection phase from disk
func (m *SwarmReflectionManager) LoadPhase(phaseID string) (*ReflectionPhase, error) {
	filename := filepath.Join(m.vaultPath, "webb-dev", "reflections", phaseID+".json")
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read phase file: %w", err)
	}

	var phase ReflectionPhase
	if err := json.Unmarshal(data, &phase); err != nil {
		return nil, fmt.Errorf("failed to unmarshal phase: %w", err)
	}

	m.mu.Lock()
	m.phases[phase.ID] = &phase
	m.mu.Unlock()

	return &phase, nil
}

// ListPhases lists all reflection phases
func (m *SwarmReflectionManager) ListPhases() ([]*ReflectionPhase, error) {
	dir := filepath.Join(m.vaultPath, "webb-dev", "reflections")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*ReflectionPhase{}, nil
		}
		return nil, err
	}

	var phases []*ReflectionPhase
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		phaseID := strings.TrimSuffix(entry.Name(), ".json")
		phase, err := m.LoadPhase(phaseID)
		if err == nil {
			phases = append(phases, phase)
		}
	}

	// Sort by created time, newest first
	sort.Slice(phases, func(i, j int) bool {
		return phases[i].CreatedAt.After(phases[j].CreatedAt)
	})

	return phases, nil
}

// GetLatestPhase returns the most recent reflection phase for a version
func (m *SwarmReflectionManager) GetLatestPhase(version string) (*ReflectionPhase, error) {
	phases, err := m.ListPhases()
	if err != nil {
		return nil, err
	}

	for _, phase := range phases {
		if phase.RoadmapVersion == version {
			return phase, nil
		}
	}

	return nil, fmt.Errorf("no reflection phase found for version %s", version)
}

// FormatReflectionReport formats a reflection phase as a markdown report
func (m *SwarmReflectionManager) FormatReflectionReport(phase *ReflectionPhase) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Reflection Report: %s\n\n", phase.RoadmapVersion))
	sb.WriteString(fmt.Sprintf("**Phase ID:** %s\n", phase.ID))
	sb.WriteString(fmt.Sprintf("**Status:** %s\n", phase.Status))
	sb.WriteString(fmt.Sprintf("**Created:** %s\n", phase.CreatedAt.Format(time.RFC3339)))
	if phase.CompletedAt != nil {
		sb.WriteString(fmt.Sprintf("**Completed:** %s\n", phase.CompletedAt.Format(time.RFC3339)))
	}
	sb.WriteString("\n")

	// Tool Usage Summary
	sb.WriteString("## Tool Usage Summary\n\n")
	if len(phase.ToolUsage) > 0 {
		sb.WriteString("| Tool | Calls | Tokens | Avg Tokens | Success Rate |\n")
		sb.WriteString("|------|-------|--------|------------|-------------|\n")
		for _, u := range phase.ToolUsage[:minReflection(10, len(phase.ToolUsage))] {
			sb.WriteString(fmt.Sprintf("| %s | %d | %d | %.0f | %.0f%% |\n",
				u.ToolName, u.CallCount, u.TotalTokens, u.AvgTokens, u.SuccessRate*100))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("No tool usage data collected.\n\n")
	}

	// Token Efficiency
	if phase.TokenAnalysis != nil {
		sb.WriteString("## Token Efficiency\n\n")
		sb.WriteString(fmt.Sprintf("- **Total Tokens:** %d\n", phase.TokenAnalysis.TotalTokensUsed))
		sb.WriteString(fmt.Sprintf("- **Wasted Tokens:** %d\n", phase.TokenAnalysis.WastedTokens))
		sb.WriteString(fmt.Sprintf("- **Efficiency Score:** %.0f%%\n", phase.TokenAnalysis.EfficiencyScore))
		sb.WriteString("\n")

		if len(phase.TokenAnalysis.SavingsOpportunities) > 0 {
			sb.WriteString("### Savings Opportunities\n\n")
			for _, opp := range phase.TokenAnalysis.SavingsOpportunities {
				sb.WriteString(fmt.Sprintf("- **%s:** Save ~%d tokens (%.0f%%) - %s\n",
					opp.ToolName, opp.PotentialSave, opp.SavePercentage, opp.Strategy))
			}
			sb.WriteString("\n")
		}
	}

	// Pain Points
	sb.WriteString("## Pain Points\n\n")
	if len(phase.PainPoints) > 0 {
		for _, pp := range phase.PainPoints {
			sb.WriteString(fmt.Sprintf("### %s (%s)\n", pp.ToolName, pp.Category))
			sb.WriteString(fmt.Sprintf("%s\n", pp.Description))
			sb.WriteString(fmt.Sprintf("- Impact: %d/10\n", pp.Impact))
			sb.WriteString(fmt.Sprintf("- Frequency: %d occurrences\n\n", pp.Frequency))
		}
	} else {
		sb.WriteString("No pain points identified.\n\n")
	}

	// Enhancements
	sb.WriteString("## Proposed Enhancements\n\n")
	if len(phase.Enhancements) > 0 {
		for _, enh := range phase.Enhancements {
			statusIcon := "[ ]"
			if enh.Status == EnhancementImplemented {
				statusIcon = "[x]"
			} else if enh.Status == EnhancementRejected {
				statusIcon = "[-]"
			}
			sb.WriteString(fmt.Sprintf("### %s %s (%s)\n", statusIcon, enh.Title, enh.Priority))
			sb.WriteString(fmt.Sprintf("**Type:** %s | **Tool:** %s | **Est. Savings:** %d tokens\n\n", enh.EnhancementType, enh.ToolName, enh.TokenSavings))
			sb.WriteString(fmt.Sprintf("%s\n\n", enh.Description))
			if enh.Implementation != "" {
				sb.WriteString(fmt.Sprintf("**Implementation Notes:**\n```\n%s\n```\n\n", enh.Implementation))
			}
		}
	} else {
		sb.WriteString("No enhancements proposed.\n\n")
	}

	// Approval Status
	if phase.Approved {
		sb.WriteString("## Approval\n\n")
		sb.WriteString("**Status:** APPROVED\n")
		if phase.ApprovalNotes != "" {
			sb.WriteString(fmt.Sprintf("**Notes:** %s\n", phase.ApprovalNotes))
		}
	}

	return sb.String()
}

// minReflection returns the minimum of two ints (local to avoid redeclaration)
func minReflection(a, b int) int {
	if a < b {
		return a
	}
	return b
}
