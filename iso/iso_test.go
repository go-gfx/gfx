// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package iso

import (
	"image/color"
	"math"
	"testing"

	"github.com/go-gfx/gfx/geometry"
	"github.com/go-gfx/gfx/raster"
)

// --- test instruments, validated by TestControlHelpers before use ------------

// ptExact reports whether p equals (x, y) exactly (the projections in these
// tests land on integers, so no tolerance is needed).
func ptExact(p geometry.Point, x, y float64) bool { return p.X == x && p.Y == y }

// polyExact reports whether poly matches want vertex-for-vertex, exactly.
func polyExact(poly []geometry.Point, want [][2]float64) bool {
	if len(poly) != len(want) {
		return false
	}
	for i, p := range poly {
		if !ptExact(p, want[i][0], want[i][1]) {
			return false
		}
	}
	return true
}

// approxEq reports whether a and b are within eps.
func approxEq(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// TestControlHelpers is the control run for the comparison instruments the rest
// of the suite relies on: it feeds them inputs whose answers are known so a
// broken helper cannot silently pass a broken primitive.
func TestControlHelpers(t *testing.T) {
	p := geometry.Pt(3, 4)
	if !ptExact(p, 3, 4) {
		t.Fatal("ptExact must accept an exact match")
	}
	if ptExact(p, 3, 5) || ptExact(p, 2, 4) {
		t.Fatal("ptExact must reject a mismatch on either axis")
	}
	poly := []geometry.Point{{X: 0, Y: 0}, {X: 1, Y: 2}}
	if !polyExact(poly, [][2]float64{{0, 0}, {1, 2}}) {
		t.Fatal("polyExact must accept an exact match")
	}
	if polyExact(poly, [][2]float64{{0, 0}}) {
		t.Fatal("polyExact must reject a length mismatch")
	}
	if polyExact(poly, [][2]float64{{0, 0}, {1, 3}}) {
		t.Fatal("polyExact must reject a vertex mismatch")
	}
	if !approxEq(1.0, 1.0+1e-12, 1e-9) || approxEq(1.0, 1.1, 1e-9) {
		t.Fatal("approxEq must honour its tolerance")
	}
}

// testProj is the reference projection used throughout: origin (100,100), a
// 64x32 tile and a 32-pixel height unit, so hx=32, hy=16, ZScale=32 and every
// integer world point projects to an integer pixel.
func testProj() *Projection { return New(geometry.Pt(100, 100), 64, 32, 32) }

// --- projection --------------------------------------------------------------

func TestProjectExact(t *testing.T) {
	p := testProj()
	cases := []struct {
		v      Vec3
		x, y   float64
		reason string
	}{
		{V(0, 0, 0), 100, 100, "origin maps to Origin"},
		{V(1, 0, 0), 132, 116, "+X is right and down by (hx,hy)"},
		{V(0, 1, 0), 68, 116, "+Y is left and down by (-hx,hy)"},
		{V(0, 0, 1), 100, 68, "+Z rises by ZScale"},
		{V(2, 3, 1), 68, 148, "combined axes"},
	}
	for _, c := range cases {
		got := p.Project(c.v)
		if !ptExact(got, c.x, c.y) {
			t.Fatalf("Project(%v) = %v, want (%g,%g): %s", c.v, got, c.x, c.y, c.reason)
		}
	}
}

func TestUnprojectExactAndRoundTrip(t *testing.T) {
	p := testProj()

	// Exact inverse on a known screen point at a chosen z-plane.
	got := p.Unproject(geometry.Pt(68, 148), 1)
	if got != (Vec3{2, 3, 1}) {
		t.Fatalf("Unproject((68,148),1) = %v, want {2,3,1}", got)
	}

	// Round-trip Unproject(Project(v), v.Z) == v for varied points, including
	// fractional coordinates and a non-zero z-plane.
	for _, v := range []Vec3{
		{0, 0, 0}, {2, 3, 1}, {-1.5, 4.25, 2}, {10, -7, 3.5}, {0.5, 0.5, 0},
	} {
		s := p.Project(v)
		r := p.Unproject(s, v.Z)
		if !approxEq(r.X, v.X, 1e-9) || !approxEq(r.Y, v.Y, 1e-9) || r.Z != v.Z {
			t.Fatalf("round-trip of %v = %v", v, r)
		}
	}
}

func TestConstructors(t *testing.T) {
	d := NewDefault(geometry.Pt(0, 0))
	if d.TileW != 64 || d.TileH != 32 || d.ZScale != 32 {
		t.Fatalf("NewDefault tile = (%g,%g,%g), want (64,32,32)", d.TileW, d.TileH, d.ZScale)
	}
	if got := d.Project(V(1, 0, 0)); !ptExact(got, 32, 16) {
		t.Fatalf("default Project(1,0,0) = %v, want (32,16)", got)
	}

	// 26.565° reproduces the 2:1 tile (tan = 1/2), 45° gives a square tile.
	a := NewFromAngle(geometry.Pt(0, 0), 64, 26.565, 32)
	if !approxEq(a.TileH, 32, 0.02) {
		t.Fatalf("NewFromAngle 26.565° TileH = %g, want ~32", a.TileH)
	}
	q := NewFromAngle(geometry.Pt(0, 0), 64, 45, 32)
	if !approxEq(q.TileH, 64, 1e-9) {
		t.Fatalf("NewFromAngle 45° TileH = %g, want ~64", q.TileH)
	}
}

func TestDepthOrdersByViewDistance(t *testing.T) {
	p := testProj()
	if d := p.Depth(V(2, 3, 1)); d != 6 {
		t.Fatalf("Depth = %g, want 6", d)
	}
	// Nearer (larger X+Y+Z) sorts after farther.
	if !(p.Depth(V(0, 0, 0)) < p.Depth(V(1, 0, 0)) &&
		p.Depth(V(1, 0, 0)) < p.Depth(V(1, 1, 1))) {
		t.Fatal("Depth must increase toward the viewer")
	}
}

// --- primitive face polygons (exact vertices) --------------------------------

var baseColor = color.RGBA{R: 200, G: 120, B: 80, A: 255}

// The three DefaultShading face colours of baseColor, precomputed.
var (
	topColor   = color.RGBA{200, 120, 80, 255} // factor 1.00
	leftColor  = color.RGBA{150, 90, 60, 255}  // factor 0.75
	rightColor = color.RGBA{110, 66, 44, 255}  // factor 0.55
)

func TestCubeFaces(t *testing.T) {
	p := testProj()
	faces := Cube{Pos: V(0, 0, 0), Size: 1, Color: baseColor}.Faces(p)
	if len(faces) != 3 {
		t.Fatalf("cube has %d faces, want 3", len(faces))
	}
	want := []struct {
		poly  [][2]float64
		color color.RGBA
	}{
		{[][2]float64{{132, 116}, {100, 132}, {100, 100}, {132, 84}}, rightColor}, // +X
		{[][2]float64{{68, 116}, {100, 132}, {100, 100}, {68, 84}}, leftColor},    // +Y
		{[][2]float64{{100, 68}, {132, 84}, {100, 100}, {68, 84}}, topColor},      // top
	}
	for i, w := range want {
		if !polyExact(faces[i].Poly, w.poly) {
			t.Fatalf("cube face %d poly = %v, want %v", i, faces[i].Poly, w.poly)
		}
		if faces[i].Color != w.color {
			t.Fatalf("cube face %d color = %v, want %v", i, faces[i].Color, w.color)
		}
	}
}

func TestBrickFace(t *testing.T) {
	p := testProj()
	faces := Brick{Pos: V(0, 0, 0), Dim: Dimension{2, 1, 1}, Color: baseColor}.Faces(p)
	// The +X (right) face of a 2x1x1 box.
	want := [][2]float64{{164, 132}, {132, 148}, {132, 116}, {164, 100}}
	if !polyExact(faces[0].Poly, want) {
		t.Fatalf("brick right face = %v, want %v", faces[0].Poly, want)
	}
	if faces[0].Color != rightColor {
		t.Fatalf("brick right color = %v, want %v", faces[0].Color, rightColor)
	}
}

func TestBrickCubeAgree(t *testing.T) {
	p := testProj()
	cf := Cube{Pos: V(1, 2, 0), Size: 3, Color: baseColor}.Faces(p)
	bf := Brick{Pos: V(1, 2, 0), Dim: Dimension{3, 3, 3}, Color: baseColor}.Faces(p)
	for i := range cf {
		if !polyExact(cf[i].Poly, toWant(bf[i].Poly)) || cf[i].Color != bf[i].Color {
			t.Fatalf("cube face %d disagrees with equal-sided brick", i)
		}
	}
}

// toWant converts a projected polygon to the [][2]float64 form polyExact wants.
func toWant(poly []geometry.Point) [][2]float64 {
	w := make([][2]float64, len(poly))
	for i, p := range poly {
		w[i] = [2]float64{p.X, p.Y}
	}
	return w
}

func TestPyramidFaces(t *testing.T) {
	p := testProj()
	faces := Pyramid{Pos: V(0, 0, 0), Dim: Dimension{2, 2, 2}, Color: baseColor}.Faces(p)
	if len(faces) != 2 {
		t.Fatalf("pyramid has %d faces, want 2", len(faces))
	}
	// +X face: base edge (2,0,0)-(2,2,0) meeting apex (1,1,2).
	wantRight := [][2]float64{{164, 132}, {100, 164}, {100, 68}}
	if !polyExact(faces[0].Poly, wantRight) || faces[0].Color != rightColor {
		t.Fatalf("pyramid right face = %v (%v), want %v (%v)",
			faces[0].Poly, faces[0].Color, wantRight, rightColor)
	}
	wantLeft := [][2]float64{{36, 132}, {100, 164}, {100, 68}}
	if !polyExact(faces[1].Poly, wantLeft) || faces[1].Color != leftColor {
		t.Fatalf("pyramid left face = %v, want %v", faces[1].Poly, wantLeft)
	}
}

func TestSlopeFaces(t *testing.T) {
	p := testProj()
	// SlopeE raises the +X edge to full height: its +X face is the tall end,
	// a full-height quad identical to a cube's right face.
	faces := Slope{Pos: V(0, 0, 0), Dim: Dimension{1, 1, 1}, Dir: SlopeE, Color: baseColor}.Faces(p)
	wantRight := [][2]float64{{132, 116}, {100, 132}, {100, 100}, {132, 84}}
	if !polyExact(faces[0].Poly, wantRight) {
		t.Fatalf("SlopeE +X face = %v, want %v", faces[0].Poly, wantRight)
	}
	// The ramp (top) rises: its far-x edge sits a full ZScale higher than the
	// near-x edge. Corner (1,0,1) projects to (132,84), corner (0,0,0) to
	// (100,100).
	top := faces[2].Poly
	if !ptExact(top[0], 100, 100) || !ptExact(top[1], 132, 84) {
		t.Fatalf("SlopeE ramp near/far edge = %v,%v, want (100,100),(132,84)", top[0], top[1])
	}

	// SlopeW raises the -X edge, so the +X (right) face collapses to the base
	// edge — a zero-area polygon the scene must skip.
	west := Slope{Pos: V(0, 0, 0), Dim: Dimension{1, 1, 1}, Dir: SlopeW, Color: baseColor}.Faces(p)
	if r := west[0].Poly; !zeroArea(r) {
		t.Fatalf("SlopeW +X face should be degenerate, got %v", r)
	}
}

// zeroArea reports whether the polygon encloses no area (shoelace == 0).
func zeroArea(poly []geometry.Point) bool {
	var a float64
	for i := range poly {
		j := (i + 1) % len(poly)
		a += poly[i].X*poly[j].Y - poly[j].X*poly[i].Y
	}
	return a == 0
}

func TestSlopeAllDirections(t *testing.T) {
	p := testProj()
	// Each direction raises exactly one edge; the ramp's four top-corner heights
	// must be full (1) at the raised edge and base (0) at the opposite edge.
	for _, tc := range []struct {
		dir  SlopeDir
		high func(cx, cy float64) bool // corner expected at full height
	}{
		{SlopeE, func(cx, _ float64) bool { return cx == 1 }},
		{SlopeW, func(cx, _ float64) bool { return cx == 0 }},
		{SlopeN, func(_, cy float64) bool { return cy == 0 }},
		{SlopeS, func(_, cy float64) bool { return cy == 1 }},
	} {
		s := Slope{Pos: V(0, 0, 0), Dim: Dimension{1, 1, 1}, Dir: tc.dir, Color: baseColor}
		top := s.Faces(p)[2].Poly // ramp corners in (0,0),(1,0),(1,1),(0,1) order
		corners := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
		for i, c := range corners {
			// A raised corner is 32px higher on screen than a base corner at the
			// same ground position, so compare projected y to the flat baseline.
			flatY := p.Project(V(c[0], c[1], 0)).Y
			raised := top[i].Y == flatY-32
			if raised != tc.high(c[0], c[1]) {
				t.Fatalf("dir %d corner %v raised=%v, want %v", tc.dir, c, raised, tc.high(c[0], c[1]))
			}
		}
	}
}

func TestSlopeInvalidDirIsFlat(t *testing.T) {
	p := testProj()
	// An out-of-range direction falls through to the flat (all-base) top.
	s := Slope{Pos: V(0, 0, 0), Dim: Dimension{1, 1, 1}, Dir: SlopeDir(99), Color: baseColor}
	top := s.Faces(p)[2].Poly
	for i, c := range [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}} {
		if top[i].Y != p.Project(V(c[0], c[1], 0)).Y {
			t.Fatalf("invalid-dir ramp corner %v not flat: %v", c, top[i])
		}
	}
}

