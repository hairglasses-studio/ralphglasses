package cloud

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestInstanceState_Values(t *testing.T) {
	tests := []struct {
		state InstanceState
		want  string
	}{
		{InstancePending, "pending"},
		{InstanceRunning, "running"},
		{InstanceStopped, "stopped"},
		{InstanceTerminated, "terminated"},
		{InstanceUnavailable, "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if string(tt.state) != tt.want {
				t.Errorf("InstanceState = %q, want %q", string(tt.state), tt.want)
			}
		})
	}
}

func TestInstanceSpec_Defaults(t *testing.T) {
	spec := InstanceSpec{}
	if spec.Name != "" {
		t.Errorf("Name = %q, want empty", spec.Name)
	}
	if spec.DiskSizeGB != 0 {
		t.Errorf("DiskSizeGB = %d, want 0", spec.DiskSizeGB)
	}
	if spec.SpotOrPreemptible {
		t.Error("SpotOrPreemptible = true, want false")
	}
	if spec.MaxSpotPrice != 0 {
		t.Errorf("MaxSpotPrice = %f, want 0", spec.MaxSpotPrice)
	}
}

func TestCostSummary_Fields(t *testing.T) {
	cs := CostSummary{
		Provider:                "aws",
		InstanceCount:           3,
		TotalHourlyCostUSD:      1.50,
		EstimatedMonthlyCostUSD: 1095.0,
	}
	if cs.Provider != "aws" {
		t.Errorf("Provider = %q, want %q", cs.Provider, "aws")
	}
	if cs.InstanceCount != 3 {
		t.Errorf("InstanceCount = %d, want 3", cs.InstanceCount)
	}
	if cs.TotalHourlyCostUSD != 1.50 {
		t.Errorf("TotalHourlyCostUSD = %f, want 1.50", cs.TotalHourlyCostUSD)
	}
	if cs.EstimatedMonthlyCostUSD != 1095.0 {
		t.Errorf("EstimatedMonthlyCostUSD = %f, want 1095.0", cs.EstimatedMonthlyCostUSD)
	}
}

func TestNewRegistry_Empty(t *testing.T) {
	reg := NewRegistry()
	names := reg.List()
	if len(names) != 0 {
		t.Errorf("List() on empty registry returned %d names, want 0", len(names))
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewRegistry()

	// Register providers concurrently.
	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("provider-%d", idx)
			errs[idx] = reg.Register(newMockProvider(name))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Register provider-%d: %v", i, err)
		}
	}

	names := reg.List()
	if len(names) != 10 {
		t.Errorf("List() returned %d names, want 10", len(names))
	}

	// Concurrent Get calls.
	wg = sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("provider-%d", idx)
			p, err := reg.Get(name)
			if err != nil {
				t.Errorf("Get(%q): %v", name, err)
				return
			}
			if p.Name() != name {
				t.Errorf("Get(%q).Name() = %q", name, p.Name())
			}
		}(i)
	}
	wg.Wait()
}

func TestAggregatedCost_Empty(t *testing.T) {
	reg := NewRegistry()
	summaries, total, err := reg.AggregatedCost(context.Background())
	if err != nil {
		t.Fatalf("AggregatedCost: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("summaries len = %d, want 0", len(summaries))
	}
	if total != 0 {
		t.Errorf("total = %f, want 0", total)
	}
}

func TestAggregatedCost_WithTerminatedInstances(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry()

	mock := newMockProvider("test")
	reg.Register(mock)

	info1, _ := mock.LaunchInstance(ctx, InstanceSpec{Name: "a", MachineType: "t3.micro"})
	info2, _ := mock.LaunchInstance(ctx, InstanceSpec{Name: "b", MachineType: "t3.micro"})
	mock.TerminateInstance(ctx, info2.ID, "")

	summaries, total, err := reg.AggregatedCost(ctx)
	if err != nil {
		t.Fatalf("AggregatedCost: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries len = %d, want 1", len(summaries))
	}
	if summaries[0].InstanceCount != 1 {
		t.Errorf("InstanceCount = %d, want 1", summaries[0].InstanceCount)
	}
	if total != 0.50 {
		t.Errorf("total = %f, want 0.50", total)
	}
	_ = info1
}

// errorProvider is a mock that always returns errors from GetCost.
type errorProvider struct {
	name string
}

func (e *errorProvider) Name() string { return e.name }
func (e *errorProvider) ListInstances(_ context.Context, _ string) ([]InstanceInfo, error) {
	return nil, fmt.Errorf("list error")
}
func (e *errorProvider) LaunchInstance(_ context.Context, _ InstanceSpec) (*InstanceInfo, error) {
	return nil, fmt.Errorf("launch error")
}
func (e *errorProvider) TerminateInstance(_ context.Context, _, _ string) error {
	return fmt.Errorf("terminate error")
}
func (e *errorProvider) GetCost(_ context.Context) (*CostSummary, error) {
	return nil, fmt.Errorf("cost error")
}

func TestAggregatedCost_PropagatesError(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&errorProvider{name: "broken"})

	_, _, err := reg.AggregatedCost(context.Background())
	if err == nil {
		t.Fatal("expected error from AggregatedCost with broken provider")
	}
}

func TestRegistry_RegisterAndGetRoundTrip(t *testing.T) {
	reg := NewRegistry()
	providers := []string{"aws", "gcp", "azure"}

	for _, name := range providers {
		if err := reg.Register(newMockProvider(name)); err != nil {
			t.Fatalf("Register(%q): %v", name, err)
		}
	}

	for _, name := range providers {
		p, err := reg.Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		if p.Name() != name {
			t.Errorf("Get(%q).Name() = %q", name, p.Name())
		}
	}
}

func TestCloudProvider_InterfaceCompliance(t *testing.T) {
	// Verify both concrete providers satisfy the interface at compile time.
	var _ CloudProvider = (*AWSProvider)(nil)
	var _ CloudProvider = (*GCPProvider)(nil)
	var _ CloudProvider = (*mockProvider)(nil)
	var _ CloudProvider = (*errorProvider)(nil)
}
