package discovery

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// ParallelScan walks the given root directory and returns repos that contain a
// .ralph/ directory or .ralphrc file, using a bounded worker pool for
// concurrency. The workers parameter controls the maximum number of goroutines
// checking directories simultaneously. If workers < 1 it defaults to 4.
func ParallelScan(ctx context.Context, root string, workers int) ([]*model.Repo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if workers < 1 {
		workers = 4
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	// Filter to directories only, build work list.
	type dirEntry struct {
		name string
		path string
	}
	var dirs []dirEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirs = append(dirs, dirEntry{
			name: e.Name(),
			path: filepath.Join(root, e.Name()),
		})
	}

	if len(dirs) == 0 {
		return nil, nil
	}

	// Cap workers to the number of directories.
	if workers > len(dirs) {
		workers = len(dirs)
	}

	type result struct {
		repo *model.Repo
	}

	work := make(chan dirEntry, len(dirs))
	results := make(chan result, len(dirs))

	// Spawn workers.
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Go(func() {
			for d := range work {
				if ctx.Err() != nil {
					return
				}
				hasRalph := dirExists(filepath.Join(d.path, ".ralph"))
				hasRC := fileExists(filepath.Join(d.path, ".ralphrc"))
				if !hasRalph && !hasRC {
					continue
				}
				r := &model.Repo{
					Name:     d.name,
					Path:     d.path,
					HasRalph: hasRalph,
					HasRC:    hasRC,
				}
				if errs := model.RefreshRepo(ctx, r); len(errs) > 0 {
					for _, e := range errs {
						slog.Warn("parallel scan: refresh failed", "repo", r.Path, "err", e)
					}
				}
				results <- result{repo: r}
			}
		})
	}

	// Feed work, checking context between sends.
	go func() {
		defer close(work)
		for _, d := range dirs {
			if ctx.Err() != nil {
				return
			}
			work <- d
		}
	}()

	// Close results after all workers finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	var repos []*model.Repo
	for r := range results {
		repos = append(repos, r.repo)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Name < repos[j].Name
	})
	return repos, nil
}
