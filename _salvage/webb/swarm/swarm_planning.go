// Package clients provides the planning-mode swarm orchestrator
// v106.2: Planning-Mode Swarm Orchestration for deeper cross-referenced analysis
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

	"github.com/google/uuid"
)

// PlanningPhase represents phases of the planning swarm
type PlanningPhase string

const (
	PlanningPhaseResearch    PlanningPhase = "research"     // Phase 1: Parallel worker research
	PlanningPhaseCrossRef    PlanningPhase = "cross_ref"    // Phase 2: Cross-reference & merge
	PlanningPhaseDeepPlan    PlanningPhase = "deep_plan"    // Phase 3: Implementation planning
	PlanningPhaseComplete    PlanningPhase = "complete"
	PlanningPhaseFailed      PlanningPhase = "failed"
)

// PlanningSwarmConfig configures the planning-mode swarm
type PlanningSwarmConfig struct {
	// Phase durations
	ResearchDuration  time.Duration `json:"research_duration"`  // Default: 30min
	CrossRefDuration  time.Duration `json:"crossref_duration"`  // Default: 5min
	DeepPlanDuration  time.Duration `json:"deepplan_duration"`  // Default: 10min

	// Token budgets
	MaxTokensTotal     int64 `json:"max_tokens_total"`
	MaxTokensPerWorker int64 `json:"max_tokens_per_worker"`

	// Output configuration
	VaultPath         string `json:"vault_path"`
	OutputPath        string `json:"output_path"` // Default: webb-dev/swarm-improvements.md
	SaveHistory       bool   `json:"save_history"`
	SaveCrossRefs     bool   `json:"save_cross_refs"`

	// Planning configuration
	TopImprovementsCount int     `json:"top_improvements_count"` // Default: 10
	MinConfidence        int     `json:"min_confidence"`         // Default: 60
	MinImpact            int     `json:"min_impact"`             // Default: 50
	SimilarityThreshold  float64 `json:"similarity_threshold"`   // Default: 0.7

	// Workers to run in planning mode
	WorkerTypes []SwarmWorkerType `json:"worker_types"`
}

// DefaultPlanningSwarmConfig returns sensible planning defaults
func DefaultPlanningSwarmConfig() *PlanningSwarmConfig {
	return &PlanningSwarmConfig{
		ResearchDuration:     30 * time.Minute,
		CrossRefDuration:     5 * time.Minute,
		DeepPlanDuration:     10 * time.Minute,
		MaxTokensTotal:       2000000, // 2M tokens for focused planning
		MaxTokensPerWorker:   200000,  // 200K per worker
		VaultPath:            filepath.Join(os.Getenv("HOME"), "webb-vault"),
		OutputPath:           "webb-dev/swarm-improvements.md",
		SaveHistory:          true,
		SaveCrossRefs:        true,
		TopImprovementsCount: 10,
		MinConfidence:        60,
		MinImpact:            50,
		SimilarityThreshold:  0.7,
		WorkerTypes: []SwarmWorkerType{
			WorkerToolAuditor,
			WorkerSecurityAuditor,
			WorkerPerformanceProfiler,
			WorkerFeatureDiscovery,
			WorkerPatternDiscovery,
			WorkerImprovementAudit,
			WorkerCodeQuality,
			WorkerComplianceAudit,
		},
	}
}

// PlanningCrossRef represents a detected relationship between findings in planning mode
type PlanningCrossRef struct {
	ID             string    `json:"id"`
	FindingIDs     []string  `json:"finding_ids"`      // IDs of related findings
	RelationType   string    `json:"relation_type"`    // depends_on, blocks, related_to, duplicates
	Confidence     int       `json:"confidence"`       // 0-100
	Evidence       []string  `json:"evidence"`         // Why these are related
	AffectedFiles  []string  `json:"affected_files"`   // Files involved
	Impact         int       `json:"impact"`           // Combined impact score
	CreatedAt      time.Time `json:"created_at"`
}

