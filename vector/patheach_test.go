// Copyright (c) the go-gfx authors.
// SPDX-License-Identifier: BSD-3-Clause

package vector

import "testing"

// allCommands builds a path using every command, with curves that are not
// straight lines and a sub-path that is closed and one that is not.
func allCommands() *Path {
	return NewPath().
		MoveTo(10, 10).
		LineTo(60, 12).
		QuadTo(80, 40, 55, 70).
		CubicTo(40, 90, 20, 85, 12, 60).
		Close().
		MoveTo(100, 20).
		LineTo(140, 20).
		LineTo(140, 60)
}

func TestEachReportsEveryCommandInOrder(t *testing.T) {
	var got []PathOp
	var pts [][][2]float64
	allCommands().Each(func(op PathOp, p ...[2]float64) {
		got = append(got, op)
		pts = append(pts, append([][2]float64(nil), p...))
	})

	want := []PathOp{OpMove, OpLine, OpQuad, OpCubic, OpClose, OpMove, OpLine, OpLine}
	if len(got) != len(want) {
		t.Fatalf("%d commands, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("command %d is %v, want %v", i, got[i], want[i])
		}
	}
	// The point counts are the contract: a consumer writing PDF or SVG indexes
	// them positionally.
	wantN := []int{1, 1, 2, 3, 0, 1, 1, 1}
	for i := range wantN {
		if len(pts[i]) != wantN[i] {
			t.Errorf("command %d (%v) carries %d points, want %d", i, got[i], len(pts[i]), wantN[i])
		}
	}
	// And the values arrive in the builder's own order: control(s), then target.
	if pts[2][0] != [2]float64{80, 40} || pts[2][1] != [2]float64{55, 70} {
		t.Errorf("QuadTo came back as %v", pts[2])
	}
	if pts[3][0] != [2]float64{40, 90} || pts[3][1] != [2]float64{20, 85} || pts[3][2] != [2]float64{12, 60} {
		t.Errorf("CubicTo came back as %v", pts[3])
	}
}

// TestEachRoundTripsThroughTheBuilder is the property that matters: a path
// read back and rebuilt command by command is the path that was read.
//
// ⛔ It is checked by RASTERISING both and comparing the coverage, not by
// comparing the recorded commands with themselves — which would only prove
// that Each is consistent with Each. The rasteriser is the thing that has to
// agree, and it is written by somebody who never heard of this method.
func TestEachRoundTripsThroughTheBuilder(t *testing.T) {
	src := allCommands()

	rebuilt := NewPath()
	src.Each(func(op PathOp, p ...[2]float64) {
		switch op {
		case OpMove:
			rebuilt.MoveTo(p[0][0], p[0][1])
		case OpLine:
			rebuilt.LineTo(p[0][0], p[0][1])
		case OpQuad:
			rebuilt.QuadTo(p[0][0], p[0][1], p[1][0], p[1][1])
		case OpCubic:
			rebuilt.CubicTo(p[0][0], p[0][1], p[1][0], p[1][1], p[2][0], p[2][1])
		case OpClose:
			rebuilt.Close()
		}
	})

	if rebuilt.Len() != src.Len() {
		t.Fatalf("rebuilt has %d commands, the original %d", rebuilt.Len(), src.Len())
	}
	for _, rule := range []FillRule{NonZero, EvenOdd} {
		var ra, rb Rasterizer
		covA, oxA, oyA, wA, hA, okA := ra.Fill(src, rule, 200, 120)
		covB, oxB, oyB, wB, hB, okB := rb.Fill(rebuilt, rule, 200, 120)
		if okA != okB || oxA != oxB || oyA != oyB || wA != wB || hA != hB {
			t.Fatalf("rule %v: boxes differ: (%v %d %d %d %d) vs (%v %d %d %d %d)",
				rule, okA, oxA, oyA, wA, hA, okB, oxB, oyB, wB, hB)
		}
		if len(covA) != len(covB) {
			t.Fatalf("rule %v: coverage lengths differ", rule)
		}
		for i := range covA {
			if covA[i] != covB[i] {
				t.Fatalf("rule %v: coverage differs at %d: %v vs %v", rule, i, covA[i], covB[i])
			}
		}
	}
}

// TestEachKeepsCurvesAsCurves is the whole reason this is not built on
// flatten: a consumer writing a vector format needs the beziers. A path whose
// curves had been flattened would come back as a long run of OpLine, and that
// is exactly the loss that makes writing PDF or SVG pointless.
func TestEachKeepsCurvesAsCurves(t *testing.T) {
	curves := 0
	lines := 0
	NewPath().MoveTo(0, 0).CubicTo(10, 0, 10, 10, 0, 10).QuadTo(-5, 5, 0, 0).
		Each(func(op PathOp, _ ...[2]float64) {
			switch op {
			case OpCubic, OpQuad:
				curves++
			case OpLine:
				lines++
			}
		})
	if curves != 2 {
		t.Errorf("%d curve commands, want 2", curves)
	}
	if lines != 0 {
		t.Errorf("%d line commands, want none — the curves were flattened", lines)
	}
}

func TestEachOnAnEmptyPathDoesNothing(t *testing.T) {
	n := 0
	NewPath().Each(func(PathOp, ...[2]float64) { n++ })
	if n != 0 {
		t.Errorf("an empty path reported %d commands", n)
	}
	if got := NewPath().Len(); got != 0 {
		t.Errorf("an empty path has length %d", got)
	}
}
