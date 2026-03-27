package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// MigrateJSONToStore reads session JSON files from dir and imports them into
// the given Store. It skips sessions that already exist in the store.
// This is intended for one-time migration when enabling SQLite persistence.
func MigrateJSONToStore(ctx context.Context, dir string, store Store) (imported int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // nothing to migrate
		}
		return 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".json")

		// Skip if already in store.
		if _, err := store.GetSession(ctx, id); err == nil {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			slog.Warn("migrate: read failed", "file", entry.Name(), "err", err)
			continue
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			slog.Warn("migrate: unmarshal failed", "file", entry.Name(), "err", err)
			continue
		}

		// Ensure RepoName is set (older JSON files may lack it).
		if s.RepoName == "" && s.RepoPath != "" {
			s.RepoName = filepath.Base(s.RepoPath)
		}

		if err := store.SaveSession(ctx, &s); err != nil {
			slog.Warn("migrate: save failed", "session", id, "err", err)
			continue
		}
		imported++
	}

	return imported, nil
}
