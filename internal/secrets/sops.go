package secrets

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// SOPSProvider wraps the mozilla/sops CLI for secret encryption/decryption.
// It supports age and PGP key backends via the sops binary.
type SOPSProvider struct {
	// FilePath is the SOPS-encrypted JSON file path.
	FilePath string

	// Binary is the path to the sops binary. Defaults to "sops" (found via PATH).
	Binary string

	// AgeRecipients is a comma-separated list of age public keys for encryption.
	// If set, --age flag is passed to sops.
	AgeRecipients string

	// PGPFingerprints is a comma-separated list of PGP fingerprints for encryption.
	// If set, --pgp flag is passed to sops.
	PGPFingerprints string

	mu sync.Mutex
}

func (p *SOPSProvider) bin() string {
	if p.Binary != "" {
		return p.Binary
	}
	return "sops"
}

// decryptFile decrypts the SOPS file and returns the parsed JSON map.
func (p *SOPSProvider) decryptFile() (map[string]string, error) {
	cmd := exec.Command(p.bin(), "--decrypt", p.FilePath)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if ok := isExitError(err, &exitErr); ok {
			return nil, fmt.Errorf("sops decrypt: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("sops decrypt: %w", err)
	}

	var m map[string]string
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("unmarshal sops output: %w", err)
	}
	return m, nil
}

// encryptMap encrypts the map and writes it to the SOPS file.
func (p *SOPSProvider) encryptMap(m map[string]string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal secrets: %w", err)
	}

	args := []string{"--encrypt"}
	if p.AgeRecipients != "" {
		args = append(args, "--age", p.AgeRecipients)
	}
	if p.PGPFingerprints != "" {
		args = append(args, "--pgp", p.PGPFingerprints)
	}
	args = append(args, "--input-type", "json", "--output-type", "json", "--output", p.FilePath, "/dev/stdin")

	cmd := exec.Command(p.bin(), args...)
	cmd.Stdin = strings.NewReader(string(data))

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sops encrypt: %s: %w", string(out), err)
	}
	return nil
}

func isExitError(err error, target **exec.ExitError) bool {
	if e, ok := err.(*exec.ExitError); ok {
		*target = e
		return true
	}
	return false
}

// Get retrieves a secret from the SOPS-encrypted file.
func (p *SOPSProvider) Get(key string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	m, err := p.decryptFile()
	if err != nil {
		return "", err
	}

	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return v, nil
}

// Set writes a secret to the SOPS-encrypted file.
func (p *SOPSProvider) Set(key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	m, err := p.decryptFile()
	if err != nil {
		// If the file doesn't exist yet, start fresh.
		m = make(map[string]string)
	}

	m[key] = value
	return p.encryptMap(m)
}

// Delete removes a secret from the SOPS-encrypted file.
func (p *SOPSProvider) Delete(key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	m, err := p.decryptFile()
	if err != nil {
		return err
	}

	if _, ok := m[key]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}

	delete(m, key)
	return p.encryptMap(m)
}

// List returns all secret keys in the SOPS-encrypted file.
func (p *SOPSProvider) List() ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	m, err := p.decryptFile()
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys, nil
}
