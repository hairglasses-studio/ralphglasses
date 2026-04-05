#!/usr/bin/env python3
"""Generate neon-glow rEFInd icons in Snazzy palette.

OS icons: cyan glow (256x256)
Tool icons: magenta glow (64x64)
"""

import argparse
import subprocess
import os

from PIL import Image, ImageDraw, ImageFilter, ImageOps

CYAN = (87, 199, 255)
MAGENTA = (255, 106, 193)
GREEN = (90, 247, 142)

# Source files
ICON_SOURCES = {
    # OS icons — SVG sources
    "os_manjaro": {
        "source": "/usr/share/icons/hicolor/scalable/apps/manjaro.svg",
        "type": "svg",
        "color": CYAN,
        "size": 256,
        "inner": 172,
    },
    "os_win": {
        "source": "/usr/share/refind/icons/svg/os_win.svg",
        "type": "svg",
        "color": CYAN,
        "size": 256,
        "inner": 160,
    },
    "os_arch": {
        "source": "/usr/share/refind/icons/os_arch.png",
        "type": "png",
        "color": CYAN,
        "size": 256,
        "inner": 172,
    },
    "os_linux": {
        "source": "/usr/share/refind/icons/os_linux.png",
        "type": "png",
        "color": CYAN,
        "size": 256,
        "inner": 160,
    },
    "os_unknown": {
        "source": "/usr/share/refind/icons/os_unknown.png",
        "type": "png",
        "color": CYAN,
        "size": 256,
        "inner": 160,
    },
    # Tool icons — PNG sources, magenta
    "func_shutdown": {
        "source": "/usr/share/refind/icons/func_shutdown.png",
        "type": "png",
        "color": MAGENTA,
        "size": 64,
        "inner": 44,
    },
    "func_reset": {
        "source": "/usr/share/refind/icons/func_reset.png",
        "type": "png",
        "color": MAGENTA,
        "size": 64,
        "inner": 44,
    },
    "func_firmware": {
        "source": "/usr/share/refind/icons/func_firmware.png",
        "type": "png",
        "color": MAGENTA,
        "size": 64,
        "inner": 44,
    },
    "func_hidden": {
        "source": "/usr/share/refind/icons/func_hidden.png",
        "type": "png",
        "color": MAGENTA,
        "size": 64,
        "inner": 44,
    },
}


def svg_to_png(svg_path: str, size: int, out_path: str):
    """Render SVG to PNG using ImageMagick, trimming whitespace."""
    subprocess.run(
        [
            "magick",
            "-background", "none",
            "-density", "300",
            svg_path,
            "-trim", "+repage",
            "-resize", f"{size}x{size}",
            "-gravity", "center",
            "-extent", f"{size}x{size}",
            out_path,
        ],
        check=True,
        capture_output=True,
    )


def load_and_resize(path: str, size: int) -> Image.Image:
    """Load an image and resize it, handling both SVG and PNG."""
    img = Image.open(path).convert("RGBA")
    img = img.resize((size, size), Image.LANCZOS)
    return img


def recolor_to_mono(img: Image.Image, color: tuple) -> Image.Image:
    """Recolor an RGBA image to a single color, preserving alpha."""
    r, g, b = color
    pixels = img.load()
    w, h = img.size
    result = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    result_pixels = result.load()

    for y in range(h):
        for x in range(w):
            _, _, _, a = pixels[x, y]
            if a > 0:
                result_pixels[x, y] = (r, g, b, a)

    return result


def add_neon_glow(icon: Image.Image, canvas_size: int, glow_radius: int, glow_alpha: float) -> Image.Image:
    """Place icon on transparent canvas with neon glow behind it."""
    canvas = Image.new("RGBA", (canvas_size, canvas_size), (0, 0, 0, 0))

    # Center icon on canvas
    offset_x = (canvas_size - icon.width) // 2
    offset_y = (canvas_size - icon.height) // 2

    # Create glow layer
    glow = Image.new("RGBA", (canvas_size, canvas_size), (0, 0, 0, 0))
    glow.paste(icon, (offset_x, offset_y), icon)

    # Scale alpha for glow intensity
    glow_data = glow.load()
    for y in range(canvas_size):
        for x in range(canvas_size):
            r, g, b, a = glow_data[x, y]
            glow_data[x, y] = (r, g, b, int(a * glow_alpha))

    glow = glow.filter(ImageFilter.GaussianBlur(radius=glow_radius))

    # Composite: glow behind icon
    canvas = Image.alpha_composite(canvas, glow)
    canvas.paste(icon, (offset_x, offset_y), icon)

    return canvas


def generate_icon(name: str, spec: dict, output_dir: str):
    source = spec["source"]
    color = spec["color"]
    final_size = spec["size"]
    inner_size = spec["inner"]

    if not os.path.exists(source):
        print(f"  SKIP {name}: source not found ({source})")
        return

    tmp_path = f"/tmp/refind-icon-{name}-tmp.png"
    out_path = os.path.join(output_dir, f"{name}.png")

    if spec["type"] == "svg":
        svg_to_png(source, inner_size, tmp_path)
        icon = load_and_resize(tmp_path, inner_size)
        os.unlink(tmp_path)
    else:
        icon = load_and_resize(source, inner_size)

    # Recolor to palette
    icon = recolor_to_mono(icon, color)

    # Add glow
    glow_radius = 18 if final_size == 256 else 8
    glow_alpha = 0.6 if final_size == 256 else 0.5
    result = add_neon_glow(icon, final_size, glow_radius, glow_alpha)

    result.save(out_path, "PNG", optimize=True)
    print(f"  Saved {name}: {out_path}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output-dir", required=True)
    args = parser.parse_args()

    os.makedirs(args.output_dir, exist_ok=True)

    for name, spec in ICON_SOURCES.items():
        generate_icon(name, spec, args.output_dir)


if __name__ == "__main__":
    main()
