package session

import (
	"context"
	"sort"
	"strings"
	"time"
)

const (
	// UnassignedRoleName is used when a session has no explicit agent/role tag.
	UnassignedRoleName = "unassigned"
)

// RoleLeaderboardOptions controls tenant role leaderboard generation.
type RoleLeaderboardOptions struct {
	Limit        int  `json:"limit,omitempty"`
	IncludeEnded bool `json:"include_ended,omitempty"`
}

// RoleLeaderboardEntry summarizes one role/agent bucket.
type RoleLeaderboardEntry struct {
	Role        string  `json:"role"`
	Sessions    int     `json:"sessions"`
	Active      int     `json:"active"`
	Completed   int     `json:"completed"`
	Errored     int     `json:"errored"`
	Stopped     int     `json:"stopped"`
	Interrupted int     `json:"interrupted"`
	SpendUSD    float64 `json:"spend_usd"`
	Turns       int     `json:"turns"`
}

// TenantRoleLeaderboard is the per-tenant batch output returned by the admin surfaces.
type TenantRoleLeaderboard struct {
	TenantID      string                 `json:"tenant_id"`
	DisplayName   string                 `json:"display_name,omitempty"`
	GeneratedAt   time.Time              `json:"generated_at"`
	TotalSessions int                    `json:"total_sessions"`
	Roles         []RoleLeaderboardEntry `json:"roles"`
}

type roleLeaderboardSession struct {
	ID     string
	Tenant string
	Role   string
	Status SessionStatus
	Spend  float64
	Turns  int
}

func normalizeRoleLeaderboardOptions(opts RoleLeaderboardOptions) RoleLeaderboardOptions {
	if opts.Limit < 0 {
		opts.Limit = 0
	}
	if opts.Limit == 0 {
		opts.Limit = 10
	}
	return opts
}

func normalizeRoleLeaderboardName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return UnassignedRoleName
	}
	return name
}

func snapshotRoleLeaderboardSession(sess *Session) roleLeaderboardSession {
	sess.Lock()
	defer sess.Unlock()
	return roleLeaderboardSession{
		ID:     sess.ID,
		Tenant: NormalizeTenantID(sess.TenantID),
		Role:   normalizeRoleLeaderboardName(sess.AgentName),
		Status: sess.Status,
		Spend:  sess.SpentUSD,
		Turns:  sess.TurnCount,
	}
}

func (m *Manager) tenantSessionsForRoleLeaderboard(ctx context.Context, tenantID string, includeEnded bool) ([]roleLeaderboardSession, error) {
	tenantID = NormalizeTenantID(tenantID)
	byID := make(map[string]roleLeaderboardSession)

	for _, sess := range m.ListByTenant("", tenantID) {
		snap := snapshotRoleLeaderboardSession(sess)
		byID[snap.ID] = snap
	}

	if includeEnded && m.store != nil {
		stored, err := m.store.ListSessions(ctx, ListOpts{TenantID: tenantID})
		if err != nil {
			return nil, err
		}
		for _, sess := range stored {
			snap := snapshotRoleLeaderboardSession(sess)
			if _, exists := byID[snap.ID]; exists {
				continue
			}
			byID[snap.ID] = snap
		}
	}

	out := make([]roleLeaderboardSession, 0, len(byID))
	for _, snap := range byID {
		out = append(out, snap)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// BuildRoleLeaderboard returns the top role/agent leaderboard for one tenant.
func (m *Manager) BuildRoleLeaderboard(ctx context.Context, tenantID string, opts RoleLeaderboardOptions) (*TenantRoleLeaderboard, error) {
	opts = normalizeRoleLeaderboardOptions(opts)
	tenantID = NormalizeTenantID(tenantID)

	tenant := &Tenant{ID: tenantID, DisplayName: tenantID}
	if existing, err := m.GetTenant(ctx, tenantID); err == nil {
		tenant = existing
	} else if err != nil && err != ErrTenantNotFound {
		return nil, err
	}

	sessions, err := m.tenantSessionsForRoleLeaderboard(ctx, tenantID, opts.IncludeEnded)
	if err != nil {
		return nil, err
	}

	byRole := make(map[string]*RoleLeaderboardEntry)
	for _, snap := range sessions {
		entry := byRole[snap.Role]
		if entry == nil {
			entry = &RoleLeaderboardEntry{Role: snap.Role}
			byRole[snap.Role] = entry
		}
		entry.Sessions++
		entry.SpendUSD += snap.Spend
		entry.Turns += snap.Turns
		switch snap.Status {
		case StatusCompleted:
			entry.Completed++
		case StatusErrored:
			entry.Errored++
		case StatusStopped:
			entry.Stopped++
		case StatusInterrupted:
			entry.Interrupted++
		default:
			entry.Active++
		}
	}

	entries := make([]RoleLeaderboardEntry, 0, len(byRole))
	for _, entry := range byRole {
		entries = append(entries, *entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Sessions != entries[j].Sessions {
			return entries[i].Sessions > entries[j].Sessions
		}
		if entries[i].SpendUSD != entries[j].SpendUSD {
			return entries[i].SpendUSD > entries[j].SpendUSD
		}
		if entries[i].Turns != entries[j].Turns {
			return entries[i].Turns > entries[j].Turns
		}
		return entries[i].Role < entries[j].Role
	})
	if opts.Limit > 0 && len(entries) > opts.Limit {
		entries = entries[:opts.Limit]
	}

	return &TenantRoleLeaderboard{
		TenantID:      tenant.ID,
		DisplayName:   tenant.DisplayName,
		GeneratedAt:   time.Now().UTC(),
		TotalSessions: len(sessions),
		Roles:         entries,
	}, nil
}

// BuildRoleLeaderboards returns role leaderboards for all known tenants.
func (m *Manager) BuildRoleLeaderboards(ctx context.Context, opts RoleLeaderboardOptions) ([]*TenantRoleLeaderboard, error) {
	tenantMap := make(map[string]*Tenant)

	tenants, err := m.ListTenants(ctx)
	if err != nil {
		return nil, err
	}
	for _, tenant := range tenants {
		if tenant == nil {
			continue
		}
		id := NormalizeTenantID(tenant.ID)
		cp := *tenant
		tenantMap[id] = &cp
	}

	m.sessionsMu.RLock()
	for _, sess := range m.sessions {
		id := NormalizeTenantID(sess.TenantID)
		if _, ok := tenantMap[id]; ok {
			continue
		}
		tenantMap[id] = &Tenant{ID: id, DisplayName: id}
	}
	m.sessionsMu.RUnlock()

	ids := make([]string, 0, len(tenantMap))
	for id := range tenantMap {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	results := make([]*TenantRoleLeaderboard, 0, len(ids))
	for _, id := range ids {
		board, err := m.BuildRoleLeaderboard(ctx, id, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, board)
	}
	return results, nil
}
