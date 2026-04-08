#!/usr/bin/env bash
# SessionStart hook: inject fleet status context for ralphglasses sessions
set -euo pipefail

context=""

# Git branch + recent commits
if git rev-parse --is-inside-work-tree &>/dev/null; then
  branch=$(git branch --show-current 2>/dev/null || echo "detached")
  recent=$(git log --oneline -3 2>/dev/null || true)
  if [[ -n "$recent" ]]; then
    context+="Git: $branch"$'\n'"$recent"$'\n'
  fi
fi

# Fleet snapshot: count .ralph/ dirs with active status
scan_path="${RALPHGLASSES_SCAN_PATH:-$HOME/hairglasses-studio}"
if [[ -d "$scan_path" ]]; then
  active=0
  total=0
  for status_file in "$scan_path"/*/.ralph/status.json; do
    [[ -f "$status_file" ]] || continue
    total=$((total + 1))
    if grep -q '"running"' "$status_file" 2>/dev/null; then
      active=$((active + 1))
    fi
  done
  if [[ "$total" -gt 0 ]]; then
    context+="Fleet: $active active / $total repos with .ralph/"$'\n'
  fi
fi

# Circuit breaker warnings
for cb_file in "$scan_path"/*/.ralph/.circuit_breaker_state; do
  [[ -f "$cb_file" ]] || continue
  state=$(cat "$cb_file" 2>/dev/null || true)
  if [[ "$state" == "OPEN" ]]; then
    repo=$(basename "$(dirname "$(dirname "$cb_file")")")
    context+="WARNING: Circuit breaker OPEN in $repo"$'\n'
  fi
done

if [[ -n "$context" ]]; then
  echo "$context"
fi
