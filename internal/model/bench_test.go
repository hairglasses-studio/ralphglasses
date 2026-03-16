package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkLoadConfig(b *testing.B) {
	dir := b.TempDir()
	var content string
	for i := 0; i < 20; i++ {
		content += "KEY_" + string(rune('A'+i)) + "=\"value_" + string(rune('A'+i)) + "\"\n"
	}
	os.WriteFile(filepath.Join(dir, ".ralphrc"), []byte(content), 0644)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LoadConfig(dir)
	}
}

func BenchmarkLoadStatus(b *testing.B) {
	dir := b.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	os.MkdirAll(ralphDir, 0755)
	status := LoopStatus{LoopCount: 100, Status: "running", Model: "sonnet", SessionSpendUSD: 12.50}
	data, _ := json.Marshal(status)
	os.WriteFile(filepath.Join(ralphDir, "status.json"), data, 0644)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LoadStatus(dir)
	}
}

func BenchmarkRefreshRepo(b *testing.B) {
	dir := b.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	os.MkdirAll(ralphDir, 0755)
	data, _ := json.Marshal(LoopStatus{Status: "running"})
	os.WriteFile(filepath.Join(ralphDir, "status.json"), data, 0644)
	data, _ = json.Marshal(CircuitBreakerState{State: "CLOSED"})
	os.WriteFile(filepath.Join(ralphDir, ".circuit_breaker_state"), data, 0644)
	data, _ = json.Marshal(Progress{Iteration: 5})
	os.WriteFile(filepath.Join(ralphDir, "progress.json"), data, 0644)
	os.WriteFile(filepath.Join(dir, ".ralphrc"), []byte("KEY=val\n"), 0644)
	r := &Repo{Path: dir}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RefreshRepo(r)
	}
}
