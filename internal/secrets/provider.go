// Package secrets provides secret management with pluggable provider backends.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
)

// Provider defines the interface for secret storage backends.
type Provider interface {
	// Get retrieves a secret by key. Returns an error if the key does not exist.
	Get(key string) (string, error)
	// Set stores a secret. Overwrites any existing value for the key.
	Set(key, value string) error
	// Delete removes a secret by key.
	Delete(key string) error
	// List returns all available secret keys.
	List() ([]string, error)
}

// ErrNotFound is returned when a secret key does not exist.
var ErrNotFound = errors.New("secret not found")

// ErrNoProviders is returned when no provider can satisfy a Get request.
var ErrNoProviders = errors.New("no providers registered")

// ErrDuplicateProvider is returned when registering a provider name that already exists.
var ErrDuplicateProvider = errors.New("provider already registered")

// registryEntry pairs a provider with its registration order for priority.
type registryEntry struct {
	name     string
	provider Provider
	order    int
}

// Registry manages multiple secret providers and queries them in registration order.
type Registry struct {
	mu      sync.RWMutex
	entries []registryEntry
	next    int
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a provider. Providers are queried in the order they are registered.
func (r *Registry) Register(name string, p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, e := range r.entries {
		if e.name == name {
			return fmt.Errorf("%w: %s", ErrDuplicateProvider, name)
		}
	}

	r.entries = append(r.entries, registryEntry{
		name:     name,
		provider: p,
		order:    r.next,
	})
	r.next++
	return nil
}

// Get tries each provider in priority (registration) order, returning the first successful result.
func (r *Registry) Get(key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.entries) == 0 {
		return "", ErrNoProviders
	}

	sorted := make([]registryEntry, len(r.entries))
	copy(sorted, r.entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].order < sorted[j].order })

	var lastErr error
	for _, e := range sorted {
		v, err := e.provider.Get(key)
		if err == nil {
			return v, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("key %q: %w", key, lastErr)
}

// Providers returns the names of all registered providers.
func (r *Registry) Providers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, len(r.entries))
	for i, e := range r.entries {
		names[i] = e.name
	}
	return names
}

// --- EnvProvider ---

// EnvProvider reads secrets from environment variables.
// Keys are upper-cased and prefixed.
type EnvProvider struct {
	// Prefix is prepended to keys when looking up env vars (e.g. "RALPH_SECRET_").
	// If empty, keys are used as-is (upper-cased).
	Prefix string
}

func (p *EnvProvider) envKey(key string) string {
	return p.Prefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// Get reads the environment variable for the given key.
func (p *EnvProvider) Get(key string) (string, error) {
	v, ok := os.LookupEnv(p.envKey(key))
	if !ok {
		return "", fmt.Errorf("%w: env %s", ErrNotFound, p.envKey(key))
	}
	return v, nil
}

// Set sets the environment variable.
func (p *EnvProvider) Set(key, value string) error {
	return os.Setenv(p.envKey(key), value)
}

// Delete unsets the environment variable.
func (p *EnvProvider) Delete(key string) error {
	return os.Unsetenv(p.envKey(key))
}

// List returns keys from the environment that match the prefix.
func (p *EnvProvider) List() ([]string, error) {
	prefix := p.Prefix
	var keys []string
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := parts[0]
		if prefix == "" || strings.HasPrefix(name, prefix) {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// --- FileProvider ---

// FileProvider reads and writes secrets to an AES-256-GCM encrypted JSON file.
// The encryption key is derived from a passphrase using SHA-256.
type FileProvider struct {
	Path       string // Path to the encrypted file (e.g. ".secrets.enc").
	Passphrase string // Passphrase used to derive the AES-256 key.

	mu sync.Mutex
}

func (p *FileProvider) deriveKey() []byte {
	h := sha256.Sum256([]byte(p.Passphrase))
	return h[:]
}

func (p *FileProvider) encrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.deriveKey())
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, data, nil), nil
}

func (p *FileProvider) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.deriveKey())
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func (p *FileProvider) load() (map[string]string, error) {
	data, err := os.ReadFile(p.Path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read secrets file: %w", err)
	}
	if len(data) == 0 {
		return make(map[string]string), nil
	}

	plain, err := p.decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("decrypt secrets: %w", err)
	}

	var m map[string]string
	if err := json.Unmarshal(plain, &m); err != nil {
		return nil, fmt.Errorf("unmarshal secrets: %w", err)
	}
	return m, nil
}

func (p *FileProvider) save(m map[string]string) error {
	plain, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal secrets: %w", err)
	}

	enc, err := p.encrypt(plain)
	if err != nil {
		return err
	}

	return os.WriteFile(p.Path, enc, 0600)
}

// Get retrieves a secret from the encrypted file.
func (p *FileProvider) Get(key string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	m, err := p.load()
	if err != nil {
		return "", err
	}

	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return v, nil
}

// Set writes a secret to the encrypted file.
func (p *FileProvider) Set(key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	m, err := p.load()
	if err != nil {
		return err
	}

	m[key] = value
	return p.save(m)
}

// Delete removes a secret from the encrypted file.
func (p *FileProvider) Delete(key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	m, err := p.load()
	if err != nil {
		return err
	}

	if _, ok := m[key]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}

	delete(m, key)
	return p.save(m)
}

// List returns all keys in the encrypted file.
func (p *FileProvider) List() ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	m, err := p.load()
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}
