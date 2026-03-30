package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// --- EnvProvider edge-case tests ---

func TestEnvProvider_EmptyPrefix(t *testing.T) {
	p := &EnvProvider{Prefix: ""}

	t.Cleanup(func() { os.Unsetenv("MY_BARE_KEY") })

	if err := p.Set("my_bare_key", "val"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	v, err := p.Get("my_bare_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "val" {
		t.Fatalf("expected 'val', got %q", v)
	}
}

func TestEnvProvider_ListEmptyPrefix(t *testing.T) {
	p := &EnvProvider{Prefix: ""}

	keys, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// With no prefix filter, should return all env vars.
	if len(keys) == 0 {
		t.Fatal("expected at least some env vars with empty prefix")
	}
}

func TestEnvProvider_SpecialCharKeys(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantEnv string
	}{
		{"dots to underscores", "db.host.port", "TSC_DB_HOST_PORT"},
		{"already upper", "ALREADY_UPPER", "TSC_ALREADY_UPPER"},
		{"mixed case", "mixedCase", "TSC_MIXEDCASE"},
	}

	p := &EnvProvider{Prefix: "TSC_"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { os.Unsetenv(tt.wantEnv) })

			if err := p.Set(tt.key, "testval"); err != nil {
				t.Fatalf("Set: %v", err)
			}

			// Verify the actual env var name.
			raw, ok := os.LookupEnv(tt.wantEnv)
			if !ok {
				t.Fatalf("expected env var %s to be set", tt.wantEnv)
			}
			if raw != "testval" {
				t.Fatalf("expected 'testval', got %q", raw)
			}
		})
	}
}

// --- FileProvider edge-case tests ---

func TestFileProvider_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.enc")

	// Create an empty file.
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	p := &FileProvider{Path: path, Passphrase: "pw"}

	// Should treat empty file as empty store.
	keys, err := p.List()
	if err != nil {
		t.Fatalf("List on empty file: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}
}

func TestFileProvider_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.enc")

	// Write garbage that isn't valid ciphertext.
	if err := os.WriteFile(path, []byte("this is not encrypted data and is long enough"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	p := &FileProvider{Path: path, Passphrase: "pw"}

	_, err := p.Get("key")
	if err == nil {
		t.Fatal("expected error on corrupt file")
	}
}

func TestFileProvider_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	p := &FileProvider{Path: filepath.Join(dir, "conc.enc"), Passphrase: "pw"}

	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "key"
			val := "val"
			if err := p.Set(key, val); err != nil {
				errCh <- err
			}
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = p.List()
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent error: %v", err)
	}
}

func TestFileProvider_GetMissing(t *testing.T) {
	dir := t.TempDir()
	p := &FileProvider{Path: filepath.Join(dir, "s.enc"), Passphrase: "pw"}

	// No file exists yet -- Get on missing key should return ErrNotFound.
	_, err := p.Get("nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFileProvider_MultipleKeys(t *testing.T) {
	dir := t.TempDir()
	p := &FileProvider{Path: filepath.Join(dir, "multi.enc"), Passphrase: "pw"}

	secrets := map[string]string{
		"alpha":   "one",
		"beta":    "two",
		"gamma":   "three",
		"delta":   "four",
		"epsilon": "five",
	}

	for k, v := range secrets {
		if err := p.Set(k, v); err != nil {
			t.Fatalf("Set(%s): %v", k, err)
		}
	}

	for k, want := range secrets {
		got, err := p.Get(k)
		if err != nil {
			t.Fatalf("Get(%s): %v", k, err)
		}
		if got != want {
			t.Fatalf("Get(%s)=%q, want %q", k, got, want)
		}
	}

	keys, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != len(secrets) {
		t.Fatalf("expected %d keys, got %d", len(secrets), len(keys))
	}
}

// --- Registry additional tests ---

func TestRegistry_RegisterNil(t *testing.T) {
	// Registering with a nil provider should not panic.
	r := NewRegistry()
	err := r.Register("nil-provider", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names := r.Providers()
	if len(names) != 1 || names[0] != "nil-provider" {
		t.Fatalf("expected [nil-provider], got %v", names)
	}
}

func TestRegistry_ConcurrentGet(t *testing.T) {
	r := NewRegistry()

	p := &EnvProvider{Prefix: "RCGET_"}
	t.Cleanup(func() { os.Unsetenv("RCGET_K") })
	os.Setenv("RCGET_K", "v")

	if err := r.Register("env", p); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := r.Get("k")
			if err != nil {
				t.Errorf("Get: %v", err)
				return
			}
			if v != "v" {
				t.Errorf("expected 'v', got %q", v)
			}
		}()
	}
	wg.Wait()
}

func TestRegistry_MultipleProviders_AllFail(t *testing.T) {
	r := NewRegistry()

	_ = r.Register("a", &EnvProvider{Prefix: "REGFAIL_A_"})
	_ = r.Register("b", &EnvProvider{Prefix: "REGFAIL_B_"})

	_, err := r.Get("missing")
	if err == nil {
		t.Fatal("expected error")
	}
	// Error should mention the key.
	if got := err.Error(); !contains(got, "missing") {
		t.Fatalf("error should mention key, got: %s", got)
	}
}

func TestRegistry_ProvidersEmpty(t *testing.T) {
	r := NewRegistry()
	names := r.Providers()
	if len(names) != 0 {
		t.Fatalf("expected empty, got %v", names)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
