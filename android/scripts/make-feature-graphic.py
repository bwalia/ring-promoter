#!/usr/bin/env python3
"""Draw the Play Store feature graphic (1024x500).

The same picture the app's own icon and its Rings screen draw: promotion rings
as orbits around a lit core. No device frames, no screenshots inside it, and
nothing invented — Play rejects a graphic that is mostly text, and a graphic
that does not look like the app is worse than none.

    python3 scripts/make-feature-graphic.py [output.png]
"""
import math
import sys

from PIL import Image, ImageDraw, ImageFont

W, H = 1024, 500
OUT = sys.argv[1] if len(sys.argv) > 1 else "store/feature-graphic.png"

SPACE = (7, 7, 10)
SPACE_FAR = (2, 3, 6)
RING = (255, 203, 107)
CORE = (255, 196, 84)
HEALTHY = (92, 204, 135)
IN_FLIGHT = (112, 173, 255)
UNHEALTHY = (255, 115, 107)
TEXT = (245, 246, 250)
MUTED = (150, 156, 170)

BOLD = "/System/Library/Fonts/Supplemental/Arial Bold.ttf"
REGULAR = "/System/Library/Fonts/Supplemental/Arial.ttf"


def vertical_gradient(size, top, bottom):
    """A gradient drawn a row at a time; PIL has no gradient primitive."""
    image = Image.new("RGB", size)
    draw = ImageDraw.Draw(image)
    for y in range(size[1]):
        t = y / max(1, size[1] - 1)
        draw.line(
            [(0, y), (size[0], y)],
            fill=tuple(round(a + (b - a) * t) for a, b in zip(top, bottom)),
        )
    return image


def main():
    # Supersample, then shrink: PIL will not antialias a drawn ellipse, and a
    # ragged orbit is exactly what the eye catches at this size.
    scale = 3
    image = vertical_gradient((W * scale, H * scale), SPACE, SPACE_FAR)
    draw = ImageDraw.Draw(image, "RGBA")

    cx, cy = 300 * scale, 250 * scale

    # Stars, from a fixed hash so the picture is the same every build.
    for i in range(70):
        hx = ((i * 9301 + 49297) % 233280) / 233280
        hy = ((i * 4591 + 17311) % 233280) / 233280
        hr = ((i * 7211 + 3571) % 233280) / 233280
        x, y = hx * W * scale, hy * H * scale
        r = (0.7 + hr * 1.3) * scale
        draw.ellipse([x - r, y - r, x + r, y + r], fill=(255, 255, 255, 40 + int(hr * 90)))

    # The orbits: four rings, the outermost faintest, as in the app.
    radii = [210, 165, 122, 82]
    for index, radius in enumerate(radii):
        r = radius * scale
        alpha = 70 + index * 30
        draw.ellipse(
            [cx - r, cy - r, cx + r, cy + r],
            outline=RING + (alpha,),
            width=max(1, round(1.6 * scale)),
        )

    # One app's journey inward: a node on each ring, newest solid.
    angles = [-0.6, 0.35, 1.25, 2.4]
    tints = [HEALTHY, IN_FLIGHT, HEALTHY, UNHEALTHY]
    for radius, angle, tint in zip(radii, angles, tints):
        r = radius * scale
        x = cx + math.cos(angle) * r
        y = cy + math.sin(angle) * r
        node = 7 * scale
        draw.ellipse([x - node, y - node, x + node, y + node], fill=tint + (255,))

    # The lit trail on one spoke, from the outer ring to the third.
    spoke = -0.6
    x0 = cx + math.cos(spoke) * radii[0] * scale
    y0 = cy + math.sin(spoke) * radii[0] * scale
    x1 = cx + math.cos(spoke) * radii[2] * scale
    y1 = cy + math.sin(spoke) * radii[2] * scale
    draw.line([x0, y0, x1, y1], fill=HEALTHY + (190,), width=max(2, round(3 * scale)))

    # Earth's stand-in: the lit core, glowing.
    for step in range(28, 0, -1):
        r = (26 + step * 2.2) * scale
        draw.ellipse(
            [cx - r, cy - r, cx + r, cy + r],
            fill=CORE + (max(1, 5 - step // 7),),
        )
    r = 26 * scale
    draw.ellipse([cx - r, cy - r, cx + r, cy + r], fill=CORE + (255,))

    image = image.resize((W, H), Image.LANCZOS)
    draw = ImageDraw.Draw(image)

    # Kept well inside the right edge: Play crops this graphic differently on
    # different surfaces, and a clipped product name is the one mistake that
    # cannot be explained away.
    title = ImageFont.truetype(BOLD, 56)
    body = ImageFont.truetype(REGULAR, 24)
    draw.text((552, 190), "Ring Promoter", font=title, fill=TEXT)
    draw.text((555, 266), "Promote through your rings,", font=body, fill=MUTED)
    draw.text((555, 300), "from the phone in your pocket.", font=body, fill=MUTED)

    image.save(OUT)
    print("wrote %s (%dx%d)" % (OUT, *image.size))


if __name__ == "__main__":
    main()
