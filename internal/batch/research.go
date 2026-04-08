package batch

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// ResearchBatchAdapter wraps BatchManager with research-specific prompt
// construction and result parsing. Used by the research daemon for
// complexity 2-3 topics that benefit from batch API cost discounts.
type ResearchBatchAdapter struct {
	manager *BatchManager

	mu       sync.Mutex
	pending  map[string]researchItem // requestID -> metadata
	batchIDs []string                // submitted batch IDs awaiting results
}

type researchItem struct {
	Topic      string
	Domain     string
	Mode       string // "new" or "expand"
	Complexity int
}

// ResearchResult holds the parsed output from a batch research request.
type ResearchResult struct {
	Topic      string `json:"topic"`
	Domain     string `json:"domain"`
	Content    string `json:"content"`
	Mode       string `json:"mode"`
	Complexity int    `json:"complexity"`
	RequestID  string `json:"request_id"`
	Error      string `json:"error,omitempty"`
}

// NewResearchBatchAdapter creates an adapter wrapping the given batch manager.
func NewResearchBatchAdapter(mgr *BatchManager) *ResearchBatchAdapter {
	return &ResearchBatchAdapter{
		manager: mgr,
		pending: make(map[string]researchItem),
	}
}

// EnqueueResearch submits a research topic to the batch queue. Returns the
// request ID for later result collection.
func (a *ResearchBatchAdapter) EnqueueResearch(ctx context.Context, topic, domain, mode string, complexity int) (string, error) {
	model := modelForComplexity(complexity)
	system := researchSystemPrompt(complexity)
	user := researchUserPrompt(topic, domain, mode, complexity)

	reqID := fmt.Sprintf("research-%s-%s", domain, sanitizeID(topic))

	req := BatchManagerRequest{
		Request: Request{
			ID:           reqID,
			Model:        model,
			SystemPrompt: system,
			UserPrompt:   user,
			MaxTokens:    maxTokensForComplexity(complexity),
			Metadata: map[string]string{
				"topic":      topic,
				"domain":     domain,
				"mode":       mode,
				"complexity": fmt.Sprintf("%d", complexity),
				"source":     "research-daemon",
			},
		},
		Priority: PriorityNormal,
	}

	submittedID, err := a.manager.Submit(ctx, req)
	if err != nil {
		return "", fmt.Errorf("enqueue research: %w", err)
	}

	a.mu.Lock()
	a.pending[reqID] = researchItem{
		Topic:      topic,
		Domain:     domain,
		Mode:       mode,
		Complexity: complexity,
	}
	a.mu.Unlock()

	slog.Debug("research-batch: enqueued", "topic", topic, "domain", domain,
		"complexity", complexity, "request_id", submittedID)
	return submittedID, nil
}

// FlushIfReady triggers a batch flush if the manager has pending items.
// Returns the batch result (batch ID, count) or nil if nothing to flush.
func (a *ResearchBatchAdapter) FlushIfReady(ctx context.Context) (*BatchManagerResult, error) {
	result, err := a.manager.Flush(ctx)
	if err != nil {
		return nil, fmt.Errorf("flush: %w", err)
	}
	if result != nil && result.BatchID != "" {
		a.mu.Lock()
		a.batchIDs = append(a.batchIDs, result.BatchID)
		a.mu.Unlock()
		slog.Info("research-batch: flushed", "batch_id", result.BatchID,
			"requests", result.RequestCount)
	}
	return result, nil
}

// PendingCount returns the number of research items awaiting batch submission.
func (a *ResearchBatchAdapter) PendingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pending)
}

// ParseResults converts raw batch results into research results.
func (a *ResearchBatchAdapter) ParseResults(results []Result) []ResearchResult {
	a.mu.Lock()
	defer a.mu.Unlock()

	var out []ResearchResult
	for _, r := range results {
		item, ok := a.pending[r.RequestID]
		if !ok {
			continue
		}
		rr := ResearchResult{
			Topic:      item.Topic,
			Domain:     item.Domain,
			Content:    r.Content,
			Mode:       item.Mode,
			Complexity: item.Complexity,
			RequestID:  r.RequestID,
			Error:      r.Error,
		}
		out = append(out, rr)
		delete(a.pending, r.RequestID)
	}
	return out
}

// modelForComplexity returns the batch model for a given complexity level.
func modelForComplexity(complexity int) string {
	switch complexity {
	case 1:
		return "gemini-3.1-flash-lite"
	case 2:
		return "gemini-3.1-flash"
	case 3:
		return "claude-sonnet-4-20250514" // batch = 50% discount
	case 4:
		return "claude-opus-4-20250514"
	default:
		return "gemini-3.1-flash"
	}
}

func maxTokensForComplexity(complexity int) int {
	switch complexity {
	case 1:
		return 2048
	case 2:
		return 4096
	case 3:
		return 8192
	case 4:
		return 16384
	default:
		return 4096
	}
}

func researchSystemPrompt(complexity int) string {
	base := `You are a research agent for a software engineering knowledge base. Your task is to produce structured, technically accurate research documents.

Guidelines:
- Use markdown with clear headings and sections
- Include code examples where relevant
- Cite sources with URLs
- Focus on practical, actionable information
- Target audience: experienced software engineers`

	if complexity >= 3 {
		base += `
- For cross-domain topics, explicitly map relationships between domains
- Include architectural diagrams (as markdown/ASCII) where helpful
- Provide comparison tables for competing approaches`
	}
	return base
}

func researchUserPrompt(topic, domain, mode string, complexity int) string {
	modeInstruction := "Provide comprehensive, well-structured research on this new topic."
	if mode == "expand" {
		modeInstruction = "Existing research partially covers this topic. Build on and expand the existing material. Focus on gaps, recent developments, and deeper analysis."
	}

	return fmt.Sprintf(`Research Topic: %s
Domain: %s
Mode: %s
Complexity: %d/4

%s

Write a complete research document covering:
1. Overview and key concepts
2. Current state of the art
3. Practical implementation details
4. Common patterns and anti-patterns
5. Key resources and references`,
		topic, domain, mode, complexity, modeInstruction)
}

func sanitizeID(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, s)
	if len(s) > 50 {
		s = s[:50]
	}
	return strings.Trim(s, "-")
}
