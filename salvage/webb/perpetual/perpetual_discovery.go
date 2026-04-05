// Package clients provides the discovery pipeline for the perpetual engine.
package clients

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DiscoveryPipeline aggregates proposals from multiple sources
type DiscoveryPipeline struct {
	featureDiscoverer *FeatureDiscoverer
	mcpCatalog        *ServerTrackingCatalog
	grafanaClient     *GrafanaClient
	config            *DiscoveryPipelineConfig
	deduplicator      *ProposalDeduplicator
	stateStore        *PerpetualStateStore // Database-backed deduplication
}

// DiscoveryPipelineConfig configures the discovery pipeline
type DiscoveryPipelineConfig struct {
	// Source toggles
	EnableMCPEcosystem     bool
	EnablePylonTickets     bool
	EnableSlackDiscussions bool
	EnableHealthMetrics    bool
	EnableResearchPapers   bool
	EnableCompetitor       bool
	EnableRoadmap          bool // Parse roadmap for planned items

	// v24.0: New discovery source toggles
	EnableGitHubIssues bool // Mine GitHub issues for enhancement requests
	EnableSentryErrors bool // Mine Sentry for recurring error patterns

	// Lookback periods
	MCPDays      int
	PylonDays    int
	SlackDays    int
	HealthDays   int
	ResearchDays int
	SentryDays   int // v24.0: Sentry lookback

	// Paths
	RoadmapPath string // Path to Roadmap.md

	// Thresholds
	MinRelevance    int // Minimum relevance score (0-100)
	MinTicketCount  int // Minimum similar tickets to create proposal
	MinAlertCount   int // Minimum similar alerts to flag health gap
	MinErrorCount   int // v24.0: Minimum error occurrences for Sentry
}

// DefaultDiscoveryPipelineConfig returns sensible defaults
func DefaultDiscoveryPipelineConfig() *DiscoveryPipelineConfig {
	vaultPath := getVaultPath()
	roadmapPath := findRoadmapPath(vaultPath)

	return &DiscoveryPipelineConfig{
		EnableMCPEcosystem:     true,
		EnablePylonTickets:     true,
		EnableSlackDiscussions: true,
		EnableHealthMetrics:    true,
		EnableResearchPapers:   true, // SRE/observability research topics
		EnableCompetitor:       true, // Competitor feature analysis
		EnableRoadmap:          true, // Parse roadmap for planned items
		EnableGitHubIssues:     true, // v24.0: GitHub issues mining
		EnableSentryErrors:     true, // v24.0: Sentry error patterns
		MCPDays:                30,
		PylonDays:              30,
		SlackDays:              14,
		HealthDays:             7,
		ResearchDays:           90,
		SentryDays:             7, // v24.0: Week of Sentry data
		RoadmapPath:            roadmapPath,
		MinRelevance:           60,
		MinTicketCount:         2,
		MinAlertCount:          3,
		MinErrorCount:          3, // v24.0: 3+ occurrences
	}
}

// NewDiscoveryPipeline creates a new discovery pipeline
func NewDiscoveryPipeline(config *DiscoveryPipelineConfig) (*DiscoveryPipeline, error) {
	return NewDiscoveryPipelineWithStore(config, nil)
}

// NewDiscoveryPipelineWithStore creates a discovery pipeline with optional state store for persistent deduplication
func NewDiscoveryPipelineWithStore(config *DiscoveryPipelineConfig, stateStore *PerpetualStateStore) (*DiscoveryPipeline, error) {
	if config == nil {
		config = DefaultDiscoveryPipelineConfig()
	}

	fd, _ := GetGlobalFeatureDiscoverer()
	grafana, _ := NewGrafanaClient()

	// Try to initialize server tracking catalog from vault
	var catalog *ServerTrackingCatalog
	vaultPath := getVaultPath()
	if vaultPath != "" {
		catalog, _ = NewServerTrackingCatalog(vaultPath)
	}

	return &DiscoveryPipeline{
		featureDiscoverer: fd,
		mcpCatalog:        catalog,
		grafanaClient:     grafana,
		config:            config,
		deduplicator:      NewProposalDeduplicator(30 * 24 * time.Hour),
		stateStore:        stateStore, // Use database for persistent deduplication
	}, nil
}

