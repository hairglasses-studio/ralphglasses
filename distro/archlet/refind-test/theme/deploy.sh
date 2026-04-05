#!/usr/bin/env bash
# deploy.sh — Deploy built theme to dotfiles + ESP, optionally preview in QEMU
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD="$DIR/build"
DOTFILES_THEME="$HOME/hairglasses-studio/dotfiles/refind/themes/matrix"
ESP_THEME="/boot/efi/EFI/refind/themes/matrix"
EMU="$DIR/../refind-emu.sh"

# Build if needed
if [[ ! -f "$BUILD/background.png" ]]; then
    echo "==> No build output found, running build.sh..."
    bash "$DIR/build.sh"
fi

# Pick a fresh random background variant
VARIANT=$(( RANDOM % 3 + 1 ))
cp "$BUILD/background_0${VARIANT}.png" "$BUILD/background.png"
echo "==> Using background variant $VARIANT"

# Sync to dotfiles repo
echo "==> Syncing to dotfiles..."
mkdir -p "$DOTFILES_THEME/icons"
cp "$BUILD/background.png" "$DOTFILES_THEME/"
cp "$BUILD/selection_big.png" "$DOTFILES_THEME/"
cp "$BUILD/selection_small.png" "$DOTFILES_THEME/"
cp "$BUILD/font_snazzy.png" "$DOTFILES_THEME/"
cp "$BUILD/icons/"*.png "$DOTFILES_THEME/icons/"
cp "$BUILD/theme.conf" "$DOTFILES_THEME/"

# Deploy to live ESP
if mountpoint -q /boot/efi; then
    echo "==> Deploying to ESP..."
    sudo mkdir -p "$ESP_THEME/icons"
    sudo cp "$BUILD/background.png" "$ESP_THEME/"
    sudo cp "$BUILD/selection_big.png" "$ESP_THEME/"
    sudo cp "$BUILD/selection_small.png" "$ESP_THEME/"
    sudo cp "$BUILD/font_snazzy.png" "$ESP_THEME/"
    sudo cp "$BUILD/icons/"*.png "$ESP_THEME/icons/"
    sudo cp "$BUILD/theme.conf" "$ESP_THEME/"
    echo "==> ESP updated"
else
    echo "==> ESP not mounted, skipping"
fi

# Optional modes
case "${1:-}" in
    --preview)
        echo "==> Launching QEMU preview..."
        bash "$EMU" --refresh
        ;;
    --screenshot)
        echo "==> Taking QEMU screenshot..."
        bash "$EMU" --refresh --screenshot "/tmp/refind-theme-preview.png"
        echo "==> Preview: /tmp/refind-theme-preview.png"
        ;;
    *)
        echo "==> Done. Use --preview or --screenshot to test in QEMU."
        ;;
esac
