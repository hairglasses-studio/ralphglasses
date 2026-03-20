# Prompt Generator Input for platform.claude.com

Paste everything below the `---` line into platform.claude.com's prompt generator.

---

**Title:** Iterative Ralphglasses Tool Improvement, Testing & Commit Cycles

**What the prompt should do:**

You are an expert Go and shell developer working on `ralphglasses` — a k9s-style TUI + MCP server for managing parallel Claude Code agent fleets. The codebase is a Go binary (Charmbracelet Bubble Tea, Cobra, mcp-go) with shell scripts (marathon supervisor, hardware detection) and Linux distro configs.

Given a prioritized issue from the backlog below, you will: (1) fix the issue with minimal, focused changes, (2) add or update tests to cover the fix, (3) verify the build passes, and (4) commit with a descriptive message. Then move to the next issue.

**Backlog (prioritized):**

1. **Watcher blocks event loop** (`internal/process/watcher.go`): `WatchStatusFiles` blocks the Bubble Tea cmd indefinitely via `for { select { ... } }` with no timeout or cancellation. If no `.ralph/` files change, the TUI hangs. Refactor to use a goroutine + channel pattern or add a `time.After` timeout (e.g. 2s) so the TUI falls back to polling. Add test for timeout behavior.

2. **MCP scan error propagation** (`internal/mcpserver/tools.go`): Six handlers silently ignore `s.scan()` errors with `_ = s.scan()`, then report "repo not found" when scan actually failed. Return the scan error directly so callers get actionable diagnostics. Update tests to cover scan failure path.

3. **Log stream silent failures** (`internal/process/logstream.go`): `TailLog` returns empty `LogLinesMsg{}` on file errors (lines 23, 31). The log view shows blank with no explanation. Return an error message in the log lines (e.g. `[]string{"[error] Log file not found: " + logPath}`) so the user sees feedback. Add test.

4. **marathon.sh macOS portability** (`marathon.sh:148`): `sed -i` without `''` fails on BSD sed (macOS). Use `sed -i '' ...` on macOS or use a temp file pattern (`sed ... > tmp && mv tmp file`). Add BATS test for sed operations.

5. **marathon.sh budget check fragility** (`marathon.sh:391`): If `bc` is missing or fails, the budget comparison evaluates to empty and the `if` falls through — silently bypassing the budget limit. Add explicit `bc` availability check and fail-safe (treat bc failure as "stop, budget unknown"). Add BATS test.

6. **Extract home expansion utility**: Deduplicate `~/` expansion logic from `cmd/root.go`, `cmd/mcp.go`, `cmd/ralphglasses-mcp/main.go` into a shared `internal/util/path.go` with `func ExpandHome(path string) string`. Update all three callers. Add tests including edge cases (no `~`, `~user`, empty).

7. **Config key validation** (`internal/model/config.go`): `Save()` writes arbitrary keys to `.ralphrc` without validation. Keys with spaces, quotes, or special chars corrupt the file. Add regex validation (`^[A-Z_][A-Z0-9_]*$`) and return an error on invalid keys. Add test + fuzz case for malformed keys.

8. **Wire up log search** (`internal/tui/views/logstream.go`): The `Search` field on `LogView` exists but filtering logic is never applied. Implement basic case-insensitive substring filtering on log lines when Search is non-empty. Add view test.

9. **Expand TUI app test coverage** (`internal/tui/app.go`): Currently 31% — lowest core package. Add tests for: key command dispatching (`:`, `/`, `?`, `q`, `enter`), view navigation (push/pop), command input handling, and filter mode. Target 60%+.

10. **hw-detect.sh GPU blacklist fix** (`distro/scripts/hw-detect.sh:237`): The GTX 1060 blacklist uses `options nvidia NVreg_EnablePCIeGen3=0` which is a boolean PCIe flag, NOT a GPU exclusion mechanism. Replace with a proper `blacklist` entry in `/etc/modprobe.d/` or use `NVreg_ExcludedGpus=<PCI_BUS_ID>`. Also remove dead PCI bus conversion code at lines 180-182 (value is immediately overwritten). Add dry-run test assertions.

11. **Update Dockerfile Go version** (`distro/Dockerfile`): Hardcoded `golang:1.22.5` doesn't match `go.mod` (1.26.1). Update to match.

12. **Add CI linting**: Add `golangci-lint run ./...` and `shellcheck distro/scripts/*.sh marathon.sh` steps to `.github/workflows/ci.yml`.

13. **Remove dead code** (`internal/tui/app.go:564`): Unused `contentHeight` variable is assigned then immediately discarded. Remove it.

**Constraints:**
- One issue per cycle. Fix → test → verify build → commit.
- Write clean, idiomatic Go. Handle errors explicitly with `fmt.Errorf("context: %w", err)`.
- Prefer simplicity. Don't refactor surrounding code beyond the fix.
- Shell scripts must pass `shellcheck` after edits.
- Commit messages: imperative mood, explain "why" not "what".
- Run `go vet ./...` and `go test -race ./...` before each commit.
- For shell fixes, run `bats scripts/test/` if BATS tests exist.
- Read `CLAUDE.md` and `go.mod` before starting.

**Context files to read first:** `CLAUDE.md`, `ROADMAP.md`, `go.mod`
