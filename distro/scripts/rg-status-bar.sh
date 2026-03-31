#!/usr/bin/env bash
# rg-status-bar.sh — Fleet observability status bar for the thin client
#
# Three modes:
#   --all --json          Cache writer (systemd timer → /tmp/rg-status.json)
#   --segment NAME        Segment reader (i3blocks → reads cache → prints i3blocks format)
#   --segment NAME --waybar  Segment reader (waybar → reads cache → prints waybar JSON)
#
# See also:
#   distro/i3/i3blocks.conf        — i3blocks block definitions
#   distro/sway/waybar/config.jsonc — waybar custom module definitions
#   distro/systemd/rg-status-bar.* — systemd timer/service for cache refresh

set -euo pipefail

# ── Constants ──

RG_BIN="/usr/local/bin/ralphglasses"
CACHE_FILE="/tmp/rg-status.json"
CACHE_MAX_AGE=90  # seconds — stale if older than 3x refresh interval

COLOR_GREEN="#5af78e"
COLOR_RED="#ff5c57"
COLOR_YELLOW="#f3f99d"
COLOR_GRAY="#686868"
COLOR_PINK="#ff6ac1"
COLOR_CYAN="#57c7ff"

# Model abbreviation map (mirrors rg_parse.py)
declare -A MODEL_ABBREV=(
  [o1-pro]=o1p
  [o4-mini]=o4m
  [gpt-4o]=4o
  [claude-sonnet-4-6]=son
  [claude-opus-4-6]=opus
  [sonnet]=son
  [codex-mini-latest]=cdx
  [gpt-5.4-xhigh]=5.4x
)

# ── Helpers ──

die() { echo "$1" >&2; exit 1; }

# Print i3blocks output: full_text, short_text, color
i3blocks_out() {
  local full="$1" short="$2" color="$3"
  printf '%s\n%s\n%s\n' "$full" "$short" "$color"
}

# Print waybar JSON output: {"text": ..., "tooltip": ..., "class": ...}
waybar_out() {
  local text="$1" tooltip="$2" css_class="$3"
  printf '{"text": "%s", "tooltip": "%s", "class": "%s"}\n' "$text" "$tooltip" "$css_class"
}

offline() {
  if [[ "$WAYBAR_MODE" = true ]]; then
    waybar_out "offline" "ralphglasses offline" "offline"
  else
    i3blocks_out "offline" "offline" "$COLOR_GRAY"
  fi
  exit 0
}

idle() {
  if [[ "$WAYBAR_MODE" = true ]]; then
    waybar_out "idle" "no active fleet" "idle"
  else
    i3blocks_out "idle" "idle" "$COLOR_GRAY"
  fi
  exit 0
}

# Check if cache file exists and is fresh (< CACHE_MAX_AGE seconds old)
is_cache_fresh() {
  [[ -f "$CACHE_FILE" ]] || return 1
  local now file_mtime age
  now=$(date +%s)
  file_mtime=$(stat -c %Y "$CACHE_FILE" 2>/dev/null || stat -f %m "$CACHE_FILE" 2>/dev/null) || return 1
  age=$(( now - file_mtime ))
  (( age < CACHE_MAX_AGE ))
}

# Abbreviate a model name using MODEL_ABBREV map
abbrev_model() {
  local m="$1"
  if [[ -n "${MODEL_ABBREV[$m]+x}" ]]; then
    echo "${MODEL_ABBREV[$m]}"
  else
    # Fallback: first 3 chars
    echo "${m:0:3}"
  fi
}

# ── Fetch ──

# Call ralphglasses MCP and extract the inner JSON from content[0].text
fetch_fleet_status() {
  if [[ ! -x "$RG_BIN" ]]; then
    return 1
  fi
  local raw inner
  raw=$("$RG_BIN" mcp-call ralphglasses_fleet_status 2>/dev/null) || return 1
  # MCP returns { content: [{ text: "<json>" }] }
  inner=$(echo "$raw" | jq -r '.content[0].text // empty' 2>/dev/null) || return 1
  [[ -n "$inner" ]] || return 1
  echo "$inner"
}

# ── Segment Computation ──

