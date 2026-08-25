// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

// Rasterizer turns paths into per-pixel coverage grids. It owns reusable scratch
// buffers (the coverage accumulator, a per-stroke-segment temporary, and a
// scanline crossings list), grown on demand and reused across Fill / Stroke
// calls so a steady stream of vector draws amortises to ~zero allocation. The
// scratch carries no state between calls — each use re-zeroes / resets the region
// it touches — but a returned coverage slice aliases the accumulator, so a caller
// must consume it before the next Fill / Stroke.
//
// The zero Rasterizer is ready to use; it is not safe for concurrent use.
type Rasterizer struct {
	cov []float64 // coverage accumulator
	so  strokeOut // the pieces one stroke is made of
	sc  sweep     // the working set of one coverage scan
}

// covScratch returns cov resized to n float64s and zeroed, growing the backing
// array only when the current one is too small.
func (rz *Rasterizer) covScratch(n int) []float64 {
	if cap(rz.cov) < n {
		rz.cov = make([]float64, n)
	}
	s := rz.cov[:n]
	for i := range s {
		s[i] = 0
	}
	return s
}

// Fill rasterizes pth's interior under rule into a per-pixel coverage grid
// (row-major, values 0..1) over the integer box [ox,oy,w,h] clamped to the
// [0,clampW) x [0,clampH) surface. It returns ok=false — and an untouched grid —
// for a nil path, a path enclosing no area, or one whose bounds fall entirely
// off the clamp surface. Curves are flattened; corner and edge pixels get
// fractional coverage (anti-aliased). The returned grid aliases the Rasterizer's
// scratch and is valid only until the next Fill / Stroke.
func (rz *Rasterizer) Fill(pth *Path, rule FillRule, clampW, clampH int) (cov []float64, ox, oy, w, h int, ok bool) {
	if pth == nil {
		return nil, 0, 0, 0, 0, false
	}
	edges := fillEdges(pth.flatten(flattenTol))
	if len(edges) == 0 {
		return nil, 0, 0, 0, 0, false
	}
	minX, minY, maxX, maxY := edgeBounds(edges)
	ox, oy, w, h, ok = clampBox(minX, minY, maxX, maxY, clampW, clampH)
	if !ok {
		return nil, 0, 0, 0, 0, false
	}
	cov = rz.covScratch(w * h)
	coverInto(cov, edges, rule, ox, oy, w, h, pathSS, &rz.sc)
	return cov, ox, oy, w, h, true
}

// Stroke rasterizes pth's outline, width units wide with round joins and caps,
// into a per-pixel coverage grid (row-major, values 0..1) over the integer box
// [ox,oy,w,h] clamped to the [0,clampW) x [0,clampH) surface. The stroke is the
// union of a rectangle per segment and a disk (round join / cap) at every vertex;
// the union is taken as a per-pixel coverage MAX so overlapping pieces never
// double-darken. A closed sub-path strokes its closing segment too. It returns
// ok=false — and an untouched grid — for a nil path, width <= 0, a path with no
// strokeable segment, or one whose bounds fall entirely off the clamp surface.
// The returned grid aliases the Rasterizer's scratch and is valid only until the
// next Fill / Stroke.
