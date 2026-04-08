package fleet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const coordinationDirEnv = "RALPHGLASSES_COORD_DIR"

var (
	// CoordDir is the shared coordination directory for ralphglasses instances.
	// It prefers an explicit override, then XDG/user-state-backed runtime paths,
	// and can still be overridden directly in tests.
	CoordDir = defaultCoordDir()
)

const claimsSubdir = "claims"

// Claim represents a resource lock held by an agent session.
type Claim struct {
	Agent     string    `json:"agent"`
	Resource  string    `json:"resource"`
	Timestamp time.Time `json:"timestamp"`
}

func defaultCoordDir() string {
	if override := strings.TrimSpace(os.Getenv(coordinationDirEnv)); override != "" {
		return override
	}
	if xdgRuntime := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); xdgRuntime != "" {
		return filepath.Join(xdgRuntime, "ralphglasses", "coordination")
	}
	if homeDir, err := os.UserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
		return filepath.Join(homeDir, ".ralphglasses", "coordination")
	}
	return filepath.Join(os.TempDir(), "ralphglasses-coordination")
}

// EnsureCoordDir creates the coordination directory structure.
func EnsureCoordDir() error {
	dirs := []string{
		CoordDir,
		filepath.Join(CoordDir, claimsSubdir),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create coordination dir %s: %w", d, err)
		}
	}
	return nil
}

// claimFileName returns a deterministic filename for a resource claim
// based on the SHA-256 hash of the resource string (safe for any path).
func claimFileName(resource string) string {
	h := sha256.Sum256([]byte(resource))
	return hex.EncodeToString(h[:16]) + ".json"
}

// ClaimResource creates a claim file for a resource using atomic write.
func ClaimResource(agent, resource string) error {
	if err := EnsureCoordDir(); err != nil {
		return err
	}

	claim := Claim{
		Agent:     agent,
		Resource:  resource,
		Timestamp: time.Now(),
	}
	data, err := json.MarshalIndent(claim, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal claim: %w", err)
	}

	claimDir := filepath.Join(CoordDir, claimsSubdir)
	target := filepath.Join(claimDir, claimFileName(resource))

	// Atomic write: temp file + rename.
	tmp, err := os.CreateTemp(claimDir, "claim-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp claim file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write claim: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close claim: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename claim: %w", err)
	}
	return nil
}

// ReleaseClaim removes the claim file for a resource.
func ReleaseClaim(resource string) error {
	target := filepath.Join(CoordDir, claimsSubdir, claimFileName(resource))
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove claim: %w", err)
	}
	return nil
}

// IsResourceClaimed checks whether a resource has an active claim.
// Returns the claim if one exists, nil otherwise.
func IsResourceClaimed(resource string) (bool, *Claim, error) {
	target := filepath.Join(CoordDir, claimsSubdir, claimFileName(resource))
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("read claim: %w", err)
	}

	var claim Claim
	if err := json.Unmarshal(data, &claim); err != nil {
		return false, nil, fmt.Errorf("unmarshal claim: %w", err)
	}
	return true, &claim, nil
}

// ListClaims returns all active claims in the coordination directory.
func ListClaims() ([]Claim, error) {
	claimDir := filepath.Join(CoordDir, claimsSubdir)
	entries, err := os.ReadDir(claimDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read claims dir: %w", err)
	}

	var claims []Claim
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(claimDir, entry.Name()))
		if err != nil {
			continue // best-effort: skip unreadable files
		}
		var claim Claim
		if err := json.Unmarshal(data, &claim); err != nil {
			continue
		}
		claims = append(claims, claim)
	}
	return claims, nil
}
