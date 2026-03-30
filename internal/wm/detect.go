// Package wm provides window manager detection and abstraction.
package wm

import "os"

// Type represents a detected window manager.
type Type string

const (
	TypeI3       Type = "i3"
	TypeSway     Type = "sway"
	TypeHyprland Type = "hyprland"
	TypeUnknown  Type = "unknown"
	TypeNone     Type = "none"
)

// Detect identifies the running window manager by checking environment variables.
func Detect() Type {
	// Sway sets SWAYSOCK
	if os.Getenv("SWAYSOCK") != "" {
		return TypeSway
	}
	// i3 sets I3SOCK
	if os.Getenv("I3SOCK") != "" {
		return TypeI3
	}
	// Hyprland sets HYPRLAND_INSTANCE_SIGNATURE
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" {
		return TypeHyprland
	}
	// Check XDG_CURRENT_DESKTOP as fallback
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	switch desktop {
	case "sway", "Sway":
		return TypeSway
	case "i3":
		return TypeI3
	case "Hyprland":
		return TypeHyprland
	}
	// Check WAYLAND_DISPLAY for generic Wayland
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") != "" {
		return TypeUnknown
	}
	return TypeNone
}

// String returns the human-readable name.
func (t Type) String() string {
	return string(t)
}

// IsWayland returns true if the WM uses Wayland.
func (t Type) IsWayland() bool {
	return t == TypeSway || t == TypeHyprland
}

// IsX11 returns true if the WM uses X11.
func (t Type) IsX11() bool {
	return t == TypeI3
}
