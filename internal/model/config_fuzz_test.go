package model

import (
	"os"
	"path/filepath"
	"testing"
)

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

	f.Fuzz(func(t *testing.T, data string) {
		dir := t.TempDir()
		rcPath := filepath.Join(dir, ".ralphrc")
		if err := os.WriteFile(rcPath, []byte(data), 0644); err != nil {
			t.Skip()
		}
		cfg, err := LoadConfig(dir)
		if err != nil {
			return
		}
		for k := range cfg.Values {
			cfg.Get(k, "default")
		}
		cfg.Get("nonexistent", "fallback")
	})
}

func FuzzConfigKey(f *testing.F) {
	f.Add("VALID_KEY")
	f.Add("bad key")
	f.Add("123")
	f.Add("")
	f.Add("KEY!")
	f.Add("lowercase")

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
