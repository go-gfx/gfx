// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package geometry

import "testing"

func TestPointArithmetic(t *testing.T) {
	p := Pt(3, 4)
	if got := p.Add(Pt(1, 2)); got != (Point{4, 6}) {
		t.Fatalf("Add = %v, want {4 6}", got)
	}
	if got := p.Sub(Pt(1, 2)); got != (Point{2, 2}) {
		t.Fatalf("Sub = %v, want {2 2}", got)
	}
	if got := p.Mul(2); got != (Point{6, 8}) {
		t.Fatalf("Mul = %v, want {6 8}", got)
	}
}

func TestRectangleCanonicalises(t *testing.T) {
	// Ends given out of order on BOTH axes must be swapped.
	r := Rectangle(3, 5, 1, 2)
	if r.Min != (Point{1, 2}) || r.Max != (Point{3, 5}) {
		t.Fatalf("Rectangle = %v, want {{1 2} {3 5}}", r)
	}
	// Already-ordered ends are preserved.
	if r2 := Rectangle(1, 2, 3, 5); r2 != r {
		t.Fatalf("ordered Rectangle = %v, want %v", r2, r)
	}
	// Canon repairs an inverted literal.
	inv := Rect{Point{3, 5}, Point{1, 2}}
	if inv.Canon() != r {
		t.Fatalf("Canon = %v, want %v", inv.Canon(), r)
	}
}

func TestRectDims(t *testing.T) {
	r := Rectangle(1, 2, 4, 8)
	if r.Dx() != 3 {
		t.Fatalf("Dx = %v, want 3", r.Dx())
	}
	if r.Dy() != 6 {
		t.Fatalf("Dy = %v, want 6", r.Dy())
	}
}

func TestRectEmpty(t *testing.T) {
	if !(Rect{Point{2, 0}, Point{1, 5}}).Empty() {
		t.Fatal("x-inverted rect should be empty")
	}
	if !(Rect{Point{0, 2}, Point{5, 1}}).Empty() {
		t.Fatal("y-inverted rect should be empty")
	}
	if Rectangle(0, 0, 1, 1).Empty() {
		t.Fatal("unit rect should not be empty")
	}
}

func TestRectContains(t *testing.T) {
	r := Rectangle(0, 0, 10, 10)
	if !r.Contains(Pt(5, 5)) {
		t.Fatal("interior point should be contained")
	}
	if !r.Contains(Pt(0, 10)) {
		t.Fatal("edge point should be contained")
	}
	if r.Contains(Pt(11, 5)) {
		t.Fatal("outside point should not be contained")
	}
}

func TestRectUnion(t *testing.T) {
	a := Rectangle(0, 0, 2, 2)
	b := Rectangle(1, 1, 4, 5)
	if got := a.Union(b); got != Rectangle(0, 0, 4, 5) {
		t.Fatalf("Union = %v, want {{0 0} {4 5}}", got)
	}
	// An empty operand contributes nothing.
	empty := Rect{}
	if got := empty.Union(b); got != b {
		t.Fatalf("empty.Union(b) = %v, want b", got)
	}
	if got := a.Union(empty); got != a {
		t.Fatalf("a.Union(empty) = %v, want a", got)
	}
}

func TestRectIntersect(t *testing.T) {
	a := Rectangle(0, 0, 3, 3)
	b := Rectangle(1, 1, 5, 2)
	if got := a.Intersect(b); got != Rectangle(1, 1, 3, 2) {
		t.Fatalf("Intersect = %v, want {{1 1} {3 2}}", got)
	}
	// Disjoint rectangles intersect to the zero (empty) rect.
	c := Rectangle(10, 10, 12, 12)
	if got := a.Intersect(c); got != (Rect{}) {
		t.Fatalf("disjoint Intersect = %v, want zero rect", got)
	}
}
