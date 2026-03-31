#!/usr/bin/env bats
# hyprland-distro.bats — Offline validation of Hyprland distro configs.

@test "hyprland.conf exists and is non-empty" {
  [ -s distro/hyprland/hyprland.conf ]
}

@test "kiosk.conf exists and is non-empty" {
  [ -s distro/hyprland/kiosk.conf ]
}

@test "hypridle.conf exists" {
  [ -f distro/hyprland/hypridle.conf ]
}

@test "hyprlock.conf exists" {
  [ -f distro/hyprland/hyprlock.conf ]
}

@test "waybar config.jsonc exists" {
  [ -s distro/hyprland/waybar/config.jsonc ]
}

@test "waybar style.css exists" {
  [ -s distro/hyprland/waybar/style.css ]
}

@test "nvidia-wayland.conf exists" {
  [ -f distro/hyprland/environment.d/nvidia-wayland.conf ]
}

@test "Dockerfile.manjaro.hyprland exists" {
  [ -s distro/Dockerfile.manjaro.hyprland ]
}

@test "compositor-detect.sh self-test passes" {
  run bash distro/scripts/compositor-detect.sh --test
  [ "$status" -eq 0 ]
}

@test "validate-hyprland-conf.sh passes on hyprland.conf" {
  run bash distro/scripts/validate-hyprland-conf.sh distro/hyprland/hyprland.conf
  [ "$status" -eq 0 ]
}

@test "validate-hyprland-conf.sh passes on kiosk.conf" {
  run bash distro/scripts/validate-hyprland-conf.sh distro/hyprland/kiosk.conf
  [ "$status" -eq 0 ]
}

@test "validate-waybar.sh passes on Hyprland waybar config" {
  run bash distro/scripts/validate-waybar.sh distro/hyprland/waybar/config.jsonc
  [ "$status" -eq 0 ]
}

@test "validate-dockerfile.sh passes on Hyprland Dockerfile" {
  run bash distro/scripts/validate-dockerfile.sh distro/Dockerfile.manjaro.hyprland
  [ "$status" -eq 0 ]
}

@test "Dockerfile references compositor abstraction scripts" {
  grep -q "compositor-detect.sh" distro/Dockerfile.manjaro.hyprland
  grep -q "compositor-cmd.sh" distro/Dockerfile.manjaro.hyprland
}

@test "hyprland.conf reload keybinding uses compositor-cmd.sh" {
  # Reload keybinding should use the abstraction layer, not raw hyprctl
  run grep -E 'bind.*exec.*hyprctl reload' distro/hyprland/hyprland.conf
  [ "$status" -ne 0 ]
}