// ImprovementPlan represents a detailed implementation plan for an improvement
type ImprovementPlan struct {
	ID              string             `json:"id"`
	Title           string             `json:"title"`
	Description     string             `json:"description"`
	Category        string             `json:"category"`
	Priority        int                `json:"priority"`        // Computed from impact/effort
	Impact          int                `json:"impact"`          // 0-100
	Effort          string             `json:"effort"`          // small, medium, large
	SourceFindings  []string           `json:"source_findings"` // Original finding IDs
	CrossRefs       []string           `json:"cross_refs"`      // Related cross-reference IDs
	Steps           []ImplementationStep `json:"steps"`
	FilesToModify   []string           `json:"files_to_modify"`
	Dependencies    []string           `json:"dependencies"`     // Other plans this depends on
	EstimatedTokens int64              `json:"estimated_tokens"` // Tokens to implement
	CreatedAt       time.Time          `json:"created_at"`
}

// ImplementationStep represents a step in the implementation plan
type ImplementationStep struct {
	Order       int      `json:"order"`
	Description string   `json:"description"`
	Files       []string `json:"files"`
	Tools       []string `json:"tools"`     // Webb tools involved
	Risks       []string `json:"risks"`
	Tests       []string `json:"tests"`     // Tests to add/modify
}

// PlanningSwarmResult represents the output of a planning swarm run
type PlanningSwarmResult struct {
	ID              string                    `json:"id"`
	Phase           PlanningPhase             `json:"phase"`
	StartedAt       time.Time                 `json:"started_at"`
	CompletedAt     *time.Time                `json:"completed_at,omitempty"`
	Duration        time.Duration             `json:"duration"`

	// Research phase outputs
	TotalFindings   int                       `json:"total_findings"`
	WorkerResults   map[SwarmWorkerType]int   `json:"worker_results"`

	// Cross-reference phase outputs
	CrossReferences []*PlanningCrossRef       `json:"cross_references"`
	RelationCounts  map[string]int            `json:"relation_counts"`

	// Planning phase outputs
	RankedImprovements []*ImprovementPlan     `json:"ranked_improvements"`
	TopPlans           []*ImprovementPlan     `json:"top_plans"`

	// Metrics
	TokensUsed      int64                     `json:"tokens_used"`
	Error           string                    `json:"error,omitempty"`
}

