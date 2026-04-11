package parity

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

type ValidateResult struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Issues []string `json:"issues,omitempty"`
	Path   string   `json:"path,omitempty"`
}

type ValidateOptions struct {
	ScanPath     string
	Repo         string
	Repos        []string
	IncludeClean bool
	Strict       bool
}

func ValidateRepos(ctx context.Context, opts ValidateOptions) ([]ValidateResult, error) {
	repos, err := discovery.Scan(ctx, opts.ScanPath)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	repoFilter := make(map[string]bool)
	if opts.Repo != "" {
		repoFilter[opts.Repo] = true
	}
	for _, repo := range opts.Repos {
		repo = strings.TrimSpace(repo)
		if repo != "" {
			repoFilter[repo] = true
		}
	}

	results := make([]ValidateResult, 0, len(repos))
	for _, repo := range repos {
		if len(repoFilter) > 0 && !repoFilter[repo.Name] {
			continue
		}
		if !repo.HasRC {
			continue
		}
		cfg, err := model.LoadConfig(ctx, repo.Path)
		if err != nil {
			results = append(results, ValidateResult{
				Name:   repo.Name,
				Path:   repo.Path,
				Status: "ERROR",
				Issues: []string{"cannot read .ralphrc: " + err.Error()},
			})
			continue
		}

		issues := ValidateConfig(cfg)
		status := "OK"
		if len(issues) > 0 {
			hasError := false
			for _, issue := range issues {
				if strings.HasPrefix(issue, "ERROR") {
					hasError = true
					break
				}
			}
			switch {
			case hasError:
				status = "ERROR"
			case opts.Strict:
				status = "ERROR"
			default:
				status = "WARN"
			}
		}
		if status == "OK" && !opts.IncludeClean {
			continue
		}
		results = append(results, ValidateResult{
			Name:   repo.Name,
			Path:   repo.Path,
			Status: status,
			Issues: issues,
		})
	}
	return results, nil
}

func ValidateConfig(cfg *model.RalphConfig) []string {
	var issues []string

	if cfg.Get("PROJECT_NAME", "") == "" {
		issues = append(issues, "ERROR: PROJECT_NAME is required but not set")
	}

	if v := cfg.Get("PROVIDER", ""); v != "" {
		allowed := []string{"claude", "gemini", "codex", "openai"}
		if !slices.Contains(allowed, strings.ToLower(v)) {
			issues = append(issues, "WARN: PROVIDER is not a recognized provider: "+v)
		}
	}

	if v := cfg.Get("MAX_CALLS_PER_HOUR", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			issues = append(issues, "ERROR: MAX_CALLS_PER_HOUR is not a valid integer: "+v)
		} else if n <= 0 {
			issues = append(issues, "ERROR: MAX_CALLS_PER_HOUR must be > 0, got: "+v)
		}
	}

	if v := cfg.Get("CLAUDE_TIMEOUT_MINUTES", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			issues = append(issues, "WARN: CLAUDE_TIMEOUT_MINUTES is not a valid integer: "+v)
		} else if n <= 0 {
			issues = append(issues, "WARN: CLAUDE_TIMEOUT_MINUTES should be > 0, got: "+v)
		}
	}

	for _, key := range []string{"CB_FAILURE_THRESHOLD", "CB_SUCCESS_THRESHOLD", "CB_HALF_OPEN_MAX_CALLS"} {
		if v := cfg.Get(key, ""); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				issues = append(issues, "ERROR: "+key+" is not a valid integer: "+v)
			} else if n < 0 || n > 100 {
				issues = append(issues, "WARN: "+key+" should be 0-100, got: "+v)
			}
		}
	}

	return issues
}

func ValidationHasError(results []ValidateResult) bool {
	for _, result := range results {
		if result.Status == "ERROR" {
			return true
		}
	}
	return false
}

func FormatValidateResults(results []ValidateResult) string {
	if len(results) == 0 {
		return "No matching .ralphrc files found."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-30s  %-7s  %s\n", "REPO", "STATUS", "ISSUES")
	b.WriteString(strings.Repeat("-", 72))
	b.WriteByte('\n')
	for _, r := range results {
		if len(r.Issues) == 0 {
			fmt.Fprintf(&b, "%-30s  %-7s\n", r.Name, r.Status)
			continue
		}
		fmt.Fprintf(&b, "%-30s  %-7s  %s\n", r.Name, r.Status, r.Issues[0])
		for _, issue := range r.Issues[1:] {
			fmt.Fprintf(&b, "%-30s  %-7s  %s\n", "", "", issue)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
