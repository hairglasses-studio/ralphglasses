# S12 — Circuit Breaker Consolidation

**Date**: 2026-04-04
**Audit scope**: `internal/enhancer/`, `internal/gateway/`, `internal/process/`, `internal/safety/`
**Related**: s2-dead-code-audit.md (original finding)

---

## 1. Locations and Line Counts

| Package | File | Lines | Exported? |
|---------|------|-------|-----------|
| `internal/enhancer` | `circuit.go` | 104 | Yes — `CircuitBreaker`, `NewCircuitBreaker()` |
| `internal/gateway` | `circuit.go` | 246 | Yes — `CircuitBreaker`, `CircuitBreakerConfig`, `CircuitBreakerStatus`, `ProviderCircuitBreakers` |
| `internal/process` | `circuit_breaker.go` | 190 | Yes — `CircuitBreaker`, `NewCircuitBreaker(int, dur, dur)` |
| `internal/safety` | `circuit_breaker.go` | 290 | Yes — `CircuitBreaker`, `New(Config)`, `MustNew(Config)`, `Metrics`, `Snapshot` |

**Note**: A fifth inline circuit state exists in `internal/notify/webhook.go` (~50 lines, unexported `circuitState` struct). It is intentionally private and not in scope for consolidation.

**Total duplication**: ~830 lines implementing the same 3-state machine across 4 packages.

---

## 2. Feature Comparison

| Feature | enhancer | gateway | process | safety |
|---------|----------|---------|---------|--------|
| **State type** | `circuitState int` (unexported iota) | `CircuitState int` (exported iota) | `CircuitState string` (exported) | `State int` (exported iota) |
| **State values** | closed/open/half-open | closed/open/half-open | "closed"/"open"/"half-open" | closed/open/half-open |
| **Failure threshold** | 3 (hardcoded) | 5 (default, configurable) | 3 (default, configurable) | 5 (default, configurable) |
| **Reset/cooldown timeout** | 60s (hardcoded) | 60s (default, configurable) | 5m (default, configurable) | 30s (default, configurable) |
| **Failure window** | none | none | 60s sliding window | none |
| **Success threshold** | 1 (implicit) | 1 (configurable via `HalfOpenMaxRequests`) | 1 (implicit) | 2 (configurable `SuccessThreshold`) |
| **Half-open probe limit** | 1 (blocks further) | configurable int (default 1) | unlimited once entered | unlimited once entered |
| **`Allow()` return** | `bool` | `error` (`ErrCircuitOpen` or nil) | `bool` (via `AllowSpawn()`) | n/a — uses `Execute(fn)` |
| **Execute wrapper** | no | no | no | yes — `Execute(fn func() error) error` |
| **`Reset()` method** | yes | yes | no | yes |
| **State query** | `State() string` | `Status() CircuitBreakerStatus` | `State() CircuitState` | `State() State` |
| **Observability** | none | `CircuitBreakerStatus` snapshot | none | `Metrics` (atomic counters) + `Snapshot()` |
| **Persistence** | none | none | `WriteStateFile()` → `/tmp/ralphglasses-coordination/circuit-state.json` | none |
| **Clock injection** | no | no | no | yes — `now func() time.Time` |
| **Config struct** | no | `CircuitBreakerConfig` | named args | `Config` with validation |
| **Config validation** | no | `applyDefaults()` | zero-value defaults | `validate()` returns error |
| **Constructor** | `NewCircuitBreaker()` (0 args) | `NewCircuitBreaker(cfg)` | `NewCircuitBreaker(int, dur, dur)` | `New(cfg)` returning error; `MustNew(cfg)` |
| **Per-provider manager** | in `hybrid.go` as a `map[string]*CircuitBreaker` | `ProviderCircuitBreakers` (lazy creation, RWMutex) | n/a | n/a |
| **Thread safety** | `sync.Mutex` | `sync.Mutex` | `sync.Mutex` | `sync.Mutex` + `sync/atomic` for metrics |
| **`ErrCircuitOpen` sentinel** | no | yes | no | yes |

---

## 3. Differences That Prevent Trivial Consolidation

