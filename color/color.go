// Package color holds go-gfx's colour helpers. Its first concern is
// premultiplied alpha: filtering, blending or interpolating straight-alpha
// pixels lets a transparent pixel's (arbitrary) colour leak into the visible
// result — the dark or pale fringe around a resized cut-out. Multiplying each
// colour channel by its alpha first, operating, then dividing back out removes
// that leak. resample uses these for its Bilinear and Box premultiplied paths;
// the bicubic and Lanczos paths premultiply in float64 inside the kernel.
package color

// Premultiply returns a new slice of the straight-alpha RGBA pixels src with
// each colour channel multiplied by its alpha (rounded to nearest), alpha left
// unchanged. len(src) must be a multiple of four.
func Premultiply(src []uint8) []uint8 {
	out := make([]uint8, len(src))
	for i := 0; i < len(src); i += 4 {
		a := uint32(src[i+3])
		out[i] = uint8((uint32(src[i])*a + 127) / 255)
		out[i+1] = uint8((uint32(src[i+1])*a + 127) / 255)
		out[i+2] = uint8((uint32(src[i+2])*a + 127) / 255)
		out[i+3] = src[i+3]
	}
	return out
}

// UnpremultiplyInPlace divides the colour channels of premultiplied RGBA pixels
// back out of their alpha, in place, rounding to nearest and clamping to
// [0, 255]. A fully transparent pixel (alpha 0) is left black. It is the inverse
// of Premultiply up to rounding. len(pix) must be a multiple of four.
func UnpremultiplyInPlace(pix []uint8) {
	for i := 0; i < len(pix); i += 4 {
		a := uint32(pix[i+3])
		if a == 0 {
			pix[i], pix[i+1], pix[i+2] = 0, 0, 0
			continue
		}
		pix[i] = unpremul(uint32(pix[i]), a)
		pix[i+1] = unpremul(uint32(pix[i+1]), a)
		pix[i+2] = unpremul(uint32(pix[i+2]), a)
	}
}

// unpremul returns c*255/a rounded to nearest and clamped to 255, for a > 0.
func unpremul(c, a uint32) uint8 {
	v := (c*255 + a/2) / a
	if v > 255 {
		return 255
	}
	return uint8(v)
}