func TestSideFaces(t *testing.T) {
	p := testProj()
	yz := Side{Pos: V(0, 0, 0), W: 1, H: 1, Plane: SideYZ, Color: baseColor}.Faces(p)
	wantYZ := [][2]float64{{100, 100}, {68, 116}, {68, 84}, {100, 68}}
	if !polyExact(yz[0].Poly, wantYZ) || yz[0].Color != rightColor {
		t.Fatalf("SideYZ = %v (%v), want %v (%v)", yz[0].Poly, yz[0].Color, wantYZ, rightColor)
	}
	xz := Side{Pos: V(0, 0, 0), W: 1, H: 1, Plane: SideXZ, Color: baseColor}.Faces(p)
	wantXZ := [][2]float64{{100, 100}, {132, 116}, {132, 84}, {100, 68}}
	if !polyExact(xz[0].Poly, wantXZ) || xz[0].Color != leftColor {
		t.Fatalf("SideXZ = %v (%v), want %v (%v)", xz[0].Poly, xz[0].Color, wantXZ, leftColor)
	}
}

func TestLineSegmentAndDepth(t *testing.T) {
	p := testProj()
	l := Line{From: V(0, 0, 0), To: V(1, 0, 0), Color: baseColor}
	a, b := l.Segment(p)
	if !ptExact(a, 100, 100) || !ptExact(b, 132, 116) {
		t.Fatalf("Line.Segment = %v,%v, want (100,100),(132,116)", a, b)
	}
	// Depth is the midpoint's sort key: midpoint (0.5,0,0) -> 0.5.
	if d := l.Depth(p); d != 0.5 {
		t.Fatalf("Line.Depth = %g, want 0.5", d)
	}
}

