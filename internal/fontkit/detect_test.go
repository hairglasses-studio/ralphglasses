package fontkit

import (
	"context"
	"testing"
)

func TestDetect(t *testing.T) {
	status, err := Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Total should be all families
	total := len(status.Installed) + len(status.NotInstalled)
	if total != len(AllFamilies()) {
		t.Errorf("Installed + NotInstalled = %d, want %d", total, len(AllFamilies()))
	}
	// BestFont should always be set (at minimum Menlo)
	if status.BestFont == "" {
		t.Error("BestFont should not be empty")
	}
}

func TestBestAvailable(t *testing.T) {
	// With no fonts installed, should fall back to Menlo
	if got := bestAvailable(nil); got != "Menlo" {
		t.Errorf("bestAvailable(nil) = %q, want Menlo", got)
	}

	// With MonaspiceNe installed, should pick it
	got := bestAvailable([]FontFamily{MonaspiceNe})
	if got != "MonaspiceNeNFM-Regular" {
		t.Errorf("bestAvailable([MonaspiceNe]) = %q, want MonaspiceNeNFM-Regular", got)
	}
}
