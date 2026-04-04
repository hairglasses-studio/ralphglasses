# Knowledge Consolidation & Branch Restructuring

## Context
Mitch has been running a job search on the `mitch` branch while Austin runs on `main`. The mitch branch has 62 Claude memory files, 21 company data dirs, and a 104MB LFS database export from a MacBook migration. The user wants to consolidate everything onto main, archive Austin's state, and make main Mitch's primary branch going forward. Stale roles (rejected/declined) and expired project memories should be pruned.

**Branch state:** mitch is 6 ahead / 9 behind main. One `.gitignore` conflict. No code conflicts.

---

## Phase 1: Clean up mitch branch

### 1.1 Check out mitch
```bash
git checkout -b mitch origin/mitch
```

### 1.2 Remove stale data files
| File | Reason |
|------|--------|
| `jobb-export-20260331.db` | 104MB LFS migration artifact |
| `.gitattributes` | LFS config for removed db |
| `data/pipeline-ranked-2026-03-19.md` | Outdated snapshot |
| `data/companies/workato/` | Rejected |
| `data/companies/replit/` | Declined |
| `data/companies/staff-software-engineer-systems-infrastructure-at-linkedin/` | Badly named duplicate |

### 1.3 Remove stale `.claude/memory/` files (10 files)
| File | Reason |
|------|--------|
| `project_week_mar16_availability.md` | Past dates |
| `project_galileo_remote.md` | Past week |
| `project_linkedin_messages_2026_03_19.md` | Old triage |
| `project_last_stand_apply_list.md` | Stale |
| `project_handoff.md` | Replacing with updated version |
| `project_blitz_improvements.md` | Resolved |
| `project_blitz_tool_errors.md` | Resolved |
| `project_tool_improvements.md` | Superseded |
| `project_tool_improvements_blitz.md` | Resolved |
| `project_tool_improvements_gmail.md` | Resolved |

### 1.4 Promote 2 general feedback memories from `docs/claude-memories/` to `.claude/memory/`
- `feedback_no_emdashes.md` — no em dashes in resumes
- `feedback_no_playwright_linkedin.md` — prefer Voyager API over Playwright

### 1.5 Rewrite `project_branching_model.md`
New model: `main` = Mitch (active), `austin` = archived, `michelle` = active on origin, `jon` = future

### 1.6 Rewrite `MEMORY.md` index
- Remove deleted file references
- Add promoted feedback files
- Update section headers (no longer "mitch branch" — just "Mitch")
- Note Austin archived to `austin` branch

### 1.7 Commit and push
```
chore: consolidate memories and data for main merge
```

---

## Phase 2: Archive Austin to `austin` branch

```bash
git checkout main
git pull origin main
git checkout -b austin
git push origin austin
git checkout main
```

---

## Phase 3: Merge mitch into main

### 3.1 Merge
```bash
git merge mitch
```

### 3.2 Resolve `.gitignore` conflict
Combine both: keep `.env.*` and `.envrc` from main + `data/*.csv`, ralph runtime, claude worktrees from mitch.

### 3.3 Post-merge cleanup (if needed)
Remove any stale data dirs that existed on main but were already cleaned on mitch (replit, duplicate linkedin dir, pipeline snapshot).

### 3.4 Commit and push
```bash
git push origin main
```

---

## Phase 4: Archive mitch branch

```bash
git branch -d mitch
git push origin --delete mitch
git checkout main
```

---

## Final State

| Branch | Purpose |
|--------|---------|
| `main` | Mitch's active search, ~52 memory files, 18 active company dirs |
| `austin` | Archived snapshot of Austin's pre-merge main |
| `origin/michelle` | Untouched |

**Preserved:** `docs/claude-memories/` (44 files including Michelle's data) as reference archive.
**Removed:** LFS db, 10 stale memories, 3 declined/rejected company dirs, outdated pipeline snapshot.
**Promoted:** 2 general feedback memories to active `.claude/memory/`.

## Verification
- `git log --oneline -5` shows merge commit on main
- `git branch -a` shows austin exists, mitch is gone
- `ls .claude/memory/ | wc -l` shows ~52 files
- `ls data/companies/ | wc -l` shows ~18 dirs (no workato, replit, duplicate linkedin)
- `git lfs ls-files` shows no tracked LFS files
- `go build ./...` still compiles
- `go test -race -count=1 ./...` passes
