// Command ralphglasses-godview provides a non-interactive, read-only,
// maximum-throughput fleet monitoring dashboard. Streams live agent output
// from Claude/Codex/Gemini across all repos in a single terminal pane.
//
// Usage:
//
//	ralphglasses-godview --scan-path ~/hairglasses-studio
//	ralphglasses-godview --scan-path ~/hairglasses-studio --refresh 30ms
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/godview"
)

func main() {
	scanPath := flag.String("scan-path", os.ExpandEnv("$HOME/hairglasses-studio"),
		"Root directory to scan for ralph-enabled repos")
	refresh := flag.Duration("refresh", 50*time.Millisecond,
		"Render refresh interval (e.g., 30ms, 50ms, 100ms)")
	provFilter := flag.String("filter", "",
		"Comma-separated provider filter (e.g., 'claude,gemini')")
	repoFilter := flag.String("repo", "",
		"Comma-separated repo filter (e.g., 'ralphglasses,mcpkit')")
	_ = flag.Float64("cost-alert", 0,
		"Budget alert threshold in USD (0 = disabled)")
	demo := flag.Bool("demo", false,
		"Run with demo data (no live connection)")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT/SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()

	gv := godview.New(*scanPath, *refresh)

	if *demo {
		runDemo(ctx, gv)
		return
	}

	// Live mode: scan repos and watch JSONL files
	runLive(ctx, gv, *scanPath, *provFilter, *repoFilter)
}

func runDemo(ctx context.Context, gv *godview.GodView) {
	// Populate with realistic demo data
	repos := generateDemoRepos()
	gv.UpdateRepos(repos)
	gv.SetBudget(50.0, 12.40)

	// Simulate live output
	go func() {
		messages := []struct {
			prov, repo, text string
		}{
			{"claude", "ralphglasses", "Analyzing selftest.go build snapshot path..."},
			{"claude", "ralphglasses", "Found issue: go build targets multiple packages"},
			{"gemini", "mcpkit", "Reading sampling/handler_test.go for coverage gaps..."},
			{"claude", "hg-mcp", "✓ All 12 handler tests passing"},
			{"claude", "ralphglasses", "Writing fix to internal/e2e/selftest.go..."},
			{"codex", "jobb", "Error: model gpt-5.4-xhigh not supported"},
			{"gemini", "mcpkit", "Found 3 uncovered edge cases in sampling package"},
			{"claude", "ralphglasses", "Running go test ./internal/e2e/... -count=1"},
			{"claude", "mesmer", "Starting security audit of consolidated tools..."},
			{"gemini", "claudekit", "Reviewing env_status handler for race conditions"},
		}
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(800 * time.Millisecond):
				msg := messages[i%len(messages)]
				gv.AppendLine(godview.LogLine{
					Provider:  msg.prov,
					Repo:      msg.repo,
					Text:      msg.text,
					Timestamp: time.Now(),
				})
				i++
			}
		}
	}()

	if err := gv.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runLive(ctx context.Context, gv *godview.GodView, scanPath, provFilter, repoFilter string) {
	provSet := parseFilter(provFilter)
	repoSet := parseFilter(repoFilter)

	// Initial scan for repos with .ralphrc
	repos := scanRepos(scanPath, provSet, repoSet)
	gv.UpdateRepos(repos)

	// Background: watch JSONL files for updates
	go watchJournals(ctx, gv, scanPath)

	// Background: periodic rescan (every 5s)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				repos := scanRepos(scanPath, provSet, repoSet)
				gv.UpdateRepos(repos)
			case <-ctx.Done():
				return
			}
		}
	}()

	if err := gv.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func scanRepos(scanPath string, provFilter, repoFilter map[string]bool) []godview.RepoStatus {
	entries, err := os.ReadDir(scanPath)
	if err != nil {
		return nil
	}

	var repos []godview.RepoStatus
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if len(repoFilter) > 0 && !repoFilter[name] {
			continue
		}

		rcPath := filepath.Join(scanPath, name, ".ralphrc")
		if _, err := os.Stat(rcPath); err != nil {
			continue // No .ralphrc
		}

		repo := godview.RepoStatus{
			Name:   name,
			Status: "idle",
		}

		// Check for recent journal entries
		journalPath := filepath.Join(scanPath, name, ".ralph", "improvement_journal.jsonl")
		if info, err := os.Stat(journalPath); err == nil {
			repo.LastUpdate = info.ModTime()
			// Read last entry for status
			if entry := lastJournalEntry(journalPath); entry != nil {
				repo.Provider = entry.Provider
				repo.TotalCost = entry.SpentUSD
				repo.Turns = entry.TurnCount
				if entry.ExitReason == "completed normally" {
					repo.Status = "completed"
				} else if len(entry.Failed) > 0 {
					repo.Status = "error"
				} else {
					repo.Status = "idle"
				}
			}
		}

		if len(provFilter) > 0 && repo.Provider != "" && !provFilter[repo.Provider] {
			continue
		}

		repos = append(repos, repo)
	}
	return repos
}

