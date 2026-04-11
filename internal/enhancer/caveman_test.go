package enhancer

import (
	"strings"
	"testing"
)

func TestCompressCaveman_Lite(t *testing.T) {
	t.Parallel()
	input := "I just really wanted to say that it might be worth checking the database configuration."
	got, _ := CompressCaveman(input, "lite")
	
	// Lite should remove filler and hedging
	if strings.Contains(got, "just") || strings.Contains(got, "really") || strings.Contains(got, "it might be worth") {
		t.Errorf("CompressCaveman(lite) failed to remove filler/hedging: %q", got)
	}
}

func TestCompressCaveman_Full(t *testing.T) {
	t.Parallel()
	input := "The developer should create a new function in the module."
	got, _ := CompressCaveman(input, "full")
	
	// Full should remove articles
	if strings.Contains(got, "The ") || strings.Contains(got, " a ") || strings.Contains(got, " the ") {
		t.Errorf("CompressCaveman(full) failed to remove articles: %q", got)
	}
}

func TestCompressCaveman_Ultra(t *testing.T) {
	t.Parallel()
	input := "The function implementation failed because the database configuration was invalid."
	got, _ := CompressCaveman(input, "ultra")
	
	// Ultra should use arrows and abbreviations
	if !strings.Contains(got, "fn") || !strings.Contains(got, "impl") || !strings.Contains(got, "->") || !strings.Contains(got, "DB") || !strings.Contains(got, "cfg") {
		t.Errorf("CompressCaveman(ultra) failed to use arrows/abbreviations: %q", got)
	}
}

func TestEnhanceWithConfig_CavemanStage(t *testing.T) {
	t.Parallel()
	cfg := Config{
		CavemanLevel: "full",
	}
	input := "Please write a function that sorts a list of users by their name and age."
	result := EnhanceWithConfig(input, TaskTypeCode, cfg)

	// Verify caveman stage was run
	found := false
	for _, stage := range result.StagesRun {
		if stage == "caveman" {
			found = true
			break
		}
	}
	if !found {
		t.Error("caveman stage should be in StagesRun")
	}

	// Verify articles are removed in enhanced output
	if strings.Contains(result.Enhanced, " a ") || strings.Contains(result.Enhanced, " the ") {
		t.Errorf("Enhanced output still contains articles with CavemanLevel=full: %q", result.Enhanced)
	}
}

func TestCompressCaveman_Wenyan(t *testing.T) {
	t.Parallel()
	input := "The function is not valid because the database configuration must be correct."
	got, _ := CompressCaveman(input, "wenyan-full")

	// Wenyan-full should use classical characters
	if !strings.Contains(got, "為") || !strings.Contains(got, "不") || !strings.Contains(got, "必") {
		t.Errorf("CompressCaveman(wenyan-full) failed to use classical characters: %q", got)
	}
}
