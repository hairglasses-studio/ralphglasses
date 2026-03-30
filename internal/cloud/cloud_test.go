package cloud

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockProvider is a test double that implements CloudProvider in-memory.
type mockProvider struct {
	name      string
	instances map[string]*InstanceInfo
	nextID    int
	launchErr error
}

func newMockProvider(name string) *mockProvider {
	return &mockProvider{
		name:      name,
		instances: make(map[string]*InstanceInfo),
	}
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) ListInstances(_ context.Context, region string) ([]InstanceInfo, error) {
	var out []InstanceInfo
	for _, inst := range m.instances {
		if region == "" || inst.Region == region {
			out = append(out, *inst)
		}
	}
	return out, nil
}

func (m *mockProvider) LaunchInstance(_ context.Context, spec InstanceSpec) (*InstanceInfo, error) {
	if m.launchErr != nil {
		return nil, m.launchErr
	}
	m.nextID++
	id := fmt.Sprintf("%s-%d", m.name, m.nextID)
	region := spec.Region
	if region == "" {
		region = "mock-region-1"
	}
	info := &InstanceInfo{
		ID:                id,
		Name:              spec.Name,
		Provider:          m.name,
		Region:            region,
		MachineType:       spec.MachineType,
		State:             InstanceRunning,
		LaunchedAt:        time.Now(),
		HourlyCostUSD:     0.50,
		SpotOrPreemptible: spec.SpotOrPreemptible,
	}
	if info.SpotOrPreemptible {
		info.HourlyCostUSD = 0.15
	}
	m.instances[id] = info
	return info, nil
}

func (m *mockProvider) TerminateInstance(_ context.Context, instanceID, _ string) error {
	inst, ok := m.instances[instanceID]
	if !ok {
		return fmt.Errorf("instance %s not found", instanceID)
	}
	inst.State = InstanceTerminated
	return nil
}

func (m *mockProvider) GetCost(_ context.Context) (*CostSummary, error) {
	var totalHourly float64
	count := 0
	for _, inst := range m.instances {
		if inst.State == InstanceRunning {
			totalHourly += inst.HourlyCostUSD
			count++
		}
	}
	return &CostSummary{
		Provider:                m.name,
		InstanceCount:           count,
		TotalHourlyCostUSD:      totalHourly,
		EstimatedMonthlyCostUSD: totalHourly * 730,
	}, nil
}

// --- Registry tests ---

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	mock := newMockProvider("test-cloud")

	if err := reg.Register(mock); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := reg.Get("test-cloud")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name() != "test-cloud" {
		t.Errorf("Name = %q, want %q", got.Name(), "test-cloud")
	}
}

func TestRegistryDuplicateRegister(t *testing.T) {
	reg := NewRegistry()
	mock := newMockProvider("dup")

	if err := reg.Register(mock); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := reg.Register(mock); err == nil {
		t.Fatal("expected error on duplicate Register, got nil")
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent provider")
	}
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockProvider("alpha"))
	reg.Register(newMockProvider("beta"))

	names := reg.List()
	if len(names) != 2 {
		t.Fatalf("List returned %d names, want 2", len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["alpha"] || !nameSet["beta"] {
		t.Errorf("List = %v, want [alpha, beta]", names)
	}
}

// --- Instance lifecycle tests ---

func TestLaunchAndListInstances(t *testing.T) {
	ctx := context.Background()
	mock := newMockProvider("mock")

	info, err := mock.LaunchInstance(ctx, InstanceSpec{
		Name:        "worker-1",
		MachineType: "m5.large",
		Region:      "us-east-1",
	})
	if err != nil {
		t.Fatalf("LaunchInstance: %v", err)
	}
	if info.State != InstanceRunning {
		t.Errorf("State = %v, want %v", info.State, InstanceRunning)
	}
	if info.Name != "worker-1" {
		t.Errorf("Name = %q, want %q", info.Name, "worker-1")
	}

	instances, err := mock.ListInstances(ctx, "")
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("len(instances) = %d, want 1", len(instances))
	}
}

