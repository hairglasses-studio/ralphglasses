# crabravee Audit Report

## Summary

crabravee is a well-architected ~6,700 LOC Python MCP server with sophisticated resume scoring (6-dimension BM25-based ATS model, 141-skill taxonomy) and robust API resilience (5-tier fallback chain). The codebase has strong separation of concerns between tools, services, and models. **The single highest-priority improvement is adding a test suite** — zero test coverage on a project with complex scoring algorithms, regex-heavy parsing, and dual-API integration means regressions are invisible. Secondary concerns are duplicated constants across modules and a `storage.py` that can silently corrupt application tracking data.

## Findings

### [1] Zero test coverage (Severity: high)
- **File(s)**: entire repo — no `tests/` directory, no `test_*.py` files
- **Issue**: 6,700 LOC with complex algorithms (BM25 scoring, TF-IDF, regex-based JD parsing, seniority inference, achievement extraction) has no automated tests. CI at `.github/workflows/ci.yml:31` gracefully falls back to `echo "No tests found"`. Any change to scoring weights, skill taxonomy, or parsing regexes could silently break matching accuracy.
- **Fix**: Add `pytest` to dev dependencies. Start with unit tests for the highest-value pure functions: `text_analysis.extract_skills()`, `text_analysis.compute_bm25_score()`, `ats_scorer.score_ats_compatibility()`, `jd_parser.parse_job_description()`, `experience_parser.parse_experience()`, and `achievement_extractor.extract_achievements()`. These are all stateless functions that take text in and return structured data — easy to test.
- **Effort**: large (half day+ for initial suite, but incremental)

### [2] storage.py has no error handling — JSON corruption loses all tracking data (Severity: high)
- **File(s)**: `src/crabrave/services/storage.py:16-30`
- **Issue**: `load_applications()` calls `json.loads()` with no try/except. If `applications.json` gets partially written (crash, disk full, concurrent write), the next load raises `JSONDecodeError` and every tracking tool fails. `save_applications()` uses non-atomic `Path.write_text()` — a crash mid-write truncates the file.
- **Fix**: Wrap `load_applications()` in try/except returning empty dict + log warning on corrupt file. Use atomic write pattern in `save_applications()`: write to a tempfile in the same directory, then `os.replace()` to the target path.
- **Effort**: small

### [3] Seniority rank mapping duplicated in 3 files with inconsistent values (Severity: medium)
- **File(s)**: `src/crabrave/services/ats_scorer.py:43-52`, `src/crabrave/services/experience_parser.py:13-21`, `src/crabrave/tools/search.py:97`
- **Issue**: Three independent seniority rank dicts with different entries. `ats_scorer` has `"lead": 4, "mid-level": 2`; `experience_parser` omits `"lead"` and `"mid-level"`; `search.py` defines its own inline dict with `"staff": 5` (vs 4 in ats_scorer). A seniority change in one file won't propagate to the others, causing inconsistent sort order vs scoring.
- **Fix**: Define `SENIORITY_RANKS` once in `models.py` (next to `Job.inferred_seniority` which produces these labels) and import it everywhere.
- **Effort**: small

### [4] Weak verb lists duplicated between resume.py and ats_scorer.py (Severity: medium)
- **File(s)**: `src/crabrave/tools/resume.py:23-27` (`_WEAK_VERBS` list), `src/crabrave/services/ats_scorer.py:135-138` (`_WEAK_VERBS` set)
- **Issue**: Two independent weak-verb collections with different entries and different data types (list vs set). `resume.py` has "worked on", "was part of" (multi-word); `ats_scorer.py` has single words "worked", "involved". They're used for different purposes (suggestion generation vs scoring penalty) but the divergence means a resume could get flagged in one tool but not the other.
- **Fix**: Consolidate into a single `_WEAK_VERBS` set in `achievement_extractor.py` (which already owns `_ACTION_VERBS`) and import from there. Keep the `_STRONG_REPLACEMENTS` dict in `resume.py` since it's tool-specific.
- **Effort**: small

### [5] HTTP client created per request — no connection pooling (Severity: medium)
- **File(s)**: `src/crabrave/services/http.py:30-32`
- **Issue**: `fetch_json()` creates a new `httpx.AsyncClient()` inside `async with` for every request. Each invocation opens a new TCP connection + TLS handshake. During startup when both Ashby and Greenhouse APIs are queried (plus potential HTML fallback pages), this means 4-8 separate connection establishments.
- **Fix**: Create a module-level `httpx.AsyncClient` with connection pooling and reuse it. Add a `close()` coroutine for graceful shutdown. The client is already async-safe.
- **Effort**: small

### [6] `_ALIAS_TO_CANONICAL` and private constants exported across module boundaries (Severity: medium)
- **File(s)**: `src/crabrave/services/text_analysis.py` (exports `_ALIAS_TO_CANONICAL`), `src/crabrave/services/achievement_extractor.py` (exports `_ACTION_VERBS`, `_METRIC_PATTERNS`)
- **Issue**: Multiple modules import underscore-prefixed "private" constants from other modules (`ats_scorer.py:31` imports `_ALIAS_TO_CANONICAL`, `resume.py:11` imports `_ACTION_VERBS`). This creates hidden coupling where renaming a "private" constant breaks downstream imports.
- **Fix**: Remove the underscore prefix from constants that are part of the module's public API: `ALIAS_TO_CANONICAL`, `ACTION_VERBS`, `METRIC_PATTERNS`. Or export them via `__all__`.
- **Effort**: small

