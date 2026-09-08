// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import (
	"math"
	"math/rand"
	"testing"
)

// popplerUnrolled is poppler's cmykToRGBMatrixMultiplication transcribed, from
// poppler/GfxState_helpers.h. It is here as the ORACLE for the rewrite beside
// it: the loop in cmyk.go is a table and an interpolation where this is
// sixteen hand-written accumulations, and the only way to say the two are the
// same thing is to run both.
func popplerUnrolled(c, m, y, k float64) (r, g, b float64) {
	c1, m1, y1, k1 := 1-c, 1-m, 1-y, 1-k
	var x float64
	x = c1 * m1 * y1 * k1
	r, g, b = x, x, x
	x = c1 * m1 * y1 * k
	r += 0.1373 * x
	g += 0.1216 * x
	b += 0.1255 * x
	x = c1 * m1 * y * k1
	r += x
	g += 0.9490 * x
	x = c1 * m1 * y * k
	r += 0.1098 * x
	g += 0.1020 * x
	x = c1 * m * y1 * k1
	r += 0.9255 * x
	b += 0.5490 * x
	x = c1 * m * y1 * k
	r += 0.1412 * x
	x = c1 * m * y * k1
	r += 0.9294 * x
	g += 0.1098 * x
	b += 0.1412 * x
	x = c1 * m * y * k
	r += 0.1333 * x
	x = c * m1 * y1 * k1
	g += 0.6784 * x
	b += 0.9373 * x
	x = c * m1 * y1 * k
	g += 0.0588 * x
	b += 0.1412 * x
	x = c * m1 * y * k1
	g += 0.6510 * x
	b += 0.3137 * x
	x = c * m1 * y * k
	g += 0.0745 * x
	x = c * m * y1 * k1
	r += 0.1804 * x
	g += 0.1922 * x
	b += 0.5725 * x
	x = c * m * y1 * k
	b += 0.0078 * x
	x = c * m * y * k1
	r += 0.2118 * x
	g += 0.2119 * x
	b += 0.2235 * x
	return r, g, b
}

func TestTheWebCoatedInterpolationIsPopplersMatrix(t *testing.T) {
	worst := 0.0
	check := func(c, m, y, k float64) {
		t.Helper()
		wr, wg, wb := CMYKToSRGBWebCoated(CMYK{c, m, y, k})
		pr, pg, pb := popplerUnrolled(c, m, y, k)
		for _, d := range []float64{math.Abs(wr - pr), math.Abs(wg - pg), math.Abs(wb - pb)} {
			if d > worst {
				worst = d
			}
			// A whole level of 255 is 1/255; this has to agree far inside that,
			// or the rewrite is a different transform wearing the same name.
			if d > 1e-12 {
				t.Fatalf("c=%g m=%g y=%g k=%g: got %g,%g,%g want %g,%g,%g",
					c, m, y, k, wr, wg, wb, pr, pg, pb)
			}
		}
	}
	for c := 0; c <= 4; c++ {
		for m := 0; m <= 4; m++ {
			for y := 0; y <= 4; y++ {
				for k := 0; k <= 4; k++ {
					check(float64(c)/4, float64(m)/4, float64(y)/4, float64(k)/4)
				}
			}
		}
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 20000; i++ {
		check(rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64())
	}
	t.Logf("worst channel difference over 20625 points: %g", worst)
}

func TestTheWebCoatedCornersAreThePrintingPrimaries(t *testing.T) {
	// If these ever stop being the SWOP inks, the table has been edited into
	// something else and the name on the function is a lie.
	for _, tc := range []struct {
		name string
		in   CMYK
		want [3]int
	}{
		{"paper", CMYK{}, [3]int{255, 255, 255}},
		{"cyan", CMYK{C: 1}, [3]int{0, 173, 239}},
		{"magenta", CMYK{M: 1}, [3]int{236, 0, 140}},
		{"yellow", CMYK{Y: 1}, [3]int{255, 242, 0}},
		{"key", CMYK{K: 1}, [3]int{35, 31, 32}},
		{"all four", CMYK{1, 1, 1, 1}, [3]int{0, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, g, b := CMYKToSRGBWebCoated(tc.in)
			got := [3]int{level(r), level(g), level(b)}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func level(v float64) int { return int(math.Round(v * 255)) }

func TestInkOutsideTheCubeIsClamped(t *testing.T) {
	// A /Decode array can hand a renderer a value outside [0,1], and four
	// corners say nothing about what is beyond them.
	over, _, _ := CMYKToSRGBWebCoated(CMYK{C: 2, M: -1, Y: 0.5, K: -0.25})
	at, _, _ := CMYKToSRGBWebCoated(CMYK{C: 1, M: 0, Y: 0.5, K: 0})
	if over != at {
		t.Errorf("ink outside the cube gave %g, want the edge's %g", over, at)
	}
}

func TestTheNaiveModelAndTheCoatedOneAreDifferentTransforms(t *testing.T) {
	// Both are kept, so something has to say they are not interchangeable:
	// over the cube the naive formula sits a mean of 27 levels away.
	sum, worst, n := 0.0, 0.0, 0
	for c := 0; c <= 4; c++ {
		for m := 0; m <= 4; m++ {
			for y := 0; y <= 4; y++ {
				for k := 0; k <= 4; k++ {
					in := CMYK{float64(c) / 4, float64(m) / 4, float64(y) / 4, float64(k) / 4}
					nr, ng, nb := CMYKToSRGB(in)
					wr, wg, wb := CMYKToSRGBWebCoated(in)
					d := math.Max(math.Abs(nr-wr), math.Max(math.Abs(ng-wg), math.Abs(nb-wb))) * 255
					sum += d
					worst = math.Max(worst, d)
					n++
				}
			}
		}
	}
	mean := sum / float64(n)
	if mean < 20 || worst < 100 {
		t.Errorf("the two models differ by a mean of %.1f and at most %.1f levels; "+
			"they were 27 and 115 when this was written, and a collapse means one "+
			"of them has been changed into the other", mean, worst)
	}
}