### 3a. Failure counting model
- **enhancer/gateway/safety**: consecutive failure counter; any success resets to zero.
- **process**: sliding-window counter — failures older than `failureWindow` (60s default) are discarded on the next failure record. This is fundamentally different: a burst of 3 failures spread over 2 minutes does NOT trip the circuit once they fall outside the window. The other three would hold the count indefinitely until a success.

### 3b. Half-open behaviour
- **enhancer** (`Allow() bool`): entering half-open lets exactly one probe through (the transition call), then blocks all subsequent calls until the probe resolves. The "one probe" is enforced by returning `true` once and `false` thereafter while state stays `circuitHalfOpen`.
- **gateway** (`Allow() error`): tracks `halfOpenRequests` counter; allows up to `HalfOpenMaxRequests` concurrent probes (default 1).
- **process** (`AllowSpawn() bool`): once in half-open, allows every call through — it is up to the next `RecordSuccess`/`RecordFailure` to transition state. Effectively allows concurrent probes.
- **safety** (`Execute(fn)`): allows all calls through in half-open; accumulates `consecutiveSuccesses` and closes only after hitting `SuccessThreshold` (default 2). Any half-open failure immediately re-opens.

### 3c. API surface — Execute vs. Allow/Record
`safety.CircuitBreaker` wraps the call as `Execute(fn func() error)`, making the circuit breaker the call site. The other three separate `Allow()` from `RecordSuccess()`/`RecordFailure()`, which allows callers to record outcomes asynchronously (e.g. enhancer records after an HTTP response resolves). These two shapes cannot share a type without one side being an adapter.

### 3d. State representation
`process` uses `CircuitState string` (values are the on-disk JSON strings `"closed"`, `"open"`, `"half-open"`), because state is written to a coordination file for cross-process visibility. A shared type using `int` iota would require a separate serialisation layer.

### 3e. Constructor ergonomics
- enhancer: zero-arg constructor with baked-in defaults — deliberate simplicity for a hot path.
- process: positional args — tight coupling to the three tunables with explicit zero-value fallback.
- gateway/safety: config struct — extensible but requires callers to import the config type.

### 3f. Isolation rationale
- `gateway` is an HTTP middleware layer; it deliberately avoids importing application packages to stay thin.
- `process` has the only on-disk persistence requirement; extracting it would force a shared package to know about the coordination directory path.
- `safety` is the only package with cumulative metrics via `sync/atomic` and injectable clock — designed for testability and observability dashboards.
- `enhancer` needs a zero-dependency, zero-config CB since it is already a deep import target.

---

## 4. Recommended Approach

### Option A — Shared `internal/circuit` package (recommended for Sprint 12)

Extract a single canonical implementation into a new `internal/circuit` package. It should be the **gateway** implementation as the baseline (most complete, configurable, tested) with additions from **safety** (clock injection, Metrics, SuccessThreshold).

The shared package provides:
- `Config` struct with `FailureThreshold`, `ResetTimeout`, `SuccessThreshold`, `HalfOpenMaxRequests`
- `DefaultConfig()` returning gateway-style defaults (5 failures, 60s timeout)
- `New(cfg Config) *CircuitBreaker` + `MustNew(cfg Config) *CircuitBreaker`
- `Allow() error` (returns `ErrCircuitOpen` or nil)
- `RecordSuccess()`, `RecordFailure()`, `Reset()`
- `State() State`, `Status() Status` snapshot
- `Execute(fn func() error) error` as a convenience wrapper over Allow/Record
- `Snapshot() Metrics`
- `WithClock(fn func() time.Time)` option for test overriding
- `ProviderBreakers` (promoted from gateway)

**Process-specific features remain in `internal/process`**: the sliding failure window (`failureWindow`) and `WriteStateFile()` do not belong in a generic CB. `process.CircuitBreaker` wraps (embeds or delegates to) `circuit.CircuitBreaker` and adds those two behaviours.

