#!/usr/bin/env bash
# check-coverage.sh — fails if any target package is below its coverage threshold
set -euo pipefail

COVERFILE="${1:-coverage.out}"

if [[ ! -f "$COVERFILE" ]]; then
  echo "error: coverage file not found: $COVERFILE" >&2
  echo "hint: run 'go test -coverprofile=$COVERFILE ./...' first" >&2
  exit 1
fi

# Thresholds (package-suffix → minimum %)
# Based on measured coverage as of 2026-03-24:
#   discovery=100%, model=90%, process=92%, mcpserver=48%, session=68%
declare -A THRESHOLDS=(
    [discovery]=90
    [model]=80
    [process]=80
    [mcpserver]=40
    [session]=60
)

FAILED=0
for pkg in "${!THRESHOLDS[@]}"; do
    min="${THRESHOLDS[$pkg]}"
    actual=$(go tool cover -func="$COVERFILE" | grep "/$pkg/" | tail -1 | awk '{print $3}' | tr -d '%')
    if [ -z "$actual" ]; then
        echo "WARN: no coverage data for $pkg"
        continue
    fi
    if (( $(echo "$actual < $min" | bc -l) )); then
        echo "FAIL: $pkg coverage ${actual}% < ${min}% threshold"
        FAILED=1
    else
        echo "OK:   $pkg coverage ${actual}% >= ${min}%"
    fi
done
exit $FAILED
