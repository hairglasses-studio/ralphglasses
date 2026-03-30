#!/usr/bin/env bash
set -euo pipefail

# validate-grub-ci.sh — CI validation for GRUB bootloader configuration.
# Checks distro/grub/grub.cfg for structural correctness, required entries,
# and common misconfigurations.
#
# Usage: scripts/validate-grub-ci.sh [path/to/grub.cfg]
# Exit code 0 = all checks pass, 1 = one or more failures.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GRUB_CFG="${1:-${REPO_ROOT}/distro/grub/grub.cfg}"

errors=0
warnings=0

pass() { printf "  PASS  %s\n" "$1"; }
fail() { printf "  FAIL  %s\n" "$1"; ((errors++)) || true; }
warn() { printf "  WARN  %s\n" "$1"; ((warnings++)) || true; }
info() { printf "  INFO  %s\n" "$1"; }

echo "=== GRUB Configuration Validation ==="
echo "File: ${GRUB_CFG}"
echo ""

# ── 1. File existence and non-empty ──────────────────────────────────────────

if [[ ! -f "$GRUB_CFG" ]]; then
    fail "GRUB config not found: ${GRUB_CFG}"
    echo ""
    echo "RESULT: FAIL (${errors} error(s), ${warnings} warning(s))"
    exit 1
fi

if [[ ! -s "$GRUB_CFG" ]]; then
    fail "GRUB config is empty: ${GRUB_CFG}"
    echo ""
    echo "RESULT: FAIL (${errors} error(s), ${warnings} warning(s))"
    exit 1
fi

pass "grub.cfg exists and is non-empty"

# ── 2. menuentry blocks ─────────────────────────────────────────────────────

mapfile -t menuentries < <(grep -n '^menuentry ' "$GRUB_CFG" || true)
entry_count="${#menuentries[@]}"

if [[ "$entry_count" -eq 0 ]]; then
    fail "No menuentry blocks found"
else
    pass "Found ${entry_count} menuentry block(s)"
fi

# Extract labels for duplicate check
mapfile -t labels < <(grep '^menuentry ' "$GRUB_CFG" | sed 's/^menuentry "\([^"]*\)".*/\1/' || true)

# Check for duplicate labels
if [[ "${#labels[@]}" -gt 0 ]]; then
    dupes=$(printf '%s\n' "${labels[@]}" | sort | uniq -d)
    if [[ -n "$dupes" ]]; then
        fail "Duplicate menuentry labels found:"
        while IFS= read -r d; do
            info "  duplicate: \"${d}\""
        done <<< "$dupes"
    else
        pass "No duplicate menuentry labels"
    fi
fi

# Check that at least one entry contains "ralphglasses"
if printf '%s\n' "${labels[@]}" | grep -qi "ralphglasses"; then
    pass "At least one ralphglasses boot entry exists"
else
    fail "No menuentry label contains 'ralphglasses'"
fi

# ── 3. Kernel (linux) and initrd references ──────────────────────────────────

linux_count=$({ grep -E '^\s+linux\s' "$GRUB_CFG" || true; } | wc -l | tr -d ' ')
initrd_count=$({ grep -E '^\s+initrd\s' "$GRUB_CFG" || true; } | wc -l | tr -d ' ')

if [[ "$linux_count" -eq 0 ]]; then
    fail "No 'linux' (kernel) directives found"
else
    pass "Found ${linux_count} linux (kernel) directive(s)"
fi

if [[ "$initrd_count" -eq 0 ]]; then
    fail "No 'initrd' directives found"
else
    pass "Found ${initrd_count} initrd directive(s)"
fi

# Each menuentry with a linux directive should also have an initrd
# (except special entries like fwsetup)
in_entry=0
entry_label=""
has_linux=0
has_initrd=0
entry_errors=0

