#!/usr/bin/env bash
set -euo pipefail
# Validates GRUB configuration for ralphglasses thin client
GRUB_CFG="${1:-/boot/grub/grub.cfg}"

if [[ ! -f "$GRUB_CFG" ]]; then
  echo "GRUB config not found: $GRUB_CFG"
  exit 1
fi

# Check for required entries
errors=0
if ! grep -q "ralphglasses" "$GRUB_CFG" 2>/dev/null; then
  echo "WARNING: No ralphglasses boot entry found"
  ((errors++)) || true
fi
if ! grep -q "quiet splash" "$GRUB_CFG" 2>/dev/null; then
  echo "WARNING: Missing quiet splash parameters"
  ((errors++)) || true
fi
echo "GRUB validation complete: $errors warnings"
exit 0
