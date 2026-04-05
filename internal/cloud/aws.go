package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os/exec"
	"strings"
	"time"
)

// AWSProvider implements CloudProvider using the AWS CLI (aws ec2).
type AWSProvider struct {
	// DefaultRegion is used when no region is specified.
	DefaultRegion string

	// Profile is the AWS CLI profile name. Empty uses the default profile.
	Profile string
}

// NewAWSProvider creates an AWSProvider with the given default region.
func NewAWSProvider(defaultRegion string) *AWSProvider {
	if defaultRegion == "" {
		defaultRegion = "us-east-1"
	}
	return &AWSProvider{DefaultRegion: defaultRegion}
}

// Name returns "aws".
func (a *AWSProvider) Name() string { return "aws" }

// baseArgs returns common CLI flags for region and profile.
func (a *AWSProvider) baseArgs(region string) []string {
	if region == "" {
		region = a.DefaultRegion
	}
	args := []string{"--region", region, "--output", "json"}
	if a.Profile != "" {
		args = append(args, "--profile", a.Profile)
	}
	return args
}

// awsEC2Reservation is the JSON shape returned by aws ec2 describe-instances.
type awsEC2Reservation struct {
	Reservations []struct {
		Instances []awsEC2Instance `json:"Instances"`
	} `json:"Reservations"`
}

type awsEC2Instance struct {
	InstanceID        string                            `json:"InstanceId"`
	InstanceType      string                            `json:"InstanceType"`
	State             struct{ Name string }             `json:"State"`
	PublicIPAddress   string                            `json:"PublicIpAddress"`
	PrivateIPAddress  string                            `json:"PrivateIpAddress"`
	LaunchTime        string                            `json:"LaunchTime"`
	Placement         struct{ AvailabilityZone string } `json:"Placement"`
	InstanceLifecycle string                            `json:"InstanceLifecycle"` // "spot" or empty
	Tags              []struct {
		Key   string `json:"Key"`
		Value string `json:"Value"`
	} `json:"Tags"`
}

// ListInstances lists EC2 instances. If region is empty, uses the default region.
func (a *AWSProvider) ListInstances(ctx context.Context, region string) ([]InstanceInfo, error) {
	if region == "" {
		region = a.DefaultRegion
	}

	args := append([]string{"ec2", "describe-instances",
		"--filters", "Name=instance-state-name,Values=pending,running,stopping,stopped",
	}, a.baseArgs(region)...)

	out, err := a.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("aws list instances: %w", err)
	}

	var resp awsEC2Reservation
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("aws parse instances: %w", err)
	}

	var instances []InstanceInfo
	for _, r := range resp.Reservations {
		for _, inst := range r.Instances {
			info := a.toInstanceInfo(inst, region)
			instances = append(instances, info)
		}
	}
	return instances, nil
}

