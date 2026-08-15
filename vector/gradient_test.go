// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

import (
	"image/color"
	"testing"
)

func TestSolidPaint(t *testing.T) {
	c := color.RGBA{10, 20, 30, 40}
	if got := (SolidPaint{c}).ColorAt(3, 7); got != c {
		t.Fatalf("SolidPaint = %v, want %v", got, c)
	}
}

func TestApplySpread(t *testing.T) {
	cases := []struct {
		method  SpreadMethod
		t, want float64
	}{
		{Pad, -0.5, 0}, {Pad, 1.5, 1}, {Pad, 0.5, 0.5},
		{Repeat, 1.25, 0.25}, {Repeat, -0.25, 0.75}, {Repeat, 0.4, 0.4},
		{Reflect, 1.5, 0.5}, {Reflect, -0.5, 0.5}, {Reflect, 0.5, 0.5},
	}
	for _, c := range cases {
		if got := applySpread(c.t, c.method); got != c.want {
			t.Fatalf("applySpread(%v, %d) = %v, want %v", c.t, c.method, got, c.want)
		}
	}
}

func TestSampleStops(t *testing.T) {
	// Empty list is transparent.
	if got := sampleStops(nil, 0.5); got != (color.RGBA{}) {
		t.Fatalf("empty stops = %v, want zero", got)
	}
	// Single stop is that colour regardless of t.
	one := []Stop{{0.3, color.RGBA{1, 2, 3, 4}}}
	if got := sampleStops(one, 0.9); got != (color.RGBA{1, 2, 3, 4}) {
		t.Fatalf("single stop = %v", got)
	}
	stops := []Stop{
		{0, color.RGBA{0, 0, 0, 255}},
		{0.5, color.RGBA{200, 100, 50, 255}},
		{1, color.RGBA{100, 100, 100, 255}},
	}
	// Below the first offset clamps to the first stop.
	if got := sampleStops(stops, -1); got != stops[0].Color {
		t.Fatalf("t<first = %v, want first", got)
	}
	// At/above the last offset clamps to the last stop.
	if got := sampleStops(stops, 2); got != stops[2].Color {
		t.Fatalf("t>last = %v, want last", got)
	}
	// Midpoint of the first segment interpolates each channel.
	if got := sampleStops(stops, 0.25); got != (color.RGBA{100, 50, 25, 255}) {
		t.Fatalf("mid-first-segment = %v, want {100 50 25 255}", got)
	}
	// A t in the SECOND segment advances the search loop.
	if got := sampleStops(stops, 0.75); got != (color.RGBA{150, 100, 75, 255}) {
		t.Fatalf("mid-second-segment = %v, want {150 100 75 255}", got)
	}
}

func TestLinearGradient(t *testing.T) {
	g := NewLinearGradient(0, 0, 10, 0, Pad,
		// Deliberately unsorted to exercise normStops.
		Stop{1, color.RGBA{255, 255, 255, 255}},
		Stop{0, color.RGBA{0, 0, 0, 255}},
	)
	if got := g.ColorAt(0, 0); got != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("start = %v, want black", got)
	}
	if got := g.ColorAt(10, 0); got != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("end = %v, want white", got)
	}
	if got := g.ColorAt(5, 0); got != (color.RGBA{128, 128, 128, 255}) {
		t.Fatalf("mid = %v, want mid grey", got)
	}
	// Off-axis: only the projection onto the axis matters.
	if got := g.ColorAt(5, 99); got != (color.RGBA{128, 128, 128, 255}) {
		t.Fatalf("off-axis mid = %v, want mid grey", got)
	}
}

func TestLinearGradientDegenerate(t *testing.T) {
	// Zero-length axis: every point is the first stop's colour.
	g := NewLinearGradient(4, 4, 4, 4, Pad,
		Stop{0, color.RGBA{9, 9, 9, 255}}, Stop{1, color.RGBA{200, 0, 0, 255}})
	if got := g.ColorAt(100, 100); got != (color.RGBA{9, 9, 9, 255}) {
		t.Fatalf("degenerate linear = %v, want first stop", got)
	}
}

func TestRadialGradient(t *testing.T) {
	g := NewRadialGradient(0, 0, 10, Pad,
		Stop{0, color.RGBA{0, 0, 0, 255}}, Stop{1, color.RGBA{100, 100, 100, 255}})
	if got := g.ColorAt(0, 0); got != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("centre = %v, want black", got)
	}
	if got := g.ColorAt(10, 0); got != (color.RGBA{100, 100, 100, 255}) {
		t.Fatalf("edge = %v, want end", got)
	}
	if got := g.ColorAt(5, 0); got != (color.RGBA{50, 50, 50, 255}) {
		t.Fatalf("half-radius = %v, want mid", got)
	}
}

func TestRadialGradientDegenerate(t *testing.T) {
	// Zero radius: every point sits at the edge (last stop).
	g := NewRadialGradient(0, 0, 0, Pad,
		Stop{0, color.RGBA{0, 0, 0, 255}}, Stop{1, color.RGBA{7, 7, 7, 255}})
	if got := g.ColorAt(3, 4); got != (color.RGBA{7, 7, 7, 255}) {
		t.Fatalf("degenerate radial = %v, want last stop", got)
	}
}
