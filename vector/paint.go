// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

import "github.com/go-gfx/gfx/raster"

// Composite lays a [Paint] onto dst through the coverage grid cov (row-major
// w*h, values 0..1) positioned at (ox, oy) on the surface, using straight-alpha
// source-over blending. This is the colour-bearing companion to
// [Rasterizer.Fill] / [Rasterizer.Stroke], which return exactly such a grid: the
// rasterizer decides SHAPE (per-pixel coverage) and Composite decides COLOUR
// (a flat [SolidPaint], a [LinearGradient] or a [RadialGradient]).
//
// The effective source alpha at a pixel is the paint's own alpha scaled by the
// pixel's coverage; a paint colour is sampled at the pixel centre. The box
// [ox, oy, w, h] is assumed to lie within dst's bounds — [Rasterizer.Fill] and
// [Rasterizer.Stroke] clamp it to the surface passed as clampW/clampH. Pixels
// with zero coverage are left untouched.
func Composite(dst *raster.Image, cov []float64, ox, oy, w, h int, p Paint) {
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			c := cov[j*w+i]
			if c <= 0 {
				continue
			}
			if c > 1 {
				c = 1
			}
			src := p.ColorAt(float64(ox+i)+0.5, float64(oy+j)+0.5)
			o := dst.PixOffset(ox+i, oy+j)
			over(dst.Pix[o:o+4], src.R, src.G, src.B, src.A, c)
		}
	}
}

// over blends the straight-alpha source (sr, sg, sb, sa), scaled by coverage cov
// in [0, 1], onto the straight-alpha destination pixel dst[0:4] in place, using
// the source-over operator.
func over(dst []uint8, sr, sg, sb, sa uint8, cov float64) {
	sA := float64(sa) / 255 * cov
	if sA <= 0 {
		return // nothing to lay down: destination is unchanged
	}
	dA := float64(dst[3]) / 255
	// With sA > 0, outA = sA + dA*(1-sA) >= sA > 0, so it is always safe to
	// divide the straight channels back out by it below.
	outA := sA + dA*(1-sA)
	inv := dA * (1 - sA)
	dst[0] = straight(float64(sr)*sA, float64(dst[0])*inv, outA)
	dst[1] = straight(float64(sg)*sA, float64(dst[1])*inv, outA)
	dst[2] = straight(float64(sb)*sA, float64(dst[2])*inv, outA)
	dst[3] = round8(outA * 255)
}

// straight recombines a premultiplied source term sPre and destination term dPre
// back to a straight-alpha 8-bit channel over the output alpha outA (> 0).
func straight(sPre, dPre, outA float64) uint8 {
	return round8((sPre + dPre) / outA)
}

// round8 rounds v to the nearest integer and clamps it to [0, 255].
func round8(v float64) uint8 {
	v += 0.5
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}
