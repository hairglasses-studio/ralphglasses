---
name: sway-rice
description: Desktop environment reference for a Manjaro Linux Sway/Wayland cyberpunk rice. Use when discussing Sway config, waybar, mako notifications, Ghostty terminal, shaders, foot terminal, starship prompt, wofi launcher, eww widgets, window management, keybindings, or terminal theming. Covers the Snazzy-on-black palette, 132+ GLSL shaders, and cross-platform dotfiles.
---

# Sway/Wayland Cyberpunk Rice

Manjaro Linux with Sway 1.11, Snazzy-on-black palette, 132+ GLSL terminal shaders.

## Active Stack

| Component | Tool | Config Location |
|-----------|------|-----------------|
| Window Manager | Sway 1.11 (Wayland) | `~/dotfiles/sway/` |
| Bar | Waybar | `~/dotfiles/waybar/` |
| Notifications | Mako | `~/dotfiles/mako/` |
| Launcher | Wofi | `~/dotfiles/wofi/` |
| Terminal | foot (primary), Ghostty (shaders) | `~/dotfiles/foot/`, `~/dotfiles/ghostty/` |
| Prompt | Starship | `~/dotfiles/starship/starship.toml` |
| Widgets | eww | `~/dotfiles/eww/` |
| Logout | wlogout | `~/dotfiles/wlogout/` |

Hyprland config exists (`~/dotfiles/hyprland/`) but Sway is the active session.

## Snazzy Palette (mandatory)

| Name | Hex | Use |
|------|-----|-----|
| Background | `#000000` | All backgrounds |
| Foreground | `#f1f1f0` | Default text |
| Cyan | `#57c7ff` | Primary accent, links, focus |
| Magenta | `#ff6ac1` | Secondary accent, highlights |
| Green | `#5af78e` | Success, active states |
| Yellow | `#f3f99d` | Warnings, emphasis |
| Red | `#ff5c57` | Errors, urgent |
| Gray | `#686868` | Inactive, borders |
| Dark BG | `#1a1a1a` | Panel backgrounds |
| Light FG | `#eff0eb` | Bright text |

Never introduce colors outside this palette unless explicitly requested.

## Font

**Maple Mono NF CN**, size 12. Fallback: JetBrainsMono Nerd Font.

## Clipboard

Always use `wl-copy` / `wl-paste` (Wayland). Never `xclip`, `xsel`, or `pbcopy`.

## Keyboard

Always US QWERTY: `xkb_layout us` in Sway, `kb_layout = us` in Hyprland. Never German QWERTZ.

## Ghostty Shaders

132+ GLSL shaders in `~/dotfiles/ghostty/shaders/`. Each must be self-contained (no `#include` — Ghostty doesn't support it). Use `// #include "lib/X.glsl"` for the build-time preprocessor.

Management scripts in `~/dotfiles/ghostty/shaders/bin/`:
- `shader-cycle.sh` — curated rotation
- `shader-random.sh` — random selection
- `shader-playlist.sh` — Fisher-Yates shuffled playlist
- `shader-test.sh` — compile via glslangValidator
- `shader-build.sh` — preprocessor (inlines includes)

Manifest: `~/dotfiles/ghostty/shaders/shaders.toml` (single source of truth).

## Config Editing Rules

- **Atomic writes**: `mktemp + mv` pattern for Ghostty configs (prevents partial reads)
- **Auto-reload**: Hooks fire `swaymsg reload`, `makoctl reload`, `pkill -SIGUSR2 waybar` when configs are edited
- **Tattoy**: section-scoped sed to avoid hitting wrong `[section].enabled`

## MCP Servers (always available)

| Server | Tools | Purpose |
|--------|-------|---------|
| sway-mcp | screenshot, windows, input, clipboard | Wayland desktop control |
| hyprland-mcp | 9 tools | Hyprland control (when active) |
| dotfiles-mcp | 4 tools | Config validation, symlink health |
| shader-mcp | 5 tools | Shader list/set/random/test/state |
| input-mcp | logiops, makima, bluetooth | Input device management |

## Dotfiles Layout

All configs live in `~/hairglasses-studio/dotfiles/` (symlinked as `~/dotfiles`), symlinked into `~/.config/` by `install.sh`. Cross-platform: macOS uses AeroSpace + SketchyBar + JankyBorders.

## Wallpaper Shaders

Live animated wallpapers via `shaderbg`:
- 5 procgen GLSL fragments in `wallpaper-shaders/`
- Keybinds: `$mod+Shift+W` (next), `$mod+Shift+Ctrl+W` (random)
