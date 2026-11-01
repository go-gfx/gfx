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

// steps flattens a Path into something a test can assert on.
type step struct {
	op  PathOp
	pts [][2]float64
}

func steps(p *Path) []step {
	var out []step
	p.Each(func(op PathOp, pts ...[2]float64) {
		out = append(out, step{op, append([][2]float64(nil), pts...)})
	})
	return out
}

// close2 compares two points to a tolerance that is loose for geometry and far
// tighter than any of the mistakes these tests are about.
func close2(a, b [2]float64) bool {
	return math.Abs(a[0]-b[0]) < 1e-9 && math.Abs(a[1]-b[1]) < 1e-9
}

// TestAShearedTransformTellsBFromC is the test the SVG suite did not have.
//
// ⛔ MEASURED, not supposed: swapping m[1] with m[2] in Apply leaves the whole
// of gfx/svg's suite green, because every drawing in it is axis-aligned and
// b == c == 0 there. A transform whose b and c DIFFER is the only witness that
// separates the two readings of [a b c d e f], and without one this package
// would ship a parser that mirrors every rotated drawing it is given.
//
// The expected points are worked out from the definition in Affine's own doc
// comment -- (a*x + c*y + e, b*x + d*y + f) -- and not from running the code,
// so the test can disagree with the implementation.
func TestAShearedTransformTellsBFromC(t *testing.T) {
	t.Parallel()

	// b = 0.25 and c = 0.75: different, so the two readings diverge.
	m := Affine{1, 0.25, 0.75, 1, 0, 0}
	p, ok := ParseData("M0 0L4 0L0 4", m)
	if !ok {
		t.Fatal("the data was refused")
	}
	got := steps(p)
	want := [][2]float64{
		{0, 0}, // (1*0 + 0.75*0, 0.25*0 + 1*0)
		{4, 1}, // (1*4 + 0.75*0, 0.25*4 + 1*0)
		{3, 4}, // (1*0 + 0.75*4, 0.25*0 + 1*4)
	}
	if len(got) != len(want) {
		t.Fatalf("%d steps, want %d", len(got), len(want))
	}
	for i := range want {
		if !close2(got[i].pts[0], want[i]) {
			t.Errorf("point %d is %v, want %v — b and c are being read the wrong way round",
				i, got[i].pts[0], want[i])
		}
	}
}

func TestIdentityChangesNothing(t *testing.T) {
	t.Parallel()

	p, ok := ParseData("M1 2L3 4", Identity())
	if !ok {
		t.Fatal("refused")
	}
	got := steps(p)
	for i, want := range [][2]float64{{1, 2}, {3, 4}} {
		if !close2(got[i].pts[0], want) {
			t.Errorf("point %d is %v, want %v", i, got[i].pts[0], want)
		}
	}
}

// TestTheCommandsOpenXPSAlsoWrites covers the subset ECMA-388 Annex C defines
// as the abbreviated geometry syntax, so a change here that broke an XPS or
// ODF reader fails in this package rather than in theirs.
func TestTheCommandsOpenXPSAlsoWrites(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		why  string
		data string
		ops  int
	}{
		{"move and line, absolute", "M0 0L10 0", 2},
		{"move and line, relative", "m0 0l10 0", 2},
		{"horizontal and vertical", "M0 0H10V10", 3},
		{"cubic", "M0 0C1 2 3 4 5 6", 2},
		{"smooth cubic", "M0 0C1 2 3 4 5 6S7 8 9 10", 3},
		{"quadratic", "M0 0Q1 2 3 4", 2},
		// ⚠ Three, not two. The chord from (0,0) to (10,10) is longer than
		// 2*r, so the radii are scaled up to the half-chord and the arc is a
		// SEMICIRCLE -- two quarter-turn cubics after the move. Written as two
		// first; the code was right and the expectation was not.
		{"arc", "M0 0A5 5 0 0 1 10 10", 3},
		{"arc with the flags run together", "M0 0A5 5 0 0110 10", 3},
		{"close", "M0 0L10 0Z", 3},
		{"two subpaths", "M0 0L1 1M5 5L6 6", 4},
	} {
		p, ok := ParseData(tc.data, Identity())
		if !ok {
			t.Errorf("%s: %q was refused", tc.why, tc.data)
			continue
		}
		if n := len(steps(p)); n != tc.ops {
			t.Errorf("%s: %q gave %d steps, want %d", tc.why, tc.data, n, tc.ops)
		}
	}
}

// TestDataThatDrawsNothingIsRefused pins the distinction a caller depends on:
// "I could not read this" and "this draws nothing" must both come back false,
// because a caller that got an empty Path would paint nothing while believing
// it had painted something.
func TestDataThatDrawsNothingIsRefused(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ why, data string }{
		{"empty", ""},
		{"a move alone", "M0 0"},
		{"a move and a close", "M0 0Z"},
		{"a draw with no move", "L10 10"},
		{"a letter that is not a command", "X10 10"},
		{"truncated", "M0 0L"},
		{"an arc flag that is neither 0 nor 1", "M0 0A5 5 0 2 1 10 10"},
	} {
		if _, ok := ParseData(tc.data, Identity()); ok {
			t.Errorf("%s: %q was accepted", tc.why, tc.data)
		}
	}
}

// TestTheCompactNumberFormsSVGAllows covers the scanning that a hand-written
// reader gets wrong: a second decimal point starts the next number, and a sign
// separates without whitespace.
func TestTheCompactNumberFormsSVGAllows(t *testing.T) {
	t.Parallel()

	p, ok := ParseData("M0 0L1.4.1", Identity())
	if !ok {
		t.Fatal(`"1.4.1" was refused; it is two numbers, 1.4 and .1`)
	}
	if got := steps(p)[1].pts[0]; !close2(got, [2]float64{1.4, 0.1}) {
		t.Errorf("got %v, want [1.4 0.1]", got)
	}

	p, ok = ParseData("M0 0L+1-2", Identity())
	if !ok {
		t.Fatal(`"+1-2" was refused; a sign separates numbers`)
	}
	if got := steps(p)[1].pts[0]; !close2(got, [2]float64{1, -2}) {
		t.Errorf("got %v, want [1 -2]", got)
	}
}

// TestADegenerateArcIsSkippedNotRefused: a zero radius or coincident endpoints
// describe no curve, and ECMA-388 and SVG both leave the subpath continuing
// rather than failing. The shape must still come back.
func TestADegenerateArcIsSkippedNotRefused(t *testing.T) {
	t.Parallel()

	for _, data := range []string{
		"M0 0L1 1A0 5 0 0 1 10 10",
		"M0 0L1 1A5 5 0 0 1 1 1",
	} {
		if _, ok := ParseData(data, Identity()); !ok {
			t.Errorf("%q was refused; a degenerate arc is skipped, not fatal", data)
		}
	}
}
