// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

import "math"

// pathSS is the number of vertical sub-scanlines sampled per pixel row. The
// horizontal coverage within a row is computed analytically (exact span overlap),
// so this only sets the vertical anti-aliasing resolution. 4 gives 4 alpha
// levels vertically — enough for crisp icon edges without a heavy inner loop.
const pathSS = 4

// edge is one flattened line segment of a contour, in pixel units.
type edge struct{ x0, y0, x1, y1 float64 }

// crossing is where an edge cuts a scanline: the x coordinate and the winding
// direction (+1 for a downward edge, -1 for an upward one).
type crossing struct {
	x   float64
	dir int
}

// clampBox turns a float bounding box into an integer sub-box clamped to the
// [0,clampW) x [0,clampH) surface, returning ok=false when nothing of it is
// on-screen.
func clampBox(minX, minY, maxX, maxY float64, clampW, clampH int) (ox, oy, w, h int, ok bool) {
	ox = int(math.Floor(minX))
	oy = int(math.Floor(minY))
	x1 := int(math.Ceil(maxX))
	y1 := int(math.Ceil(maxY))
	if ox < 0 {
		ox = 0
	}
	if oy < 0 {
		oy = 0
	}
	if x1 > clampW {
		x1 = clampW
	}
	if y1 > clampH {
		y1 = clampH
	}
	w, h = x1-ox, y1-oy
	if w <= 0 || h <= 0 {
		return 0, 0, 0, 0, false
	}
	return ox, oy, w, h, true
}

// subBox intersects a float bounding box with the integer box [ox,oy,w,h],
// returning the clamped integer sub-box origin/extent and ok=false when the
// overlap is empty.
func subBox(minX, minY, maxX, maxY float64, ox, oy, w, h int) (sox, soy, sw, sh int, ok bool) {
	sox = int(math.Floor(minX))
	soy = int(math.Floor(minY))
	sx1 := int(math.Ceil(maxX))
	sy1 := int(math.Ceil(maxY))
	if sox < ox {
		sox = ox
	}
	if soy < oy {
		soy = oy
	}
	if sx1 > ox+w {
		sx1 = ox + w
	}
	if sy1 > oy+h {
		sy1 = oy + h
	}
	sw, sh = sx1-sox, sy1-soy
	if sw <= 0 || sh <= 0 {
		return 0, 0, 0, 0, false
	}
	return sox, soy, sw, sh, true
}

// maxSub merges the sw*sh coverage tile src into the dstW-wide accumulator dst
// at offset (dx, dy) by per-pixel maximum (a coverage union over a sub-region).
func maxSub(dst []float64, dstW int, src []float64, dx, dy, sw, sh int) {
	for j := 0; j < sh; j++ {
		drow := dst[(dy+j)*dstW+dx : (dy+j)*dstW+dx+sw]
		srow := src[j*sw : j*sw+sw]
		for i, v := range srow {
			if v > drow[i] {
				drow[i] = v
			}
		}
	}
}

// fillEdges builds the fill edge list from flattened contours: consecutive
// vertices plus an implicit closing edge per sub-path. Sub-paths with fewer than
// two points enclose no area and are dropped.
func fillEdges(subs []subpath) []edge {
	var e []edge
	for _, s := range subs {
		n := len(s.pts)
		if n < 2 {
			continue
		}
		for i := 0; i+1 < n; i++ {
			e = append(e, edge{s.pts[i].x, s.pts[i].y, s.pts[i+1].x, s.pts[i+1].y})
		}
		if last, first := s.pts[n-1], s.pts[0]; last != first {
			e = append(e, edge{last.x, last.y, first.x, first.y})
		}
	}
	return e
}

// coverGrid returns per-pixel coverage (0..1) of the region enclosed by edges
// under rule, over the integer box [ox,oy,w,h], sampling ss vertical
// sub-scanlines per row with analytic horizontal coverage. It allocates the
// grid; the hot render paths call coverInto with a reusable scratch instead.
func coverGrid(edges []edge, rule FillRule, ox, oy, w, h, ss int) []float64 {
	cov := make([]float64, w*h)
	coverInto(cov, edges, rule, ox, oy, w, h, ss, nil)
	return cov
}

// coverInto accumulates per-pixel coverage (0..1) of the region enclosed by
// edges under rule INTO cov (row-major w*h, caller-zeroed) over the integer box
// [ox,oy,w,h], sampling ss vertical sub-scanlines per row with analytic
// horizontal coverage. xs is a scratch crossings buffer whose grown backing is
// returned so a caller can reuse it across many calls (no per-call allocation).
func coverInto(cov []float64, edges []edge, rule FillRule, ox, oy, w, h, ss int, xs []crossing) []crossing {
	inv := 1.0 / float64(ss)
	oxf := float64(ox)
	for py := 0; py < h; py++ {
		row := cov[py*w : py*w+w]
		for s := 0; s < ss; s++ {
			sy := float64(oy+py) + (float64(s)+0.5)*inv
			xs = crossingsAt(edges, sy, xs[:0])
			if len(xs) < 2 {
				continue
			}
			sortCrossings(xs)
			wind := 0
			for i := 0; i < len(xs)-1; i++ {
				wind += xs[i].dir
				if insideRule(wind, rule) {
					addSpan(row, xs[i].x, xs[i+1].x, oxf, w, inv)
				}
			}
		}
	}
	return xs
}

