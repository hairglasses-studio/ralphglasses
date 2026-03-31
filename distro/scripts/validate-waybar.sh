#!/usr/bin/env bash
# validate-waybar.sh — Static validator for Waybar JSONC config files.
#
# Usage:
#   bash distro/scripts/validate-waybar.sh [config.jsonc]
#
# Returns 0 on success, 1 on any validation failure.
# Requires: jq

set -euo pipefail

CONF="${1:-distro/hyprland/waybar/config.jsonc}"
ERRORS=0

if [ ! -f "${CONF}" ]; then
  echo "ERROR: waybar config not found at ${CONF}" >&2
  exit 1
fi

echo "==> Validating ${CONF}"

# ── 1. Strip JSONC comments and validate JSON ───────────────────────────────
# Remove // line comments and /* block comments */, then validate with jq.
STRIPPED=$(sed 's|//.*$||' "${CONF}" | sed ':a;N;$!ba;s|/\*[^*]*\*/||g')
if ! echo "${STRIPPED}" | jq . > /dev/null 2>&1; then
  echo "ERROR: invalid JSON after stripping comments" >&2
  ERRORS=$((ERRORS + 1))
else
  echo "    Valid JSON"
fi

# ── 2. Extract all referenced module names ───────────────────────────────────
# Modules appear in modules-left, modules-center, modules-right arrays.
MODULES=$(echo "${STRIPPED}" | jq -r '
  [."modules-left"[]?, ."modules-center"[]?, ."modules-right"[]?] | unique | .[]
' 2>/dev/null || true)

if [ -z "${MODULES}" ]; then
  echo "WARNING: no modules found in config" >&2
else
  MODULE_COUNT=$(echo "${MODULES}" | wc -l)
  echo "    Modules referenced: ${MODULE_COUNT}"
fi

# ── 3. Check each referenced module has a config block ───────────────────────
for mod in ${MODULES}; do
  # Built-in modules like "clock", "cpu" don't always need explicit config.
  # But custom/* modules must have their block.
  if [[ "${mod}" == custom/* ]]; then
    HAS_BLOCK=$(echo "${STRIPPED}" | jq --arg m "${mod}" 'has($m)' 2>/dev/null || echo "false")
    if [ "${HAS_BLOCK}" != "true" ]; then
      echo "ERROR: custom module '${mod}' referenced but has no config block" >&2
      ERRORS=$((ERRORS + 1))
    fi
  fi
done
echo "    Custom module config blocks checked"

# ── 4. Verify custom/* exec paths are absolute ──────────────────────────────
for mod in ${MODULES}; do
  if [[ "${mod}" == custom/* ]]; then
    EXEC=$(echo "${STRIPPED}" | jq -r --arg m "${mod}" '.[$m].exec // empty' 2>/dev/null || true)
    if [ -n "${EXEC}" ]; then
      # Extract the first word (the command)
      CMD=$(echo "${EXEC}" | awk '{print $1}')
      if [[ "${CMD}" != /* ]]; then
        echo "WARNING: custom module '${mod}' exec command '${CMD}' is not an absolute path" >&2
      fi
    fi
  fi
done
echo "    Custom module exec paths checked"

# ── 5. Note Hyprland module requirements ─────────────────────────────────────
HYPR_MODS=$(echo "${MODULES}" | grep '^hyprland/' || true)
if [ -n "${HYPR_MODS}" ]; then
  echo "    Hyprland-specific modules detected:"
  for mod in ${HYPR_MODS}; do
    echo "      - ${mod} (requires Waybar built with Hyprland IPC support)"
  done
fi

# ── 6. Check for mixed compositor modules (likely mistake) ───────────────────
SWAY_MODS=$(echo "${MODULES}" | grep '^sway/' || true)
HYPR_MODS_LIST=$(echo "${MODULES}" | grep '^hyprland/' || true)
if [ -n "${SWAY_MODS}" ] && [ -n "${HYPR_MODS_LIST}" ]; then
  echo "ERROR: both Sway and Hyprland modules found in same config (pick one):" >&2
  for mod in ${SWAY_MODS} ${HYPR_MODS_LIST}; do
    echo "  - ${mod}" >&2
  done
  ERRORS=$((ERRORS + 1))
fi

# ── Summary ──────────────────────────────────────────────────────────────────
if [ "${ERRORS}" -gt 0 ]; then
  echo "FAIL: ${ERRORS} error(s) found in ${CONF}" >&2
  exit 1
fi

echo "PASS: ${CONF} validated successfully"
