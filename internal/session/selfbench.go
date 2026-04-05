package session

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SelfBenchmark captures per-iteration build quality metrics.
type SelfBenchmark struct {
	Timestamp  time.Time `json:"timestamp"`
	Iteration  int       `json:"iteration"`
	BuildTime  string    `json:"build_time"`
	TestResult string    `json:"test_result"` // "pass" or "fail"
	BinarySize int64     `json:"binary_size"` // bytes
	LintScore  string    `json:"lint_score"`  // "clean" or error count
	Coverage   string    `json:"coverage"`    // percentage string
}

// RunSelfBenchmark executes build quality checks on a repo and returns metrics.
func RunSelfBenchmark(repoPath string, iteration int) SelfBenchmark {
	b := SelfBenchmark{
		Timestamp: time.Now(),
		Iteration: iteration,
	}

	// Build time
	start := time.Now()
	buildCmd := exec.Command("go", "build", "./...")
	buildCmd.Dir = repoPath
	if err := buildCmd.Run(); err != nil {
		b.BuildTime = "failed"
	} else {
		b.BuildTime = time.Since(start).Round(time.Millisecond).String()
	}

	// Binary size (main binary)
	binPath := filepath.Join(repoPath, filepath.Base(repoPath))
	if info, err := os.Stat(binPath); err == nil {
		b.BinarySize = info.Size()
	}

	// Test result
	testCmd := exec.Command("go", "test", "./...", "-count=1", "-timeout", "120s")
	testCmd.Dir = repoPath
	if err := testCmd.Run(); err != nil {
		b.TestResult = "fail"
	} else {
		b.TestResult = "pass"
	}

	// Vet/lint
	vetCmd := exec.Command("go", "vet", "./...")
	vetCmd.Dir = repoPath
	if out, err := vetCmd.CombinedOutput(); err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		b.LintScore = fmt.Sprintf("%d issues", len(lines))
	} else {
		b.LintScore = "clean"
	}

	return b
}

// SaveBenchmark appends a benchmark result to the repo's .ralph directory.
func SaveBenchmark(repoPath string, b SelfBenchmark) error {
	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "benchmarks.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, string(data))
	return err
}

// LoadBenchmarks reads benchmark history from the repo's .ralph directory.
func LoadBenchmarks(repoPath string) ([]SelfBenchmark, error) {
	path := filepath.Join(repoPath, ".ralph", "benchmarks.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var results []SelfBenchmark
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var b SelfBenchmark
		if err := json.Unmarshal([]byte(line), &b); err != nil {
			continue
		}
		results = append(results, b)
	}
	return results, nil
}
