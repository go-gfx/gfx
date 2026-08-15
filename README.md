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
| `color`    | RGB/Lab/XYZ, sRGB↔linear, premultiplied alpha, blend modes |
| `raster`   | shared pixel-buffer / pixel-format substrate |
| `resample` | high-quality resizing: box/area, bicubic (Catmull-Rom), Lanczos |
| `codec`    | image decoders/encoders: png, jpeg, webp, ico, icns, gif, tiff |
| `vector`   | paths, anti-aliased fill/stroke rasterizer (round joins & caps, non-zero / even-odd), linear/radial gradients, source-over compositing |

## Consumers

- **go-images** — image processing/analysis, built on `raster`/`color`/`resample`.
- **go-widgets/painter** — tri-backend renderer, built on `vector`/`raster`/`color`.
- **go-webengine** — HTML/CSS → image, built on `resample`/`codec`/`vector`.
- **go-widgets/desktop** — icon & thumbnail rendering.

## Status

Landed: **`geometry`** (float64 point / axis-aligned rect / 2-D affine matrix —
translate, scale, rotate, shear, compose, invert, transform point & rect),
**`raster`** (pixel substrate), **`color`** (premultiplied alpha, the sRGB
transfer function, linear-RGB↔CIE XYZ↔CIE L\*a\*b\*, and the W3C separable blend
modes), **`resample`** (nearest, bilinear, box/area, bicubic, Lanczos — each with
a premultiplied-alpha variant), and **`vector`** (2-D path builder +
anti-aliased scanline fill/stroke rasterizer, extracted verbatim from
go-widgets/painter and proven pixel-identical to it, now with linear/radial
gradients and source-over compositing of any paint through a coverage grid).
Colour-science and blend results match the published CIE / W3C reference values.
Pure-Go, CGO=0, 100% covered, six-arch CI, numeric parity with Pillow. See
[docs/perf.md](docs/perf.md).

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-gfx authors.