// DiscoveryResult contains the results of a discovery run
type DiscoveryResult struct {
	Proposals        []*PerpetualProposal
	BySource         map[FeatureSource]int
	DuplicatesFound  int
	Errors           []string
	DiscoveredAt     time.Time
	DurationMs       int64
}

// RunDiscovery executes the full discovery pipeline
func (dp *DiscoveryPipeline) RunDiscovery(ctx context.Context) (*DiscoveryResult, error) {
	start := time.Now()
	result := &DiscoveryResult{
		Proposals:    make([]*PerpetualProposal, 0),
		BySource:     make(map[FeatureSource]int),
		DiscoveredAt: start,
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan string, 6)

	// Run all discovery sources in parallel
	sources := []struct {
		enabled bool
		name    string
		fn      func(context.Context) ([]*PerpetualProposal, error)
	}{
		{dp.config.EnableRoadmap, "Roadmap", dp.discoverFromRoadmap},
		{dp.config.EnableMCPEcosystem, "MCP Ecosystem", dp.discoverFromMCPEcosystem},
		{dp.config.EnablePylonTickets, "Pylon Tickets", dp.discoverFromPylon},
		{dp.config.EnableSlackDiscussions, "Slack Discussions", dp.discoverFromSlack},
		{dp.config.EnableHealthMetrics, "Health Metrics", dp.discoverFromHealthMetrics},
		{dp.config.EnableResearchPapers, "Research Papers", dp.discoverFromResearch},
		{dp.config.EnableCompetitor, "Competitor Analysis", dp.discoverFromCompetitors},
		// v24.0: New discovery sources
		{dp.config.EnableGitHubIssues, "GitHub Issues", dp.discoverFromGitHubIssues},
		{dp.config.EnableSentryErrors, "Sentry Errors", dp.discoverFromSentryErrors},
	}

	for _, src := range sources {
		if !src.enabled {
			continue
		}

		wg.Add(1)
		go func(name string, fn func(context.Context) ([]*PerpetualProposal, error)) {
			defer wg.Done()

			proposals, err := fn(ctx)
			if err != nil {
				errCh <- fmt.Sprintf("%s: %v", name, err)
				return
			}

			mu.Lock()
			result.Proposals = append(result.Proposals, proposals...)
			mu.Unlock()
		}(src.name, src.fn)
	}

	wg.Wait()
	close(errCh)

	// Collect errors
	for err := range errCh {
		result.Errors = append(result.Errors, err)
	}

	// Deduplicate proposals (prefer database if available, falls back to in-memory)
	dedupedProposals := make([]*PerpetualProposal, 0, len(result.Proposals))
	for _, p := range result.Proposals {
		isDuplicate := false

		// Check database first for persistent deduplication (survives restarts)
		if dp.stateStore != nil {
			exists, err := dp.stateStore.CheckContentHashExists(p.ContentHash, 30) // 30 day window
			if err == nil && exists {
				isDuplicate = true
			}
		}

		// Fall back to in-memory deduplicator for current session
		if !isDuplicate && dp.deduplicator.IsDuplicate(p) {
			isDuplicate = true
		}

		if !isDuplicate {
			dp.deduplicator.Add(p)
			dedupedProposals = append(dedupedProposals, p)
			result.BySource[p.Source]++
		} else {
			result.DuplicatesFound++
		}
	}
	result.Proposals = dedupedProposals

	// Sort by score (will be calculated later by the engine)
	sort.Slice(result.Proposals, func(i, j int) bool {
		return result.Proposals[i].Impact > result.Proposals[j].Impact
	})

	result.DurationMs = time.Since(start).Milliseconds()
	return result, nil
}

