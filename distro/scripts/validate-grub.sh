#!/usr/bin/env bash
# validate-grub.sh — Parse all menuentry blocks in a grub.cfg and verify that
# referenced kernel image paths look well-formed.
#
# Usage:
#   ./distro/scripts/validate-grub.sh [grub.cfg]
#
# Returns 0 on success, 1 on any validation failure.
# This script is safe to run in CI without a built ISO — it only does static
# analysis of the grub.cfg source file.

set -euo pipefail

GRUB_CFG="${1:-distro/grub/grub.cfg}"

if [ ! -f "${GRUB_CFG}" ]; then
  echo "ERROR: grub.cfg not found at ${GRUB_CFG}" >&2
  exit 1
fi

echo "==> Validating ${GRUB_CFG}"

# ── 1. Parse menuentry names ──────────────────────────────────────────────────
mapfile -t ENTRIES < <(grep -E '^menuentry ' "${GRUB_CFG}" | sed 's/menuentry "\(.*\)" {/\1/')
if [ "${#ENTRIES[@]}" -eq 0 ]; then
  echo "ERROR: no menuentry blocks found in ${GRUB_CFG}" >&2
  exit 1
fi
echo "    Found ${#ENTRIES[@]} menuentry block(s):"
for e in "${ENTRIES[@]}"; do
  echo "      • ${e}"
done

# ── 2. Verify kernel image path patterns ─────────────────────────────────────
# We require every 'linux' line to reference a path that starts with /live/
# or an absolute path. This catches typos like missing leading slashes.
ERRORS=0
LINUX_LINES=$(grep -E '^\s+linux ' "${GRUB_CFG}" || true)
while IFS= read -r line; do
  # Extract the path (first word after 'linux ').
  path=$(echo "${line}" | awk '{print $2}')
  if [ -z "${path}" ]; then
    echo "  ERROR: linux line has no path: ${line}" >&2
    ERRORS=$((ERRORS + 1))
    continue
  fi
  if [[ "${path}" != /* ]]; then
    echo "  ERROR: kernel path '${path}' is not absolute (missing leading /)" >&2
    ERRORS=$((ERRORS + 1))
    continue
  fi
  echo "    kernel: ${path} — OK"
done <<< "${LINUX_LINES}"

# ── 3. Verify initrd path patterns ───────────────────────────────────────────
INITRD_LINES=$(grep -E '^\s+initrd ' "${GRUB_CFG}" || true)
while IFS= read -r line; do
  path=$(echo "${line}" | awk '{print $2}')
  if [ -z "${path}" ]; then
    echo "  ERROR: initrd line has no path: ${line}" >&2
    ERRORS=$((ERRORS + 1))
    continue
  fi
  if [[ "${path}" != /* ]]; then
    echo "  ERROR: initrd path '${path}' is not absolute (missing leading /)" >&2
    ERRORS=$((ERRORS + 1))
    continue
  fi
  echo "    initrd: ${path} — OK"
done <<< "${INITRD_LINES}"

# ── 4. Verify set timeout value is numeric ────────────────────────────────────
TIMEOUT_LINE=$(grep -E '^set timeout=' "${GRUB_CFG}" || true)
if [ -z "${TIMEOUT_LINE}" ]; then
  echo "  WARNING: no 'set timeout=' directive found — GRUB will use default" >&2
else
  TIMEOUT_VAL=$(echo "${TIMEOUT_LINE}" | sed 's/set timeout=//')
  if ! echo "${TIMEOUT_VAL}" | grep -qE '^[0-9]+$'; then
    echo "  ERROR: timeout value '${TIMEOUT_VAL}' is not a non-negative integer" >&2
    ERRORS=$((ERRORS + 1))
  else
    echo "    timeout: ${TIMEOUT_VAL}s — OK"
  fi
fi

# ── 5. Summary ────────────────────────────────────────────────────────────────
if [ "${ERRORS}" -gt 0 ]; then
  echo "==> FAILED: ${ERRORS} error(s) found in ${GRUB_CFG}" >&2
  exit 1
fi
echo "==> grub.cfg validation passed (${#ENTRIES[@]} entries, ${ERRORS} errors)"
