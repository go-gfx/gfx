// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// This file provides the naive device CMYK model: a plain algebraic
// under-colour-removal of RGB, with no ICC profile, ink-limit or dot-gain
// modelling. It matches the transform in Go's standard image/color package and
// is adequate for previews and round-tripping, not for print colour management.

// CMYK is a naive device CMYK colour, each of cyan, magenta, yellow and the
// black key in [0, 1].
type CMYK struct{ C, M, Y, K float64 }

// SRGBToCMYK converts a gamma-encoded sRGB colour (each channel 0..1) to naive
// CMYK. Pure black maps to K == 1 with zero coloured ink.
func SRGBToCMYK(r, g, b float64) CMYK {
	k := 1 - math.Max(r, math.Max(g, b))
	if k == 1 {
		return CMYK{K: 1}
	}
	w := 1 - k
	return CMYK{
		C: (1 - r - k) / w,
		M: (1 - g - k) / w,
		Y: (1 - b - k) / w,
		K: k,
	}
}

// CMYKToSRGB converts a naive CMYK colour back to gamma-encoded sRGB, the
// inverse of [SRGBToCMYK] up to the black-recovery rounding.
func CMYKToSRGB(c CMYK) (r, g, b float64) {
	w := 1 - c.K
	return (1 - c.C) * w, (1 - c.M) * w, (1 - c.Y) * w
}

// webCoatedCorners are the sRGB values of the sixteen corners of the CMYK cube
// in U.S. Web Coated (SWOP), indexed by c<<3 | m<<2 | y<<1 | k.
//
// They are the printing primaries, not a fit to them: cyan is (0, 173, 239),
// magenta (236, 0, 140) and yellow (255, 242, 0), which are the SWOP process
// inks. Everything between the corners is multilinear interpolation, which is
// what poppler and Ghostscript both do; pdf.js instead fits a quadratic to the
// same table, and lands a mean of 11 levels away from it.
var webCoatedCorners = [16][3]float64{
	0b0000: {1, 1, 1},
	0b0001: {0.1373, 0.1216, 0.1255},
	0b0010: {1, 0.9490, 0},
	0b0011: {0.1098, 0.1020, 0},
	0b0100: {0.9255, 0, 0.5490},
	0b0101: {0.1412, 0, 0},
	0b0110: {0.9294, 0.1098, 0.1412},
	0b0111: {0.1333, 0, 0},
	0b1000: {0, 0.6784, 0.9373},
	0b1001: {0, 0.0588, 0.1412},
	0b1010: {0, 0.6510, 0.3137},
	0b1011: {0, 0.0745, 0},
	0b1100: {0.1804, 0.1922, 0.5725},
	0b1101: {0, 0, 0.0078},
	0b1110: {0.2118, 0.2119, 0.2235},
	0b1111: {0, 0, 0},
}

// CMYKToSRGBWebCoated converts CMYK to gamma-encoded sRGB through the U.S. Web
// Coated (SWOP) primaries, which is what a document rendered for the screen is
// asking for when it names DeviceCMYK.
//
// [CMYKToSRGB] is the naive model this file opens with, and it is the inverse
// of [SRGBToCMYK] rather than a rendering of ink. The difference between the
// two is not a rounding: over a 625-point grid of the cube the naive formula
// sits a mean of 27 levels and a maximum of 115 from this one. It is also the
// odd one out rather than the disagreeing one -- poppler and pdf.js, which
// approximate the SWOP table independently, agree with each other to a mean of
// 11 levels and with the naive formula to 27 and 28.
//
// Both belong here. Round-tripping a screen colour through ink and back wants
// the naive pair, and nothing else does.
func CMYKToSRGBWebCoated(v CMYK) (r, g, b float64) {
	c, m, y, k := clamp01(v.C), clamp01(v.M), clamp01(v.Y), clamp01(v.K)
	// The weight of a corner is the product of, for each ink, how much of that
	// ink the colour has if the corner has it and how much it lacks if not.
	for i, corner := range webCoatedCorners {
		w := 1.0
		for bit, ink := range [4]float64{c, m, y, k} {
			if i&(1<<(3-bit)) != 0 {
				w *= ink
			} else {
				w *= 1 - ink
			}
		}
		if w == 0 {
			continue
		}
		r += w * corner[0]
		g += w * corner[1]
		b += w * corner[2]
	}
	return r, g, b
}

// clamp01 keeps an ink amount inside the range the corners were measured over.
// A /Decode array may hand a PDF renderer a value outside it, and extrapolating
// from four corners is not a colour anybody printed.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
