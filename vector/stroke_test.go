package vector

import (
	"math"
	"testing"
)

// strokeBox rasterises a stroke and returns its coverage and box.
func strokeBox(t *testing.T, pth *Path, style StrokeStyle, size int) (cov []float64, ox, oy, w, h int) {
	t.Helper()
	var rz Rasterizer
	cov, ox, oy, w, h, ok := rz.StrokeWith(pth, style, size, size)
	if !ok {
		t.Fatal("the stroke produced nothing")
	}
	return cov, ox, oy, w, h
}

// at reports the coverage at a pixel of a stroke, zero outside its box.
func at(cov []float64, ox, oy, w, h, x, y int) float64 {
	if x < ox || y < oy || x >= ox+w || y >= oy+h {
		return 0
	}
	return cov[(y-oy)*w+(x-ox)]
}

func TestCapsReachDifferentDistances(t *testing.T) {
	// A horizontal line from (20,20) to (40,20), ten wide. Past the end point
	// there is nothing with a butt cap, a half disc with a round one, and a
	// full square with a square one.
	line := NewPath().MoveTo(20, 20).LineTo(40, 20)
	style := StrokeStyle{Width: 10}
	for _, c := range []struct {
		cap       LineCap
		wantAt44  bool
		wantWidth int
	}{
		{ButtCap, false, 20},
		{RoundCap, true, 30},
		{SquareCap, true, 30},
	} {
		style.Cap = c.cap
		cov, ox, oy, w, h := strokeBox(t, line, style, 64)
		if got := at(cov, ox, oy, w, h, 43, 20) > 0.5; got != c.wantAt44 {
			t.Errorf("cap %d: ink past the end = %v", c.cap, got)
		}
		if w != c.wantWidth {
			t.Errorf("cap %d: box is %d wide, want %d", c.cap, w, c.wantWidth)
		}
		// The line itself is always drawn.
		if at(cov, ox, oy, w, h, 30, 20) < 0.9 {
			t.Errorf("cap %d: the line is missing", c.cap)
		}
	}
	// A round cap is round: the corner of the square it would fill is empty.
	style.Cap = RoundCap
	cov, ox, oy, w, h := strokeBox(t, line, style, 64)
	if at(cov, ox, oy, w, h, 44, 24) > 0.1 {
		t.Error("a round cap filled the corner of its square")
	}
	style.Cap = SquareCap
	cov, ox, oy, w, h = strokeBox(t, line, style, 64)
	if at(cov, ox, oy, w, h, 44, 24) < 0.5 {
		t.Error("a square cap left its corner empty")
	}
}

func TestJoinsFillTheCornerDifferently(t *testing.T) {
	// A right angle at (30,30), coming from the left and turning upwards. The
	// outer corner is at (35,35) for a miter, cut off for a bevel.
	corner := NewPath().MoveTo(10, 30).LineTo(30, 30).LineTo(30, 10)
	style := StrokeStyle{Width: 10, Cap: ButtCap}

	style.Join = MiterJoin
	cov, ox, oy, w, h := strokeBox(t, corner, style, 64)
	if at(cov, ox, oy, w, h, 34, 34) < 0.5 {
		t.Error("a miter join left its own corner empty")
	}

	style.Join = BevelJoin
	cov, ox, oy, w, h = strokeBox(t, corner, style, 64)
	if at(cov, ox, oy, w, h, 34, 34) > 0.1 {
		t.Error("a bevel join filled the corner a miter would")
	}
	// But the corner itself is still covered.
	if at(cov, ox, oy, w, h, 31, 31) < 0.5 {
		t.Error("a bevel join left a hole at the corner")
	}

	style.Join = RoundJoin
	cov, ox, oy, w, h = strokeBox(t, corner, style, 64)
	if at(cov, ox, oy, w, h, 34, 34) > 0.2 {
		t.Error("a round join reached as far as a miter")
	}
	if at(cov, ox, oy, w, h, 33, 31) < 0.5 {
		t.Error("a round join left a hole near the corner")
	}
}

func TestAMiterTooSharpBecomesABevel(t *testing.T) {
	// A hairpin: the miter would run far past the corner, so the limit turns
	// it into a bevel.
	hairpin := NewPath().MoveTo(10, 30).LineTo(50, 31).LineTo(10, 32)
	tight := StrokeStyle{Width: 8, Join: MiterJoin, MiterLimit: 2}
	loose := StrokeStyle{Width: 8, Join: MiterJoin, MiterLimit: 100}
	_, _, _, tw, _ := strokeBox(t, hairpin, tight, 400)
	_, _, _, lw, _ := strokeBox(t, hairpin, loose, 400)
	if lw <= tw {
		t.Errorf("the limit made no difference: %d wide against %d", lw, tw)
	}
	// And the default limit is the usual ten.
	def := StrokeStyle{Width: 8, Join: MiterJoin}
	_, _, _, dw, _ := strokeBox(t, hairpin, def, 400)
	explicit := StrokeStyle{Width: 8, Join: MiterJoin, MiterLimit: defaultMiterLimit}
	_, _, _, ew, _ := strokeBox(t, hairpin, explicit, 400)
	if dw != ew {
		t.Errorf("the default limit is not %d: %d against %d", defaultMiterLimit, dw, ew)
	}
}