func TestListInstancesFilterByRegion(t *testing.T) {
	ctx := context.Background()
	mock := newMockProvider("mock")

	mock.LaunchInstance(ctx, InstanceSpec{Name: "east-1", Region: "us-east-1", MachineType: "t3.micro"})
	mock.LaunchInstance(ctx, InstanceSpec{Name: "west-1", Region: "us-west-2", MachineType: "t3.micro"})

	east, err := mock.ListInstances(ctx, "us-east-1")
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(east) != 1 {
		t.Fatalf("east instances = %d, want 1", len(east))
	}
	if east[0].Name != "east-1" {
		t.Errorf("Name = %q, want %q", east[0].Name, "east-1")
	}
}

func TestTerminateInstance(t *testing.T) {
	ctx := context.Background()
	mock := newMockProvider("mock")

	info, _ := mock.LaunchInstance(ctx, InstanceSpec{Name: "doomed", MachineType: "t3.micro"})

	if err := mock.TerminateInstance(ctx, info.ID, ""); err != nil {
		t.Fatalf("TerminateInstance: %v", err)
	}

	instances, _ := mock.ListInstances(ctx, "")
	for _, inst := range instances {
		if inst.ID == info.ID && inst.State != InstanceTerminated {
			t.Errorf("State = %v, want %v", inst.State, InstanceTerminated)
		}
	}
}

func TestTerminateNonexistent(t *testing.T) {
	ctx := context.Background()
	mock := newMockProvider("mock")

	err := mock.TerminateInstance(ctx, "does-not-exist", "")
	if err == nil {
		t.Fatal("expected error terminating nonexistent instance")
	}
}

func TestSpotInstance(t *testing.T) {
	ctx := context.Background()
	mock := newMockProvider("mock")

	info, err := mock.LaunchInstance(ctx, InstanceSpec{
		Name:              "spot-worker",
		MachineType:       "c5.xlarge",
		SpotOrPreemptible: true,
	})
	if err != nil {
		t.Fatalf("LaunchInstance: %v", err)
	}
	if !info.SpotOrPreemptible {
		t.Error("SpotOrPreemptible = false, want true")
	}
	if info.HourlyCostUSD >= 0.50 {
		t.Errorf("spot cost %f should be less than on-demand 0.50", info.HourlyCostUSD)
	}
}

// --- Cost aggregation tests ---

func TestGetCostSingleProvider(t *testing.T) {
	ctx := context.Background()
	mock := newMockProvider("mock")

	mock.LaunchInstance(ctx, InstanceSpec{Name: "a", MachineType: "t3.micro"})
	mock.LaunchInstance(ctx, InstanceSpec{Name: "b", MachineType: "t3.micro"})

	cost, err := mock.GetCost(ctx)
	if err != nil {
		t.Fatalf("GetCost: %v", err)
	}
	if cost.InstanceCount != 2 {
		t.Errorf("InstanceCount = %d, want 2", cost.InstanceCount)
	}
	if cost.TotalHourlyCostUSD != 1.0 {
		t.Errorf("TotalHourlyCostUSD = %f, want 1.0", cost.TotalHourlyCostUSD)
	}
	if cost.EstimatedMonthlyCostUSD != 730.0 {
		t.Errorf("EstimatedMonthlyCostUSD = %f, want 730.0", cost.EstimatedMonthlyCostUSD)
	}
}

func TestGetCostExcludesTerminated(t *testing.T) {
	ctx := context.Background()
	mock := newMockProvider("mock")

	info, _ := mock.LaunchInstance(ctx, InstanceSpec{Name: "alive", MachineType: "t3.micro"})
	dead, _ := mock.LaunchInstance(ctx, InstanceSpec{Name: "dead", MachineType: "t3.micro"})
	mock.TerminateInstance(ctx, dead.ID, "")

	cost, err := mock.GetCost(ctx)
	if err != nil {
		t.Fatalf("GetCost: %v", err)
	}
	if cost.InstanceCount != 1 {
		t.Errorf("InstanceCount = %d, want 1", cost.InstanceCount)
	}
	_ = info // keep linter happy
}

