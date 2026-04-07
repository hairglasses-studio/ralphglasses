package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// DefaultTenantID preserves legacy single-tenant behavior.
	DefaultTenantID = "_default"
)

var tenantIDSanitizer = regexp.MustCompile(`[^a-z0-9._-]+`)

// Tenant represents an isolated workspace tenant for shared-host operation.
type Tenant struct {
	ID               string    `json:"id"`
	DisplayName      string    `json:"display_name,omitempty"`
	AllowedRepoRoots []string  `json:"allowed_repo_roots,omitempty"`
	BudgetCapUSD     float64   `json:"budget_cap_usd,omitempty"`
	TriggerTokenHash string    `json:"trigger_token_hash,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// NormalizeTenantID converts a caller-provided tenant identifier into the
// canonical storage form. Empty input resolves to the legacy default tenant.
func NormalizeTenantID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return DefaultTenantID
	}
	id = tenantIDSanitizer.ReplaceAllString(id, "-")
	id = strings.Trim(id, "-.")
	if id == "" {
		return DefaultTenantID
	}
	return id
}

func sanitizeTenantPathSegment(id string) string {
	id = NormalizeTenantID(id)
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, `\`, "-")
	if id == "" {
		return DefaultTenantID
	}
	return id
}

func normalizeRepoRoots(roots []string) []string {
	if len(roots) == 0 {
		return nil
	}
	out := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if abs, err := filepath.Abs(root); err == nil {
			root = filepath.Clean(abs)
		} else {
			root = filepath.Clean(root)
		}
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		out = append(out, root)
	}
	return out
}

// Normalize mutates the tenant into canonical storage form.
func (t *Tenant) Normalize() {
	if t == nil {
		return
	}
	t.ID = NormalizeTenantID(t.ID)
	t.DisplayName = strings.TrimSpace(t.DisplayName)
	if t.DisplayName == "" {
		t.DisplayName = t.ID
	}
	t.AllowedRepoRoots = normalizeRepoRoots(t.AllowedRepoRoots)
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
}

// AllowsRepoPath reports whether the tenant may access the given repo path.
// An empty allowlist remains permissive for additive rollout compatibility.
func (t *Tenant) AllowsRepoPath(repoPath string) bool {
	if t == nil {
		return true
	}
	if len(t.AllowedRepoRoots) == 0 {
		return true
	}
	cleanRepo, err := filepath.Abs(repoPath)
	if err != nil {
		cleanRepo = filepath.Clean(repoPath)
	} else {
		cleanRepo = filepath.Clean(cleanRepo)
	}
	for _, root := range t.AllowedRepoRoots {
		if root == "" {
			continue
		}
		if cleanRepo == root || strings.HasPrefix(cleanRepo, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// DefaultTenant returns the implicit legacy tenant configuration.
func DefaultTenant() *Tenant {
	now := time.Now().UTC()
	return &Tenant{
		ID:          DefaultTenantID,
		DisplayName: "Default",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func teamStorageKey(tenantID, name string) string {
	tenantID = NormalizeTenantID(tenantID)
	name = strings.TrimSpace(name)
	if tenantID == DefaultTenantID {
		return name
	}
	return tenantID + ":" + name
}

func normalizeTenantRef(tenantID string) string {
	return NormalizeTenantID(tenantID)
}

// HashTenantTriggerToken returns the stable hash stored for a tenant trigger token.
func HashTenantTriggerToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

// VerifyTenantTriggerToken compares a stored hash to a caller-provided token.
func VerifyTenantTriggerToken(hash, token string) bool {
	if hash == "" || strings.TrimSpace(token) == "" {
		return false
	}
	expected := HashTenantTriggerToken(token)
	return subtle.ConstantTimeCompare([]byte(hash), []byte(expected)) == 1
}

// NewTenantTriggerToken generates a new random trigger bearer token.
func NewTenantTriggerToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate trigger token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func (m *Manager) resolveTenant(ctx context.Context, tenantID string) (*Tenant, error) {
	tenantID = NormalizeTenantID(tenantID)
	if tenantID == DefaultTenantID || m.store == nil {
		return DefaultTenant(), nil
	}
	tenant, err := m.store.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return tenant, nil
}

// SaveTenant normalizes and persists a tenant definition.
func (m *Manager) SaveTenant(ctx context.Context, tenant *Tenant) (*Tenant, error) {
	if m.store == nil {
		return nil, fmt.Errorf("tenant store not configured")
	}
	if tenant == nil {
		return nil, fmt.Errorf("tenant is nil")
	}
	cp := *tenant
	cp.Normalize()
	if err := m.store.SaveTenant(ctx, &cp); err != nil {
		return nil, err
	}
	return &cp, nil
}

// GetTenant returns a persisted tenant definition.
func (m *Manager) GetTenant(ctx context.Context, tenantID string) (*Tenant, error) {
	if NormalizeTenantID(tenantID) == DefaultTenantID && m.store == nil {
		return DefaultTenant(), nil
	}
	if m.store == nil {
		return nil, ErrTenantNotFound
	}
	return m.store.GetTenant(ctx, tenantID)
}

// ListTenants returns all known tenants, including the implicit default.
func (m *Manager) ListTenants(ctx context.Context) ([]*Tenant, error) {
	if m.store == nil {
		return []*Tenant{DefaultTenant()}, nil
	}
	tenants, err := m.store.ListTenants(ctx)
	if err != nil {
		return nil, err
	}
	if len(tenants) == 0 {
		return []*Tenant{DefaultTenant()}, nil
	}
	return tenants, nil
}

// RotateTenantTriggerToken stores a fresh token hash and returns the plaintext once.
func (m *Manager) RotateTenantTriggerToken(ctx context.Context, tenantID string) (string, *Tenant, error) {
	tenant, err := m.GetTenant(ctx, tenantID)
	if err != nil {
		return "", nil, err
	}
	token, err := NewTenantTriggerToken()
	if err != nil {
		return "", nil, err
	}
	tenant.TriggerTokenHash = HashTenantTriggerToken(token)
	updated, err := m.SaveTenant(ctx, tenant)
	if err != nil {
		return "", nil, err
	}
	return token, updated, nil
}