func TestDashesLeaveGaps(t *testing.T) {
	// Ten on, ten off, along a line from x=0 to x=100 at y=20.
	line := NewPath().MoveTo(0, 20).LineTo(100, 20)
	style := StrokeStyle{Width: 4, Dash: []float64{10, 10}}
	cov, ox, oy, w, h := strokeBox(t, line, style, 128)
	for _, c := range []struct {
		x    int
		want bool
	}{{5, true}, {15, false}, {25, true}, {35, false}, {95, false}} {
		if got := at(cov, ox, oy, w, h, c.x, 20) > 0.5; got != c.want {
			t.Errorf("at x=%d: ink = %v, want %v", c.x, got, c.want)
		}
	}
	// The phase moves the pattern along.
	style.DashPhase = 10
	cov, ox, oy, w, h = strokeBox(t, line, style, 128)
	if at(cov, ox, oy, w, h, 5, 20) > 0.5 {
		t.Error("a phase of ten did not start in a gap")
	}
	if at(cov, ox, oy, w, h, 15, 20) < 0.5 {
		t.Error("a phase of ten did not put ink where the gap was")
	}
}

func TestAnOddDashPatternRepeats(t *testing.T) {
	// A single length means equal dashes and gaps.
	line := NewPath().MoveTo(0, 20).LineTo(100, 20)
	single := StrokeStyle{Width: 4, Dash: []float64{10}}
	pair := StrokeStyle{Width: 4, Dash: []float64{10, 10}}
	a, aox, aoy, aw, ah := strokeBox(t, line, single, 128)
	b, box, boy, bw, bh := strokeBox(t, line, pair, 128)
	for x := 0; x < 100; x += 3 {
		if (at(a, aox, aoy, aw, ah, x, 20) > 0.5) != (at(b, box, boy, bw, bh, x, 20) > 0.5) {
			t.Fatalf("the two patterns differ at x=%d", x)
		}
	}
}

func TestDashesThatAreNoDashes(t *testing.T) {
	line := NewPath().MoveTo(0, 20).LineTo(60, 20)
	solid, sox, soy, sw, sh := strokeBox(t, line, StrokeStyle{Width: 4}, 128)
	for _, dash := range [][]float64{{}, {0, 0}, {-5, 5}} {
		cov, ox, oy, w, h := strokeBox(t, line, StrokeStyle{Width: 4, Dash: dash}, 128)
		if w != sw || h != sh {
			t.Errorf("%v: box %dx%d, want %dx%d", dash, w, h, sw, sh)
		}
		if at(cov, ox, oy, w, h, 30, 20) != at(solid, sox, soy, sw, sh, 30, 20) {
			t.Errorf("%v: the line is not solid", dash)
		}
	}
}

func TestADashPatternWithAZeroLengthInIt(t *testing.T) {
	// A zero length must be stepped over rather than looped on.
	line := NewPath().MoveTo(0, 20).LineTo(60, 20)
	cov, ox, oy, w, h := strokeBox(t, line, StrokeStyle{Width: 4, Dash: []float64{10, 0, 10}}, 128)
	if at(cov, ox, oy, w, h, 5, 20) < 0.5 {
		t.Error("the first dash is missing")
	}
}

func TestAClosedPathJoinsWhereItMeetsItself(t *testing.T) {
	square := NewPath().MoveTo(20, 20).LineTo(40, 20).LineTo(40, 40).LineTo(20, 40).Close()
	style := StrokeStyle{Width: 8, Join: MiterJoin}
	cov, ox, oy, w, h := strokeBox(t, square, style, 64)
	// Every one of the four corners is mitred, the starting one included.
	for _, c := range [][2]int{{17, 17}, {43, 17}, {43, 43}, {17, 43}} {
		if at(cov, ox, oy, w, h, c[0], c[1]) < 0.5 {
			t.Errorf("corner %v is empty", c)
		}
	}
}

