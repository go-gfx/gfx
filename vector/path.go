// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// Package vector is go-gfx's pure-Go 2-D vector rasterizer: it turns arbitrary
// outlines (move / line / quadratic / cubic / close) into per-pixel coverage,
// with an anti-aliased scanline accumulator and a round-join / round-cap stroker.
//
// It computes COVERAGE, not colour: a [Rasterizer] hands back a float64 grid of
// per-pixel coverage (0..1) over an integer box, which a consumer composites into
// its own pixel buffer however it likes (honouring its own clip, blend mode, and
// colour). Keeping the geometry and coverage here — decoupled from any particular
// surface — lets go-widgets/painter, go-webengine and any other renderer share
// one exact rasterizer rather than hand-rolling their own.
//
// Coordinates are float64 pixel units. Curves are flattened to line segments by
// an error tolerance at fill / stroke time, so a consumer never deals with
// beziers directly.
package vector

import "math"

// FillRule selects how a path's winding count decides which side of the outline
// is "inside" and therefore filled.
type FillRule int

const (
	// NonZero fills a point when the signed edge-crossing count around it is
	// non-zero. Overlapping sub-paths that wind the same way merge into a solid
	// region; this is the intuitive rule for most icon shapes.
	NonZero FillRule = iota

	// EvenOdd fills a point when the crossing count is odd. Overlapping regions
	// alternate filled/empty, so a shape drawn over itself punches a hole — the
	// rule that turns a self-overlapping star's centre into a cut-out.
	EvenOdd
)

// pathOp identifies one command recorded on a Path.
type pathOp uint8

const (
	opMove pathOp = iota
	opLine
	opQuad
	opCubic
	opClose
)

// pathSeg is one recorded command. Unused control fields stay zero.
type pathSeg struct {
	op       pathOp
	x, y     float64 // end / target point (move, line, quad, cubic)
	cx1, cy1 float64 // quad control, or cubic first control
	cx2, cy2 float64 // cubic second control
}

// Path is a mutable 2-D outline: a sequence of sub-paths built from move / line
// / quadratic / cubic / close commands. A Path carries no colour or width — it
// is handed to a [Rasterizer]'s Fill / Stroke together with a clamp box. The
// builder methods return the receiver so calls chain:
//
//	pth := vector.NewPath().MoveTo(0, 0).LineTo(10, 0).LineTo(5, 8).Close()
//
// Coordinates are float64 pixel units. Curves are flattened to line segments by
// an error tolerance at fill/stroke time, so a consumer never deals with beziers
// directly.
type Path struct {
	segs []pathSeg
}

// NewPath returns an empty Path ready to build.
func NewPath() *Path { return &Path{} }

// MoveTo starts a new sub-path at (x, y).
func (p *Path) MoveTo(x, y float64) *Path {
	p.segs = append(p.segs, pathSeg{op: opMove, x: x, y: y})
	return p
}

// LineTo adds a straight segment from the current point to (x, y).
func (p *Path) LineTo(x, y float64) *Path {
	p.segs = append(p.segs, pathSeg{op: opLine, x: x, y: y})
	return p
}

// QuadTo adds a quadratic bezier from the current point through control
// (cx, cy) to (x, y).
func (p *Path) QuadTo(cx, cy, x, y float64) *Path {
	p.segs = append(p.segs, pathSeg{op: opQuad, cx1: cx, cy1: cy, x: x, y: y})
	return p
}

// CubicTo adds a cubic bezier from the current point through controls
// (cx1, cy1) and (cx2, cy2) to (x, y).
func (p *Path) CubicTo(cx1, cy1, cx2, cy2, x, y float64) *Path {
	p.segs = append(p.segs, pathSeg{op: opCubic, cx1: cx1, cy1: cy1, cx2: cx2, cy2: cy2, x: x, y: y})
	return p
}

// Close marks the current sub-path closed — a straight segment joins its last
// point back to its start. A fill implicitly closes every sub-path regardless;
// Close matters to Stroke, where it turns end caps into a join.
func (p *Path) Close() *Path {
	p.segs = append(p.segs, pathSeg{op: opClose})
	return p
}

// A PathOp is one command of a [Path], as [Path.Each] reports it.
type PathOp uint8

// The commands a Path records. They are exactly the builder methods, so a
// path read back and rebuilt command by command is the path that was read.
const (
	// OpMove starts a new sub-path at the point.
	OpMove PathOp = iota
	// OpLine draws a straight segment to the point.
	OpLine
	// OpQuad draws a quadratic bezier through one control to the point.
	OpQuad
	// OpCubic draws a cubic bezier through two controls to the point.
	OpCubic
	// OpClose closes the current sub-path. It carries no points.
	OpClose
)

