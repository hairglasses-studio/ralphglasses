package healthz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// CheckStatus represents the outcome of a single health check.
type CheckStatus string

const (
	StatusPass CheckStatus = "pass"
	StatusFail CheckStatus = "fail"
	StatusWarn CheckStatus = "warn"
)

// CheckResult holds the outcome and metadata for a single probe check.
type CheckResult struct {
	Name     string      `json:"name"`
	Status   CheckStatus `json:"status"`
	Message  string      `json:"message,omitempty"`
	Duration string      `json:"duration"`
}

// ProbeResponse is the structured JSON response for /readyz and /livez.
type ProbeResponse struct {
	Status  CheckStatus   `json:"status"`
	Checks  []CheckResult `json:"checks"`
	Elapsed string        `json:"elapsed"`
}

// CheckFunc is a health check function. Implementations should respect the
// context deadline and return a non-nil error to indicate failure.
type CheckFunc func(ctx context.Context) error

// ProbeCheck pairs a name with a check function and a flag indicating
// whether the check is required for readiness (vs. advisory/warning only).
type ProbeCheck struct {
	Name     string
	Check    CheckFunc
	Required bool // if true, failure marks the probe as failed
}

// ProbeRegistry holds named health checks that are evaluated by the
// /readyz and /livez endpoints. Checks are registered at startup and
// evaluated concurrently on each request.
type ProbeRegistry struct {
	mu          sync.RWMutex
	liveness    []ProbeCheck
	readiness   []ProbeCheck
	timeout     time.Duration
}

// DefaultProbeTimeout is the maximum time a single check may run before
// being considered failed.
const DefaultProbeTimeout = 5 * time.Second

// NewProbeRegistry creates a registry with the default timeout.
func NewProbeRegistry() *ProbeRegistry {
	return &ProbeRegistry{
		timeout: DefaultProbeTimeout,
	}
}

// SetTimeout overrides the per-check timeout.
func (r *ProbeRegistry) SetTimeout(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.timeout = d
}

// AddLivenessCheck registers a check that runs on /livez.
func (r *ProbeRegistry) AddLivenessCheck(name string, fn CheckFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.liveness = append(r.liveness, ProbeCheck{Name: name, Check: fn, Required: true})
}

// AddReadinessCheck registers a required check that runs on /readyz.
func (r *ProbeRegistry) AddReadinessCheck(name string, fn CheckFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readiness = append(r.readiness, ProbeCheck{Name: name, Check: fn, Required: true})
}

// AddReadinessWarning registers an advisory check that runs on /readyz
// but does not cause failure -- it reports as a warning.
func (r *ProbeRegistry) AddReadinessWarning(name string, fn CheckFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readiness = append(r.readiness, ProbeCheck{Name: name, Check: fn, Required: false})
}

// runChecks evaluates all checks concurrently and returns the aggregate
// response. Each check gets its own timeout derived from the registry timeout.
func (r *ProbeRegistry) runChecks(parentCtx context.Context, checks []ProbeCheck) ProbeResponse {
	start := time.Now()

	r.mu.RLock()
	timeout := r.timeout
	r.mu.RUnlock()

	results := make([]CheckResult, len(checks))
	var wg sync.WaitGroup

	for i, pc := range checks {
		wg.Add(1)
		go func(idx int, pc ProbeCheck) {
			defer wg.Done()
			checkStart := time.Now()

			ctx, cancel := context.WithTimeout(parentCtx, timeout)
			defer cancel()

			err := pc.Check(ctx)
			elapsed := time.Since(checkStart)

			result := CheckResult{
				Name:     pc.Name,
				Duration: elapsed.Round(time.Microsecond).String(),
			}
			if err != nil {
				if pc.Required {
					result.Status = StatusFail
				} else {
					result.Status = StatusWarn
				}
				result.Message = err.Error()
			} else {
				result.Status = StatusPass
			}
			results[idx] = result
		}(i, pc)
	}
	wg.Wait()

	overall := StatusPass
	for _, r := range results {
		if r.Status == StatusFail {
			overall = StatusFail
			break
		}
		if r.Status == StatusWarn && overall == StatusPass {
			overall = StatusWarn
		}
	}

	return ProbeResponse{
		Status:  overall,
		Checks:  results,
		Elapsed: time.Since(start).Round(time.Microsecond).String(),
	}
}

// RunLiveness evaluates all liveness checks and returns the response.
func (r *ProbeRegistry) RunLiveness(ctx context.Context) ProbeResponse {
	r.mu.RLock()
	checks := make([]ProbeCheck, len(r.liveness))
	copy(checks, r.liveness)
	r.mu.RUnlock()

	return r.runChecks(ctx, checks)
}

// RunReadiness evaluates all readiness checks and returns the response.
func (r *ProbeRegistry) RunReadiness(ctx context.Context) ProbeResponse {
	r.mu.RLock()
	checks := make([]ProbeCheck, len(r.readiness))
	copy(checks, r.readiness)
	r.mu.RUnlock()

	return r.runChecks(ctx, checks)
}

// HandleLivez is an http.HandlerFunc that evaluates liveness checks.
func (r *ProbeRegistry) HandleLivez(w http.ResponseWriter, req *http.Request) {
	resp := r.RunLiveness(req.Context())
	writeProbeResponse(w, resp)
}

// HandleReadyz is an http.HandlerFunc that evaluates readiness checks.
func (r *ProbeRegistry) HandleReadyz(w http.ResponseWriter, req *http.Request) {
	resp := r.RunReadiness(req.Context())
	writeProbeResponse(w, resp)
}

func writeProbeResponse(w http.ResponseWriter, resp ProbeResponse) {
	w.Header().Set("Content-Type", "application/json")
	if resp.Status == StatusFail {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// --- Built-in check constructors ---

// DatabaseCheck returns a CheckFunc that verifies store connectivity by
// calling the provided ping function. The ping function should attempt a
// lightweight operation (e.g., listing zero sessions) and return an error
// if the store is unreachable.
func DatabaseCheck(ping func(ctx context.Context) error) CheckFunc {
	return func(ctx context.Context) error {
		if ping == nil {
			return fmt.Errorf("no database configured")
		}
		return ping(ctx)
	}
}

// EventBusCheck returns a CheckFunc that verifies the event bus is
// operational by performing a publish-subscribe round trip. The bus
// argument must be non-nil.
func EventBusCheck(publish func(ctx context.Context) error) CheckFunc {
	return func(ctx context.Context) error {
		if publish == nil {
			return fmt.Errorf("event bus not configured")
		}
		return publish(ctx)
	}
}

// SessionManagerCheck returns a CheckFunc that verifies the session manager
// responds within the context deadline. The list function should call the
// manager's List method (or similar lightweight query).
func SessionManagerCheck(list func(ctx context.Context) error) CheckFunc {
	return func(ctx context.Context) error {
		if list == nil {
			return fmt.Errorf("session manager not configured")
		}
		return list(ctx)
	}
}

// DiskSpaceCheck returns a CheckFunc that reports failure when available
// disk space on the given path drops below minBytes.
func DiskSpaceCheck(path string, minBytes uint64) CheckFunc {
	return func(ctx context.Context) error {
		avail, err := diskAvailable(path)
		if err != nil {
			return fmt.Errorf("disk check: %w", err)
		}
		if avail < minBytes {
			return fmt.Errorf("disk space low: %s available, need %s",
				formatBytes(avail), formatBytes(minBytes))
		}
		return nil
	}
}

func formatBytes(b uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
