package fleet

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

// FleetTag for peer filtering is defined in server.go (tag:ralph-fleet).

// PeerMapping associates a Tailscale peer with its fleet worker identity.
type PeerMapping struct {
	Hostname     string `json:"hostname"`
	WorkerID     string `json:"worker_id"`
	TailscaleIP  string `json:"tailscale_ip"`
	DNSName      string `json:"dns_name"`
	Online       bool   `json:"online"`
	OS           string `json:"os,omitempty"`
	HasFleetTag  bool   `json:"has_fleet_tag"`
	LastSeen     time.Time `json:"last_seen"`
}

// TsnetDiscovery discovers fleet peers on the Tailscale network using
// the TailscaleClient interface (exec-based CLI or LocalAPI). It maps
// Tailscale hostnames to fleet worker IDs and tracks peer online status.
type TsnetDiscovery struct {
	mu       sync.RWMutex
	tsClient TailscaleClient

	// peers maps Tailscale hostname -> PeerMapping
	peers map[string]*PeerMapping

	// workerIndex maps worker ID -> hostname for reverse lookups
	workerIndex map[string]string

	// hostnameToWorkerID is the user-supplied mapping from Tailscale
	// hostname prefixes to fleet worker IDs. If nil, hostnames are used
	// directly as worker IDs.
	hostnameToWorkerID map[string]string

	// fleetPort is the port where fleet workers serve their API.
	fleetPort int

	// requireFleetTag filters peers to only those with tag:ralph-fleet.
	requireFleetTag bool
}

// TsnetDiscoveryConfig configures a TsnetDiscovery instance.
type TsnetDiscoveryConfig struct {
	// TSClient is the Tailscale client to use. If nil, DefaultTailscaleClient() is used.
	TSClient TailscaleClient

	// HostnameMap maps Tailscale hostnames to fleet worker IDs.
	// If empty, the Tailscale hostname is used directly as the worker ID.
	HostnameMap map[string]string

	// FleetPort is the port on which fleet workers listen. Defaults to DefaultPort.
	FleetPort int

	// RequireFleetTag restricts discovery to peers tagged with tag:ralph-fleet.
	RequireFleetTag bool
}

// NewTsnetDiscovery creates a new Tailscale-based peer discovery service.
func NewTsnetDiscovery(cfg TsnetDiscoveryConfig) *TsnetDiscovery {
	tc := cfg.TSClient
	if tc == nil {
		tc = DefaultTailscaleClient()
	}
	port := cfg.FleetPort
	if port == 0 {
		port = DefaultPort
	}
	return &TsnetDiscovery{
		tsClient:           tc,
		peers:              make(map[string]*PeerMapping),
		workerIndex:        make(map[string]string),
		hostnameToWorkerID: cfg.HostnameMap,
		fleetPort:          port,
		requireFleetTag:    cfg.RequireFleetTag,
	}
}

// Refresh queries Tailscale status and updates the internal peer map.
// Returns the number of online fleet peers discovered.
func (d *TsnetDiscovery) Refresh(ctx context.Context) (int, error) {
	status, err := d.tsClient.Status(ctx)
	if err != nil {
		return 0, fmt.Errorf("tsnet discovery: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	seen := make(map[string]bool)

	for _, peer := range status.Peer {
		if !peer.Online {
			continue
		}
		if d.requireFleetTag && !peer.HasTag(FleetTag) {
			continue
		}

		hostname := peer.HostName
		if hostname == "" {
			continue
		}
		seen[hostname] = true

		workerID := d.resolveWorkerID(hostname)
		ip := firstIPv4(peer.TailscaleIPs)

		existing, ok := d.peers[hostname]
		if ok {
			existing.Online = true
			existing.TailscaleIP = ip
			existing.HasFleetTag = peer.HasTag(FleetTag)
			existing.LastSeen = now
			existing.OS = peer.OS
			// Update worker index if mapping changed
			if existing.WorkerID != workerID {
				delete(d.workerIndex, existing.WorkerID)
				existing.WorkerID = workerID
				d.workerIndex[workerID] = hostname
			}
		} else {
			mapping := &PeerMapping{
				Hostname:    hostname,
				WorkerID:    workerID,
				TailscaleIP: ip,
				DNSName:     peer.DNSName,
				Online:      true,
				OS:          peer.OS,
				HasFleetTag: peer.HasTag(FleetTag),
				LastSeen:    now,
			}
			d.peers[hostname] = mapping
			d.workerIndex[workerID] = hostname
		}
	}

	// Mark unseen peers as offline
	for hostname, peer := range d.peers {
		if !seen[hostname] {
			peer.Online = false
		}
	}

	count := len(seen)
	util.Debug.Debugf("tsnet discovery: found %d online fleet peers", count)
	return count, nil
}

// Peers returns a snapshot of all known peer mappings.
func (d *TsnetDiscovery) Peers() []PeerMapping {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]PeerMapping, 0, len(d.peers))
	for _, p := range d.peers {
		result = append(result, *p)
	}
	return result
}

