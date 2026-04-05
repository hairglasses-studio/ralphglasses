package scoring

import (
	"strings"
)

// slopPatterns are common AI-generated text patterns that signal low-quality boilerplate.
var slopPatterns = []string{
	// Greetings / openers
	"i hope this email finds you well",
	"i hope this message finds you well",
	"i hope this finds you well",
	"i hope you are doing well",
	"i hope you're doing well",
	"i wanted to reach out",
	"i'm reaching out to",
	"just wanted to touch base",
	"just checking in",
	"i trust this message",
	"greetings!",

	// Closings / sign-offs
	"please don't hesitate to reach out",
	"please do not hesitate to contact",
	"please feel free to reach out",
	"don't hesitate to let me know",
	"feel free to reach out",
	"looking forward to hearing from you",
	"i look forward to your response",
	"please let me know if you have any questions",
	"let me know if there's anything else",
	"warm regards",
	"best regards",

	// Filler / padding
	"in today's fast-paced world",
	"in this day and age",
	"it's worth noting that",
	"it is important to note that",
	"it goes without saying",
	"as you may know",
	"as we all know",
	"at the end of the day",
	"having said that",
	"with that being said",
	"that said",
	"needless to say",

	// Overclaiming / hyperbole
	"game-changer",
	"game changer",
	"cutting-edge",
	"cutting edge",
	"state-of-the-art",
	"best-in-class",
	"world-class",
	"industry-leading",
	"thought leader",
	"paradigm shift",
	"move the needle",
	"leverage synergies",
	"circle back",
	"low-hanging fruit",
	"deep dive",
	"take it to the next level",
	"unlock the full potential",

	// Passive / hedge language
	"i believe that",
	"i think that",
	"it seems like",
	"it appears that",
	"arguably",
	"it could be argued",
	"one might say",
	"some might argue",

	// AI-specific tells
	"as an ai",
	"as a language model",
	"i'd be happy to help",
	"i'd be glad to help",
	"absolutely! here",
	"great question!",
	"that's a great question",
	"certainly! let me",
	"of course! i'd be",
	"sure thing!",
	"delve into",
	"delve deeper",
	"let's delve",
	"tapestry of",
	"rich tapestry",
	"vibrant tapestry",
	"landscape of",
	"navigate the",
	"navigate this",
	"foster a",
	"foster an",
	"foster the",
	"holistic approach",
	"multifaceted",
	"it's important to remember that",
	"it's crucial to",
	"it's essential to",
	"in conclusion",
	"to summarize",
	"in summary",
	"ultimately",
	"moreover",
	"furthermore",
	"additionally",
	"consequently",
	"henceforth",
	"notwithstanding",
	"nevertheless",
	"commendable",
	"underscores",
	"underscoring",
	"pivotal",
	"groundbreaking",
	"robust",
	"seamless",
	"seamlessly",
	"streamline",
	"streamlined",
	"empower",
	"empowering",
	"leverage",
	"leveraging",
	"revolutionize",
	"revolutionizing",
	"transformative",
	"innovative",
	"impactful",
}

// ScoreSlop returns a slop score (0-100, higher = more slop) and matched patterns.
func ScoreSlop(text string) (float64, []string) {
	lower := strings.ToLower(text)
	wordCount := len(strings.Fields(text))
	if wordCount == 0 {
		return 0, nil
	}

	var matches []string
	seen := make(map[string]bool)

	for _, pattern := range slopPatterns {
		if strings.Contains(lower, pattern) && !seen[pattern] {
			matches = append(matches, pattern)
			seen[pattern] = true
		}
	}

	if len(matches) == 0 {
		return 0, nil
	}

	// Score based on slop density (matches per 100 words)
	density := float64(len(matches)) / float64(wordCount) * 100
	score := density * 20 // 5 matches per 100 words = 100

	return clamp(score, 0, 100), matches
}
