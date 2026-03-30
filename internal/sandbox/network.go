// Package sandbox provides container isolation for LLM sessions.
// Network namespace management for sandboxed agents.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// Sentinel errors for network namespace operations.
var (
	ErrNotLinux       = errors.New("network namespaces require Linux")
	ErrNotRoot        = errors.New("network namespaces require root privileges")
	ErrNSNotFound     = errors.New("network namespace not found")
	ErrNSAlreadyExist = errors.New("network namespace already exists")
)

// NetworkNS represents an isolated network namespace.
type NetworkNS struct {
	// Name is the namespace identifier used with ip-netns.
	Name string `json:"name"`

	// Path is the filesystem path to the namespace (e.g. /var/run/netns/<name>).
	Path string `json:"path"`

	mu sync.Mutex
}

// netnsPath returns the filesystem path for a named network namespace.
func netnsPath(name string) string {
	return "/var/run/netns/" + name
}

// requireLinux returns ErrNotLinux if not running on Linux.
func requireLinux() error {
	if runtime.GOOS != "linux" {
		return ErrNotLinux
	}
	return nil
}

// CreateNetworkNS creates a new network namespace with the given name.
// Requires Linux and root privileges.
func CreateNetworkNS(name string) (*NetworkNS, error) {
	if err := requireLinux(); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("network namespace name must not be empty")
	}

	// Create the namespace via ip netns add.
	out, err := exec.Command("ip", "netns", "add", name).CombinedOutput()
	if err != nil {
		s := strings.TrimSpace(string(out))
		if strings.Contains(s, "File exists") {
			return nil, ErrNSAlreadyExist
		}
		return nil, fmt.Errorf("ip netns add %s: %s: %w", name, s, err)
	}

	// Bring up loopback inside the namespace.
	lo, err := exec.Command("ip", "netns", "exec", name, "ip", "link", "set", "lo", "up").CombinedOutput()
	if err != nil {
		// Best-effort cleanup.
		_ = exec.Command("ip", "netns", "delete", name).Run()
		return nil, fmt.Errorf("bring up loopback in %s: %s: %w", name, strings.TrimSpace(string(lo)), err)
	}

	return &NetworkNS{
		Name: name,
		Path: netnsPath(name),
	}, nil
}

// DeleteNetworkNS removes a network namespace and its associated iptables rules.
func DeleteNetworkNS(ns *NetworkNS) error {
	if err := requireLinux(); err != nil {
		return err
	}
	if ns == nil {
		return fmt.Errorf("nil NetworkNS")
	}
	ns.mu.Lock()
	defer ns.mu.Unlock()

	out, err := exec.Command("ip", "netns", "delete", ns.Name).CombinedOutput()
	if err != nil {
		s := strings.TrimSpace(string(out))
		if strings.Contains(s, "No such file") {
			return ErrNSNotFound
		}
		return fmt.Errorf("ip netns delete %s: %s: %w", ns.Name, s, err)
	}
	return nil
}

// AllowList configures iptables inside the namespace to allow traffic only to
// the specified CIDRs and drop everything else.
func AllowList(ns *NetworkNS, cidrs []string) error {
	if err := requireLinux(); err != nil {
		return err
	}
	if ns == nil {
		return fmt.Errorf("nil NetworkNS")
	}
	ns.mu.Lock()
	defer ns.mu.Unlock()

	// Flush existing rules.
	if err := iptablesInNS(ns.Name, "-F"); err != nil {
		return fmt.Errorf("flush iptables in %s: %w", ns.Name, err)
	}

	// Allow loopback.
	if err := iptablesInNS(ns.Name, "-A", "INPUT", "-i", "lo", "-j", "ACCEPT"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT"); err != nil {
		return err
	}

	// Allow established/related connections.
	if err := iptablesInNS(ns.Name, "-A", "INPUT", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return err
	}

	// Allow each CIDR.
	for _, cidr := range cidrs {
		if err := iptablesInNS(ns.Name, "-A", "OUTPUT", "-d", cidr, "-j", "ACCEPT"); err != nil {
			return fmt.Errorf("allow CIDR %s: %w", cidr, err)
		}
	}

	// Drop everything else.
	if err := iptablesInNS(ns.Name, "-P", "INPUT", "DROP"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-P", "OUTPUT", "DROP"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-P", "FORWARD", "DROP"); err != nil {
		return err
	}

	return nil
}

// BlockAll blocks all network traffic in the namespace.
func BlockAll(ns *NetworkNS) error {
	if err := requireLinux(); err != nil {
		return err
	}
	if ns == nil {
		return fmt.Errorf("nil NetworkNS")
	}
	ns.mu.Lock()
	defer ns.mu.Unlock()

	// Flush and set all policies to DROP.
	if err := iptablesInNS(ns.Name, "-F"); err != nil {
		return fmt.Errorf("flush iptables in %s: %w", ns.Name, err)
	}
	if err := iptablesInNS(ns.Name, "-P", "INPUT", "DROP"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-P", "OUTPUT", "DROP"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-P", "FORWARD", "DROP"); err != nil {
		return err
	}
	return nil
}