// discoverFromMCPEcosystem scans MCP ecosystem for integration opportunities
func (dp *DiscoveryPipeline) discoverFromMCPEcosystem(ctx context.Context) ([]*PerpetualProposal, error) {
	var proposals []*PerpetualProposal

	// Use existing feature discoverer for MCP
	if dp.featureDiscoverer != nil {
		featureProposals, err := dp.featureDiscoverer.DiscoverFromMCPEcosystem(ctx, dp.config.MCPDays)
		if err != nil {
			return nil, err
		}

		for _, fp := range featureProposals {
			proposals = append(proposals, convertFeatureProposal(fp))
		}
	}

	// Also check tracked MCP servers from catalog
	if dp.mcpCatalog != nil {
		servers := dp.mcpCatalog.List(ServerTrackingListOptions{Status: "planned"})
		for _, server := range servers {
			// High-relevance servers that haven't been implemented
			if server.RelevanceScore >= dp.config.MinRelevance {
				evidence := []string{}
				if server.RepoURL != "" {
					evidence = append(evidence, server.RepoURL)
				}

				proposal := &PerpetualProposal{
					ID:           fmt.Sprintf("mcp-tracked-%s", strings.ToLower(strings.ReplaceAll(server.Name, " ", "-"))),
					Source:       SourceMCPEcosystem,
					Title:        fmt.Sprintf("Integrate %s MCP server", server.Name),
					Description:  server.Description,
					Evidence:     evidence,
					Impact:       server.RelevanceScore,
					Effort:       mapEffort(server.IntegrationApproach),
					ContentHash:  generateContentHash(server.Name, server.Description),
					DiscoveredAt: time.Now(),
					Status:       "queued",
				}
				proposals = append(proposals, proposal)
			}
		}
	}

	return proposals, nil
}

// discoverFromPylon mines feature requests from support tickets
func (dp *DiscoveryPipeline) discoverFromPylon(ctx context.Context) ([]*PerpetualProposal, error) {
	if dp.featureDiscoverer == nil {
		return nil, nil
	}

	featureProposals, err := dp.featureDiscoverer.DiscoverFromPylonTickets(ctx, dp.config.PylonDays, dp.config.MinTicketCount)
	if err != nil {
		return nil, err
	}

	var proposals []*PerpetualProposal
	for _, fp := range featureProposals {
		proposals = append(proposals, convertFeatureProposal(fp))
	}

	return proposals, nil
}

// discoverFromSlack scans Slack for feature discussions
func (dp *DiscoveryPipeline) discoverFromSlack(ctx context.Context) ([]*PerpetualProposal, error) {
	if dp.featureDiscoverer == nil {
		return nil, nil
	}

	featureProposals, err := dp.featureDiscoverer.DiscoverFromSlack(ctx, dp.config.SlackDays)
	if err != nil {
		return nil, err
	}

	var proposals []*PerpetualProposal
	for _, fp := range featureProposals {
		proposals = append(proposals, convertFeatureProposal(fp))
	}

	return proposals, nil
}

