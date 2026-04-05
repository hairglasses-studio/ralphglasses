package marathon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadWaveFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "waves.yaml")

	content := `waves:
  - name: "wave-1"
    duration: "30m"
    budget: 5.0
    repos:
      - "/home/user/repo-a"
      - "/home/user/repo-b"
    agent: "claude"
    prompt: "prompts/improve.md"
  - name: "wave-2"
    duration: "1h"
    budget: 10.0
    repos:
      - "/home/user/repo-c"
    agent: "gemini"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	wf, err := LoadWaveFile(path)
	if err != nil {
		t.Fatalf("LoadWaveFile: %v", err)
	}

	if len(wf.Waves) != 2 {
		t.Fatalf("expected 2 waves, got %d", len(wf.Waves))
	}

	w1 := wf.Waves[0]
	if w1.Name != "wave-1" {
		t.Fatalf("wave[0].Name: got %q, want %q", w1.Name, "wave-1")
	}
	if w1.Duration != "30m" {
		t.Fatalf("wave[0].Duration: got %q, want %q", w1.Duration, "30m")
	}
	if w1.Budget != 5.0 {
		t.Fatalf("wave[0].Budget: got %f, want 5.0", w1.Budget)
	}
	if len(w1.Repos) != 2 {
		t.Fatalf("wave[0].Repos: got %d, want 2", len(w1.Repos))
	}
	if w1.Agent != "claude" {
		t.Fatalf("wave[0].Agent: got %q, want %q", w1.Agent, "claude")
	}
	if w1.Prompt != "prompts/improve.md" {
		t.Fatalf("wave[0].Prompt: got %q, want %q", w1.Prompt, "prompts/improve.md")
	}

	w2 := wf.Waves[1]
	if w2.Name != "wave-2" {
		t.Fatalf("wave[1].Name: got %q, want %q", w2.Name, "wave-2")
	}
	if w2.Budget != 10.0 {
		t.Fatalf("wave[1].Budget: got %f, want 10.0", w2.Budget)
	}
}

func TestLoadWaveFile_NonExistent(t *testing.T) {
	_, err := LoadWaveFile("/nonexistent/waves.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestLoadWaveFile_InvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("{{invalid yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWaveFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadWaveDir_MultipleFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "waves")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write two wave files.
	file1 := `waves:
  - name: "alpha"
    duration: "15m"
    budget: 2.0
    repos: ["/repo/a"]
    agent: "claude"
`
	file2 := `waves:
  - name: "beta"
    duration: "30m"
    budget: 4.0
    repos: ["/repo/b"]
    agent: "gemini"
