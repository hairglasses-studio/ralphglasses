# claudekit Audit Report

## Summary

claudekit is a well-structured Go MCP toolkit with clean package boundaries, consistent patterns, and solid test coverage in leaf packages (themekit 90%, skillkit 88%, pluginkit 77%). The codebase is healthy overall — the architecture is layered correctly, dependencies flow downward, and the ToolModule abstraction is used consistently. The **single highest-priority improvement** is fixing a goroutine/resource leak in the ralph module where starting a new loop doesn't cancel the previous loop's context, and addressing the 14 files with `gofmt` drift that CI should be catching.

## Findings

### [1] Ralph module leaks goroutines when restarting loops (Severity: high)
- **File(s)**: `mcpserver/ralph.go:106-170`
- **Issue**: When `ralph_start` is called and the previous loop is not `StatusRunning` (e.g., completed or failed), the old `m.cancel` context function is never called before being overwritten at line 168. The old `loop.Run(ctx)` goroutine may still hold resources or be blocked on I/O. Each restart leaks the previous context's cancel func.
- **Fix**: Before creating a new loop, cancel the previous context if it exists:
  ```go
  // After the "already running" check, add:
  if m.cancel != nil {
      m.cancel()
  }
  m.loop = nil
  ```
- **Effort**: small

### [2] 14 files have gofmt drift (Severity: high)
- **File(s)**: `mcpserver/ralph.go`, `mcpserver/envmodule.go`, `mcpserver/fontmodule.go`, `mcpserver/memorymodule.go`, `mcpserver/skillmodule.go`, `mcpserver/statuslinemodule.go`, `mcpserver/thememodule.go`, `mcpserver/workflowmodule.go`, `pluginkit/config.go`, `skillkit/index.go`, `themekit/palette.go`, `themekit/iterm2_theme.go`, `fontkit/iterm2.go`, `envkit/dotfiles_test.go`
- **Issue**: `gofmt -l .` reports 14 files with formatting issues. CI runs `golangci-lint` but apparently doesn't enforce `gofmt`. The ralph.go file has visibly inconsistent indentation at lines 128-141 (mixed tab depths inside a function literal).
- **Fix**: Run `gofmt -w .` across the repo. Add a `gofmt` check to CI or ensure golangci-lint's `gofmt` linter is enabled.
- **Effort**: small

### [3] `cmd/claudekit/main.go` fontsSetup calls Detect twice (Severity: medium)
- **File(s)**: `cmd/claudekit/main.go:254-290`
- **Issue**: `fontsSetup` calls `fontsStatus(ctx)` at line 256 (which internally calls `fontkit.Detect(ctx)`), then immediately calls `fontkit.Detect(ctx)` again at line 260. Font detection scans the filesystem and runs `brew` — this doubles the I/O cost of the setup command.
- **Fix**: Refactor `fontsStatus` to accept an optional pre-computed `*FontStatus`, or extract the detection into a shared variable:
  ```go
  status, err := fontkit.Detect(ctx)
  if err != nil { return err }
  // Print status using status directly instead of calling fontsStatus
  ```
- **Effort**: small

### [4] Unused `ctx` parameter in `runStatusline` (Severity: low)
- **File(s)**: `cmd/claudekit/main.go:447`
- **Issue**: `runStatusline(ctx context.Context, cmd string)` accepts a context but none of its sub-commands (`statuslineInstall`, `statuslinePreview`, `statuslineRender`) use it. This is misleading and `go vet` could flag it in future Go versions.
- **Fix**: Remove the `ctx` parameter: `func runStatusline(cmd string) error`. Update the call site at line 50.
- **Effort**: small

### [5] `--dry-run` flag inconsistency in envMise (Severity: medium)
- **File(s)**: `cmd/claudekit/main.go:559`
- **Issue**: `parseFlag("dry-run", "false") == "true"` requires `--dry-run true` (with explicit value). But `hasFlag("dry-run")` exists for boolean flags. A user running `claudekit env mise --dry-run` (no value) gets `false` because parseFlag returns `"false"` (the fallback). Every other boolean flag in the CLI uses `hasFlag`.
- **Fix**: Change to `hasFlag("dry-run")`:
  ```go
  dryRun := hasFlag("dry-run")
  ```
- **Effort**: small

### [6] Plugin subprocess command injection risk (Severity: high)
- **File(s)**: `pluginkit/subprocess.go:49`
- **Issue**: `exec.CommandContext(ctx, "sh", "-c", h.Command)` passes the plugin's `Command` field directly to a shell. Plugin YAML files are loaded from the plugin directory, so if an attacker can write a YAML file there, they get arbitrary command execution. No validation or sanitization is performed on the command string.
- **Fix**: Document this as an intentional trust boundary (plugins are user-installed). Add a validation step in `LoadPlugin` that warns or rejects commands containing shell metacharacters (`; | & $(`), or at minimum log which command is about to be executed. Consider using `exec.CommandContext(ctx, parts[0], parts[1:]...)` instead of shell invocation where possible.
- **Effort**: medium

### [7] WebMCP HTTP server has no security controls (Severity: medium)
- **File(s)**: `mcpserver/webmcp.go:26-76`
- **Issue**: The HTTP handler returned by `WebMCPHandler` has no authentication, no CORS headers, no rate limiting, and binds to `:8080` by default. While it's a read-only discovery endpoint, it exposes the full tool inventory to any network client.
- **Fix**: Add `Access-Control-Allow-Origin` headers (restrict to localhost), consider binding to `127.0.0.1:8080` by default instead of `:8080`, and add a brief comment documenting the security model (local-only, read-only).
- **Effort**: small

