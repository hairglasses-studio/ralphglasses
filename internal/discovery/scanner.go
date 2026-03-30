package discovery

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// Scan walks the given root directory (one level deep) and returns repos
// that contain a .ralph/ directory or .ralphrc file.
func Scan(ctx context.Context, root string) ([]*model.Repo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var repos []*model.Repo
	for _, e := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !e.IsDir() {
			continue
		}
		repoPath := filepath.Join(root, e.Name())
		hasRalph := dirExists(filepath.Join(repoPath, ".ralph"))
		hasRC := fileExists(filepath.Join(repoPath, ".ralphrc"))

		if !hasRalph && !hasRC {
			continue
		}

		r := &model.Repo{
			Name:     e.Name(),
			Path:     repoPath,
			HasRalph: hasRalph,
			HasRC:    hasRC,
		}
		if errs := model.RefreshRepo(ctx, r); len(errs) > 0 {
			for _, e := range errs {
				slog.Warn("scan: refresh failed", "repo", r.Path, "err", e)
			}
		}
		repos = append(repos, r)
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Name < repos[j].Name
	})
	return repos, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
