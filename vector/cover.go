// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

import (
	"math"
	"slices"
)

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

// A sweep is the working set of one coverage scan: the crossings of the
// sub-scanline being looked at, and the index that says which edges can cross
// it at all. A path of any size is walked once rather than once per scanline,
// which is what keeps a finely cut curve — tens of thousands of edges over a
// tall box — from taking the product of the two. It is kept on the
// [Rasterizer] so a stream of draws allocates nothing.
type sweep struct {
	xs     []crossing
	ymin   []float64 // per edge, the scanline it starts to matter on
	ymax   []float64 // per edge, the one it stops mattering on
	order  []int32   // edges by ascending ymin
	active []int32   // the ones the scan is inside, in no particular order
	next   int       // how far into order the scan has reached
}

// start indexes the edges for a scan that will ask for ascending scanlines.
// Horizontal edges cross no scanline at all and are left out entirely.
func (sc *sweep) start(edges []edge) {
	sc.ymin = grow(sc.ymin, len(edges))
	sc.ymax = grow(sc.ymax, len(edges))
	sc.order = sc.order[:0]
	sc.active = sc.active[:0]
	sc.next = 0
	for i, e := range edges {
		if e.y0 == e.y1 {
			continue
		}
		lo, hi := e.y0, e.y1
		if lo > hi {
			lo, hi = hi, lo
		}
		sc.ymin[i], sc.ymax[i] = lo, hi
		sc.order = append(sc.order, int32(i))
	}
	slices.SortFunc(sc.order, func(a, b int32) int {
		switch {
		case sc.ymin[a] < sc.ymin[b]:
			return -1
		case sc.ymin[a] > sc.ymin[b]:
			return 1
		}
		return 0
	})
}

// grow returns s resized to n, reusing its backing array when it is big enough.
func grow(s []float64, n int) []float64 {
	if cap(s) < n {
		return make([]float64, n)
	}
	return s[:n]
}

// crossings is where the edges cut the line y=sy. Successive calls must ask
// for ascending sy: the scan takes up each edge as it reaches it and drops it
// once past, so no edge is looked at on a scanline it cannot reach.
func (sc *sweep) crossings(edges []edge, sy float64) []crossing {
	for sc.next < len(sc.order) && sc.ymin[sc.order[sc.next]] <= sy {
		sc.active = append(sc.active, sc.order[sc.next])
		sc.next++
	}
	keep := sc.active[:0]
	for _, i := range sc.active {
		if sc.ymax[i] > sy {
			keep = append(keep, i)
		}
	}
	sc.active = keep
	sc.xs = sc.xs[:0]
	for _, i := range sc.active {
		// Every active edge has been reached and not yet passed, under the
		// half-open [ymin, ymax) convention that counts a vertex shared by two
		// edges exactly once.
		e := edges[i]
		dir := 1
		if e.y0 > e.y1 {
			dir = -1
		}
		t := (sy - e.y0) / (e.y1 - e.y0)
		sc.xs = append(sc.xs, crossing{x: e.x0 + t*(e.x1-e.x0), dir: dir})
	}
	return sc.xs
}

// coverInto accumulates per-pixel coverage (0..1) of the region enclosed by
// edges under rule INTO cov (row-major w*h, caller-zeroed) over the integer box
// [ox,oy,w,h], sampling ss vertical sub-scanlines per row with analytic
// horizontal coverage. sc is a scratch working set a caller reuses across many
// calls so nothing is allocated per call; nil asks for a throwaway one.
func coverInto(cov []float64, edges []edge, rule FillRule, ox, oy, w, h, ss int, sc *sweep) {
	if sc == nil {
		sc = &sweep{}
	}
	sc.start(edges)
	inv := 1.0 / float64(ss)
	oxf := float64(ox)
	for py := 0; py < h; py++ {
		row := cov[py*w : py*w+w]
		for s := 0; s < ss; s++ {
			sy := float64(oy+py) + (float64(s)+0.5)*inv
			xs := sc.crossings(edges, sy)
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
	first := int(math.Floor(xa - ox))
	last := int(math.Ceil(xb-ox)) - 1
	if first == last {
		// The span begins and ends inside one pixel, so that pixel's overlap
		// with it is the span itself.
		row[first] += (xb - xa) * weight
		return
	}
	// Only the two end pixels are partly covered. Every pixel between them is
	// covered from edge to edge, and its overlap is exactly one — the pixel
	// boundaries either side of it are whole numbers, which add to a
	// coordinate exactly, so their difference is exactly 1.0 and not merely
	// close to it. Working that out per pixel, through two comparisons and a
	// subtraction whose answer is always the same, was 42% of the time spent
	// drawing a page: a clip path covers a large area, and a large area is
	// almost entirely middle.
	//
	// The arithmetic at the ends is written the way the general form
	// evaluated it, right-to-left, so that the rounding is the same one and
	// the coverage comes out bit for bit as it did.
	row[first] += ((ox + float64(first+1)) - xa) * weight
	for ix := first + 1; ix < last; ix++ {
		row[ix] += weight
	}
	row[last] += (xb - (ox + float64(last))) * weight
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
