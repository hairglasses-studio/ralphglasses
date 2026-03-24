package model

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzLoadStatus(f *testing.F) {
	f.Add(`{"status":"running","loop_count":1}`)
	f.Add(`{}`)
	f.Add(`not json`)
	f.Add("")

	f.Fuzz(func(t *testing.T, data string) {
		dir := t.TempDir()
		ralphDir := filepath.Join(dir, ".ralph")
		_ = os.MkdirAll(ralphDir, 0755)
		_ = os.WriteFile(filepath.Join(ralphDir, "status.json"), []byte(data), 0644)
		_, _ = LoadStatus(dir)
	})
}

func FuzzLoadCircuitBreaker(f *testing.F) {
	f.Add(`{"state":"CLOSED"}`)
	f.Add(`{}`)
	f.Add(`not json`)
	f.Add("")

	f.Fuzz(func(t *testing.T, data string) {
		dir := t.TempDir()
		ralphDir := filepath.Join(dir, ".ralph")
		_ = os.MkdirAll(ralphDir, 0755)
		_ = os.WriteFile(filepath.Join(ralphDir, ".circuit_breaker_state"), []byte(data), 0644)
		_, _ = LoadCircuitBreaker(dir)
	})
}

func FuzzLoadProgress(f *testing.F) {
	f.Add(`{"iteration":1,"status":"running"}`)
	f.Add(`{}`)
	f.Add(`not json`)
	f.Add("")

	f.Fuzz(func(t *testing.T, data string) {
		dir := t.TempDir()
		ralphDir := filepath.Join(dir, ".ralph")
		_ = os.MkdirAll(ralphDir, 0755)
		_ = os.WriteFile(filepath.Join(ralphDir, "progress.json"), []byte(data), 0644)
		_, _ = LoadProgress(dir)
	})
}
