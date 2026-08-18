// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "image/color"

// Shade returns c with its gamma-encoded sRGB R, G, B channels multiplied by
// factor and rounded to the nearest 8-bit value, clamping the result to
// [0, 255]; alpha is preserved unchanged. A factor below 1 darkens the colour
// and a factor above 1 lightens it; a negative factor is treated as 0 (black).
//
// This is the diffuse-shading primitive an isometric renderer uses to derive a
// solid's per-face colours from one base colour: the top face is drawn at
// factor 1, and the side faces at progressively smaller factors so a lit solid
// reads with depth. Multiplying the encoded channels (rather than adapting
// lightness in a perceptual space) reproduces the flat, banded look of the
// pixel-art isometric convention and is what makes the three faces of a cube
// distinguishable by a constant ratio.
func Shade(c color.RGBA, factor float64) color.RGBA {
	return color.RGBA{
		R: scaleChannel(c.R, factor),
		G: scaleChannel(c.G, factor),
		B: scaleChannel(c.B, factor),
		A: c.A,
	}
}

// scaleChannel multiplies the 8-bit channel v by factor, rounds to nearest, and
// clamps to [0, 255]. A non-positive factor yields 0.
func scaleChannel(v uint8, factor float64) uint8 {
	if factor <= 0 {
		return 0
	}
	s := float64(v)*factor + 0.5
	if s >= 255 {
		return 255
	}
	return uint8(s)
}
