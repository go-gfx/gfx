#!/usr/bin/env python3
"""Generate the Pillow resampling parity fixtures for the Go tests.

Everything here is SYNTHETIC and generated deterministically: a smooth RGB
gradient, a fine two-colour checker (high spatial frequency, to exercise the
antialiasing low-pass on a reduction), and a white opaque disk on a fully
transparent background (the cut-out whose edge straight-alpha resampling
fringes). No external or personal data is read.

For each fixture, mode (BICUBIC, LANCZOS) and target size, we emit Pillow's
native `Image.resize` output as raw row-major RGBA bytes (`.bin`), plus the raw
input, plus a manifest the Go test enumerates.

Pillow's RGBA resize is premultiplied-alpha aware: a transparent pixel's colour
does not bleed into the visible edge (verified — a transparent green background
leaves a red disk's edge pure red, and fully transparent output pixels keep zero
colour). So the reference maps onto the two entry points by fixture:

  * opaque fixtures (gradient, checker): alpha is 255 everywhere, so premultiplied
    and straight filtering coincide; the reference is compared to `Resize`.
  * transparent fixture (disk): the reference is Pillow's premultiplied result and
    is compared to `ResizePremultiplied`. `Resize` (straight, per-channel) is the
    naive path that fringes and is checked to DIFFER, in a separate property test.

Run:  python3 generate.py     (writes the .bin files and manifest.json here)

Pillow only (no numpy), so it runs anywhere Pillow does.
"""
import json
import os
import struct
import warnings

from PIL import Image

warnings.simplefilter("ignore")  # getdata() deprecation is irrelevant here.

HERE = os.path.dirname(os.path.abspath(__file__))
SIZE = 64                      # every input is SIZE x SIZE
TARGETS = [24, 100]            # one reduction, one enlargement
MODES = {"bicubic": Image.Resampling.BICUBIC, "lanczos": Image.Resampling.LANCZOS}


def gradient():
    """Smooth opaque RGB gradient."""
    px = []
    for y in range(SIZE):
        for x in range(SIZE):
            px.append((x * 255 // (SIZE - 1),
                       y * 255 // (SIZE - 1),
                       (x + y) * 255 // (2 * (SIZE - 1)),
                       255))
    return px


def checker():
    """Fine 3-pixel two-colour checker, opaque: high frequency for reduction."""
    px = []
    for y in range(SIZE):
        for x in range(SIZE):
            on = ((x // 3) + (y // 3)) % 2 == 0
            c = (240, 40, 20) if on else (20, 60, 210)
            px.append((c[0], c[1], c[2], 255))
    return px


def disk():
    """White opaque disk on a fully transparent (RGB 0) background."""
    cx = cy = (SIZE - 1) / 2.0
    r = SIZE * 0.34
    px = []
    for y in range(SIZE):
        for x in range(SIZE):
            if (x - cx) ** 2 + (y - cy) ** 2 <= r * r:
                px.append((255, 255, 255, 255))
            else:
                px.append((0, 0, 0, 0))
    return px


FIXTURES = {"gradient": (gradient, False), "checker": (checker, False), "disk": (disk, True)}


def main():
    manifest = []
    for name, (make, has_alpha) in FIXTURES.items():
        px = make()
        img = Image.new("RGBA", (SIZE, SIZE))
        img.putdata(px)
        with open(os.path.join(HERE, f"{name}_{SIZE}.bin"), "wb") as f:
            f.write(img.tobytes())
        # An opaque fixture's reference is compared to Resize (straight); the
        # transparent fixture's to ResizePremultiplied. The bytes are the same
        # Pillow native resize either way — only the entry point under test differs.
        kind = "premul" if has_alpha else "straight"
        for mode, resample in MODES.items():
            for t in TARGETS:
                ref = f"{name}_{mode}_{kind}_{t}x{t}.bin"
                with open(os.path.join(HERE, ref), "wb") as f:
                    f.write(img.resize((t, t), resample).tobytes())
                manifest.append({"input": f"{name}_{SIZE}.bin", "w": SIZE, "h": SIZE,
                                 "mode": mode, "premul": has_alpha, "dw": t, "dh": t, "ref": ref})
    with open(os.path.join(HERE, "manifest.json"), "w") as f:
        json.dump(manifest, f, indent=2, sort_keys=True)
        f.write("\n")
    print(f"wrote {len(manifest)} references + {len(FIXTURES)} inputs + manifest.json")


if __name__ == "__main__":
    main()
