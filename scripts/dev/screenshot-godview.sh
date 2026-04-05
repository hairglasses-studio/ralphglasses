#!/usr/bin/env bash
set -euo pipefail

# Screenshot the most recently focused Ghostty window running godview.
# Usage: ./screenshot-godview.sh [output.png]

OUT="${1:-/tmp/godview-screenshot.png}"

# Try to find a Ghostty window (most recent)
WINDOW=$(hyprctl clients -j | jq -r '
  [.[] | select(.class == "com.mitchellh.ghostty")] | last |
  {x: .at[0], y: .at[1], w: .size[0], h: .size[1]}
')

X=$(echo "$WINDOW" | jq -r '.x')
Y=$(echo "$WINDOW" | jq -r '.y')
W=$(echo "$WINDOW" | jq -r '.w')
H=$(echo "$WINDOW" | jq -r '.h')

if [ "$X" = "null" ] || [ -z "$X" ]; then
  echo "ERROR: No Ghostty window found" >&2
  exit 1
fi

grim -g "$X,$Y ${W}x${H}" "$OUT"
magick "$OUT" -resize "1568x1568>" "$OUT"
echo "$OUT"