// Each calls fn for every command of the path, in the order they were added.
//
// The points are the command's own, in the order the matching builder method
// takes them: nothing for [OpClose], the target for [OpMove] and [OpLine],
// control-then-target for [OpQuad], and both controls then the target for
// [OpCubic].
//
// ⛔ THIS EXISTS BECAUSE A PATH THAT CANNOT BE READ BACK CANNOT BE EXPORTED.
// A Path is built to be rasterised, and for that nothing needs to read it —
// the rasteriser owns the only consumer and reaches the fields directly. But a
// consumer that writes a VECTOR format (PDF, SVG, PostScript) needs the curves
// themselves, and flattening to line segments, which is what [Path.flatten]
// does for the scan converter, throws away exactly what makes those formats
// worth writing to.
func (p *Path) Each(fn func(op PathOp, pts ...[2]float64)) {
	for _, s := range p.segs {
		switch s.op {
		case opMove:
			fn(OpMove, [2]float64{s.x, s.y})
		case opLine:
			fn(OpLine, [2]float64{s.x, s.y})
		case opQuad:
			fn(OpQuad, [2]float64{s.cx1, s.cy1}, [2]float64{s.x, s.y})
		case opCubic:
			fn(OpCubic, [2]float64{s.cx1, s.cy1}, [2]float64{s.cx2, s.cy2}, [2]float64{s.x, s.y})
		default:
			fn(OpClose)
		}
	}
}

// Len is how many commands the path records.
func (p *Path) Len() int { return len(p.segs) }

// flattenTol is the maximum distance, in pixel units, a flattened line segment
// may deviate from the true curve. Smaller = smoother + more segments.
const flattenTol = 0.2

// flattenMaxDepth caps bezier subdivision recursion so a pathological control
// polygon cannot recurse without bound.
const flattenMaxDepth = 24

// point is a flattened vertex in pixel units.
type point struct{ x, y float64 }

// subpath is one flattened contour: its vertices plus whether it is closed.
type subpath struct {
	pts    []point
	closed bool
}

// flatten converts the recorded commands into flattened contours, subdividing
// beziers until each segment is within tol of the true curve. A drawing command
// issued before any MoveTo implicitly starts a sub-path at the current point
// (initially the origin); a Close before any point is a no-op.
func (p *Path) flatten(tol float64) []subpath {
	var out []subpath
	var cur *subpath
	var cx, cy float64 // current point
	var sx, sy float64 // current sub-path start

	startSub := func(x, y float64) {
		out = append(out, subpath{pts: []point{{x, y}}})
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
			cur.pts = append(cur.pts, point{s.x, s.y})
			cx, cy = s.x, s.y
		case opQuad:
			ensure()
			cur.pts = flattenQuad(cur.pts, cx, cy, s.cx1, s.cy1, s.x, s.y, tol, flattenMaxDepth)
			cx, cy = s.x, s.y
		case opCubic:
			ensure()
			cur.pts = flattenCubic(cur.pts, cx, cy, s.cx1, s.cy1, s.cx2, s.cy2, s.x, s.y, tol, flattenMaxDepth)
			cx, cy = s.x, s.y
		default: // opClose
			if cur != nil {
				cur.closed = true
				cx, cy = sx, sy
				cur = nil
			}
		}
	}
	return out
}

// flattenQuad appends the flattened vertices of the quadratic bezier
// P0(x0,y0)-control(cx,cy)-P1(x1,y1) to out, EXCLUDING the start point P0 (the
// caller already holds it). It subdivides at the midpoint until the control lies
// within tol of the P0->P1 chord, or the depth budget runs out.
func flattenQuad(out []point, x0, y0, cx, cy, x1, y1, tol float64, depth int) []point {
	if depth <= 0 || distToLine(cx, cy, x0, y0, x1, y1) <= tol {
		return append(out, point{x1, y1})
	}
	x01, y01 := (x0+cx)/2, (y0+cy)/2
	x12, y12 := (cx+x1)/2, (cy+y1)/2
	xm, ym := (x01+x12)/2, (y01+y12)/2
	out = flattenQuad(out, x0, y0, x01, y01, xm, ym, tol, depth-1)
	out = flattenQuad(out, xm, ym, x12, y12, x1, y1, tol, depth-1)
	return out
}

// flattenCubic appends the flattened vertices of the cubic bezier
// P0-c1-c2-P1 to out, EXCLUDING the start point P0. It subdivides at the
// midpoint until both controls lie within tol of the P0->P1 chord, or the depth
// budget runs out.
func flattenCubic(out []point, x0, y0, c1x, c1y, c2x, c2y, x1, y1, tol float64, depth int) []point {
	if depth <= 0 || (distToLine(c1x, c1y, x0, y0, x1, y1) <= tol && distToLine(c2x, c2y, x0, y0, x1, y1) <= tol) {
		return append(out, point{x1, y1})
	}
	x01, y01 := (x0+c1x)/2, (y0+c1y)/2
	x12, y12 := (c1x+c2x)/2, (c1y+c2y)/2
	x23, y23 := (c2x+x1)/2, (c2y+y1)/2
	x012, y012 := (x01+x12)/2, (y01+y12)/2
	x123, y123 := (x12+x23)/2, (y12+y23)/2
	xm, ym := (x012+x123)/2, (y012+y123)/2
	out = flattenCubic(out, x0, y0, x01, y01, x012, y012, xm, ym, tol, depth-1)
	out = flattenCubic(out, xm, ym, x123, y123, x23, y23, x1, y1, tol, depth-1)
	return out
}

// distToLine returns the perpendicular distance from (px, py) to the infinite
// line through (ax, ay) and (bx, by). When the two anchors coincide it degrades
// to the point-to-point distance.
func distToLine(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	return math.Abs((px-ax)*dy-(py-ay)*dx) / math.Sqrt(l2)
}
