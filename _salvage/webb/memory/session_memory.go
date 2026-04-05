// Package clients provides API clients for external services.
// session_memory.go implements session memory & learning (v17.0)
// Based on Azure SRE Agent memory patterns + Devin session insights
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

// SessionMemoryClient manages the four-component memory system
// Pattern: Azure SRE Agent memory architecture
// v101.0: Updated for multi-user isolation (STATE-002-FIX)
type SessionMemoryClient struct {
	mu            sync.RWMutex
	vaultPath     string
	embedder      *EmbeddingClient
	// v101.0: Changed from slices to maps keyed by userID for multi-user isolation
	userMemories  map[string][]*UserMemory     // userID -> #remember commands
	insights      map[string][]*SessionInsight // userID -> Auto-generated from sessions
	memoryIndex   map[string]map[string]int    // userID -> memory_id -> slice index
	insightIndex  map[string]map[string]int    // userID -> insight_id -> slice index
}

// v101.0: DefaultUserID is used when no user context is available (backward compatibility)
const DefaultUserID = "default"

// v101.0: getUserID extracts userID from context or returns DefaultUserID
func getUserID(ctx context.Context) string {
	if ctx == nil {
		return DefaultUserID
	}
	// Try to get userID from context (set by MCP request handler)
	if userID, ok := ctx.Value("userID").(string); ok && userID != "" {
		return userID
	}
	return DefaultUserID
}

// UserMemory represents team knowledge (#remember pattern from Azure SRE Agent)
type UserMemory struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`   // "Team owns headspace in prod"
	Category  string    `json:"category"`  // team, service, workflow, standard
	AddedBy   string    `json:"added_by"`
	AddedAt   time.Time `json:"added_at"`
	Embedding []float32 `json:"embedding,omitempty"` // For semantic search
	Tags      []string  `json:"tags,omitempty"`
}

// SessionInsight captures learnings from an investigation (Azure + Devin pattern)
type SessionInsight struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`

	// What was observed
	Symptoms []string `json:"symptoms"`

	// Resolution path (what worked)
	StepsWorked []InsightStep `json:"steps_worked"`

	// Causal analysis
	RootCause string `json:"root_cause,omitempty"`

	// Negative learning (what to avoid)
	Pitfalls []string `json:"pitfalls,omitempty"`

	// Context & resources
	Context   map[string]string `json:"context"`   // cluster, customer, etc.
	Resources []string          `json:"resources"` // pods, services involved
	Services  []string          `json:"services"`  // affected services

	// Quality assessment
	QualityScore int    `json:"quality_score"` // 1-5
	Completeness string `json:"completeness"`  // complete, partial, minimal

	// Timeline (up to 8 milestones per Azure pattern)
	Timeline []TimelineMilestone `json:"timeline"`

	// Metadata
	GeneratedAt time.Time `json:"generated_at"`
	Embedding   []float32 `json:"embedding,omitempty"`

	// Summary for quick display
	Summary string `json:"summary,omitempty"`
}

// InsightStep represents a resolution step that worked
type InsightStep struct {
	Tool        string `json:"tool"`
	Description string `json:"description"`
	Outcome     string `json:"outcome"` // success, failed, partial
	Order       int    `json:"order"`
}

// TimelineMilestone represents a key event during investigation
type TimelineMilestone struct {
	Timestamp    time.Time `json:"timestamp"`
	Event        string    `json:"event"`
	Significance string    `json:"significance"` // discovery, decision, resolution
}

// MemorySearchResult contains results from unified memory search
type MemorySearchResult struct {
	UserMemories []*ScoredMemory  `json:"user_memories,omitempty"`
	Insights     []*ScoredInsight `json:"insights,omitempty"`
	TotalResults int              `json:"total_results"`
	SearchTime   time.Duration    `json:"search_time"`
}

// ScoredMemory is a memory with relevance score
type ScoredMemory struct {
	Memory *UserMemory `json:"memory"`
	Score  float32     `json:"score"`
}

// ScoredInsight is an insight with relevance score
type ScoredInsight struct {
	Insight *SessionInsight `json:"insight"`
	Score   float32         `json:"score"`
}

// RelevantContext contains auto-loaded memories for a new investigation
type RelevantContext struct {
	Memories       []*UserMemory     `json:"memories"`
	PastInsights   []*SessionInsight `json:"past_insights"`
	SuggestedSteps []string          `json:"suggested_steps"`
	Warnings       []string          `json:"warnings"`
}

var (
	sessionMemoryOnce     sync.Once
	sessionMemoryInstance *SessionMemoryClient
)

