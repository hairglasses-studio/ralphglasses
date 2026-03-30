// Package firecracker provides Firecracker microVM management for sandboxed LLM sessions.
//
//go:build linux

package firecracker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestVMConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  VMConfig
		wantErr string
	}{
		{
			name:    "missing kernel",
			config:  VMConfig{VCPUs: 1, MemoryMB: 512, RootFS: "/tmp/rootfs.ext4"},
			wantErr: "kernel path is required",
		},
		{
			name:    "missing rootfs",
			config:  VMConfig{VCPUs: 1, MemoryMB: 512, Kernel: "/tmp/vmlinux"},
			wantErr: "rootfs path is required",
		},
		{
			name:    "vcpus too low",
			config:  VMConfig{Kernel: "/tmp/vmlinux", RootFS: "/tmp/rootfs.ext4", VCPUs: 0, MemoryMB: 512},
			wantErr: "vcpus must be >= 1",
		},
		{
			name:    "vcpus too high",
			config:  VMConfig{Kernel: "/tmp/vmlinux", RootFS: "/tmp/rootfs.ext4", VCPUs: 64, MemoryMB: 512},
			wantErr: "vcpus must be <= 32",
		},
		{
			name:    "memory too low",
			config:  VMConfig{Kernel: "/tmp/vmlinux", RootFS: "/tmp/rootfs.ext4", VCPUs: 1, MemoryMB: 64},
			wantErr: "memory_mb must be >= 128",
		},
		{
			name:    "memory too high",
			config:  VMConfig{Kernel: "/tmp/vmlinux", RootFS: "/tmp/rootfs.ext4", VCPUs: 1, MemoryMB: 100000},
			wantErr: "memory_mb must be <= 65536",
		},
		{
			name:    "bad network mode",
			config:  VMConfig{Kernel: "/tmp/vmlinux", RootFS: "/tmp/rootfs.ext4", VCPUs: 1, MemoryMB: 512, NetworkMode: "invalid"},
			wantErr: "unknown network mode",
		},
		{
			name:   "valid minimal",
			config: VMConfig{Kernel: "/tmp/vmlinux", RootFS: "/tmp/rootfs.ext4", VCPUs: 1, MemoryMB: 512},
		},
		{
			name:   "valid with network",
			config: VMConfig{Kernel: "/tmp/vmlinux", RootFS: "/tmp/rootfs.ext4", VCPUs: 4, MemoryMB: 2048, NetworkMode: NetworkTap},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.config.validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); !contains(got, tt.wantErr) {
				t.Errorf("error %q does not contain %q", got, tt.wantErr)
			}
		})
	}
}

func TestNewVMDefaults(t *testing.T) {
	t.Parallel()

	vm := NewVM("test-vm")
	if vm.Name() != "test-vm" {
		t.Errorf("Name() = %q, want %q", vm.Name(), "test-vm")
	}

	cfg := vm.Config()
	if cfg.VCPUs != 1 {
		t.Errorf("VCPUs = %d, want 1", cfg.VCPUs)
	}
	if cfg.MemoryMB != 512 {
		t.Errorf("MemoryMB = %d, want 512", cfg.MemoryMB)
	}
	if vm.Status() != VMStatusStopped {
		t.Errorf("Status() = %v, want %v", vm.Status(), VMStatusStopped)
	}
	if vm.PID() != 0 {
		t.Errorf("PID() = %d, want 0", vm.PID())
	}
}

