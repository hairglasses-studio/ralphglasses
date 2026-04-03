// E2.2: Graph-Integrated Context Injection — queries the knowledge graph
// to find code entities relevant to a task, then formats them for injection
// into LLM prompts.
//
// Informed by Code Graph Model (ArXiv 2505.16901): graph-integrated context
// reduces search space from thousands to ~20 candidates.
package graph

import (
	"fmt"
	"sort"
	"strings"
)

// CodeChunk represents a piece of code context retrieved from the knowledge graph.
type CodeChunk struct {
	NodeID   string   `json:"node_id"`
	Kind     NodeKind `json:"kind"`
	Name     string   `json:"name"`
	Package  string   `json:"package"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Hops     int      `json:"hops"`     // graph distance from query match
	Score    float64  `json:"score"`    // relevance score (0-1)
	Related  []string `json:"related"`  // names of directly connected entities
}

// ContextInjector queries the knowledge graph to find relevant code context
// for a given task description.
type ContextInjector struct {
	graph *GraphStore
	query *QueryEngine
}

// NewContextInjector creates a context injector backed by a graph store.
func NewContextInjector(gs *GraphStore, qe *QueryEngine) *ContextInjector {
	return &ContextInjector{graph: gs, query: qe}
}

// RelevantContext finds code entities related to a task description.
// It extracts keywords, matches them against graph nodes, then traverses
// edges to find connected context (up to maxHops away).
//
// Returns at most maxChunks results, sorted by relevance score.
func (ci *ContextInjector) RelevantContext(taskDesc string, maxChunks, maxHops int) []CodeChunk {
	if ci.graph == nil || ci.query == nil {
		return nil
	}
	if maxChunks <= 0 {
		maxChunks = 20
	}
	if maxHops <= 0 {
		maxHops = 2
	}

	keywords := extractKeywords(taskDesc)
	if len(keywords) == 0 {
		return nil
	}

	// Phase 1: Find seed nodes matching keywords
	seeds := ci.findSeedNodes(keywords)
	if len(seeds) == 0 {
		return nil
	}

	// Phase 2: Expand via graph traversal (BFS up to maxHops)
	expanded := ci.expandNodes(seeds, maxHops)

	// Phase 3: Score and rank
	scored := ci.scoreChunks(expanded, keywords)

	// Phase 4: Deduplicate and limit
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	if len(scored) > maxChunks {
		scored = scored[:maxChunks]
	}

	return scored
}

// FormatForPrompt renders code chunks as a markdown context section
// suitable for injection into LLM prompts.
func FormatForPrompt(chunks []CodeChunk) string {
	if len(chunks) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Relevant Code Context\n\n")
	sb.WriteString(fmt.Sprintf("Found %d related code entities:\n\n", len(chunks)))

	for _, c := range chunks {
		sb.WriteString(fmt.Sprintf("- **%s** `%s` (%s)", c.Kind, c.Name, c.Package))
		if c.File != "" {
			sb.WriteString(fmt.Sprintf(" — `%s:%d`", c.File, c.Line))
		}
		if len(c.Related) > 0 {
			sb.WriteString(fmt.Sprintf(" → related: %s", strings.Join(c.Related, ", ")))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// findSeedNodes matches keywords against graph node names.
func (ci *ContextInjector) findSeedNodes(keywords []string) []seedNode {
	ci.graph.mu.RLock()
	defer ci.graph.mu.RUnlock()

	var seeds []seedNode
	seen := make(map[string]bool)

	for _, kw := range keywords {
		kwLower := strings.ToLower(kw)
		for id, node := range ci.graph.nodes {
			if seen[id] {
				continue
			}
			nameLower := strings.ToLower(node.Name)
			score := 0.0

			// Exact match
			if nameLower == kwLower {
				score = 1.0
			} else if strings.Contains(nameLower, kwLower) {
				score = 0.7
			} else if strings.Contains(kwLower, nameLower) && len(nameLower) > 3 {
				score = 0.4
			}

			if score > 0 {
				// Boost by node kind (types/functions more relevant than fields)
				switch node.Kind {
				case KindFunction, KindMethod:
					score *= 1.2
				case KindType, KindInterface:
					score *= 1.1
				case KindPackage:
					score *= 0.8
				}
				if score > 1.0 {
					score = 1.0
				}

				seeds = append(seeds, seedNode{id: id, score: score})
				seen[id] = true
			}
		}
	}

	return seeds
}

type seedNode struct {
	id    string
	score float64
}

// expandNodes performs BFS from seed nodes up to maxHops.
func (ci *ContextInjector) expandNodes(seeds []seedNode, maxHops int) map[string]*expandedNode {
	ci.graph.mu.RLock()
	defer ci.graph.mu.RUnlock()

	result := make(map[string]*expandedNode)

	// Initialize with seeds
	type queueItem struct {
		id   string
		hops int
	}
	var queue []queueItem

	for _, s := range seeds {
		result[s.id] = &expandedNode{id: s.id, hops: 0, seedScore: s.score}
		queue = append(queue, queueItem{id: s.id, hops: 0})
	}

	// BFS expansion
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.hops >= maxHops {
			continue
		}

		// Follow outgoing edges
		for _, edge := range ci.graph.edges[item.id] {
			if _, exists := result[edge.To]; !exists {
				result[edge.To] = &expandedNode{
					id:        edge.To,
					hops:      item.hops + 1,
					seedScore: result[item.id].seedScore * 0.6, // decay with distance
					edgeKind:  edge.Kind,
				}
				queue = append(queue, queueItem{id: edge.To, hops: item.hops + 1})
			}
		}

		// Follow incoming edges (reverse)
		for _, edge := range ci.graph.rev[item.id] {
			if _, exists := result[edge.From]; !exists {
				result[edge.From] = &expandedNode{
					id:        edge.From,
					hops:      item.hops + 1,
					seedScore: result[item.id].seedScore * 0.5, // slightly less for reverse
					edgeKind:  edge.Kind,
				}
				queue = append(queue, queueItem{id: edge.From, hops: item.hops + 1})
			}
		}
	}

	return result
}

type expandedNode struct {
	id        string
	hops      int
	seedScore float64
	edgeKind  EdgeKind
}

// scoreChunks converts expanded nodes into scored CodeChunks.
func (ci *ContextInjector) scoreChunks(expanded map[string]*expandedNode, keywords []string) []CodeChunk {
	ci.graph.mu.RLock()
	defer ci.graph.mu.RUnlock()

	var chunks []CodeChunk

	for id, en := range expanded {
		node, ok := ci.graph.nodes[id]
		if !ok {
			continue
		}

		// Collect related node names
		var related []string
		for _, edge := range ci.graph.edges[id] {
			if toNode, ok := ci.graph.nodes[edge.To]; ok {
				related = append(related, toNode.Name)
			}
		}
		if len(related) > 5 {
			related = related[:5]
		}

		chunks = append(chunks, CodeChunk{
			NodeID:  id,
			Kind:    node.Kind,
			Name:    node.Name,
			Package: node.Package,
			File:    node.File,
			Line:    node.Line,
			Hops:    en.hops,
			Score:   en.seedScore,
			Related: related,
		})
	}

	return chunks
}

// extractKeywords splits a task description into searchable terms.
func extractKeywords(desc string) []string {
	// Split on whitespace and common delimiters
	words := strings.FieldsFunc(desc, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == ':' || r == ';' ||
			r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}'
	})

	// Filter: keep words > 3 chars, skip stop words
	stopWords := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "that": true,
		"this": true, "from": true, "have": true, "been": true, "will": true,
		"should": true, "would": true, "could": true, "into": true, "about": true,
		"when": true, "then": true, "than": true, "also": true, "which": true,
	}

	var keywords []string
	seen := make(map[string]bool)
	for _, w := range words {
		w = strings.ToLower(strings.Trim(w, "`\"'"))
		if len(w) <= 3 || stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		keywords = append(keywords, w)
	}

	return keywords
}
