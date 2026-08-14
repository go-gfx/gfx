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
	cov []float64  // coverage accumulator
	tmp []float64  // per-stroke-segment temporary
	xs  []crossing // one scanline's edge crossings
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

// tmpScratch returns tmp resized to n float64s and zeroed (see covScratch).
func (rz *Rasterizer) tmpScratch(n int) []float64 {
	if cap(rz.tmp) < n {
		rz.tmp = make([]float64, n)
	}
	s := rz.tmp[:n]
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
	rz.xs = coverInto(cov, edges, rule, ox, oy, w, h, pathSS, rz.xs[:0])
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
func (rz *Rasterizer) Stroke(pth *Path, width float64, clampW, clampH int) (cov []float64, ox, oy, w, h int, ok bool) {
	if pth == nil || width <= 0 {
		return nil, 0, 0, 0, 0, false
	}
	hw := width / 2
	var segs []edge
	var verts []point
	for _, s := range pth.flatten(flattenTol) {
		n := len(s.pts)
		if n < 2 {
			continue // a lone point is not strokeable
		}
		for i := 0; i+1 < n; i++ {
			segs = append(segs, edge{s.pts[i].x, s.pts[i].y, s.pts[i+1].x, s.pts[i+1].y})
		}
		if s.closed && s.pts[n-1] != s.pts[0] {
			segs = append(segs, edge{s.pts[n-1].x, s.pts[n-1].y, s.pts[0].x, s.pts[0].y})
		}
		verts = append(verts, s.pts...)
	}
	if len(verts) == 0 {
		return nil, 0, 0, 0, 0, false
	}
	minX, minY, maxX, maxY := pointBounds(verts)
	ox, oy, w, h, ok = clampBox(minX-hw, minY-hw, maxX+hw, maxY+hw, clampW, clampH)
	if !ok {
		return nil, 0, 0, 0, 0, false
	}
	cov = rz.covScratch(w * h)
	for _, s := range segs {
		re := segRectEdges(s.x0, s.y0, s.x1, s.y1, hw)
		if re == nil {
			continue
		}
		// Rasterise this segment's rectangle over its OWN clamped sub-box (a
		// small fraction of the whole stroke box) into a reusable temp, then
		// union it into the accumulator by per-pixel MAX. Coverage is computed
		// from absolute pixel positions, so a sub-box gives values identical to
		// the full box; pixels outside it contribute zero to the MAX.
		rminX, rminY, rmaxX, rmaxY := edgeBounds(re)
		sox, soy, sw, sh, ok := subBox(rminX, rminY, rmaxX, rmaxY, ox, oy, w, h)
		if !ok {
			continue
		}
		tmp := rz.tmpScratch(sw * sh)
		rz.xs = coverInto(tmp, re, NonZero, sox, soy, sw, sh, pathSS, rz.xs[:0])
		maxSub(cov, w, tmp, sox-ox, soy-oy, sw, sh)
	}
	for _, v := range verts {
		diskMax(cov, ox, oy, w, h, v.x, v.y, hw)
	}
	return cov, ox, oy, w, h, true
}
