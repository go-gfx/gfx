// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package geometry

import (
	"math"
	"testing"
)

// approxPt reports whether two points agree within a small tolerance.
func approxPt(a, b Point) bool {
	return math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) < 1e-9
}

func TestIdentity(t *testing.T) {
	p := Pt(3, -4)
	if got := Identity().TransformPoint(p); got != p {
		t.Fatalf("Identity mapped %v to %v", p, got)
	}
}

func TestTranslateScaleShear(t *testing.T) {
	if got := Translate(5, -2).TransformPoint(Pt(1, 1)); !approxPt(got, Pt(6, -1)) {
		t.Fatalf("Translate = %v, want {6 -1}", got)
	}
	if got := Scale(2, 3).TransformPoint(Pt(1, 1)); !approxPt(got, Pt(2, 3)) {
		t.Fatalf("Scale = %v, want {2 3}", got)
	}
	if got := Shear(1, 0).TransformPoint(Pt(1, 1)); !approxPt(got, Pt(2, 1)) {
		t.Fatalf("Shear = %v, want {2 1}", got)
	}
}

func TestRotate(t *testing.T) {
	// A quarter turn (y-down, CCW) sends the +x unit vector to +y.
	if got := Rotate(math.Pi / 2).TransformPoint(Pt(1, 0)); !approxPt(got, Pt(0, 1)) {
		t.Fatalf("Rotate 90 of (1,0) = %v, want {0 1}", got)
	}
}

func TestMulComposesRightToLeft(t *testing.T) {
	m := Translate(10, 0)
	n := Scale(2, 2)
	p := Pt(1, 1)
	// m.Mul(n) applies n first, then m.
	want := m.TransformPoint(n.TransformPoint(p)) // scale then translate -> (12, 2)
	if !approxPt(want, Pt(12, 2)) {
		t.Fatalf("manual compose = %v, want {12 2}", want)
	}
	if got := m.Mul(n).TransformPoint(p); !approxPt(got, want) {
		t.Fatalf("Mul compose = %v, want %v", got, want)
	}
}

func TestTransformRect(t *testing.T) {
	// Rotating the unit square 90 degrees yields the tight upright box
	// [-1,0] x [0,1].
	got := Rotate(math.Pi / 2).TransformRect(Rectangle(0, 0, 1, 1))
	want := Rectangle(-1, 0, 0, 1)
	if !approxPt(got.Min, want.Min) || !approxPt(got.Max, want.Max) {
		t.Fatalf("TransformRect = %v, want %v", got, want)
	}
}

func TestDeterminantAndInvertible(t *testing.T) {
	if d := Scale(2, 3).Determinant(); math.Abs(d-6) > 1e-9 {
		t.Fatalf("Determinant = %v, want 6", d)
	}
	if !Scale(2, 3).IsInvertible() {
		t.Fatal("non-degenerate scale should be invertible")
	}
	if Scale(0, 1).IsInvertible() {
		t.Fatal("collapsing scale should not be invertible")
	}
}

func TestInvertRoundTrip(t *testing.T) {
	m := Translate(4, -3).Mul(Rotate(0.7)).Mul(Scale(2, 1.5))
	inv, ok := m.Invert()
	if !ok {
		t.Fatal("expected invertible matrix")
	}
	p := Pt(2.5, -1.25)
	if got := inv.TransformPoint(m.TransformPoint(p)); !approxPt(got, p) {
		t.Fatalf("inverse round trip = %v, want %v", got, p)
	}
	// inv.Mul(m) is the identity up to rounding.
	id := inv.Mul(m)
	if got := id.TransformPoint(p); !approxPt(got, p) {
		t.Fatalf("inv.Mul(m) not identity: mapped %v to %v", p, got)
	}
}

func TestInvertSingular(t *testing.T) {
	if _, ok := Scale(0, 0).Invert(); ok {
		t.Fatal("singular matrix reported invertible")
	}
}
