package fleet

import (
	"testing"
)

func TestWorkerAgent_NodeID_Empty(t *testing.T) {
	w := NewWorkerAgent("http://coordinator", "localhost", 8080, "1.0", "", nil, nil)
	got := w.NodeID()
	if got != "" {
		t.Errorf("NodeID() = %q, want empty before registration", got)
	}
}


func TestWorkerAgent_DiscoverProviders_ReturnsAtLeastOne(t *testing.T) {
	w := NewWorkerAgent("http://coordinator", "localhost", 8080, "1.0", "", nil, nil)
	providers := w.discoverProviders()
	if len(providers) == 0 {
		t.Error("discoverProviders should return at least one provider")
	}
}
