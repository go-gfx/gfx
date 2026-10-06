# gfx — pure-Go 2D graphics foundation

`github.com/go-gfx/gfx` is a pure-Go, **CGO=0** 2D graphics foundation: the shared
substrate that sits *below* the fleet's image-processing and rendering libraries.

It is **not** an image-processing library (that is [go-images](https://github.com/go-images/images),
the scikit-image analogue) and **not** a widget toolkit (that is
[go-widgets](https://github.com/go-widgets)). It is the common layer both stand on:
pixel buffers, colour science, high-quality resampling, image codecs, and vector
rasterization — each usable on its own, with no third-party dependencies.

## Packages

| Package | Purpose |
|---|---|
| `geometry` | points, rectangles, affine transforms |
| `color`    | full colour science: HSV/HSL/HWB, XYZ/Lab/LCh/Luv/OKLab/OKLCH, Y′CbCr, CMYK, D65↔D50 Bradford, ΔE (76/94/2000/OK), separable + non-separable blends, premultiplied alpha |
| `raster`   | shared pixel-buffer / pixel-format substrate |
| `resample` | high-quality resizing: box/area, bicubic (Catmull-Rom), Lanczos |
| `codec`    | image decoders/encoders: png, jpeg, webp, ico, icns, gif, tiff |
| `vector`   | paths, anti-aliased fill/stroke rasterizer (round joins & caps, non-zero / even-odd), linear/radial gradients, source-over compositing |
| `iso`      | isometric projection (project / unproject / depth-sort) and shaded pixel primitives — cube, brick, pyramid, slope, line, side — composited into a depth-ordered scene (the pure-Go obelisk.js) |

## Consumers

- **go-images** — image processing/analysis, built on `raster`/`color`/`resample`.
- **go-widgets/painter** — tri-backend renderer, built on `vector`/`raster`/`color`.
- **go-webengine** — HTML/CSS → image, built on `resample`/`codec`/`vector`.
- **go-widgets/desktop** — icon & thumbnail rendering.

## Status

Landed: **`geometry`** (float64 point / axis-aligned rect / 2-D affine matrix —
translate, scale, rotate, shear, compose, invert, transform point & rect),
**`raster`** (pixel substrate), **`color`** (a complete colour-science layer: premultiplied
alpha; the sRGB and generic-gamma transfer functions; the cylindrical sRGB models
HSV/HSL/HWB; the CIE hub XYZ with D65 **and** D50 white points and Bradford
D65↔D50 adaptation; CIE L\*a\*b\*/LCh, L\*u\*v\*/LCh(uv), Ottosson OKLab/OKLCH;
Y′CbCr BT.601/709 and naive CMYK; the ΔE colour-difference metrics ΔE76, ΔE94,
CIEDE2000 and ΔE-OK; the W3C separable **and** non-separable blend modes; and a
second, scikit-image-byte-compatible sRGB↔XYZ↔Lab regime so go-images can share
one colour library — all numerically checked against the colour-science oracle
and the published CIE / W3C / Sharma reference values), **`resample`** (nearest, bilinear, box/area, bicubic, Lanczos — each with
a premultiplied-alpha variant), and **`vector`** (2-D path builder +
anti-aliased scanline fill/stroke rasterizer, extracted verbatim from
go-widgets/painter and proven pixel-identical to it, now with linear/radial
gradients and source-over compositing of any paint through a coverage grid).
Colour-science and blend results match the published CIE / W3C reference values.
Pure-Go, CGO=0, 100% covered, six-arch CI, numeric parity with Pillow. See
[docs/perf.md](docs/perf.md).

## Known issue: loong64 with Go 1.27.0 or 1.27.1

[golang/go#81000](https://github.com/golang/go/issues/81000) is a loong64
code-generation fault that **this package's own test suite found**, and the
upstream report is this repository's `color` test run on real LoongArch
silicon (cfarm401), not under emulation. On loong64 the rule
`(Cvt64Fto32 x) => (TRUNCDW x)` leaves an integer result in a floating-point
register, so spilling it emits an 8-byte store; when the destination slot is
narrower, that store reaches the live stack slots beside it.

| | |
|---|---|
| affected | `GOARCH=loong64` built with **go1.27.0 or go1.27.1** |
| not affected | go1.26.x, and every other architecture |
| fixed in | Go **1.28** (`CL 820040`) |
| backport | [golang/go#81147](https://github.com/golang/go/issues/81147), still **open** for 1.27.2 — and no go1.27.2 is released |

`go.mod` declares a **floor**, so nothing stops you building this library with
an affected toolchain. If you target loong64, build with go1.26.x until the
backport lands.

**What we have measured, and what we have not.** Scanning the emitted loong64
code of every package here under go1.27.1 finds **64 of these conversions in
shipped code, one of them spilled, and that one into an 8-aligned 8-byte `int`
slot** — a store that fills its own slot and reaches no neighbour. The
corrupting shape appears only in two of our *test* frames, where a test's
register pressure spills a `uint8` onto consecutive bytes. So we know of no
library path that is wrong today.

That is a bound, not a guarantee: inlining and register allocation are decided
per build, so a program that compiles this library into a larger binary can
create pressure these frames do not have. The scan and its controls are
[`cmd/truncspill`](https://github.com/go-fleettools/fleettools) and the figures
are in [issue #13](https://github.com/go-gfx/gfx/issues/13).

Our own `loong64` CI lane is pinned to go1.26.4 for this reason, which is
written out at the pin in [`.github/workflows/ci.yml`](.github/workflows/ci.yml)
with the condition for removing it.

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-gfx authors.