// PlanningSwarmOrchestrator orchestrates 3-phase planning workflow
type PlanningSwarmOrchestrator struct {
	id        string
	config    *PlanningSwarmConfig
	phase     PlanningPhase
	result    *PlanningSwarmResult

	// Internal state
	baseOrchestrator *SwarmOrchestrator   // Underlying swarm for Phase 1
	aggregator       *FindingAggregator
	crossRefs        []*PlanningCrossRef
	plans            []*ImprovementPlan

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// Global planning orchestrator singleton
var (
	globalPlanningOrchestrator   *PlanningSwarmOrchestrator
	globalPlanningOrchestratorMu sync.RWMutex
)

// SetGlobalPlanningOrchestrator sets the global planning orchestrator
func SetGlobalPlanningOrchestrator(p *PlanningSwarmOrchestrator) {
	globalPlanningOrchestratorMu.Lock()
	defer globalPlanningOrchestratorMu.Unlock()
	globalPlanningOrchestrator = p
}

// GetGlobalPlanningOrchestrator returns the global planning orchestrator
func GetGlobalPlanningOrchestrator() *PlanningSwarmOrchestrator {
	globalPlanningOrchestratorMu.RLock()
	defer globalPlanningOrchestratorMu.RUnlock()
	return globalPlanningOrchestrator
}

// NewPlanningSwarmOrchestrator creates a new planning-mode swarm orchestrator
func NewPlanningSwarmOrchestrator(config *PlanningSwarmConfig) (*PlanningSwarmOrchestrator, error) {
	if config == nil {
		config = DefaultPlanningSwarmConfig()
	}

	id := uuid.New().String()[:8]
	now := time.Now()

	p := &PlanningSwarmOrchestrator{
		id:         id,
		config:     config,
		phase:      PlanningPhaseResearch,
		aggregator: NewFindingAggregator(nil),
		crossRefs:  make([]*PlanningCrossRef, 0),
		plans:      make([]*ImprovementPlan, 0),
		result: &PlanningSwarmResult{
			ID:             id,
			Phase:          PlanningPhaseResearch,
			StartedAt:      now,
			WorkerResults:  make(map[SwarmWorkerType]int),
			RelationCounts: make(map[string]int),
		},
	}

	// Create base swarm with planning-specific config
	baseConfig := &SwarmConfig{
		Duration:           config.ResearchDuration,
		MaxTokensTotal:     config.MaxTokensTotal,
		MaxTokensPerWorker: config.MaxTokensPerWorker,
		VaultPath:          config.VaultPath,
		VaultLogging:       true,
		LocalMode:          true,
	}

	// Add workers from planning config
	for _, wt := range config.WorkerTypes {
		baseConfig.Workers = append(baseConfig.Workers, SwarmWorkerConfig{
			Type:  wt,
			Count: 1,
		})
	}

	base, err := NewSwarmOrchestrator(baseConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create base orchestrator: %w", err)
	}
	p.baseOrchestrator = base

	return p, nil
}

// Start initiates the 3-phase planning workflow
func (p *PlanningSwarmOrchestrator) Start(ctx context.Context) error {
	p.mu.Lock()
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.mu.Unlock()

	SetGlobalPlanningOrchestrator(p)

	// Phase 1: Parallel Research
	if err := p.runResearchPhase(); err != nil {
		p.setPhase(PlanningPhaseFailed)
		p.result.Error = fmt.Sprintf("research phase failed: %v", err)
		return err
	}

	// Phase 2: Cross-Reference & Merge
	if err := p.runCrossRefPhase(); err != nil {
		p.setPhase(PlanningPhaseFailed)
		p.result.Error = fmt.Sprintf("cross-ref phase failed: %v", err)
		return err
	}

	// Phase 3: Deep Planning
	if err := p.runDeepPlanPhase(); err != nil {
		p.setPhase(PlanningPhaseFailed)
		p.result.Error = fmt.Sprintf("deep planning phase failed: %v", err)
		return err
	}

	p.setPhase(PlanningPhaseComplete)
	now := time.Now()
	p.result.CompletedAt = &now
	p.result.Duration = now.Sub(p.result.StartedAt)

	// Save outputs
	if err := p.saveOutputs(); err != nil {
		return fmt.Errorf("failed to save outputs: %w", err)
	}

	return nil
}

// runResearchPhase executes Phase 1: Parallel worker research
func (p *PlanningSwarmOrchestrator) runResearchPhase() error {
	p.setPhase(PlanningPhaseResearch)

	// Start base swarm with timeout
	ctx, cancel := context.WithTimeout(p.ctx, p.config.ResearchDuration)
	defer cancel()

	if err := p.baseOrchestrator.Start(ctx); err != nil {
		return err
	}

	// Wait for research to complete or timeout
	select {
	case <-ctx.Done():
		// Timeout - stop gracefully
		_ = p.baseOrchestrator.Stop()
	case <-p.ctx.Done():
		// Cancelled
		_ = p.baseOrchestrator.Stop()
		return fmt.Errorf("planning swarm cancelled")
	}

	// Collect findings (empty filter = all findings)
	findings := p.baseOrchestrator.GetFindings("", "", 0)
	for _, f := range findings {
		p.aggregator.Add(f)
		p.result.WorkerResults[f.WorkerType]++
	}
	p.result.TotalFindings = len(findings)

	return nil
}

// runCrossRefPhase executes Phase 2: Cross-reference & merge
func (p *PlanningSwarmOrchestrator) runCrossRefPhase() error {
	p.setPhase(PlanningPhaseCrossRef)

	ctx, cancel := context.WithTimeout(p.ctx, p.config.CrossRefDuration)
	defer cancel()

	// Get aggregated findings
	aggregated := p.aggregator.GetAggregatedFindings()

	// Detect cross-references
	p.crossRefs = p.detectCrossReferences(ctx, aggregated)
	p.result.CrossReferences = p.crossRefs

	// Count relation types
	for _, xref := range p.crossRefs {
		p.result.RelationCounts[xref.RelationType]++
	}

	return nil
}

// detectCrossReferences finds relationships between findings
func (p *PlanningSwarmOrchestrator) detectCrossReferences(ctx context.Context, findings []*SwarmResearchFinding) []*PlanningCrossRef {
	var refs []*PlanningCrossRef

	// Build file → findings index
	fileIndex := make(map[string][]string)
	for _, f := range findings {
		files := extractFilesFromFinding(f)
		for _, file := range files {
			fileIndex[file] = append(fileIndex[file], f.ID)
		}
	}

	// Find findings that affect same files (co-location relationship)
	seen := make(map[string]bool)
	for file, ids := range fileIndex {
		if len(ids) < 2 {
			continue
		}

		// Create cross-reference for co-located findings
		key := strings.Join(sortedStrings(ids), ",")
		if seen[key] {
			continue
		}
		seen[key] = true

		xref := &PlanningCrossRef{
			ID:            uuid.New().String()[:8],
			FindingIDs:    ids,
			RelationType:  "related_to",
			Confidence:    75,
			Evidence:      []string{fmt.Sprintf("Both affect: %s", file)},
			AffectedFiles: []string{file},
			CreatedAt:     time.Now(),
		}

		// Calculate combined impact
		var maxImpact int
		for _, f := range findings {
			for _, id := range ids {
				if f.ID == id && f.Impact > maxImpact {
					maxImpact = f.Impact
				}
			}
		}
		xref.Impact = maxImpact

		refs = append(refs, xref)
	}

	// Find category-based relationships
	categoryIndex := make(map[string][]*SwarmResearchFinding)
	for _, f := range findings {
		categoryIndex[f.Category] = append(categoryIndex[f.Category], f)
	}

	// Detect potential duplicates via title similarity
	for _, f1 := range findings {
		for _, f2 := range findings {
			if f1.ID >= f2.ID { // Avoid duplicates and self-comparison
				continue
			}

			select {
			case <-ctx.Done():
				return refs
			default:
			}

			similarity := planningTitleSimilarity(f1.Title, f2.Title)
			if similarity >= p.config.SimilarityThreshold {
				xref := &PlanningCrossRef{
					ID:           uuid.New().String()[:8],
					FindingIDs:   []string{f1.ID, f2.ID},
					RelationType: "duplicates",
					Confidence:   int(similarity * 100),
					Evidence:     []string{fmt.Sprintf("Title similarity: %.0f%%", similarity*100)},
					Impact:       planningMax(f1.Impact, f2.Impact),
					CreatedAt:    time.Now(),
				}
				refs = append(refs, xref)
			}
		}
	}

	return refs
}

// runDeepPlanPhase executes Phase 3: Implementation planning
func (p *PlanningSwarmOrchestrator) runDeepPlanPhase() error {
	p.setPhase(PlanningPhaseDeepPlan)

	ctx, cancel := context.WithTimeout(p.ctx, p.config.DeepPlanDuration)
	defer cancel()

	// Get aggregated findings
	aggregated := p.aggregator.GetAggregatedFindings()

	// Filter by confidence/impact thresholds
	var qualified []*SwarmResearchFinding
	for _, f := range aggregated {
		if f.Confidence >= p.config.MinConfidence && f.Impact >= p.config.MinImpact {
			qualified = append(qualified, f)
		}
	}

	// Sort by impact (descending)
	sort.Slice(qualified, func(i, j int) bool {
		return qualified[i].Impact > qualified[j].Impact
	})

	// Take top N for deep planning
	topN := p.config.TopImprovementsCount
	if len(qualified) < topN {
		topN = len(qualified)
	}
	topFindings := qualified[:topN]

	// Generate implementation plans
	for i, f := range topFindings {
		select {
		case <-ctx.Done():
			break
		default:
		}

		plan := p.createImplementationPlan(f, i+1)

		// Link related cross-references
		for _, xref := range p.crossRefs {
			for _, id := range xref.FindingIDs {
				if id == f.ID {
					plan.CrossRefs = append(plan.CrossRefs, xref.ID)
				}
			}
		}

		p.plans = append(p.plans, plan)
	}

	// Rank and set top plans
	p.rankPlans()
	p.result.RankedImprovements = p.plans
	if len(p.plans) > 5 {
		p.result.TopPlans = p.plans[:5]
	} else {
		p.result.TopPlans = p.plans
	}

	return nil
}

// createImplementationPlan generates a detailed plan for a finding
func (p *PlanningSwarmOrchestrator) createImplementationPlan(f *SwarmResearchFinding, priority int) *ImprovementPlan {
	files := extractFilesFromFinding(f)

	plan := &ImprovementPlan{
		ID:             uuid.New().String()[:8],
		Title:          f.Title,
		Description:    f.Description,
		Category:       f.Category,
		Priority:       priority,
		Impact:         f.Impact,
		Effort:         f.Effort,
		SourceFindings: []string{f.ID},
		FilesToModify:  files,
		Steps:          []ImplementationStep{},
		CreatedAt:      time.Now(),
	}

	// Generate implementation steps based on category
	switch f.Category {
	case "tool-improvement", "performance-issue":
		plan.Steps = append(plan.Steps,
			ImplementationStep{
				Order:       1,
				Description: "Analyze current implementation",
				Files:       files,
				Tools:       []string{"webb_tool_schema", "webb_codebase_patterns"},
			},
			ImplementationStep{
				Order:       2,
				Description: "Implement improvement",
				Files:       files,
				Risks:       []string{"Potential breaking changes"},
			},
			ImplementationStep{
				Order:       3,
				Description: "Add tests",
				Tests:       []string{"unit tests", "integration tests"},
			},
			ImplementationStep{
				Order:       4,
				Description: "Verify with build",
				Tools:       []string{"go build", "go test"},
			},
		)
		plan.EstimatedTokens = 50000

	case "security-finding":
		plan.Steps = append(plan.Steps,
			ImplementationStep{
				Order:       1,
				Description: "Assess security impact",
				Tools:       []string{"webb_security_audit_full"},
				Risks:       []string{"Potential data exposure"},
			},
			ImplementationStep{
				Order:       2,
				Description: "Implement security fix",
				Files:       files,
			},
			ImplementationStep{
				Order:       3,
				Description: "Validate fix",
				Tests:       []string{"security tests", "penetration testing"},
			},
		)
		plan.EstimatedTokens = 75000

	case "knowledge-gap":
		plan.Steps = append(plan.Steps,
			ImplementationStep{
				Order:       1,
				Description: "Identify missing knowledge",
				Tools:       []string{"webb_vault_search", "webb_graph_orphans"},
			},
			ImplementationStep{
				Order:       2,
				Description: "Create missing documentation",
				Tools:       []string{"webb_obsidian_create", "webb_graph_link"},
			},
		)
		plan.EstimatedTokens = 20000

	default:
		plan.Steps = append(plan.Steps,
			ImplementationStep{
				Order:       1,
				Description: "Review finding details",
			},
			ImplementationStep{
				Order:       2,
				Description: "Implement change",
				Files:       files,
			},
			ImplementationStep{
				Order:       3,
				Description: "Verify implementation",
				Tools:       []string{"go build"},
			},
		)
		plan.EstimatedTokens = 30000
	}

	return plan
}

// rankPlans sorts plans by computed priority
func (p *PlanningSwarmOrchestrator) rankPlans() {
	sort.Slice(p.plans, func(i, j int) bool {
		// Priority score = Impact * 2 - Effort penalty
		scoreI := p.plans[i].Impact * 2
		if p.plans[i].Effort == "large" {
			scoreI -= 30
		} else if p.plans[i].Effort == "medium" {
			scoreI -= 15
		}

		scoreJ := p.plans[j].Impact * 2
		if p.plans[j].Effort == "large" {
			scoreJ -= 30
		} else if p.plans[j].Effort == "medium" {
			scoreJ -= 15
		}

		return scoreI > scoreJ
	})

	// Update priority numbers
	for i, plan := range p.plans {
		plan.Priority = i + 1
	}
}

// saveOutputs saves results to vault using centralized SwarmVaultLogger
func (p *PlanningSwarmOrchestrator) saveOutputs() error {
	// Try centralized logger first
	logger, err := GetSwarmVaultLogger()
	if err == nil {
		// Build paths
		outputPath := p.config.OutputPath
		historyPath := ""
		xrefPath := ""

		if p.config.SaveHistory {
			historyPath = filepath.Join("webb-dev", "swarm-history", "planning-runs",
				fmt.Sprintf("%s-planning.md", time.Now().Format("2006-01-02")))
		}

		if p.config.SaveCrossRefs && len(p.crossRefs) > 0 {
			xrefPath = filepath.Join("webb-dev", "swarm-plans", "cross-references.md")
		}

		if err := logger.LogPlanningResult(p.result, outputPath, historyPath, xrefPath); err != nil {
			// Fall back to legacy
			return p.saveOutputsLegacy()
		}

		// Save JSON result separately (always local)
		outDir := filepath.Join(p.config.VaultPath, filepath.Dir(p.config.OutputPath))
		if err := os.MkdirAll(outDir, 0755); err == nil {
			jsonPath := filepath.Join(outDir, "latest-planning-result.json")
			jsonData, _ := json.MarshalIndent(p.result, "", "  ")
			_ = os.WriteFile(jsonPath, jsonData, 0644)
		}

		return nil
	}

	// Fall back to legacy
	return p.saveOutputsLegacy()
}

// saveOutputsLegacy is the fallback for direct file writes
func (p *PlanningSwarmOrchestrator) saveOutputsLegacy() error {
	// Create output directory
	outDir := filepath.Join(p.config.VaultPath, filepath.Dir(p.config.OutputPath))
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	// Generate markdown output
	md := p.generateMarkdownReport()

	// Write main output
	outputPath := filepath.Join(p.config.VaultPath, p.config.OutputPath)
	if err := os.WriteFile(outputPath, []byte(md), 0644); err != nil {
		return err
	}

	// Save history if enabled
	if p.config.SaveHistory {
		historyDir := filepath.Join(p.config.VaultPath, "webb-dev", "swarm-history", "planning-runs")
		if err := os.MkdirAll(historyDir, 0755); err == nil {
			historyFile := filepath.Join(historyDir, fmt.Sprintf("%s-planning.md", time.Now().Format("2006-01-02")))
			_ = os.WriteFile(historyFile, []byte(md), 0644)
		}
	}

	// Save cross-references if enabled
	if p.config.SaveCrossRefs && len(p.crossRefs) > 0 {
		xrefFile := filepath.Join(p.config.VaultPath, "webb-dev", "swarm-plans", "cross-references.md")
		if err := os.MkdirAll(filepath.Dir(xrefFile), 0755); err == nil {
			xrefMd := p.generateCrossRefReport()
			_ = os.WriteFile(xrefFile, []byte(xrefMd), 0644)
		}
	}

	// Save JSON result
	jsonPath := filepath.Join(outDir, "latest-planning-result.json")
	jsonData, _ := json.MarshalIndent(p.result, "", "  ")
	_ = os.WriteFile(jsonPath, jsonData, 0644)

	return nil
}

// generateMarkdownReport creates the main improvements report
func (p *PlanningSwarmOrchestrator) generateMarkdownReport() string {
	var sb strings.Builder

	sb.WriteString("# Swarm Planning Improvements\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04 MST")))
	sb.WriteString(fmt.Sprintf("Duration: %s\n\n", p.result.Duration.Round(time.Second)))

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Findings**: %d\n", p.result.TotalFindings))
	sb.WriteString(fmt.Sprintf("- **Cross-References Detected**: %d\n", len(p.result.CrossReferences)))
	sb.WriteString(fmt.Sprintf("- **Implementation Plans**: %d\n\n", len(p.plans)))

	// Worker results
	if len(p.result.WorkerResults) > 0 {
		sb.WriteString("### Worker Results\n\n")
		sb.WriteString("| Worker | Findings |\n|--------|----------|\n")
		for wt, count := range p.result.WorkerResults {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", wt, count))
		}
		sb.WriteString("\n")
	}

	// Relation counts
	if len(p.result.RelationCounts) > 0 {
		sb.WriteString("### Cross-Reference Types\n\n")
		sb.WriteString("| Relation | Count |\n|----------|-------|\n")
		for rel, count := range p.result.RelationCounts {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", rel, count))
		}
		sb.WriteString("\n")
	}

	// Top Plans
	sb.WriteString("## Top Improvement Plans\n\n")
	for _, plan := range p.result.TopPlans {
		sb.WriteString(fmt.Sprintf("### %d. %s\n\n", plan.Priority, plan.Title))
		sb.WriteString(fmt.Sprintf("**Category**: %s | **Impact**: %d/100 | **Effort**: %s\n\n", plan.Category, plan.Impact, plan.Effort))
		sb.WriteString(fmt.Sprintf("%s\n\n", plan.Description))

		if len(plan.FilesToModify) > 0 {
			sb.WriteString("**Files to modify**:\n")
			for _, f := range plan.FilesToModify {
				sb.WriteString(fmt.Sprintf("- `%s`\n", f))
			}
			sb.WriteString("\n")
		}

		sb.WriteString("**Implementation Steps**:\n")
		for _, step := range plan.Steps {
			sb.WriteString(fmt.Sprintf("%d. %s\n", step.Order, step.Description))
		}
		sb.WriteString("\n---\n\n")
	}

	// All ranked improvements (condensed)
	if len(p.plans) > 5 {
		sb.WriteString("## All Ranked Improvements\n\n")
		sb.WriteString("| # | Title | Category | Impact | Effort |\n")
		sb.WriteString("|---|-------|----------|--------|--------|\n")
		for _, plan := range p.plans {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %d | %s |\n",
				plan.Priority, planningTruncate(plan.Title, 50), plan.Category, plan.Impact, plan.Effort))
		}
	}

	return sb.String()
}

