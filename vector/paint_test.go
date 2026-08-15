// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

import (
	"image/color"
	"testing"

	"github.com/go-gfx/gfx/raster"
)

// Composite lays an opaque solid through a coverage grid, skips zero-coverage
// pixels, and clamps over-unity coverage.
func TestCompositeSolidOverTransparent(t *testing.T) {
	dst := raster.New(3, 1)
	// cov: pixel 0 fully covered, pixel 1 uncovered, pixel 2 over-unity (clamped).
	cov := []float64{1, 0, 2}
	Composite(dst, cov, 0, 0, 3, 1, SolidPaint{color.RGBA{255, 0, 0, 255}})
	if got := dst.At(0, 0); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("covered pixel = %v, want opaque red", got)
	}
	if got := dst.At(1, 0); got != (color.RGBA{0, 0, 0, 0}) {
		t.Fatalf("uncovered pixel = %v, want transparent", got)
	}
	if got := dst.At(2, 0); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("over-unity pixel = %v, want clamped opaque red", got)
	}
}

// A transparent source leaves the destination untouched (the sA<=0 early out).
func TestCompositeTransparentSource(t *testing.T) {
	dst := raster.New(1, 1)
	dst.Set(0, 0, color.RGBA{10, 20, 30, 255})
	Composite(dst, []float64{1}, 0, 0, 1, 1, SolidPaint{color.RGBA{0, 0, 0, 0}})
	if got := dst.At(0, 0); got != (color.RGBA{10, 20, 30, 255}) {
		t.Fatalf("transparent-source result = %v, want unchanged", got)
	}
}

// Half-covered opaque red over opaque blue is the expected straight-alpha
// source-over mix.
func TestCompositeSourceOverBlend(t *testing.T) {
	dst := raster.New(1, 1)
	dst.Set(0, 0, color.RGBA{0, 0, 255, 255})
	Composite(dst, []float64{0.5}, 0, 0, 1, 1, SolidPaint{color.RGBA{255, 0, 0, 255}})
	if got := dst.At(0, 0); got != (color.RGBA{128, 0, 128, 255}) {
		t.Fatalf("blended pixel = %v, want {128 0 128 255}", got)
	}
}

// Composite paints a linear gradient so a consumer sees position-varying colour.
func TestCompositeGradient(t *testing.T) {
	dst := raster.New(3, 1)
	g := NewLinearGradient(0, 0, 3, 0, Pad,
		Stop{0, color.RGBA{0, 0, 0, 255}}, Stop{1, color.RGBA{255, 255, 255, 255}})
	Composite(dst, []float64{1, 1, 1}, 0, 0, 3, 1, g)
	// Sampled at pixel centres 0.5, 1.5, 2.5 over a width-3 axis: increasing grey.
	if a, b, c := dst.At(0, 0).R, dst.At(1, 0).R, dst.At(2, 0).R; !(a < b && b < c) {
		t.Fatalf("gradient reds not increasing: %d %d %d", a, b, c)
	}
}

// round8 rounds to nearest and clamps to [0,255] across its whole domain,
// including the negative guard the compositing path never hits.
func TestRound8(t *testing.T) {
	cases := []struct {
		in   float64
		want uint8
	}{
		{-5, 0}, {10.4, 10}, {10.6, 11}, {254.6, 255}, {300, 255},
	}
	for _, c := range cases {
		if got := round8(c.in); got != c.want {
			t.Fatalf("round8(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
