package secrets

import (
	"fmt"
	"sync"
)

// VaultProvider is a stub for HashiCorp Vault secret storage.
// This provider is not yet implemented and returns errors for all operations.
type VaultProvider struct {
	// Address is the Vault server address (e.g. "https://vault.example.com:8200").
	Address string

	// Token is the Vault authentication token.
	Token string

	// MountPath is the secrets engine mount path (e.g. "secret").
	MountPath string

	// SecretPath is the path within the mount (e.g. "data/ralphglasses").
	SecretPath string

	mu sync.Mutex
}

// errNotImplemented is returned for all VaultProvider operations.
var errNotImplemented = fmt.Errorf("vault provider not implemented")

// Get retrieves a secret from Vault. Not yet implemented.
func (p *VaultProvider) Get(key string) (string, error) {
	return "", fmt.Errorf("%w: Get(%q)", errNotImplemented, key)
}

// Set stores a secret in Vault. Not yet implemented.
func (p *VaultProvider) Set(key, value string) error {
	return fmt.Errorf("%w: Set(%q)", errNotImplemented, key)
}

// Delete removes a secret from Vault. Not yet implemented.
func (p *VaultProvider) Delete(key string) error {
	return fmt.Errorf("%w: Delete(%q)", errNotImplemented, key)
}

// List returns all secret keys from Vault. Not yet implemented.
func (p *VaultProvider) List() ([]string, error) {
	return nil, fmt.Errorf("%w: List()", errNotImplemented)
}