type journalEntry struct {
	Provider   string   `json:"provider"`
	SpentUSD   float64  `json:"spent_usd"`
	TurnCount  int      `json:"turn_count"`
	ExitReason string   `json:"exit_reason"`
	Failed     []string `json:"failed"`
}

func lastJournalEntry(path string) *journalEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		return nil
	}
	var entry journalEntry
	if json.Unmarshal([]byte(lines[len(lines)-1]), &entry) != nil {
		return nil
	}
	return &entry
}

func watchJournals(ctx context.Context, gv *godview.GodView, scanPath string) {
	// Poll journal files every 2 seconds for new entries
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastSeen := make(map[string]int64) // path -> last size

	for {
		select {
		case <-ticker.C:
			entries, _ := os.ReadDir(scanPath)
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				jPath := filepath.Join(scanPath, e.Name(), ".ralph", "improvement_journal.jsonl")
				info, err := os.Stat(jPath)
				if err != nil {
					continue
				}
				size := info.Size()
				prev, ok := lastSeen[jPath]
				if ok && size <= prev {
					continue // No new data
				}
				lastSeen[jPath] = size

				// Read new entries
				if entry := lastJournalEntry(jPath); entry != nil && entry.Provider != "" {
					status := "completed"
					if len(entry.Failed) > 0 {
						status = "failed"
					}
					gv.AppendLine(godview.LogLine{
						Provider:  entry.Provider,
						Repo:      e.Name(),
						Text:      fmt.Sprintf("[%s] turns=%d cost=%s exit=%s", status, entry.TurnCount, godview.FormatCost(entry.SpentUSD), entry.ExitReason),
						Timestamp: time.Now(),
					})
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func parseFilter(s string) map[string]bool {
	if s == "" {
		return nil
	}
	m := make(map[string]bool)
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			m[part] = true
		}
	}
	return m
}

func generateDemoRepos() []godview.RepoStatus {
	return []godview.RepoStatus{
		{Name: "ralphglasses", Provider: "claude", Status: "running", Turns: 21, CostPerHr: 2.40, TotalCost: 0.116, CurrentTask: "Fix selftest build snapshot", Progress: 80},
		{Name: "mcpkit", Provider: "gemini", Status: "running", Turns: 8, CostPerHr: 0.80, TotalCost: 0.032, CurrentTask: "Review sampling coverage", Progress: 40},
		{Name: "hg-mcp", Provider: "claude", Status: "completed", Turns: 16, TotalCost: 0.052, CurrentTask: "Add handler tests", Progress: 100},
		{Name: "jobb", Provider: "codex", Status: "error", CurrentTask: "Model not supported"},
		{Name: "mesmer", Provider: "claude", Status: "running", Turns: 5, CostPerHr: 1.20, TotalCost: 0.028, CurrentTask: "Security audit", Progress: 25},
		{Name: "claudekit", Provider: "gemini", Status: "running", Turns: 3, CostPerHr: 0.40, TotalCost: 0.012, CurrentTask: "Race condition review", Progress: 15},
		{Name: "dotfiles-mcp", Status: "idle"},
		{Name: "systemd-mcp", Status: "idle"},
		{Name: "tmux-mcp", Status: "idle"},
		{Name: "process-mcp", Status: "idle"},
		{Name: "dotfiles", Status: "idle"},
		{Name: "docs", Status: "idle"},
		{Name: "cr8-cli", Status: "idle"},
	}
}
