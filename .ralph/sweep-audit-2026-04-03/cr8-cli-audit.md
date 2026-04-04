# cr8-cli Audit Report

## Summary

cr8-cli is a maturing media processing and DJ crate management system with a solid MCP integration layer and well-organized module structure. The codebase's highest-priority issue is **committed secrets in git history** — the `.env` file containing live Supabase, SoundCloud, GitHub, Cloudflare, and database credentials is tracked by git despite being in `.gitignore`. Beyond that, the main themes are: inconsistent error handling (bare excepts, swallowed errors, print-over-logging), a broken CLI entry point, dependency declaration drift between `pyproject.toml` and `requirements.txt`, and stale infrastructure artifacts (deprecated Dockerfiles, duplicate migrations).

---

## Findings

### [1] Live secrets committed to git history (Severity: high)
- **File(s)**: `.env` (tracked by git), `.env.test`
- **Issue**: `.env` contains production Supabase keys, SoundCloud OAuth tokens, GitHub PAT, Cloudflare tunnel tokens, database passwords, and Discogs/Beatport credentials. Although `.env` is now in `.gitignore` (line 62), it was committed before that rule existed and remains tracked (`git ls-files .env` confirms). Anyone who clones the repo gets all secrets.
- **Fix**: `git rm --cached .env .env.test` to untrack, then use `git-filter-repo` to purge from history. Rotate every credential in the file immediately. Add `.env.test` to `.gitignore` as well.
- **Effort**: medium (credential rotation is the time sink)

### [2] Broken CLI entry point — missing `cli.py` (Severity: high)
- **File(s)**: `pyproject.toml:71`, `cr8_cli/` (no `cli.py` exists)
- **Issue**: `pyproject.toml` declares `cr8 = "cr8_cli.cli:main"` as a console script, but `cr8_cli/cli.py` does not exist. `pip install -e .` will succeed but `cr8` command will crash with `ModuleNotFoundError`.
- **Fix**: Either create `cr8_cli/cli.py` with a `main()` function (wrapping existing functionality), or update the entry point to point at an existing module.
- **Effort**: small

### [3] Bare `except:` swallows all exceptions including SystemExit (Severity: high)
- **File(s)**: `cr8_cli/storage.py:198`
- **Issue**: `file_exists()` uses bare `except:` which catches `SystemExit`, `KeyboardInterrupt`, and `GeneratorExit`. The error is also not logged, making S3 failures invisible.
- **Fix**: Change to `except (ClientError, BotoCoreError) as e:` with `logger.debug(f"file_exists check failed: {e}")`. At minimum use `except Exception:`.
- **Effort**: small

### [4] Full file read into memory before S3 upload (Severity: high)
- **File(s)**: `cr8_cli/storage.py:110-113`
- **Issue**: `upload_file()` reads the entire file into memory (`f.read()`) to compute SHA256, then calls `s3_client.upload_file()` which reads from disk again. For large audio files (FLAC/WAV can be 500MB+), this doubles memory usage unnecessarily.
- **Fix**: Stream the hash computation in chunks (`hashlib` supports `update()`), get size via `os.path.getsize()`, and let `upload_file()` handle its own streaming.
- **Effort**: small

### [5] Insecure default SECRET_KEY in production path (Severity: high)
- **File(s)**: `cr8_cli/config.py:181`
- **Issue**: `APIConfig.secret_key` defaults to `"dev-secret-key-change-in-production"`. While `validate()` warns about this, it doesn't prevent startup. If the env var is unset, the API runs with a guessable secret.
- **Fix**: Make `validate(strict=True)` the default in non-debug mode, or raise immediately if `SECRET_KEY` is the default and `DEBUG=false`.
- **Effort**: small

### [6] Global config reload is not thread-safe (Severity: medium)
- **File(s)**: `cr8_cli/config.py:257-261`
- **Issue**: `reload()` reassigns the global `config` variable without any synchronization. In the FastAPI async server, concurrent requests could see a half-initialized config object.
- **Fix**: Use a `threading.Lock` around the assignment, or use an immutable config pattern where the new config is fully constructed before a single atomic reference swap.
- **Effort**: small

### [7] print() statements instead of logging in production code (Severity: medium)
- **File(s)**: `cr8_cli/circuit_breaker.py:167,199,277,284`, `cr8_cli/notifications.py:93,142`, `api/auth_middleware.py:109`, `cr8_cli/supabase_pg_client.py:306-310`
- **Issue**: ~15 `print()` calls in production paths bypass the logging framework — no timestamps, no log levels, no capture by log aggregators.
- **Fix**: Replace with `logger.info()` / `logger.warning()` / `logger.error()` as appropriate. The circuit breaker state transitions are particularly important to capture in structured logs.
- **Effort**: small

### [8] Duplicate migration number 009 (Severity: medium)
- **File(s)**: `database/migrations/009_add_duplicate_detection.sql`, `database/migrations/009_worker_heartbeats.sql`
- **Issue**: Two migrations share sequence number 009. Depending on execution order, one may silently fail or schema may diverge between environments.
- **Fix**: Renumber `009_worker_heartbeats.sql` → `019_worker_heartbeats.sql` (next available after 018).
- **Effort**: small

### [9] docker-compose.yml references deprecated Dockerfiles (Severity: medium)
- **File(s)**: `docker-compose.yml:8`, `Dockerfile.api`, `Dockerfile.frontend`, `Dockerfile.worker`
- **Issue**: `docker-compose.yml` still points to `Dockerfile.api`, `Dockerfile.frontend`, `Dockerfile.worker` — all marked DEPRECATED in their headers. The canonical build uses the unified `Dockerfile` with `--target` stages.
- **Fix**: Update docker-compose to use `dockerfile: Dockerfile` with `target: api` / `target: worker` / `target: frontend`. Delete deprecated Dockerfiles.
- **Effort**: small

