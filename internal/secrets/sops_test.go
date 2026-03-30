package secrets

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeSops creates a shell script that mimics sops behavior for testing.
// decrypt: reads the file and outputs it as-is (assumes plaintext JSON for tests).
// encrypt: reads stdin and writes to --output path.
func writeFakeSops(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "fake-sops")
	script := `#!/bin/sh
# Minimal fake sops for testing.
# Supports: --decrypt FILE, --encrypt ... --output FILE /dev/stdin

mode=""
output=""
file=""
age=""
pgp=""

while [ $# -gt 0 ]; do
  case "$1" in
    --decrypt) mode="decrypt"; shift; file="$1"; shift ;;
    --encrypt) mode="encrypt"; shift ;;
    --output)  shift; output="$1"; shift ;;
    --age)     shift; age="$1"; shift ;;
    --pgp)     shift; pgp="$1"; shift ;;
    --input-type|--output-type) shift; shift ;;
    /dev/stdin) shift ;;
    *) shift ;;
  esac
done

if [ "$mode" = "decrypt" ]; then
  if [ ! -f "$file" ]; then
    echo "file not found: $file" >&2
    exit 1
  fi
  cat "$file"
  exit 0
fi

if [ "$mode" = "encrypt" ]; then
  if [ -z "$output" ]; then
    echo "no --output specified" >&2
    exit 1
  fi
  cat > "$output"
  exit 0
fi

echo "unknown mode" >&2
exit 1
`
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write fake sops: %v", err)
	}
	return bin
}

// writeFakeSopsFailDecrypt creates a sops binary that always fails on decrypt.
func writeFakeSopsFailDecrypt(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "fail-sops")
	script := `#!/bin/sh
echo "decryption failed: no key found" >&2
exit 1
`
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write fail sops: %v", err)
	}
	return bin
}

