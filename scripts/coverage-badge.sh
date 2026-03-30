#!/bin/bash
# coverage-badge.sh — Generate a shields.io coverage badge for the project.
#
# Usage:
#   scripts/coverage-badge.sh [--output FILE] [--min PERCENT] [--skip-tests]
#
# Options:
#   --output FILE    Write badge markdown to FILE instead of stdout
#   --min PERCENT    Minimum coverage threshold (default: 80). Exit 1 if below.
#   --skip-tests     Skip running tests; reuse existing coverage.out
#
# Exit codes:
#   0  Coverage meets or exceeds the minimum threshold
#   1  Coverage below threshold, or an error occurred

set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────

MIN_COVERAGE=80
OUTPUT_FILE=""
SKIP_TESTS=false
COVER_PROFILE="coverage.out"

# ── Helpers ───────────────────────────────────────────────────────────────────

usage() {
    sed -n '2,/^$/s/^# \?//p' "$0"
    exit 0
}

die() {
    printf "error: %s\n" "$1" >&2
    exit 1
}

# ── Parse arguments ──────────────────────────────────────────────────────────

while [ $# -gt 0 ]; do
    case "$1" in
        --help|-h)
            usage
            ;;
        --output)
            [ $# -ge 2 ] || die "--output requires a file path"
            OUTPUT_FILE="$2"
            shift 2
            ;;
        --min)
            [ $# -ge 2 ] || die "--min requires a number"
            MIN_COVERAGE="$2"
            # Validate that min is a number
            case "$MIN_COVERAGE" in
                ''|*[!0-9.]*) die "--min must be a number, got: $MIN_COVERAGE" ;;
            esac
            shift 2
            ;;
        --skip-tests)
            SKIP_TESTS=true
            shift
            ;;
        *)
            die "unknown option: $1"
            ;;
    esac
done

# ── Locate project root (directory containing go.mod) ────────────────────────

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [ ! -f "$PROJECT_ROOT/go.mod" ]; then
    die "cannot find go.mod in $PROJECT_ROOT"
fi

cd "$PROJECT_ROOT"

# ── Run tests with coverage ──────────────────────────────────────────────────

if [ "$SKIP_TESTS" = false ]; then
    printf "Running tests with coverage...\n" >&2
    if ! go test -coverprofile="$COVER_PROFILE" ./... 2>&1; then
        die "go test failed"
    fi
fi

if [ ! -f "$COVER_PROFILE" ]; then
    die "coverage profile not found: $COVER_PROFILE"
fi

# ── Parse total coverage percentage ──────────────────────────────────────────

# `go tool cover -func` outputs lines like:
#   total:    (statements)    86.3%
TOTAL_LINE="$(go tool cover -func="$COVER_PROFILE" | grep '^total:')"
if [ -z "$TOTAL_LINE" ]; then
    die "could not parse total coverage from $COVER_PROFILE"
fi

# Extract the percentage (e.g. "86.3") — strip the trailing %
COVERAGE="$(printf '%s' "$TOTAL_LINE" | awk '{print $NF}' | tr -d '%')"

if [ -z "$COVERAGE" ]; then
    die "could not extract coverage percentage"
fi

printf "Total coverage: %s%%\n" "$COVERAGE" >&2

# ── Determine badge color ───────────────────────────────────────────────────

# Compare as integers (truncate decimal) for color thresholds.
COVERAGE_INT="${COVERAGE%%.*}"

if [ "$COVERAGE_INT" -ge 80 ]; then
    COLOR="brightgreen"
elif [ "$COVERAGE_INT" -ge 60 ]; then
    COLOR="yellow"
else
    COLOR="red"
fi

# ── Build badge markdown ─────────────────────────────────────────────────────

# URL-encode the percent sign as %25 for shields.io
BADGE_URL="https://img.shields.io/badge/coverage-${COVERAGE}%25-${COLOR}"
BADGE_MD="![Coverage](${BADGE_URL})"

# ── Output ───────────────────────────────────────────────────────────────────

if [ -n "$OUTPUT_FILE" ]; then
    printf '%s\n' "$BADGE_MD" > "$OUTPUT_FILE"
    printf "Badge written to %s\n" "$OUTPUT_FILE" >&2
else
    printf '%s\n' "$BADGE_MD"
fi

# ── Threshold check ──────────────────────────────────────────────────────────

# Use awk for floating-point comparison
BELOW_THRESHOLD="$(awk "BEGIN { print ($COVERAGE < $MIN_COVERAGE) ? 1 : 0 }")"

if [ "$BELOW_THRESHOLD" -eq 1 ]; then
    printf "FAIL: coverage %s%% is below minimum threshold %s%%\n" "$COVERAGE" "$MIN_COVERAGE" >&2
    exit 1
fi

printf "OK: coverage %s%% meets minimum threshold %s%%\n" "$COVERAGE" "$MIN_COVERAGE" >&2
exit 0
