package healthz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// --- ProbeRegistry unit tests ---

func TestProbeRegistry_EmptyReturnsPass(t *testing.T) {
	r := NewProbeRegistry()
	resp := r.RunLiveness(context.Background())
	if resp.Status != StatusPass {
		t.Fatalf("empty liveness status = %q, want %q", resp.Status, StatusPass)
	}
	if len(resp.Checks) != 0 {
		t.Fatalf("empty liveness checks = %d, want 0", len(resp.Checks))
	}

	resp = r.RunReadiness(context.Background())
	if resp.Status != StatusPass {
		t.Fatalf("empty readiness status = %q, want %q", resp.Status, StatusPass)
	}
}

func TestProbeRegistry_AllPass(t *testing.T) {
	r := NewProbeRegistry()
	r.AddLivenessCheck("alpha", func(ctx context.Context) error { return nil })
	r.AddLivenessCheck("beta", func(ctx context.Context) error { return nil })

	resp := r.RunLiveness(context.Background())
	if resp.Status != StatusPass {
		t.Fatalf("all-pass status = %q, want %q", resp.Status, StatusPass)
	}
	if len(resp.Checks) != 2 {
		t.Fatalf("check count = %d, want 2", len(resp.Checks))
	}
	for _, c := range resp.Checks {
		if c.Status != StatusPass {
			t.Errorf("check %q status = %q, want %q", c.Name, c.Status, StatusPass)
		}
	}
}

func TestProbeRegistry_OneFails(t *testing.T) {
	r := NewProbeRegistry()
	r.AddReadinessCheck("good", func(ctx context.Context) error { return nil })
	r.AddReadinessCheck("bad", func(ctx context.Context) error { return fmt.Errorf("broken") })

	resp := r.RunReadiness(context.Background())
	if resp.Status != StatusFail {
		t.Fatalf("status = %q, want %q", resp.Status, StatusFail)
	}

	var failCount int
	for _, c := range resp.Checks {
		if c.Status == StatusFail {
			failCount++
			if c.Message != "broken" {
				t.Errorf("check %q message = %q, want %q", c.Name, c.Message, "broken")
			}
		}
	}
	if failCount != 1 {
		t.Fatalf("fail count = %d, want 1", failCount)
	}
}

func TestProbeRegistry_WarningDoesNotFail(t *testing.T) {
	r := NewProbeRegistry()
	r.AddReadinessCheck("required", func(ctx context.Context) error { return nil })
	r.AddReadinessWarning("advisory", func(ctx context.Context) error { return fmt.Errorf("low disk") })

	resp := r.RunReadiness(context.Background())
	if resp.Status != StatusWarn {
		t.Fatalf("status = %q, want %q", resp.Status, StatusWarn)
	}

	for _, c := range resp.Checks {
		if c.Name == "advisory" {
			if c.Status != StatusWarn {
				t.Errorf("advisory status = %q, want %q", c.Status, StatusWarn)
			}
		}
		if c.Name == "required" {
			if c.Status != StatusPass {
				t.Errorf("required status = %q, want %q", c.Status, StatusPass)
			}
		}
	}
}

func TestProbeRegistry_FailOverridesWarn(t *testing.T) {
	r := NewProbeRegistry()
	r.AddReadinessCheck("required_fail", func(ctx context.Context) error { return fmt.Errorf("down") })
	r.AddReadinessWarning("advisory_warn", func(ctx context.Context) error { return fmt.Errorf("slow") })

	resp := r.RunReadiness(context.Background())
	if resp.Status != StatusFail {
		t.Fatalf("status = %q, want %q (fail overrides warn)", resp.Status, StatusFail)
	}
}

func TestProbeRegistry_Timeout(t *testing.T) {
	r := NewProbeRegistry()
	r.SetTimeout(50 * time.Millisecond)
	r.AddLivenessCheck("slow", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	resp := r.RunLiveness(context.Background())
	if resp.Status != StatusFail {
		t.Fatalf("timeout status = %q, want %q", resp.Status, StatusFail)
	}
	if resp.Checks[0].Message == "" {
		t.Error("expected timeout error message")
	}
}

func TestProbeRegistry_ConcurrentExecution(t *testing.T) {
	r := NewProbeRegistry()
	r.SetTimeout(2 * time.Second)

	// Two checks that each sleep 100ms. If run concurrently, total < 250ms.
	for i := range 2 {
		name := fmt.Sprintf("check_%d", i)
		r.AddLivenessCheck(name, func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		})
	}

	start := time.Now()
	resp := r.RunLiveness(context.Background())
	elapsed := time.Since(start)

	if resp.Status != StatusPass {
		t.Fatalf("status = %q, want %q", resp.Status, StatusPass)
	}
	if elapsed > 250*time.Millisecond {
		t.Errorf("took %v, expected < 250ms (checks should run concurrently)", elapsed)
	}
}

// --- Built-in check constructor tests ---

func TestDatabaseCheck_NilPing(t *testing.T) {
	check := DatabaseCheck(nil)
	err := check(context.Background())
	if err == nil {
		t.Fatal("expected error for nil ping")
	}
}

