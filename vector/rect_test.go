// Copyright (c) 2026 the go-gfx/gfx authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

import (
	"math/rand"
	"testing"
)

// fillRect is the rectangle drawn the long way, through the rasteriser, which
// is what Rect has to agree with.
func fillRect(t *testing.T, rz *Rasterizer, r Rect, clampW, clampH int) (cov []float64, ox, oy, w, h int, ok bool) {
	t.Helper()
	p := NewPath()
	p.MoveTo(r.X0, r.Y0)
	p.LineTo(r.X1, r.Y0)
	p.LineTo(r.X1, r.Y1)
	p.LineTo(r.X0, r.Y1)
	p.Close()
	return rz.Fill(p, NonZero, clampW, clampH)
}

// TestRectAgreesWithFill is the whole point of the type: what it returns is
// what the rasteriser would have put in the grid, BIT FOR BIT, at every pixel
// of every rectangle it is asked about. Anything less makes it an
// approximation, and a renderer that swapped one for the other would draw a
// different picture.
func TestRectAgreesWithFill(t *testing.T) {
	const clampW, clampH = 40, 30
	rz := &Rasterizer{}
	checked := 0

	check := func(name string, r Rect) {
		t.Helper()
		cov, ox, oy, w, h, ok := fillRect(t, rz, r, clampW, clampH)
		bx, by, bw, bh, bok := r.Box(clampW, clampH)
		if ok != bok || (ok && (bx != ox || by != oy || bw != w || bh != h)) {
			t.Errorf("%s %+v: Box gives (%d,%d,%d,%d,%v), Fill gives (%d,%d,%d,%d,%v)",
				name, r, bx, by, bw, bh, bok, ox, oy, w, h, ok)
			return
		}
		if !ok {
			return
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				want := cov[y*w+x]
				got := r.At(ox+x, oy+y)
				checked++
				if got != want {
					t.Fatalf("%s %+v pixel (%d,%d): Fill %.17g, Rect %.17g",
						name, r, ox+x, oy+y, want, got)
				}
			}
		}
	}

	// Whole pixels, so every value is 0 or 1.
	check("pixel-aligned", Rect{2, 3, 12, 9})
	// Fractional on every side, so every edge is partly covered.
	check("fractional", Rect{2.37, 3.11, 12.62, 9.83})
	// Thinner than a pixel, in each direction and both.
	check("thin in x", Rect{5.2, 3, 5.4, 9})
	check("thin in y", Rect{2, 5.2, 12, 5.4})
	check("thin in both", Rect{5.2, 5.2, 5.4, 5.4})
	// Inside one pixel.
	check("inside one pixel", Rect{5.25, 5.25, 5.75, 5.75})
	// Off the surface on each side, so the box is clamped.
	check("off the left", Rect{-8, 3, 6, 9})
	check("off the top", Rect{2, -8, 12, 6})
	check("off the right", Rect{30, 3, 90, 9})
	check("off the bottom", Rect{2, 20, 12, 90})
	check("larger than the surface", Rect{-10, -10, 90, 90})
	// Sub-scanlines sit at y+0.125, 0.375, 0.625, 0.875: a rectangle whose
	// edge falls exactly on one is where a comparison that is < rather than
	// <= goes wrong.
	for _, e := range []float64{0, 0.125, 0.375, 0.5, 0.625, 0.875, 1} {
		check("edge on a sub-scanline", Rect{2, 4 + e, 12, 8 + e})
	}

	rnd := rand.New(rand.NewSource(3))
	for i := 0; i < 400; i++ {
		x0 := rnd.Float64()*50 - 5
		y0 := rnd.Float64()*40 - 5
		r := Rect{x0, y0, x0 + rnd.Float64()*20, y0 + rnd.Float64()*15}
		check("random", r)
	}
	if checked < 10000 {
		t.Errorf("only %d pixels compared; the sweep is not reaching the rectangles", checked)
	}
	t.Logf("%d pixels agreed bit for bit", checked)
}

// TestRectBoxRefusesAnEmptyRectangle. A rectangle with no area covers nothing,
// and the rasteriser says so by returning ok=false rather than a grid of
// zeroes.
func TestRectBoxRefusesAnEmptyRectangle(t *testing.T) {
	for _, r := range []Rect{
		{5, 5, 5, 9}, {5, 5, 12, 5}, {5, 5, 5, 5}, {12, 9, 5, 5},
	} {
		if _, _, _, _, ok := r.Box(40, 30); ok {
			t.Errorf("%+v: Box says it is on the surface", r)
		}
		if got := r.At(5, 5); got != 0 {
			t.Errorf("%+v: At = %v, want 0", r, got)
		}
	}
}

// TestRectWholeIsSoundAndComplete. Whole is an optimisation: a caller trusts
// it and skips the arithmetic, so a pixel it claims that is not entirely
// covered is a wrong picture. It is checked against At at every pixel of the
// box Fill would produce -- both directions, because a Whole that claimed
// nothing would be sound and useless.
func TestRectWholeIsSoundAndComplete(t *testing.T) {
	const clampW, clampH = 40, 30
	rz := &Rasterizer{}
	claimed, whole := 0, 0

	check := func(name string, r Rect) {
		t.Helper()
		_, ox, oy, w, h, ok := fillRect(t, rz, r, clampW, clampH)
		if !ok {
			return
		}
		wx, wy, ww, wh, wok := r.Whole(clampW, clampH)
		for y := oy; y < oy+h; y++ {
			for x := ox; x < ox+w; x++ {
				in := wok && x >= wx && x < wx+ww && y >= wy && y < wy+wh
				at := r.At(x, y)
				if in {
					claimed++
					if at != 1 {
						t.Fatalf("%s %+v: Whole claims (%d,%d) but At = %.17g", name, r, x, y, at)
					}
				}
				if at == 1 {
					whole++
					if !in {
						t.Fatalf("%s %+v: At(%d,%d) = 1 and Whole leaves it out", name, r, x, y)
					}
				}
			}
		}
	}

	check("pixel-aligned", Rect{2, 3, 12, 9})
	check("fractional", Rect{2.37, 3.11, 12.62, 9.83})
	check("thin in x", Rect{5.2, 3, 5.4, 9})
	check("thin in y", Rect{2, 5.2, 12, 5.4})
	check("off the left", Rect{-8, 3, 6, 9})
	check("larger than the surface", Rect{-10, -10, 90, 90})
	for _, e := range []float64{0, 0.125, 0.375, 0.5, 0.625, 0.875, 1} {
		check("edge on a sub-scanline", Rect{2, 4 + e, 12, 8 + e})
	}
	rnd := rand.New(rand.NewSource(5))
	for i := 0; i < 400; i++ {
		x0 := rnd.Float64()*50 - 5
		y0 := rnd.Float64()*40 - 5
		check("random", Rect{x0, y0, x0 + rnd.Float64()*20, y0 + rnd.Float64()*15})
	}
	if claimed != whole {
		t.Errorf("Whole claims %d pixels, At says %d are covered entirely", claimed, whole)
	}
	if claimed < 5000 {
		t.Errorf("only %d pixels claimed; the optimisation would not pay", claimed)
	}
	t.Logf("%d pixels covered entirely, all of them claimed", claimed)
}