### [7] `Job.inferred_seniority` returns mixed-case labels that don't match rank dict keys (Severity: medium)
- **File(s)**: `src/crabrave/models.py:82-99`, `src/crabrave/services/ats_scorer.py:281-282`
- **Issue**: `Job.inferred_seniority` returns title-cased strings like `"Director+"`, `"Staff"`, `"Mid-Level"`. But `_SENIORITY_RANKS` uses lowercase keys (`"director+"`, `"staff"`, `"mid-level"`). Every consumer must call `.lower()` to look up the rank. `search.py:110` does this correctly, but any new consumer that forgets `.lower()` will always get the default rank (2/mid).
- **Fix**: Have `inferred_seniority` return lowercase, or add a `seniority_rank` property on `Job` that does the lookup once.
- **Effort**: small

### [8] No dev dependencies defined — no linter/formatter/test framework in pyproject.toml (Severity: medium)
- **File(s)**: `pyproject.toml:7-11`
- **Issue**: Only runtime deps (`mcp[cli]`, `httpx`, `pydantic`) are declared. No `[project.optional-dependencies]` for dev tools. CI installs `ruff` via bare `pip install ruff` and attempts `pip install -e ".[dev]"` which fails silently. New contributors have no way to know which tools to install.
- **Fix**: Add `[project.optional-dependencies] dev = ["pytest", "pytest-asyncio", "ruff"]` and update CI to use `pip install -e ".[dev]"`.
- **Effort**: small

### [9] `compute_bm25_score` uses hardcoded `avg_dl=25.0` regardless of actual corpus (Severity: low)
- **File(s)**: `src/crabrave/services/text_analysis.py` (BM25 function)
- **Issue**: BM25's document-length normalization uses a fixed `avg_dl=25.0` ("typical resume skill count") rather than computing from the actual corpus. If Anthropic's job descriptions are significantly longer or shorter than this assumption, the length normalization will over- or under-penalize.
- **Fix**: Accept `avg_dl` as a parameter with `None` default. When `None`, compute from the actual document vectors passed in. Fall back to 25.0 only if no corpus is available.
- **Effort**: small

### [10] CLAUDE.md says "27 MCP tools" and lists `http.py` and `experience_parser.py` inconsistently (Severity: low)
- **File(s)**: `CLAUDE.md` project structure section
- **Issue**: CLAUDE.md's file tree omits `services/http.py` and `services/experience_parser.py` entirely. The tool count "27 MCP tools across 7 categories" should be verified — actual count may differ. The "resume_parser.py" description says "Resume file reading" but the file also handles priority resolution logic.
- **Fix**: Add `http.py` and `experience_parser.py` to the project structure. Verify actual tool count by grepping `@mcp.tool()` decorators.
- **Effort**: small

### [11] `fetch_json` retry logic duplicates warning log code (Severity: low)
- **File(s)**: `src/crabrave/services/http.py:36-78`
- **Issue**: The retry/backoff logic for `HTTPStatusError` (lines 36-58) and `RequestError` (lines 59-78) is nearly identical — same delay calculation, same log format strings, same final-attempt branch. This is the kind of duplication that leads to one branch getting a fix while the other doesn't.
- **Fix**: Extract the retry/backoff into a single except clause catching `(httpx.HTTPStatusError, httpx.RequestError)`, with the 4xx-non-retry check as an early `if isinstance()` guard.
- **Effort**: small

### [12] Resume parser only supports .txt/.md — ignores PDF files in resume/ directory (Severity: low)
- **File(s)**: `src/crabrave/services/resume_parser.py:14`
- **Issue**: `list_resume_files()` filters for `{".txt", ".md"}` only. The `resume/` directory actually contains PDF files (`Mitch_Burk_Resume_2026.pdf`, `Mitch Burk - Resume 2025.pdf`). Users who drop a PDF resume into the directory won't see it picked up.
- **Fix**: Note this as a known limitation in the tool's docstring, or add basic PDF text extraction via `pdfminer.six` or similar lightweight library. Given the CLAUDE.md states "future tools in Go," a docstring note may be more appropriate.
- **Effort**: medium (if adding PDF support) / small (if documenting limitation)

## CLAUDE.md Accuracy

| Section | Status | Issue |
|---------|--------|-------|
| Project Structure | **Outdated** | Missing `services/http.py` (HTTP retry client) and `services/experience_parser.py` (date/seniority parsing). Both are significant modules (80 and 326 LOC). |
| Tool count "27 MCP tools" | **Unverified** | Should be verified against actual `@mcp.tool()` count; may have drifted. |
| "141 skills" in skill taxonomy | **Possibly outdated** | Agent reports found 186+ skills — taxonomy may have grown since CLAUDE.md was last updated. |
| Key Patterns | **Accurate** | Lazy import, text returns, UnifiedJobClient singleton — all confirmed. |
| MCP Resources "7" | **Accurate** | Confirmed 7 resources (though `crabrave://resume/{filename}` and `crabrave://resume` might count as 1 or 2 depending on perspective). |
| Dependencies | **Accurate** | `mcp[cli]`, `httpx`, `pydantic` confirmed in pyproject.toml. |
| Future Direction ("Go") | **Accurate** | Matches stated direction; no Go code exists yet. |

**Suggested additions to CLAUDE.md project structure:**
```
│   ├── services/
│   │   ├── http.py            # Async HTTP with retry + exponential backoff
│   │   ├── experience_parser.py # Resume date/seniority/career trajectory parsing
```

## Recommended Next Actions

1. **Add atomic writes + error handling to `storage.py`** — 15 minutes, prevents silent data loss in application tracking (Finding #2)
2. **Consolidate seniority ranks into a single source of truth in `models.py`** — 20 minutes, eliminates 3-way inconsistency across scoring, sorting, and parsing (Finding #3)
3. **Add `[project.optional-dependencies]` dev section and a minimal pytest suite** — 2 hours for scaffolding + first 10 tests on `extract_skills()` and `parse_job_description()`, unlocks CI-enforced regression detection (Findings #1, #8)
