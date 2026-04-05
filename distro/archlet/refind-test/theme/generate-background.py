#!/usr/bin/env python3
"""Generate cyberpunk Snazzy rEFInd background.

Layered composition: black base, perspective grid, matrix rain,
neon horizon glow, scanlines, vignette, particle bokeh.
"""

import argparse
import math
import random

from PIL import Image, ImageDraw, ImageFilter, ImageFont

# Snazzy palette
CYAN = (87, 199, 255)
MAGENTA = (255, 106, 193)
GREEN = (90, 247, 142)
BLACK = (0, 0, 0)
DIM = (104, 104, 104)

FONT_PATH = "/usr/share/fonts/MapleMono-NF-CN/MapleMono-NF-CN-Medium.ttf"

# Katakana + ASCII mix for matrix rain
MATRIX_CHARS = (
    "\u30a2\u30a4\u30a6\u30a8\u30aa\u30ab\u30ad\u30af\u30b1\u30b3"
    "\u30b5\u30b7\u30b9\u30bb\u30bd\u30bf\u30c1\u30c4\u30c6\u30c8"
    "\u30ca\u30cb\u30cc\u30cd\u30ce\u30cf\u30d2\u30d5\u30d8\u30db"
    "\u30de\u30df\u30e0\u30e1\u30e2\u30e4\u30e6\u30e8\u30e9\u30ea"
    "\u30eb\u30ec\u30ed\u30ef\u30f2\u30f3"
    "0123456789ABCDEF"
)


def layer_perspective_grid(w: int, h: int, rng: random.Random) -> Image.Image:
    """Tron-style receding perspective grid in dim cyan."""
    layer = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)

    # Vanishing point
    vx = w // 2 + rng.randint(-80, 80)
    vy = int(h * 0.42) + rng.randint(-20, 20)

    # Horizontal lines — denser near horizon, sparser at bottom
    alpha_base = 38
    for i in range(35):
        t = i / 34.0
        # Exponential spacing: lines cluster near horizon
        y = vy + int((h - vy) * (t ** 1.8))
        if y >= h:
            break
        alpha = int(alpha_base * (1.0 - t * 0.5))
        color = (*CYAN, alpha)
        draw.line([(0, y), (w, y)], fill=color, width=1)

    # Vertical lines radiating from vanishing point
    num_vlines = 32
    for i in range(num_vlines):
        t = (i - num_vlines // 2) / (num_vlines // 2)
        # Bottom edge x position
        bx = vx + int(t * w * 0.9)
        alpha = int(alpha_base * (1.0 - abs(t) * 0.3))
        color = (*CYAN, alpha)
        draw.line([(vx, vy), (bx, h)], fill=color, width=1)

    return layer


def layer_matrix_rain(w: int, h: int, rng: random.Random) -> Image.Image:
    """Frozen matrix rain columns in green, concentrated at edges."""
    layer = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)

    try:
        font = ImageFont.truetype(FONT_PATH, 16)
    except OSError:
        font = ImageFont.load_default()

    char_h = 20
    num_columns = rng.randint(35, 50)

    for _ in range(num_columns):
        # Bias columns toward left/right edges
        if rng.random() < 0.75:
            # Edge zone (left 25% or right 25%)
            if rng.random() < 0.5:
                x = rng.randint(0, int(w * 0.25))
            else:
                x = rng.randint(int(w * 0.75), w - 20)
        else:
            # Sparse in center
            x = rng.randint(int(w * 0.25), int(w * 0.75))

        col_len = rng.randint(8, 30)
        start_y = rng.randint(-col_len * char_h // 2, h - char_h * 3)

        for j in range(col_len):
            y = start_y + j * char_h
            if y < -char_h or y > h:
                continue

            # Gradient: bright head, dim tail
            head_pos = col_len - 1
            t = j / max(col_len - 1, 1)
            if j == head_pos:
                alpha = rng.randint(180, 255)
            elif j >= head_pos - 2:
                alpha = rng.randint(120, 180)
            else:
                alpha = int(50 + 80 * (1.0 - t))

            char = rng.choice(MATRIX_CHARS)
            color = (*GREEN, alpha)
            draw.text((x, y), char, fill=color, font=font)

    return layer


def layer_neon_glow(w: int, h: int, rng: random.Random) -> Image.Image:
    """Horizontal neon glow band — cyan center, magenta edges."""
    layer = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)

    # Central cyan glow
    cy = int(h * 0.40) + rng.randint(-30, 30)
    glow_w = int(w * 0.5)
    glow_h = int(h * 0.10)
    cx = w // 2 + rng.randint(-50, 50)
    draw.ellipse(
        [cx - glow_w, cy - glow_h, cx + glow_w, cy + glow_h],
        fill=(*CYAN, 50),
    )

    # Magenta accent glows at sides
    for offset_x in [-1, 1]:
        mx = cx + offset_x * int(w * 0.30) + rng.randint(-40, 40)
        my = cy + rng.randint(-20, 20)
        mg_w = int(w * 0.18)
        mg_h = int(h * 0.06)
        draw.ellipse(
            [mx - mg_w, my - mg_h, mx + mg_w, my + mg_h],
            fill=(*MAGENTA, 35),
        )

    # Heavy blur for soft glow
    layer = layer.filter(ImageFilter.GaussianBlur(radius=50))

    return layer


