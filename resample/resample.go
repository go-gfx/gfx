// Package resample resizes RGBA images with a choice of separable filters:
// nearest-neighbour, bilinear, box/area, bicubic (Catmull-Rom) and Lanczos
// (a=3). It is the shared resizer the go-gfx consumers — go-images, go-widgets,
// go-webengine, desktop icon/thumbnail rendering — build on instead of pulling
// in x/image/draw.
//
// Every mode has a straight variant (Resize) and a premultiplied-alpha variant
// (ResizePremultiplied). Resizing mixes neighbouring pixels, and mixing straight
// (non-premultiplied) colour lets a fully transparent pixel's arbitrary colour
// bleed into the visible edge of a cut-out — a dark or pale fringe.
// ResizePremultiplied multiplies colour by alpha before filtering and divides it
// back out afterwards, which removes the fringe; it is what you want whenever the
// source has transparency. The two coincide on a fully opaque image.
//
// The bicubic and Lanczos modes follow Pillow's convolution resampler: the
// filter footprint widens with the reduction factor, so they antialias a
// reduction as well as interpolate an enlargement. They match Pillow to a high
// PSNR (see docs/perf.md); the premultiplied variants match Pillow's
// premultiplied-alpha RGBA resize.
package resample

import (
	"fmt"

	"github.com/go-gfx/gfx/color"
	"github.com/go-gfx/gfx/raster"
)

// Mode selects the resampling filter.
type Mode int

const (
	// Nearest selects the source pixel nearest each destination pixel. Fast and
	// exact for integer enlargements, but blocky and aliasing.
	Nearest Mode = iota
	// Bilinear linearly interpolates the four nearest source pixels. Smoother
	// than Nearest, but only ever looks at four neighbours, so it aliases a heavy
	// reduction.
	Bilinear
	// Box averages the source region each destination pixel covers, weighting
	// every source pixel by how much of it falls inside — PIL's Image.BOX /
	// OpenCV's INTER_AREA / scikit-image's downscale_local_mean at integer ratios.
	// The plain antialiasing reduction filter; enlarging, it reduces to Nearest.
	Box
	// Bicubic resamples with the Keys cubic (a = -1/2, the Catmull-Rom spline),
	// a smooth four-tap kernel whose footprint widens with the reduction factor:
	// sharper than Bilinear when enlarging and a proper antialiasing low-pass when
	// reducing. Pillow's BICUBIC / x/image/draw's CatmullRom.
	Bicubic
	// Lanczos resamples with the a = 3 windowed-sinc kernel: wider and sharper
	// than Bicubic at more cost, the highest-fidelity mode either direction.
	// Pillow's LANCZOS / x/image/draw's Lanczos.
	Lanczos
)

// Resize returns src scaled to w by h pixels using the given mode, filtering
// each channel independently (straight alpha). It returns an error if w or h is
// not positive, or for an unknown mode.
func Resize(src *raster.Image, w, h int, mode Mode) (*raster.Image, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("resample: dimensions must be positive, got %dx%d", w, h)
	}
	dst := raster.New(w, h)
	switch mode {
	case Nearest:
		resizeNearest(dst.Pix, src.Pix, src.W, src.H, w, h)
	case Bilinear:
		resizeBilinear(dst.Pix, src.Pix, src.W, src.H, w, h)
	case Box:
		resizeArea(dst.Pix, src.Pix, src.W, src.H, w, h)
	case Bicubic:
		resampleFiltered(dst.Pix, src.Pix, src.W, src.H, w, h, cubicFilter, false)
	case Lanczos:
		resampleFiltered(dst.Pix, src.Pix, src.W, src.H, w, h, lanczosFilter, false)
	default:
		return nil, fmt.Errorf("resample: unknown mode %d", mode)
	}
	return dst, nil
}

// ResizePremultiplied is Resize with the colour channels filtered in
// premultiplied-alpha space, so a transparent pixel's colour does not bleed into
// the visible edge of a cut-out. On a fully opaque image it is identical to
// Resize.
//
// Bicubic and Lanczos premultiply in float64 inside the kernel (full precision);
// Bilinear and Box premultiply in the byte domain around their kernels; Nearest
// selects whole pixels and never blends, so it is unchanged. It returns an error
// if w or h is not positive, or for an unknown mode.
func ResizePremultiplied(src *raster.Image, w, h int, mode Mode) (*raster.Image, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("resample: dimensions must be positive, got %dx%d", w, h)
	}
	dst := raster.New(w, h)
	switch mode {
	case Nearest:
		resizeNearest(dst.Pix, src.Pix, src.W, src.H, w, h)
	case Bilinear:
		pm := color.Premultiply(src.Pix)
		resizeBilinear(dst.Pix, pm, src.W, src.H, w, h)
		color.UnpremultiplyInPlace(dst.Pix)
	case Box:
		pm := color.Premultiply(src.Pix)
		resizeArea(dst.Pix, pm, src.W, src.H, w, h)
		color.UnpremultiplyInPlace(dst.Pix)
	case Bicubic:
		resampleFiltered(dst.Pix, src.Pix, src.W, src.H, w, h, cubicFilter, true)
	case Lanczos:
		resampleFiltered(dst.Pix, src.Pix, src.W, src.H, w, h, lanczosFilter, true)
	default:
		return nil, fmt.Errorf("resample: unknown mode %d", mode)
	}
	return dst, nil
}