// discoverFromHealthMetrics analyzes alerts and metrics for tooling gaps
func (dp *DiscoveryPipeline) discoverFromHealthMetrics(ctx context.Context) ([]*PerpetualProposal, error) {
	var proposals []*PerpetualProposal

	// Analyze recurring alert patterns to find automation opportunities
	alertPatterns := []struct {
		pattern     string
		title       string
		description string
		impact      int
		effort      EffortLevel
	}{
		{
			pattern:     "memory",
			title:       "Memory pressure automation",
			description: "Automated memory pressure detection and remediation suggestions",
			impact:      75,
			effort:      EffortMedium,
		},
		{
			pattern:     "disk",
			title:       "Disk space monitoring enhancement",
			description: "Proactive disk space alerts with cleanup recommendations",
			impact:      70,
			effort:      EffortSmall,
		},
		{
			pattern:     "connection",
			title:       "Connection pool health monitoring",
			description: "Monitor database and Redis connection pool health",
			impact:      80,
			effort:      EffortMedium,
		},
		{
			pattern:     "latency",
			title:       "Latency anomaly detection",
			description: "Detect and alert on API latency anomalies",
			impact:      75,
			effort:      EffortMedium,
		},
		{
			pattern:     "error rate",
			title:       "Error rate trending",
			description: "Track error rate trends and predict spikes",
			impact:      85,
			effort:      EffortLarge,
		},
		{
			pattern:     "queue",
			title:       "Queue depth prediction",
			description: "Predict queue backlogs before they become critical",
			impact:      80,
			effort:      EffortMedium,
		},
	}

	// Check if these monitoring capabilities exist in webb tools
	// For now, generate proposals for known gaps
	knownGaps := map[string]bool{
		"memory pressure automation": true,
		"latency anomaly detection":  true,
		"error rate trending":        true,
	}

	for _, ap := range alertPatterns {
		if knownGaps[strings.ToLower(ap.title)] {
			proposal := &PerpetualProposal{
				ID:           fmt.Sprintf("health-%s-%d", strings.ToLower(strings.ReplaceAll(ap.pattern, " ", "-")), time.Now().Unix()),
				Source:       SourceHealthMetrics,
				Title:        ap.title,
				Description:  ap.description,
				Evidence:     []string{fmt.Sprintf("Alert pattern: %s", ap.pattern)},
				Impact:       ap.impact,
				Effort:       ap.effort,
				ContentHash:  generateContentHash(ap.title, ap.description),
				DiscoveredAt: time.Now(),
				Status:       "queued",
			}
			proposals = append(proposals, proposal)
		}
	}

	// If Grafana client is available, check for firing alerts
	if dp.grafanaClient != nil {
		alerts, err := dp.grafanaClient.GetFiringAlerts(ctx, "")
		if err == nil && len(alerts) > 0 {
			// Group alerts by rule name to find patterns
			alertCounts := make(map[string]int)
			for _, alert := range alerts {
				ruleName := alert.Labels["alertname"]
				if ruleName == "" {
					ruleName = "unknown"
				}
				alertCounts[ruleName]++
			}

			for ruleName, count := range alertCounts {
				if count >= dp.config.MinAlertCount {
					proposal := &PerpetualProposal{
						ID:           fmt.Sprintf("health-alert-%s-%d", strings.ToLower(strings.ReplaceAll(ruleName, " ", "-")), time.Now().Unix()),
						Source:       SourceHealthMetrics,
						Title:        fmt.Sprintf("Automate response to '%s' alerts", ruleName),
						Description:  fmt.Sprintf("Recurring alert (%d instances) - create automated response or runbook", count),
						Evidence:     []string{fmt.Sprintf("Alert count: %d in past %d days", count, dp.config.HealthDays)},
						Impact:       60 + min(count*5, 30),
						Effort:       EffortMedium,
						ContentHash:  generateContentHash(ruleName, "alert-automation"),
						DiscoveredAt: time.Now(),
						Status:       "queued",
					}
					proposals = append(proposals, proposal)
				}
			}
		}
	}

	return proposals, nil
}

