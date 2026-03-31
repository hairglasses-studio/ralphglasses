# Hyprland Integration Marathon — Handoff Instructions

Resume instructions for another Claude Code session on a different machine.

## What Was Completed

All 8 sessions of the Sway/Manjaro/Hyprland integration marathon are **complete**:

### Session 1: Compositor Abstraction Shell Layer
- `distro/scripts/compositor-detect.sh` — Detects sway/hyprland/i3/unknown from env vars, XDG_CURRENT_DESKTOP, and running processes. Has `--test` self-test flag (6 tests pass).
- `distro/scripts/compositor-cmd.sh` — Unified command dispatcher with subcommands: `workspace`, `exec-on`, `outputs`, `clients`, `reload`, `dpms`, `version`, `fullscreen`, `move-to-workspace`. Has `--test` self-test flag.

### Session 2: Hyprland Distro Configs
- `distro/hyprland/hyprland.conf` — Full 7-monitor config ported from Sway. NVIDIA env vars, Snazzy palette, hjkl keybinds, gaps/borders, exec-once waybar + ralphglasses.
- `distro/hyprland/kiosk.conf` — Zero-chrome kiosk mode. Fullscreen Alacritty on all workspaces, emergency keybinds (Ctrl+Alt+Backspace), hypridle always-on.
- `distro/hyprland/hypridle.conf` — Minimal idle config (kiosk: no timeout).
- `distro/hyprland/hyprlock.conf` — Emergency lock screen with Snazzy palette.
- `distro/hyprland/environment.d/nvidia-wayland.conf` — Same as Sway but `XDG_CURRENT_DESKTOP=Hyprland`.

### Session 3: Waybar Dual-Compositor + hw-detect Expansion
- `distro/hyprland/waybar/config.jsonc` — Waybar config with `hyprland/workspaces`, `hyprland/submap`, `hyprland/window` modules.
- `distro/hyprland/waybar/style.css` — Identical Snazzy theme (CSS works for both compositors).
- `distro/scripts/waybar-launch.sh` — Sources compositor-detect.sh, copies correct waybar config, execs waybar.
- `distro/scripts/hw-detect.sh` — Expanded to write BOTH Sway and Hyprland monitor configs when `--wayland-only`.

### Session 4: Kiosk Generalization + Systemd
- `distro/scripts/compositor-kiosk-setup.sh` — Generic kiosk installer supporting `--compositor sway|hyprland`. Handles config install, NVIDIA env, autologin, bash_profile, systemd watchdog service.
- `distro/systemd/ralphglasses.service` — Already compositor-agnostic (WAYLAND_DISPLAY works for both).

### Session 5: Go ParseHyprlandMonitors + DetectMonitors
- `internal/wm/monitors.go` — Added `ParseHyprlandMonitors()` function and `case TypeHyprland:` in `DetectMonitors()`.
- `internal/wm/hyprland.go` — Refactored to use IPC client instead of `exec.Command("hyprctl")` shell-outs. Removed duplicate `HyprlandMonitor` struct.
- `internal/wm/monitors_test.go` — Added `TestParseHyprlandMonitors` with fixture data.

### Session 6: Go Hyprland Event Streaming
- `internal/wm/hyprland/events.go` — Full `EventListener` with `Listen(ctx, handler)`, auto-reconnect with exponential backoff (100ms→5s), `ParseEvent()` for `EVENT>>DATA\n` text protocol.
- `internal/wm/hyprland/events_test.go` — Tests for ParseEvent (14 cases), EventListener receive/close/cancel. All pass.

### Session 7: Dockerfile + Makefile
- `distro/Dockerfile.manjaro.hyprland` — Fork of Dockerfile.manjaro with Hyprland + AUR packages (hypridle, hyprlock, xdg-desktop-portal-hyprland).
- `distro/Makefile` — Added `docker-sway`, `docker-hyprland`, `build-both` targets.

### Session 8: Testing + Documentation
- All Go tests pass: `go test ./internal/wm/... -count=1` (6 packages, 0 failures)
- Shell self-tests pass: `compositor-detect.sh --test` (6/6)
- Full project builds: `go build ./...`
- `distro/README.md` — Updated directory structure, added Hyprland section
- `CLAUDE.md` — Updated distro description to mention Hyprland

## Verification Commands

Run these to confirm everything works:

```bash
# Go compilation
go build ./...

# Go tests (wm packages)
go test ./internal/wm/... -count=1 -timeout 30s

# Shell self-tests
bash distro/scripts/compositor-detect.sh --test

# Check new files exist
ls distro/hyprland/hyprland.conf
ls distro/hyprland/kiosk.conf
ls distro/hyprland/waybar/config.jsonc
ls distro/scripts/compositor-detect.sh
ls distro/scripts/compositor-cmd.sh
ls distro/scripts/compositor-kiosk-setup.sh
ls distro/scripts/waybar-launch.sh
ls distro/Dockerfile.manjaro.hyprland
ls internal/wm/hyprland/events.go
```

## What Could Be Done Next

These are stretch goals not covered in the marathon:

1. **On-device testing** — Boot the Hyprland ISO on actual ProArt X870E hardware, verify 7-monitor layout, NVIDIA Wayland, and kiosk mode.
2. **Hyprland socket path migration** — `client.go` line 206 hardcodes `/tmp/hypr/`. Newer Hyprland versions use `$XDG_RUNTIME_DIR/hypr/`. Add fallback check.
3. **compositor-cmd.sh integration into kiosk watchdog** — Replace raw `swaymsg`/`hyprctl` calls in watchdog loops with `compositor-cmd.sh clients`.
4. **Docker build verification** — Actually run `make docker-hyprland` (requires Docker + Manjaro base image).
5. **Waybar Hyprland module validation** — Confirm Manjaro's `waybar` package includes Hyprland IPC support (may need `waybar-hyprland` variant).
6. **Hyprland config syntax validation** — `hyprctl reload` on a live Hyprland instance to verify syntax.

## Files Summary

**New files (17):**
- `distro/scripts/compositor-detect.sh`
- `distro/scripts/compositor-cmd.sh`
- `distro/scripts/compositor-kiosk-setup.sh`
- `distro/scripts/waybar-launch.sh`
- `distro/hyprland/hyprland.conf`
- `distro/hyprland/kiosk.conf`
- `distro/hyprland/hypridle.conf`
- `distro/hyprland/hyprlock.conf`
- `distro/hyprland/environment.d/nvidia-wayland.conf`
- `distro/hyprland/waybar/config.jsonc`
- `distro/hyprland/waybar/style.css`
- `distro/Dockerfile.manjaro.hyprland`
- `distro/docs/HYPRLAND-MARATHON-HANDOFF.md`
- `internal/wm/hyprland/events.go`
- `internal/wm/hyprland/events_test.go`

**Modified files (7):**
- `distro/scripts/hw-detect.sh` — Hyprland monitor config output
- `distro/Makefile` — Hyprland build targets
- `distro/README.md` — Directory structure update
- `CLAUDE.md` — Hyprland mention
- `internal/wm/monitors.go` — ParseHyprlandMonitors + DetectMonitors expansion
- `internal/wm/hyprland.go` — Refactored to IPC client
- `internal/wm/monitors_test.go` — Hyprland test cases
