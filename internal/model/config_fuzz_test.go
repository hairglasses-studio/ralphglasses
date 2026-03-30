package model

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run: go test -fuzz=FuzzLoadConfig -fuzztime=30s ./internal/model/...
func FuzzLoadConfig(f *testing.F) {
	f.Add("KEY=value\n")
	f.Add("# comment\nKEY=\"quoted\"\n")
	f.Add("")
	f.Add("=\n")
	f.Add("no-newline")
	f.Add("MULTI=line\nSECOND=line\n")
	f.Add("SPACES = spaced value \n")
	f.Add("QUOTED=\"hello world\"\n")
	f.Add("EMPTY=\n")
	f.Add("#\n#\n#\n")
	f.Add("   \n\t  ")
	f.Add("KEY=こんにちは世界 🎉\n")
	f.Add("\x00\x00")
	f.Add(strings.Repeat("KEY=value\n", 200))
	f.Add("KEY==equals=in=value\n")

	f.Fuzz(func(t *testing.T, data string) {
		dir := t.TempDir()
		rcPath := filepath.Join(dir, ".ralphrc")
		if err := os.WriteFile(rcPath, []byte(data), 0644); err != nil {
			t.Skip()
		}
		cfg, err := LoadConfig(context.Background(), dir)
		if err != nil {
			return
		}
		for k := range cfg.Values {
			cfg.Get(k, "default")
		}
		cfg.Get("nonexistent", "fallback")
	})
}

// Run: go test -fuzz=FuzzConfigKey -fuzztime=30s ./internal/model/...
func FuzzConfigKey(f *testing.F) {
	f.Add("VALID_KEY")
	f.Add("bad key")
	f.Add("123")
	f.Add("")
	f.Add("KEY!")
	f.Add("lowercase")
	f.Add("こんにちは世界 🎉")
	f.Add("\x00\x00")
	f.Add("   \n\t  ")
	f.Add(strings.Repeat("K", 500))

	f.Fuzz(func(t *testing.T, key string) {
		dir := t.TempDir()
		rcPath := filepath.Join(dir, ".ralphrc")
		cfg := &RalphConfig{
			Path:   rcPath,
			Values: map[string]string{key: "value"},
		}
		// Should not panic regardless of key
		_ = cfg.Save()
	})
}