// LaunchInstance creates an EC2 instance (or spot instance request).
func (a *AWSProvider) LaunchInstance(ctx context.Context, spec InstanceSpec) (*InstanceInfo, error) {
	region := spec.Region
	if region == "" {
		region = a.DefaultRegion
	}

	args := []string{"ec2", "run-instances",
		"--image-id", spec.ImageID,
		"--instance-type", spec.MachineType,
		"--count", "1",
	}

	if spec.SSHKeyName != "" {
		args = append(args, "--key-name", spec.SSHKeyName)
	}

	if spec.DiskSizeGB > 0 {
		bdm := fmt.Sprintf(`[{"DeviceName":"/dev/sda1","Ebs":{"VolumeSize":%d,"VolumeType":"gp3"}}]`, spec.DiskSizeGB)
		args = append(args, "--block-device-mappings", bdm)
	}

	if spec.SpotOrPreemptible {
		marketOpts := `{"MarketType":"spot"}`
		if spec.MaxSpotPrice > 0 {
			marketOpts = fmt.Sprintf(`{"MarketType":"spot","SpotOptions":{"MaxPrice":"%0.4f"}}`, spec.MaxSpotPrice)
		}
		args = append(args, "--instance-market-options", marketOpts)
	}

	if spec.UserData != "" {
		args = append(args, "--user-data", spec.UserData)
	}

	// Build tag specifications.
	tags := make(map[string]string)
	maps.Copy(tags, spec.Tags)
	if spec.Name != "" {
		tags["Name"] = spec.Name
	}
	if len(tags) > 0 {
		var tagSpecs strings.Builder
		tagSpecs.WriteString("ResourceType=instance")
		for k, v := range tags {
			tagSpecs.WriteString(fmt.Sprintf(",Key=%s,Value=%s", k, v))
		}
		args = append(args, "--tag-specifications", tagSpecs.String())
	}

	args = append(args, a.baseArgs(region)...)

	out, err := a.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("aws launch instance: %w", err)
	}

	var resp struct {
		Instances []awsEC2Instance `json:"Instances"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("aws parse launch response: %w", err)
	}
	if len(resp.Instances) == 0 {
		return nil, fmt.Errorf("aws launch: no instances in response")
	}

	info := a.toInstanceInfo(resp.Instances[0], region)
	return &info, nil
}

// TerminateInstance terminates an EC2 instance by ID.
func (a *AWSProvider) TerminateInstance(ctx context.Context, instanceID, region string) error {
	if region == "" {
		region = a.DefaultRegion
	}

	args := append([]string{"ec2", "terminate-instances",
		"--instance-ids", instanceID,
	}, a.baseArgs(region)...)

	if _, err := a.run(ctx, args...); err != nil {
		return fmt.Errorf("aws terminate %s: %w", instanceID, err)
	}
	return nil
}

// GetCost returns an estimated cost summary based on running instances.
// This is a local estimate using known instance pricing, not the AWS Cost Explorer API.
func (a *AWSProvider) GetCost(ctx context.Context) (*CostSummary, error) {
	instances, err := a.ListInstances(ctx, "")
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
		Provider:                "aws",
		InstanceCount:           count,
		TotalHourlyCostUSD:      totalHourly,
		EstimatedMonthlyCostUSD: totalHourly * 730,
	}, nil
}

// toInstanceInfo converts an AWS EC2 JSON instance to InstanceInfo.
func (a *AWSProvider) toInstanceInfo(inst awsEC2Instance, region string) InstanceInfo {
	state := mapAWSState(inst.State.Name)
	launched, _ := time.Parse(time.RFC3339, inst.LaunchTime)

	name := ""
	for _, tag := range inst.Tags {
		if tag.Key == "Name" {
			name = tag.Value
			break
		}
	}

	return InstanceInfo{
		ID:                inst.InstanceID,
		Name:              name,
		Provider:          "aws",
		Region:            region,
		MachineType:       inst.InstanceType,
		State:             state,
		PublicIP:          inst.PublicIPAddress,
		PrivateIP:         inst.PrivateIPAddress,
		LaunchedAt:        launched,
		HourlyCostUSD:     estimateAWSHourlyCost(inst.InstanceType, inst.InstanceLifecycle == "spot"),
		SpotOrPreemptible: inst.InstanceLifecycle == "spot",
	}
}

// run executes the AWS CLI and returns stdout.
func (a *AWSProvider) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "aws", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

// mapAWSState converts AWS state names to InstanceState.
func mapAWSState(s string) InstanceState {
	switch s {
	case "pending":
		return InstancePending
	case "running":
		return InstanceRunning
	case "stopped", "stopping":
		return InstanceStopped
	case "terminated", "shutting-down":
		return InstanceTerminated
	default:
		return InstanceUnavailable
	}
}

// estimateAWSHourlyCost returns a rough USD/hr estimate for common instance types.
// This avoids calling the Pricing API; values are approximate on-demand rates.
func estimateAWSHourlyCost(instanceType string, isSpot bool) float64 {
	// Approximate on-demand prices (us-east-1, USD/hr).
	prices := map[string]float64{
		"t3.micro":     0.0104,
		"t3.small":     0.0208,
		"t3.medium":    0.0416,
		"t3.large":     0.0832,
		"t3.xlarge":    0.1664,
		"m5.large":     0.096,
		"m5.xlarge":    0.192,
		"m5.2xlarge":   0.384,
		"c5.large":     0.085,
		"c5.xlarge":    0.17,
		"c5.2xlarge":   0.34,
		"g4dn.xlarge":  0.526,
		"g4dn.2xlarge": 0.752,
		"p3.2xlarge":   3.06,
	}

	price, ok := prices[instanceType]
	if !ok {
		price = 0.10 // fallback estimate
	}
	if isSpot {
		price *= 0.3 // spot is roughly 60-70% cheaper
	}
	return price
}