func TestAggregatedCostMultiProvider(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry()

	aws := newMockProvider("aws")
	gcp := newMockProvider("gcp")
	reg.Register(aws)
	reg.Register(gcp)

	aws.LaunchInstance(ctx, InstanceSpec{Name: "aws-1", MachineType: "t3.micro"})
	aws.LaunchInstance(ctx, InstanceSpec{Name: "aws-2", MachineType: "t3.micro"})
	gcp.LaunchInstance(ctx, InstanceSpec{Name: "gcp-1", MachineType: "n1-standard-1"})

	summaries, total, err := reg.AggregatedCost(ctx)
	if err != nil {
		t.Fatalf("AggregatedCost: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("len(summaries) = %d, want 2", len(summaries))
	}
	// 2 AWS at $0.50/hr + 1 GCP at $0.50/hr = $1.50/hr
	if total != 1.5 {
		t.Errorf("total hourly = %f, want 1.5", total)
	}
}

// --- Interface compliance ---

func TestAWSProviderImplementsInterface(t *testing.T) {
	var _ CloudProvider = (*AWSProvider)(nil)
}

func TestGCPProviderImplementsInterface(t *testing.T) {
	var _ CloudProvider = (*GCPProvider)(nil)
}

// --- Price estimation tests ---

func TestEstimateAWSHourlyCost(t *testing.T) {
	onDemand := estimateAWSHourlyCost("t3.medium", false)
	if onDemand != 0.0416 {
		t.Errorf("t3.medium on-demand = %f, want 0.0416", onDemand)
	}

	spot := estimateAWSHourlyCost("t3.medium", true)
	if spot >= onDemand {
		t.Errorf("spot price %f should be less than on-demand %f", spot, onDemand)
	}

	unknown := estimateAWSHourlyCost("x99.metal", false)
	if unknown != 0.10 {
		t.Errorf("unknown type = %f, want fallback 0.10", unknown)
	}
}

func TestEstimateGCPHourlyCost(t *testing.T) {
	onDemand := estimateGCPHourlyCost("n1-standard-4", false)
	if onDemand != 0.19 {
		t.Errorf("n1-standard-4 on-demand = %f, want 0.19", onDemand)
	}

	preemptible := estimateGCPHourlyCost("n1-standard-4", true)
	if preemptible >= onDemand {
		t.Errorf("preemptible price %f should be less than on-demand %f", preemptible, onDemand)
	}
}

// --- State mapping tests ---

func TestMapAWSState(t *testing.T) {
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
		{"weird", InstanceUnavailable},
	}
	for _, tt := range tests {
		if got := mapAWSState(tt.input); got != tt.want {
			t.Errorf("mapAWSState(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestMapGCPState(t *testing.T) {
	tests := []struct {
		input string
		want  InstanceState
	}{
		{"PROVISIONING", InstancePending},
		{"STAGING", InstancePending},
		{"RUNNING", InstanceRunning},
		{"STOPPED", InstanceStopped},
		{"SUSPENDED", InstanceStopped},
		{"TERMINATED", InstanceTerminated},
		{"UNKNOWN", InstanceUnavailable},
	}
	for _, tt := range tests {
		if got := mapGCPState(tt.input); got != tt.want {
			t.Errorf("mapGCPState(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestLaunchError(t *testing.T) {
	ctx := context.Background()
	mock := newMockProvider("mock")
	mock.launchErr = fmt.Errorf("quota exceeded")

	_, err := mock.LaunchInstance(ctx, InstanceSpec{Name: "fail"})
	if err == nil {
		t.Fatal("expected launch error")
	}
	if err.Error() != "quota exceeded" {
		t.Errorf("error = %q, want %q", err.Error(), "quota exceeded")
	}
}
