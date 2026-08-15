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
