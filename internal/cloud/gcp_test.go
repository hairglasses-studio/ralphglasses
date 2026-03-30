package cloud

import (
	"testing"
	"time"
)

func TestNewGCPProvider(t *testing.T) {
	tests := []struct {
		name        string
		project     string
		defaultZone string
		wantZone    string
	}{
		{"explicit zone", "my-project", "europe-west1-b", "europe-west1-b"},
		{"empty defaults to us-central1-a", "my-project", "", "us-central1-a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewGCPProvider(tt.project, tt.defaultZone)
			if p.DefaultZone != tt.wantZone {
				t.Errorf("DefaultZone = %q, want %q", p.DefaultZone, tt.wantZone)
			}
			if p.Project != tt.project {
				t.Errorf("Project = %q, want %q", p.Project, tt.project)
			}
		})
	}
}

func TestGCPProvider_Name(t *testing.T) {
	p := NewGCPProvider("proj", "")
	if got := p.Name(); got != "gcp" {
		t.Errorf("Name() = %q, want %q", got, "gcp")
	}
}

func TestGCPProvider_toInstanceInfo(t *testing.T) {
	p := NewGCPProvider("my-project", "us-central1-a")

	tests := []struct {
		name string
		inst gcloudInstance
		want InstanceInfo
	}{
		{
			name: "running instance with full resource URLs",
			inst: gcloudInstance{
				Name:              "worker-1",
				ID:                "1234567890",
				Zone:              "projects/my-project/zones/us-central1-a",
				MachineType:       "projects/my-project/zones/us-central1-a/machineTypes/n1-standard-4",
				Status:            "RUNNING",
				CreationTimestamp: "2026-03-01T12:00:00Z",
				NetworkInterfaces: []struct {
					NetworkIP     string `json:"networkIP"`
					AccessConfigs []struct {
						NatIP string `json:"natIP"`
					} `json:"accessConfigs"`
				}{
					{
						NetworkIP: "10.128.0.5",
						AccessConfigs: []struct {
							NatIP string `json:"natIP"`
						}{
							{NatIP: "35.192.1.2"},
						},
					},
				},
			},
			want: InstanceInfo{
				ID:            "1234567890",
				Name:          "worker-1",
				Provider:      "gcp",
				Region:        "us-central1-a",
				MachineType:   "n1-standard-4",
				State:         InstanceRunning,
				PublicIP:      "35.192.1.2",
				PrivateIP:     "10.128.0.5",
				LaunchedAt:    time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
				HourlyCostUSD: 0.19,
			},
		},
		{
			name: "preemptible instance",
			inst: gcloudInstance{
				Name:              "preempt-1",
				ID:                "9876543210",
				Zone:              "us-west1-b",
				MachineType:       "e2-medium",
				Status:            "RUNNING",
				CreationTimestamp: "2026-03-15T08:30:00Z",
				Scheduling: struct {
					Preemptible bool `json:"preemptible"`
				}{Preemptible: true},
			},
			want: InstanceInfo{
				ID:                "9876543210",
				Name:              "preempt-1",
				Provider:          "gcp",
				Region:            "us-west1-b",
				MachineType:       "e2-medium",
				State:             InstanceRunning,
				LaunchedAt:        time.Date(2026, 3, 15, 8, 30, 0, 0, time.UTC),
				HourlyCostUSD:     estimateGCPHourlyCost("e2-medium", true),
				SpotOrPreemptible: true,
			},
		},
		{
			name: "stopped instance no network interfaces",
			inst: gcloudInstance{
				Name:              "stopped-vm",
				ID:                "1111111111",
				Zone:              "europe-west1-b",
				MachineType:       "n2-standard-2",
				Status:            "STOPPED",
				CreationTimestamp: "2026-02-01T00:00:00Z",
			},
			want: InstanceInfo{
				ID:            "1111111111",
				Name:          "stopped-vm",
				Provider:      "gcp",
				Region:        "europe-west1-b",
				MachineType:   "n2-standard-2",
				State:         InstanceStopped,
				LaunchedAt:    time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
				HourlyCostUSD: 0.0971,
			},
		},
		{
			name: "provisioning instance",
			inst: gcloudInstance{
				Name:              "new-vm",
				ID:                "2222222222",
				Zone:              "us-east1-c",
				MachineType:       "e2-micro",
				Status:            "PROVISIONING",
				CreationTimestamp: "2026-03-30T10:00:00Z",
			},
			want: InstanceInfo{
				ID:            "2222222222",
				Name:          "new-vm",
				Provider:      "gcp",
				Region:        "us-east1-c",
				MachineType:   "e2-micro",
				State:         InstancePending,
				LaunchedAt:    time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC),
				HourlyCostUSD: 0.0084,
			},
		},
		{
			name: "network interface without access configs (no public IP)",
			inst: gcloudInstance{
				Name:        "private-vm",
				ID:          "3333333333",
				Zone:        "us-central1-a",
				MachineType: "e2-small",
				Status:      "RUNNING",
				NetworkInterfaces: []struct {
					NetworkIP     string `json:"networkIP"`
					AccessConfigs []struct {
						NatIP string `json:"natIP"`
					} `json:"accessConfigs"`
				}{
					{NetworkIP: "10.0.0.99"},
				},
			},
			want: InstanceInfo{
				ID:            "3333333333",
				Name:          "private-vm",
				Provider:      "gcp",
				Region:        "us-central1-a",
				MachineType:   "e2-small",
				State:         InstanceRunning,
				PrivateIP:     "10.0.0.99",
				HourlyCostUSD: 0.0168,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.toInstanceInfo(tt.inst)
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

func TestMapGCPState_Exhaustive(t *testing.T) {
	tests := []struct {
		input string
		want  InstanceState
	}{
		{"PROVISIONING", InstancePending},
		{"STAGING", InstancePending},
		{"RUNNING", InstanceRunning},
		{"STOPPED", InstanceStopped},
		{"SUSPENDED", InstanceStopped},
		{"SUSPENDING", InstanceStopped},
		{"STOPPING", InstanceStopped},
		{"TERMINATED", InstanceTerminated},
		{"", InstanceUnavailable},
		{"running", InstanceUnavailable}, // case sensitive
		{"REPAIRING", InstanceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := mapGCPState(tt.input); got != tt.want {
				t.Errorf("mapGCPState(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestEstimateGCPHourlyCost_AllKnownTypes(t *testing.T) {
	knownTypes := []string{
		"e2-micro", "e2-small", "e2-medium",
		"n1-standard-1", "n1-standard-2", "n1-standard-4", "n1-standard-8",
		"n2-standard-2", "n2-standard-4", "n2-standard-8",
		"c2-standard-4", "c2-standard-8",
		"a2-highgpu-1g", "g2-standard-4",
	}

	for _, mtype := range knownTypes {
		t.Run(mtype, func(t *testing.T) {
			onDemand := estimateGCPHourlyCost(mtype, false)
			if onDemand <= 0 {
				t.Errorf("on-demand price for %s = %f, want > 0", mtype, onDemand)
			}

			preemptible := estimateGCPHourlyCost(mtype, true)
			if preemptible >= onDemand {
				t.Errorf("preemptible price %f >= on-demand %f for %s", preemptible, onDemand, mtype)
			}
			expected := onDemand * 0.3
			if preemptible != expected {
				t.Errorf("preemptible price %f != expected %f for %s", preemptible, expected, mtype)
			}
		})
	}
}

func TestEstimateGCPHourlyCost_UnknownFallback(t *testing.T) {
	got := estimateGCPHourlyCost("custom-96-624", false)
	if got != 0.10 {
		t.Errorf("unknown type = %f, want 0.10", got)
	}

	gotPreempt := estimateGCPHourlyCost("custom-96-624", true)
	if gotPreempt != 0.03 {
		t.Errorf("unknown preemptible type = %f, want 0.03", gotPreempt)
	}
}

func TestGCPProvider_toInstanceInfo_BadTimestamp(t *testing.T) {
	p := NewGCPProvider("proj", "us-central1-a")
	inst := gcloudInstance{
		Name:              "bad-time-vm",
		ID:                "999",
		Zone:              "us-central1-a",
		MachineType:       "e2-micro",
		Status:            "RUNNING",
		CreationTimestamp: "invalid-timestamp",
	}
	got := p.toInstanceInfo(inst)
	if !got.LaunchedAt.IsZero() {
		t.Errorf("LaunchedAt should be zero for unparseable time, got %v", got.LaunchedAt)
	}
}

func TestGCPProvider_toInstanceInfo_ShortZoneAndMachineType(t *testing.T) {
	p := NewGCPProvider("proj", "us-central1-a")
	// When zone/machineType are already short (no "/" prefix).
	inst := gcloudInstance{
		Name:        "short-paths",
		ID:          "444",
		Zone:        "us-east1-b",
		MachineType: "n1-standard-1",
		Status:      "RUNNING",
	}
	got := p.toInstanceInfo(inst)
	if got.Region != "us-east1-b" {
		t.Errorf("Region = %q, want %q", got.Region, "us-east1-b")
	}
	if got.MachineType != "n1-standard-1" {
		t.Errorf("MachineType = %q, want %q", got.MachineType, "n1-standard-1")
	}
}
