package marathon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Wave defines a single marathon wave — a named phase with its own duration,
// budget, target repos, agent provider, and prompt template.
type Wave struct {
	Name     string   `yaml:"name"`
	Duration string   `yaml:"duration"` // parseable as time.Duration
	Budget   float64  `yaml:"budget"`   // USD
	Repos    []string `yaml:"repos"`
	Agent    string   `yaml:"agent"`  // claude, gemini, codex
	Prompt   string   `yaml:"prompt"` // prompt template path
}

// WaveFile is the top-level structure of a wave YAML file.
type WaveFile struct {
	Waves []Wave `yaml:"waves"`
}

// ParsedWave is a Wave with its Duration string resolved to time.Duration.
type ParsedWave struct {
	Wave
	ParsedDuration time.Duration
}

// ParseDuration resolves the Duration string field into a time.Duration.
func (w *Wave) ParseDuration() (time.Duration, error) {
	if w.Duration == "" {
		return 0, fmt.Errorf("wave %q: duration is empty", w.Name)
	}
	d, err := time.ParseDuration(w.Duration)
	if err != nil {
		return 0, fmt.Errorf("wave %q: parse duration %q: %w", w.Name, w.Duration, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("wave %q: duration must be positive, got %s", w.Name, d)
	}
	return d, nil
}

// Validate checks that the wave has all required fields and valid values.
func (w *Wave) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("wave name is required")
	}
	if _, err := w.ParseDuration(); err != nil {
		return err
	}
	if w.Budget < 0 {
		return fmt.Errorf("wave %q: budget must be non-negative, got %f", w.Name, w.Budget)
	}
	if len(w.Repos) == 0 {
		return fmt.Errorf("wave %q: at least one repo is required", w.Name)
	}

	validAgents := map[string]bool{"claude": true, "gemini": true, "codex": true}
	agent := strings.ToLower(w.Agent)
	if agent != "" && !validAgents[agent] {
		return fmt.Errorf("wave %q: unknown agent %q (valid: claude, gemini, codex)", w.Name, w.Agent)
	}

	return nil
}

// LoadWaveFile reads and parses a single wave YAML file.
func LoadWaveFile(path string) (*WaveFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read wave file %s: %w", path, err)
	}

	var wf WaveFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parse wave file %s: %w", path, err)
	}

	return &wf, nil
}

// LoadWaveDir loads all *.yaml and *.yml files from the given directory and
// returns a merged slice of waves in file order. Files are sorted by name.
func LoadWaveDir(dir string) ([]Wave, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read wave dir %s: %w", dir, err)
	}

	var waves []Wave
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		wf, err := LoadWaveFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", name, err)
		}
		waves = append(waves, wf.Waves...)
	}

	return waves, nil
}

// ParseWaves validates and resolves durations for a slice of waves.
func ParseWaves(waves []Wave) ([]ParsedWave, error) {
	if len(waves) == 0 {
		return nil, fmt.Errorf("no waves defined")
	}

	parsed := make([]ParsedWave, 0, len(waves))
	for i := range waves {
		w := &waves[i]
		if err := w.Validate(); err != nil {
			return nil, fmt.Errorf("wave[%d]: %w", i, err)
		}
		d, _ := w.ParseDuration() // already validated
		parsed = append(parsed, ParsedWave{
			Wave:           *w,
			ParsedDuration: d,
		})
	}

	return parsed, nil
}

// WaveConfigs converts a slice of ParsedWaves into marathon Config values,
// one per wave-repo combination. The checkpointInterval is applied uniformly.
func WaveConfigs(waves []ParsedWave, checkpointInterval time.Duration) []Config {
	var configs []Config
	for _, pw := range waves {
		for _, repo := range pw.Repos {
			configs = append(configs, Config{
				BudgetUSD:          pw.Budget,
				Duration:           pw.ParsedDuration,
				CheckpointInterval: checkpointInterval,
				SessionCount:       1,
				RepoPath:           repo,
			})
		}
	}
	return configs
}

// DefaultWaveDir returns the conventional wave directory for a repo.
func DefaultWaveDir(repoPath string) string {
	return filepath.Join(repoPath, ".ralph", "waves")
}
