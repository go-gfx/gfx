// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// Package geometry is go-gfx's floating-point 2-D geometry substrate: a [Point],
// an axis-aligned [Rect], and a 2-D affine [Matrix] (translate / scale / rotate /
// shear, composed, inverted, and applied to points and rectangles).
//
// Everything here is in continuous float64 coordinates, deliberately distinct
// from the standard library's integer image.Point / image.Rectangle (which raster
// keeps using for pixel-grid bounds): a vector transform needs sub-pixel
// positions, so this layer never rounds until a consumer asks it to. The types
// are small value types passed and returned by value.
package geometry

import "math"

// Point is a location in continuous 2-D space.
type Point struct{ X, Y float64 }

// Pt is shorthand for Point{x, y}.
func Pt(x, y float64) Point { return Point{x, y} }

// Add returns the vector sum p+q.
func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }

// Sub returns the vector difference p-q.
func (p Point) Sub(q Point) Point { return Point{p.X - q.X, p.Y - q.Y} }

// Mul returns p scaled by s.
func (p Point) Mul(s float64) Point { return Point{p.X * s, p.Y * s} }

// Rect is an axis-aligned rectangle spanning [Min.X, Max.X] x [Min.Y, Max.Y]. It
// is well-formed ("canonical") when Min <= Max on both axes; the query and set
// methods assume canonical input, and [Rect.Canon] repairs one that is not.
type Rect struct{ Min, Max Point }

// Rectangle returns the canonical Rect with the given corner coordinates,
// swapping ends as needed so Min <= Max on each axis.
func Rectangle(x0, y0, x1, y1 float64) Rect {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	return Rect{Point{x0, y0}, Point{x1, y1}}
}

// Dx returns the rectangle's width, Max.X-Min.X.
func (r Rect) Dx() float64 { return r.Max.X - r.Min.X }

// Dy returns the rectangle's height, Max.Y-Min.Y.
func (r Rect) Dy() float64 { return r.Max.Y - r.Min.Y }

// Empty reports whether the rectangle encloses no area (zero or inverted on
// either axis).
func (r Rect) Empty() bool { return r.Min.X >= r.Max.X || r.Min.Y >= r.Max.Y }

// Canon returns the rectangle with its corners ordered so Min <= Max on each
// axis.
func (r Rect) Canon() Rect { return Rectangle(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y) }

// Contains reports whether p lies within the closed rectangle (edges included).
func (r Rect) Contains(p Point) bool {
	return p.X >= r.Min.X && p.X <= r.Max.X && p.Y >= r.Min.Y && p.Y <= r.Max.Y
}

// Union returns the smallest rectangle that contains both r and s. An empty
// operand contributes nothing, so the union of an empty rectangle with s is s.
func (r Rect) Union(s Rect) Rect {
	if r.Empty() {
		return s
	}
	if s.Empty() {
		return r
	}
	return Rect{
		Point{math.Min(r.Min.X, s.Min.X), math.Min(r.Min.Y, s.Min.Y)},
		Point{math.Max(r.Max.X, s.Max.X), math.Max(r.Max.Y, s.Max.Y)},
	}
}

// Intersect returns the overlap of r and s. When they do not overlap it returns
// the zero Rect (which is empty).
func (r Rect) Intersect(s Rect) Rect {
	out := Rect{
		Point{math.Max(r.Min.X, s.Min.X), math.Max(r.Min.Y, s.Min.Y)},
		Point{math.Min(r.Max.X, s.Max.X), math.Min(r.Max.Y, s.Max.Y)},
	}
	if out.Empty() {
		return Rect{}
	}
	return out
}
