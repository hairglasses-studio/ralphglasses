#!/usr/bin/env bash
# validate-hyprland-conf.sh — Static linter for Hyprland config files.
#
# Usage:
#   bash distro/scripts/validate-hyprland-conf.sh [config-file]
#
# Returns 0 on success, 1 on any validation failure.
# Safe to run in CI — static analysis only, no running Hyprland needed.

set -euo pipefail

CONF="${1:-distro/hyprland/hyprland.conf}"
ERRORS=0

if [ ! -f "${CONF}" ]; then
  echo "ERROR: config file not found at ${CONF}" >&2
  exit 1
fi

echo "==> Validating ${CONF}"

# ── 1. Non-empty file ────────────────────────────────────────────────────────
if [ ! -s "${CONF}" ]; then
  echo "ERROR: config file is empty" >&2
  ERRORS=$((ERRORS + 1))
fi

# ── 2. Balanced braces ──────────────────────────────────────────────────────
OPEN=$(grep -c '{' "${CONF}" || true)
CLOSE=$(grep -c '}' "${CONF}" || true)
if [ "${OPEN}" -ne "${CLOSE}" ]; then
  echo "ERROR: unbalanced braces: ${OPEN} open vs ${CLOSE} close" >&2
  ERRORS=$((ERRORS + 1))
else
  echo "    Braces balanced: ${OPEN} pairs"
fi

# ── 3. Variable definitions ─────────────────────────────────────────────────
# Collect defined variables ($var = value)
mapfile -t DEFINED_VARS < <(grep -oP '^\$\w+' "${CONF}" | sort -u || true)
echo "    Variables defined: ${#DEFINED_VARS[@]}"

# Check that $variables used in bind/exec/monitor/workspace lines are defined.
# Skip lines that are comments.
USED_VARS=$(grep -v '^\s*#' "${CONF}" | grep -oP '\$\w+' | sort -u || true)
for var in ${USED_VARS}; do
  found=0
  for def in "${DEFINED_VARS[@]:-}"; do
    if [ "${var}" = "${def}" ]; then
      found=1
      break
    fi
  done
  if [ "${found}" -eq 0 ]; then
    echo "WARNING: variable ${var} used but not defined in config" >&2
  fi
done

# ── 4. bind lines have enough fields ────────────────────────────────────────
LINE_NUM=0
while IFS= read -r line; do
  LINE_NUM=$((LINE_NUM + 1))
  # Skip comments and empty lines
  [[ "${line}" =~ ^[[:space:]]*# ]] && continue
  [[ -z "${line// /}" ]] && continue

  # Match bind, binde, bindm directives
  if [[ "${line}" =~ ^(bind[em]?)[[:space:]]*= ]]; then
    # Count comma-separated fields after the =
    rhs="${line#*=}"
    IFS=',' read -ra fields <<< "${rhs}"
    if [ "${#fields[@]}" -lt 3 ]; then
      echo "ERROR: line ${LINE_NUM}: bind directive has ${#fields[@]} fields (need >= 3): ${line}" >&2
      ERRORS=$((ERRORS + 1))
    fi
  fi
done < "${CONF}"
echo "    Bind directives checked"

# ── 5. exec-once lines have a command ────────────────────────────────────────
LINE_NUM=0
while IFS= read -r line; do
  LINE_NUM=$((LINE_NUM + 1))
  [[ "${line}" =~ ^[[:space:]]*# ]] && continue
  if [[ "${line}" =~ ^exec-once[[:space:]]*=[[:space:]]*$ ]]; then
    echo "ERROR: line ${LINE_NUM}: exec-once with no command" >&2
    ERRORS=$((ERRORS + 1))
  fi
done < "${CONF}"
echo "    exec-once directives checked"

# ── 6. submap blocks closed ─────────────────────────────────────────────────
OPEN_SUBMAPS=0
LINE_NUM=0
while IFS= read -r line; do
  LINE_NUM=$((LINE_NUM + 1))
  [[ "${line}" =~ ^[[:space:]]*# ]] && continue
  if [[ "${line}" =~ ^submap[[:space:]]*=[[:space:]]*(.*) ]]; then
    name="${BASH_REMATCH[1]}"
    name="${name## }"  # trim leading space
    name="${name%% }"  # trim trailing space
    if [ "${name}" = "reset" ]; then
      OPEN_SUBMAPS=$((OPEN_SUBMAPS - 1))
      if [ "${OPEN_SUBMAPS}" -lt 0 ]; then
        echo "ERROR: line ${LINE_NUM}: submap = reset without matching submap open" >&2
        ERRORS=$((ERRORS + 1))
        OPEN_SUBMAPS=0
      fi
    else
      OPEN_SUBMAPS=$((OPEN_SUBMAPS + 1))
    fi
  fi
done < "${CONF}"
if [ "${OPEN_SUBMAPS}" -gt 0 ]; then
  echo "ERROR: ${OPEN_SUBMAPS} unclosed submap block(s)" >&2
  ERRORS=$((ERRORS + 1))
fi
echo "    Submap blocks checked"

# ── 7. No duplicate workspace-monitor assignments ───────────────────────────
DUPS=$(grep -E '^workspace\s*=' "${CONF}" | sed 's/workspace\s*=\s*//' | awk -F',' '{print $1}' | sort | uniq -d || true)
if [ -n "${DUPS}" ]; then
  echo "ERROR: duplicate workspace assignments: ${DUPS}" >&2
  ERRORS=$((ERRORS + 1))
else
  echo "    No duplicate workspace assignments"
fi

# ── 8. Section keywords followed by { ───────────────────────────────────────
LINE_NUM=0
while IFS= read -r line; do
  LINE_NUM=$((LINE_NUM + 1))
  [[ "${line}" =~ ^[[:space:]]*# ]] && continue
  # Match section keywords at start of line
  if [[ "${line}" =~ ^(general|decoration|input|animations|dwindle|master|misc|xwayland|gestures|binds|debug)[[:space:]]*$ ]]; then
    echo "ERROR: line ${LINE_NUM}: section keyword '${BASH_REMATCH[1]}' without opening brace" >&2
    ERRORS=$((ERRORS + 1))
  fi
done < "${CONF}"
echo "    Section keywords checked"

# ── Summary ──────────────────────────────────────────────────────────────────
if [ "${ERRORS}" -gt 0 ]; then
  echo "FAIL: ${ERRORS} error(s) found in ${CONF}" >&2
  exit 1
fi

echo "PASS: ${CONF} validated successfully"