// discoverFromResearch scans research papers for SRE/observability ideas
func (dp *DiscoveryPipeline) discoverFromResearch(ctx context.Context) ([]*PerpetualProposal, error) {
	var proposals []*PerpetualProposal

	// SRE/observability research topics that could inspire new tools
	researchTopics := []struct {
		keyword     string
		title       string
		description string
		impact      int
	}{
		{
			keyword:     "AIOps",
			title:       "AI-powered anomaly detection",
			description: "Implement ML-based anomaly detection for metrics and logs, inspired by AIOps research",
			impact:      75,
		},
		{
			keyword:     "chaos engineering",
			title:       "Chaos experiment automation",
			description: "Tools for automating chaos engineering experiments based on Gremlin/LitmusChaos patterns",
			impact:      70,
		},
		{
			keyword:     "observability correlation",
			title:       "Cross-signal correlation",
			description: "Automatically correlate metrics, logs, and traces to identify root causes",
			impact:      80,
		},
		{
			keyword:     "incident prediction",
			title:       "Predictive incident detection",
			description: "Use historical patterns to predict incidents before they occur",
			impact:      85,
		},
		{
			keyword:     "runbook automation",
			title:       "Auto-remediation from runbooks",
			description: "Parse runbooks and automatically execute remediation steps",
			impact:      75,
		},
		{
			keyword:     "SLO optimization",
			title:       "SLO budget management",
			description: "Track error budgets and automatically adjust alerting thresholds",
			impact:      70,
		},
	}

	// Generate proposals from research topics (one per discovery cycle to avoid flooding)
	// Use a simple rotation based on time
	topicIndex := int(time.Now().Unix()/3600) % len(researchTopics) // Rotate hourly
	topic := researchTopics[topicIndex]

	proposal := &PerpetualProposal{
		ID:          fmt.Sprintf("research-%s-%d", strings.ReplaceAll(topic.keyword, " ", "-"), time.Now().Unix()),
		Source:      SourceResearchPapers,
		Title:       topic.title,
		Description: topic.description,
		Evidence:    []string{fmt.Sprintf("Research topic: %s", topic.keyword)},
		Impact:      topic.impact,
		Effort:      EffortLarge,
		ContentHash: generateContentHash(topic.title, topic.description),
		DiscoveredAt: time.Now(),
		Status:      "queued",
	}
	proposals = append(proposals, proposal)

	return proposals, nil
}

// discoverFromCompetitors analyzes competitor feature sets
func (dp *DiscoveryPipeline) discoverFromCompetitors(ctx context.Context) ([]*PerpetualProposal, error) {
	var proposals []*PerpetualProposal

	// Competitor features that webb could implement
	competitorFeatures := []struct {
		competitor  string
		feature     string
		title       string
		description string
		impact      int
	}{
		{
			competitor:  "DataDog",
			feature:     "Service Catalog",
			title:       "Service catalog integration",
			description: "Build a service catalog like DataDog with ownership, dependencies, and SLOs",
			impact:      80,
		},
		{
			competitor:  "Grafana OnCall",
			feature:     "Escalation Policies",
			title:       "Advanced escalation policies",
			description: "Multi-tier escalation with customizable rules like Grafana OnCall",
			impact:      75,
		},
		{
			competitor:  "PagerDuty",
			feature:     "Event Intelligence",
			title:       "Intelligent alert grouping",
			description: "ML-based alert grouping and noise reduction like PagerDuty Event Intelligence",
			impact:      85,
		},
		{
			competitor:  "New Relic",
			feature:     "Change Tracking",
			title:       "Deployment correlation",
			description: "Automatically correlate deployments with performance changes like New Relic",
			impact:      70,
		},
		{
			competitor:  "Honeycomb",
			feature:     "BubbleUp",
			title:       "Automatic attribute analysis",
			description: "Find attributes that explain anomalies like Honeycomb BubbleUp",
			impact:      75,
		},
	}

	// Rotate through competitor features (one per discovery cycle)
	featureIndex := int(time.Now().Unix()/3600) % len(competitorFeatures)
	feature := competitorFeatures[featureIndex]

	proposal := &PerpetualProposal{
		ID:          fmt.Sprintf("competitor-%s-%s-%d", strings.ToLower(feature.competitor), strings.ReplaceAll(strings.ToLower(feature.feature), " ", "-"), time.Now().Unix()),
		Source:      SourceCompetitor,
		Title:       feature.title,
		Description: feature.description,
		Evidence:    []string{fmt.Sprintf("Competitor: %s - Feature: %s", feature.competitor, feature.feature)},
		Impact:      feature.impact,
		Effort:      EffortLarge,
		ContentHash: generateContentHash(feature.title, feature.description),
		DiscoveredAt: time.Now(),
		Status:      "queued",
	}
	proposals = append(proposals, proposal)

	return proposals, nil
}

