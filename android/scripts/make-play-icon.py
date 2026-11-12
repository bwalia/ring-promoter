#!/usr/bin/env python3
"""Render the Play Store icon (512x512) from the adaptive icon's geometry.

Play wants a 512x512 PNG, and the app's icon is a vector adaptive icon, so
there is no PNG anywhere in the build to upload. This draws the same picture
from the same numbers rather than screenshotting a launcher.

The geometry below is read straight out of:

    app/src/main/res/drawable/ic_launcher_foreground.xml
    app/src/main/res/values/ic_launcher_background.xml

If either changes, change it here too — this file is a second copy of those
numbers, which is the price of Play needing a raster. Run it and compare.

Why the crop: an adaptive icon is a 108dp canvas of which a launcher mask shows
only the centre 72dp. Uploading the whole 108dp canvas would put the store icon
at two-thirds the scale users see on their home screen, so this renders the
108dp canvas and crops to that centre 72dp — the store icon then matches the
launcher. Play applies its own rounded corners, so this stays a full-bleed
opaque square.

    python3 scripts/make-play-icon.py [output.png]
"""
import sys

from PIL import Image, ImageDraw

OUT = sys.argv[1] if len(sys.argv) > 1 else "store/play-icon-512.png"

VIEWPORT = 108          # the adaptive icon's canvas, in dp
VISIBLE = 72            # what a launcher mask actually shows
FINAL = 512             # what Play asks for
SS = 3072               # supersample canvas; 3072 -> crop 2048 -> 512 is exact

BACKGROUND = (0x16, 0x16, 0x2F)
GLOW = (0xFF, 0xCB, 0x6B)
ORBIT = (0xB9, 0xA6, 0x6A)
CORE_INNER = (0xFF, 0xF0, 0xC4)
CORE_OUTER = (0xFF, 0xC2, 0x4D)

# (centre_x, centre_y, radius) in viewport units. The vector writes these as
# two-arc circle paths: "M24,54 a30,30 0 1,0 60,0 ..." is centre (54,54) r=30.
ORBITS = [(54, 54, 30.0), (54, 54, 23.0), (54, 54, 16.5)]
ORBIT_WIDTH = 1.1
ORBIT_ALPHA = 0.85

GLOW_CENTRE = (54, 54, 24.0)        # radial, #59FFCB6B -> #1FFFCB6B -> #00
GLOW_STOPS = [(0.0, 0x59 / 255), (0.5, 0x1F / 255), (1.0, 0.0)]

CORE = (54, 54, 10.0)               # the lit core: live users
CORE_GRADIENT = (50, 50, 13.0)      # offset up-left, so the core reads as lit

# Applications riding their orbits, in the same colours the app uses for state.
DOTS = [
    (42.33, 42.33, 3.2, (0x8F, 0xC4, 0xF0)),   # in flight
    (71.20, 29.43, 3.4, (0xF4, 0xA4, 0x9B)),   # unhealthy
    (71.62, 68.78, 3.3, (0x9B, 0xE8, 0xAE)),   # healthy
]

S = SS / VIEWPORT                    # viewport units -> supersampled pixels


def px(v):
    return v * S


def lerp(a, b, t):
    return a + (b - a) * t


def alpha_at(distance_fraction):
    """Alpha along the glow's stops, linear between them like the vector's."""
    for (t0, a0), (t1, a1) in zip(GLOW_STOPS, GLOW_STOPS[1:]):
        if distance_fraction <= t1:
            span = t1 - t0
            return lerp(a0, a1, 0 if span == 0 else (distance_fraction - t0) / span)
    return GLOW_STOPS[-1][1]


