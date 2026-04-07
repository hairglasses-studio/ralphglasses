package session

// cloneSession returns a detached copy of a session suitable for persistence
// or read-only consumers. Runtime-only process handles are intentionally
// stripped so callers do not retain references back into the live session.
func cloneSession(s *Session) *Session {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSessionLocked(s)
}

// cloneSessionLocked clones a session while the caller holds s.mu.
func cloneSessionLocked(s *Session) *Session {
	clone := &Session{
		ID:                  s.ID,
		TenantID:            s.TenantID,
		Provider:            s.Provider,
		ProviderSessionID:   s.ProviderSessionID,
		RepoPath:            s.RepoPath,
		RepoName:            s.RepoName,
		Status:              s.Status,
		Prompt:              s.Prompt,
		Model:               s.Model,
		EnhancementSource:   s.EnhancementSource,
		EnhancementPreScore: s.EnhancementPreScore,
		AgentName:           s.AgentName,
		TeamName:            s.TeamName,
		SweepID:             s.SweepID,
		PermissionMode:      s.PermissionMode,
		Resumed:             s.Resumed,
		BudgetUSD:           s.BudgetUSD,
		SpentUSD:            s.SpentUSD,
		TurnCount:           s.TurnCount,
		MaxTurns:            s.MaxTurns,
		LaunchedAt:          s.LaunchedAt,
		LastActivity:        s.LastActivity,
		ExitReason:          s.ExitReason,
		LastOutput:          s.LastOutput,
		Error:               s.Error,
		LastEventType:       s.LastEventType,
		StreamParseErrors:   s.StreamParseErrors,
		CostSource:          s.CostSource,
		CacheReadTokens:     s.CacheReadTokens,
		CacheWriteTokens:    s.CacheWriteTokens,
		CacheAnomaly:        s.CacheAnomaly,
		TotalOutputCount:    s.TotalOutputCount,
		Pid:                 s.Pid,
		ParentID:            s.ParentID,
		ForkPoint:           s.ForkPoint,
	}

	if s.EndedAt != nil {
		endedAt := *s.EndedAt
		clone.EndedAt = &endedAt
	}
	if len(s.CostHistory) > 0 {
		clone.CostHistory = append([]float64(nil), s.CostHistory...)
	}
	if len(s.OutputHistory) > 0 {
		clone.OutputHistory = append([]string(nil), s.OutputHistory...)
	}
	if len(s.ChildPids) > 0 {
		clone.ChildPids = append([]int(nil), s.ChildPids...)
	}
	if len(s.ChildIDs) > 0 {
		clone.ChildIDs = append([]string(nil), s.ChildIDs...)
	}

	return clone
}