func TestStrokeWithRefusesWhatCannotBeStroked(t *testing.T) {
	var rz Rasterizer
	for _, c := range []struct {
		name  string
		pth   *Path
		style StrokeStyle
	}{
		{"no path", nil, StrokeStyle{Width: 4}},
		{"no width", NewPath().MoveTo(0, 0).LineTo(10, 10), StrokeStyle{}},
		{"a lone point", NewPath().MoveTo(5, 5), StrokeStyle{Width: 4}},
		{"nothing at all", NewPath(), StrokeStyle{Width: 4}},
		{"off the surface", NewPath().MoveTo(500, 500).LineTo(600, 600), StrokeStyle{Width: 4}},
	} {
		if _, _, _, _, _, ok := rz.StrokeWith(c.pth, c.style, 64, 64); ok {
			t.Errorf("%s: the stroke produced something", c.name)
		}
	}
}

func TestStrokeIsStrokeWithRoundEverything(t *testing.T) {
	var a, b Rasterizer
	pth := NewPath().MoveTo(10, 10).LineTo(40, 20).LineTo(20, 40)
	cov1, ox1, oy1, w1, h1, ok1 := a.Stroke(pth, 7, 64, 64)
	cov2, ox2, oy2, w2, h2, ok2 := b.StrokeWith(pth, StrokeStyle{Width: 7, Cap: RoundCap, Join: RoundJoin}, 64, 64)
	if !ok1 || !ok2 || ox1 != ox2 || oy1 != oy2 || w1 != w2 || h1 != h2 {
		t.Fatalf("boxes differ: %v %d,%d %dx%d against %v %d,%d %dx%d", ok1, ox1, oy1, w1, h1, ok2, ox2, oy2, w2, h2)
	}
	for i := range cov1 {
		if cov1[i] != cov2[i] {
			t.Fatalf("coverage differs at %d", i)
		}
	}
}

func TestJoinsThatAreNotCorners(t *testing.T) {
	// Three points in a straight line have no corner to fill, and a path that
	// doubles back on itself has none either.
	for _, pth := range []*Path{
		NewPath().MoveTo(10, 20).LineTo(20, 20).LineTo(30, 20),
		NewPath().MoveTo(10, 20).LineTo(30, 20).LineTo(10, 20),
	} {
		if _, _, _, _, _, ok := (&Rasterizer{}).StrokeWith(pth, StrokeStyle{Width: 6, Join: MiterJoin}, 64, 64); !ok {
			t.Error("the stroke produced nothing")
		}
	}
}

func TestUnitAndPolyEdges(t *testing.T) {
	if x, y, ok := unit(3, 4); !ok || math.Abs(x-0.6) > 1e-9 || math.Abs(y-0.8) > 1e-9 {
		t.Errorf("unit(3,4) = %g, %g, %v", x, y, ok)
	}
	if _, _, ok := unit(0, 0); ok {
		t.Error("a vector of no length has a direction")
	}
	if got := polyEdges([]point{{0, 0}, {1, 0}, {1, 1}}); len(got) != 3 {
		t.Errorf("polyEdges gave %d edges", len(got))
	}
}

func TestDashStartWalksThePattern(t *testing.T) {
	pattern := []float64{10, 5, 3, 2}
	cases := []struct {
		phase float64
		index int
		on    bool
		left  float64
	}{
		{0, 0, true, 10},
		{4, 0, true, 6},
		{12, 1, false, 3},
		{16, 2, true, 2},
		{19, 3, false, 1},
		{20, 0, true, 10}, // the pattern comes round again
		{-4, 0, true, 10}, // a phase below zero is no phase at all
	}
	for _, c := range cases {
		index, on, left := dashStart(pattern, c.phase)
		if index != c.index || on != c.on || math.Abs(left-c.left) > 1e-9 {
			t.Errorf("phase %g: %d, %v, %g — want %d, %v, %g", c.phase, index, on, left, c.index, c.on, c.left)
		}
	}
}

func TestASquareCapOnARunThatGoesNowhere(t *testing.T) {
	// Two points in the same place: there is no direction, and the cap still
	// has to land somewhere rather than divide by nothing.
	pth := NewPath().MoveTo(20, 20).LineTo(20, 20)
	if _, _, _, _, _, ok := (&Rasterizer{}).StrokeWith(pth, StrokeStyle{Width: 6, Cap: SquareCap}, 64, 64); !ok {
		t.Error("the stroke produced nothing")
	}
}

func TestAJoinBesideASegmentOfNoLength(t *testing.T) {
	// A repeated point leaves a corner with no direction on one side of it.
	pth := NewPath().MoveTo(10, 20).LineTo(20, 20).LineTo(20, 20).LineTo(20, 30)
	if _, _, _, _, _, ok := (&Rasterizer{}).StrokeWith(pth, StrokeStyle{Width: 6, Join: MiterJoin}, 64, 64); !ok {
		t.Error("the stroke produced nothing")
	}
}