func TestSOPSProvider_Bin(t *testing.T) {
	tests := []struct {
		name     string
		binary   string
		expected string
	}{
		{"default binary", "", "sops"},
		{"custom binary", "/usr/local/bin/sops2", "/usr/local/bin/sops2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &SOPSProvider{Binary: tt.binary}
			if got := p.bin(); got != tt.expected {
				t.Fatalf("bin()=%q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSOPSProvider_GetSetDelete(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeSops(t, dir)
	filePath := filepath.Join(dir, "secrets.json")

	p := &SOPSProvider{
		FilePath: filePath,
		Binary:   bin,
	}

	// Set a secret (decrypt will fail since file doesn't exist, Set handles that).
	if err := p.Set("api_key", "sk-abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Verify the file was written.
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal written file: %v", err)
	}
	if m["api_key"] != "sk-abc123" {
		t.Fatalf("expected 'sk-abc123' in file, got %q", m["api_key"])
	}

	// Get the secret.
	v, err := p.Get("api_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "sk-abc123" {
		t.Fatalf("expected 'sk-abc123', got %q", v)
	}

	// Set another.
	if err := p.Set("db_pass", "hunter2"); err != nil {
		t.Fatalf("Set db_pass: %v", err)
	}

	// Delete first key.
	if err := p.Delete("api_key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify deleted.
	_, err = p.Get("api_key")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	// Other key still there.
	v, err = p.Get("db_pass")
	if err != nil {
		t.Fatalf("Get db_pass: %v", err)
	}
	if v != "hunter2" {
		t.Fatalf("expected 'hunter2', got %q", v)
	}
}

func TestSOPSProvider_GetMissing(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeSops(t, dir)
	filePath := filepath.Join(dir, "secrets.json")

	// Create a valid file with one key.
	data, _ := json.Marshal(map[string]string{"exists": "yes"})
	os.WriteFile(filePath, data, 0644)

	p := &SOPSProvider{FilePath: filePath, Binary: bin}

	_, err := p.Get("nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSOPSProvider_List(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeSops(t, dir)
	filePath := filepath.Join(dir, "secrets.json")

	secrets := map[string]string{"alpha": "1", "beta": "2", "gamma": "3"}
	data, _ := json.Marshal(secrets)
	os.WriteFile(filePath, data, 0644)

	p := &SOPSProvider{FilePath: filePath, Binary: bin}

	keys, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
}

func TestSOPSProvider_DeleteMissing(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeSops(t, dir)
	filePath := filepath.Join(dir, "secrets.json")

	data, _ := json.Marshal(map[string]string{"a": "1"})
	os.WriteFile(filePath, data, 0644)

	p := &SOPSProvider{FilePath: filePath, Binary: bin}

	err := p.Delete("nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSOPSProvider_DecryptFailure(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeSopsFailDecrypt(t, dir)

	p := &SOPSProvider{
		FilePath: filepath.Join(dir, "secrets.json"),
		Binary:   bin,
	}

	// Create a dummy file so the script runs.
	os.WriteFile(p.FilePath, []byte("{}"), 0644)

	_, err := p.Get("key")
	if err == nil {
		t.Fatal("expected error on decrypt failure")
	}
	if !strings.Contains(err.Error(), "sops decrypt") {
		t.Fatalf("expected 'sops decrypt' in error, got: %s", err.Error())
	}
}

func TestSOPSProvider_SetWithDecryptFailure(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeSopsFailDecrypt(t, dir)

	p := &SOPSProvider{
		FilePath: filepath.Join(dir, "secrets.json"),
		Binary:   bin,
	}

	os.WriteFile(p.FilePath, []byte("{}"), 0644)

	// Set should start with fresh map when decrypt fails.
	// But encryptMap will also fail with this binary, so we expect encrypt error.
	err := p.Set("key", "val")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSOPSProvider_EncryptWithAge(t *testing.T) {
	dir := t.TempDir()

	// Create a fake sops that logs its args to a file.
	argLog := filepath.Join(dir, "args.log")
	bin := filepath.Join(dir, "log-sops")
	script := `#!/bin/sh
echo "$@" >> "` + argLog + `"
# Handle decrypt
for arg in "$@"; do
  case "$arg" in
    --decrypt) cat "$2" 2>/dev/null || echo "{}"; exit 0 ;;
  esac
done
# Handle encrypt: read stdin and write to output
output=""
while [ $# -gt 0 ]; do
  case "$1" in
    --output) shift; output="$1"; shift ;;
    *) shift ;;
  esac
done
if [ -n "$output" ]; then
  cat > "$output"
fi
exit 0
`
	os.WriteFile(bin, []byte(script), 0755)

	filePath := filepath.Join(dir, "secrets.json")

	p := &SOPSProvider{
		FilePath:      filePath,
		Binary:        bin,
		AgeRecipients: "age1abc123",
	}

	if err := p.Set("key", "val"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Check the arg log to verify --age was passed.
	logged, err := os.ReadFile(argLog)
	if err != nil {
		t.Fatalf("ReadFile args log: %v", err)
	}
	if !strings.Contains(string(logged), "--age age1abc123") {
		t.Fatalf("expected --age flag in args, got: %s", string(logged))
	}
}

func TestSOPSProvider_EncryptWithPGP(t *testing.T) {
	dir := t.TempDir()

	argLog := filepath.Join(dir, "args.log")
	bin := filepath.Join(dir, "log-sops")
	script := `#!/bin/sh
echo "$@" >> "` + argLog + `"
for arg in "$@"; do
  case "$arg" in
    --decrypt) cat "$2" 2>/dev/null || echo "{}"; exit 0 ;;
  esac
done
output=""
while [ $# -gt 0 ]; do
  case "$1" in
    --output) shift; output="$1"; shift ;;
    *) shift ;;
  esac
done
if [ -n "$output" ]; then
  cat > "$output"
fi
exit 0
`
	os.WriteFile(bin, []byte(script), 0755)

	filePath := filepath.Join(dir, "secrets.json")

	p := &SOPSProvider{
		FilePath:       filePath,
		Binary:         bin,
		PGPFingerprints: "ABCD1234",
	}

	if err := p.Set("key", "val"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	logged, err := os.ReadFile(argLog)
	if err != nil {
		t.Fatalf("ReadFile args log: %v", err)
	}
	if !strings.Contains(string(logged), "--pgp ABCD1234") {
		t.Fatalf("expected --pgp flag in args, got: %s", string(logged))
	}
}

func TestSOPSProvider_ImplementsProvider(t *testing.T) {
	var p Provider = &SOPSProvider{}
	_ = p
}

func TestIsExitError(t *testing.T) {
	// isExitError with non-ExitError should return false.
	err := errors.New("not an exit error")
	var exErrPtr *os.ProcessState
	_ = exErrPtr

	result := isExitError(err, nil)
	if result {
		t.Fatal("expected false for non-ExitError")
	}
}
