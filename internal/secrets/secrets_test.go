package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// --- EnvProvider tests ---

func TestEnvProvider_GetSet(t *testing.T) {
	p := &EnvProvider{Prefix: "TEST_SECRET_"}

	// Clean up after test.
	t.Cleanup(func() { os.Unsetenv("TEST_SECRET_MY_KEY") })

	// Get missing key.
	_, err := p.Get("my_key")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Set and get.
	if err := p.Set("my_key", "hello"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, err := p.Get("my_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "hello" {
		t.Fatalf("expected 'hello', got %q", v)
	}
}

func TestEnvProvider_Delete(t *testing.T) {
	p := &EnvProvider{Prefix: "TEST_SECRET_"}
	t.Cleanup(func() { os.Unsetenv("TEST_SECRET_DEL") })

	if err := p.Set("del", "val"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := p.Delete("del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := p.Get("del")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestEnvProvider_List(t *testing.T) {
	p := &EnvProvider{Prefix: "TEST_LIST_"}
	t.Cleanup(func() {
		os.Unsetenv("TEST_LIST_A")
		os.Unsetenv("TEST_LIST_B")
	})

	_ = p.Set("a", "1")
	_ = p.Set("b", "2")

	keys, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	found := 0
	for _, k := range keys {
		if k == "TEST_LIST_A" || k == "TEST_LIST_B" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("expected 2 matching keys, found %d in %v", found, keys)
	}
}

func TestEnvProvider_KeyTransform(t *testing.T) {
	p := &EnvProvider{Prefix: "PFX_"}

	// Dots become underscores, lower becomes upper.
	t.Cleanup(func() { os.Unsetenv("PFX_DB_HOST") })

	if err := p.Set("db.host", "localhost"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	v, err := p.Get("db.host")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "localhost" {
		t.Fatalf("expected 'localhost', got %q", v)
	}
}

// --- FileProvider tests ---

func TestFileProvider_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".secrets.enc")

	p := &FileProvider{Path: path, Passphrase: "test-passphrase"}

	// Empty file: List returns empty.
	keys, err := p.List()
	if err != nil {
		t.Fatalf("List on empty: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}

	// Set and get.
	if err := p.Set("api_key", "sk-123"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	v, err := p.Get("api_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "sk-123" {
		t.Fatalf("expected 'sk-123', got %q", v)
	}

	// File must exist and be non-empty.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("encrypted file is empty")
	}

	// Verify file permissions.
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 permissions, got %04o", info.Mode().Perm())
	}
}

func TestFileProvider_Delete(t *testing.T) {
	dir := t.TempDir()
	p := &FileProvider{Path: filepath.Join(dir, "s.enc"), Passphrase: "pw"}

	_ = p.Set("a", "1")
	_ = p.Set("b", "2")

	if err := p.Delete("a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := p.Get("a")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// "b" should still be there.
	v, err := p.Get("b")
	if err != nil || v != "2" {
		t.Fatalf("expected b=2, got %q, err=%v", v, err)
	}
}

func TestFileProvider_DeleteMissing(t *testing.T) {
	dir := t.TempDir()
	p := &FileProvider{Path: filepath.Join(dir, "s.enc"), Passphrase: "pw"}

	err := p.Delete("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFileProvider_WrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.enc")

	p1 := &FileProvider{Path: path, Passphrase: "correct"}
	_ = p1.Set("key", "value")

	p2 := &FileProvider{Path: path, Passphrase: "wrong"}
	_, err := p2.Get("key")
	if err == nil {
		t.Fatal("expected error with wrong passphrase")
	}
}

func TestFileProvider_List(t *testing.T) {
	dir := t.TempDir()
	p := &FileProvider{Path: filepath.Join(dir, "s.enc"), Passphrase: "pw"}

	_ = p.Set("z", "1")
	_ = p.Set("a", "2")
	_ = p.Set("m", "3")

	keys, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	// Keys should be sorted.
	if keys[0] != "a" || keys[1] != "m" || keys[2] != "z" {
		t.Fatalf("expected sorted [a m z], got %v", keys)
	}
}

func TestFileProvider_Overwrite(t *testing.T) {
	dir := t.TempDir()
	p := &FileProvider{Path: filepath.Join(dir, "s.enc"), Passphrase: "pw"}

	_ = p.Set("k", "v1")
	_ = p.Set("k", "v2")

	v, err := p.Get("k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "v2" {
		t.Fatalf("expected 'v2', got %q", v)
	}
}

// --- Registry tests ---

func TestRegistry_Get_Priority(t *testing.T) {
	r := NewRegistry()

	env1 := &EnvProvider{Prefix: "REG_P1_"}
	env2 := &EnvProvider{Prefix: "REG_P2_"}

	t.Cleanup(func() {
		os.Unsetenv("REG_P1_KEY")
		os.Unsetenv("REG_P2_KEY")
	})

	if err := r.Register("first", env1); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("second", env2); err != nil {
		t.Fatal(err)
	}

	// Set on both. First provider wins.
	os.Setenv("REG_P1_KEY", "from-first")
	os.Setenv("REG_P2_KEY", "from-second")

	v, err := r.Get("key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "from-first" {
		t.Fatalf("expected 'from-first', got %q", v)
	}
}

func TestRegistry_Get_Fallthrough(t *testing.T) {
	r := NewRegistry()

	env1 := &EnvProvider{Prefix: "REG_F1_"}
	env2 := &EnvProvider{Prefix: "REG_F2_"}

	t.Cleanup(func() {
		os.Unsetenv("REG_F2_ONLY_IN_SECOND")
	})

	_ = r.Register("first", env1)
	_ = r.Register("second", env2)

	// Only set on second provider.
	os.Setenv("REG_F2_ONLY_IN_SECOND", "found-it")

	v, err := r.Get("only_in_second")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "found-it" {
		t.Fatalf("expected 'found-it', got %q", v)
	}
}

func TestRegistry_Get_NoProviders(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("anything")
	if !errors.Is(err, ErrNoProviders) {
		t.Fatalf("expected ErrNoProviders, got %v", err)
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	r := NewRegistry()
	_ = r.Register("env", &EnvProvider{})
	err := r.Register("env", &EnvProvider{})
	if !errors.Is(err, ErrDuplicateProvider) {
		t.Fatalf("expected ErrDuplicateProvider, got %v", err)
	}
}

func TestRegistry_Providers(t *testing.T) {
	r := NewRegistry()
	_ = r.Register("a", &EnvProvider{})
	_ = r.Register("b", &EnvProvider{})

	names := r.Providers()
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("expected [a b], got %v", names)
	}
}

func TestRegistry_Get_AllFail(t *testing.T) {
	r := NewRegistry()
	_ = r.Register("env", &EnvProvider{Prefix: "NONEXIST_"})

	_, err := r.Get("missing")
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

// --- VaultProvider tests (stub) ---

func TestVaultProvider_NotImplemented(t *testing.T) {
	v := &VaultProvider{Address: "https://vault:8200", Token: "tok"}

	if _, err := v.Get("key"); err == nil {
		t.Fatal("expected error")
	}
	if err := v.Set("key", "val"); err == nil {
		t.Fatal("expected error")
	}
	if err := v.Delete("key"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := v.List(); err == nil {
		t.Fatal("expected error")
	}
}

// --- Interface compliance ---

var (
	_ Provider = (*EnvProvider)(nil)
	_ Provider = (*FileProvider)(nil)
	_ Provider = (*SOPSProvider)(nil)
	_ Provider = (*VaultProvider)(nil)
)