// discoverFromGitHubIssues mines GitHub issues for enhancement requests
// v24.0: New discovery source for direct user feedback
func (dp *DiscoveryPipeline) discoverFromGitHubIssues(ctx context.Context) ([]*PerpetualProposal, error) {
	var proposals []*PerpetualProposal

	// Initialize GitHub client
	ghClient, err := NewGitHubClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %w", err)
	}

	// Search for enhancement and feature request issues in webb repo
	labels := []string{"enhancement", "feature-request", "improvement"}
	for _, label := range labels {
		issues, err := ghClient.SearchIssues("hairglasses/webb", label, "open", 10)
		if err != nil {
			continue // Skip on error, try next label
		}

		for _, issue := range issues {
			// Skip if already has PR or is a bug
			if issue.PullRequest != nil {
				continue
			}

			// Calculate impact based on reactions and comments
			impact := 60 // Base impact
			if issue.Reactions.TotalCount > 5 {
				impact += 15
			}
			if issue.Comments > 3 {
				impact += 10
			}

			// Estimate effort from title/body
			effort := EffortMedium
			titleLower := strings.ToLower(issue.Title)
			if strings.Contains(titleLower, "add") || strings.Contains(titleLower, "new") {
				effort = EffortMedium
			} else if strings.Contains(titleLower, "fix") || strings.Contains(titleLower, "update") {
				effort = EffortSmall
			} else if strings.Contains(titleLower, "refactor") || strings.Contains(titleLower, "redesign") {
				effort = EffortLarge
			}

			proposal := &PerpetualProposal{
				ID:           fmt.Sprintf("github-issue-%d", issue.Number),
				Source:       SourceGitHubIssues,
				Title:        issue.Title,
				Description:  truncateDescription(issue.Body, 500),
				Evidence:     []string{issue.HTMLURL},
				Impact:       impact,
				Effort:       effort,
				ContentHash:  generateContentHash(fmt.Sprintf("github-%d", issue.Number), issue.Title),
				DiscoveredAt: time.Now(),
				Status:       "queued",
			}
			proposals = append(proposals, proposal)
		}
	}

	return proposals, nil
}

// discoverFromSentryErrors mines Sentry for recurring error patterns
// v24.0: New discovery source for operational pain points
func (dp *DiscoveryPipeline) discoverFromSentryErrors(ctx context.Context) ([]*PerpetualProposal, error) {
	var proposals []*PerpetualProposal

	// Initialize Sentry client
	sentryClient, err := NewSentryClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Sentry client: %w", err)
	}

	// Get unresolved issues with high frequency
	issues, err := sentryClient.GetIssues(ctx, "", "unresolved", 50)
	if err != nil {
		return nil, fmt.Errorf("failed to get Sentry issues: %w", err)
	}

	// Group by error type to find patterns
	errorPatterns := make(map[string][]SentryIssue)
	for _, issue := range issues {
		// Use metadata type as pattern key
		patternKey := issue.Metadata.Type
		if patternKey == "" {
			patternKey = issue.Culprit
		}
		if patternKey == "" {
			patternKey = "unknown"
		}
		errorPatterns[patternKey] = append(errorPatterns[patternKey], issue)
	}

	// Create proposals for recurring patterns
	for pattern, patternIssues := range errorPatterns {
		if len(patternIssues) < dp.config.MinErrorCount {
			continue
		}

		// Calculate total occurrences
		var totalCount int
		var evidence []string
		for _, issue := range patternIssues[:min(3, len(patternIssues))] {
			count := 1
			if issue.Count != "" {
				fmt.Sscanf(issue.Count, "%d", &count)
			}
			totalCount += count
			if issue.PermalinkURL != "" {
				evidence = append(evidence, issue.PermalinkURL)
			}
		}

		// Higher impact for more frequent errors
		impact := 60 + min(totalCount/10, 30)

		proposal := &PerpetualProposal{
			ID:           fmt.Sprintf("sentry-pattern-%s-%d", strings.ToLower(strings.ReplaceAll(pattern, " ", "-")), time.Now().Unix()),
			Source:       SourceSentryErrors,
			Title:        fmt.Sprintf("Fix recurring error: %s", pattern),
			Description:  fmt.Sprintf("Recurring error pattern with %d total occurrences across %d issues. Error type: %s", totalCount, len(patternIssues), pattern),
			Evidence:     evidence,
			Impact:       impact,
			Effort:       EffortMedium,
			ContentHash:  generateContentHash("sentry-"+pattern, fmt.Sprintf("%d-occurrences", totalCount)),
			DiscoveredAt: time.Now(),
			Status:       "queued",
		}
		proposals = append(proposals, proposal)
	}

	return proposals, nil
}