// generateCrossRefReport creates the cross-references report
func (p *PlanningSwarmOrchestrator) generateCrossRefReport() string {
	var sb strings.Builder

	sb.WriteString("# Cross-References\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04 MST")))

	for _, xref := range p.crossRefs {
		sb.WriteString(fmt.Sprintf("## %s\n\n", xref.ID))
		sb.WriteString(fmt.Sprintf("- **Relation**: %s\n", xref.RelationType))
		sb.WriteString(fmt.Sprintf("- **Confidence**: %d%%\n", xref.Confidence))
		sb.WriteString(fmt.Sprintf("- **Impact**: %d\n", xref.Impact))
		sb.WriteString(fmt.Sprintf("- **Findings**: %s\n", strings.Join(xref.FindingIDs, ", ")))
		if len(xref.Evidence) > 0 {
			sb.WriteString(fmt.Sprintf("- **Evidence**: %s\n", strings.Join(xref.Evidence, "; ")))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// Stop halts the planning swarm
func (p *PlanningSwarmOrchestrator) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancel != nil {
		p.cancel()
	}

	if p.baseOrchestrator != nil {
		_ = p.baseOrchestrator.Stop()
	}

	return nil
}

// GetStatus returns current planning swarm status
func (p *PlanningSwarmOrchestrator) GetStatus() *PlanningSwarmResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Update duration
	if p.result.CompletedAt == nil {
		p.result.Duration = time.Since(p.result.StartedAt)
	}

	return p.result
}

