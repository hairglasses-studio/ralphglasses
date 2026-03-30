package remote

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// KeyPair holds metadata about a managed SSH key pair.
type KeyPair struct {
	Name        string    `json:"name"`
	PublicKey   string    `json:"public_key"`
	PrivatePath string    `json:"private_path"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

// KeyStore manages SSH Ed25519 key pairs in a directory.
type KeyStore struct {
	dir string
}

// NewKeyStore creates or opens a key store rooted at dir. The directory is
// created with 0700 permissions if it does not exist.
func NewKeyStore(dir string) (*KeyStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("keystore directory must not be empty")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create keystore dir: %w", err)
	}
	return &KeyStore{dir: dir}, nil
}

// GenerateKey generates a new Ed25519 key pair and stores it under name.
// Returns an error if a key with the same name already exists.
func (ks *KeyStore) GenerateKey(name string) (*KeyPair, error) {
	if name == "" {
		return nil, fmt.Errorf("key name must not be empty")
	}
	if strings.ContainsAny(name, "/\\") {
		return nil, fmt.Errorf("key name must not contain path separators")
	}

	privPath := ks.privatePath(name)
	if _, err := os.Stat(privPath); err == nil {
		return nil, fmt.Errorf("key %q already exists", name)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	// Marshal private key to OpenSSH PEM format.
	privPEM, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	if err := os.WriteFile(privPath, pem.EncodeToMemory(privPEM), 0600); err != nil {
		return nil, fmt.Errorf("write private key: %w", err)
	}

	// Marshal public key to authorized_keys format.
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("convert public key: %w", err)
	}
	pubLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + name
	pubPath := ks.publicPath(name)
	if err := os.WriteFile(pubPath, []byte(pubLine+"\n"), 0644); err != nil {
		// Clean up private key on failure.
		_ = os.Remove(privPath)
		return nil, fmt.Errorf("write public key: %w", err)
	}

	return &KeyPair{
		Name:        name,
		PublicKey:   pubLine,
		PrivatePath: privPath,
		Fingerprint: fingerprint(sshPub),
		CreatedAt:   time.Now().UTC(),
	}, nil
}

// ListKeys returns all managed key pairs sorted by name.
func (ks *KeyStore) ListKeys() ([]*KeyPair, error) {
	entries, err := os.ReadDir(ks.dir)
	if err != nil {
		return nil, fmt.Errorf("read keystore dir: %w", err)
	}

	var keys []*KeyPair
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pub") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".pub")
		kp, err := ks.GetKey(name)
		if err != nil {
			continue // skip broken pairs
		}
		keys = append(keys, kp)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Name < keys[j].Name
	})
	return keys, nil
}

// GetKey returns the key pair with the given name.
func (ks *KeyStore) GetKey(name string) (*KeyPair, error) {
	privPath := ks.privatePath(name)
	pubPath := ks.publicPath(name)

	privInfo, err := os.Stat(privPath)
	if err != nil {
		return nil, fmt.Errorf("key %q not found", name)
	}

	pubBytes, err := os.ReadFile(pubPath)
	if err != nil {
		return nil, fmt.Errorf("read public key for %q: %w", name, err)
	}
	pubLine := strings.TrimSpace(string(pubBytes))

	// Parse public key to compute fingerprint.
	sshPub, _, _, _, err := ssh.ParseAuthorizedKey(pubBytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key for %q: %w", name, err)
	}

	return &KeyPair{
		Name:        name,
		PublicKey:   pubLine,
		PrivatePath: privPath,
		Fingerprint: fingerprint(sshPub),
		CreatedAt:   privInfo.ModTime().UTC(),
	}, nil
}

// DeleteKey removes both the private and public key files for name.
func (ks *KeyStore) DeleteKey(name string) error {
	privPath := ks.privatePath(name)
	if _, err := os.Stat(privPath); err != nil {
		return fmt.Errorf("key %q not found", name)
	}

	if err := os.Remove(privPath); err != nil {
		return fmt.Errorf("remove private key: %w", err)
	}
	// Remove public key; ignore error if missing.
	_ = os.Remove(ks.publicPath(name))
	return nil
}

// AuthorizedKeys generates authorized_keys content from all managed public
// keys, one per line.
func (ks *KeyStore) AuthorizedKeys() (string, error) {
	keys, err := ks.ListKeys()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, kp := range keys {
		b.WriteString(kp.PublicKey)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// privatePath returns the filesystem path for the private key file.
func (ks *KeyStore) privatePath(name string) string {
	return filepath.Join(ks.dir, name)
}

// publicPath returns the filesystem path for the public key file.
func (ks *KeyStore) publicPath(name string) string {
	return filepath.Join(ks.dir, name+".pub")
}

// fingerprint computes the SHA-256 fingerprint of an SSH public key,
// matching the format used by ssh-keygen -l.
func fingerprint(key ssh.PublicKey) string {
	return ssh.FingerprintSHA256(key)
}