func TestNewVMWithOptions(t *testing.T) {
	t.Parallel()

	vm := NewVM("opts-vm",
		WithKernel("/boot/vmlinux"),
		WithRootFS("/images/rootfs.ext4"),
		WithVCPUs(4),
		WithMemMB(2048),
		WithNetwork(NetworkTap, 2222),
		WithSSHKey("/home/user/.ssh/id_ed25519"),
		WithFirecrackerBin("/usr/local/bin/firecracker"),
	)

	cfg := vm.Config()
	if cfg.Kernel != "/boot/vmlinux" {
		t.Errorf("Kernel = %q, want %q", cfg.Kernel, "/boot/vmlinux")
	}
	if cfg.RootFS != "/images/rootfs.ext4" {
		t.Errorf("RootFS = %q, want %q", cfg.RootFS, "/images/rootfs.ext4")
	}
	if cfg.VCPUs != 4 {
		t.Errorf("VCPUs = %d, want 4", cfg.VCPUs)
	}
	if cfg.MemoryMB != 2048 {
		t.Errorf("MemoryMB = %d, want 2048", cfg.MemoryMB)
	}
	if cfg.NetworkMode != NetworkTap {
		t.Errorf("NetworkMode = %q, want %q", cfg.NetworkMode, NetworkTap)
	}
	if cfg.SSHPort != 2222 {
		t.Errorf("SSHPort = %d, want 2222", cfg.SSHPort)
	}
	if cfg.SSHKeyPath != "/home/user/.ssh/id_ed25519" {
		t.Errorf("SSHKeyPath = %q, want %q", cfg.SSHKeyPath, "/home/user/.ssh/id_ed25519")
	}
	if cfg.FirecrackerBin != "/usr/local/bin/firecracker" {
		t.Errorf("FirecrackerBin = %q, want %q", cfg.FirecrackerBin, "/usr/local/bin/firecracker")
	}
}

func TestVMStatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status VMStatus
		want   string
	}{
		{VMStatusStopped, "stopped"},
		{VMStatusStarting, "starting"},
		{VMStatusRunning, "running"},
		{VMStatusError, "error"},
		{VMStatus(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("VMStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestVMStartValidationError(t *testing.T) {
	t.Parallel()

	vm := NewVM("bad-vm") // no kernel/rootfs
	err := vm.Start(context.Background())
	if err == nil {
		t.Fatal("expected validation error")
	}
	if vm.Status() != VMStatusError {
		t.Errorf("Status() = %v, want %v", vm.Status(), VMStatusError)
	}
	if vm.Err() == nil {
		t.Error("Err() should be non-nil after failed start")
	}
}

func TestVMStartAlreadyRunning(t *testing.T) {
	t.Parallel()

	vm := NewVM("running-vm",
		WithKernel("/tmp/vmlinux"),
		WithRootFS("/tmp/rootfs.ext4"),
	)
	// Override start to succeed without launching a real process.
	vm.startFn = func(_ context.Context) error { return nil }

	if err := vm.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if vm.Status() != VMStatusRunning {
		t.Fatalf("Status() = %v, want running", vm.Status())
	}

	// Second start should fail.
	err := vm.Start(context.Background())
	if err == nil {
		t.Fatal("expected error on double start")
	}
	if got := err.Error(); !contains(got, "already running") {
		t.Errorf("error %q does not mention 'already running'", got)
	}
}

func TestVMStopAlreadyStopped(t *testing.T) {
	t.Parallel()

	vm := NewVM("stopped-vm")
	if err := vm.Stop(context.Background()); err != nil {
		t.Fatalf("Stop on stopped VM should be nil: %v", err)
	}
}

func TestVMExecSSHNotRunning(t *testing.T) {
	t.Parallel()

	vm := NewVM("stopped-vm")
	_, err := vm.ExecSSH(context.Background(), "echo hello")
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "cannot exec") {
		t.Errorf("error %q should mention 'cannot exec'", err.Error())
	}
}

func TestVMExecSSHNoPort(t *testing.T) {
	t.Parallel()

	vm := NewVM("no-port",
		WithKernel("/tmp/vmlinux"),
		WithRootFS("/tmp/rootfs.ext4"),
	)
	vm.startFn = func(_ context.Context) error { return nil }
	// Use default execFn which checks SSHPort.

	if err := vm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	_, err := vm.ExecSSH(context.Background(), "echo hello")
	if err == nil {
		t.Fatal("expected error for no SSH port")
	}
	if !contains(err.Error(), "SSH port not configured") {
		t.Errorf("error %q should mention SSH port", err.Error())
	}
}

func TestVMExecSSHMocked(t *testing.T) {
	t.Parallel()

	vm := NewVM("mock-ssh",
		WithKernel("/tmp/vmlinux"),
		WithRootFS("/tmp/rootfs.ext4"),
	)
	vm.startFn = func(_ context.Context) error { return nil }
	vm.execFn = func(_ context.Context, cmd string) (string, error) {
		return "hello from guest: " + cmd, nil
	}

	if err := vm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	out, err := vm.ExecSSH(context.Background(), "uname -a")
	if err != nil {
		t.Fatalf("ExecSSH: %v", err)
	}
	if want := "hello from guest: uname -a"; out != want {
		t.Errorf("ExecSSH = %q, want %q", out, want)
	}
}

func TestVMStartError(t *testing.T) {
	t.Parallel()

	vm := NewVM("fail-vm",
		WithKernel("/tmp/vmlinux"),
		WithRootFS("/tmp/rootfs.ext4"),
	)
	wantErr := errors.New("injected failure")
	vm.startFn = func(_ context.Context) error { return wantErr }

	err := vm.Start(context.Background())
	if !errors.Is(err, wantErr) {
		t.Errorf("Start error = %v, want %v", err, wantErr)
	}
	if vm.Status() != VMStatusError {
		t.Errorf("Status() = %v, want error", vm.Status())
	}
}

func TestVMConcurrentStatusAccess(t *testing.T) {
	t.Parallel()

	vm := NewVM("concurrent-vm",
		WithKernel("/tmp/vmlinux"),
		WithRootFS("/tmp/rootfs.ext4"),
	)
	vm.startFn = func(_ context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	var wg sync.WaitGroup
	// Read status concurrently while starting.
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = vm.Status()
			_ = vm.PID()
			_ = vm.Name()
			_ = vm.Config()
			_ = vm.Err()
		}()
	}

	_ = vm.Start(context.Background())
	wg.Wait()
}

// TestMockSocketAPI verifies the apiPut method against a mock Unix socket server.
func TestMockSocketAPI(t *testing.T) {
	t.Parallel()

	// Create a mock server on a Unix socket.
	dir := t.TempDir()
	sockPath := dir + "/test.sock"

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	var receivedPath string
	var receivedBody map[string]any
	var mu sync.Mutex

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		receivedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusNoContent)
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()
	defer srv.Close()

	vm := NewVM("api-test")
	vm.httpClient = &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	ctx := context.Background()
	err = vm.apiPut(ctx, "/machine-config", map[string]any{
		"vcpu_count":  2,
		"mem_size_mib": 1024,
	})
	if err != nil {
		t.Fatalf("apiPut: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if receivedPath != "/machine-config" {
		t.Errorf("path = %q, want /machine-config", receivedPath)
	}
	if v, ok := receivedBody["vcpu_count"].(float64); !ok || int(v) != 2 {
		t.Errorf("vcpu_count = %v, want 2", receivedBody["vcpu_count"])
	}
}

// TestMockSocketAPIError verifies error handling for non-2xx responses.
func TestMockSocketAPIError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sockPath := dir + "/err.sock"

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad config"}`))
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()
	defer srv.Close()

	vm := NewVM("err-test")
	vm.httpClient = &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	err = vm.apiPut(context.Background(), "/machine-config", map[string]any{"bad": true})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !contains(err.Error(), "400") {
		t.Errorf("error %q should contain status code 400", err.Error())
	}
}

func TestFirecrackerBinDefault(t *testing.T) {
	t.Parallel()

	cfg := VMConfig{}
	if got := cfg.firecrackerBin(); got != "firecracker" {
		t.Errorf("firecrackerBin() = %q, want %q", got, "firecracker")
	}

	cfg.FirecrackerBin = "/opt/bin/firecracker"
	if got := cfg.firecrackerBin(); got != "/opt/bin/firecracker" {
		t.Errorf("firecrackerBin() = %q, want %q", got, "/opt/bin/firecracker")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && containsImpl(s, sub)
}

func containsImpl(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
