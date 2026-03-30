package cloud

import (
	"testing"
	"time"
)

func TestNewAWSProvider(t *testing.T) {
	tests := []struct {
		name          string
		defaultRegion string
		wantRegion    string
	}{
		{"explicit region", "eu-west-1", "eu-west-1"},
		{"empty defaults to us-east-1", "", "us-east-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAWSProvider(tt.defaultRegion)
			if p.DefaultRegion != tt.wantRegion {
				t.Errorf("DefaultRegion = %q, want %q", p.DefaultRegion, tt.wantRegion)
			}
		})
	}
}

func TestAWSProvider_Name(t *testing.T) {
	p := NewAWSProvider("")
	if got := p.Name(); got != "aws" {
		t.Errorf("Name() = %q, want %q", got, "aws")
	}
}

func TestAWSProvider_baseArgs(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		region  string
		want    []string
	}{
		{
			name:   "explicit region no profile",
			region: "us-west-2",
			want:   []string{"--region", "us-west-2", "--output", "json"},
		},
		{
			name:   "empty region uses default",
			region: "",
			want:   []string{"--region", "us-east-1", "--output", "json"},
		},
		{
			name:    "with profile",
			profile: "prod",
			region:  "eu-central-1",
			want:    []string{"--region", "eu-central-1", "--output", "json", "--profile", "prod"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAWSProvider("us-east-1")
			p.Profile = tt.profile
			got := p.baseArgs(tt.region)
			if len(got) != len(tt.want) {
				t.Fatalf("baseArgs() len = %d, want %d; got %v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("baseArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAWSProvider_toInstanceInfo(t *testing.T) {
	p := NewAWSProvider("us-east-1")

	tests := []struct {
		name   string
		inst   awsEC2Instance
		region string
		want   InstanceInfo
	}{
		{
			name: "running on-demand instance with tags",
			inst: awsEC2Instance{
				InstanceID:       "i-abc123",
				InstanceType:     "t3.medium",
				State:            struct{ Name string }{Name: "running"},
				PublicIPAddress:  "54.1.2.3",
				PrivateIPAddress: "10.0.0.5",
				LaunchTime:       "2026-03-01T12:00:00Z",
				Tags: []struct {
					Key   string `json:"Key"`
					Value string `json:"Value"`
				}{
					{Key: "Name", Value: "worker-1"},
					{Key: "env", Value: "prod"},
				},
			},
			region: "us-east-1",
			want: InstanceInfo{
				ID:            "i-abc123",
				Name:          "worker-1",
				Provider:      "aws",
				Region:        "us-east-1",
				MachineType:   "t3.medium",
				State:         InstanceRunning,
				PublicIP:      "54.1.2.3",
				PrivateIP:     "10.0.0.5",
				LaunchedAt:    time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
				HourlyCostUSD: 0.0416,
			},
		},
		{
			name: "spot instance",
			inst: awsEC2Instance{
				InstanceID:        "i-spot456",
				InstanceType:      "c5.xlarge",
				State:             struct{ Name string }{Name: "running"},
				InstanceLifecycle: "spot",
				LaunchTime:        "2026-03-15T08:30:00Z",
			},
			region: "us-west-2",
			want: InstanceInfo{
				ID:                "i-spot456",
				Provider:          "aws",
				Region:            "us-west-2",
				MachineType:       "c5.xlarge",
				State:             InstanceRunning,
				LaunchedAt:        time.Date(2026, 3, 15, 8, 30, 0, 0, time.UTC),
				HourlyCostUSD:     estimateAWSHourlyCost("c5.xlarge", true),
				SpotOrPreemptible: true,
			},
		},
		{
			name: "stopped instance no tags",
			inst: awsEC2Instance{
				InstanceID:   "i-stopped789",
				InstanceType: "m5.large",
				State:        struct{ Name string }{Name: "stopped"},
				LaunchTime:   "2026-02-20T00:00:00Z",
			},
			region: "eu-west-1",
			want: InstanceInfo{
				ID:            "i-stopped789",
				Provider:      "aws",
				Region:        "eu-west-1",
				MachineType:   "m5.large",
				State:         InstanceStopped,
				LaunchedAt:    time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC),
				HourlyCostUSD: 0.096,
			},
		},
		{
			name: "unknown instance type uses fallback price",
			inst: awsEC2Instance{
				InstanceID:   "i-custom",
				InstanceType: "z99.128xlarge",
				State:        struct{ Name string }{Name: "running"},
				LaunchTime:   "2026-01-01T00:00:00Z",
			},
			region: "ap-southeast-1",
			want: InstanceInfo{
				ID:            "i-custom",
				Provider:      "aws",
				Region:        "ap-southeast-1",
				MachineType:   "z99.128xlarge",
				State:         InstanceRunning,
				LaunchedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				HourlyCostUSD: 0.10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.toInstanceInfo(tt.inst, tt.region)
			if got.ID != tt.want.ID {
				t.Errorf("ID = %q, want %q", got.ID, tt.want.ID)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.Provider != tt.want.Provider {
				t.Errorf("Provider = %q, want %q", got.Provider, tt.want.Provider)
			}
			if got.Region != tt.want.Region {
				t.Errorf("Region = %q, want %q", got.Region, tt.want.Region)
			}
			if got.MachineType != tt.want.MachineType {
				t.Errorf("MachineType = %q, want %q", got.MachineType, tt.want.MachineType)
			}
			if got.State != tt.want.State {
				t.Errorf("State = %v, want %v", got.State, tt.want.State)
			}
			if got.PublicIP != tt.want.PublicIP {
				t.Errorf("PublicIP = %q, want %q", got.PublicIP, tt.want.PublicIP)
			}
			if got.PrivateIP != tt.want.PrivateIP {
				t.Errorf("PrivateIP = %q, want %q", got.PrivateIP, tt.want.PrivateIP)
			}
			if !got.LaunchedAt.Equal(tt.want.LaunchedAt) {
				t.Errorf("LaunchedAt = %v, want %v", got.LaunchedAt, tt.want.LaunchedAt)
			}
			if got.HourlyCostUSD != tt.want.HourlyCostUSD {
				t.Errorf("HourlyCostUSD = %f, want %f", got.HourlyCostUSD, tt.want.HourlyCostUSD)
			}
			if got.SpotOrPreemptible != tt.want.SpotOrPreemptible {
				t.Errorf("SpotOrPreemptible = %v, want %v", got.SpotOrPreemptible, tt.want.SpotOrPreemptible)
			}
		})
	}
}

func TestMapAWSState_Exhaustive(t *testing.T) {
	tests := []struct {
		input string
		want  InstanceState
	}{
		{"pending", InstancePending},
		{"running", InstanceRunning},
		{"stopped", InstanceStopped},
		{"stopping", InstanceStopped},
		{"terminated", InstanceTerminated},
		{"shutting-down", InstanceTerminated},
		{"", InstanceUnavailable},
		{"RUNNING", InstanceUnavailable}, // case sensitive
		{"unknown-state", InstanceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := mapAWSState(tt.input); got != tt.want {
				t.Errorf("mapAWSState(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestEstimateAWSHourlyCost_AllKnownTypes(t *testing.T) {
	knownTypes := []string{
		"t3.micro", "t3.small", "t3.medium", "t3.large", "t3.xlarge",
		"m5.large", "m5.xlarge", "m5.2xlarge",
		"c5.large", "c5.xlarge", "c5.2xlarge",
		"g4dn.xlarge", "g4dn.2xlarge",
		"p3.2xlarge",
	}

	for _, itype := range knownTypes {
		t.Run(itype, func(t *testing.T) {
			onDemand := estimateAWSHourlyCost(itype, false)
			if onDemand <= 0 {
				t.Errorf("on-demand price for %s = %f, want > 0", itype, onDemand)
			}

			spot := estimateAWSHourlyCost(itype, true)
			if spot >= onDemand {
				t.Errorf("spot price %f >= on-demand %f for %s", spot, onDemand, itype)
			}
			// Spot should be 30% of on-demand.
			expected := onDemand * 0.3
			if spot != expected {
				t.Errorf("spot price %f != expected %f (0.3 * on-demand) for %s", spot, expected, itype)
			}
		})
	}
}

func TestEstimateAWSHourlyCost_UnknownFallback(t *testing.T) {
	got := estimateAWSHourlyCost("imaginary.mega", false)
	if got != 0.10 {
		t.Errorf("unknown type = %f, want 0.10", got)
	}

	gotSpot := estimateAWSHourlyCost("imaginary.mega", true)
	if gotSpot != 0.03 {
		t.Errorf("unknown spot type = %f, want 0.03", gotSpot)
	}
}

func TestAWSProvider_toInstanceInfo_BadLaunchTime(t *testing.T) {
	p := NewAWSProvider("us-east-1")
	inst := awsEC2Instance{
		InstanceID:   "i-badtime",
		InstanceType: "t3.micro",
		State:        struct{ Name string }{Name: "running"},
		LaunchTime:   "not-a-valid-time",
	}
	got := p.toInstanceInfo(inst, "us-east-1")
	if !got.LaunchedAt.IsZero() {
		t.Errorf("LaunchedAt should be zero for unparseable time, got %v", got.LaunchedAt)
	}
}