### [8] `cmd/claudekit-mcp/main.go` swallows `os.Getwd` error (Severity: low)
- **File(s)**: `cmd/claudekit-mcp/main.go:24`
- **Issue**: `wd, _ := os.Getwd()` discards the error. If `Getwd` fails, `wd` is empty string, which propagates as project root to multiple modules (skill, ralph, roadmap, rdcycle). This could cause tools to write artifacts to unexpected locations.
- **Fix**: Handle the error:
  ```go
  wd, err := os.Getwd()
  if err != nil {
      log.Fatalf("cannot determine working directory: %v", err)
  }
  ```
- **Effort**: small

### [9] Low test coverage on CLI and MCP server entrypoints (Severity: medium)
- **File(s)**: `cmd/claudekit/main.go` (20.6%), `cmd/claudekit-mcp/main.go` (11.0%), `envkit/` (43.9%)
- **Issue**: The two entrypoints have the lowest coverage. `cmd/claudekit` at 20.6% means most CLI paths (fontsSetup, themeSync, envMise, ralphTail, mcpServe, mcpPublish) are untested. `cmd/claudekit-mcp` at 11% means the server wiring, profile resolution, and gateway setup lack tests. `envkit` at 43.9% is missing coverage on `MiseInstall` and `Restore`.
- **Fix**: Add integration tests for key CLI flows using `t.TempDir()` and mocked binaries. For claudekit-mcp, test `loadDotenv` edge cases (malformed lines, quoted values) and gateway flag parsing. For envkit, add `Restore` round-trip tests.
- **Effort**: large

### [10] Workflow graph comment contradicts implementation (Severity: low)
- **File(s)**: `mcpserver/workflowmodule.go:36-66`
- **Issue**: The comment on line 36 says `"detect → font_install, theme_apply (parallel) → statusline_install → env_snapshot → END"` but the actual graph is fully sequential: detect → font_install → theme_apply → statusline_install → env_snapshot → END. The comment on line 59 acknowledges this ("parallelism is a client concern") but the function doc is misleading.
- **Fix**: Update the comment to reflect the actual sequential graph:
  ```go
  // detect → font_install → theme_apply → statusline_install → env_snapshot → END
  ```
- **Effort**: small

### [11] `ralphStatus` has confusing `--json` flag parsing (Severity: low)
- **File(s)**: `cmd/claudekit/main.go:829`
- **Issue**: The condition `parseFlag("json", "") == "true" || parseFlag("json", "") == "" && hasFlag("json")` is hard to read and relies on Go operator precedence (`&&` binds tighter than `||`). It calls `parseFlag` twice. The intent is to support both `--json` and `--json true`.
- **Fix**: Simplify to:
  ```go
  if hasFlag("json") {
  ```
  Since `hasFlag` already checks for `--json` presence, and nobody would write `--json false`.
- **Effort**: small

### [12] MiseInstall pipes curl to bash without verification (Severity: medium)
- **File(s)**: `envkit/mise.go:111`
- **Issue**: `curl -fsSL https://mise.jdx.dev/install.sh | bash` is a supply-chain risk. If the mise CDN is compromised, this executes arbitrary code. While this is a common pattern, it's worth noting for a tool that runs in developer terminals.
- **Fix**: Document this risk in a code comment. Consider checking for a package manager first (pacman on Arch/Manjaro, apt on Debian) before falling back to curl-pipe-bash. On Manjaro, `pacman -S mise` is available.
- **Effort**: small (comment) / medium (add pacman support)

## CLAUDE.md Accuracy

1. **Tool count outdated**: CLAUDE.md says "37 tools across 10 modules" — the actual count is higher. rdcycle alone has 12 tools, plus memory (4), workflow (2), finops (2), ralph (3), font (3), theme (2), statusline (1), env (2), skill (2) = **33 core tools** minimum, plus gateway and discovery modules contribute dynamically. The "10 modules" should be "13+ modules" (font, theme, statusline, env, skill, ralph, roadmap, finops, memory, rdcycle, workflow, gateway, discovery).

2. **Missing modules in Package Map**: The Package Map table doesn't list `rdcycle/` as a package even though `mcpserver/rdcyclemodule.go` exists. The rdcycle logic lives in mcpkit, but the module bridge is in claudekit.

3. **Missing `webmcp` in MCP Tools section**: The WebMCP HTTP handler is documented in README but not in CLAUDE.md's tool list.

4. **`cmd/claudekit/main.go` line count**: Not a CLAUDE.md claim, but the file is 904 lines — worth noting in the package map for complexity awareness.

5. **Gateway not mentioned**: CLAUDE.md doesn't mention the gateway aggregation feature or the `--gateway` flag.

## Recommended Next Actions

1. **Run `gofmt -w .` and fix the ralph goroutine leak** — two small changes that fix the highest-severity issues (formatting drift + resource leak).
2. **Fix `--dry-run` flag parsing and bind WebMCP to localhost** — two small fixes that improve CLI correctness and default security posture.
3. **Add CLI integration tests** — the 20.6% coverage on the main entrypoint means most user-facing paths are untested; focus on `fontsSetup`, `themeSync`, and `ralphTail` flows.
