// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

import (
	"math"
	"testing"
)

// --- builder + flatten -----------------------------------------------------

func TestPathBuilderChaining(t *testing.T) {
	p := NewPath().MoveTo(0, 0).LineTo(4, 0).QuadTo(4, 4, 0, 4).CubicTo(-2, 2, -2, 1, 0, 0).Close()
	wantOps := []pathOp{opMove, opLine, opQuad, opCubic, opClose}
	if len(p.segs) != len(wantOps) {
		t.Fatalf("recorded %d segs, want %d", len(p.segs), len(wantOps))
	}
	for i, op := range wantOps {
		if p.segs[i].op != op {
			t.Errorf("seg %d op = %d, want %d", i, p.segs[i].op, op)
		}
	}
}

func TestFlattenImplicitMoveFromOrigin(t *testing.T) {
	subs := NewPath().LineTo(3, 0).flatten(flattenTol)
	if len(subs) != 1 {
		t.Fatalf("got %d sub-paths, want 1", len(subs))
	}
	if subs[0].pts[0] != (point{0, 0}) {
		t.Errorf("implicit start = %v, want origin", subs[0].pts[0])
	}
}

func TestFlattenQuadAndCubicImplicitStart(t *testing.T) {
	q := NewPath().QuadTo(2, 2, 4, 0).flatten(flattenTol)
	if q[0].pts[0] != (point{0, 0}) {
		t.Errorf("quad implicit start = %v, want origin", q[0].pts[0])
	}
	c := NewPath().CubicTo(1, 3, 3, 3, 4, 0).flatten(flattenTol)
	if c[0].pts[0] != (point{0, 0}) {
		t.Errorf("cubic implicit start = %v, want origin", c[0].pts[0])
	}
}

func TestFlattenCloseThenReopen(t *testing.T) {
	subs := NewPath().MoveTo(2, 2).LineTo(6, 2).Close().LineTo(6, 6).flatten(flattenTol)
	if len(subs) != 2 {
		t.Fatalf("got %d sub-paths, want 2", len(subs))
	}
	if !subs[0].closed {
		t.Error("first sub-path should be closed")
	}
	if subs[1].pts[0] != (point{2, 2}) {
		t.Errorf("reopened sub-path start = %v, want the closed sub-path's start (2,2)", subs[1].pts[0])
	}
}

func TestFlattenCloseBeforeAnyPointIsNoOp(t *testing.T) {
	subs := NewPath().Close().flatten(flattenTol)
	if len(subs) != 0 {
		t.Fatalf("Close on an empty path produced %d sub-paths, want 0", len(subs))
	}
}

func TestFlattenBezierSegmentCountMonotonic(t *testing.T) {
	counts := make([]int, 0, 4)
	for _, bow := range []float64{0.1, 2, 8, 30} {
		pts := flattenQuad(nil, 0, 0, 10, bow, 20, 0, flattenTol, flattenMaxDepth)
		counts = append(counts, len(pts))
	}
	for i := 1; i < len(counts); i++ {
		if counts[i] < counts[i-1] {
			t.Errorf("segment counts not monotonic: %v", counts)
		}
	}
	if counts[0] != 1 {
		t.Errorf("near-flat quad flattened to %d segments, want 1", counts[0])
	}
	if counts[len(counts)-1] <= counts[0] {
		t.Errorf("high-curvature quad (%d) did not exceed low-curvature (%d)", counts[len(counts)-1], counts[0])
	}
	lo := flattenCubic(nil, 0, 0, 4, 0.2, 16, 0.2, 20, 0, flattenTol, flattenMaxDepth)
	hi := flattenCubic(nil, 0, 0, 4, 20, 16, -20, 20, 0, flattenTol, flattenMaxDepth)
	if len(hi) <= len(lo) {
		t.Errorf("cubic: high-curvature %d not > low-curvature %d", len(hi), len(lo))
	}
}

func TestFlattenDepthGuard(t *testing.T) {
	q := flattenQuad(nil, 0, 0, 10, 10, 20, 0, -1, 3)
	if len(q) != 1<<3 {
		t.Errorf("quad depth-3 produced %d segments, want %d", len(q), 1<<3)
	}
	c := flattenCubic(nil, 0, 0, 4, 8, 16, 8, 20, 0, -1, 3)
	if len(c) != 1<<3 {
		t.Errorf("cubic depth-3 produced %d segments, want %d", len(c), 1<<3)
	}
}

func TestDistToLine(t *testing.T) {
	if d := distToLine(0, 3, -1, 0, 1, 0); math.Abs(d-3) > 1e-9 {
		t.Errorf("distToLine perpendicular = %v, want 3", d)
	}
	// Degenerate anchor (a == b): falls back to point distance (3-4-5).
	if d := distToLine(3, 4, 0, 0, 0, 0); math.Abs(d-5) > 1e-9 {
		t.Errorf("distToLine degenerate = %v, want 5", d)
	}
}