// --- shading factors (explicit and default) ----------------------------------

func TestShadingResolveAndExplicit(t *testing.T) {
	p := testProj()
	// A zero Shading falls back to DefaultShading (top == base).
	def := Cube{Pos: V(0, 0, 0), Size: 1, Color: baseColor}.Faces(p)
	if def[2].Color != topColor {
		t.Fatalf("default top color = %v, want %v", def[2].Color, topColor)
	}
	// An explicit Shading overrides it on every face.
	sh := Shading{Top: 0.5, Left: 0.25, Right: 0.1}
	ex := Brick{Pos: V(0, 0, 0), Dim: Dimension{1, 1, 1}, Color: baseColor, Shading: sh}.Faces(p)
	wantRight := color.RGBA{20, 12, 8, 255} // 200*.1, 120*.1, 80*.1
	wantLeft := color.RGBA{50, 30, 20, 255} // *.25
	wantTop := color.RGBA{100, 60, 40, 255} // *.5
	if ex[0].Color != wantRight || ex[1].Color != wantLeft || ex[2].Color != wantTop {
		t.Fatalf("explicit shading colors = %v,%v,%v", ex[0].Color, ex[1].Color, ex[2].Color)
	}
}

// --- scene render: shading pixels, z-order, strokes ---------------------------

