// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package geometry

import "math"

// Matrix is a 2-D affine transform stored as the six meaningful entries of a
// 3x3 homogeneous matrix (the bottom row is always [0 0 1]):
//
//	| Xx Yx X0 |        x' = Xx*x + Yx*y + X0
//	| Xy Yy Y0 |        y' = Xy*x + Yy*y + Y0
//	|  0  0  1 |
//
// It maps a [Point] with [Matrix.TransformPoint]. Transforms compose with
// [Matrix.Mul]: m.Mul(n) is the transform that applies n first and then m, so
// m.Mul(n).TransformPoint(p) equals m.TransformPoint(n.TransformPoint(p)).
// A Matrix is a small value type, passed and returned by value.
type Matrix struct {
	Xx, Yx, X0 float64
	Xy, Yy, Y0 float64
}

// Identity returns the transform that leaves every point unchanged.
func Identity() Matrix {
	return Matrix{
		1, 0, 0,
		0, 1, 0,
	}
}

// Translate returns the transform that shifts every point by (tx, ty).
func Translate(tx, ty float64) Matrix {
	return Matrix{
		1, 0, tx,
		0, 1, ty,
	}
}

// Scale returns the transform that scales the axes by (sx, sy) about the origin.
func Scale(sx, sy float64) Matrix {
	return Matrix{
		sx, 0, 0,
		0, sy, 0,
	}
}

// Rotate returns the transform that rotates about the origin by theta radians,
// counter-clockwise in a y-down (screen) coordinate system.
func Rotate(theta float64) Matrix {
	s, c := math.Sincos(theta)
	return Matrix{
		c, -s, 0,
		s, c, 0,
	}
}

// Shear returns the transform that shears by (shx, shy): x gains shx*y and y
// gains shy*x.
func Shear(shx, shy float64) Matrix {
	return Matrix{
		1, shx, 0,
		shy, 1, 0,
	}
}

// Mul returns the composed transform m*n: the one that applies n first and then
// m to a point.
func (m Matrix) Mul(n Matrix) Matrix {
	return Matrix{
		Xx: m.Xx*n.Xx + m.Yx*n.Xy,
		Yx: m.Xx*n.Yx + m.Yx*n.Yy,
		X0: m.Xx*n.X0 + m.Yx*n.Y0 + m.X0,
		Xy: m.Xy*n.Xx + m.Yy*n.Xy,
		Yy: m.Xy*n.Yx + m.Yy*n.Yy,
		Y0: m.Xy*n.X0 + m.Yy*n.Y0 + m.Y0,
	}
}

// TransformPoint returns p mapped through the transform.
func (m Matrix) TransformPoint(p Point) Point {
	return Point{
		X: m.Xx*p.X + m.Yx*p.Y + m.X0,
		Y: m.Xy*p.X + m.Yy*p.Y + m.Y0,
	}
}

// TransformRect returns the axis-aligned bounding box of r's four corners after
// they are mapped through the transform. Under a rotation or shear the mapped
// rectangle is no longer axis-aligned, so this reports the tight enclosing
// upright box rather than the true (rotated) quadrilateral.
func (m Matrix) TransformRect(r Rect) Rect {
	c0 := m.TransformPoint(Point{r.Min.X, r.Min.Y})
	c1 := m.TransformPoint(Point{r.Max.X, r.Min.Y})
	c2 := m.TransformPoint(Point{r.Max.X, r.Max.Y})
	c3 := m.TransformPoint(Point{r.Min.X, r.Max.Y})
	minX := math.Min(math.Min(c0.X, c1.X), math.Min(c2.X, c3.X))
	minY := math.Min(math.Min(c0.Y, c1.Y), math.Min(c2.Y, c3.Y))
	maxX := math.Max(math.Max(c0.X, c1.X), math.Max(c2.X, c3.X))
	maxY := math.Max(math.Max(c0.Y, c1.Y), math.Max(c2.Y, c3.Y))
	return Rect{Point{minX, minY}, Point{maxX, maxY}}
}

// Determinant returns the determinant of the transform's linear part
// (Xx*Yy - Yx*Xy). Its magnitude is the area-scaling factor; a zero value means
// the transform collapses the plane and cannot be inverted.
func (m Matrix) Determinant() float64 { return m.Xx*m.Yy - m.Yx*m.Xy }

// IsInvertible reports whether the transform has a non-zero determinant and can
// therefore be inverted.
func (m Matrix) IsInvertible() bool { return m.Determinant() != 0 }

// Invert returns the inverse transform and ok=true, or the zero Matrix and
// ok=false when the transform is singular (zero determinant). When ok is true,
// inv.Mul(m) is the identity up to floating-point rounding.
func (m Matrix) Invert() (inv Matrix, ok bool) {
	det := m.Determinant()
	if det == 0 {
		return Matrix{}, false
	}
	id := 1 / det
	ixx := m.Yy * id
	iyx := -m.Yx * id
	ixy := -m.Xy * id
	iyy := m.Xx * id
	return Matrix{
		Xx: ixx, Yx: iyx, X0: -(ixx*m.X0 + iyx*m.Y0),
		Xy: ixy, Yy: iyy, Y0: -(ixy*m.X0 + iyy*m.Y0),
	}, true
}
