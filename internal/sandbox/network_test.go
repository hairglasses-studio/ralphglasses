package sandbox

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// skipUnlessLinuxRoot skips the test if not running on Linux with root privileges.
func skipUnlessLinuxRoot(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("network namespace tests require Linux")
	}
	if os.Geteuid() != 0 {
		t.Skip("network namespace tests require root")
	}
	// Verify ip command is available.
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip command not found")
	}
}

func TestRequireLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		if err := requireLinux(); err != nil {
			t.Errorf("requireLinux() on Linux should return nil, got %v", err)
		}
	} else {
		if err := requireLinux(); err != ErrNotLinux {
			t.Errorf("requireLinux() on %s should return ErrNotLinux, got %v", runtime.GOOS, err)
		}
	}
}

func TestNetnsPath(t *testing.T) {
	got := netnsPath("test-ns")
	want := "/var/run/netns/test-ns"
	if got != want {
		t.Errorf("netnsPath(\"test-ns\") = %q, want %q", got, want)
	}
}

func TestCreateNetworkNS_NotLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test is for non-Linux platforms")
	}
	_, err := CreateNetworkNS("test")
	if err != ErrNotLinux {
		t.Errorf("CreateNetworkNS on non-Linux should return ErrNotLinux, got %v", err)
	}
}

func TestDeleteNetworkNS_Nil(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux")
	}
	err := DeleteNetworkNS(nil)
	if err == nil {
		t.Error("DeleteNetworkNS(nil) should return error")
	}
}

func TestExecInNS_Nil(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux")
	}
	_, err := ExecInNS(context.Background(), nil, []string{"echo", "hi"})
	if err == nil {
		t.Error("ExecInNS with nil NS should return error")
	}
}

func TestExecInNS_EmptyCmd(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux")
	}
	ns := &NetworkNS{Name: "fake"}
	_, err := ExecInNS(context.Background(), ns, nil)
	if err == nil {
		t.Error("ExecInNS with empty command should return error")
	}
}

func TestCreateNetworkNS_EmptyName(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux")
	}
	_, err := CreateNetworkNS("")
	if err == nil {
		t.Error("CreateNetworkNS with empty name should return error")
	}
}

func TestAllowList_NotLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test is for non-Linux platforms")
	}
	err := AllowList(&NetworkNS{Name: "test"}, []string{"10.0.0.0/8"})
	if err != ErrNotLinux {
		t.Errorf("AllowList on non-Linux should return ErrNotLinux, got %v", err)
	}
}

func TestBlockAll_NotLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test is for non-Linux platforms")
	}
	err := BlockAll(&NetworkNS{Name: "test"})
	if err != ErrNotLinux {
		t.Errorf("BlockAll on non-Linux should return ErrNotLinux, got %v", err)
	}
}

func TestAllowDNS_NotLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test is for non-Linux platforms")
	}
	err := AllowDNS(&NetworkNS{Name: "test"})
	if err != ErrNotLinux {
		t.Errorf("AllowDNS on non-Linux should return ErrNotLinux, got %v", err)
	}
}

func TestAllowHTTPS_NotLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test is for non-Linux platforms")
	}
	err := AllowHTTPS(&NetworkNS{Name: "test"}, []string{"api.anthropic.com"})
	if err != ErrNotLinux {
		t.Errorf("AllowHTTPS on non-Linux should return ErrNotLinux, got %v", err)
	}
}

func TestListNamespaces_NotLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test is for non-Linux platforms")
	}
	_, err := ListNamespaces()
	if err != ErrNotLinux {
		t.Errorf("ListNamespaces on non-Linux should return ErrNotLinux, got %v", err)
	}
}

// Integration tests — require Linux + root.

func TestCreateDeleteNetworkNS(t *testing.T) {
	skipUnlessLinuxRoot(t)

	ns, err := CreateNetworkNS("ralph-test-ns")
	if err != nil {
		t.Fatalf("CreateNetworkNS: %v", err)
	}
	defer func() { _ = DeleteNetworkNS(ns) }()

	if ns.Name != "ralph-test-ns" {
		t.Errorf("Name = %q, want %q", ns.Name, "ralph-test-ns")
	}
	if ns.Path != "/var/run/netns/ralph-test-ns" {
		t.Errorf("Path = %q, want %q", ns.Path, "/var/run/netns/ralph-test-ns")
	}

	// Verify it appears in the list.
	nsList, err := ListNamespaces()
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	found := false
	for _, n := range nsList {
		if n.Name == "ralph-test-ns" {
			found = true
			break
		}
	}
	if !found {
		t.Error("namespace ralph-test-ns not found in ListNamespaces()")
	}

	// Creating again should fail.
	_, err = CreateNetworkNS("ralph-test-ns")
	if err != ErrNSAlreadyExist {
		t.Errorf("duplicate CreateNetworkNS should return ErrNSAlreadyExist, got %v", err)
	}

	// Delete.
	if err := DeleteNetworkNS(ns); err != nil {
		t.Fatalf("DeleteNetworkNS: %v", err)
	}

	// Delete again should fail.
	if err := DeleteNetworkNS(ns); err != ErrNSNotFound {
		t.Errorf("double DeleteNetworkNS should return ErrNSNotFound, got %v", err)
	}
}

