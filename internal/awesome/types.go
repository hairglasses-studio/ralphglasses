package awesome

import "time"

// AwesomeEntry is one repo link from an awesome-list README.
type AwesomeEntry struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// Index is the parsed awesome-list: entries grouped by category.
type Index struct {
	Source     string         `json:"source"`
	FetchedAt time.Time      `json:"fetched_at"`
	Entries   []AwesomeEntry `json:"entries"`
}

// AnalysisEntry is a deep-analyzed repo with value/complexity rating.
type AnalysisEntry struct {
	AwesomeEntry
	Stars            int      `json:"stars"`
	Language         string   `json:"language,omitempty"`
	Features         []string `json:"features,omitempty"`
	CapabilityMatches int     `json:"capability_matches"`
	Rating           Rating   `json:"rating"`
	Complexity       string   `json:"complexity,omitempty"` // drop-in, moderate, complex
	Rationale        string   `json:"rationale,omitempty"`
}

// Rating is the value assessment for a repo.
type Rating string

const (
	RatingHigh   Rating = "HIGH"
	RatingMedium Rating = "MEDIUM"
	RatingLow    Rating = "LOW"
	RatingNone   Rating = "NONE"
)

// Analysis holds the full analysis of all entries.
type Analysis struct {
	Source    string          `json:"source"`
	Analyzed time.Time       `json:"analyzed"`
	Entries  []AnalysisEntry `json:"entries"`
	Summary  AnalysisSummary `json:"summary"`
}

// AnalysisSummary provides counts by rating.
type AnalysisSummary struct {
	Total  int `json:"total"`
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
	None   int `json:"none"`
}

// DiffResult shows new and removed entries between two fetches.
type DiffResult struct {
	New     []AwesomeEntry `json:"new"`
	Removed []AwesomeEntry `json:"removed"`
}

// Report is the final output document.
type Report struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Source      string          `json:"source"`
	Summary     AnalysisSummary `json:"summary"`
	High        []AnalysisEntry `json:"high"`
	Medium      []AnalysisEntry `json:"medium"`
	Low         []AnalysisEntry `json:"low"`
}