// GetPhase returns current phase
func (p *PlanningSwarmOrchestrator) GetPhase() PlanningPhase {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.phase
}

func (p *PlanningSwarmOrchestrator) setPhase(phase PlanningPhase) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.phase = phase
	p.result.Phase = phase
}

// GetCrossReferences returns detected cross-references
func (p *PlanningSwarmOrchestrator) GetCrossReferences() []*PlanningCrossRef {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.crossRefs
}

// GetPlans returns ranked improvement plans
func (p *PlanningSwarmOrchestrator) GetPlans() []*ImprovementPlan {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.plans
}

// GetAggregatorStats returns aggregation statistics
func (p *PlanningSwarmOrchestrator) GetAggregatorStats() *AggregatorStats {
	if p.aggregator == nil {
		return nil
	}
	return p.aggregator.GetStats()
}

// RankImprovements ranks improvements by impact and effort
func RankImprovements(findings []*SwarmResearchFinding) []*SwarmResearchFinding {
	result := make([]*SwarmResearchFinding, len(findings))
	copy(result, findings)

	sort.Slice(result, func(i, j int) bool {
		// Score = Impact * 2 - Effort penalty
		scoreI := result[i].Impact * 2
		switch result[i].Effort {
		case "large":
			scoreI -= 30
		case "medium":
			scoreI -= 15
		}

		scoreJ := result[j].Impact * 2
		switch result[j].Effort {
		case "large":
			scoreJ -= 30
		case "medium":
			scoreJ -= 15
		}

		return scoreI > scoreJ
	})

	return result
}

// Helper functions

func extractFilesFromFinding(f *SwarmResearchFinding) []string {
	var files []string
	for _, ev := range f.Evidence {
		if strings.Contains(ev, ".go:") || strings.Contains(ev, ".md:") {
			// Extract file path
			parts := strings.Split(ev, ":")
			if len(parts) > 0 {
				file := strings.TrimSpace(parts[0])
				if strings.HasSuffix(file, ".go") || strings.HasSuffix(file, ".md") {
					files = append(files, file)
				}
			}
		}
	}
	return files
}

func planningTitleSimilarity(a, b string) float64 {
	// Simple Jaccard similarity on words
	wordsA := strings.Fields(strings.ToLower(a))
	wordsB := strings.Fields(strings.ToLower(b))

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	setA := make(map[string]bool)
	for _, w := range wordsA {
		setA[w] = true
	}

	var intersection int
	for _, w := range wordsB {
		if setA[w] {
			intersection++
		}
	}

	union := len(wordsA) + len(wordsB) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

func sortedStrings(s []string) []string {
	result := make([]string, len(s))
	copy(result, s)
	sort.Strings(result)
	return result
}

func planningTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func planningMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
