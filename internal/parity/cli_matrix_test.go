package parity

import "testing"

func TestCLIParityCoverage_Summary(t *testing.T) {
	t.Parallel()

	got := CLIParityCoverage()
	if got.TotalSurfaces != 24 {
		t.Fatalf("TotalSurfaces = %d, want 24", got.TotalSurfaces)
	}
	if got.MCPNative != 18 {
		t.Fatalf("MCPNative = %d, want 18", got.MCPNative)
	}
	if got.SkillBacked != 2 {
		t.Fatalf("SkillBacked = %d, want 2", got.SkillBacked)
	}
	if got.Hybrid != 1 {
		t.Fatalf("Hybrid = %d, want 1", got.Hybrid)
	}
	if got.CommandOnlyByDesign != 3 {
		t.Fatalf("CommandOnlyByDesign = %d, want 3", got.CommandOnlyByDesign)
	}
	if got.CoveredSurfaces != 21 {
		t.Fatalf("CoveredSurfaces = %d, want 21", got.CoveredSurfaces)
	}
	if got.BespokeCoveragePct != 87.5 {
		t.Fatalf("BespokeCoveragePct = %.1f, want 87.5", got.BespokeCoveragePct)
	}
	if got.BusinessCoveragePct != 100.0 {
		t.Fatalf("BusinessCoveragePct = %.1f, want 100.0", got.BusinessCoveragePct)
	}
}

func TestCLIParityDocument_IncludesFirstbootHybrid(t *testing.T) {
	t.Parallel()

	doc := CLIParityDocument()
	entries, ok := doc["entries"].([]CLIParityEntry)
	if !ok {
		t.Fatalf("entries type = %T, want []CLIParityEntry", doc["entries"])
	}
	found := false
	for _, entry := range entries {
		if entry.Surface == "ralphglasses firstboot" {
			found = true
			if entry.Status != CLIParityHybrid {
				t.Fatalf("firstboot status = %q, want %q", entry.Status, CLIParityHybrid)
			}
			break
		}
	}
	if !found {
		t.Fatal("expected firstboot entry in parity document")
	}
}
