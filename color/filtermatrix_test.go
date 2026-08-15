// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import (
	"math"
	"testing"
)

// applyClose reports whether m applied to (r,g,b) matches (wr,wg,wb) within tol.
func applyClose(m ColorMatrix, r, g, b, wr, wg, wb, tol float64) bool {
	or, og, ob := m.Apply(r, g, b)
	return approx(or, wr, tol) && approx(og, wg, tol) && approx(ob, wb, tol)
}

// Brightness is a per-channel multiply: identity at 1, doubles at 2, zeroes at 0.
func TestBrightnessMatrix(t *testing.T) {
	if !applyClose(BrightnessMatrix(1), 0.2, 0.4, 0.6, 0.2, 0.4, 0.6, 1e-12) {
		t.Error("brightness(1) is not identity")
	}
	if !applyClose(BrightnessMatrix(2), 0.1, 0.2, 0.3, 0.2, 0.4, 0.6, 1e-12) {
		t.Error("brightness(2) does not double")
	}
	if !applyClose(BrightnessMatrix(0), 0.5, 0.5, 0.5, 0, 0, 0, 1e-12) {
		t.Error("brightness(0) does not zero")
	}
}

// Invert(0) is identity; invert(1) is a full negative; invert(0.5) is flat grey.
func TestInvertMatrix(t *testing.T) {
	if !applyClose(InvertMatrix(0), 0.2, 0.5, 0.9, 0.2, 0.5, 0.9, 1e-12) {
		t.Error("invert(0) is not identity")
	}
	if !applyClose(InvertMatrix(1), 0.2, 0.5, 0.9, 0.8, 0.5, 0.1, 1e-12) {
		t.Error("invert(1) is not a full negative")
	}
	// invert(0.5) maps every input to 0.5 (1-2a = 0, offset 0.5).
	if !applyClose(InvertMatrix(0.5), 0.2, 0.5, 0.9, 0.5, 0.5, 0.5, 1e-12) {
		t.Error("invert(0.5) is not uniform grey")
	}
}

// Saturate(1) is identity; saturate(0) collapses to Rec.601 luminance grey.
func TestSaturateMatrix(t *testing.T) {
	if !applyClose(SaturateMatrix(1), 0.2, 0.5, 0.9, 0.2, 0.5, 0.9, 1e-12) {
		t.Error("saturate(1) is not identity")
	}
	// Pure red at saturate(0) is the luminance coefficient on all channels.
	l := Rec601Luma[0]
	if !applyClose(SaturateMatrix(0), 1, 0, 0, l, l, l, 1e-12) {
		t.Error("saturate(0) does not collapse red to Rec.601 luma")
	}
	// A grey is invariant under any saturation factor (rows sum to 1).
	if !applyClose(SaturateMatrix(2.5), 0.4, 0.4, 0.4, 0.4, 0.4, 0.4, 1e-12) {
		t.Error("saturate(2.5) does not preserve grey")
	}
}

// Grayscale(0) is identity; grayscale(1) is Rec.709 luminance on every channel.
func TestGrayscaleMatrix(t *testing.T) {
	if !applyClose(GrayscaleMatrix(0), 0.2, 0.5, 0.9, 0.2, 0.5, 0.9, 1e-12) {
		t.Error("grayscale(0) is not identity")
	}
	r, g, b := 0.3, 0.6, 0.1
	y := Rec709Luma[0]*r + Rec709Luma[1]*g + Rec709Luma[2]*b
	if !applyClose(GrayscaleMatrix(1), r, g, b, y, y, y, 1e-12) {
		t.Error("grayscale(1) does not map to Rec.709 luma")
	}
}

// Sepia(0) is identity; sepia(1) matches the spec's fixed matrix on white.
func TestSepiaMatrix(t *testing.T) {
	if !applyClose(SepiaMatrix(0), 0.2, 0.5, 0.9, 0.2, 0.5, 0.9, 1e-12) {
		t.Error("sepia(0) is not identity")
	}
	// White through full sepia is each row's coefficient sum.
	wr := 0.393 + 0.769 + 0.189
	wg := 0.349 + 0.686 + 0.168
	wb := 0.272 + 0.534 + 0.131
	if !applyClose(SepiaMatrix(1), 1, 1, 1, wr, wg, wb, 1e-12) {
		t.Error("sepia(1) does not match the spec matrix on white")
	}
}

// Hue-rotate(0) is exact identity; a full turn returns to the start; and a grey
// is invariant under any rotation (each row sums to 1).
func TestHueRotateMatrix(t *testing.T) {
	if !applyClose(HueRotateMatrix(0), 0.2, 0.5, 0.9, 0.2, 0.5, 0.9, 1e-15) {
		t.Error("hue-rotate(0) is not identity")
	}
	if !applyClose(HueRotateMatrix(2*math.Pi), 0.2, 0.5, 0.9, 0.2, 0.5, 0.9, 1e-9) {
		t.Error("hue-rotate(2pi) is not a no-op")
	}
	if !applyClose(HueRotateMatrix(1.2), 0.4, 0.4, 0.4, 0.4, 0.4, 0.4, 1e-12) {
		t.Error("hue-rotate does not preserve grey")
	}
	// A non-trivial rotation actually changes a chromatic colour.
	or, og, ob := HueRotateMatrix(math.Pi/2).Apply(0.9, 0.1, 0.1)
	if approx(or, 0.9, 1e-3) && approx(og, 0.1, 1e-3) && approx(ob, 0.1, 1e-3) {
		t.Error("hue-rotate(90deg) left a chromatic colour unchanged")
	}
}
