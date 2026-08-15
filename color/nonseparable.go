// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// This file adds the four non-separable blend modes of the W3C Compositing and
// Blending Level 1 specification: Hue, Saturation, Color and Luminosity. Unlike
// the separable modes in blend.go, these cannot be computed one channel at a
// time — they mix an attribute (hue, chroma or luminosity) of one colour with
// the rest of the other, so each operates on a whole RGB triple. Inputs and
// outputs are gamma-encoded sRGB with each channel in [0, 1], per the spec.

// RGB is a gamma-encoded sRGB triple with each channel in [0, 1], the operand of
// the non-separable blend modes.
type RGB struct{ R, G, B float64 }

// lum is the W3C non-separable luminosity of an RGB colour.
func lum(c RGB) float64 { return 0.3*c.R + 0.59*c.G + 0.11*c.B }

// clipColor clips an out-of-range RGB colour back into [0, 1] while preserving
// its luminosity, per the W3C ClipColor procedure.
func clipColor(c RGB) RGB {
	l := lum(c)
	n := math.Min(c.R, math.Min(c.G, c.B))
	x := math.Max(c.R, math.Max(c.G, c.B))
	if n < 0 {
		c.R = l + (c.R-l)*l/(l-n)
		c.G = l + (c.G-l)*l/(l-n)
		c.B = l + (c.B-l)*l/(l-n)
	}
	if x > 1 {
		c.R = l + (c.R-l)*(1-l)/(x-l)
		c.G = l + (c.G-l)*(1-l)/(x-l)
		c.B = l + (c.B-l)*(1-l)/(x-l)
	}
	return c
}

// setLum returns c shifted to have luminosity l, then clipped to gamut.
func setLum(c RGB, l float64) RGB {
	d := l - lum(c)
	return clipColor(RGB{R: c.R + d, G: c.G + d, B: c.B + d})
}

// sat is the W3C non-separable saturation (channel range) of an RGB colour.
func sat(c RGB) float64 {
	return math.Max(c.R, math.Max(c.G, c.B)) - math.Min(c.R, math.Min(c.G, c.B))
}

// setSat returns c rescaled to have saturation s, following the W3C SetSat
// procedure on the (min, mid, max) ordered channels.
func setSat(c RGB, s float64) RGB {
	// Work on pointers to the three channels sorted by value so the result is
	// written back to the correct component.
	lo, mid, hi := &c.R, &c.G, &c.B
	if *lo > *mid {
		lo, mid = mid, lo
	}
	if *mid > *hi {
		mid, hi = hi, mid
	}
	if *lo > *mid {
		lo, mid = mid, lo
	}
	if *hi > *lo {
		*mid = (*mid - *lo) * s / (*hi - *lo)
		*hi = s
	} else {
		*mid = 0
		*hi = 0
	}
	*lo = 0
	return c
}

// BlendHue is the W3C Hue blend: the hue of the source with the saturation and
// luminosity of the backdrop.
func BlendHue(cb, cs RGB) RGB { return setLum(setSat(cs, sat(cb)), lum(cb)) }

// BlendSaturation is the W3C Saturation blend: the saturation of the source with
// the hue and luminosity of the backdrop.
func BlendSaturation(cb, cs RGB) RGB { return setLum(setSat(cb, sat(cs)), lum(cb)) }

// BlendColor is the W3C Color blend: the hue and saturation of the source with
// the luminosity of the backdrop.
func BlendColor(cb, cs RGB) RGB { return setLum(cs, lum(cb)) }

// BlendLuminosity is the W3C Luminosity blend: the luminosity of the source with
// the hue and saturation of the backdrop.
func BlendLuminosity(cb, cs RGB) RGB { return setLum(cb, lum(cs)) }
