// Copyright (c) 2026 the go-gfx/gfx authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

// A Rect is an axis-aligned rectangle whose coverage is worked out rather than
// rasterised.
//
// It exists because a consumer that knows its shape is a rectangle before it
// draws one -- a PDF renderer meets `re W n`, and 91% of the clips in two
// corpora of 3215 documents are exactly that -- pays for a coverage grid it
// could have computed: the edge list, the sweep, and w*h floats to store a
// shape four numbers describe.
//
// What [Rect.At] returns is what [Rasterizer.Fill] would have put in that
// grid, BIT FOR BIT, which is what makes it a substitution rather than an
// approximation. That is not free to arrange: the fill samples pathSS vertical
// sub-scanlines a row at y+(s+0.5)/pathSS and adds a weight for each one that
// is inside, with the horizontal overlap worked out the way addSpan works it
// out. The closed form has to sample the same places and do the same
// arithmetic; TestRectAgreesWithFill holds that down, and catches a
// sub-scanline moved by half a step or an edge compared the other way round.
//
// The weight is ACCUMULATED rather than multiplied by the count. At pathSS = 4
// the two agree for every rectangle that test tries -- a sum of at most four
// equal doubles is rounded once, as the product is -- so this is not what
// keeps it exact today. It is written as the rasteriser writes it so that it
// still matches if pathSS ever stops being small.
//
// It lives beside the rasteriser it has to agree with, so that a change to one
// is made in sight of the other.
type Rect struct {
	X0, Y0, X1, Y1 float64
}

// Box is the integer box [Rasterizer.Fill] would return for this rectangle,
// clamped to the [0,clampW) x [0,clampH) surface, and ok=false when nothing of
// it is on it. It is the same rule the rasteriser uses, called rather than
// restated.
func (r Rect) Box(clampW, clampH int) (ox, oy, w, h int, ok bool) {
	if r.X1 <= r.X0 || r.Y1 <= r.Y0 {
		return 0, 0, 0, 0, false
	}
	return clampBox(r.X0, r.Y0, r.X1, r.Y1, clampW, clampH)
}

// At is how much of pixel (px,py) the rectangle covers, 0..1.
func (r Rect) At(px, py int) float64 {
	const weight = 1.0 / pathSS
	inv := 1.0 / float64(pathSS)
	ox := float64(px)
	// The horizontal overlap is the same for every sub-scanline, but it is
	// added once per sub-scanline that is inside rather than multiplied by
	// their number, because that is how the rasteriser reaches its value.
	xa, xb := r.X0, r.X1
	if xa < ox {
		xa = ox
	}
	if hi := ox + 1; xb > hi {
		xb = hi
	}
	if xb <= xa {
		return 0
	}
	// addSpan splits a span into a first pixel, whole middle pixels and a
	// last one; here the span has been cut to ONE pixel, so it is always the
	// case addSpan writes as first == last -- the overlap is the span itself.
	// A whole pixel comes out of it as 1*weight, which is the same value
	// addSpan adds for a middle pixel.
	span := (xb - xa) * weight
	acc := 0.0
	for s := 0; s < pathSS; s++ {
		sy := float64(py) + (float64(s)+0.5)*inv
		if sy < r.Y0 || sy >= r.Y1 {
			continue
		}
		acc += span
	}
	return acc
}
