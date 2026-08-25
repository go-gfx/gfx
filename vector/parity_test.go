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

// A named path builder, so a failure names the shape that diverged.
type namedPath struct {
	name string
	make func() *Path
}

// star builds a self-overlapping n-point star (NonZero fills it, EvenOdd cuts the
// centre out — exercises both winding rules on the same geometry).
func star(cx, cy, rOuter, rInner float64, points int) *Path {
	p := NewPath()
	for i := 0; i < points*2; i++ {
		r := rOuter
		if i%2 == 1 {
			r = rInner
		}
		a := math.Pi * float64(i) / float64(points)
		x, y := cx+r*math.Sin(a), cy-r*math.Cos(a)
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	return p.Close()
}

// pentagram builds a classic self-overlapping 5-point star by stepping every
// second vertex (144°). Its centre pentagon is wound twice: NonZero fills it,
// EvenOdd cuts it out — the shape that separates the two winding rules.
func pentagram(cx, cy, r float64) *Path {
	p := NewPath()
	for i := 0; i <= 5; i++ {
		ang := -math.Pi/2 + float64(i)*4*math.Pi/5
		x := cx + r*math.Cos(ang)
		y := cy + r*math.Sin(ang)
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	return p.Close()
}

// circle approximates a circle with four cubic quadrants (many flattened edges,
// the canonical curved fill).
func circle(cx, cy, r float64) *Path {
	const k = 0.5522847498307936
	o := r * k
	return NewPath().
		MoveTo(cx+r, cy).
		CubicTo(cx+r, cy+o, cx+o, cy+r, cx, cy+r).
		CubicTo(cx-o, cy+r, cx-r, cy+o, cx-r, cy).
		CubicTo(cx-r, cy-o, cx-o, cy-r, cx, cy-r).
		CubicTo(cx+o, cy-r, cx+r, cy-o, cx+r, cy).
		Close()
}

func ngon(cx, cy, r float64, n int) *Path {
	p := NewPath()
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		x, y := cx+r*math.Cos(a), cy+r*math.Sin(a)
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	return p.Close()
}

func zigzag(n int, w, h float64) *Path {
	p := NewPath()
	for i := 0; i < n; i++ {
		x := w * float64(i) / float64(n-1)
		y := h / 2
		if i%2 == 1 {
			y = h / 4
		}
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	return p
}

// parityPaths is the shared geometry table. It deliberately includes shapes with
// fractional (non-integer, non-half-integer) vertices, curves, self-overlap,
// holes, sub-paths, shapes spilling off every edge, shapes fully off-screen, and
// degenerate shapes — the cases feedback-prove-against-replaced-code calls for:
// ones that do NOT divide evenly, off each edge, and clipped.
func parityPaths() []namedPath {
	return []namedPath{
		{"empty", func() *Path { return NewPath() }},
		{"single-point", func() *Path { return NewPath().MoveTo(5, 5) }},
		{"rect-aligned", func() *Path {
			return NewPath().MoveTo(4, 4).LineTo(14, 4).LineTo(14, 12).LineTo(4, 12).Close()
		}},
		{"rect-fractional", func() *Path {
			return NewPath().MoveTo(3.37, 4.91).LineTo(15.2, 5.05).LineTo(14.8, 13.6).LineTo(2.1, 12.25).Close()
		}},
		{"triangle", func() *Path {
			return NewPath().MoveTo(5, 5).LineTo(25, 5).LineTo(5, 25).Close()
		}},
		{"triangle-slanted", func() *Path {
			return NewPath().MoveTo(2.5, 2.5).LineTo(30.1, 6.3).LineTo(6.7, 29.9).Close()
		}},
		{"diamond", func() *Path {
			return NewPath().MoveTo(20, 5).LineTo(35, 20).LineTo(20, 35).LineTo(5, 20).Close()
		}},
		{"quad-curve", func() *Path {
			return NewPath().MoveTo(4, 20).QuadTo(20, -6, 40, 20).LineTo(40, 30).LineTo(4, 30).Close()
		}},
		{"cubic-blob", func() *Path {
			return NewPath().MoveTo(8, 24).CubicTo(4, 4, 40, 4, 36, 24).CubicTo(40, 44, 4, 44, 8, 24).Close()
		}},
		{"circle-r25", func() *Path { return circle(30, 30, 25) }},
		{"circle-fractional", func() *Path { return circle(31.4, 29.7, 22.35) }},
		{"ngon-13", func() *Path { return ngon(30, 30, 24, 13) }},
		{"star-5", func() *Path { return star(30, 30, 26, 10, 5) }},
		{"star-8", func() *Path { return star(30, 30, 27, 11, 8) }},
		{"pentagram", func() *Path { return pentagram(30, 30, 26) }},
		{"two-subpaths", func() *Path {
			return NewPath().
				MoveTo(4, 4).LineTo(20, 4).LineTo(20, 20).LineTo(4, 20).Close().
				MoveTo(24, 24).LineTo(44, 24).LineTo(44, 44).LineTo(24, 44).Close()
		}},
		{"donut", func() *Path { // outer square + inner square (hole under EvenOdd)
			return NewPath().
				MoveTo(4, 4).LineTo(44, 4).LineTo(44, 44).LineTo(4, 44).Close().
				MoveTo(16, 16).LineTo(32, 16).LineTo(32, 32).LineTo(16, 32).Close()
		}},
		{"spill-right-bottom", func() *Path {
			return NewPath().MoveTo(30, 30).LineTo(80, 30).LineTo(80, 80).LineTo(30, 80).Close()
		}},
		{"spill-left-top", func() *Path {
			return NewPath().MoveTo(-20, -20).LineTo(20, -20).LineTo(20, 20).LineTo(-20, 20).Close()
		}},
		{"spill-all-sides", func() *Path {
			return NewPath().MoveTo(-10, -10).LineTo(90, -10).LineTo(90, 90).LineTo(-10, 90).Close()
		}},
		{"off-screen", func() *Path {
			return NewPath().MoveTo(-40, -40).LineTo(-20, -40).LineTo(-30, -20).Close()
		}},
		{"open-polyline", func() *Path { return zigzag(9, 60, 40) }},
		{"open-line", func() *Path { return NewPath().MoveTo(4, 6).LineTo(54, 33) }},
		{"closed-poly", func() *Path {
			return NewPath().MoveTo(6, 6).LineTo(50, 8).LineTo(30, 40).Close()
		}},
		{"dot-degenerate", func() *Path { return NewPath().MoveTo(20, 20).LineTo(20, 20) }},
		{"implicit-move", func() *Path { return NewPath().LineTo(30, 10).LineTo(10, 30) }},
		{"close-reopen", func() *Path {
			return NewPath().MoveTo(4, 4).LineTo(20, 4).Close().LineTo(20, 30).LineTo(4, 30).Close()
		}},
	}
}

// clamp surfaces: a generous one that contains most shapes, and tight ones that
// force the box to clip on each edge (exercises every clampBox branch).
var parityClamps = []struct {
	name string
	w, h int
}{
	{"large", 64, 64},
	{"tight", 24, 24},
	{"wide-short", 64, 10},
	{"tall-narrow", 10, 64},
	{"tiny", 3, 3},
	{"zero", 0, 0},
}

var parityRules = []struct {
	name string
	rule FillRule
}{
	{"nonzero", NonZero},
	{"evenodd", EvenOdd},
}

var parityWidths = []float64{0.75, 1, 2, 3.5, 6, 11}

// coversEqual reports the first index where two coverage grids differ bit-for-bit
// (via the raw float64 bits, so +0 vs -0 and any last-ULP drift is caught), or
// -1 when identical. Lengths must match.
func coversEqual(a, b []float64) int {
	if len(a) != len(b) {
		return 0
	}
	for i := range a {
		if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
			return i
		}
	}
	return -1
}

// TestFillParityAgainstReplacedCode sweeps every (path, rule, clamp) and asserts
// Rasterizer.Fill reproduces the verbatim old refFill byte-for-byte: same ok,
// same integer box, same coverage bits. One shared Rasterizer is reused across
// the whole sweep so the scratch grow/reuse/zero path is exercised too.
func TestFillParityAgainstReplacedCode(t *testing.T) {
	var rz Rasterizer
	total := 0
	for _, np := range parityPaths() {
		for _, r := range parityRules {
			for _, cl := range parityClamps {
				pth := np.make()
				wantCov, wox, woy, ww, wh, wok := refFill(pth, r.rule, cl.w, cl.h)
				gotCov, gox, goy, gw, gh, gok := rz.Fill(pth, r.rule, cl.w, cl.h)
				ctx := np.name + "/" + r.name + "/" + cl.name
				if gok != wok {
					t.Errorf("%s: ok = %v, want %v", ctx, gok, wok)
					continue
				}
				if !gok {
					continue
				}
				if gox != wox || goy != woy || gw != ww || gh != wh {
					t.Errorf("%s: box = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
						ctx, gox, goy, gw, gh, wox, woy, ww, wh)
					continue
				}
				if idx := coversEqual(gotCov, wantCov); idx >= 0 {
					t.Errorf("%s: coverage diverged at index %d: got %v want %v",
						ctx, idx, gotCov[idx], wantCov[idx])
				}
				total++
			}
		}
	}
	if total == 0 {
		t.Fatal("parity sweep asserted nothing")
	}
	t.Logf("fill parity: %d shape/rule/clamp combinations, all byte-identical", total)
}

// strokeParityTol is how far the two accounts of a stroke may differ. The
// reference samples sixty-four points along each sub-scanline where the
// rasteriser works the overlap out exactly, so it quantises by about a
// sixtieth; the rasteriser draws a round end as a many-sided polygon, so its
// rim falls up to a fiftieth of a pixel inside the circle. Neither slack is
// anywhere near the half a pixel's worth of coverage that a stroke assembled
// wrongly out of its pieces loses at every seam, which is what this is here to
// catch.
const strokeParityTol = 0.05

// coversClose reports the first index at which two coverage grids differ by
// more than tol, or -1 when they agree everywhere.
func coversClose(got, want []float64, tol float64) int {
	if len(got) != len(want) {
		return 0
	}
	for i := range got {
		if math.Abs(got[i]-want[i]) > tol {
			return i
		}
	}
	return -1
}

// TestStrokeParityAgainstReplacedCode is the stroke counterpart.
func TestStrokeParityAgainstReplacedCode(t *testing.T) {
	var rz Rasterizer
	total := 0
	for _, np := range parityPaths() {
		for _, width := range parityWidths {
			for _, cl := range parityClamps {
				pth := np.make()
				wantCov, wox, woy, ww, wh, wok := refStroke(pth, width, cl.w, cl.h)
				gotCov, gox, goy, gw, gh, gok := rz.Stroke(pth, width, cl.w, cl.h)
				ctx := np.name + "/" + ftoa(width) + "/" + cl.name
				if gok != wok {
					t.Errorf("%s: ok = %v, want %v", ctx, gok, wok)
					continue
				}
				if !gok {
					continue
				}
				if gox != wox || goy != woy || gw != ww || gh != wh {
					t.Errorf("%s: box = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
						ctx, gox, goy, gw, gh, wox, woy, ww, wh)
					continue
				}
				if idx := coversClose(gotCov, wantCov, strokeParityTol); idx >= 0 {
					t.Errorf("%s: coverage diverged at index %d: got %v want %v",
						ctx, idx, gotCov[idx], wantCov[idx])
				}
				total++
			}
		}
	}
	if total == 0 {
		t.Fatal("stroke parity sweep asserted nothing")
	}
	t.Logf("stroke parity: %d shape/width/clamp combinations, all within %v coverage", total, strokeParityTol)
}

func ftoa(f float64) string {
	// small helper: fixed 2-decimal label without importing strconv formatting
	// noise into the failure context.
	n := int(math.Round(f * 100))
	whole := n / 100
	frac := n % 100
	return itoa(whole) + "." + pad2(frac)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

// TestParityHarnessDetectsDifference controls the instrument: a coverage grid
// that differs from its reference by a single perturbed element MUST be reported
// unequal (feedback-control-run-new-instruments). Without this, a silently-broken
// comparator would pass the whole sweep vacuously.
func TestParityHarnessDetectsDifference(t *testing.T) {
	pth := circle(30, 30, 25)
	ref, _, _, _, _, ok := refFill(pth, NonZero, 64, 64)
	if !ok || len(ref) == 0 {
		t.Fatal("reference produced no coverage")
	}
	perturbed := append([]float64(nil), ref...)
	// find a covered pixel and nudge it by one ULP.
	idx := -1
	for i, v := range perturbed {
		if v > 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("no covered pixel to perturb")
	}
	perturbed[idx] = math.Nextafter(perturbed[idx], math.Inf(1))
	if coversEqual(ref, perturbed) != idx {
		t.Errorf("comparator failed to flag a one-ULP difference at %d", idx)
	}
	// identical inputs must report equal.
	if coversEqual(ref, append([]float64(nil), ref...)) != -1 {
		t.Error("comparator flagged identical grids as different")
	}
	// a length mismatch must report unequal (index 0).
	if coversEqual(ref, ref[:len(ref)-1]) != 0 {
		t.Error("comparator failed to flag a length mismatch")
	}
}