def radial(size, colour_at, alpha_at_, spread=1.0):
    """A radial gradient drawn per pixel.

    Drawn small and upscaled: PIL has no gradient primitive, a stack of
    concentric circles composites its own overlaps and gets the alpha wrong,
    and a smooth radial gradient loses nothing to interpolation.

    `spread` is how many gradient radii the drawn box spans, so a gradient can
    be drawn into a box wider than itself. Past the gradient radius the colour
    clamps to the last stop, which is what Android's radial gradients do.
    """
    image = Image.new("RGBA", (size, size))
    half = size / 2
    pixels = []
    for y in range(size):
        dy = (y + 0.5 - half) / half
        for x in range(size):
            dx = (x + 0.5 - half) / half
            d = min(1.0, (dx * dx + dy * dy) ** 0.5 * spread)
            r, g, b = colour_at(d)
            pixels.append((r, g, b, round(255 * alpha_at_(d))))
    image.putdata(pixels)
    return image


def paste_radial(base, box_centre, box_radius, gradient, clip=None):
    """Composite a radial gradient onto the base, optionally clipped.

    `box_radius` must reach every pixel the clip circle can show, or the box's
    own corners cut the clipped shape — which is how a round core ends up an
    octagon.
    """
    bx, by = box_centre
    side = round(px(box_radius * 2))
    layer = Image.new("RGBA", base.size)
    layer.paste(
        gradient.resize((side, side), Image.LANCZOS),
        (round(px(bx) - side / 2), round(px(by) - side / 2)),
    )
    if clip is not None:
        ccx, ccy, cr = clip
        mask = Image.new("L", base.size, 0)
        ImageDraw.Draw(mask).ellipse(
            [px(ccx - cr), px(ccy - cr), px(ccx + cr), px(ccy + cr)], fill=255
        )
        layer.putalpha(Image.composite(layer.getchannel("A"), mask, mask))
    return Image.alpha_composite(base, layer)


def main():
    image = Image.new("RGBA", (SS, SS), BACKGROUND + (255,))

    # The glow behind the core.
    gx, gy, gr = GLOW_CENTRE
    image = paste_radial(
        image,
        (gx, gy),
        gr,
        radial(256, lambda d: GLOW, alpha_at),
    )

    # Three orbits: int outermost, prod closest to the core.
    orbits = Image.new("RGBA", (SS, SS))
    draw = ImageDraw.Draw(orbits)
    for cx, cy, r in ORBITS:
        draw.ellipse(
            [px(cx - r), px(cy - r), px(cx + r), px(cy + r)],
            outline=ORBIT + (round(255 * ORBIT_ALPHA),),
            width=max(1, round(px(ORBIT_WIDTH))),
        )
    image = Image.alpha_composite(image, orbits)

    # The core, lit from up-left by its offset gradient. The box has to reach
    # the far side of the core circle, which the offset puts further away than
    # the gradient's own radius.
    cgx, cgy, cgr = CORE_GRADIENT
    ccx, ccy, ccr = CORE
    reach = ((ccx - cgx) ** 2 + (ccy - cgy) ** 2) ** 0.5 + ccr
    image = paste_radial(
        image,
        (cgx, cgy),
        reach,
        radial(
            192,
            lambda d: tuple(round(lerp(a, b, d)) for a, b in zip(CORE_INNER, CORE_OUTER)),
            lambda d: 1.0,
            spread=reach / cgr,
        ),
        clip=CORE,
    )

    # Applications riding their orbits.
    dots = Image.new("RGBA", (SS, SS))
    draw = ImageDraw.Draw(dots)
    for cx, cy, r, colour in DOTS:
        draw.ellipse([px(cx - r), px(cy - r), px(cx + r), px(cy + r)], fill=colour + (255,))
    image = Image.alpha_composite(image, dots)

    # Crop to what a launcher shows, then down to what Play asks for.
    visible = round(SS * VISIBLE / VIEWPORT)
    inset = (SS - visible) // 2
    image = image.crop((inset, inset, inset + visible, inset + visible))
    image = image.resize((FINAL, FINAL), Image.LANCZOS)

    # Opaque: Play composites nothing behind it, and a transparent corner would
    # show as black in some surfaces.
    flat = Image.new("RGBA", image.size, BACKGROUND + (255,))
    Image.alpha_composite(flat, image).save(OUT, "PNG")
    print(f"wrote {OUT} ({FINAL}x{FINAL})")


if __name__ == "__main__":
    main()
