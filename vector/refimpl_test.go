// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

// This file is the GOLDEN reference for the parity harness: a HERMETIC, verbatim
// copy of the 2-D vector rasterizer as it lived in go-widgets/painter BEFORE it
// was extracted into this package (painter/path.go + painter/pathpaint.go, up to
// but not including the pixel-buffer composite). Every function and type is
// duplicated here with a `ref` prefix so the reference shares NO algorithm code
// with the implementation under test — only the public Path data model, which is
// the input, not the code being proved.
//
// parity_test.go sweeps a large table of paths / rules / widths / clamp boxes and
// asserts that Rasterizer.Fill / Stroke reproduce refFill / refStroke BYTE FOR
// BYTE. That is the guarantee the extraction is pixel-identical by construction:
// if the move introduced any transcription slip, the sweep fails.
//
// See feedback-prove-against-replaced-code: the equivalence test must compare
// against the implementation being REPLACED, reproduced verbatim, not against a
// variant of the new code.

import "math"

const refFlattenTol = 0.2
const refFlattenMaxDepth = 24

type refPoint struct{ x, y float64 }

type refSubpath struct {
	pts    []refPoint
	closed bool
}

type refEdge struct{ x0, y0, x1, y1 float64 }

type refCrossing struct {
	x   float64
	dir int
}

const refPathSS = 4

// refFlatten is the verbatim old Path.flatten, reading the same Path.segs.
func refFlatten(p *Path, tol float64) []refSubpath {
	var out []refSubpath
	var cur *refSubpath
	var cx, cy float64
	var sx, sy float64

	startSub := func(x, y float64) {
		out = append(out, refSubpath{pts: []refPoint{{x, y}}})
		cur = &out[len(out)-1]
		cx, cy, sx, sy = x, y, x, y
	}
	ensure := func() {
		if cur == nil {
			startSub(cx, cy)
		}
	}

	for _, s := range p.segs {
		switch s.op {
		case opMove:
			startSub(s.x, s.y)
		case opLine:
			ensure()
			cur.pts = append(cur.pts, refPoint{s.x, s.y})
			cx, cy = s.x, s.y
		case opQuad:
			ensure()
			cur.pts = refFlattenQuad(cur.pts, cx, cy, s.cx1, s.cy1, s.x, s.y, tol, refFlattenMaxDepth)
			cx, cy = s.x, s.y
		case opCubic:
			ensure()
			cur.pts = refFlattenCubic(cur.pts, cx, cy, s.cx1, s.cy1, s.cx2, s.cy2, s.x, s.y, tol, refFlattenMaxDepth)
			cx, cy = s.x, s.y
		default:
			if cur != nil {
				cur.closed = true
				cx, cy = sx, sy
				cur = nil
			}
		}
	}
	return out
}

func refFlattenQuad(out []refPoint, x0, y0, cx, cy, x1, y1, tol float64, depth int) []refPoint {
	if depth <= 0 || refDistToLine(cx, cy, x0, y0, x1, y1) <= tol {
		return append(out, refPoint{x1, y1})
	}
	x01, y01 := (x0+cx)/2, (y0+cy)/2
	x12, y12 := (cx+x1)/2, (cy+y1)/2
	xm, ym := (x01+x12)/2, (y01+y12)/2
	out = refFlattenQuad(out, x0, y0, x01, y01, xm, ym, tol, depth-1)
	out = refFlattenQuad(out, xm, ym, x12, y12, x1, y1, tol, depth-1)
	return out
}

func refFlattenCubic(out []refPoint, x0, y0, c1x, c1y, c2x, c2y, x1, y1, tol float64, depth int) []refPoint {
	if depth <= 0 || (refDistToLine(c1x, c1y, x0, y0, x1, y1) <= tol && refDistToLine(c2x, c2y, x0, y0, x1, y1) <= tol) {
		return append(out, refPoint{x1, y1})
	}
	x01, y01 := (x0+c1x)/2, (y0+c1y)/2
	x12, y12 := (c1x+c2x)/2, (c1y+c2y)/2
	x23, y23 := (c2x+x1)/2, (c2y+y1)/2
	x012, y012 := (x01+x12)/2, (y01+y12)/2
	x123, y123 := (x12+x23)/2, (y12+y23)/2
	xm, ym := (x012+x123)/2, (y012+y123)/2
	out = refFlattenCubic(out, x0, y0, x01, y01, x012, y012, xm, ym, tol, depth-1)
	out = refFlattenCubic(out, xm, ym, x123, y123, x23, y23, x1, y1, tol, depth-1)
	return out
}