### [10] API dependencies missing from pyproject.toml (Severity: medium)
- **File(s)**: `pyproject.toml`, `requirements.txt`, `api/*.py`
- **Issue**: FastAPI, Flask, Pydantic, boto3, psycopg2-binary, and other packages are imported in `api/` and `cr8_cli/` but only declared in `requirements.txt`, not in `pyproject.toml`. Anyone installing via `pip install .` gets an incomplete dependency set.
- **Fix**: Add an `[project.optional-dependencies] api = [...]` group in `pyproject.toml` with the API-specific deps. Keep `requirements.txt` as a flat lock file generated from pyproject.toml.
- **Effort**: medium

### [11] YAML registry load crashes on empty file (Severity: medium)
- **File(s)**: `cr8_cli/playlist_registry.py:109-113`
- **Issue**: `yaml.safe_load()` returns `None` for empty files. Line 113 then calls `self.config.get('defaults', {})` which raises `AttributeError: 'NoneType' object has no attribute 'get'`.
- **Fix**: Add `if not self.config: raise ValueError(f"Empty or invalid YAML: {self.yaml_path}")` after the safe_load call.
- **Effort**: small

### [12] Supabase client silently returns 0 on missing migration (Severity: medium)
- **File(s)**: `cr8_cli/supabase_client.py:566-569`
- **Issue**: `get_total_storage_size()` catches `APIError` and returns 0 when the `file_size_bytes` column doesn't exist. This makes it look like storage is empty rather than signaling that migration 004 hasn't been applied.
- **Fix**: Log a warning: `logger.warning("file_size_bytes column missing — apply migration 004")` and return 0, or raise a custom `MigrationRequiredError`.
- **Effort**: small

### [13] No upload timeout on Supabase storage operations (Severity: medium)
- **File(s)**: `cr8_cli/storage.py:328` (SupabaseStorageBackend.upload_file)
- **Issue**: The Supabase storage upload call has no timeout parameter. A network stall will block the worker thread indefinitely.
- **Fix**: Configure a timeout via the Supabase client's underlying httpx/requests session, or wrap the call with `asyncio.wait_for()` / `signal.alarm()`.
- **Effort**: medium

### [14] Stale .import-linter config references non-existent modules (Severity: low)
- **File(s)**: `.import-linter`
- **Issue**: References modules like `cr8_cli.core`, `cr8_cli.auth`, `cr8_cli.converters`, `cr8_cli.gdrive`, `cr8_cli.vault` — none of which exist. The linter config is completely out of sync with the actual package structure.
- **Fix**: Rewrite `.import-linter` to reflect actual module hierarchy, or remove it if not actively enforced.
- **Effort**: small

### [15] Redundant CI workflows with overlapping triggers (Severity: low)
- **File(s)**: `.github/workflows/ci.yml` (32 lines), `.github/workflows/tests.yml` (143 lines)
- **Issue**: Both trigger on `push` and `pull_request`. `ci.yml` only runs Ruff; `tests.yml` runs Ruff + Black + isort + mypy + security + multi-version tests. Every PR runs Ruff twice.
- **Fix**: Remove `ci.yml` and rely solely on `tests.yml`, or make `ci.yml` a fast-path check on draft PRs only.
- **Effort**: small

---

## CLAUDE.md Accuracy

| Section | Status | Issue |
|---------|--------|-------|
| Project Overview | Accurate | — |
| Common Development Commands | Accurate | — |
| High-Level Architecture | Mostly accurate | Does not mention the MCP tool layer (`cr8_cli/mcp/`) which is now a major component |
| Key Files Reference | Outdated | Missing `cr8_cli/aws_client.py`, `cr8_cli/circuit_breaker.py`, `cr8_cli/config.py`, `cr8_cli/mcp/server.py`, entire `cr8_cli/mcp/tools/` tree |
| Data Flow: Playlist Conversion | Accurate | — |
| Testing Strategy | Accurate | — |
| MCP Tool Recommendations | Partially implemented | The "Proposed Module Structure" section describes tools as future work, but many are already built in `cr8_cli/mcp/tools/` — this section should be updated to reflect current state |
| Quick Start for MCP Development | Outdated paths | References `~/Docs/cr8-cli/cr8-cli` which should be `~/hairglasses-studio/cr8-cli` |
| MCP Server Configuration | Outdated | `cwd` path references `/Users/mitch/Docs/cr8-cli/cr8-cli` (macOS path, wrong user) |
| Dependencies - Core Runtime | Missing entries | `boto3`, `pyjwt`, `botocore` not listed; `redis` listed but marked optional |
| Entry point `cr8 = "cr8_cli.cli:main"` | Broken | `cli.py` doesn't exist; CLAUDE.md doesn't mention this |

**Suggested corrections**: Update "Key Files Reference" to include the MCP layer, mark implemented MCP tools as complete in the roadmap, fix paths to use `~/hairglasses-studio/cr8-cli`, and add a note about the broken CLI entry point.

---

## Recommended Next Actions

1. **Untrack `.env` and rotate all committed credentials** — highest blast radius, zero code changes needed, prevents active credential exposure.
2. **Fix bare `except:` in `storage.py:198` and add logging to the 5 silent `except Exception` handlers in health_report.py** — 15 minutes of work, eliminates the most dangerous silent failure paths.
3. **Create `cr8_cli/cli.py` or fix the pyproject.toml entry point** — the package is currently uninstallable as a CLI tool, blocking any user who runs `pip install`.
