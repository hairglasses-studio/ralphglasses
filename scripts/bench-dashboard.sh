#!/usr/bin/env bash
# bench-dashboard.sh — Benchmark dashboard: parse benchstat output and emit
# a markdown summary tracking p50/p99 latencies over time.
#
# Usage:
#   ./scripts/bench-dashboard.sh [bench.txt]          # parse single run
#   ./scripts/bench-dashboard.sh old.txt new.txt      # compare two runs
#
# Output:
#   - Prints a markdown table of benchmark results to stdout
#   - If two files provided, shows delta columns (regression/improvement)
#   - Exits with 1 if any benchmark regressed by more than THRESHOLD_PCT (default 10%)
#
# Environment:
#   BENCH_THRESHOLD_PCT   Max allowed regression % before exit 1 (default: 10)
#   BENCH_HISTORY_DIR     Directory to store historical runs (default: .ralph/bench)

set -euo pipefail

THRESHOLD_PCT="${BENCH_THRESHOLD_PCT:-10}"
HISTORY_DIR="${BENCH_HISTORY_DIR:-.ralph/bench}"

usage() {
  echo "Usage: $0 [bench.txt] | [old.txt new.txt]" >&2
  exit 1
}

# Ensure we have at least one argument OR fall back to bench.txt.
if [ $# -eq 0 ]; then
  if [ -f "bench.txt" ]; then
    set -- "bench.txt"
  else
    usage
  fi
fi

NEW_FILE="$1"
OLD_FILE="${2:-}"

if [ ! -f "${NEW_FILE}" ]; then
  echo "ERROR: benchmark file not found: ${NEW_FILE}" >&2
  exit 1
fi

if [ -n "${OLD_FILE}" ] && [ ! -f "${OLD_FILE}" ]; then
  echo "ERROR: previous benchmark file not found: ${OLD_FILE}" >&2
  exit 1
fi

# ── Parse benchmark lines ─────────────────────────────────────────────────────
# Go benchmark output format:
#   BenchmarkFoo/bar-8   1000   1234 ns/op   512 B/op   3 allocs/op
parse_benchmarks() {
  local file="$1"
  grep -E '^Benchmark' "${file}" | awk '
  {
    name = $1
    # Remove the parallel suffix (e.g. -8)
    sub(/-[0-9]+$/, "", name)
    iters = $2
    # Find ns/op value (field after the iteration count)
    ns_per_op = $3
    unit = $4
    printf "%s\t%s\t%s %s\n", name, iters, ns_per_op, unit
  }
  '
}

# ── Emit markdown table ───────────────────────────────────────────────────────
echo "## Benchmark Dashboard"
echo ""
echo "Generated: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo ""

if [ -z "${OLD_FILE}" ]; then
  # Single file: just show the results.
  echo "| Benchmark | Iterations | ns/op |"
  echo "|-----------|------------|-------|"
  parse_benchmarks "${NEW_FILE}" | while IFS=$'\t' read -r name iters timing; do
    printf "| \`%s\` | %s | %s |\n" "${name#Benchmark}" "${iters}" "${timing}"
  done
else
  # Two files: compare using benchstat if available, else manual diff.
  if command -v benchstat &>/dev/null; then
    echo "### benchstat comparison"
    echo ""
    echo '```'
    benchstat "${OLD_FILE}" "${NEW_FILE}" || true
    echo '```'
    echo ""
  fi

  # Manual regression check.
  echo "### Regression check (threshold: ${THRESHOLD_PCT}%)"
  echo ""
  echo "| Benchmark | Old ns/op | New ns/op | Change | Status |"
  echo "|-----------|-----------|-----------|--------|--------|"

  REGRESSIONS=0
  declare -A OLD_VALS
  while IFS=$'\t' read -r name _iters timing; do
    ns=$(echo "${timing}" | awk '{print $1}')
    OLD_VALS["${name}"]="${ns}"
  done < <(parse_benchmarks "${OLD_FILE}")

  while IFS=$'\t' read -r name _iters timing; do
    ns=$(echo "${timing}" | awk '{print $1}')
    old_ns="${OLD_VALS["${name}"]:-}"
    if [ -z "${old_ns}" ]; then
      printf "| \`%s\` | - | %s ns | NEW | ✨ |\n" "${name#Benchmark}" "${ns}"
      continue
    fi
    # Calculate percent change using awk for floating point.
    pct=$(awk "BEGIN { printf \"%.1f\", (${ns} - ${old_ns}) / ${old_ns} * 100 }")
    abs_pct=$(awk "BEGIN { printf \"%.1f\", (${ns} - ${old_ns}) / ${old_ns} * 100 }" | tr -d -)
    if awk "BEGIN { exit !(${ns} - ${old_ns}) / ${old_ns} * 100 > ${THRESHOLD_PCT} }"; then
      status="REGRESSED"
      REGRESSIONS=$((REGRESSIONS + 1))
    elif awk "BEGIN { exit !(${old_ns} - ${ns}) / ${old_ns} * 100 > 5 }"; then
      status="improved"
    else
      status="stable"
    fi
    printf "| \`%s\` | %s ns | %s ns | %+.1f%% | %s |\n" \
      "${name#Benchmark}" "${old_ns}" "${ns}" "${pct}" "${status}"
  done < <(parse_benchmarks "${NEW_FILE}")

  echo ""
  if [ "${REGRESSIONS}" -gt 0 ]; then
    echo "**FAIL**: ${REGRESSIONS} benchmark(s) regressed by more than ${THRESHOLD_PCT}%"
    exit 1
  else
    echo "**PASS**: no regressions exceeding ${THRESHOLD_PCT}%"
  fi
fi

# ── Persist to history dir ────────────────────────────────────────────────────
if [ -n "${HISTORY_DIR}" ]; then
  mkdir -p "${HISTORY_DIR}"
  TS=$(date -u '+%Y%m%dT%H%M%SZ')
  DEST="${HISTORY_DIR}/bench-${TS}.txt"
  cp "${NEW_FILE}" "${DEST}"
fi
