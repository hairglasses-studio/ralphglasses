package awesome

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// SyncResult is the output of a full sync pipeline.
type SyncResult struct {
	Source    string          `json:"source"`
	Fetched   int            `json:"fetched"`
	New       int            `json:"new"`
	Removed   int            `json:"removed"`
	Analyzed  int            `json:"analyzed"`
	Summary   AnalysisSummary `json:"summary"`
	SavedTo   string          `json:"saved_to,omitempty"`
	ReportPath string         `json:"report_path,omitempty"`
}

// SyncOptions controls the sync pipeline.
type SyncOptions struct {
	Repo       string // awesome-list repo (default: hesreallyhim/awesome-claude-code)
	SaveTo     string // repo path to save results (empty = don't save)
	FullRescan bool   // re-analyze all entries, not just new ones
	MaxWorkers int    // concurrent fetches (default 5)
}

// Sync runs the full pipeline: fetch → diff → analyze → report → save.
func Sync(ctx context.Context, client *http.Client, opts SyncOptions) (*SyncResult, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if opts.Repo == "" {
		opts.Repo = DefaultSource
	}

	// Step 1: Fetch
	idx, err := Fetch(ctx, client, opts.Repo)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	result := &SyncResult{
		Source:  opts.Repo,
		Fetched: len(idx.Entries),
	}

	// Step 2: Diff against previous
	var prev *Index
	if opts.SaveTo != "" {
		prev, _ = LoadIndex(opts.SaveTo) // OK if not found
	}

	diff := Diff(prev, idx)
	result.New = len(diff.New)
	result.Removed = len(diff.Removed)

	// Step 3: Determine what to analyze
	toAnalyze := diff.New
	if opts.FullRescan || prev == nil {
		toAnalyze = idx.Entries
	}

	// Step 4: Analyze
	var analysis *Analysis
	if len(toAnalyze) > 0 {
		analysis, err = Analyze(ctx, client, toAnalyze, AnalyzeOptions{
			MaxWorkers: opts.MaxWorkers,
		})
		if err != nil {
			return nil, fmt.Errorf("analyze: %w", err)
		}
		analysis.Source = opts.Repo

		// Merge with existing analysis if incremental
		if !opts.FullRescan && prev != nil && opts.SaveTo != "" {
			existing, _ := LoadAnalysis(opts.SaveTo)
			if existing != nil {
				analysis = mergeAnalysis(existing, analysis)
			}
		}

		result.Analyzed = len(toAnalyze)
		result.Summary = analysis.Summary
	} else if opts.SaveTo != "" {
		// No new entries, load existing analysis
		analysis, _ = LoadAnalysis(opts.SaveTo)
		if analysis != nil {
			result.Summary = analysis.Summary
		}
	}

	// Step 5: Save
	if opts.SaveTo != "" {
		if err := SaveIndex(opts.SaveTo, idx); err != nil {
			return nil, fmt.Errorf("save index: %w", err)
		}
		result.SavedTo = StorePath(opts.SaveTo)

		if analysis != nil {
			if err := SaveAnalysis(opts.SaveTo, analysis); err != nil {
				return nil, fmt.Errorf("save analysis: %w", err)
			}

			// Generate and save report
			report := GenerateReport(analysis)
			md := FormatMarkdown(report)
			if err := SaveReport(opts.SaveTo, md); err != nil {
				return nil, fmt.Errorf("save report: %w", err)
			}
			result.ReportPath = StorePath(opts.SaveTo) + "/report.md"
		}
	}

	return result, nil
}

// mergeAnalysis combines existing and new analysis, preferring new entries.
func mergeAnalysis(existing, new *Analysis) *Analysis {
	byURL := make(map[string]AnalysisEntry, len(existing.Entries))
	for _, e := range existing.Entries {
		byURL[e.URL] = e
	}
	for _, e := range new.Entries {
		byURL[e.URL] = e // overwrite with new
	}

	merged := &Analysis{
		Source:   new.Source,
		Analyzed: new.Analyzed,
	}
	for _, e := range byURL {
		merged.Entries = append(merged.Entries, e)
		merged.Summary.Total++
		switch e.Rating {
		case RatingHigh:
			merged.Summary.High++
		case RatingMedium:
			merged.Summary.Medium++
		case RatingLow:
			merged.Summary.Low++
		case RatingNone:
			merged.Summary.None++
		}
	}

	return merged
}
