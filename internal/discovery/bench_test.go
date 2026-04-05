package discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

func BenchmarkScan(b *testing.B) {
	b.ReportAllocs()
	root := b.TempDir()

	// Create 10 realistic repo dirs with .ralph/ and .ralphrc.
	for i := range 10 {
		name := "repo-" + string(rune('a'+i))
		repoPath := filepath.Join(root, name)
		ralphDir := filepath.Join(repoPath, ".ralph")
		if err := os.MkdirAll(ralphDir, 0755); err != nil {
			b.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(repoPath, ".ralphrc"), []byte("PROJECT_NAME="+name+"\n"), 0644)
		data, _ := json.Marshal(model.LoopStatus{Status: "idle"})
		_ = os.WriteFile(filepath.Join(ralphDir, "status.json"), data, 0644)
		data, _ = json.Marshal(model.CircuitBreakerState{State: "CLOSED"})
		_ = os.WriteFile(filepath.Join(ralphDir, ".circuit_breaker_state"), data, 0644)
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repos, err := Scan(ctx, root)
		if err != nil {
			b.Fatal(err)
		}
		if len(repos) != 10 {
			b.Fatalf("expected 10 repos, got %d", len(repos))
		}
	}
}

func BenchmarkScan_Empty(b *testing.B) {
	b.ReportAllocs()
	root := b.TempDir()

	// Create dirs without .ralph/ or .ralphrc — should be skipped.
	for i := range 20 {
		os.MkdirAll(filepath.Join(root, "plain-"+string(rune('a'+i))), 0755)
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repos, err := Scan(ctx, root)
		if err != nil {
			b.Fatal(err)
		}
		if len(repos) != 0 {
			b.Fatalf("expected 0 repos, got %d", len(repos))
		}
	}
}