func TestSceneShadingPixels(t *testing.T) {
	p := testProj()
	img := raster.New(220, 220)
	NewScene(p).Add(Cube{Pos: V(0, 0, 0), Size: 1, Color: baseColor}).Render(img)

	// Sample the interior (centroid) of each face; a fully covered pixel takes
	// the face's exact flat colour. Top brightest, left mid, right darkest.
	check := func(x, y int, want color.RGBA, face string) {
		if got := img.At(x, y); got != want {
			t.Fatalf("%s face pixel (%d,%d) = %v, want %v", face, x, y, got, want)
		}
	}
	check(100, 84, topColor, "top")
	check(84, 108, leftColor, "left")
	check(116, 108, rightColor, "right")

	// Ordering of brightness must hold: top lighter than left lighter than right.
	if !(topColor.R > leftColor.R && leftColor.R > rightColor.R) {
		t.Fatal("expected top > left > right brightness")
	}
}

func TestSceneDepthOrder(t *testing.T) {
	p := testProj()
	red := color.RGBA{R: 200, G: 40, B: 40, A: 255}
	blue := color.RGBA{R: 40, G: 40, B: 200, A: 255}

	// A at (0,0,0) and B at (1,1,1) project to the identical footprint (the view
	// direction), but B is nearer (Depth 3 > 0). Insert B first to prove Render
	// sorts by depth, not insertion order: the nearer blue cube must win.
	img := raster.New(220, 220)
	NewScene(p).
		Add(Cube{Pos: V(1, 1, 1), Size: 1, Color: blue}).
		Add(Cube{Pos: V(0, 0, 0), Size: 1, Color: red}).
		Render(img)
	if got := img.At(100, 84); got != blue {
		t.Fatalf("nearer cube should win: top pixel = %v, want blue %v", got, blue)
	}

	// Stable tie-break: two cubes at the SAME depth and footprint keep insertion
	// order, so the later-inserted (blue) is drawn last and wins.
	img2 := raster.New(220, 220)
	NewScene(p).
		Add(Cube{Pos: V(0, 0, 0), Size: 1, Color: red}).
		Add(Cube{Pos: V(0, 0, 0), Size: 1, Color: blue}).
		Render(img2)
	if got := img2.At(100, 84); got != blue {
		t.Fatalf("equal-depth tie-break: top pixel = %v, want blue %v", got, blue)
	}
}