func TestDatabaseCheck_Healthy(t *testing.T) {
	check := DatabaseCheck(func(ctx context.Context) error { return nil })
	if err := check(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatabaseCheck_Unhealthy(t *testing.T) {
	check := DatabaseCheck(func(ctx context.Context) error {
		return fmt.Errorf("connection refused")
	})
	err := check(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEventBusCheck_NilPublish(t *testing.T) {
	check := EventBusCheck(nil)
	if err := check(context.Background()); err == nil {
		t.Fatal("expected error for nil bus")
	}
}

func TestEventBusCheck_Healthy(t *testing.T) {
	check := EventBusCheck(func(ctx context.Context) error { return nil })
	if err := check(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEventBusCheck_Unhealthy(t *testing.T) {
	check := EventBusCheck(func(ctx context.Context) error {
		return fmt.Errorf("bus closed")
	})
	if err := check(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestSessionManagerCheck_NilList(t *testing.T) {
	check := SessionManagerCheck(nil)
	if err := check(context.Background()); err == nil {
		t.Fatal("expected error for nil manager")
	}
}

func TestSessionManagerCheck_Healthy(t *testing.T) {
	check := SessionManagerCheck(func(ctx context.Context) error { return nil })
	if err := check(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSessionManagerCheck_Unhealthy(t *testing.T) {
	check := SessionManagerCheck(func(ctx context.Context) error {
		return fmt.Errorf("manager deadlocked")
	})
	if err := check(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestDiskSpaceCheck_CurrentDir(t *testing.T) {
	// Use a very low threshold that should always pass.
	check := DiskSpaceCheck(os.TempDir(), 1)
	if err := check(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiskSpaceCheck_ImpossibleThreshold(t *testing.T) {
	// Use an impossibly high threshold to trigger failure.
	check := DiskSpaceCheck(os.TempDir(), ^uint64(0))
	err := check(context.Background())
	if err == nil {
		t.Fatal("expected error for impossible threshold")
	}
}

func TestDiskSpaceCheck_BadPath(t *testing.T) {
	check := DiskSpaceCheck("/nonexistent/path/that/does/not/exist", 1)
	if err := check(context.Background()); err == nil {
		t.Fatal("expected error for bad path")
	}
}

// --- HTTP endpoint tests ---

func TestHandleLivez_AllHealthy(t *testing.T) {
	r := NewProbeRegistry()
	r.AddLivenessCheck("process", func(ctx context.Context) error { return nil })

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/livez", nil)
	r.HandleLivez(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp ProbeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != StatusPass {
		t.Errorf("response status = %q, want %q", resp.Status, StatusPass)
	}
	if len(resp.Checks) != 1 {
		t.Fatalf("check count = %d, want 1", len(resp.Checks))
	}
}

func TestHandleLivez_OneUnhealthy(t *testing.T) {
	r := NewProbeRegistry()
	r.AddLivenessCheck("goroutine_leak", func(ctx context.Context) error {
		return fmt.Errorf("too many goroutines")
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/livez", nil)
	r.HandleLivez(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}

	var resp ProbeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != StatusFail {
		t.Errorf("response status = %q, want %q", resp.Status, StatusFail)
	}
}

func TestHandleReadyz_AllHealthy(t *testing.T) {
	r := NewProbeRegistry()
	r.AddReadinessCheck("database", func(ctx context.Context) error { return nil })
	r.AddReadinessCheck("event_bus", func(ctx context.Context) error { return nil })
	r.AddReadinessCheck("session_manager", func(ctx context.Context) error { return nil })

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)
	r.HandleReadyz(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp ProbeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != StatusPass {
		t.Errorf("response status = %q, want %q", resp.Status, StatusPass)
	}
	if len(resp.Checks) != 3 {
		t.Fatalf("check count = %d, want 3", len(resp.Checks))
	}
}

func TestHandleReadyz_DatabaseDown(t *testing.T) {
	r := NewProbeRegistry()
	r.AddReadinessCheck("database", DatabaseCheck(func(ctx context.Context) error {
		return fmt.Errorf("connection refused")
	}))
	r.AddReadinessCheck("event_bus", EventBusCheck(func(ctx context.Context) error { return nil }))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)
	r.HandleReadyz(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}

	var resp ProbeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != StatusFail {
		t.Fatalf("overall status = %q, want %q", resp.Status, StatusFail)
	}

	// Verify individual results.
	for _, c := range resp.Checks {
		switch c.Name {
		case "database":
			if c.Status != StatusFail {
				t.Errorf("database status = %q, want fail", c.Status)
			}
			if c.Message != "connection refused" {
				t.Errorf("database message = %q, want %q", c.Message, "connection refused")
			}
		case "event_bus":
			if c.Status != StatusPass {
				t.Errorf("event_bus status = %q, want pass", c.Status)
			}
		}
	}
}

func TestHandleReadyz_DiskSpaceWarning(t *testing.T) {
	r := NewProbeRegistry()
	r.AddReadinessCheck("database", DatabaseCheck(func(ctx context.Context) error { return nil }))
	r.AddReadinessWarning("disk_space", DiskSpaceCheck(os.TempDir(), ^uint64(0)))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)
	r.HandleReadyz(rr, req)

	// Warning does not cause 503.
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp ProbeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != StatusWarn {
		t.Errorf("overall status = %q, want %q", resp.Status, StatusWarn)
	}
}

// --- Server integration: /readyz with probes ---

func TestServer_ReadyzWithProbes(t *testing.T) {
	s := New(":0")
	s.Probes().AddReadinessCheck("db", func(ctx context.Context) error { return nil })

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)
	s.srv.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp ProbeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != StatusPass {
		t.Errorf("status = %q, want %q", resp.Status, StatusPass)
	}
}

func TestServer_ReadyzFallbackNoProbes(t *testing.T) {
	s := New(":0")

	// No probes registered, should use legacy boolean readiness.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)
	s.srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("not-ready status = %d, want 503", rr.Code)
	}

	s.SetReady()
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/readyz", nil)
	s.srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want 200", rr.Code)
	}
}

// --- Server integration: /livez ---

func TestServer_LivezWithProbes(t *testing.T) {
	s := New(":0")
	s.Probes().AddLivenessCheck("process", func(ctx context.Context) error { return nil })

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/livez", nil)
	s.srv.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp ProbeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != StatusPass {
		t.Errorf("status = %q, want %q", resp.Status, StatusPass)
	}
}

func TestServer_LivezFallback(t *testing.T) {
	s := New(":0")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/livez", nil)
	s.srv.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp ProbeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != StatusPass {
		t.Errorf("fallback status = %q, want %q", resp.Status, StatusPass)
	}
}

func TestServer_LivezUnhealthy(t *testing.T) {
	s := New(":0")
	s.Probes().AddLivenessCheck("deadlock", func(ctx context.Context) error {
		return fmt.Errorf("deadlocked")
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/livez", nil)
	s.srv.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

// --- End-to-end scenario: realistic multi-subsystem checks ---

func TestEndToEnd_RealisticProbeSet(t *testing.T) {
	reg := NewProbeRegistry()
	reg.SetTimeout(time.Second)

	// Simulate: database healthy, event bus healthy, session manager healthy,
	// disk space warning.
	reg.AddReadinessCheck("database", DatabaseCheck(func(ctx context.Context) error {
		return nil
	}))
	reg.AddReadinessCheck("event_bus", EventBusCheck(func(ctx context.Context) error {
		return nil
	}))
	reg.AddReadinessCheck("session_manager", SessionManagerCheck(func(ctx context.Context) error {
		return nil
	}))
	reg.AddReadinessWarning("disk_space", DiskSpaceCheck(os.TempDir(), ^uint64(0)))

	reg.AddLivenessCheck("process", func(ctx context.Context) error { return nil })

	// Readiness should warn (disk), not fail.
	readiness := reg.RunReadiness(context.Background())
	if readiness.Status != StatusWarn {
		t.Errorf("readiness = %q, want warn", readiness.Status)
	}
	if len(readiness.Checks) != 4 {
		t.Errorf("readiness checks = %d, want 4", len(readiness.Checks))
	}

	// Liveness should pass.
	liveness := reg.RunLiveness(context.Background())
	if liveness.Status != StatusPass {
		t.Errorf("liveness = %q, want pass", liveness.Status)
	}
}

func TestEndToEnd_AllDown(t *testing.T) {
	reg := NewProbeRegistry()
	reg.AddReadinessCheck("database", DatabaseCheck(func(ctx context.Context) error {
		return fmt.Errorf("connection refused")
	}))
	reg.AddReadinessCheck("event_bus", EventBusCheck(func(ctx context.Context) error {
		return fmt.Errorf("bus closed")
	}))
	reg.AddReadinessCheck("session_manager", SessionManagerCheck(func(ctx context.Context) error {
		return fmt.Errorf("deadlocked")
	}))

	resp := reg.RunReadiness(context.Background())
	if resp.Status != StatusFail {
		t.Fatalf("status = %q, want fail", resp.Status)
	}

	failCount := 0
	for _, c := range resp.Checks {
		if c.Status == StatusFail {
			failCount++
		}
	}
	if failCount != 3 {
		t.Errorf("fail count = %d, want 3", failCount)
	}
}

// --- JSON structure validation ---

func TestProbeResponse_JSONShape(t *testing.T) {
	reg := NewProbeRegistry()
	reg.AddReadinessCheck("db", func(ctx context.Context) error { return nil })
	reg.AddReadinessCheck("bus", func(ctx context.Context) error {
		return fmt.Errorf("test error")
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)
	reg.HandleReadyz(rr, req)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}

	// Validate top-level fields exist.
	for _, key := range []string{"status", "checks", "elapsed"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing key %q in response", key)
		}
	}

	// Validate check shape.
	var checks []map[string]json.RawMessage
	if err := json.Unmarshal(raw["checks"], &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks) != 2 {
		t.Fatalf("checks length = %d, want 2", len(checks))
	}
	for _, c := range checks {
		for _, key := range []string{"name", "status", "duration"} {
			if _, ok := c[key]; !ok {
				t.Errorf("check missing key %q", key)
			}
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input uint64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{2147483648, "2.0 GB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
