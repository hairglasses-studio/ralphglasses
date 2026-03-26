package mcpserver

import (
	"testing"
)

func TestEveryRegisteredToolHasAnnotation(t *testing.T) {
	s := &Server{}
	groups := s.buildToolGroups()

	var missing []string
	for _, g := range groups {
		for _, te := range g.Tools {
			if _, ok := ToolAnnotations[te.Tool.Name]; !ok {
				missing = append(missing, te.Tool.Name)
			}
		}
	}
	// Also check the two dispatch-only tools.
	for _, name := range []string{"ralphglasses_tool_groups", "ralphglasses_load_tool_group"} {
		if _, ok := ToolAnnotations[name]; !ok {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		for _, name := range missing {
			t.Errorf("tool %q registered but has no entry in ToolAnnotations", name)
		}
	}
}

func TestGetAnnotationDefault(t *testing.T) {
	a := GetAnnotation("nonexistent_tool_xyz")
	if !a.ReadOnly {
		t.Error("default annotation should be ReadOnly")
	}
	if !a.Idempotent {
		t.Error("default annotation should be Idempotent")
	}
	if a.Destructive {
		t.Error("default annotation should not be Destructive")
	}
	if a.OpenWorld {
		t.Error("default annotation should not be OpenWorld")
	}
}

func TestGetAnnotationKnownTool(t *testing.T) {
	a := GetAnnotation("ralphglasses_status")
	if !a.ReadOnly {
		t.Error("ralphglasses_status should be ReadOnly")
	}
	if !a.Idempotent {
		t.Error("ralphglasses_status should be Idempotent")
	}
}

func TestDestructiveToolsMarkedCorrectly(t *testing.T) {
	destructive := []string{
		"ralphglasses_stop",
		"ralphglasses_stop_all",
		"ralphglasses_session_stop",
		"ralphglasses_session_stop_all",
		"ralphglasses_loop_stop",
		"ralphglasses_journal_prune",
	}
	for _, name := range destructive {
		a, ok := ToolAnnotations[name]
		if !ok {
			t.Errorf("expected %q in ToolAnnotations", name)
			continue
		}
		if !a.Destructive {
			t.Errorf("%q should be marked Destructive", name)
		}
	}
}

func TestDestructiveToolsNotReadOnly(t *testing.T) {
	for name, a := range ToolAnnotations {
		if a.Destructive && a.ReadOnly {
			t.Errorf("%q is marked both Destructive and ReadOnly", name)
		}
	}
}

func TestAnnotationMapNotEmpty(t *testing.T) {
	if len(ToolAnnotations) == 0 {
		t.Fatal("ToolAnnotations should have entries")
	}
}