func TestSceneCoversAllShapes(t *testing.T) {
	p := testProj()
	img := raster.New(220, 220)

	brick := Brick{Pos: V(0, 0, 0), Dim: Dimension{1, 2, 1}, Color: baseColor}
	pyr := Pyramid{Pos: V(2, 0, 0), Dim: Dimension{1, 1, 2}, Color: baseColor}
	slope := Slope{Pos: V(0, 2, 0), Dim: Dimension{1, 1, 1}, Dir: SlopeE, Color: baseColor}
	side := Side{Pos: V(3, 0, 0), W: 1, H: 1, Plane: SideXZ, Color: baseColor}
	// A brick placed far off the surface exercises the fill !ok (fully-clipped)
	// path without drawing anything.
	off := Brick{Pos: V(-100, -100, 0), Dim: Dimension{1, 1, 1}, Color: baseColor}

	NewScene(p).Add(brick, pyr, slope, side, off).Render(img) // must not panic

	// Each solid's Depth is its near-corner (or anchor) X+Y+Z sum.
	for _, tc := range []struct {
		got, want float64
		name      string
	}{
		{brick.Depth(p), 0, "brick"},
		{pyr.Depth(p), 2, "pyramid"},
		{slope.Depth(p), 2, "slope"},
		{side.Depth(p), 3, "side"},
		{off.Depth(p), -200, "off-screen brick"},
	} {
		if tc.got != tc.want {
			t.Fatalf("%s Depth = %g, want %g", tc.name, tc.got, tc.want)
		}
	}
}

func TestSceneStrokeAndSkips(t *testing.T) {
	p := testProj()
	img := raster.New(220, 220)
	green := color.RGBA{R: 20, G: 200, B: 60, A: 255}

	sc := NewScene(p).Add(
		Line{From: V(0, 0, 0), To: V(2, 0, 0), Color: green, Width: 9},                 // thick, on-screen
		Line{From: V(0, 0, 0), To: V(0, 0, 0), Color: green},                           // zero width -> defaults to 1
		Line{From: V(-50, -50, 0), To: V(-51, -51, 0), Color: green},                   // fully off-surface -> skipped
		Slope{Pos: V(0, 0, 3), Dim: Dimension{1, 1, 1}, Dir: SlopeW, Color: baseColor}, // degenerate +X face -> skipped
	)
	if got := sc.Shapes(); len(got) != 4 {
		t.Fatalf("Shapes() = %d, want 4", len(got))
	}
	sc.Render(img) // must not panic on the skipped shapes

	// The 9px on-screen line fully covers its centreline; its midpoint (132,116)
	// takes the exact stroke colour.
	if got := img.At(132, 116); got != green {
		t.Fatalf("stroked line midpoint = %v, want green %v", got, green)
	}
}