// AllowDNS allows only DNS traffic (TCP/UDP port 53) in the namespace.
// All other traffic is blocked.
func AllowDNS(ns *NetworkNS) error {
	if err := requireLinux(); err != nil {
		return err
	}
	if ns == nil {
		return fmt.Errorf("nil NetworkNS")
	}
	ns.mu.Lock()
	defer ns.mu.Unlock()

	// Flush existing rules.
	if err := iptablesInNS(ns.Name, "-F"); err != nil {
		return fmt.Errorf("flush iptables in %s: %w", ns.Name, err)
	}

	// Allow loopback.
	if err := iptablesInNS(ns.Name, "-A", "INPUT", "-i", "lo", "-j", "ACCEPT"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT"); err != nil {
		return err
	}

	// Allow established/related.
	if err := iptablesInNS(ns.Name, "-A", "INPUT", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return err
	}

	// Allow DNS outbound (UDP and TCP port 53).
	if err := iptablesInNS(ns.Name, "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-j", "ACCEPT"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-j", "ACCEPT"); err != nil {
		return err
	}

	// Drop everything else.
	if err := iptablesInNS(ns.Name, "-P", "INPUT", "DROP"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-P", "OUTPUT", "DROP"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-P", "FORWARD", "DROP"); err != nil {
		return err
	}

	return nil
}

// AllowHTTPS allows DNS and HTTPS (TCP port 443) traffic to the specified hosts.
// All other traffic is blocked. Hosts are resolved to IPs and added as iptables rules.
func AllowHTTPS(ns *NetworkNS, hosts []string) error {
	if err := requireLinux(); err != nil {
		return err
	}
	if ns == nil {
		return fmt.Errorf("nil NetworkNS")
	}
	ns.mu.Lock()
	defer ns.mu.Unlock()

	// Flush existing rules.
	if err := iptablesInNS(ns.Name, "-F"); err != nil {
		return fmt.Errorf("flush iptables in %s: %w", ns.Name, err)
	}

	// Allow loopback.
	if err := iptablesInNS(ns.Name, "-A", "INPUT", "-i", "lo", "-j", "ACCEPT"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT"); err != nil {
		return err
	}

	// Allow established/related.
	if err := iptablesInNS(ns.Name, "-A", "INPUT", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return err
	}

	// Allow DNS (needed to resolve hosts).
	if err := iptablesInNS(ns.Name, "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-j", "ACCEPT"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-j", "ACCEPT"); err != nil {
		return err
	}

	// Resolve each host and allow HTTPS.
	for _, host := range hosts {
		// Use getent to resolve in the namespace context.
		out, err := exec.Command("ip", "netns", "exec", ns.Name,
			"getent", "ahosts", host).CombinedOutput()
		if err != nil {
			// If resolution fails, allow by hostname string as a CIDR or raw IP.
			// This lets callers pass CIDRs directly too.
			if err2 := iptablesInNS(ns.Name, "-A", "OUTPUT", "-p", "tcp", "--dport", "443", "-d", host, "-j", "ACCEPT"); err2 != nil {
				return fmt.Errorf("allow HTTPS to %s: %w", host, err2)
			}
			continue
		}

		// Parse resolved IPs (unique).
		seen := make(map[string]bool)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			ip := fields[0]
			if seen[ip] {
				continue
			}
			seen[ip] = true
			if err := iptablesInNS(ns.Name, "-A", "OUTPUT", "-p", "tcp", "--dport", "443", "-d", ip, "-j", "ACCEPT"); err != nil {
				return fmt.Errorf("allow HTTPS to %s (%s): %w", host, ip, err)
			}
		}
	}

	// Drop everything else.
	if err := iptablesInNS(ns.Name, "-P", "INPUT", "DROP"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-P", "OUTPUT", "DROP"); err != nil {
		return err
	}
	if err := iptablesInNS(ns.Name, "-P", "FORWARD", "DROP"); err != nil {
		return err
	}

	return nil
}

// ExecInNS runs a command inside the given network namespace and returns
// combined stdout/stderr output. The context controls the execution deadline.
func ExecInNS(ctx context.Context, ns *NetworkNS, cmd []string) (string, error) {
	if err := requireLinux(); err != nil {
		return "", err
	}
	if ns == nil {
		return "", fmt.Errorf("nil NetworkNS")
	}
	if len(cmd) == 0 {
		return "", fmt.Errorf("empty command")
	}

	// Build: ip netns exec <name> <cmd...>
	args := append([]string{"netns", "exec", ns.Name}, cmd...)
	c := exec.CommandContext(ctx, "ip", args...)
	out, err := c.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("exec in ns %s: %s: %w", ns.Name, strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ListNamespaces returns all network namespaces currently configured on the system.
func ListNamespaces() ([]*NetworkNS, error) {
	if err := requireLinux(); err != nil {
		return nil, err
	}

	out, err := exec.Command("ip", "netns", "list").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ip netns list: %s: %w", strings.TrimSpace(string(out)), err)
	}

	var result []*NetworkNS
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// ip netns list outputs "name (id: N)" or just "name".
		name := strings.Fields(line)[0]
		result = append(result, &NetworkNS{
			Name: name,
			Path: netnsPath(name),
		})
	}
	return result, nil
}

// iptablesInNS runs iptables with the given arguments inside a network namespace.
func iptablesInNS(nsName string, args ...string) error {
	fullArgs := append([]string{"netns", "exec", nsName, "iptables"}, args...)
	out, err := exec.Command("ip", fullArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %s in ns %s: %s: %w",
			strings.Join(args, " "), nsName, strings.TrimSpace(string(out)), err)
	}
	return nil
}
