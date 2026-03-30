package mcpserver

import (
	"strings"
	"testing"
)

func TestInferCategory_Ralphglasses(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"ralphglasses_session_launch", "session"},
		{"ralphglasses_fleet_status", "fleet"},
		{"ralphglasses_scan", "core"},
		{"my_tool", "my"},
		{"notool", "general"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := inferCategory(tc.name)
			if got != tc.want {
				t.Errorf("inferCategory(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestAdaptToolGroup_Empty(t *testing.T) {
	g := ToolGroup{Name: "test", Tools: nil}
	regs := AdaptToolGroup(g)
	if len(regs) != 0 {
		t.Errorf("AdaptToolGroup(empty) = %d regs, want 0", len(regs))
	}
}

func TestExportJSON_EmptySkills(t *testing.T) {
	data, err := ExportJSON(nil)
	if err != nil {
		t.Fatalf("ExportJSON(nil) returned error: %v", err)
	}
	if len(data) == 0 {
		t.Error("ExportJSON(nil) returned empty data")
	}
}

func TestExportMarkdown_Empty(t *testing.T) {
	got := ExportMarkdown(nil)
	if !strings.Contains(got, "No skills") {
		t.Errorf("ExportMarkdown(nil) = %q, want 'No skills'", got)
	}
}

func TestExportMarkdown_WithSkills(t *testing.T) {
	skills := []SkillDef{
		{Name: "ralphglasses_session_launch", Description: "Launch a session", Category: "session"},
	}
	got := ExportMarkdown(skills)
	if !strings.Contains(got, "session") {
		t.Errorf("ExportMarkdown with skills = %q, want 'session' section", got)
	}
	if !strings.Contains(got, "ralphglasses_session_launch") {
		t.Errorf("ExportMarkdown with skills = %q, want tool name", got)
	}
}
