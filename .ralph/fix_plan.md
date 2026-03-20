# Ralph Fix Plan — Phase 0.5: Critical Fixes

Source: `ROADMAP.md` Phase 0.5. All task groups are independent and can be worked in parallel.

## Rules
- **One task group per loop** (e.g., all of 0.5.1 in one loop)
- Run `make ci` (vet + test + build) before committing
- After each loop, append a cycle entry to `.ralph/cycle_notes.md`
- Check off subtasks as you complete them

---

## Task Groups

### 0.5.1 — Silent error suppression in RefreshRepo
- [ ] 0.5.1.1 — Return `[]error` from `RefreshRepo()` in `internal/model/status.go:49-54` instead of discarding with `_ =`
- [ ] 0.5.1.2 — Propagate errors to TUI layer: emit `RefreshErrorMsg` with repo path and error details
- [ ] 0.5.1.3 — Display parse errors in repo detail view status bar (non-blocking, yellow warning)
- [ ] 0.5.1.4 — Add unit test: corrupt status.json → RefreshRepo returns error, not silent zero-value
- **Acceptance:** parse errors in `.ralph/` files visible to user, not silently dropped

### 0.5.2 — Watcher error handling
- [ ] 0.5.2.1 — Replace `return nil` on watcher error (`process/watcher.go:47-48`) with error propagation
- [ ] 0.5.2.2 — Emit `WatcherErrorMsg` to TUI when fsnotify errors occur
- [ ] 0.5.2.3 — Auto-fallback: on watcher error, switch to polling mode and notify user
- [ ] 0.5.2.4 — Add exponential backoff on repeated watcher failures (max 30s)
- **Acceptance:** watcher failures visible in TUI, automatic fallback to polling

### 0.5.3 — Process reaper exit status
- [ ] 0.5.3.1 — Capture `cmd.Wait()` error in `process/manager.go:59` goroutine
- [ ] 0.5.3.2 — Parse exit code: distinguish crash (non-zero) from clean exit (0)
- [ ] 0.5.3.3 — Emit `ProcessExitMsg{RepoPath, ExitCode, Error}` to TUI
- [ ] 0.5.3.4 — Update repo status to "crashed" or "stopped" based on exit code
- [ ] 0.5.3.5 — Add unit test: simulate ralph crash, assert TUI receives crash notification
- **Acceptance:** TUI correctly reports ralph crash vs clean stop

### 0.5.4 — Getpgid fallback safety
- [ ] 0.5.4.1 — Log warning when `Getpgid` fails in `manager.go:78-82` (currently silent fallback to PID-only signal)
- [ ] 0.5.4.2 — Track child PIDs: on process launch, record PID + all child PIDs if available
- [ ] 0.5.4.3 — Fallback kill sequence: SIGTERM to PID → wait 5s → SIGTERM to known children → wait 5s → SIGKILL
- [ ] 0.5.4.4 — Post-stop audit: check for orphaned processes matching `ralph_loop` pattern
- **Acceptance:** `Stop()` reliably kills all child processes, no orphans

### 0.5.5 — Distro path mismatch
- [ ] 0.5.5.1 — Align `hw-detect.service` ExecStart path with Dockerfile install location (`/usr/local/bin/`)
- [ ] 0.5.5.2 — Add path consistency check to `distro/Makefile`: validate service files reference correct paths
- [ ] 0.5.5.3 — Document path conventions in `distro/README.md`: scripts → `/usr/local/bin/`, configs → `/etc/ralphglasses/`
- **Acceptance:** `hw-detect.service` starts successfully on first boot

### 0.5.6 — Grub AMD iGPU fallback
- [ ] 0.5.6.1 — Add GRUB menuentry for AMD iGPU boot: `nomodeset` removed, `amdgpu.dc=1` enabled
- [ ] 0.5.6.2 — Add GRUB menuentry for headless/serial console boot
- [ ] 0.5.6.3 — Set GRUB timeout to 5s (allow human intervention on boot failure)
- [ ] 0.5.6.4 — Add `grub.cfg` validation to CI: parse all menuentry blocks, verify kernel image paths exist
- **Acceptance:** system boots on AMD iGPU when NVIDIA unavailable

### 0.5.7 — Hardcoded version string
- [ ] 0.5.7.1 — Define `var Version = "dev"` in `internal/version/version.go`
- [ ] 0.5.7.2 — Replace hardcoded `"0.1.0"` in `cmd/mcp.go:19` and `cmd/ralphglasses-mcp/main.go:22`
- [ ] 0.5.7.3 — Add `-ldflags "-X internal/version.Version=$(git describe)"` to build commands
- [ ] 0.5.7.4 — Add `ralphglasses version` subcommand: print version, go version, build date, commit SHA
- [ ] 0.5.7.5 — Display version in TUI help view and MCP server info
- **Acceptance:** `ralphglasses version` outputs correct git-derived version

### 0.5.8 — CI BATS guard
- [ ] 0.5.8.1 — Guard BATS step in CI: check `scripts/test/` exists and contains `.bats` files before running
- [ ] 0.5.8.2 — Add BATS install step to CI (install `bats-core` if not present)
- [ ] 0.5.8.3 — Run `marathon.bats` in CI with mock ANTHROPIC_API_KEY
- [ ] 0.5.8.4 — Add CI matrix: test on ubuntu-latest and macos-latest
- **Acceptance:** CI passes regardless of test directory presence

### 0.5.9 — Race condition in MCP scan
- [ ] 0.5.9.1 — Add `sync.RWMutex` to protect `repos` map in `internal/mcpserver/` during concurrent scans
- [ ] 0.5.9.2 — Add `go test -race` to CI pipeline for all packages
- [ ] 0.5.9.3 — Write concurrent scan test: 10 goroutines scanning simultaneously
- **Acceptance:** `go test -race ./...` passes clean

### 0.5.10 — Marathon.sh edge cases
- [ ] 0.5.10.1 — Add `bc` availability check at script start (budget calculation depends on it)
- [ ] 0.5.10.2 — Add disk space check before marathon start (warn if < 5GB free)
- [ ] 0.5.10.3 — Fix infinite restart loop: cap MAX_RESTARTS, add cooldown between restarts
- [ ] 0.5.10.4 — Add memory pressure monitoring: check `/proc/meminfo` AvailMem, warn at < 2GB
- [ ] 0.5.10.5 — Add log rotation: rotate marathon logs at 100MB, keep last 3
- **Acceptance:** marathon.sh handles resource exhaustion gracefully

### 0.5.11 — Config validation strictness
- [ ] 0.5.11.1 — Define canonical key list with types: `internal/model/config_schema.go`
- [ ] 0.5.11.2 — Warn on unknown keys in `.ralphrc` (typo detection)
- [ ] 0.5.11.3 — Validate numeric ranges: `MAX_CALLS_PER_HOUR` must be 1-1000, `CB_COOLDOWN_MINUTES` must be 1-60
- [ ] 0.5.11.4 — Validate boolean values: only "true"/"false", reject "yes"/"1"/"on"
- **Acceptance:** invalid `.ralphrc` values produce clear error messages

---

## Completed
- [x] Project enabled for Ralph

## Next Up
After Phase 0.5 is complete, proceed to **Phase 1: Harden & Test** in `ROADMAP.md` (38 subtasks).
