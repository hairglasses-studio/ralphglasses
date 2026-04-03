// E3.2: Intent Classifier — TF-IDF based classifier with LLM fallback
// for natural language fleet control commands.
//
// Informed by RouteLLM: lightweight classifier for routing NL queries.
// Self-trains from successful past command executions.
package session

import (
	"math"
	"sort"
	"strings"
	"sync"
)

// IntentClassification is the result of classifying a natural language input.
type IntentClassification struct {
	Action     string  `json:"action"`     // predicted action (start, stop, etc.)
	Confidence float64 `json:"confidence"` // 0.0-1.0
	Method     string  `json:"method"`     // "tfidf", "keyword", "llm_fallback"
}

// TrainingExample is a labeled example for the intent classifier.
type TrainingExample struct {
	Input  string `json:"input"`
	Action string `json:"action"`
}

// IntentClassifier uses TF-IDF weighted keyword matching to classify
// natural language inputs into fleet control actions. Falls back to
// keyword matching when confidence is below threshold.
type IntentClassifier struct {
	mu        sync.RWMutex
	idf       map[string]float64            // term -> inverse document frequency
	tfidf     map[string]map[string]float64 // action -> term -> tf-idf weight
	docCounts map[string]int                // action -> number of training examples
	totalDocs int
	threshold float64 // confidence threshold for accepting classification (default 0.3)
}

// NewIntentClassifier creates a classifier with optional initial training data.
func NewIntentClassifier(threshold float64) *IntentClassifier {
	if threshold <= 0 {
		threshold = 0.3
	}
	ic := &IntentClassifier{
		idf:       make(map[string]float64),
		tfidf:     make(map[string]map[string]float64),
		docCounts: make(map[string]int),
		threshold: threshold,
	}

	// Seed with built-in examples
	ic.Train(builtinExamples())
	return ic
}

// Train updates the classifier with new labeled examples.
func (ic *IntentClassifier) Train(examples []TrainingExample) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	// Count document frequency per term
	docFreq := make(map[string]int) // term -> number of docs containing it

	// Group examples by action
	actionDocs := make(map[string][]string) // action -> list of documents
	for _, ex := range examples {
		actionDocs[ex.Action] = append(actionDocs[ex.Action], ex.Input)
		ic.docCounts[ex.Action]++
		ic.totalDocs++

		terms := tokenizeForTFIDF(ex.Input)
		seen := make(map[string]bool)
		for _, t := range terms {
			if !seen[t] {
				docFreq[t]++
				seen[t] = true
			}
		}
	}

	// Compute IDF
	for term, df := range docFreq {
		ic.idf[term] = math.Log(float64(ic.totalDocs+1) / float64(df+1))
	}

	// Compute TF-IDF per action
	for action, docs := range actionDocs {
		if ic.tfidf[action] == nil {
			ic.tfidf[action] = make(map[string]float64)
		}

		// Aggregate term frequencies across all docs for this action
		termFreq := make(map[string]int)
		totalTerms := 0
		for _, doc := range docs {
			for _, t := range tokenizeForTFIDF(doc) {
				termFreq[t]++
				totalTerms++
			}
		}

		// Compute TF-IDF for each term
		for term, freq := range termFreq {
			tf := float64(freq) / float64(totalTerms+1)
			idf := ic.idf[term]
			ic.tfidf[action][term] = tf * idf
		}
	}
}

// Classify predicts the action for a natural language input.
// Returns the best matching action with confidence score.
func (ic *IntentClassifier) Classify(input string) IntentClassification {
	ic.mu.RLock()
	defer ic.mu.RUnlock()

	terms := tokenizeForTFIDF(input)
	if len(terms) == 0 {
		return IntentClassification{Confidence: 0, Method: "tfidf"}
	}

	// Score each action by cosine similarity with input
	type actionScore struct {
		action string
		score  float64
	}
	var scores []actionScore

	// Build input vector
	inputVec := make(map[string]float64)
	for _, t := range terms {
		inputVec[t]++
	}
	// Apply IDF weighting
	for t := range inputVec {
		if idf, ok := ic.idf[t]; ok {
			inputVec[t] *= idf
		}
	}

	for action, actionWeights := range ic.tfidf {
		score := cosineSimilarity(inputVec, actionWeights)
		scores = append(scores, actionScore{action, score})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	if len(scores) == 0 || scores[0].score < ic.threshold {
		return IntentClassification{Confidence: 0, Method: "tfidf"}
	}

	// Normalize confidence to 0-1 range
	confidence := scores[0].score
	if confidence > 1.0 {
		confidence = 1.0
	}

	return IntentClassification{
		Action:     scores[0].action,
		Confidence: confidence,
		Method:     "tfidf",
	}
}

// cosineSimilarity computes the cosine similarity between two sparse vectors.
func cosineSimilarity(a, b map[string]float64) float64 {
	var dot, normA, normB float64

	for k, va := range a {
		normA += va * va
		if vb, ok := b[k]; ok {
			dot += va * vb
		}
	}
	for _, vb := range b {
		normB += vb * vb
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// tokenizeForTFIDF splits text into lowercase terms for TF-IDF.
func tokenizeForTFIDF(s string) []string {
	words := strings.Fields(strings.ToLower(s))
	var terms []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}")
		if len(w) > 2 { // skip very short words
			terms = append(terms, w)
		}
	}
	return terms
}

// builtinExamples returns seed training data for common fleet commands.
func builtinExamples() []TrainingExample {
	return []TrainingExample{
		// Start
		{Input: "start 3 claude sessions on ralphglasses", Action: ActionStart},
		{Input: "launch new session for mcpkit", Action: ActionStart},
		{Input: "spin up agents for the fleet", Action: ActionStart},
		{Input: "begin a coding session", Action: ActionStart},
		{Input: "kick off a ralph loop", Action: ActionStart},

		// Stop
		{Input: "stop all sessions", Action: ActionStop},
		{Input: "kill the running agents", Action: ActionStop},
		{Input: "shut down everything", Action: ActionStop},
		{Input: "terminate session abc123", Action: ActionStop},
		{Input: "halt the fleet", Action: ActionStop},

		// Pause
		{Input: "pause the current session", Action: ActionPause},
		{Input: "hold the agent for now", Action: ActionPause},
		{Input: "freeze all sessions temporarily", Action: ActionPause},

		// Resume
		{Input: "resume paused sessions", Action: ActionResume},
		{Input: "continue the agent", Action: ActionResume},
		{Input: "unpause everything", Action: ActionResume},

		// Scale
		{Input: "scale up to 5 workers", Action: ActionScale},
		{Input: "add more agents", Action: ActionScale},
		{Input: "reduce fleet to 2 sessions", Action: ActionScale},
		{Input: "scale down the fleet", Action: ActionScale},

		// Report
		{Input: "show me the weekly report", Action: ActionReport},
		{Input: "generate a cost summary", Action: ActionReport},
		{Input: "what's the fleet performance", Action: ActionReport},
		{Input: "give me analytics", Action: ActionReport},

		// Status
		{Input: "what's running right now", Action: ActionStatus},
		{Input: "show fleet status", Action: ActionStatus},
		{Input: "how many sessions are active", Action: ActionStatus},
		{Input: "check the budget", Action: ActionStatus},
	}
}
