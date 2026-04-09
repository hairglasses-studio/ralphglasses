# Task 05: WM-4 — Existing-Equivalent Detection Before New Repos

**ROADMAP ID**: WM-4  
**Priority**: P1 | **Size**: S  
**Assigned to**: openrouter/free agent

---

## Goal

Before proposing to add a new repo to the scan path, detect if an equivalent repo already exists to prevent duplicates.

## Acceptance Criteria (from ROADMAP)

> **Acceptance:** `repo_add` warns when an existing repo has ≥70% name/path similarity to the proposed repo

## Context

The `ralphglasses_repo_add` MCP tool adds repos to the scan path. Currently it doesn't check for near-duplicates. We need heuristic matching to detect when the user is about to add something very similar to what's already tracked.

## Similarity Heuristics

1. **Exact path match** — same absolute path = duplicate (block)
2. **Same directory name** — e.g. adding `/new/path/myrepo` when `/old/path/myrepo` exists = high similarity
3. **Levenshtein distance on repo name** — normalize names (strip hyphens/underscores), compare
4. **Remote URL match** — if git remote URLs match = duplicate (block)

## Files to Create/Modify

### New files:
- `internal/roadmap/dedup.go` — `FindSimilarRepos(candidate string, existing []string) []SimilarRepo`
- `internal/roadmap/dedup_test.go` — unit tests

### Modified files:
- `internal/mcpserver/handler_repo.go` (or wherever `repo_add` is handled) — call `FindSimilarRepos` before adding, return warning in response if similarity >= 0.7

## Schema

```go
type SimilarRepo struct {
    ExistingPath string  `json:"existing_path"`
    Similarity   float64 `json:"similarity"` // 0.0-1.0
    Reason       string  `json:"reason"`     // "exact_name", "remote_url", "name_similarity"
}
```

Response when similar repos found:
```json
{
  "warning": "similar repos already tracked",
  "similar": [{"existing_path": "/path/myrepo", "similarity": 0.85, "reason": "exact_name"}],
  "added": false
}
```

## Verification

```bash
go build ./...
go test ./internal/roadmap/... -run TestFindSimilarRepos -v
```