func TestExecInNS(t *testing.T) {
	skipUnlessLinuxRoot(t)

	ns, err := CreateNetworkNS("ralph-test-exec")
	if err != nil {
		t.Fatalf("CreateNetworkNS: %v", err)
	}
	defer func() { _ = DeleteNetworkNS(ns) }()

	out, err := ExecInNS(context.Background(), ns, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("ExecInNS: %v", err)
	}
	if out != "hello" {
		t.Errorf("output = %q, want %q", out, "hello")
	}

	// Test context cancellation.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond) // Ensure timeout fires.
	_, err = ExecInNS(ctx, ns, []string{"sleep", "10"})
	if err == nil {
		t.Error("ExecInNS with expired context should return error")
	}
}

func TestBlockAll(t *testing.T) {
	skipUnlessLinuxRoot(t)

	ns, err := CreateNetworkNS("ralph-test-block")
	if err != nil {
		t.Fatalf("CreateNetworkNS: %v", err)
	}
	defer func() { _ = DeleteNetworkNS(ns) }()

	if err := BlockAll(ns); err != nil {
		t.Fatalf("BlockAll: %v", err)
	}

	// Verify iptables rules are set to DROP.
	out, err := ExecInNS(context.Background(), ns, []string{"iptables", "-L", "OUTPUT", "-n"})
	if err != nil {
		t.Fatalf("iptables -L: %v", err)
	}
	if !strings.Contains(out, "DROP") {
		t.Errorf("expected DROP policy, got: %s", out)
	}
}

func TestAllowDNS(t *testing.T) {
	skipUnlessLinuxRoot(t)

	ns, err := CreateNetworkNS("ralph-test-dns")
	if err != nil {
		t.Fatalf("CreateNetworkNS: %v", err)
	}
	defer func() { _ = DeleteNetworkNS(ns) }()

	if err := AllowDNS(ns); err != nil {
		t.Fatalf("AllowDNS: %v", err)
	}

	// Verify DNS port 53 is allowed.
	out, err := ExecInNS(context.Background(), ns, []string{"iptables", "-L", "OUTPUT", "-n"})
	if err != nil {
		t.Fatalf("iptables -L: %v", err)
	}
	if !strings.Contains(out, "53") {
		t.Errorf("expected port 53 in rules, got: %s", out)
	}
}

func TestAllowList(t *testing.T) {
	skipUnlessLinuxRoot(t)

	ns, err := CreateNetworkNS("ralph-test-allow")
	if err != nil {
		t.Fatalf("CreateNetworkNS: %v", err)
	}
	defer func() { _ = DeleteNetworkNS(ns) }()

	cidrs := []string{"10.0.0.0/8", "172.16.0.0/12"}
	if err := AllowList(ns, cidrs); err != nil {
		t.Fatalf("AllowList: %v", err)
	}

	out, err := ExecInNS(context.Background(), ns, []string{"iptables", "-L", "OUTPUT", "-n"})
	if err != nil {
		t.Fatalf("iptables -L: %v", err)
	}
	if !strings.Contains(out, "10.0.0.0/8") {
		t.Errorf("expected 10.0.0.0/8 in rules, got: %s", out)
	}
	if !strings.Contains(out, "172.16.0.0/12") {
		t.Errorf("expected 172.16.0.0/12 in rules, got: %s", out)
	}
}

func TestAllowHTTPS(t *testing.T) {
	skipUnlessLinuxRoot(t)

	ns, err := CreateNetworkNS("ralph-test-https")
	if err != nil {
		t.Fatalf("CreateNetworkNS: %v", err)
	}
	defer func() { _ = DeleteNetworkNS(ns) }()

	// Use IP directly to avoid DNS resolution dependency in test.
	if err := AllowHTTPS(ns, []string{"1.1.1.1"}); err != nil {
		t.Fatalf("AllowHTTPS: %v", err)
	}

	out, err := ExecInNS(context.Background(), ns, []string{"iptables", "-L", "OUTPUT", "-n"})
	if err != nil {
		t.Fatalf("iptables -L: %v", err)
	}
	if !strings.Contains(out, "443") {
		t.Errorf("expected port 443 in rules, got: %s", out)
	}
	if !strings.Contains(out, "1.1.1.1") {
		t.Errorf("expected 1.1.1.1 in rules, got: %s", out)
	}
}

func TestAllowList_NilNS(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux")
	}
	err := AllowList(nil, []string{"10.0.0.0/8"})
	if err == nil {
		t.Error("AllowList with nil NS should return error")
	}
}

func TestBlockAll_NilNS(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux")
	}
	err := BlockAll(nil)
	if err == nil {
		t.Error("BlockAll with nil NS should return error")
	}
}

func TestAllowDNS_NilNS(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux")
	}
	err := AllowDNS(nil)
	if err == nil {
		t.Error("AllowDNS with nil NS should return error")
	}
}

func TestAllowHTTPS_NilNS(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux")
	}
	err := AllowHTTPS(nil, []string{"example.com"})
	if err == nil {
		t.Error("AllowHTTPS with nil NS should return error")
	}
}