// OnlinePeers returns only peers currently marked as online.
func (d *TsnetDiscovery) OnlinePeers() []PeerMapping {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []PeerMapping
	for _, p := range d.peers {
		if p.Online {
			result = append(result, *p)
		}
	}
	return result
}

// LookupByHostname returns the peer mapping for a given Tailscale hostname.
func (d *TsnetDiscovery) LookupByHostname(hostname string) (PeerMapping, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	p, ok := d.peers[hostname]
	if !ok {
		return PeerMapping{}, false
	}
	return *p, true
}

// LookupByWorkerID returns the peer mapping for a given fleet worker ID.
func (d *TsnetDiscovery) LookupByWorkerID(workerID string) (PeerMapping, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	hostname, ok := d.workerIndex[workerID]
	if !ok {
		return PeerMapping{}, false
	}
	p, ok := d.peers[hostname]
	if !ok {
		return PeerMapping{}, false
	}
	return *p, true
}

// WorkerURL returns the fleet API base URL for a given worker ID.
func (d *TsnetDiscovery) WorkerURL(workerID string) (string, bool) {
	peer, ok := d.LookupByWorkerID(workerID)
	if !ok || peer.TailscaleIP == "" {
		return "", false
	}
	return fmt.Sprintf("http://%s:%d", peer.TailscaleIP, d.fleetPort), true
}

// SelfInfo returns the current node's Tailscale identity.
func (d *TsnetDiscovery) SelfInfo(ctx context.Context) (*PeerMapping, error) {
	status, err := d.tsClient.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("tsnet discovery self: %w", err)
	}

	hostname := status.Self.HostName
	workerID := d.resolveWorkerID(hostname)
	ip := firstIPv4(status.Self.TailscaleIPs)

	return &PeerMapping{
		Hostname:    hostname,
		WorkerID:    workerID,
		TailscaleIP: ip,
		DNSName:     status.Self.DNSName,
		Online:      status.Self.Online,
		OS:          status.Self.OS,
		HasFleetTag: status.Self.HasTag(FleetTag),
		LastSeen:    time.Now(),
	}, nil
}

// resolveWorkerID maps a Tailscale hostname to a fleet worker ID.
// Checks the explicit hostname map first, then falls back to using the
// hostname directly as the worker ID.
func (d *TsnetDiscovery) resolveWorkerID(hostname string) string {
	if d.hostnameToWorkerID != nil {
		// Exact match
		if wid, ok := d.hostnameToWorkerID[hostname]; ok {
			return wid
		}
		// Try lowercase match
		lower := strings.ToLower(hostname)
		if wid, ok := d.hostnameToWorkerID[lower]; ok {
			return wid
		}
	}
	return hostname
}

// firstIPv4 returns the first IPv4 address from a list of Tailscale IPs,
// or the first address of any kind if no IPv4 is found.
func firstIPv4(ips []string) string {
	for _, ip := range ips {
		if !strings.Contains(ip, ":") {
			return ip
		}
	}
	if len(ips) > 0 {
		return ips[0]
	}
	return ""
}
