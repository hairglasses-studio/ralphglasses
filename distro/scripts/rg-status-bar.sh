#!/bin/bash
# rg-status-bar.sh — Fleet observability status bar for the thin client
#
# This script feeds live ralphglasses fleet metrics into i3status/i3blocks
# on the bootable thin client. It mirrors the SketchyBar observability
# implemented for macOS in the dotfiles repo (sketchybar/plugins/ralphglasses.sh
# + rg_parse.py), adapted for Linux + i3.
#
# ── Architecture ──
# On macOS, SketchyBar polls ralphglasses.sh every 30s, which calls
#   `go run . mcp-call ralphglasses_fleet_status`
# and pipes the JSON through rg_parse.py to populate 6 bar items:
#   rg.fleet  — active/completed/failed loop counts + color-coded status
#   rg.loops  — total runs + convergence %
#   rg.cost   — total spend USD
#   rg.models — top 3 planner models (abbreviated)
#   rg.repos  — scanned vs targeted repo counts
#   rg.iters  — total iterations + avg per run
#
# On the thin client, we use i3blocks (or i3status-rust) with this script
# as a "command" block. It outputs one line per block or a single pango-
# formatted line depending on the bar implementation chosen.
#
# ── Data Source ──
# The ralphglasses binary is installed at /usr/local/bin/ralphglasses.
# Fleet status: `ralphglasses mcp-call ralphglasses_fleet_status`
# Returns JSON: { content: [{ text: "{ loops: [...], summary: {...}, repos: [...] }" }] }
#
# ── Metrics to Display ──
# 1. Fleet status  — running/completed/failed/pending counts
# 2. Loop detail   — total runs, convergence %
# 3. Cost          — total_spend_usd from summary
# 4. Models        — top planner models (abbreviated: o1p, o4m, 4o, son, opus, cdx, 5.4x)
# 5. Repos         — scanned count, unique target count
# 6. Iterations    — total iterations, average per run
#
# ── Integration Points ──
# i3blocks: Add a block per metric in i3blocks.conf, each calling this script
#           with an argument: `command=rg-status-bar.sh fleet`
# i3status-rust: Use custom_dbus or script block
# polybar: Use custom/script modules
#
# ── Color Coding ──
# Green  (#5af78e) — fleet has running loops
# Red    (#ff5c57) — all failed, none completed
# Yellow (#f3f99d) — pending loops waiting
# Gray   (#686868) — idle / offline
#
# TODO: Implement the following:
# - [ ] Parse ralphglasses_fleet_status JSON (reuse rg_parse.py logic or rewrite in bash/jq)
# - [ ] Output format: i3blocks expects `full_text\nshort_text\ncolor` per block
# - [ ] Handle offline/missing binary gracefully (show "offline" in gray)
# - [ ] Add --json flag for structured output (for i3status-rust integration)
# - [ ] Add --segment <name> flag to output a single metric (fleet|loops|cost|models|repos|iters)
# - [ ] Add --all flag to output all metrics as a single pango-formatted line
# - [ ] Consider caching: write to /tmp/rg-status.json with TTL to avoid
#       6 separate `ralphglasses mcp-call` invocations per refresh cycle
# - [ ] Test with the 7-monitor i3 setup (bar appears on all outputs)
# - [ ] Wire into systemd: rg-status-bar.timer for periodic refresh to cache file,
#       i3blocks reads from cache (avoids blocking the bar on slow MCP calls)
#
# See also:
#   dotfiles/sketchybar/plugins/ralphglasses.sh  — macOS implementation
#   dotfiles/sketchybar/plugins/rg_parse.py      — JSON parser (Python)
#   distro/i3/i3blocks.conf                      — i3blocks config (to be created)
#   distro/systemd/rg-status-bar.service          — cache refresh service (to be created)
#   distro/systemd/rg-status-bar.timer            — cache refresh timer (to be created)

set -euo pipefail

echo "rg-status-bar.sh: Not yet implemented"
echo "Usage: rg-status-bar.sh [--segment fleet|loops|cost|models|repos|iters] [--all] [--json]"
exit 1
