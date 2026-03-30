// Package cloud provides a multi-cloud provider abstraction for launching
// and managing compute instances across AWS, GCP, and other providers.
// All provider implementations use CLI tools (aws, gcloud) via os/exec
// rather than cloud SDKs, keeping the dependency footprint minimal.
package cloud

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// InstanceState represents the lifecycle state of a cloud instance.
type InstanceState string

const (
	InstancePending     InstanceState = "pending"
	InstanceRunning     InstanceState = "running"
	InstanceStopped     InstanceState = "stopped"
	InstanceTerminated  InstanceState = "terminated"
	InstanceUnavailable InstanceState = "unavailable"
)

// InstanceSpec describes the desired configuration for a new instance.
type InstanceSpec struct {
	// Name is a human-readable label for the instance.
	Name string `json:"name"`

	// Region is the cloud region/zone to launch in (e.g., "us-east-1", "us-central1-a").
	Region string `json:"region"`

	// MachineType is the instance type (e.g., "t3.medium", "n1-standard-4").
	MachineType string `json:"machine_type"`

	// ImageID is the AMI, image family, or equivalent (e.g., "ami-0abcdef", "ubuntu-2404-lts").
	ImageID string `json:"image_id"`

	// DiskSizeGB is the root disk size in gigabytes.
	DiskSizeGB int `json:"disk_size_gb,omitempty"`

	// SpotOrPreemptible requests spot/preemptible pricing when true.
	SpotOrPreemptible bool `json:"spot_or_preemptible,omitempty"`

	// MaxSpotPrice is the maximum hourly price for spot instances (AWS only, USD).
	// Zero means use current spot price.
	MaxSpotPrice float64 `json:"max_spot_price,omitempty"`

	// Tags are key-value metadata applied to the instance.
	Tags map[string]string `json:"tags,omitempty"`

	// SSHKeyName is the name of the SSH key pair to attach.
	SSHKeyName string `json:"ssh_key_name,omitempty"`

	// UserData is a cloud-init or startup script passed to the instance.
	UserData string `json:"user_data,omitempty"`
}

// InstanceInfo describes a running or recently-terminated instance.
type InstanceInfo struct {
	// ID is the provider-specific instance identifier.
	ID string `json:"id"`

	// Name is the human-readable label.
	Name string `json:"name"`

	// Provider is the cloud provider name (e.g., "aws", "gcp").
	Provider string `json:"provider"`

	// Region where the instance is running.
	Region string `json:"region"`

	// MachineType is the instance type.
	MachineType string `json:"machine_type"`

	// State is the current lifecycle state.
	State InstanceState `json:"state"`

	// PublicIP is the external IP address, if assigned.
	PublicIP string `json:"public_ip,omitempty"`

	// PrivateIP is the internal IP address.
	PrivateIP string `json:"private_ip,omitempty"`

	// LaunchedAt is when the instance was started.
	LaunchedAt time.Time `json:"launched_at"`

	// HourlyCostUSD is the estimated hourly cost.
	HourlyCostUSD float64 `json:"hourly_cost_usd"`

	// SpotOrPreemptible indicates whether this is a spot/preemptible instance.
	SpotOrPreemptible bool `json:"spot_or_preemptible,omitempty"`
}

// CostSummary aggregates cost information across instances.
type CostSummary struct {
	// Provider name.
	Provider string `json:"provider"`

	// InstanceCount is the number of active instances.
	InstanceCount int `json:"instance_count"`

	// TotalHourlyCostUSD is the sum of hourly costs across all instances.
	TotalHourlyCostUSD float64 `json:"total_hourly_cost_usd"`

	// EstimatedMonthlyCostUSD projects the monthly spend (hourly * 730).
	EstimatedMonthlyCostUSD float64 `json:"estimated_monthly_cost_usd"`
}

// CloudProvider defines the interface that all cloud backends must implement.
type CloudProvider interface {
	// Name returns the provider identifier (e.g., "aws", "gcp").
	Name() string

	// ListInstances returns all instances visible to this provider,
	// optionally filtered by region. An empty region returns all regions.
	ListInstances(ctx context.Context, region string) ([]InstanceInfo, error)

	// LaunchInstance creates a new instance from the given spec.
	LaunchInstance(ctx context.Context, spec InstanceSpec) (*InstanceInfo, error)

	// TerminateInstance destroys the instance with the given ID.
	TerminateInstance(ctx context.Context, instanceID string, region string) error

	// GetCost returns the aggregated cost summary for active instances.
	GetCost(ctx context.Context) (*CostSummary, error)
}

// ProviderRegistry manages named cloud provider implementations.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]CloudProvider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]CloudProvider),
	}
}

// Register adds a provider to the registry. It returns an error if a
// provider with the same name is already registered.
func (r *ProviderRegistry) Register(p CloudProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := p.Name()
	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("cloud: provider %q already registered", name)
	}
	r.providers[name] = p
	return nil
}

// Get retrieves a provider by name. Returns an error if not found.
func (r *ProviderRegistry) Get(name string) (CloudProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("cloud: provider %q not registered", name)
	}
	return p, nil
}

// List returns the names of all registered providers.
func (r *ProviderRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// AggregatedCost returns cost summaries across all registered providers.
func (r *ProviderRegistry) AggregatedCost(ctx context.Context) ([]CostSummary, float64, error) {
	r.mu.RLock()
	providers := make([]CloudProvider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	r.mu.RUnlock()

	var (
		summaries []CostSummary
		total     float64
	)
	for _, p := range providers {
		cs, err := p.GetCost(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("cloud: cost from %s: %w", p.Name(), err)
		}
		summaries = append(summaries, *cs)
		total += cs.TotalHourlyCostUSD
	}
	return summaries, total, nil
}