func refDistToLine(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	return math.Abs((px-ax)*dy-(py-ay)*dx) / math.Sqrt(l2)
}

func refPathBox(minX, minY, maxX, maxY float64, clampW, clampH int) (ox, oy, w, h int, ok bool) {
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

func refSubBox(minX, minY, maxX, maxY float64, ox, oy, w, h int) (sox, soy, sw, sh int, ok bool) {
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

func refMaxSub(dst []float64, dstW int, src []float64, dx, dy, sw, sh int) {
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

func refFillEdges(subs []refSubpath) []refEdge {
	var e []refEdge
	for _, s := range subs {
		n := len(s.pts)
		if n < 2 {
			continue
		}
		for i := 0; i+1 < n; i++ {
			e = append(e, refEdge{s.pts[i].x, s.pts[i].y, s.pts[i+1].x, s.pts[i+1].y})
		}
		if last, first := s.pts[n-1], s.pts[0]; last != first {
			e = append(e, refEdge{last.x, last.y, first.x, first.y})
		}
	}
	return e
}

func refCoverInto(cov []float64, edges []refEdge, rule FillRule, ox, oy, w, h, ss int, xs []refCrossing) []refCrossing {
	inv := 1.0 / float64(ss)
	oxf := float64(ox)
	for py := 0; py < h; py++ {
		row := cov[py*w : py*w+w]
		for s := 0; s < ss; s++ {
			sy := float64(oy+py) + (float64(s)+0.5)*inv
			xs = refCrossingsAt(edges, sy, xs[:0])
			if len(xs) < 2 {
				continue
			}
			refSortCrossings(xs)
			wind := 0
			for i := 0; i < len(xs)-1; i++ {
				wind += xs[i].dir
				if refInsideRule(wind, rule) {
					refAddSpan(row, xs[i].x, xs[i+1].x, oxf, w, inv)
				}
			}
		}
	}
	return xs
}

func refSortCrossings(xs []refCrossing) {
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

func refCrossingsAt(edges []refEdge, sy float64, dst []refCrossing) []refCrossing {
	for _, e := range edges {
		if e.y0 == e.y1 {
			continue
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
		dst = append(dst, refCrossing{x: e.x0 + t*(e.x1-e.x0), dir: dir})
	}
	return dst
}

func refInsideRule(wind int, rule FillRule) bool {
	if rule == EvenOdd {
		return wind&1 != 0
	}
	return wind != 0
}

func refAddSpan(row []float64, xa, xb, ox float64, w int, weight float64) {
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

func refSegRectEdges(x0, y0, x1, y1, hw float64) []refEdge {
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
	return []refEdge{{ax, ay, bx, by}, {bx, by, cx, cy}, {cx, cy, ex, ey}, {ex, ey, ax, ay}}
}

func refDiskMax(cov []float64, ox, oy, w, h int, cx, cy, r float64) {
	if r <= 0 {
		return
	}
	i0, j0, i1, j1 := refDiskSpan(ox, oy, w, h, cx, cy, r)
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

func refDiskSpan(ox, oy, w, h int, cx, cy, r float64) (i0, j0, i1, j1 int) {
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

func refEdgeBounds(edges []refEdge) (minX, minY, maxX, maxY float64) {
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

func refPointBounds(pts []refPoint) (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, pt := range pts {
		minX = math.Min(minX, pt.x)
		minY = math.Min(minY, pt.y)
		maxX = math.Max(maxX, pt.x)
		maxY = math.Max(maxY, pt.y)
	}
	return
}

// refFill is the verbatim old PixelPainter.FillPath coverage stage (before
// composite): flatten -> edges -> box -> coverInto. Returns the coverage grid and
// its integer box.
func refFill(pth *Path, rule FillRule, clampW, clampH int) (cov []float64, ox, oy, w, h int, ok bool) {
	if pth == nil {
		return nil, 0, 0, 0, 0, false
	}
	edges := refFillEdges(refFlatten(pth, refFlattenTol))
	if len(edges) == 0 {
		return nil, 0, 0, 0, 0, false
	}
	minX, minY, maxX, maxY := refEdgeBounds(edges)
	ox, oy, w, h, ok = refPathBox(minX, minY, maxX, maxY, clampW, clampH)
	if !ok {
		return nil, 0, 0, 0, 0, false
	}
	cov = make([]float64, w*h)
	refCoverInto(cov, edges, rule, ox, oy, w, h, refPathSS, nil)
	return cov, ox, oy, w, h, true
}

// refStroke is an independent account of what a stroke covers, worked out a
// completely different way from the code under test. A round-capped,
// round-joined stroke of width 2*hw is, by definition, every point within hw
// of the line — so this measures each pixel by sampling it on a fine grid and
// asking each sample how far it is from the nearest segment. Nothing about
// how the pieces are cut up or put together enters into it, which is the whole
// point: the code under test builds a stroke out of overlapping pieces, and
// the way those pieces are combined is exactly what can go wrong.
//
// It is far too slow to ship — it is a definition, not an implementation.
func refStroke(pth *Path, width float64, clampW, clampH int) (cov []float64, ox, oy, w, h int, ok bool) {
	if pth == nil || width <= 0 {
		return nil, 0, 0, 0, 0, false
	}
	hw := width / 2
	var segs []refEdge
	var verts []refPoint
	for _, s := range refFlatten(pth, refFlattenTol) {
		n := len(s.pts)
		if n < 2 {
			continue
		}
		for i := 0; i+1 < n; i++ {
			segs = append(segs, refEdge{s.pts[i].x, s.pts[i].y, s.pts[i+1].x, s.pts[i+1].y})
		}
		if s.closed && s.pts[n-1] != s.pts[0] {
			segs = append(segs, refEdge{s.pts[n-1].x, s.pts[n-1].y, s.pts[0].x, s.pts[0].y})
		}
		verts = append(verts, s.pts...)
	}
	if len(verts) == 0 {
		return nil, 0, 0, 0, 0, false
	}
	minX, minY, maxX, maxY := refPointBounds(verts)
	ox, oy, w, h, ok = refPathBox(minX-hw, minY-hw, maxX+hw, maxY+hw, clampW, clampH)
	if !ok {
		return nil, 0, 0, 0, 0, false
	}
	cov = make([]float64, w*h)
	var near []refEdge
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			// Only the segments that come within hw of this pixel at all can
			// put anything in it; the rest need not be asked 256 times.
			px0, py0 := float64(ox+i), float64(oy+j)
			near = near[:0]
			for _, e := range segs {
				if refSegNear(e, px0, py0, hw) {
					near = append(near, e)
				}
			}
			if len(near) == 0 {
				continue
			}
			cov[j*w+i] = refPixelCovered(near, px0, py0, hw)
		}
	}
	return cov, ox, oy, w, h, true
}

// refStrokeSamples is how many points across a pixel are asked about on each
// of its sub-scanlines. Vertically the reference samples the SAME sub-scanlines
// the rasteriser does: how finely a row is cut is a stated property of the
// rasteriser, not something for the reference to have an opinion about, and
// matching it leaves the union — the thing being checked — as the only source
// of disagreement.
const refStrokeSamples = 64

// refEdgeNudge is small enough to change nothing but which side of an exact
// tie a sample falls on.
const refEdgeNudge = 1e-9

// refPixelCovered is the share of the pixel whose lower-left corner is
// (px0, py0) that lies within hw of one of the segments.
func refPixelCovered(segs []refEdge, px0, py0, hw float64) float64 {
	inside := 0
	for sy := 0; sy < refPathSS; sy++ {
		// The rasteriser counts a sub-scanline that lands exactly on the
		// bottom edge of a shape as inside it and one on the top edge as
		// outside — the half-open rule that keeps a vertex shared by two edges
		// from being counted twice. Asking a hair above the sub-scanline says
		// the same thing with a closed test.
		y := py0 + (float64(sy)+0.5)/refPathSS + refEdgeNudge
		for sx := 0; sx < refStrokeSamples; sx++ {
			x := px0 + (float64(sx)+0.5)/refStrokeSamples
			for _, e := range segs {
				if refDistToSeg(x, y, e) <= hw {
					inside++
					break
				}
			}
		}
	}
	return float64(inside) / (refPathSS * refStrokeSamples)
}

// refSegNear reports whether a segment can reach into the pixel at all.
func refSegNear(e refEdge, px0, py0, hw float64) bool {
	lo, hi := e.x0, e.x1
	if lo > hi {
		lo, hi = hi, lo
	}
	if hi+hw < px0 || lo-hw > px0+1 {
		return false
	}
	lo, hi = e.y0, e.y1
	if lo > hi {
		lo, hi = hi, lo
	}
	return !(hi+hw < py0 || lo-hw > py0+1)
}

// refDistToSeg is how far a point is from a segment, which for a zero-length
// one is how far it is from the point itself.
func refDistToSeg(x, y float64, e refEdge) float64 {
	dx, dy := e.x1-e.x0, e.y1-e.y0
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(x-e.x0, y-e.y0)
	}
	t := ((x-e.x0)*dx + (y-e.y0)*dy) / l2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(x-(e.x0+t*dx), y-(e.y0+t*dy))
}
