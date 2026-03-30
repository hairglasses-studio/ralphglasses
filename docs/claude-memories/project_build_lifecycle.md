---
name: Build lifecycle notes
description: Build/test/lint lifecycle patterns and gotchas for the ralphglasses Go project
type: reference
---

## Build Commands
- `go build ./...` — full build
- `go test ./... -count=1 -timeout 120s` — full test suite (~37 packages, ~2min)
- `go test ./... -race` — race detector (slower)
- `go test ./... -coverprofile=coverage.out` — coverage report
- `make lint` — golangci-lint (falls back to system install if .tools/ missing)
- `make install-local` — builds release binary, codesigns on macOS, installs to PATH

## Known Gotchas
- ~47 errcheck violations exist (mostly in test files) — tolerated
- Deprecated viewport methods: use `ScrollUp`/`ScrollDown`/`HalfPageUp`/`HalfPageDown` (not LineUp/LineDown)
- `json.Number` implements `fmt.Stringer` — watch type switch ordering
- Flaky test: `TestLoadExternalSessions_SkipExisting` — TempDir cleanup race, passes in isolation
- No `make uninstall` target

## Coverage Trajectory
Sprint 5: 84.4% | Sprint 6: 84.6% | Sprint 7: 86.0% (20 zero-coverage functions)
