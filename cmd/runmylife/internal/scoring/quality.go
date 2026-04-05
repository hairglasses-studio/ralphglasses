// Package scoring provides text quality analysis for communications.
package scoring

import (
	"fmt"
	"math"
	"strings"
	"unicode"
)

// QualityScore holds the multi-dimensional quality assessment of a text.
type QualityScore struct {
	Overall         float64            `json:"overall"`          // 0-100
	Clarity         float64            `json:"clarity"`          // 0-100
	Conciseness     float64            `json:"conciseness"`      // 0-100
	Professionalism float64            `json:"professionalism"`  // 0-100
	Specificity     float64            `json:"specificity"`      // 0-100
	Engagement      float64            `json:"engagement"`       // 0-100
	SlopScore       float64            `json:"slop_score"`       // 0-100 (higher = more slop)
	SlopMatches     []string           `json:"slop_matches"`     // matched slop patterns
	Suggestions     []string           `json:"suggestions"`      // improvement hints
	Details         map[string]float64 `json:"details"`          // sub-metrics
}

// ScoreText analyzes text quality across multiple dimensions.
func ScoreText(text string) *QualityScore {
	score := &QualityScore{
		Details: make(map[string]float64),
	}

	if strings.TrimSpace(text) == "" {
		return score
	}

	words := strings.Fields(text)
	sentences := countSentences(text)
	wordCount := len(words)

	// Clarity: sentence length, readability
	score.Clarity = scoreClarity(words, sentences, wordCount)
	score.Details["avg_sentence_length"] = float64(wordCount) / math.Max(float64(sentences), 1)

	// Conciseness: penalize filler, excessive length
	score.Conciseness = scoreConciseness(text, words, wordCount)

	// Professionalism: grammar signals, tone
	score.Professionalism = scoreProfessionalism(text, words)

	// Specificity: concrete vs vague language
	score.Specificity = scoreSpecificity(text, words)

	// Engagement: questions, active voice signals
	score.Engagement = scoreEngagement(text, sentences)

	// Slop detection
	slopScore, matches := ScoreSlop(text)
	score.SlopScore = slopScore
	score.SlopMatches = matches

	// Overall: weighted average with slop penalty
	score.Overall = (score.Clarity*0.25 + score.Conciseness*0.20 + score.Professionalism*0.20 +
		score.Specificity*0.20 + score.Engagement*0.15) * (1 - slopScore/200)

	score.Overall = clamp(score.Overall, 0, 100)
	score.Suggestions = generateSuggestions(score, wordCount, sentences)
	score.Details["word_count"] = float64(wordCount)
	score.Details["sentence_count"] = float64(sentences)

	return score
}

// FormatScore returns a markdown-formatted quality report.
func FormatScore(score *QualityScore) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**Overall: %.0f/100**\n\n", score.Overall))
	b.WriteString(fmt.Sprintf("| Dimension | Score |\n|-----------|-------|\n"))
	b.WriteString(fmt.Sprintf("| Clarity | %.0f |\n", score.Clarity))
	b.WriteString(fmt.Sprintf("| Conciseness | %.0f |\n", score.Conciseness))
	b.WriteString(fmt.Sprintf("| Professionalism | %.0f |\n", score.Professionalism))
	b.WriteString(fmt.Sprintf("| Specificity | %.0f |\n", score.Specificity))
	b.WriteString(fmt.Sprintf("| Engagement | %.0f |\n", score.Engagement))
	b.WriteString(fmt.Sprintf("| AI Slop | %.0f |\n", score.SlopScore))

	if len(score.SlopMatches) > 0 {
		b.WriteString("\n**Slop Patterns Detected:**\n")
		for _, m := range score.SlopMatches {
			b.WriteString(fmt.Sprintf("- %q\n", m))
		}
	}

	if len(score.Suggestions) > 0 {
		b.WriteString("\n**Suggestions:**\n")
		for _, s := range score.Suggestions {
			b.WriteString(fmt.Sprintf("- %s\n", s))
		}
	}

	return b.String()
}

func scoreClarity(words []string, sentences, wordCount int) float64 {
	if wordCount == 0 {
		return 0
	}
	avgSentLen := float64(wordCount) / math.Max(float64(sentences), 1)
	// Ideal: 12-20 words per sentence
	score := 100.0
	if avgSentLen > 25 {
		score -= (avgSentLen - 25) * 3
	} else if avgSentLen < 5 && sentences > 1 {
		score -= (5 - avgSentLen) * 5
	}

	// Penalize very long words (jargon/complexity)
	longWords := 0
	for _, w := range words {
		if len(w) > 12 {
			longWords++
		}
	}
	longWordRatio := float64(longWords) / float64(wordCount)
	score -= longWordRatio * 50

	return clamp(score, 0, 100)
}

