package fewshot

import (
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

// stopWords are common words filtered from BM25 scoring.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "being": true, "have": true,
	"has": true, "had": true, "do": true, "does": true, "did": true,
	"will": true, "would": true, "could": true, "should": true, "may": true,
	"might": true, "shall": true, "can": true, "to": true, "of": true,
	"in": true, "for": true, "on": true, "with": true, "at": true, "by": true,
	"from": true, "as": true, "into": true, "through": true, "during": true,
	"it": true, "its": true, "this": true, "that": true, "these": true,
	"those": true, "i": true, "you": true, "he": true, "she": true, "we": true,
	"they": true, "me": true, "him": true, "her": true, "us": true, "them": true,
	"and": true, "or": true, "but": true, "not": true, "if": true, "then": true,
}

// Tokenize splits text into lowercase tokens, filtering stop words and short tokens.
func Tokenize(text string) []string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
	var tokens []string
	for _, w := range words {
		if len(w) < 2 || stopWords[w] {
			continue
		}
		tokens = append(tokens, w)
	}
	return tokens
}

// IDFTable stores pre-computed inverse document frequency values.
type IDFTable struct {
	idf    map[string]float64
	docCount int
}

// NewIDFTable builds an IDF table from a corpus of entries.
func NewIDFTable(entries []PromptEntry) *IDFTable {
	N := len(entries)
	if N == 0 {
		return &IDFTable{idf: map[string]float64{}, docCount: 0}
	}
	df := make(map[string]int) // document frequency per term
	for _, e := range entries {
		seen := make(map[string]bool)
		for _, tok := range Tokenize(e.Prompt) {
			if !seen[tok] {
				df[tok]++
				seen[tok] = true
			}
		}
	}
	idf := make(map[string]float64, len(df))
	for term, freq := range df {
		idf[term] = math.Log((float64(N)-float64(freq)+0.5) / (float64(freq) + 0.5))
	}
	return &IDFTable{idf: idf, docCount: N}
}

// BM25Score computes a BM25-lite score for a query against a document.
func (t *IDFTable) BM25Score(queryTokens []string, docTokens []string) float64 {
	if len(queryTokens) == 0 || len(docTokens) == 0 {
		return 0
	}
	// Term frequency in document
	tf := make(map[string]int)
	for _, tok := range docTokens {
		tf[tok]++
	}
	docLen := float64(len(docTokens))
	var score float64
	for _, qt := range queryTokens {
		idf := t.idf[qt]
		if idf <= 0 {
			continue
		}
		freq := float64(tf[qt])
		score += idf * (freq / docLen)
	}
	return score
}

// TaskTypeScore returns a similarity score for two task types.
func TaskTypeScore(query, candidate enhancer.TaskType) float64 {
	if query == candidate {
		return 1.0
	}
	// Same-family pairs get partial credit
	families := map[enhancer.TaskType]string{
		enhancer.TaskTypeCode:            "implementation",
		enhancer.TaskTypeAnalysis:        "review",
		enhancer.TaskTypeTroubleshooting: "review",
		enhancer.TaskTypeWorkflow:        "implementation",
		enhancer.TaskTypeCreative:        "creative",
		enhancer.TaskTypeGeneral:         "general",
	}
	if families[query] == families[candidate] && families[query] != "general" {
		return 0.5
	}
	return 0.0
}

// JaccardSimilarity computes |A ∩ B| / |A ∪ B| for two tag sets.
func JaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	setA := make(map[string]bool, len(a))
	for _, t := range a {
		setA[t] = true
	}
	setB := make(map[string]bool, len(b))
	for _, t := range b {
		setB[t] = true
	}
	var intersect, union int
	all := make(map[string]bool)
	for k := range setA {
		all[k] = true
	}
	for k := range setB {
		all[k] = true
	}
	union = len(all)
	for k := range all {
		if setA[k] && setB[k] {
			intersect++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

// RepoContextScore returns 1.0 for same repo, 0.5 for same tier, 0.0 otherwise.
func RepoContextScore(queryRepo, candidateRepo string) float64 {
	if queryRepo == candidateRepo {
		return 1.0
	}
	// Same tier = both are Go repos in hairglasses-studio
	if queryRepo != "" && candidateRepo != "" {
		return 0.3 // some context overlap
	}
	return 0.0
}

// RecencyDecay returns exp(-age_days / 30) for exponential decay.
func RecencyDecay(candidateTime time.Time) float64 {
	ageDays := time.Since(candidateTime).Hours() / 24
	if ageDays < 0 {
		ageDays = 0
	}
	return math.Exp(-ageDays / 30)
}

// CompositeScore computes the weighted similarity between a query and candidate.
func CompositeScore(query Query, candidate PromptEntry, idf *IDFTable, w SimilarityWeights) float64 {
	taskScore := TaskTypeScore(query.TaskType, enhancer.TaskType(candidate.TaskType))
	tagScore := JaccardSimilarity(query.Tags, candidate.Tags)

	candidateTokens := Tokenize(candidate.Prompt)
	keywordScore := 0.0
	if idf != nil {
		keywordScore = idf.BM25Score(query.Keywords, candidateTokens)
		// Normalize to 0-1 range (BM25 can exceed 1.0)
		if keywordScore > 1.0 {
			keywordScore = 1.0
		}
	}

	repoScore := RepoContextScore(query.Repo, candidate.Repo)
	recencyScore := RecencyDecay(candidate.Timestamp)

	return w.TaskType*taskScore + w.TagOverlap*tagScore + w.Keyword*keywordScore +
		w.Repo*repoScore + w.Recency*recencyScore
}

// MMRRerank applies Maximal Marginal Relevance to select K diverse examples.
// lambda controls the relevance/diversity tradeoff (higher = more relevant, less diverse).
func MMRRerank(candidates []scoredCandidate, k int, lambda float64) []scoredCandidate {
	if len(candidates) <= k {
		return candidates
	}

	selected := make([]scoredCandidate, 0, k)
	remaining := make([]scoredCandidate, len(candidates))
	copy(remaining, candidates)

	for len(selected) < k && len(remaining) > 0 {
		bestIdx := 0
		bestMMR := math.Inf(-1)

		for i, cand := range remaining {
			// Max similarity to any already-selected example
			maxSim := 0.0
			for _, sel := range selected {
				sim := JaccardSimilarity(cand.entry.Tags, sel.entry.Tags)
				if sim > maxSim {
					maxSim = sim
				}
			}
			mmr := lambda*cand.score - (1-lambda)*maxSim
			if mmr > bestMMR {
				bestMMR = mmr
				bestIdx = i
			}
		}

		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
}

type scoredCandidate struct {
	entry PromptEntry
	score float64
}
