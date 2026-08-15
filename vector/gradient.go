// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

import (
	"image/color"
	"math"
	"sort"
)

// A Paint is a per-pixel colour source: given a point in surface coordinates it
// returns the straight-alpha colour to lay down there. [Composite] pushes a
// Paint through a coverage grid onto a pixel buffer, so the rasterizer stays
// colour-agnostic while gradients and solid fills plug in as Paints.
type Paint interface {
	// ColorAt returns the straight-alpha colour at surface point (x, y).
	ColorAt(x, y float64) color.RGBA
}

// SolidPaint is a [Paint] that returns one flat colour everywhere.
type SolidPaint struct{ Color color.RGBA }

// ColorAt returns the flat colour, ignoring position.
func (s SolidPaint) ColorAt(_, _ float64) color.RGBA { return s.Color }

// SpreadMethod decides the colour of points whose gradient parameter falls
// outside the [0, 1] range spanned by the stops.
type SpreadMethod int

const (
	// Pad clamps out-of-range parameters, extending the end stops' colours.
	Pad SpreadMethod = iota
	// Repeat tiles the gradient, wrapping the parameter modulo 1.
	Repeat
	// Reflect mirrors the gradient on every other tile (a triangle wave).
	Reflect
)

// Stop is one colour stop of a gradient at parameter Offset (nominally in
// [0, 1]).
type Stop struct {
	Offset float64
	Color  color.RGBA
}

// normStops returns a sorted (by ascending Offset) copy of stops so a gradient
// never mutates or depends on the caller's slice order.
func normStops(stops []Stop) []Stop {
	out := append([]Stop(nil), stops...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Offset < out[j].Offset })
	return out
}

// applySpread maps an arbitrary gradient parameter t into [0, 1] under method.
func applySpread(t float64, method SpreadMethod) float64 {
	switch method {
	case Repeat:
		t -= math.Floor(t)
		return t
	case Reflect:
		t = math.Mod(t, 2)
		if t < 0 {
			t += 2
		}
		if t > 1 {
			t = 2 - t
		}
		return t
	default: // Pad
		if t < 0 {
			return 0
		}
		if t > 1 {
			return 1
		}
		return t
	}
}

// sampleStops returns the colour of the stop list at parameter t (assumed in
// [0, 1]), linearly interpolating each straight-alpha channel between the two
// bracketing stops. An empty list is transparent; a single stop is that colour.
func sampleStops(stops []Stop, t float64) color.RGBA {
	switch len(stops) {
	case 0:
		return color.RGBA{}
	case 1:
		return stops[0].Color
	}
	if t <= stops[0].Offset {
		return stops[0].Color
	}
	last := len(stops) - 1
	if t >= stops[last].Offset {
		return stops[last].Color
	}
	// Find the segment [stops[i], stops[i+1]] that brackets t. The loop advances
	// only while stops[i+1].Offset < t and stops before t are strictly less, so
	// on exit stops[i].Offset < t <= stops[i+1].Offset and the span is > 0.
	i := 0
	for i < last && stops[i+1].Offset < t {
		i++
	}
	a, b := stops[i], stops[i+1]
	f := (t - a.Offset) / (b.Offset - a.Offset)
	return color.RGBA{
		R: lerp8(a.Color.R, b.Color.R, f),
		G: lerp8(a.Color.G, b.Color.G, f),
		B: lerp8(a.Color.B, b.Color.B, f),
		A: lerp8(a.Color.A, b.Color.A, f),
	}
}

// lerp8 linearly interpolates between 8-bit channels u and v by fraction f in
// [0, 1], rounding to the nearest integer.
func lerp8(u, v uint8, f float64) uint8 {
	return uint8(float64(u) + (float64(v)-float64(u))*f + 0.5)
}

// LinearGradient is a [Paint] whose colour varies along the axis from (X0, Y0)
// to (X1, Y1): a point's parameter is its projection onto that axis, 0 at the
// start and 1 at the end, extended past the ends by the Spread method.
type LinearGradient struct {
	X0, Y0, X1, Y1 float64
	Spread         SpreadMethod
	stops          []Stop
}

// NewLinearGradient returns a linear gradient along (x0,y0)->(x1,y1) with the
// given spread method and colour stops (copied and sorted by offset).
func NewLinearGradient(x0, y0, x1, y1 float64, spread SpreadMethod, stops ...Stop) *LinearGradient {
	return &LinearGradient{X0: x0, Y0: y0, X1: x1, Y1: y1, Spread: spread, stops: normStops(stops)}
}

// ColorAt returns the gradient colour at (x, y).
func (g *LinearGradient) ColorAt(x, y float64) color.RGBA {
	dx, dy := g.X1-g.X0, g.Y1-g.Y0
	d2 := dx*dx + dy*dy
	var t float64
	if d2 == 0 {
		t = 0 // degenerate axis: a single point, all one colour
	} else {
		t = ((x-g.X0)*dx + (y-g.Y0)*dy) / d2
	}
	return sampleStops(g.stops, applySpread(t, g.Spread))
}

// RadialGradient is a [Paint] whose colour varies with distance from the centre
// (CX, CY): a point's parameter is its distance divided by radius R, 0 at the
// centre and 1 on the circle, extended past the edge by the Spread method.
type RadialGradient struct {
	CX, CY, R float64
	Spread    SpreadMethod
	stops     []Stop
}

// NewRadialGradient returns a radial gradient centred at (cx,cy) with radius r,
// the given spread method and colour stops (copied and sorted by offset).
func NewRadialGradient(cx, cy, r float64, spread SpreadMethod, stops ...Stop) *RadialGradient {
	return &RadialGradient{CX: cx, CY: cy, R: r, Spread: spread, stops: normStops(stops)}
}

// ColorAt returns the gradient colour at (x, y).
func (g *RadialGradient) ColorAt(x, y float64) color.RGBA {
	if g.R <= 0 {
		return sampleStops(g.stops, 1) // degenerate: everything sits at the edge
	}
	t := math.Hypot(x-g.CX, y-g.CY) / g.R
	return sampleStops(g.stops, applySpread(t, g.Spread))
}
