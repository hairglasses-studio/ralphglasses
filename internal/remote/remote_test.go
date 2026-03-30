package remote

import (
	"fmt"
	"sync"
	"testing"
)

func TestHostRegistry_RegisterAndGet(t *testing.T) {
	reg := NewHostRegistry()

	h := Host{Address: "10.0.0.1", User: "admin", Port: 22}
	if err := reg.Register("node1", h); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := reg.Get("node1")
	if !ok {
		t.Fatal("Get returned false for registered host")
	}
	if got.Address != "10.0.0.1" || got.User != "admin" || got.Port != 22 {
		t.Fatalf("Get returned unexpected host: %+v", got)
	}
}

func TestHostRegistry_RegisterDuplicate(t *testing.T) {
	reg := NewHostRegistry()
	h := Host{Address: "10.0.0.1", User: "admin", Port: 22}

	if err := reg.Register("node1", h); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := reg.Register("node1", h); err == nil {
		t.Fatal("expected error on duplicate register")
	}
}

func TestHostRegistry_GetMissing(t *testing.T) {
	reg := NewHostRegistry()
	_, ok := reg.Get("nonexistent")
	if ok {
		t.Fatal("Get should return false for missing host")
	}
}

func TestHostRegistry_List(t *testing.T) {
	reg := NewHostRegistry()
	_ = reg.Register("b", Host{Address: "10.0.0.2", User: "u", Port: 22})
	_ = reg.Register("a", Host{Address: "10.0.0.1", User: "u", Port: 22})

	hosts := reg.List()
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}
	if hosts[0].Address != "10.0.0.1" {
		t.Fatalf("expected sorted order, got %s first", hosts[0].Address)
	}
}

func TestHostRegistry_Remove(t *testing.T) {
	reg := NewHostRegistry()
	_ = reg.Register("node1", Host{Address: "10.0.0.1", User: "u", Port: 22})

	if err := reg.Remove("node1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := reg.Get("node1"); ok {
		t.Fatal("host should be removed")
	}
}

func TestHostRegistry_RemoveMissing(t *testing.T) {
	reg := NewHostRegistry()
	if err := reg.Remove("nonexistent"); err == nil {
		t.Fatal("expected error removing nonexistent host")
	}
}

func TestHostRegistry_FilterByLabel(t *testing.T) {
	reg := NewHostRegistry()
	_ = reg.Register("gpu1", Host{
		Address: "10.0.0.1", User: "u", Port: 22,
		Labels: map[string]string{"role": "gpu", "rack": "a"},
	})
	_ = reg.Register("gpu2", Host{
		Address: "10.0.0.2", User: "u", Port: 22,
		Labels: map[string]string{"role": "gpu", "rack": "b"},
	})
	_ = reg.Register("cpu1", Host{
		Address: "10.0.0.3", User: "u", Port: 22,
		Labels: map[string]string{"role": "cpu"},
	})

	gpus := reg.FilterByLabel("role", "gpu")
	if len(gpus) != 2 {
		t.Fatalf("expected 2 gpu hosts, got %d", len(gpus))
	}

	cpus := reg.FilterByLabel("role", "cpu")
	if len(cpus) != 1 {
		t.Fatalf("expected 1 cpu host, got %d", len(cpus))
	}

	none := reg.FilterByLabel("role", "storage")
	if len(none) != 0 {
		t.Fatalf("expected 0 hosts, got %d", len(none))
	}
}

func TestHostRegistry_FilterByLabel_NilLabels(t *testing.T) {
	reg := NewHostRegistry()
	_ = reg.Register("bare", Host{Address: "10.0.0.1", User: "u", Port: 22})

	results := reg.FilterByLabel("role", "gpu")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for host with nil labels, got %d", len(results))
	}
}

func TestHostRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewHostRegistry()
	var wg sync.WaitGroup

	// Concurrent writers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := fmt.Sprintf("node-%d", n)
			_ = reg.Register(name, Host{
				Address: fmt.Sprintf("10.0.0.%d", n),
				User:    "u",
				Port:    22,
			})
		}(i)
	}

	// Concurrent readers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = reg.List()
			reg.FilterByLabel("role", "gpu")
		}()
	}

	wg.Wait()

	hosts := reg.List()
	if len(hosts) != 50 {
		t.Fatalf("expected 50 hosts after concurrent writes, got %d", len(hosts))
	}
}
