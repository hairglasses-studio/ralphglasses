package session

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type CycleBaseline struct {
	Timestamp    time.Time `json:"timestamp"`
	RepoPath     string    `json:"repo_path"`
	CoveragePC   float64   `json:"coverage_pct"`
	TestCount    int       `json:"test_count"`
	LintCount    int       `json:"lint_count"`
	BuildTimeSec float64   `json:"build_time_sec"`
	GoVersion    string    `json:"go_version"`
}

func RunCycleBaseline(repoPath string) (*CycleBaseline, error) {
	baseline := &CycleBaseline{
		Timestamp: time.Now(),
		RepoPath:  repoPath,
	}

	if out, err := exec.Command("go", "version").CombinedOutput(); err == nil {
		baseline.GoVersion = strings.TrimSpace(string(out))
	}

	start := time.Now()
	buildCmd := exec.Command("go", "build", "./...")
	buildCmd.Dir = repoPath
	_ = buildCmd.Run()
	baseline.BuildTimeSec = time.Since(start).Seconds()

	testListCmd := exec.Command("go", "test", "-list", ".*", "./...")
	testListCmd.Dir = repoPath
	if out, err := testListCmd.CombinedOutput(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Test") {
				baseline.TestCount++
			}
		}
	}

	coverageCmd := exec.Command("go", "test", "-cover", "./...")
	coverageCmd.Dir = repoPath
	if out, err := coverageCmd.CombinedOutput(); err == nil {
		outStr := string(out)
		if idx := strings.Index(outStr, "coverage: "); idx != -1 {
			sub := outStr[idx+10:]
			if end := strings.Index(sub, "%"); end != -1 {
				if val, err := strconv.ParseFloat(sub[:end], 64); err == nil {
					baseline.CoveragePC = val
				}
			}
		}
	}

	return baseline, nil
}

func WriteCycleBaselineToFile(baseline *CycleBaseline, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
