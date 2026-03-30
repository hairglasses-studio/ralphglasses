package envkit

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// AuditEntry records a single environment variable access.
type AuditEntry struct {
	VarName   string    `json:"var_name"`
	Component string    `json:"component"`
	Timestamp time.Time `json:"timestamp"`
	WasSet    bool      `json:"was_set"`
	// Value is intentionally omitted to avoid leaking secrets.
}

// EnvAuditor tracks environment variable reads across components.
// It is safe for concurrent use.
type EnvAuditor struct {
	mu      sync.Mutex
	entries []AuditEntry
	nowFn   func() time.Time // injectable clock for testing
}

// NewEnvAuditor creates an EnvAuditor that uses the real clock.
func NewEnvAuditor() *EnvAuditor {
	return &EnvAuditor{
		nowFn: time.Now,
	}
}

// newEnvAuditorWithClock creates an EnvAuditor with an injectable clock.
// Exported only for package-level tests.
func newEnvAuditorWithClock(nowFn func() time.Time) *EnvAuditor {
	return &EnvAuditor{
		nowFn: nowFn,
	}
}

// Get reads an environment variable and records the access.
// component identifies the caller (e.g. "session.runner", "provider.claude").
func (a *EnvAuditor) Get(varName, component string) string {
	value := os.Getenv(varName)
	a.record(varName, component, value != "")
	return value
}

// Lookup reads an environment variable and records the access,
// returning both the value and whether it was set.
func (a *EnvAuditor) Lookup(varName, component string) (string, bool) {
	value, ok := os.LookupEnv(varName)
	a.record(varName, component, ok)
	return value, ok
}

// record appends an audit entry under the lock.
func (a *EnvAuditor) record(varName, component string, wasSet bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, AuditEntry{
		VarName:   varName,
		Component: component,
		Timestamp: a.nowFn(),
		WasSet:    wasSet,
	})
}

// AuditLog returns a timestamped log of all recorded env var accesses,
// ordered chronologically. Each line has the format:
//
//	2024-01-15T10:30:00Z [component] VAR_NAME (set|missing)
func (a *EnvAuditor) AuditLog() string {
	a.mu.Lock()
	snapshot := make([]AuditEntry, len(a.entries))
	copy(snapshot, a.entries)
	a.mu.Unlock()

	if len(snapshot) == 0 {
		return ""
	}

	var b strings.Builder
	for _, e := range snapshot {
		status := "set"
		if !e.WasSet {
			status = "missing"
		}
		fmt.Fprintf(&b, "%s [%s] %s (%s)\n",
			e.Timestamp.UTC().Format(time.RFC3339),
			e.Component,
			e.VarName,
			status,
		)
	}
	return b.String()
}

// Entries returns a copy of all recorded entries.
func (a *EnvAuditor) Entries() []AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]AuditEntry, len(a.entries))
	copy(out, a.entries)
	return out
}

// MissingVars returns the names of env vars that were accessed but not set,
// deduplicated and sorted alphabetically.
func (a *EnvAuditor) MissingVars() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	seen := make(map[string]struct{})
	for _, e := range a.entries {
		if !e.WasSet {
			seen[e.VarName] = struct{}{}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Components returns the set of component names that have recorded accesses,
// sorted alphabetically.
func (a *EnvAuditor) Components() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	seen := make(map[string]struct{})
	for _, e := range a.entries {
		seen[e.Component] = struct{}{}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Reset clears all recorded entries.
func (a *EnvAuditor) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = nil
}