`
	if err := os.WriteFile(filepath.Join(dir, "01-alpha.yaml"), []byte(file1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "02-beta.yml"), []byte(file2), 0644); err != nil {
		t.Fatal(err)
	}

	// Also write a non-yaml file that should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatal(err)
	}

	waves, err := LoadWaveDir(dir)
	if err != nil {
		t.Fatalf("LoadWaveDir: %v", err)
	}

	if len(waves) != 2 {
		t.Fatalf("expected 2 waves, got %d", len(waves))
	}

	if waves[0].Name != "alpha" {
		t.Fatalf("expected first wave 'alpha', got %q", waves[0].Name)
	}
	if waves[1].Name != "beta" {
		t.Fatalf("expected second wave 'beta', got %q", waves[1].Name)
	}
}

func TestLoadWaveDir_NonExistent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	waves, err := LoadWaveDir(dir)
	if err != nil {
		t.Fatalf("expected nil error for non-existent dir, got: %v", err)
	}
	if len(waves) != 0 {
		t.Fatalf("expected 0 waves, got %d", len(waves))
	}
}

func TestLoadWaveDir_Empty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty-waves")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	waves, err := LoadWaveDir(dir)
	if err != nil {
		t.Fatalf("LoadWaveDir: %v", err)
	}
	if len(waves) != 0 {
		t.Fatalf("expected 0 waves, got %d", len(waves))
	}
}

func TestWave_ParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		dur     string
		want    time.Duration
		wantErr bool
	}{
		{"30 minutes", "30m", 30 * time.Minute, false},
		{"1 hour", "1h", time.Hour, false},
		{"2.5 hours", "2h30m", 2*time.Hour + 30*time.Minute, false},
		{"empty", "", 0, true},
		{"invalid", "xyz", 0, true},
		{"negative", "-30m", 0, true},
		{"zero", "0s", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := Wave{Name: "test", Duration: tt.dur}
			got, err := w.ParseDuration()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestWave_Validate(t *testing.T) {
	tests := []struct {
		name    string
		wave    Wave
		wantErr bool
	}{
		{
			"valid wave",
			Wave{Name: "w1", Duration: "30m", Budget: 5.0, Repos: []string{"/repo"}, Agent: "claude"},
			false,
		},
		{
			"missing name",
			Wave{Duration: "30m", Budget: 5.0, Repos: []string{"/repo"}},
			true,
		},
		{
			"missing duration",
			Wave{Name: "w1", Budget: 5.0, Repos: []string{"/repo"}},
			true,
		},
		{
			"negative budget",
			Wave{Name: "w1", Duration: "30m", Budget: -1.0, Repos: []string{"/repo"}},
			true,
		},
		{
			"no repos",
			Wave{Name: "w1", Duration: "30m", Budget: 5.0},
			true,
		},
		{
			"unknown agent",
			Wave{Name: "w1", Duration: "30m", Budget: 5.0, Repos: []string{"/repo"}, Agent: "gpt4"},
			true,
		},
		{
			"zero budget is valid",
			Wave{Name: "w1", Duration: "30m", Budget: 0, Repos: []string{"/repo"}},
			false,
		},
		{
			"empty agent is valid",
			Wave{Name: "w1", Duration: "30m", Budget: 5.0, Repos: []string{"/repo"}},
			false,
		},
		{
			"codex agent",
			Wave{Name: "w1", Duration: "1h", Budget: 10.0, Repos: []string{"/repo"}, Agent: "codex"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.wave.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseWaves_Valid(t *testing.T) {
	waves := []Wave{
		{Name: "w1", Duration: "30m", Budget: 5.0, Repos: []string{"/repo/a"}, Agent: "claude"},
		{Name: "w2", Duration: "1h", Budget: 10.0, Repos: []string{"/repo/b"}, Agent: "gemini"},
	}

	parsed, err := ParseWaves(waves)
	if err != nil {
		t.Fatalf("ParseWaves: %v", err)
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 parsed waves, got %d", len(parsed))
	}

	if parsed[0].ParsedDuration != 30*time.Minute {
		t.Fatalf("wave[0] duration: got %s, want 30m", parsed[0].ParsedDuration)
	}
	if parsed[1].ParsedDuration != time.Hour {
		t.Fatalf("wave[1] duration: got %s, want 1h", parsed[1].ParsedDuration)
	}
}

func TestParseWaves_Empty(t *testing.T) {
	_, err := ParseWaves(nil)
	if err == nil {
		t.Fatal("expected error for empty waves")
	}
}

func TestParseWaves_InvalidWave(t *testing.T) {
	waves := []Wave{
		{Name: "w1", Duration: "30m", Budget: 5.0, Repos: []string{"/repo"}},
		{Name: "", Duration: "1h", Budget: 10.0, Repos: []string{"/repo"}}, // missing name
	}

	_, err := ParseWaves(waves)
	if err == nil {
		t.Fatal("expected error for invalid wave")
	}
}

func TestWaveConfigs(t *testing.T) {
	parsed := []ParsedWave{
		{
			Wave:           Wave{Name: "w1", Budget: 5.0, Repos: []string{"/repo/a", "/repo/b"}},
			ParsedDuration: 30 * time.Minute,
		},
		{
			Wave:           Wave{Name: "w2", Budget: 10.0, Repos: []string{"/repo/c"}},
			ParsedDuration: time.Hour,
		},
	}

	configs := WaveConfigs(parsed, 5*time.Minute)
	if len(configs) != 3 {
		t.Fatalf("expected 3 configs (2 repos + 1 repo), got %d", len(configs))
	}

	// First wave, first repo.
	if configs[0].RepoPath != "/repo/a" {
		t.Fatalf("config[0].RepoPath: got %q, want %q", configs[0].RepoPath, "/repo/a")
	}
	if configs[0].BudgetUSD != 5.0 {
		t.Fatalf("config[0].BudgetUSD: got %f, want 5.0", configs[0].BudgetUSD)
	}
	if configs[0].Duration != 30*time.Minute {
		t.Fatalf("config[0].Duration: got %s, want 30m", configs[0].Duration)
	}
	if configs[0].CheckpointInterval != 5*time.Minute {
		t.Fatalf("config[0].CheckpointInterval: got %s, want 5m", configs[0].CheckpointInterval)
	}

	// First wave, second repo.
	if configs[1].RepoPath != "/repo/b" {
		t.Fatalf("config[1].RepoPath: got %q, want %q", configs[1].RepoPath, "/repo/b")
	}

	// Second wave.
	if configs[2].RepoPath != "/repo/c" {
		t.Fatalf("config[2].RepoPath: got %q, want %q", configs[2].RepoPath, "/repo/c")
	}
	if configs[2].BudgetUSD != 10.0 {
		t.Fatalf("config[2].BudgetUSD: got %f, want 10.0", configs[2].BudgetUSD)
	}
	if configs[2].Duration != time.Hour {
		t.Fatalf("config[2].Duration: got %s, want 1h", configs[2].Duration)
	}
}

func TestDefaultWaveDir(t *testing.T) {
	got := DefaultWaveDir("/home/user/repo")
	want := "/home/user/repo/.ralph/waves"
	if got != want {
		t.Fatalf("DefaultWaveDir: got %q, want %q", got, want)
	}
}

func TestLoadWaveFile_YAMLRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	content := `waves:
  - name: "full-test"
    duration: "2h30m"
    budget: 25.5
    repos:
      - "/home/user/repo-1"
      - "/home/user/repo-2"
      - "/home/user/repo-3"
    agent: "codex"
    prompt: "templates/refactor.md"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	wf, err := LoadWaveFile(path)
	if err != nil {
		t.Fatalf("LoadWaveFile: %v", err)
	}

	if len(wf.Waves) != 1 {
		t.Fatalf("expected 1 wave, got %d", len(wf.Waves))
	}

	w := wf.Waves[0]
	if w.Duration != "2h30m" {
		t.Fatalf("Duration: got %q, want %q", w.Duration, "2h30m")
	}
	if w.Budget != 25.5 {
		t.Fatalf("Budget: got %f, want 25.5", w.Budget)
	}
	if len(w.Repos) != 3 {
		t.Fatalf("Repos: got %d, want 3", len(w.Repos))
	}
	if w.Agent != "codex" {
		t.Fatalf("Agent: got %q, want %q", w.Agent, "codex")
	}
	if w.Prompt != "templates/refactor.md" {
		t.Fatalf("Prompt: got %q, want %q", w.Prompt, "templates/refactor.md")
	}
}