// sortCrossings orders a scanline's crossings by ascending x with an in-place
// insertion sort. The list is short (a handful of edges cross any one scanline),
// so insertion sort beats sort.Slice and — unlike it — allocates nothing (no
// reflection, no escaping closure). Ties in x bound only a zero-width span, so
// the sort need not be stable for the coverage to stay byte-identical.
func sortCrossings(xs []crossing) {
	for i := 1; i < len(xs); i++ {
		c := xs[i]
		j := i - 1
		for j >= 0 && xs[j].x > c.x {
			xs[j+1] = xs[j]
			j--
		}
		xs[j+1] = c
	}
}

// crossingsAt collects the x-crossings of every edge with the horizontal line
// y=sy into dst (which the caller resets), using the half-open [ymin, ymax)
// convention so a vertex shared by two edges is counted exactly once.
func crossingsAt(edges []edge, sy float64, dst []crossing) []crossing {
	for _, e := range edges {
		if e.y0 == e.y1 {
			continue // horizontal edge never crosses a scanline
		}
		var dir int
		if e.y0 < e.y1 {
			if sy < e.y0 || sy >= e.y1 {
				continue
			}
			dir = 1
		} else {
			if sy < e.y1 || sy >= e.y0 {
				continue
			}
			dir = -1
		}
		t := (sy - e.y0) / (e.y1 - e.y0)
		dst = append(dst, crossing{x: e.x0 + t*(e.x1-e.x0), dir: dir})
	}
	return dst
}

// insideRule reports whether a winding count means "inside" under rule.
func insideRule(wind int, rule FillRule) bool {
	if rule == EvenOdd {
		return wind&1 != 0
	}
	return wind != 0
}

// addSpan adds the horizontal coverage of the inside interval [xa, xb] to row,
// scaled by weight. row[i] maps to pixel column ox+i; a pixel gets the length of
// its overlap with [xa, xb] (0..1), giving analytic horizontal anti-aliasing.
func addSpan(row []float64, xa, xb, ox float64, w int, weight float64) {
	if xa < ox {
		xa = ox
	}
	if hi := ox + float64(w); xb > hi {
		xb = hi
	}
	if xb <= xa {
		return
	}
	ixa := int(math.Floor(xa - ox))
	ixb := int(math.Ceil(xb - ox))
	for ix := ixa; ix < ixb; ix++ {
		left := xa
		if l := ox + float64(ix); l > left {
			left = l
		}
		right := xb
		if r := ox + float64(ix+1); r < right {
			right = r
		}
		if c := right - left; c > 0 {
			row[ix] += c * weight
		}
	}
}

// segRectEdges returns the four edges of the width-2*hw rectangle centred on the
// segment (x0,y0)->(x1,y1). A zero-length segment has no rectangle (nil): the
// vertex disk covers that spot.
func segRectEdges(x0, y0, x1, y1, hw float64) []edge {
	dx, dy := x1-x0, y1-y0
	l := math.Hypot(dx, dy)
	if l == 0 {
		return nil
	}
	nx, ny := -dy/l*hw, dx/l*hw
	ax, ay := x0+nx, y0+ny
	bx, by := x1+nx, y1+ny
	cx, cy := x1-nx, y1-ny
	ex, ey := x0-nx, y0-ny
	return []edge{{ax, ay, bx, by}, {bx, by, cx, cy}, {cx, cy, ex, ey}, {ex, ey, ax, ay}}
}

// diskMax unions (per-pixel MAX) a filled disk of radius r centred at (cx, cy)
// into cov, anti-aliasing the rim with the distance formula.
func diskMax(cov []float64, ox, oy, w, h int, cx, cy, r float64) {
	if r <= 0 {
		return
	}
	// Scan only the disk's clamped bounding box; pixels beyond r+0.5 from the
	// centre have coverage <= 0 and were skipped anyway, so the output is
	// unchanged while a big surrounding stroke box is no longer walked in full.
	i0, j0, i1, j1 := diskSpan(ox, oy, w, h, cx, cy, r)
	for j := j0; j < j1; j++ {
		py := float64(oy+j) + 0.5
		for i := i0; i < i1; i++ {
			px := float64(ox+i) + 0.5
			c := r + 0.5 - math.Hypot(px-cx, py-cy)
			if c <= 0 {
				continue
			}
			if c > 1 {
				c = 1
			}
			if k := j*w + i; c > cov[k] {
				cov[k] = c
			}
		}
	}
}

// diskSpan returns the half-open pixel index range [i0,i1)x[j0,j1) of the disk
// of radius r centred at (cx,cy), clamped to the box [ox,oy,w,h]. The +0.5
// margin matches diskMax's rim: any pixel whose centre is farther than r+0.5
// from the disk centre has zero coverage, so it need never be visited.
func diskSpan(ox, oy, w, h int, cx, cy, r float64) (i0, j0, i1, j1 int) {
	i0 = int(math.Floor(cx-r-0.5)) - ox
	i1 = int(math.Ceil(cx+r+0.5)) - ox
	j0 = int(math.Floor(cy-r-0.5)) - oy
	j1 = int(math.Ceil(cy+r+0.5)) - oy
	if i0 < 0 {
		i0 = 0
	}
	if j0 < 0 {
		j0 = 0
	}
	if i1 > w {
		i1 = w
	}
	if j1 > h {
		j1 = h
	}
	return
}

// edgeBounds returns the bounding box of an edge list (len >= 1 assumed).
func edgeBounds(edges []edge) (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, e := range edges {
		minX = math.Min(minX, math.Min(e.x0, e.x1))
		minY = math.Min(minY, math.Min(e.y0, e.y1))
		maxX = math.Max(maxX, math.Max(e.x0, e.x1))
		maxY = math.Max(maxY, math.Max(e.y0, e.y1))
	}
	return
}
