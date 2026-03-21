# Ralph Agent Configuration

## Build & Quality Gate

Always run `make ci` before committing. This runs vet + test + build in sequence.

```bash
# CI pipeline (REQUIRED before every commit)
make ci
```

## Available Makefile Targets

```bash
make ci              # vet + test + build (quality gate)
make test            # go test -race ./...
make test-verbose    # go test -race -v ./...
make test-cover      # coverage report with threshold
make test-integration # integration tests (requires -tags integration)
make fuzz            # fuzz tests (30s each, config/status/circuit breaker/MCP parsers)
make test-scripts    # BATS tests for shell scripts
make build           # go build ./...
make vet             # go vet ./...
make lint            # golangci-lint (if installed)
make clean           # remove build artifacts
```

## Workflow

1. `make ci` — run before every commit
2. `make test-verbose` — when debugging test failures
3. `make test-cover` — to check coverage after adding tests
4. `make fuzz` — after modifying parsers in `internal/model/` or `internal/mcpserver/`

## Multi-Provider Notes

`make ci` works regardless of which LLM provider runs it — it's pure Go toolchain.

Provider-specific headless launch commands:
```bash
# Claude
claude -p "task" --output-format stream-json
# Gemini
gemini -p "task" --output-format stream-json --yolo
# Codex
codex exec --full-auto "task"
```

## Notes
- Race detector is enabled by default in all test targets
- Integration tests require the `integration` build tag
- Fuzz tests run for 30s per target by default