// --- Rasterizer no-op / degenerate branches --------------------------------

func TestFillNoOps(t *testing.T) {
	var rz Rasterizer
	cases := []struct {
		name string
		pth  *Path
	}{
		{"nil path", nil},
		{"empty path", NewPath()},
		{"single point", NewPath().MoveTo(5, 5)},
		{"off-screen", NewPath().MoveTo(-20, -20).LineTo(-10, -20).LineTo(-15, -10).Close()},
	}
	for _, tc := range cases {
		if _, _, _, _, _, ok := rz.Fill(tc.pth, NonZero, 16, 16); ok {
			t.Errorf("%s: Fill ok = true, want false", tc.name)
		}
	}
}

func TestStrokeNoOps(t *testing.T) {
	var rz Rasterizer
	cases := []struct {
		name  string
		pth   *Path
		width float64
	}{
		{"nil path", nil, 2},
		{"empty path", NewPath(), 2},
		{"zero width", NewPath().MoveTo(2, 2).LineTo(12, 2), 0},
		{"negative width", NewPath().MoveTo(2, 2).LineTo(12, 2), -3},
		{"single point (no segment)", NewPath().MoveTo(8, 8), 4},
		{"off-screen", NewPath().MoveTo(-40, -40).LineTo(-30, -40), 3},
	}
	for _, tc := range cases {
		if _, _, _, _, _, ok := rz.Stroke(tc.pth, tc.width, 16, 16); ok {
			t.Errorf("%s: Stroke ok = true, want false", tc.name)
		}
	}
}

func TestStrokeOffSurfaceSegmentSkipped(t *testing.T) {
	// A segment lying entirely off the surface exercises Stroke's subBox !ok
	// branch: it contributes nothing, but the on-surface part still strokes.
	var rz Rasterizer
	pth := NewPath().MoveTo(-100, 8).LineTo(-90, 8).LineTo(8, 8)
	cov, ox, oy, w, h, ok := rz.Stroke(pth, 3, 16, 16)
	if !ok {
		t.Fatal("on-surface part of the stroke produced no coverage")
	}
	var sum float64
	for _, c := range cov {
		sum += c
	}
	if sum == 0 {
		t.Fatal("on-surface part of the stroke painted nothing")
	}
	// centre of the visible segment must be covered.
	if k := (8-oy)*w + (8 - ox); k >= 0 && k < w*h && cov[k] == 0 {
		t.Error("visible segment centre not covered")
	}
}

func TestStrokeZeroLengthSegmentDot(t *testing.T) {
	// A degenerate LineTo to the same point yields a zero-length segment (nil
	// rectangle); the vertex disk still paints a round dot.
	var rz Rasterizer
	dot := NewPath().MoveTo(10, 10).LineTo(10, 10)
	cov, ox, oy, w, h, ok := rz.Stroke(dot, 6, 20, 20)
	if !ok {
		t.Fatal("zero-length segment produced no coverage box")
	}
	if k := (10-oy)*w + (10 - ox); k < 0 || k >= w*h || cov[k] == 0 {
		t.Error("zero-length segment should still paint a round dot via the join disk")
	}
}

func TestFillWindingNonZeroVsEvenOdd(t *testing.T) {
	// The centre pentagon of a pentagram is wound twice: NonZero fills it,
	// EvenOdd cuts it out. Coverage at the centre reflects that.
	var rz Rasterizer
	s := pentagram(30, 30, 26)
	nzCov, nzox, nzoy, nzw, nzh, ok := rz.Fill(s, NonZero, 60, 60)
	if !ok {
		t.Fatal("nonzero fill produced nothing")
	}
	nz := append([]float64(nil), nzCov...)
	eoCov, eoox, eooy, eow, eoh, ok := rz.Fill(s, EvenOdd, 60, 60)
	if !ok {
		t.Fatal("evenodd fill produced nothing")
	}
	// same geometry -> same box.
	if nzox != eoox || nzoy != eooy || nzw != eow || nzh != eoh {
		t.Fatalf("box differs by rule: nz (%d,%d,%d,%d) eo (%d,%d,%d,%d)", nzox, nzoy, nzw, nzh, eoox, eooy, eow, eoh)
	}
	ci := (30-nzoy)*nzw + (30 - nzox)
	if nz[ci] == 0 {
		t.Error("NonZero: star centre should be filled")
	}
	if eoCov[ci] != 0 {
		t.Errorf("EvenOdd: star centre should be a hole, got coverage %v", eoCov[ci])
	}
	// EvenOdd removes area, so it sums strictly less coverage.
	var nzSum, eoSum float64
	for i := range nz {
		nzSum += nz[i]
		eoSum += eoCov[i]
	}
	if eoSum >= nzSum {
		t.Errorf("EvenOdd coverage %.2f should be < NonZero %.2f", eoSum, nzSum)
	}
}

// --- cover.go white-box branches -------------------------------------------

