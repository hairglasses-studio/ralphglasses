package wm

import "testing"

func TestDetect(t *testing.T) {
	// Just verify it doesn't panic and returns a valid type
	wmType := Detect()
	if wmType == "" {
		t.Error("expected non-empty type")
	}
}

func TestType_IsWayland(t *testing.T) {
	if !TypeSway.IsWayland() {
		t.Error("sway should be wayland")
	}
	if !TypeHyprland.IsWayland() {
		t.Error("hyprland should be wayland")
	}
	if TypeI3.IsWayland() {
		t.Error("i3 should not be wayland")
	}
}

func TestType_IsX11(t *testing.T) {
	if !TypeI3.IsX11() {
		t.Error("i3 should be X11")
	}
	if TypeSway.IsX11() {
		t.Error("sway should not be X11")
	}
}

func TestType_String(t *testing.T) {
	if TypeI3.String() != "i3" {
		t.Errorf("unexpected: %s", TypeI3.String())
	}
}