# Given fleet JSON on stdin, compute all 6 segments as a JSON object
compute_all_segments() {
  jq -c '{
    fleet: {
      running:   [.loops[] | select(.status == "running")] | length,
      completed: [.loops[] | select(.status == "completed")] | length,
      failed:    [.loops[] | select(.status == "failed")] | length,
      pending:   [.loops[] | select(.status == "pending" or .status == "queued")] | length,
      total:     (.loops // []) | length
    },
    loops: {
      total_runs: (.loops // []) | length,
      completed:  [.loops[] | select(.status == "completed")] | length,
      converge_pct: (
        if (.loops // []) | length > 0 then
          (([.loops[] | select(.status == "completed")] | length) * 100 / ((.loops // []) | length))
        else 0 end
      )
    },
    cost: {
      total_spend_usd: (.summary.total_spend_usd // 0)
    },
    models: (
      [.loops[] | .planner_model // empty]
      | group_by(.) | map({model: .[0], count: length})
      | sort_by(-.count) | .[0:3]
    ),
    repos: {
      scanned: (.summary.total_repos // 0),
      targeted: ([.loops[].repo // empty] | unique | length)
    },
    iters: {
      total: ([.loops[].iterations // 0] | add // 0),
      avg_per_run: (
        if (.loops // []) | length > 0 then
          (([.loops[].iterations // 0] | add // 0) / ((.loops // []) | length) * 10 | round / 10)
        else 0 end
      )
    }
  }'
}

# ── Segment Readers ──

read_segment_fleet() {
  local data="$1"
  local running completed failed pending total color full short
  running=$(echo "$data" | jq -r '.fleet.running')
  completed=$(echo "$data" | jq -r '.fleet.completed')
  failed=$(echo "$data" | jq -r '.fleet.failed')
  pending=$(echo "$data" | jq -r '.fleet.pending')
  total=$(echo "$data" | jq -r '.fleet.total')

  if (( total == 0 )); then idle; fi

  full="↻${running} ✓${completed} ✗${failed}"
  short="↻${running}"

  if (( running > 0 )); then
    color="$COLOR_GREEN"
  elif (( failed > 0 && completed == 0 )); then
    color="$COLOR_RED"
  elif (( pending > 0 )); then
    color="$COLOR_YELLOW"
  else
    color="$COLOR_GRAY"
  fi

  i3blocks_out "$full" "$short" "$color"
}

read_segment_loops() {
  local data="$1"
  local total_runs converge_pct
  total_runs=$(echo "$data" | jq -r '.loops.total_runs')
  converge_pct=$(echo "$data" | jq -r '.loops.converge_pct')
  i3blocks_out "${total_runs} runs ${converge_pct}%" "${total_runs}r" "$COLOR_PINK"
}

read_segment_cost() {
  local data="$1"
  local spend spend_int
  spend=$(echo "$data" | jq -r '.cost.total_spend_usd')
  spend_int=$(printf '%.0f' "$spend")
  i3blocks_out "\$${spend}" "\$${spend_int}" "$COLOR_GREEN"
}

read_segment_models() {
  local data="$1"
  local count full short model abbr cnt
  count=$(echo "$data" | jq -r '.models | length')
  if (( count == 0 )); then
    i3blocks_out "no models" "-" "$COLOR_GRAY"
    return
  fi
  full=""
  short=""
  while IFS=$'\t' read -r model cnt; do
    abbr=$(abbrev_model "$model")
    if [[ -n "$full" ]]; then full+=" "; fi
    full+="${abbr}:${cnt}"
    if [[ -z "$short" ]]; then short="${abbr}:${cnt}"; fi
  done < <(echo "$data" | jq -r '.models[] | [.model, .count] | @tsv')
  i3blocks_out "$full" "$short" "$COLOR_YELLOW"
}

read_segment_repos() {
  local data="$1"
  local scanned targeted
  scanned=$(echo "$data" | jq -r '.repos.scanned')
  targeted=$(echo "$data" | jq -r '.repos.targeted')
  i3blocks_out "${scanned} scan ${targeted} tgt" "${scanned}s" "$COLOR_CYAN"
}

read_segment_iters() {
  local data="$1"
  local total avg
  total=$(echo "$data" | jq -r '.iters.total')
  avg=$(echo "$data" | jq -r '.iters.avg_per_run')
  i3blocks_out "${total} iters ~${avg}/run" "${total}i" "$COLOR_GRAY"
}

# ── Waybar Segment Readers ──

read_segment_fleet_waybar() {
  local data="$1"
  local running completed failed total
  running=$(echo "$data" | jq -r '.fleet.running')
  completed=$(echo "$data" | jq -r '.fleet.completed')
  failed=$(echo "$data" | jq -r '.fleet.failed')
  total=$(echo "$data" | jq -r '.fleet.total')
  if (( total == 0 )); then idle; fi
  local css="ok"
  if (( running > 0 )); then css="running"; elif (( failed > 0 )); then css="error"; fi
  waybar_out "↻${running} ✓${completed} ✗${failed}" "${running} running, ${completed} completed, ${failed} failed" "rg-fleet-${css}"
}

read_segment_loops_waybar() {
  local data="$1"
  local total_runs converge_pct
  total_runs=$(echo "$data" | jq -r '.loops.total_runs')
  converge_pct=$(echo "$data" | jq -r '.loops.converge_pct')
  waybar_out "${total_runs} runs ${converge_pct}%" "${total_runs} total runs, ${converge_pct}% converged" "rg-loops"
}

read_segment_cost_waybar() {
  local data="$1"
  local spend
  spend=$(echo "$data" | jq -r '.cost.total_spend_usd')
  waybar_out "\$${spend}" "Total spend: \$${spend}" "rg-cost"
}

read_segment_models_waybar() {
  local data="$1"
  local count text model abbr cnt
  count=$(echo "$data" | jq -r '.models | length')
  if (( count == 0 )); then
    waybar_out "no models" "No active models" "rg-models"
    return
  fi
  text=""
  while IFS=$'\t' read -r model cnt; do
    abbr=$(abbrev_model "$model")
    if [[ -n "$text" ]]; then text+=" "; fi
    text+="${abbr}:${cnt}"
  done < <(echo "$data" | jq -r '.models[] | [.model, .count] | @tsv')
  waybar_out "$text" "Model distribution: ${text}" "rg-models"
}

read_segment_repos_waybar() {
  local data="$1"
  local scanned targeted
  scanned=$(echo "$data" | jq -r '.repos.scanned')
  targeted=$(echo "$data" | jq -r '.repos.targeted')
  waybar_out "${scanned} scan ${targeted} tgt" "${scanned} scanned, ${targeted} targeted" "rg-repos"
}

read_segment_iters_waybar() {
  local data="$1"
  local total avg
  total=$(echo "$data" | jq -r '.iters.total')
  avg=$(echo "$data" | jq -r '.iters.avg_per_run')
  waybar_out "${total} iters ~${avg}/run" "${total} total iterations, ~${avg} per run" "rg-iters"
}

# ── Main ──

MODE=""
SEGMENT=""
JSON_OUT=false
WAYBAR_MODE=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --all)     MODE="all"; shift ;;
    --json)    JSON_OUT=true; shift ;;
    --waybar)  WAYBAR_MODE=true; shift ;;
    --segment) MODE="segment"; SEGMENT="${2:-}"; shift 2 ;;
    --help|-h) echo "Usage: rg-status-bar.sh [--all --json] [--segment NAME [--waybar]]"; exit 0 ;;
    *)         die "Unknown argument: $1" ;;
  esac
done

# Mode 1: Cache writer — fetch from MCP + compute all segments
if [[ "$MODE" == "all" ]]; then
  fleet_json=$(fetch_fleet_status) || die "Failed to fetch fleet status"
  segments=$(echo "$fleet_json" | compute_all_segments) || die "Failed to compute segments"
  if $JSON_OUT; then
    echo "$segments"
  else
    echo "$segments" | jq .
  fi
  exit 0
fi

# Mode 2: Segment reader — read from cache, output i3blocks format
if [[ "$MODE" == "segment" ]]; then
  [[ -n "$SEGMENT" ]] || die "Missing segment name"

  # Try cache first
  if is_cache_fresh; then
    data=$(cat "$CACHE_FILE")
  else
    # Cache stale — try direct fetch
    fleet_json=$(fetch_fleet_status 2>/dev/null) || true
    if [[ -n "${fleet_json:-}" ]]; then
      data=$(echo "$fleet_json" | compute_all_segments 2>/dev/null) || true
    fi
    # If direct fetch also failed, use stale cache if it exists
    if [[ -z "${data:-}" ]] && [[ -f "$CACHE_FILE" ]]; then
      data=$(cat "$CACHE_FILE")
      # Signal stale data — override color to yellow in output
    fi
    [[ -n "${data:-}" ]] || offline
  fi

  if [[ "$WAYBAR_MODE" = true ]]; then
    case "$SEGMENT" in
      fleet)  read_segment_fleet_waybar  "$data" ;;
      loops)  read_segment_loops_waybar  "$data" ;;
      cost)   read_segment_cost_waybar   "$data" ;;
      models) read_segment_models_waybar "$data" ;;
      repos)  read_segment_repos_waybar  "$data" ;;
      iters)  read_segment_iters_waybar  "$data" ;;
      *)      die "Unknown segment: $SEGMENT" ;;
    esac
  else
    case "$SEGMENT" in
      fleet)  read_segment_fleet  "$data" ;;
      loops)  read_segment_loops  "$data" ;;
      cost)   read_segment_cost   "$data" ;;
      models) read_segment_models "$data" ;;
      repos)  read_segment_repos  "$data" ;;
      iters)  read_segment_iters  "$data" ;;
      *)      die "Unknown segment: $SEGMENT" ;;
    esac
  fi
  exit 0
fi

# No mode specified
echo "Usage: rg-status-bar.sh [--all --json] [--segment fleet|loops|cost|models|repos|iters]"
exit 1