func TestCoverGridHorizontalEdgeSkipped(t *testing.T) {
	// A purely horizontal edge crosses no scanline; a shape that is only a flat
	// line (zero height) yields zero coverage everywhere. Exercises the
	// allocating coverGrid entry point.
	edges := []edge{{0, 5, 10, 5}, {10, 5, 0, 5}}
	cov := coverGrid(edges, NonZero, 0, 0, 10, 10, pathSS)
	for _, c := range cov {
		if c != 0 {
			t.Fatalf("horizontal-only edges produced coverage %v", c)
		}
	}
}

func TestInsideRule(t *testing.T) {
	if !insideRule(2, NonZero) || insideRule(0, NonZero) {
		t.Error("NonZero: inside iff winding != 0")
	}
	if !insideRule(1, EvenOdd) || insideRule(2, EvenOdd) {
		t.Error("EvenOdd: inside iff winding odd")
	}
}

func TestAddSpanClampsAndEmpty(t *testing.T) {
	row := make([]float64, 4)
	addSpan(row, -2, 6, 0, 4, 1)
	for i, v := range row {
		if math.Abs(v-1) > 1e-9 {
			t.Errorf("clamped span: row[%d] = %v, want 1", i, v)
		}
	}
	before := append([]float64(nil), row...)
	addSpan(row, 3, 3, 0, 4, 1) // xb <= xa -> no-op
	for i := range row {
		if row[i] != before[i] {
			t.Errorf("empty span mutated row[%d]", i)
		}
	}
}

func TestDiskMaxZeroRadiusNoOp(t *testing.T) {
	cov := make([]float64, 4)
	diskMax(cov, 0, 0, 2, 2, 0.5, 0.5, 0)
	diskMax(cov, 0, 0, 2, 2, 0.5, 0.5, -3)
	for i, v := range cov {
		if v != 0 {
			t.Errorf("zero/negative-radius disk wrote cov[%d] = %v", i, v)
		}
	}
}

func TestClampBoxBranches(t *testing.T) {
	// inside, low overhang (ox/oy<0), high overhang (x1>W, y1>H), and empty.
	cases := []struct {
		name                       string
		minX, minY, maxX, maxY     float64
		w, h                       int
		wantX, wantY, wantW, wantH int
		wantOK                     bool
	}{
		{"inside", 2, 2, 5, 5, 10, 10, 2, 2, 3, 3, true},
		{"clampLow", -3, -4, 4, 4, 10, 10, 0, 0, 4, 4, true},
		{"clampHigh", 6, 6, 15, 15, 10, 10, 6, 6, 4, 4, true},
		{"empty", 20, 20, 30, 30, 10, 10, 0, 0, 0, 0, false},
	}
	for _, c := range cases {
		ox, oy, w, h, ok := clampBox(c.minX, c.minY, c.maxX, c.maxY, c.w, c.h)
		if ok != c.wantOK {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if ox != c.wantX || oy != c.wantY || w != c.wantW || h != c.wantH {
			t.Errorf("%s: got (%d,%d,%d,%d), want (%d,%d,%d,%d)", c.name, ox, oy, w, h, c.wantX, c.wantY, c.wantW, c.wantH)
		}
	}
}

func TestSubBoxClampAndEmpty(t *testing.T) {
	cases := []struct {
		name                       string
		minX, minY, maxX, maxY     float64
		wantX, wantY, wantW, wantH int
		wantOK                     bool
	}{
		{"inside", 2, 2, 5, 5, 2, 2, 3, 3, true},
		{"clampLow", -3, -3, 4, 4, 0, 0, 4, 4, true},
		{"clampHigh", 6, 6, 15, 15, 6, 6, 4, 4, true},
		{"empty", 20, 20, 30, 30, 0, 0, 0, 0, false},
	}
	for _, c := range cases {
		sox, soy, sw, sh, ok := subBox(c.minX, c.minY, c.maxX, c.maxY, 0, 0, 10, 10)
		if ok != c.wantOK {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if sox != c.wantX || soy != c.wantY || sw != c.wantW || sh != c.wantH {
			t.Errorf("%s: got (%d,%d,%d,%d), want (%d,%d,%d,%d)", c.name, sox, soy, sw, sh, c.wantX, c.wantY, c.wantW, c.wantH)
		}
	}
}

func TestMaxSubMergesTileAtOffset(t *testing.T) {
	dst := []float64{0.1, 0.1, 0.1, 0.1, 0.9, 0.1, 0.1, 0.1, 0.1}
	src := []float64{0.5, 0.2, 0.05, 0.5} // 2x2 tile
	maxSub(dst, 3, src, 1, 1, 2, 2)
	want := []float64{0.1, 0.1, 0.1, 0.1, 0.9, 0.2, 0.1, 0.1, 0.5}
	for i := range want {
		if math.Abs(dst[i]-want[i]) > 1e-12 {
			t.Errorf("dst[%d] = %v, want %v", i, dst[i], want[i])
		}
	}
}