// GetSessionMemoryClient returns the global session memory client singleton
func GetSessionMemoryClient() *SessionMemoryClient {
	sessionMemoryOnce.Do(func() {
		sessionMemoryInstance, _ = NewSessionMemoryClient()
	})
	return sessionMemoryInstance
}

// NewSessionMemoryClient creates a new session memory client
// v101.0: Updated for multi-user isolation (STATE-002-FIX)
func NewSessionMemoryClient() (*SessionMemoryClient, error) {
	vaultPath := os.Getenv("OBSIDIAN_VAULT")
	if vaultPath == "" {
		vaultPath = filepath.Join(os.Getenv("HOME"), "webb-vault")
	}

	client := &SessionMemoryClient{
		vaultPath:    vaultPath,
		embedder:     GetEmbeddingClient(),
		userMemories: make(map[string][]*UserMemory),
		insights:     make(map[string][]*SessionInsight),
		memoryIndex:  make(map[string]map[string]int),
		insightIndex: make(map[string]map[string]int),
	}

	// Load existing memories and insights for default user (backward compatibility)
	if err := client.loadMemoriesForUser(DefaultUserID); err != nil {
		// Non-fatal, just log
		fmt.Fprintf(os.Stderr, "Warning: failed to load memories: %v\n", err)
	}
	if err := client.loadInsightsForUser(DefaultUserID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load insights: %v\n", err)
	}

	return client, nil
}

// Remember saves team knowledge (#remember pattern from Azure SRE Agent)
// v101.0: Uses default user for backward compatibility
func (c *SessionMemoryClient) Remember(content, category, addedBy string, tags []string) (*UserMemory, error) {
	return c.RememberWithContext(context.Background(), content, category, addedBy, tags)
}

