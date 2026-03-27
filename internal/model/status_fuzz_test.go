package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run: go test -fuzz=FuzzLoadStatus -fuzztime=30s ./internal/model/...
func FuzzLoadStatus(f *testing.F) {
	f.Add(`{"status":"running","loop_count":1}`)
	f.Add(`{}`)
	f.Add(`not json`)
	f.Add("")
	f.Add(`{invalid`)
	f.Add(`{"status":"running","loop_count":1,"extra_field":"surprise","nested":{"deep":true}}`)
	f.Add("こんにちは世界 🎉")
	f.Add("\x00\x00")
	f.Add("   \n\t  ")
	f.Add(strings.Repeat(`{"status":"x"}`, 100))

	f.Fuzz(func(t *testing.T, data string) {
		dir := t.TempDir()
		ralphDir := filepath.Join(dir, ".ralph")
		_ = os.MkdirAll(ralphDir, 0755)
		_ = os.WriteFile(filepath.Join(ralphDir, "status.json"), []byte(data), 0644)
		_, _ = LoadStatus(dir)
	})
}

// Run: go test -fuzz=FuzzLoadCircuitBreaker -fuzztime=30s ./internal/model/...
func FuzzLoadCircuitBreaker(f *testing.F) {
	f.Add(`{"state":"CLOSED"}`)
	f.Add(`{}`)
	f.Add(`not json`)
	f.Add("")
	f.Add(`{invalid`)
	f.Add(`{"state":"OPEN","failures":999,"extra":"unexpected"}`)
	f.Add("こんにちは世界 🎉")
	f.Add("\x00\x00")
	f.Add("   \n\t  ")
	f.Add(strings.Repeat("x", 500))

	f.Fuzz(func(t *testing.T, data string) {
		dir := t.TempDir()
		ralphDir := filepath.Join(dir, ".ralph")
		_ = os.MkdirAll(ralphDir, 0755)
		_ = os.WriteFile(filepath.Join(ralphDir, ".circuit_breaker_state"), []byte(data), 0644)
		_, _ = LoadCircuitBreaker(dir)
	})
}

// Run: go test -fuzz=FuzzLoadProgress -fuzztime=30s ./internal/model/...
func FuzzLoadProgress(f *testing.F) {
	f.Add(`{"iteration":1,"status":"running"}`)
	f.Add(`{}`)
	f.Add(`not json`)
	f.Add("")
	f.Add(`{invalid`)
	f.Add(`{"iteration":1,"status":"running","extra_field":true,"nested":{"a":"b"}}`)
	f.Add("こんにちは世界 🎉")
	f.Add("\x00\x00")
	f.Add("   \n\t  ")
	f.Add(strings.Repeat(`{"iteration":0}`, 100))

	f.Fuzz(func(t *testing.T, data string) {
		dir := t.TempDir()
		ralphDir := filepath.Join(dir, ".ralph")
		_ = os.MkdirAll(ralphDir, 0755)
		_ = os.WriteFile(filepath.Join(ralphDir, "progress.json"), []byte(data), 0644)
		_, _ = LoadProgress(dir)
	})
}
