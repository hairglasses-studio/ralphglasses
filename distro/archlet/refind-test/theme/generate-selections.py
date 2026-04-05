#!/usr/bin/env python3
"""Generate cyberpunk Snazzy selection highlights for rEFInd."""

import argparse

from PIL import Image, ImageDraw, ImageFilter

CYAN = (87, 199, 255)
MAGENTA = (255, 106, 193)


def generate_selection_big(output: str):
    """256x256 selection highlight with glowing border and HUD corner accents."""
    size = 256
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))

    # Glow layer — blurred version of the border
    glow = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    glow_draw = ImageDraw.Draw(glow)
    margin = 16
    glow_draw.rounded_rectangle(
        [margin, margin, size - margin, size - margin],
        radius=12,
        outline=(*CYAN, 80),
        width=4,
    )
    # Brighter bottom edge
    glow_draw.line(
        [(margin + 12, size - margin), (size - margin - 12, size - margin)],
        fill=(*CYAN, 140),
        width=5,
    )
    glow = glow.filter(ImageFilter.GaussianBlur(radius=10))

    # Sharp border layer
    border = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    border_draw = ImageDraw.Draw(border)
    border_draw.rounded_rectangle(
        [margin, margin, size - margin, size - margin],
        radius=12,
        outline=(*CYAN, 180),
        width=2,
    )
    # Thicker, brighter bottom edge (waybar underline motif)
    border_draw.line(
        [(margin + 12, size - margin), (size - margin - 12, size - margin)],
        fill=(*CYAN, 255),
        width=3,
    )

    # Corner accents — magenta targeting reticle brackets
    accent = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    accent_draw = ImageDraw.Draw(accent)
    bracket_len = 14
    m = margin - 2
    corners = [
        # Top-left
        [(m, m + bracket_len), (m, m), (m + bracket_len, m)],
        # Top-right
        [(size - m - bracket_len, m), (size - m, m), (size - m, m + bracket_len)],
        # Bottom-left
        [(m, size - m - bracket_len), (m, size - m), (m + bracket_len, size - m)],
        # Bottom-right
        [(size - m - bracket_len, size - m), (size - m, size - m), (size - m, size - m - bracket_len)],
    ]
    for pts in corners:
        accent_draw.line(pts, fill=(*MAGENTA, 160), width=2)

    # Composite: glow -> border -> accents
    img = Image.alpha_composite(img, glow)
    img = Image.alpha_composite(img, border)
    img = Image.alpha_composite(img, accent)

    img.save(output, "PNG", optimize=True)
    print(f"  Saved selection_big: {output}")


def generate_selection_small(output: str):
    """64x64 selection highlight — simplified glow border."""
    size = 64
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))

    # Glow layer
    glow = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    glow_draw = ImageDraw.Draw(glow)
    margin = 4
    glow_draw.rounded_rectangle(
        [margin, margin, size - margin, size - margin],
        radius=6,
        outline=(*CYAN, 80),
        width=2,
    )
    glow_draw.line(
        [(margin + 6, size - margin), (size - margin - 6, size - margin)],
        fill=(*CYAN, 120),
        width=3,
    )
    glow = glow.filter(ImageFilter.GaussianBlur(radius=4))

    # Sharp border
    border = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    border_draw = ImageDraw.Draw(border)
    border_draw.rounded_rectangle(
        [margin, margin, size - margin, size - margin],
        radius=6,
        outline=(*CYAN, 180),
        width=1,
    )
    border_draw.line(
        [(margin + 6, size - margin), (size - margin - 6, size - margin)],
        fill=(*CYAN, 255),
        width=2,
    )

    img = Image.alpha_composite(img, glow)
    img = Image.alpha_composite(img, border)

    img.save(output, "PNG", optimize=True)
    print(f"  Saved selection_small: {output}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output-big", required=True)
    parser.add_argument("--output-small", required=True)
    args = parser.parse_args()

    generate_selection_big(args.output_big)
    generate_selection_small(args.output_small)


if __name__ == "__main__":
    main()