while IFS= read -r line; do
    if [[ "$line" =~ ^menuentry\ \" ]]; then
        # Check previous entry
        if [[ "$in_entry" -eq 1 && "$has_linux" -eq 1 && "$has_initrd" -eq 0 ]]; then
            fail "menuentry \"${entry_label}\" has linux directive but missing initrd"
            ((entry_errors++)) || true
        fi
        entry_label=$(echo "$line" | sed 's/^menuentry "\([^"]*\)".*/\1/')
        in_entry=1
        has_linux=0
        has_initrd=0
    fi
    if [[ "$line" =~ ^[[:space:]]+linux[[:space:]] ]]; then
        has_linux=1
    fi
    if [[ "$line" =~ ^[[:space:]]+initrd[[:space:]] ]]; then
        has_initrd=1
    fi
    if [[ "$line" == "}" ]]; then
        if [[ "$in_entry" -eq 1 && "$has_linux" -eq 1 && "$has_initrd" -eq 0 ]]; then
            fail "menuentry \"${entry_label}\" has linux directive but missing initrd"
            ((entry_errors++)) || true
        fi
        in_entry=0
    fi
done < "$GRUB_CFG"

if [[ "$entry_errors" -eq 0 && "$linux_count" -gt 0 ]]; then
    pass "All kernel entries have matching initrd"
fi

# ── 4. Kernel path validation ───────────────────────────────────────────────

# Kernel paths should be absolute (start with /)
bad_kernel_paths=$({ grep -E '^\s+linux\s' "$GRUB_CFG" || true; } | { grep -vE '^\s+linux\s+/' || true; } | wc -l | tr -d ' ')
if [[ "$bad_kernel_paths" -gt 0 ]]; then
    fail "Found kernel path(s) that are not absolute (must start with /)"
else
    pass "All kernel paths are absolute"
fi

# initrd paths should also be absolute
bad_initrd_paths=$({ grep -E '^\s+initrd\s' "$GRUB_CFG" || true; } | { grep -vE '^\s+initrd\s+/' || true; } | wc -l | tr -d ' ')
if [[ "$bad_initrd_paths" -gt 0 ]]; then
    fail "Found initrd path(s) that are not absolute (must start with /)"
else
    pass "All initrd paths are absolute"
fi

# ── 5. Kernel command line checks ───────────────────────────────────────────

# For live boot ISOs, we expect boot=live; for installed systems, root= is needed.
# This config is a live ISO, so check for boot= parameter.
if grep -qE '^\s+linux\s.*\bboot=' "$GRUB_CFG"; then
    pass "Kernel command line includes boot= parameter"
else
    # Not a live config — check for root= instead
    if grep -qE '^\s+linux\s.*\broot=' "$GRUB_CFG"; then
        pass "Kernel command line includes root= parameter"
    else
        warn "No boot= or root= parameter found in any kernel command line"
    fi
fi

# Check for dangerous or suspicious parameters
if grep -qE '^\s+linux\s.*\binit=/bin/sh\b' "$GRUB_CFG"; then
    warn "Found init=/bin/sh — this drops to a root shell (debug only)"
fi

if grep -qE '^\s+linux\s.*\bsingle\b' "$GRUB_CFG"; then
    warn "Found 'single' mode entry — ensure this is intentional"
fi

# Ensure systemd target is set where expected
systemd_entries=$({ grep -E '^\s+linux\s.*systemd\.unit=' "$GRUB_CFG" || true; } | wc -l | tr -d ' ')
if [[ "$systemd_entries" -gt 0 ]]; then
    pass "Found ${systemd_entries} entry/entries with explicit systemd.unit target"
fi

# ── 6. Timeout and default values ───────────────────────────────────────────

timeout_val=$(grep -E '^set\s+timeout=' "$GRUB_CFG" | head -1 | sed 's/.*timeout=//' || true)
if [[ -z "$timeout_val" ]]; then
    warn "No 'set timeout' directive found"
else
    if [[ "$timeout_val" =~ ^[0-9]+$ ]]; then
        if [[ "$timeout_val" -lt 0 || "$timeout_val" -gt 60 ]]; then
            warn "Timeout value ${timeout_val} seems unusual (expected 0-60)"
        else
            pass "Timeout set to ${timeout_val}s (reasonable)"
        fi
    else
        fail "Timeout value '${timeout_val}' is not a valid integer"
    fi
fi

default_val=$(grep -E '^set\s+default=' "$GRUB_CFG" | head -1 | sed 's/.*default=//' || true)
if [[ -z "$default_val" ]]; then
    warn "No 'set default' directive found"
else
    pass "Default entry set to '${default_val}'"
fi

# ── 7. Balanced braces ──────────────────────────────────────────────────────

open_braces=$({ grep '{' "$GRUB_CFG" || true; } | wc -l | tr -d ' ')
close_braces=$({ grep '}' "$GRUB_CFG" || true; } | wc -l | tr -d ' ')

if [[ "$open_braces" -ne "$close_braces" ]]; then
    fail "Unbalanced braces: ${open_braces} opening vs ${close_braces} closing"
else
    pass "Braces are balanced (${open_braces} pairs)"
fi

# ── 8. No trailing whitespace on linux/initrd lines (avoids subtle bugs) ───

trailing_ws=$({ grep -nE '^\s+(linux|initrd)\s.*\s$' "$GRUB_CFG" || true; } | wc -l | tr -d ' ')
if [[ "$trailing_ws" -gt 0 ]]; then
    warn "Found ${trailing_ws} linux/initrd line(s) with trailing whitespace"
else
    pass "No trailing whitespace on kernel/initrd lines"
fi

# ── Summary ──────────────────────────────────────────────────────────────────

echo ""
if [[ "$errors" -eq 0 ]]; then
    echo "RESULT: PASS (${warnings} warning(s))"
    exit 0
else
    echo "RESULT: FAIL (${errors} error(s), ${warnings} warning(s))"
    exit 1
fi