func scoreConciseness(text string, words []string, wordCount int) float64 {
	score := 100.0

	// Penalize filler phrases
	fillers := []string{
		"in order to", "at the end of the day", "it goes without saying",
		"as a matter of fact", "for what it's worth", "at this point in time",
		"in terms of", "with regard to", "due to the fact that",
		"in the event that", "on the other hand", "as previously mentioned",
		"it should be noted that", "needless to say",
	}
	lower := strings.ToLower(text)
	fillerCount := 0
	for _, f := range fillers {
		fillerCount += strings.Count(lower, f)
	}
	score -= float64(fillerCount) * 10

	// Penalize excessive length (>300 words for a message)
	if wordCount > 300 {
		score -= float64(wordCount-300) / 10
	}

	// Penalize repeated words (excluding common ones)
	score -= penalizeRepetition(words) * 5

	return clamp(score, 0, 100)
}

func scoreProfessionalism(text string, words []string) float64 {
	score := 85.0 // start neutral-positive

	lower := strings.ToLower(text)

	// Penalize all-caps words (shouting)
	capsCount := 0
	for _, w := range words {
		if len(w) > 2 && w == strings.ToUpper(w) && containsLetter(w) {
			capsCount++
		}
	}
	score -= float64(capsCount) * 5

	// Penalize excessive exclamation marks
	exclamations := strings.Count(text, "!")
	if exclamations > 2 {
		score -= float64(exclamations-2) * 5
	}

	// Penalize text speak
	textSpeak := []string{" u ", " ur ", " thx ", " pls ", " tbh ", " imo ", " idk "}
	for _, t := range textSpeak {
		if strings.Contains(" "+lower+" ", t) {
			score -= 8
		}
	}

	// Reward greeting/closing
	if strings.Contains(lower, "thank") || strings.Contains(lower, "regards") || strings.Contains(lower, "best,") {
		score += 5
	}

	return clamp(score, 0, 100)
}

func scoreSpecificity(text string, words []string) float64 {
	score := 70.0

	lower := strings.ToLower(text)

	// Penalize vague language
	vagueWords := []string{
		"things", "stuff", "something", "somehow", "somewhat",
		"various", "several", "many", "some kind of", "sort of",
		"kind of", "pretty much", "basically", "literally",
	}
	vagueCount := 0
	for _, v := range vagueWords {
		vagueCount += strings.Count(lower, v)
	}
	score -= float64(vagueCount) * 8

	// Reward numbers, dates, specific references
	numberCount := 0
	for _, w := range words {
		for _, r := range w {
			if unicode.IsDigit(r) {
				numberCount++
				break
			}
		}
	}
	score += float64(numberCount) * 3

	// Reward proper nouns (capitalized mid-sentence words)
	properNouns := 0
	for i, w := range words {
		if i > 0 && len(w) > 1 && unicode.IsUpper(rune(w[0])) {
			properNouns++
		}
	}
	score += float64(properNouns) * 2

	return clamp(score, 0, 100)
}

func scoreEngagement(text string, sentences int) float64 {
	score := 60.0

	// Reward questions
	questions := strings.Count(text, "?")
	score += float64(questions) * 8

	// Reward direct address
	lower := strings.ToLower(text)
	if strings.Contains(lower, "you") || strings.Contains(lower, "your") {
		score += 10
	}

	// Reward action items / calls to action
	actionPhrases := []string{
		"let me know", "please", "could you", "would you", "can you",
		"next step", "action item", "follow up",
	}
	for _, a := range actionPhrases {
		if strings.Contains(lower, a) {
			score += 5
		}
	}

	return clamp(score, 0, 100)
}

func generateSuggestions(score *QualityScore, wordCount, sentences int) []string {
	var suggestions []string
	avgSentLen := float64(wordCount) / math.Max(float64(sentences), 1)

	if avgSentLen > 25 {
		suggestions = append(suggestions, "Break up long sentences for clarity")
	}
	if score.Conciseness < 60 {
		suggestions = append(suggestions, "Remove filler phrases and tighten language")
	}
	if score.SlopScore > 30 {
		suggestions = append(suggestions, "Reduce AI-generated boilerplate phrasing")
	}
	if score.Specificity < 50 {
		suggestions = append(suggestions, "Add concrete details, numbers, or specific references")
	}
	if score.Engagement < 50 {
		suggestions = append(suggestions, "Add a question or call-to-action to drive response")
	}
	if wordCount > 500 {
		suggestions = append(suggestions, "Consider shortening — messages over 500 words lose reader attention")
	}
	return suggestions
}

func countSentences(text string) int {
	count := 0
	for _, r := range text {
		if r == '.' || r == '!' || r == '?' {
			count++
		}
	}
	if count == 0 && len(strings.TrimSpace(text)) > 0 {
		count = 1
	}
	return count
}

func penalizeRepetition(words []string) float64 {
	if len(words) < 10 {
		return 0
	}
	common := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
		"were": true, "be": true, "been": true, "to": true, "of": true, "and": true,
		"in": true, "that": true, "it": true, "for": true, "on": true, "with": true,
		"as": true, "at": true, "by": true, "from": true, "or": true, "this": true,
		"i": true, "we": true, "you": true, "they": true, "he": true, "she": true,
	}
	counts := make(map[string]int)
	for _, w := range words {
		lower := strings.ToLower(w)
		if !common[lower] && len(lower) > 3 {
			counts[lower]++
		}
	}
	penalty := 0.0
	for _, c := range counts {
		if c > 3 {
			penalty += float64(c - 3)
		}
	}
	return penalty
}

func containsLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
