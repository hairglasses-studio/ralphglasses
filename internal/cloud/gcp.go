package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GCPProvider implements CloudProvider using the gcloud CLI.
type GCPProvider struct {
	// Project is the GCP project ID.
	Project string

	// DefaultZone is used when no region/zone is specified (e.g., "us-central1-a").
	DefaultZone string
}

// NewGCPProvider creates a GCPProvider with the given project and default zone.
func NewGCPProvider(project, defaultZone string) *GCPProvider {
	if defaultZone == "" {
		defaultZone = "us-central1-a"
	}
	return &GCPProvider{
		Project:     project,
		DefaultZone: defaultZone,
	}
}

// Name returns "gcp".
func (g *GCPProvider) Name() string { return "gcp" }

// gcloudInstance is the JSON shape returned by gcloud compute instances list/describe.
type gcloudInstance struct {
	Name              string `json:"name"`
	ID                string `json:"id"`
	Zone              string `json:"zone"`
	MachineType       string `json:"machineType"`
	Status            string `json:"status"`
	CreationTimestamp string `json:"creationTimestamp"`
	Scheduling        struct {
		Preemptible bool `json:"preemptible"`
	} `json:"scheduling"`
	NetworkInterfaces []struct {
		NetworkIP    string `json:"networkIP"`
		AccessConfigs []struct {
			NatIP string `json:"natIP"`
		} `json:"accessConfigs"`
	} `json:"networkInterfaces"`
}

// ListInstances lists GCE instances. If region is empty, uses the default zone.
func (g *GCPProvider) ListInstances(ctx context.Context, region string) ([]InstanceInfo, error) {
	zone := region
	if zone == "" {
		zone = g.DefaultZone
	}

	args := []string{"compute", "instances", "list",
		"--format=json",
	}
	if g.Project != "" {
		args = append(args, "--project", g.Project)
	}
	// If a specific zone is requested, filter by it.
	if zone != "" {
		args = append(args, "--zones", zone)
	}

	out, err := g.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("gcp list instances: %w", err)
	}

	var gcpInstances []gcloudInstance
	if err := json.Unmarshal(out, &gcpInstances); err != nil {
		return nil, fmt.Errorf("gcp parse instances: %w", err)
	}

	instances := make([]InstanceInfo, 0, len(gcpInstances))
	for _, gi := range gcpInstances {
		instances = append(instances, g.toInstanceInfo(gi))
	}
	return instances, nil
}

// LaunchInstance creates a GCE instance (or preemptible VM).
func (g *GCPProvider) LaunchInstance(ctx context.Context, spec InstanceSpec) (*InstanceInfo, error) {
	zone := spec.Region
	if zone == "" {
		zone = g.DefaultZone
	}

	args := []string{"compute", "instances", "create", spec.Name,
		"--zone", zone,
		"--machine-type", spec.MachineType,
		"--image", spec.ImageID,
		"--format=json",
	}

	if g.Project != "" {
		args = append(args, "--project", g.Project)
	}

	if spec.DiskSizeGB > 0 {
		args = append(args, "--boot-disk-size", fmt.Sprintf("%dGB", spec.DiskSizeGB))
	}

	if spec.SpotOrPreemptible {
		args = append(args, "--preemptible")
	}

	if spec.UserData != "" {
		args = append(args, "--metadata", fmt.Sprintf("startup-script=%s", spec.UserData))
	}

	// Apply labels from tags.
	if len(spec.Tags) > 0 {
		var labels []string
		for k, v := range spec.Tags {
			// GCP labels must be lowercase.
			labels = append(labels, fmt.Sprintf("%s=%s", strings.ToLower(k), strings.ToLower(v)))
		}
		args = append(args, "--labels", strings.Join(labels, ","))
	}

	out, err := g.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("gcp launch instance: %w", err)
	}

	var created []gcloudInstance
	if err := json.Unmarshal(out, &created); err != nil {
		return nil, fmt.Errorf("gcp parse launch response: %w", err)
	}
	if len(created) == 0 {
		return nil, fmt.Errorf("gcp launch: no instances in response")
	}

	info := g.toInstanceInfo(created[0])
	return &info, nil
}

