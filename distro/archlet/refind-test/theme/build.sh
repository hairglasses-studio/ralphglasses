#!/usr/bin/env bash
# build.sh — Generate all cyberpunk Snazzy rEFInd theme assets
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
source "$DIR/palette.sh"

mkdir -p "$DIR/build/icons"

echo "==> Generating backgrounds (3 variants)..."
for i in 1 2 3; do
    python3 "$DIR/generate-background.py" \
        --width 2560 --height 1440 --seed "$i" \
        --output "$DIR/build/background_0${i}.png"
done

echo "==> Generating icons..."
python3 "$DIR/generate-icons.py" --output-dir "$DIR/build/icons/"

echo "==> Generating selection highlights..."
python3 "$DIR/generate-selections.py" \
    --output-big "$DIR/build/selection_big.png" \
    --output-small "$DIR/build/selection_small.png"

echo "==> Generating font..."
python3 "$DIR/generate-font.py" \
    --font "/usr/share/fonts/MapleMono-NF-CN/MapleMono-NF-CN-Medium.ttf" \
    --size 18 \
    --output "$DIR/build/font_snazzy.png"

echo "==> Optimizing PNGs..."
for f in "$DIR/build/"*.png "$DIR/build/icons/"*.png; do
    magick "$f" -strip -define png:bit-depth=8 "$f"
done

echo "==> Picking random background variant..."
VARIANT=$(( RANDOM % 3 + 1 ))
cp "$DIR/build/background_0${VARIANT}.png" "$DIR/build/background.png"
echo "    Using variant $VARIANT"

echo "==> Copying theme.conf..."
cp "$DIR/theme.conf" "$DIR/build/theme.conf"

echo "==> Build complete. Asset sizes:"
du -sh "$DIR/build/"
echo "  Individual files:"
du -h "$DIR/build/"*.png "$DIR/build/icons/"*.png 2>/dev/null | sort -rh