def layer_scanlines(w: int, h: int) -> Image.Image:
    """CRT scanline overlay — alternating dark lines."""
    layer = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)

    for y in range(0, h, 3):
        draw.line([(0, y), (w, y)], fill=(0, 0, 0, 12), width=1)

    return layer


def layer_vignette(w: int, h: int) -> Image.Image:
    """Radial darkening from center to corners."""
    layer = Image.new("RGBA", (w, h), (0, 0, 0, 0))

    cx, cy = w / 2, h / 2
    max_dist = math.sqrt(cx * cx + cy * cy)

    # Build vignette at quarter res for speed, then upscale
    qw, qh = w // 4, h // 4
    qcx, qcy = qw / 2, qh / 2
    q_max = math.sqrt(qcx * qcx + qcy * qcy)

    quarter = Image.new("RGBA", (qw, qh), (0, 0, 0, 0))
    pixels = quarter.load()

    for py in range(qh):
        for px in range(qw):
            dist = math.sqrt((px - qcx) ** 2 + (py - qcy) ** 2)
            t = dist / q_max
            # Smooth falloff starting at 40% from center
            alpha = int(min(255, max(0, (t - 0.4) * 1.6 * 120)))
            pixels[px, py] = (0, 0, 0, alpha)

    # Scale up with bilinear filtering for smooth result
    layer = quarter.resize((w, h), Image.BILINEAR)

    return layer


def layer_particles(w: int, h: int, rng: random.Random) -> Image.Image:
    """Scattered bokeh particles in cyan and magenta."""
    layer = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)

    num_particles = rng.randint(30, 55)
    for _ in range(num_particles):
        x = rng.randint(0, w)
        y = rng.randint(0, h)
        size = rng.randint(2, 5)
        color = rng.choice([CYAN, MAGENTA])
        alpha = rng.randint(60, 160)

        draw.ellipse(
            [x - size, y - size, x + size, y + size],
            fill=(*color, alpha),
        )

    # Blur for bokeh effect
    layer = layer.filter(ImageFilter.GaussianBlur(radius=4))

    return layer


def generate_background(width: int, height: int, seed: int, output: str):
    rng = random.Random(seed)

    print(f"  Generating {width}x{height} background (seed={seed})...")
    canvas = Image.new("RGBA", (width, height), (*BLACK, 255))

    # Layer composition
    print("    Layer: perspective grid")
    canvas = Image.alpha_composite(canvas, layer_perspective_grid(width, height, rng))

    print("    Layer: matrix rain")
    canvas = Image.alpha_composite(canvas, layer_matrix_rain(width, height, rng))

    print("    Layer: neon horizon glow")
    canvas = Image.alpha_composite(canvas, layer_neon_glow(width, height, rng))

    print("    Layer: scanlines")
    canvas = Image.alpha_composite(canvas, layer_scanlines(width, height))

    print("    Layer: vignette")
    canvas = Image.alpha_composite(canvas, layer_vignette(width, height))

    print("    Layer: particles")
    canvas = Image.alpha_composite(canvas, layer_particles(width, height, rng))

    # Convert to RGB (no alpha needed for final background)
    final = canvas.convert("RGB")
    final.save(output, "PNG", optimize=True)
    print(f"    Saved: {output}")


def main():
    parser = argparse.ArgumentParser(description="Generate cyberpunk rEFInd background")
    parser.add_argument("--width", type=int, default=2560)
    parser.add_argument("--height", type=int, default=1440)
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    generate_background(args.width, args.height, args.seed, args.output)


if __name__ == "__main__":
    main()
