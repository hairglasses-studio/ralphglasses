package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewKeyStore(t *testing.T) {
	t.Run("creates directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "keys")
		ks, err := NewKeyStore(dir)
		if err != nil {
			t.Fatalf("NewKeyStore: %v", err)
		}
		if ks == nil {
			t.Fatal("expected non-nil KeyStore")
		}
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat dir: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("expected directory")
		}
	})

	t.Run("empty dir error", func(t *testing.T) {
		_, err := NewKeyStore("")
		if err == nil {
			t.Fatal("expected error for empty dir")
		}
	})
}

func TestGenerateKey(t *testing.T) {
	ks := newTestKeyStore(t)

	kp, err := ks.GenerateKey("test-key")
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if kp.Name != "test-key" {
		t.Errorf("Name = %q, want %q", kp.Name, "test-key")
	}
	if !strings.HasPrefix(kp.PublicKey, "ssh-ed25519 ") {
		t.Errorf("PublicKey should start with ssh-ed25519, got %q", kp.PublicKey)
	}
	if !strings.HasSuffix(kp.PublicKey, " test-key") {
		t.Errorf("PublicKey should end with key name, got %q", kp.PublicKey)
	}
	if !strings.HasPrefix(kp.Fingerprint, "SHA256:") {
		t.Errorf("Fingerprint should start with SHA256:, got %q", kp.Fingerprint)
	}
	if kp.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	// Verify private key file permissions.
	info, err := os.Stat(kp.PrivatePath)
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("private key perms = %o, want 0600", perm)
	}

	// Verify public key file permissions.
	pubPath := kp.PrivatePath + ".pub"
	info, err = os.Stat(pubPath)
	if err != nil {
		t.Fatalf("stat public key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Errorf("public key perms = %o, want 0644", perm)
	}
}

func TestGenerateKeyDuplicate(t *testing.T) {
	ks := newTestKeyStore(t)

	if _, err := ks.GenerateKey("dup"); err != nil {
		t.Fatalf("first GenerateKey: %v", err)
	}
	if _, err := ks.GenerateKey("dup"); err == nil {
		t.Fatal("expected error for duplicate key name")
	}
}

func TestGenerateKeyInvalidName(t *testing.T) {
	ks := newTestKeyStore(t)

	if _, err := ks.GenerateKey(""); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := ks.GenerateKey("foo/bar"); err == nil {
		t.Fatal("expected error for name with slash")
	}
}

func TestListKeys(t *testing.T) {
	ks := newTestKeyStore(t)

	// Empty store.
	keys, err := ks.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}

	// Generate a few keys.
	for _, name := range []string{"charlie", "alice", "bob"} {
		if _, err := ks.GenerateKey(name); err != nil {
			t.Fatalf("GenerateKey(%q): %v", name, err)
		}
	}

	keys, err = ks.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}

	// Sorted by name.
	want := []string{"alice", "bob", "charlie"}
	for i, kp := range keys {
		if kp.Name != want[i] {
			t.Errorf("keys[%d].Name = %q, want %q", i, kp.Name, want[i])
		}
	}
}

func TestGetKey(t *testing.T) {
	ks := newTestKeyStore(t)

	if _, err := ks.GenerateKey("mykey"); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	kp, err := ks.GetKey("mykey")
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if kp.Name != "mykey" {
		t.Errorf("Name = %q, want %q", kp.Name, "mykey")
	}

	// Not found.
	if _, err := ks.GetKey("noexist"); err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestDeleteKey(t *testing.T) {
	ks := newTestKeyStore(t)

	if _, err := ks.GenerateKey("todelete"); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if err := ks.DeleteKey("todelete"); err != nil {
		t.Fatalf("DeleteKey: %v", err)
	}

	// Verify files removed.
	if _, err := os.Stat(ks.privatePath("todelete")); !os.IsNotExist(err) {
		t.Error("private key file should be removed")
	}
	if _, err := os.Stat(ks.publicPath("todelete")); !os.IsNotExist(err) {
		t.Error("public key file should be removed")
	}

	// Delete again should error.
	if err := ks.DeleteKey("todelete"); err == nil {
		t.Fatal("expected error deleting non-existent key")
	}
}

func TestAuthorizedKeys(t *testing.T) {
	ks := newTestKeyStore(t)

	for _, name := range []string{"key1", "key2"} {
		if _, err := ks.GenerateKey(name); err != nil {
			t.Fatalf("GenerateKey(%q): %v", name, err)
		}
	}

	content, err := ks.AuthorizedKeys()
	if err != nil {
		t.Fatalf("AuthorizedKeys: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), content)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "ssh-ed25519 ") {
			t.Errorf("line should start with ssh-ed25519: %q", line)
		}
	}
}

func newTestKeyStore(t *testing.T) *KeyStore {
	t.Helper()
	ks, err := NewKeyStore(filepath.Join(t.TempDir(), "keys"))
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}
	return ks
}