// TerminateInstance deletes a GCE instance by name.
func (g *GCPProvider) TerminateInstance(ctx context.Context, instanceName, zone string) error {
	if zone == "" {
		zone = g.DefaultZone
	}

	args := []string{"compute", "instances", "delete", instanceName,
		"--zone", zone,
		"--quiet",
	}
	if g.Project != "" {
		args = append(args, "--project", g.Project)
	}

	if _, err := g.run(ctx, args...); err != nil {
		return fmt.Errorf("gcp terminate %s: %w", instanceName, err)
	}
	return nil
}

// GetCost returns an estimated cost summary based on running instances.
func (g *GCPProvider) GetCost(ctx context.Context) (*CostSummary, error) {
	instances, err := g.ListInstances(ctx, "")
	if err != nil {
		return nil, err
	}

	var totalHourly float64
	count := 0
	for _, inst := range instances {
		if inst.State == InstanceRunning || inst.State == InstancePending {
			totalHourly += inst.HourlyCostUSD
			count++
		}
	}

	return &CostSummary{
		Provider:                "gcp",
		InstanceCount:           count,
		TotalHourlyCostUSD:      totalHourly,
		EstimatedMonthlyCostUSD: totalHourly * 730,
	}, nil
}

// toInstanceInfo converts a gcloud JSON instance to InstanceInfo.
func (g *GCPProvider) toInstanceInfo(gi gcloudInstance) InstanceInfo {
	state := mapGCPState(gi.Status)
	launched, _ := time.Parse(time.RFC3339, gi.CreationTimestamp)

	// Extract the short machine type from the full resource URL.
	machineType := gi.MachineType
	if idx := strings.LastIndex(machineType, "/"); idx >= 0 {
		machineType = machineType[idx+1:]
	}

	// Extract zone from full resource URL.
	zone := gi.Zone
	if idx := strings.LastIndex(zone, "/"); idx >= 0 {
		zone = zone[idx+1:]
	}

	var publicIP, privateIP string
	if len(gi.NetworkInterfaces) > 0 {
		privateIP = gi.NetworkInterfaces[0].NetworkIP
		if len(gi.NetworkInterfaces[0].AccessConfigs) > 0 {
			publicIP = gi.NetworkInterfaces[0].AccessConfigs[0].NatIP
		}
	}

	return InstanceInfo{
		ID:                gi.ID,
		Name:              gi.Name,
		Provider:          "gcp",
		Region:            zone,
		MachineType:       machineType,
		State:             state,
		PublicIP:          publicIP,
		PrivateIP:         privateIP,
		LaunchedAt:        launched,
		HourlyCostUSD:     estimateGCPHourlyCost(machineType, gi.Scheduling.Preemptible),
		SpotOrPreemptible: gi.Scheduling.Preemptible,
	}
}

// run executes the gcloud CLI and returns stdout.
func (g *GCPProvider) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

// mapGCPState converts GCE status strings to InstanceState.
func mapGCPState(s string) InstanceState {
	switch s {
	case "PROVISIONING", "STAGING":
		return InstancePending
	case "RUNNING":
		return InstanceRunning
	case "STOPPED", "SUSPENDED", "SUSPENDING", "STOPPING":
		return InstanceStopped
	case "TERMINATED":
		return InstanceTerminated
	default:
		return InstanceUnavailable
	}
}

// estimateGCPHourlyCost returns a rough USD/hr estimate for common GCE machine types.
func estimateGCPHourlyCost(machineType string, preemptible bool) float64 {
	prices := map[string]float64{
		"e2-micro":       0.0084,
		"e2-small":       0.0168,
		"e2-medium":      0.0336,
		"n1-standard-1":  0.0475,
		"n1-standard-2":  0.095,
		"n1-standard-4":  0.19,
		"n1-standard-8":  0.38,
		"n2-standard-2":  0.0971,
		"n2-standard-4":  0.1942,
		"n2-standard-8":  0.3884,
		"c2-standard-4":  0.2088,
		"c2-standard-8":  0.4176,
		"a2-highgpu-1g":  3.6731,
		"g2-standard-4":  0.7688,
	}

	price, ok := prices[machineType]
	if !ok {
		price = 0.10 // fallback estimate
	}
	if preemptible {
		price *= 0.3 // preemptible is roughly 60-80% cheaper
	}
	return price
}