// RememberWithContext saves team knowledge with user isolation
// v101.0: Added for multi-user isolation (STATE-002-FIX)
func (c *SessionMemoryClient) RememberWithContext(ctx context.Context, content, category, addedBy string, tags []string) (*UserMemory, error) {
	userID := getUserID(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	if content == "" {
		return nil, fmt.Errorf("memory content cannot be empty")
	}

	// Default category
	if category == "" {
		category = "general"
	}

	// Generate ID
	id := fmt.Sprintf("mem-%s-%d", strings.ToLower(category[:3]), time.Now().UnixNano())

	memory := &UserMemory{
		ID:       id,
		Content:  content,
		Category: category,
		AddedBy:  addedBy,
		AddedAt:  time.Now(),
		Tags:     tags,
	}

	// Generate embedding for semantic search
	if c.embedder != nil {
		result, err := c.embedder.Embed(ctx, content)
		if err == nil {
			memory.Embedding = result.Vector
		}
	}

	// v101.0: Initialize user's memory structures if needed
	if c.userMemories[userID] == nil {
		c.userMemories[userID] = []*UserMemory{}
	}
	if c.memoryIndex[userID] == nil {
		c.memoryIndex[userID] = make(map[string]int)
	}

	// Add to in-memory store for this user
	c.memoryIndex[userID][id] = len(c.userMemories[userID])
	c.userMemories[userID] = append(c.userMemories[userID], memory)

	// Persist
	if err := c.saveMemoriesForUser(userID); err != nil {
		return memory, fmt.Errorf("memory saved but persistence failed: %w", err)
	}

	return memory, nil
}

// Forget removes a memory by ID or semantic match
// v101.0: Uses default user for backward compatibility
func (c *SessionMemoryClient) Forget(idOrDescription string) error {
	return c.ForgetWithContext(context.Background(), idOrDescription)
}

// ForgetWithContext removes a memory by ID or semantic match with user isolation
// v101.0: Added for multi-user isolation (STATE-002-FIX)
func (c *SessionMemoryClient) ForgetWithContext(ctx context.Context, idOrDescription string) error {
	userID := getUserID(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Initialize user structures if needed
	if c.memoryIndex[userID] == nil {
		return fmt.Errorf("no matching memory found for: %s", idOrDescription)
	}

	// Try exact ID match first
	if idx, ok := c.memoryIndex[userID][idOrDescription]; ok {
		return c.removeMemoryAtForUser(userID, idx)
	}

	memories := c.userMemories[userID]
	if memories == nil {
		return fmt.Errorf("no matching memory found for: %s", idOrDescription)
	}

	// Try semantic match
	if c.embedder != nil {
		result, err := c.embedder.Embed(ctx, idOrDescription)
		if err == nil {
			var bestIdx int
			var bestScore float32

			for i, mem := range memories {
				if len(mem.Embedding) > 0 {
					score := CosineSimilarity(result.Vector, mem.Embedding)
					if score > bestScore {
						bestScore = score
						bestIdx = i
					}
				}
			}

			// Require high confidence for semantic delete
			if bestScore > 0.85 {
				return c.removeMemoryAtForUser(userID, bestIdx)
			}
		}
	}

	// Try content substring match
	lowerDesc := strings.ToLower(idOrDescription)
	for i, mem := range memories {
		if strings.Contains(strings.ToLower(mem.Content), lowerDesc) {
			return c.removeMemoryAtForUser(userID, i)
		}
	}

	return fmt.Errorf("no matching memory found for: %s", idOrDescription)
}

// removeMemoryAtForUser removes memory at index for a specific user and rebuilds index
// v101.0: Added for multi-user isolation (STATE-002-FIX)
func (c *SessionMemoryClient) removeMemoryAtForUser(userID string, idx int) error {
	memories := c.userMemories[userID]
	if idx < 0 || idx >= len(memories) {
		return fmt.Errorf("invalid memory index")
	}

	// Remove from slice
	c.userMemories[userID] = append(memories[:idx], memories[idx+1:]...)

	// Rebuild index for this user
	c.memoryIndex[userID] = make(map[string]int)
	for i, mem := range c.userMemories[userID] {
		c.memoryIndex[userID][mem.ID] = i
	}

	return c.saveMemoriesForUser(userID)
}

// Search queries across all memory components (unified SearchMemory)
// v101.0: Uses default user for backward compatibility
func (c *SessionMemoryClient) Search(query string, limit int) (*MemorySearchResult, error) {
	return c.SearchWithContext(context.Background(), query, limit)
}

// SearchWithContext queries across all memory components with user isolation
// v101.0: Added for multi-user isolation (STATE-002-FIX)
func (c *SessionMemoryClient) SearchWithContext(ctx context.Context, query string, limit int) (*MemorySearchResult, error) {
	userID := getUserID(ctx)
	start := time.Now()
	c.mu.RLock()
	defer c.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	result := &MemorySearchResult{
		UserMemories: []*ScoredMemory{},
		Insights:     []*ScoredInsight{},
	}

	// Generate query embedding
	var queryVector []float32
	if c.embedder != nil {
		if embResult, err := c.embedder.Embed(ctx, query); err == nil {
			queryVector = embResult.Vector
		}
	}

	lowerQuery := strings.ToLower(query)

	// v101.0: Search only this user's memories
	memories := c.userMemories[userID]
	for _, mem := range memories {
		var score float32

		// Semantic score
		if len(queryVector) > 0 && len(mem.Embedding) > 0 {
			score = CosineSimilarity(queryVector, mem.Embedding)
		}

		// Boost with keyword match
		if strings.Contains(strings.ToLower(mem.Content), lowerQuery) {
			score += 0.3
		}

		// Category match
		if strings.Contains(strings.ToLower(mem.Category), lowerQuery) {
			score += 0.1
		}

		if score > 0.2 {
			result.UserMemories = append(result.UserMemories, &ScoredMemory{
				Memory: mem,
				Score:  score,
			})
		}
	}

	// v101.0: Search only this user's insights
	insights := c.insights[userID]
	for _, insight := range insights {
		var score float32

		// Semantic score
		if len(queryVector) > 0 && len(insight.Embedding) > 0 {
			score = CosineSimilarity(queryVector, insight.Embedding)
		}

		// Keyword matching in various fields
		if strings.Contains(strings.ToLower(insight.Summary), lowerQuery) {
			score += 0.3
		}
		if strings.Contains(strings.ToLower(insight.RootCause), lowerQuery) {
			score += 0.25
		}
		for _, symptom := range insight.Symptoms {
			if strings.Contains(strings.ToLower(symptom), lowerQuery) {
				score += 0.15
				break
			}
		}

		// Context match (cluster, customer)
		for key, val := range insight.Context {
			if strings.Contains(strings.ToLower(val), lowerQuery) ||
				strings.Contains(strings.ToLower(key), lowerQuery) {
				score += 0.2
				break
			}
		}

		if score > 0.2 {
			result.Insights = append(result.Insights, &ScoredInsight{
				Insight: insight,
				Score:   score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(result.UserMemories, func(i, j int) bool {
		return result.UserMemories[i].Score > result.UserMemories[j].Score
	})
	sort.Slice(result.Insights, func(i, j int) bool {
		return result.Insights[i].Score > result.Insights[j].Score
	})

	// Apply limit
	if len(result.UserMemories) > limit {
		result.UserMemories = result.UserMemories[:limit]
	}
	if len(result.Insights) > limit {
		result.Insights = result.Insights[:limit]
	}

	result.TotalResults = len(result.UserMemories) + len(result.Insights)
	result.SearchTime = time.Since(start)

	return result, nil
}

// ListMemories lists user memories by category
// v101.0: Uses default user for backward compatibility
func (c *SessionMemoryClient) ListMemories(category string, limit int) ([]*UserMemory, error) {
	return c.ListMemoriesWithContext(context.Background(), category, limit)
}

// ListMemoriesWithContext lists user memories by category with user isolation
// v101.0: Added for multi-user isolation (STATE-002-FIX)
func (c *SessionMemoryClient) ListMemoriesWithContext(ctx context.Context, category string, limit int) ([]*UserMemory, error) {
	userID := getUserID(ctx)
	c.mu.RLock()
	defer c.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	var memories []*UserMemory
	// v101.0: Only list this user's memories
	for _, mem := range c.userMemories[userID] {
		if category == "" || strings.EqualFold(mem.Category, category) {
			memories = append(memories, mem)
		}
	}

	// Sort by added time descending
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].AddedAt.After(memories[j].AddedAt)
	})

	if len(memories) > limit {
		memories = memories[:limit]
	}

	return memories, nil
}

// GenerateInsight extracts learnings from a completed session
func (c *SessionMemoryClient) GenerateInsight(session *Session) (*SessionInsight, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}

	insight := &SessionInsight{
		ID:          fmt.Sprintf("insight-%s", session.ID),
		SessionID:   session.ID,
		Context:     make(map[string]string),
		GeneratedAt: time.Now(),
		Symptoms:    []string{},
		StepsWorked: []InsightStep{},
		Pitfalls:    []string{},
		Resources:   []string{},
		Services:    []string{},
		Timeline:    []TimelineMilestone{},
	}

	// 1. Extract symptoms from findings with category="symptom"
	for _, f := range session.Findings {
		switch f.Category {
		case "symptom":
			insight.Symptoms = append(insight.Symptoms, f.Description)
		case "root_cause":
			if insight.RootCause == "" {
				insight.RootCause = f.Description
			}
		case "fix", "workaround":
			insight.StepsWorked = append(insight.StepsWorked, InsightStep{
				Tool:        f.Source,
				Description: f.Description,
				Outcome:     "success",
				Order:       len(insight.StepsWorked) + 1,
			})
		case "pitfall", "failed":
			insight.Pitfalls = append(insight.Pitfalls, f.Description)
		}

		// Build timeline from findings
		if len(insight.Timeline) < 8 {
			significance := "discovery"
			if f.Category == "root_cause" {
				significance = "decision"
			} else if f.Category == "fix" {
				significance = "resolution"
			}
			insight.Timeline = append(insight.Timeline, TimelineMilestone{
				Timestamp:    f.Timestamp,
				Event:        f.Description,
				Significance: significance,
			})
		}
	}

	// 2. Extract context from session
	insight.Context["cluster"] = session.Cluster
	insight.Context["namespace"] = session.Namespace
	if session.Context.Customer != "" {
		insight.Context["customer"] = session.Context.Customer
	}
	if session.Context.IncidentID != "" {
		insight.Context["incident_id"] = session.Context.IncidentID
	}
	if session.Context.Environment != "" {
		insight.Context["environment"] = session.Context.Environment
	}

	// 3. Extract resources and services
	insight.Resources = session.Context.Resources
	insight.Services = session.Context.Services

	// 4. Calculate quality score (1-5)
	insight.QualityScore = c.calculateQualityScore(insight)

	// 5. Determine completeness
	insight.Completeness = c.assessCompleteness(insight)

	// 6. Generate summary
	insight.Summary = c.summarizeInsight(insight)

	// 7. Generate embedding for semantic search
	if c.embedder != nil {
		result, err := c.embedder.Embed(context.Background(), insight.Summary)
		if err == nil {
			insight.Embedding = result.Vector
		}
	}

	return insight, nil
}

// SaveInsight persists an insight
// v101.0: Uses default user for backward compatibility
func (c *SessionMemoryClient) SaveInsight(insight *SessionInsight) error {
	return c.SaveInsightWithContext(context.Background(), insight)
}

// SaveInsightWithContext persists an insight with user isolation
// v101.0: Added for multi-user isolation (STATE-002-FIX)
func (c *SessionMemoryClient) SaveInsightWithContext(ctx context.Context, insight *SessionInsight) error {
	userID := getUserID(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Initialize user structures if needed
	if c.insights[userID] == nil {
		c.insights[userID] = []*SessionInsight{}
	}
	if c.insightIndex[userID] == nil {
		c.insightIndex[userID] = make(map[string]int)
	}

	// Check if already exists for this user
	if idx, ok := c.insightIndex[userID][insight.ID]; ok {
		c.insights[userID][idx] = insight
	} else {
		c.insightIndex[userID][insight.ID] = len(c.insights[userID])
		c.insights[userID] = append(c.insights[userID], insight)
	}

	return c.saveInsightsForUser(userID)
}

// ListInsights lists insights by filters
// v101.0: Uses default user for backward compatibility
func (c *SessionMemoryClient) ListInsights(service, customer, cluster string, limit int) ([]*SessionInsight, error) {
	return c.ListInsightsWithContext(context.Background(), service, customer, cluster, limit)
}

// ListInsightsWithContext lists insights by filters with user isolation
// v101.0: Added for multi-user isolation (STATE-002-FIX)
func (c *SessionMemoryClient) ListInsightsWithContext(ctx context.Context, service, customer, cluster string, limit int) ([]*SessionInsight, error) {
	userID := getUserID(ctx)
	c.mu.RLock()
	defer c.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var insights []*SessionInsight
	// v101.0: Only list this user's insights
	for _, insight := range c.insights[userID] {
		// Apply filters
		if service != "" {
			found := false
			for _, s := range insight.Services {
				if strings.Contains(strings.ToLower(s), strings.ToLower(service)) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if customer != "" && insight.Context["customer"] != customer {
			continue
		}

		if cluster != "" && insight.Context["cluster"] != cluster {
			continue
		}

		insights = append(insights, insight)
	}

	// Sort by generated time descending
	sort.Slice(insights, func(i, j int) bool {
		return insights[i].GeneratedAt.After(insights[j].GeneratedAt)
	})

	if len(insights) > limit {
		insights = insights[:limit]
	}

	return insights, nil
}

// LoadRelevantContext auto-loads memories for a new investigation
func (c *SessionMemoryClient) LoadRelevantContext(symptoms []string, cluster, customer string) (*RelevantContext, error) {
	ctx := &RelevantContext{
		Memories:       []*UserMemory{},
		PastInsights:   []*SessionInsight{},
		SuggestedSteps: []string{},
		Warnings:       []string{},
	}

	// Build search query from inputs
	queryParts := append([]string{}, symptoms...)
	if cluster != "" {
		queryParts = append(queryParts, cluster)
	}
	if customer != "" {
		queryParts = append(queryParts, customer)
	}
	query := strings.Join(queryParts, " ")

	// Search memories
	searchResult, err := c.Search(query, 5)
	if err != nil {
		return ctx, err
	}

	// Add relevant memories
	for _, scored := range searchResult.UserMemories {
		if scored.Score > 0.4 {
			ctx.Memories = append(ctx.Memories, scored.Memory)
		}
	}

	// Add relevant past insights
	for _, scored := range searchResult.Insights {
		if scored.Score > 0.4 {
			ctx.PastInsights = append(ctx.PastInsights, scored.Insight)

			// Extract suggested steps from past insights
			for _, step := range scored.Insight.StepsWorked {
				if step.Outcome == "success" {
					ctx.SuggestedSteps = append(ctx.SuggestedSteps, step.Description)
				}
			}

			// Extract warnings from pitfalls
			ctx.Warnings = append(ctx.Warnings, scored.Insight.Pitfalls...)
		}
	}

	// Deduplicate suggested steps
	ctx.SuggestedSteps = uniqueStrings(ctx.SuggestedSteps)
	ctx.Warnings = uniqueStrings(ctx.Warnings)

	return ctx, nil
}

// calculateQualityScore computes investigation quality (1-5)
func (c *SessionMemoryClient) calculateQualityScore(insight *SessionInsight) int {
	score := 1

	// Has symptoms
	if len(insight.Symptoms) > 0 {
		score++
	}

	// Has root cause
	if insight.RootCause != "" {
		score++
	}

	// Has resolution steps
	if len(insight.StepsWorked) > 0 {
		score++
	}

	// Has resources/context
	if len(insight.Resources) > 0 || len(insight.Services) > 0 {
		score++
	}

	return score
}

// assessCompleteness determines investigation completeness
func (c *SessionMemoryClient) assessCompleteness(insight *SessionInsight) string {
	score := insight.QualityScore

	if score >= 4 {
		return "complete"
	} else if score >= 2 {
		return "partial"
	}
	return "minimal"
}

// summarizeInsight generates a text summary for embedding/display
func (c *SessionMemoryClient) summarizeInsight(insight *SessionInsight) string {
	var parts []string

	// Context
	if cluster := insight.Context["cluster"]; cluster != "" {
		parts = append(parts, fmt.Sprintf("Cluster: %s", cluster))
	}
	if customer := insight.Context["customer"]; customer != "" {
		parts = append(parts, fmt.Sprintf("Customer: %s", customer))
	}

	// Symptoms
	if len(insight.Symptoms) > 0 {
		parts = append(parts, fmt.Sprintf("Symptoms: %s", strings.Join(insight.Symptoms, "; ")))
	}

	// Root cause
	if insight.RootCause != "" {
		parts = append(parts, fmt.Sprintf("Root cause: %s", insight.RootCause))
	}

	// Resolution
	if len(insight.StepsWorked) > 0 {
		var steps []string
		for _, s := range insight.StepsWorked {
			steps = append(steps, s.Description)
		}
		parts = append(parts, fmt.Sprintf("Resolution: %s", strings.Join(steps, "; ")))
	}

	return strings.Join(parts, ". ")
}

// Persistence methods

// v101.0: loadMemoriesForUser loads memories for a specific user (STATE-002-FIX)
func (c *SessionMemoryClient) loadMemoriesForUser(userID string) error {
	// Try user-specific path first, fall back to legacy path for default user
	var memoriesFile string
	if userID == DefaultUserID {
		// Legacy path for backward compatibility
		memoriesFile = filepath.Join(c.vaultPath, "memories", "user_memories.json")
	} else {
		memoriesFile = filepath.Join(c.vaultPath, "memories", userID, "user_memories.json")
	}

	data, err := os.ReadFile(memoriesFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No memories yet
		}
		return err
	}

	var memories []*UserMemory
	if err := json.Unmarshal(data, &memories); err != nil {
		return err
	}

	c.userMemories[userID] = memories
	c.memoryIndex[userID] = make(map[string]int)
	for i, mem := range memories {
		c.memoryIndex[userID][mem.ID] = i
	}

	return nil
}

// v101.0: saveMemoriesForUser saves memories for a specific user (STATE-002-FIX)
func (c *SessionMemoryClient) saveMemoriesForUser(userID string) error {
	var memoriesDir string
	if userID == DefaultUserID {
		// Legacy path for backward compatibility
		memoriesDir = filepath.Join(c.vaultPath, "memories")
	} else {
		memoriesDir = filepath.Join(c.vaultPath, "memories", userID)
	}

	if err := os.MkdirAll(memoriesDir, 0755); err != nil {
		return err
	}

	memoriesFile := filepath.Join(memoriesDir, "user_memories.json")
	data, err := json.MarshalIndent(c.userMemories[userID], "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(memoriesFile, data, 0644)
}

// v101.0: loadInsightsForUser loads insights for a specific user (STATE-002-FIX)
func (c *SessionMemoryClient) loadInsightsForUser(userID string) error {
	var insightsDir string
	if userID == DefaultUserID {
		// Legacy path for backward compatibility
		insightsDir = filepath.Join(c.vaultPath, "insights")
	} else {
		insightsDir = filepath.Join(c.vaultPath, "insights", userID)
	}

	entries, err := os.ReadDir(insightsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if c.insights[userID] == nil {
		c.insights[userID] = []*SessionInsight{}
	}
	if c.insightIndex[userID] == nil {
		c.insightIndex[userID] = make(map[string]int)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "index.json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(insightsDir, entry.Name()))
		if err != nil {
			continue
		}

		var insight SessionInsight
		if err := json.Unmarshal(data, &insight); err != nil {
			continue
		}

		c.insightIndex[userID][insight.ID] = len(c.insights[userID])
		c.insights[userID] = append(c.insights[userID], &insight)
	}

	return nil
}

// v101.0: saveInsightsForUser saves insights for a specific user (STATE-002-FIX)
func (c *SessionMemoryClient) saveInsightsForUser(userID string) error {
	var insightsDir string
	if userID == DefaultUserID {
		// Legacy path for backward compatibility
		insightsDir = filepath.Join(c.vaultPath, "insights")
	} else {
		insightsDir = filepath.Join(c.vaultPath, "insights", userID)
	}

	if err := os.MkdirAll(insightsDir, 0755); err != nil {
		return err
	}

	// Save each insight to its own file
	for _, insight := range c.insights[userID] {
		filename := filepath.Join(insightsDir, insight.ID+".json")
		data, err := json.MarshalIndent(insight, "", "  ")
		if err != nil {
			continue
		}
		os.WriteFile(filename, data, 0644)
	}

	return nil
}

// Formatting methods

// FormatMemoryList formats memories for display
func FormatMemoryList(memories []*UserMemory) string {
	var sb strings.Builder

	sb.WriteString("## Team Memories\n\n")

	if len(memories) == 0 {
		sb.WriteString("No memories found. Use `#remember` to add team knowledge.\n")
		return sb.String()
	}

	sb.WriteString("| Category | Memory | Added By | Added |\n")
	sb.WriteString("|----------|--------|----------|-------|\n")

	for _, mem := range memories {
		content := mem.Content
		if len(content) > 60 {
			content = content[:57] + "..."
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			mem.Category,
			content,
			mem.AddedBy,
			mem.AddedAt.Format("2006-01-02"),
		))
	}

	return sb.String()
}

// FormatInsightList formats insights for display
func FormatInsightList(insights []*SessionInsight) string {
	var sb strings.Builder

	sb.WriteString("## Session Insights\n\n")

	if len(insights) == 0 {
		sb.WriteString("No insights found.\n")
		return sb.String()
	}

	for _, insight := range insights {
		sb.WriteString(fmt.Sprintf("### %s\n\n", insight.ID))

		// Quality badge
		quality := strings.Repeat("*", insight.QualityScore)
		sb.WriteString(fmt.Sprintf("**Quality:** %s (%s)\n\n", quality, insight.Completeness))

		// Context
		if cluster := insight.Context["cluster"]; cluster != "" {
			sb.WriteString(fmt.Sprintf("**Cluster:** %s\n", cluster))
		}
		if customer := insight.Context["customer"]; customer != "" {
			sb.WriteString(fmt.Sprintf("**Customer:** %s\n", customer))
		}
		sb.WriteString("\n")

		// Symptoms
		if len(insight.Symptoms) > 0 {
			sb.WriteString("**Symptoms:**\n")
			for _, s := range insight.Symptoms {
				sb.WriteString(fmt.Sprintf("- %s\n", s))
			}
			sb.WriteString("\n")
		}

		// Root cause
		if insight.RootCause != "" {
			sb.WriteString(fmt.Sprintf("**Root Cause:** %s\n\n", insight.RootCause))
		}

		// Steps that worked
		if len(insight.StepsWorked) > 0 {
			sb.WriteString("**Resolution Steps:**\n")
			for i, step := range insight.StepsWorked {
				sb.WriteString(fmt.Sprintf("%d. %s", i+1, step.Description))
				if step.Tool != "" {
					sb.WriteString(fmt.Sprintf(" (via %s)", step.Tool))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}

		// Pitfalls
		if len(insight.Pitfalls) > 0 {
			sb.WriteString("**Pitfalls to Avoid:**\n")
			for _, p := range insight.Pitfalls {
				sb.WriteString(fmt.Sprintf("- %s\n", p))
			}
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("*Generated: %s*\n\n---\n\n", insight.GeneratedAt.Format("2006-01-02 15:04")))
	}

	return sb.String()
}

// FormatSearchResult formats search results for display
func FormatSearchResult(result *MemorySearchResult) string {
	var sb strings.Builder

	sb.WriteString("## Memory Search Results\n\n")
	sb.WriteString(fmt.Sprintf("Found %d results in %v\n\n", result.TotalResults, result.SearchTime.Round(time.Millisecond)))

	if len(result.UserMemories) > 0 {
		sb.WriteString("### Team Memories\n\n")
		for _, scored := range result.UserMemories {
			sb.WriteString(fmt.Sprintf("- **[%.0f%%]** %s (%s)\n",
				scored.Score*100,
				scored.Memory.Content,
				scored.Memory.Category,
			))
		}
		sb.WriteString("\n")
	}

	if len(result.Insights) > 0 {
		sb.WriteString("### Past Insights\n\n")
		for _, scored := range result.Insights {
			summary := scored.Insight.Summary
			if len(summary) > 100 {
				summary = summary[:97] + "..."
			}
			sb.WriteString(fmt.Sprintf("- **[%.0f%%]** %s\n", scored.Score*100, summary))
		}
	}

	return sb.String()
}

// FormatRelevantContext formats auto-loaded context for display
func FormatRelevantContext(ctx *RelevantContext) string {
	var sb strings.Builder

	sb.WriteString("## Relevant Context Loaded\n\n")

	if len(ctx.Memories) > 0 {
		sb.WriteString("### Team Knowledge\n\n")
		for _, mem := range ctx.Memories {
			sb.WriteString(fmt.Sprintf("- %s\n", mem.Content))
		}
		sb.WriteString("\n")
	}

	if len(ctx.SuggestedSteps) > 0 {
		sb.WriteString("### Suggested Investigation Steps\n\n")
		sb.WriteString("Based on similar past incidents:\n")
		for i, step := range ctx.SuggestedSteps {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
		}
		sb.WriteString("\n")
	}

	if len(ctx.Warnings) > 0 {
		sb.WriteString("### Pitfalls to Avoid\n\n")
		for _, warning := range ctx.Warnings {
			sb.WriteString(fmt.Sprintf("- %s\n", warning))
		}
		sb.WriteString("\n")
	}

	if len(ctx.PastInsights) > 0 {
		sb.WriteString(fmt.Sprintf("### Related Incidents: %d past investigations found\n\n", len(ctx.PastInsights)))
	}

	return sb.String()
}

// Note: uniqueStrings helper is defined in proactive_alert.go

// =============================================================================
// v103.0 THINK-002: Thinking Context Clearing
// =============================================================================

// ThinkingContext tracks extended thinking state for a session
type ThinkingContext struct {
	SessionID       string    `json:"session_id"`
	UserID          string    `json:"user_id"`
	ThinkingBlocks  int       `json:"thinking_blocks"`    // Number of thinking blocks in session
	TotalTokens     int64     `json:"total_tokens"`       // Total thinking tokens used
	ClearedAt       time.Time `json:"cleared_at,omitempty"`
	LastThinkingAt  time.Time `json:"last_thinking_at"`
}

var (
	thinkingContexts   = make(map[string]*ThinkingContext) // sessionID -> context
	thinkingContextsMu sync.RWMutex
)

// RecordThinkingBlock records a thinking block for a session
func RecordThinkingBlock(sessionID, userID string, tokens int64) {
	thinkingContextsMu.Lock()
	defer thinkingContextsMu.Unlock()

	ctx, exists := thinkingContexts[sessionID]
	if !exists {
		ctx = &ThinkingContext{
			SessionID: sessionID,
			UserID:    userID,
		}
		thinkingContexts[sessionID] = ctx
	}

	ctx.ThinkingBlocks++
	ctx.TotalTokens += tokens
	ctx.LastThinkingAt = time.Now()
}

// GetThinkingContext returns the thinking context for a session (Bug 1.9 fix: nil check)
func GetThinkingContext(sessionID string) *ThinkingContext {
	thinkingContextsMu.RLock()
	defer thinkingContextsMu.RUnlock()

	if ctx, ok := thinkingContexts[sessionID]; ok && ctx != nil {
		return ctx
	}
	return nil
}

// ClearThinkingContext clears thinking blocks at session boundary
// This implements the clear_thinking_20251015 pattern from Claude API
func ClearThinkingContext(sessionID string) bool {
	thinkingContextsMu.Lock()
	defer thinkingContextsMu.Unlock()

	if ctx, ok := thinkingContexts[sessionID]; ok {
		ctx.ClearedAt = time.Now()
		ctx.ThinkingBlocks = 0
		ctx.TotalTokens = 0
		return true
	}
	return false
}

// ClearAllThinkingContexts clears all thinking contexts (e.g., at server restart)
func ClearAllThinkingContexts() int {
	thinkingContextsMu.Lock()
	defer thinkingContextsMu.Unlock()

	count := len(thinkingContexts)
	thinkingContexts = make(map[string]*ThinkingContext)
	return count
}

// CleanupStaleThinkingContexts removes contexts older than the given duration
func CleanupStaleThinkingContexts(maxAge time.Duration) int {
	thinkingContextsMu.Lock()
	defer thinkingContextsMu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for sessionID, ctx := range thinkingContexts {
		if ctx.LastThinkingAt.Before(cutoff) {
			delete(thinkingContexts, sessionID)
			removed++
		}
	}

	return removed
}

// GetThinkingStats returns overall thinking usage statistics
func GetThinkingStats() map[string]interface{} {
	thinkingContextsMu.RLock()
	defer thinkingContextsMu.RUnlock()

	totalSessions := len(thinkingContexts)
	totalBlocks := 0
	totalTokens := int64(0)

	for _, ctx := range thinkingContexts {
		totalBlocks += ctx.ThinkingBlocks
		totalTokens += ctx.TotalTokens
	}

	return map[string]interface{}{
		"active_sessions":   totalSessions,
		"total_blocks":      totalBlocks,
		"total_tokens":      totalTokens,
		"avg_tokens_per_session": func() float64 {
			if totalSessions == 0 {
				return 0
			}
			return float64(totalTokens) / float64(totalSessions)
		}(),
	}
}