// truncateDescription truncates a string to max length with ellipsis
func truncateDescription(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// convertFeatureProposal converts a FeatureProposal to PerpetualProposal
func convertFeatureProposal(fp *FeatureProposal) *PerpetualProposal {
	var evidence []string
	for _, e := range fp.Evidence {
		if e.URL != "" {
			evidence = append(evidence, e.URL)
		} else if e.ID != "" {
			evidence = append(evidence, e.ID)
		}
	}

	return &PerpetualProposal{
		ID:           fp.ID,
		Source:       fp.Source,
		Title:        fp.Title,
		Description:  fp.Description,
		Evidence:     evidence,
		Impact:       fp.Impact,
		Effort:       fp.Effort,
		ContentHash:  generateContentHash(fp.Title, fp.Description),
		DiscoveredAt: fp.CreatedAt,
		Status:       "queued",
	}
}

// mapEffort maps approach strings to EffortLevel
func mapEffort(approach string) EffortLevel {
	switch strings.ToLower(approach) {
	case "native_port", "direct_install":
		return EffortLarge
	case "mcp_proxy":
		return EffortMedium
	case "pattern_ref":
		return EffortSmall
	default:
		return EffortMedium
	}
}

// generateContentHash creates a hash for deduplication
func generateContentHash(title, description string) string {
	content := strings.ToLower(strings.TrimSpace(title + "|" + description))
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:8])
}

// ProposalDeduplicator tracks seen proposals for deduplication
type ProposalDeduplicator struct {
	seen   map[string]time.Time
	window time.Duration
	mu     sync.RWMutex
}

// NewProposalDeduplicator creates a new deduplicator
func NewProposalDeduplicator(window time.Duration) *ProposalDeduplicator {
	return &ProposalDeduplicator{
		seen:   make(map[string]time.Time),
		window: window,
	}
}

// IsDuplicate checks if a proposal is a duplicate
func (d *ProposalDeduplicator) IsDuplicate(p *PerpetualProposal) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if seen, ok := d.seen[p.ContentHash]; ok {
		if time.Since(seen) < d.window {
			return true
		}
	}
	return false
}

// Add marks a proposal as seen
func (d *ProposalDeduplicator) Add(p *PerpetualProposal) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen[p.ContentHash] = time.Now()
}

// Cleanup removes old entries
func (d *ProposalDeduplicator) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoff := time.Now().Add(-d.window)
	for hash, seen := range d.seen {
		if seen.Before(cutoff) {
			delete(d.seen, hash)
		}
	}
}

// Count returns the number of tracked proposals
func (d *ProposalDeduplicator) Count() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.seen)
}

// getVaultPath returns the Obsidian vault path from environment or default
func getVaultPath() string {
	vaultPath := os.Getenv("OBSIDIAN_VAULT")
	if vaultPath == "" {
		home := os.Getenv("HOME")
		if home != "" {
			vaultPath = filepath.Join(home, "obsidian-vaults", "webb")
		}
	}
	return vaultPath
}

