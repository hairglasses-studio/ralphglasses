#!/usr/bin/env python3
"""Generate rEFInd font strip from Maple Mono NF CN.

rEFInd font format: single PNG, 96 glyphs (ASCII 32-126 + fallback),
monospaced, width divisible by 96.
"""

import argparse

from PIL import Image, ImageDraw, ImageFont

FG_COLOR = (241, 241, 240, 255)  # Snazzy foreground


def generate_font(font_path: str, size: int, output: str):
    font = ImageFont.truetype(font_path, size)

    # Measure max glyph width across ASCII 32-126
    max_w = 0
    max_h = 0
    for code in range(32, 127):
        bbox = font.getbbox(chr(code))
        glyph_w = bbox[2] - bbox[0]
        glyph_h = bbox[3] - bbox[1]
        max_w = max(max_w, glyph_w)
        max_h = max(max_h, glyph_h)

    # Cell size — add 1px padding for safety
    cell_w = max_w + 2
    cell_h = max_h + 4

    # Total strip: 96 cells (95 ASCII chars + 1 fallback)
    strip_w = cell_w * 96
    strip_h = cell_h

    print(f"  Font: {font_path}")
    print(f"  Size: {size}px, cell: {cell_w}x{cell_h}, strip: {strip_w}x{strip_h}")

    img = Image.new("RGBA", (strip_w, strip_h), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    # Render ASCII 32 (space) through 126 (~)
    for i, code in enumerate(range(32, 127)):
        char = chr(code)
        bbox = font.getbbox(char)
        # Center glyph in cell
        x_offset = (cell_w - (bbox[2] - bbox[0])) // 2
        y_offset = (cell_h - (bbox[3] - bbox[1])) // 2 - bbox[1]
        x = i * cell_w + x_offset
        draw.text((x, y_offset), char, fill=FG_COLOR, font=font)

    # Glyph 96: fallback character (block)
    fallback = "\u2588"  # Full block
    bbox = font.getbbox(fallback)
    x_offset = (cell_w - (bbox[2] - bbox[0])) // 2
    y_offset = (cell_h - (bbox[3] - bbox[1])) // 2 - bbox[1]
    x = 95 * cell_w + x_offset
    draw.text((x, y_offset), fallback, fill=FG_COLOR, font=font)

    img.save(output, "PNG", optimize=True)
    print(f"  Saved: {output}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--font", default="/usr/share/fonts/MapleMono-NF-CN/MapleMono-NF-CN-Medium.ttf")
    parser.add_argument("--size", type=int, default=18)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    generate_font(args.font, args.size, args.output)


if __name__ == "__main__":
    main()