**Enhancer keeps a thin local shim** or switches to `circuit.MustNew(circuit.Config{FailureThreshold: 3, ResetTimeout: 60*time.Second, SuccessThreshold: 1})` and stores per-provider breakers via `circuit.ProviderBreakers`.

**Safety** migrates directly — its `Config`, `New`, `MustNew`, `Execute`, `Metrics`, and `Snapshot` are the model for the shared package.

### Option B — Interface extraction only (lower effort, less cleanup)

Define a `circuit.Breaker` interface:
```go
type Breaker interface {
    Allow() error
    RecordSuccess()
    RecordFailure()
    Reset()
    State() string
}
```
Each package keeps its implementation; callers that cross package boundaries accept the interface. Prevents sharing code but enables mock injection in tests and reduces coupling.

**Recommended**: Option A for long-term health; Option B as an interim step if Sprint 12 time is tight.

---

## 5. Callers That Would Need Updating

### `internal/enhancer` (Option A migration)
| File | Change |
|------|--------|
| `circuit.go` | Delete. Replace with `circuit.MustNew(...)`. |
| `hybrid.go` | Change `CBs map[string]*CircuitBreaker` to `CBs map[string]*circuit.CircuitBreaker`. Constructor uses `circuit.ProviderBreakers`. |
| `hybrid_test.go`, `circuit_test.go`, `circuit_coverage_test.go`, `errorpaths_test.go`, `sampling_test.go` | Update import and type references. |

### `internal/gateway` (Option A migration)
| File | Change |
|------|--------|
| `circuit.go` | Delete. `gateway` imports `circuit` package for `CircuitBreaker`, `ProviderCircuitBreakers` (or uses `circuit.ProviderBreakers`). |
| `circuit_test.go` | Update imports. |

### `internal/process` (partial migration)
| File | Change |
|------|--------|
| `circuit_breaker.go` | Embed `circuit.CircuitBreaker`, keep `failureWindow` field and `WriteStateFile()`. Override `RecordFailure()` to apply sliding window before delegating. Keep `AllowSpawn()` as a domain-named alias for `Allow() == nil`. |
| `circuit_breaker_test.go` | Adjust for new constructor shape. |

### `internal/safety` (Option A migration)
| File | Change |
|------|--------|
| `circuit_breaker.go` | Delete. The shared `circuit` package is built from this implementation. Any callers inside `safety` that use `safety.New()`/`safety.MustNew()` switch to `circuit.New()`/`circuit.MustNew()`. |
| `circuit_breaker_test.go` | Move tests to `internal/circuit`. |

### No external callers to update
Neither `internal/safety` nor `internal/gateway` circuit breakers are imported by any package outside their own directory. The `process` and `enhancer` CBs are used only within their own packages (plus tests). Cross-package consolidation therefore has zero surface area outside the four packages and their test files.

---

## 6. Estimated Effort

| Task | Effort |
|------|--------|
| Create `internal/circuit` package from safety + gateway best parts | 2h |
| Write `internal/circuit` tests (clock injection, all state transitions, ProviderBreakers) | 2h |
| Migrate `internal/enhancer` | 1h |
| Migrate `internal/gateway` | 1h |
| Refactor `internal/process` to wrap shared CB | 1.5h |
| Migrate `internal/safety` (delete file, re-export if needed) | 0.5h |
| Update all test files | 1.5h |
| `go vet ./... && go test ./... -count=1 -race` clean pass | 0.5h |
| **Total** | **~10h** |

The process migration is the only non-trivial part due to the sliding failure window. All other migrations are mechanical find-and-replace with import path changes.

---

## 7. Decision Checklist

- [ ] Confirm `internal/process` sliding-window behaviour is still needed (or simplify to match shared model)
- [ ] Decide whether `WriteStateFile()` stays in `process` or becomes a `circuit.Persistence` option
- [ ] Confirm `gateway.ProviderCircuitBreakers` name moves to `circuit.ProviderBreakers` without breaking MCP tool handlers
- [ ] Ensure `internal/circuit` has no imports from other `internal/` packages (keep it a leaf dependency)
- [ ] Tag sprint task: create `internal/circuit`, then migrate in package order: safety → gateway → enhancer → process