// findRoadmapPath searches for Roadmap.md in multiple locations
// Lessons learned: vault structures vary between webb-dev and gops-dev
func findRoadmapPath(vaultPath string) string {
	if vaultPath == "" {
		return ""
	}
	paths := []string{
		filepath.Join(vaultPath, "webb-dev", "Roadmap.md"),
		filepath.Join(vaultPath, "gops-dev", "Roadmap.md"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return paths[0] // Default to webb-dev
}

// discoverFromRoadmap parses the roadmap for planned but unimplemented items
func (dp *DiscoveryPipeline) discoverFromRoadmap(ctx context.Context) ([]*PerpetualProposal, error) {
	if dp.config.RoadmapPath == "" {
		return nil, nil
	}

	content, err := os.ReadFile(dp.config.RoadmapPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read roadmap: %w", err)
	}

	proposals := make([]*PerpetualProposal, 0)
	lines := strings.Split(string(content), "\n")

	var currentVersion string
	var inNextSteps bool

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track version headers (e.g., "### v6.12 - Feedback Loop")
		if strings.HasPrefix(trimmed, "### v") {
			currentVersion = trimmed
			inNextSteps = false

			// Skip completed versions
			if strings.Contains(trimmed, "✅") || strings.Contains(trimmed, "COMPLETE") {
				currentVersion = ""
				continue
			}
		}

		// Track "Next Steps" sections
		if strings.Contains(trimmed, "Next Steps") {
			inNextSteps = true
			continue
		}

		// Parse planned items from Next Steps (- v6.12: Description)
		if inNextSteps && strings.HasPrefix(trimmed, "- v") && !strings.Contains(trimmed, "✅") {
			// Extract version and title
			// Format: "- v6.12: Feedback Loop (weight adjustment)"
			parts := strings.SplitN(trimmed[2:], ":", 2)
			if len(parts) == 2 {
				version := strings.TrimSpace(parts[0])
				title := strings.TrimSpace(parts[1])

				// Estimate effort based on description
				effort := EffortMedium
				titleLower := strings.ToLower(title)
				if strings.Contains(titleLower, "mcp tools") || strings.Contains(titleLower, "cli") {
					effort = EffortSmall
				} else if strings.Contains(titleLower, "engine") || strings.Contains(titleLower, "orchestrator") {
					effort = EffortLarge
				}

				proposal := &PerpetualProposal{
					ID:           fmt.Sprintf("roadmap-%s-%d", version, i),
					Source:       SourceRoadmap,
					Title:        fmt.Sprintf("%s: %s", version, title),
					Description:  fmt.Sprintf("Implement %s from webb roadmap. %s", version, title),
					Evidence:     []string{dp.config.RoadmapPath, fmt.Sprintf("Line %d", i+1)},
					Impact:       85, // Roadmap items are high priority
					Effort:       effort,
					ContentHash:  generateContentHash(version, title),
					DiscoveredAt: time.Now(),
					Status:       "queued",
				}
				proposals = append(proposals, proposal)
			}
		}

		// Parse unchecked items in current version (- [ ] item)
		if currentVersion != "" && strings.HasPrefix(trimmed, "- [ ]") {
			itemText := strings.TrimPrefix(trimmed, "- [ ]")
			itemText = strings.TrimSpace(itemText)

			// Skip empty items
			if itemText == "" {
				continue
			}

			// Extract item, remove markdown formatting
			itemText = strings.Trim(itemText, "`")

			effort := EffortSmall
			if strings.Contains(strings.ToLower(itemText), "implement") {
				effort = EffortMedium
			}

			proposal := &PerpetualProposal{
				ID:           fmt.Sprintf("roadmap-item-%d", i),
				Source:       SourceRoadmap,
				Title:        itemText,
				Description:  fmt.Sprintf("Roadmap item from %s: %s", currentVersion, itemText),
				Evidence:     []string{dp.config.RoadmapPath, currentVersion, fmt.Sprintf("Line %d", i+1)},
				Impact:       80,
				Effort:       effort,
				ContentHash:  generateContentHash(currentVersion, itemText),
				DiscoveredAt: time.Now(),
				Status:       "queued",
			}
			proposals = append(proposals, proposal)
		}
	}

	return proposals, nil
}
